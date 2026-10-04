# Task: F-multiwan-wiring-a — Multi-WAN agent wiring, part A: live link monitor + health-driven default route / weighted ECMP   (prepend 00-CONTEXT.md)

> Split of board row F-multiwan-wiring (10 h, D-176) into **-a (this, 6 h)** and **-b (8 h: per-member SNAT + sticky +
> dead-link clear, ABF pinning)**. -b starts after -a merges (shared `desired/multiwan*.go`, `test/topology/multiwan/**`).

## Goal
Make the running agent do multi-WAN routing in FAST MODE: start the `wanmon` monitor from the desired `routing.wanGroups`
with a prober that tests each member link **through VPP**, and install the default route (failover) or the weighted ECMP
default (balance) from the live selection — through the existing route family, no new binapi.
Reference behaviour: TNSR/pfSense "gateway groups" (failover tiers, weighted balance). Source: F-multiwan-host R7
BLOCKER 2 items 1+2 (`docs/status/tasks/F-multiwan-host-review-R7.md`), D-176, `docs/tech-debt.md` F-multiwan row.

## Inputs to read first
- `docs/status/tasks/F-multiwan-host.md` (what is built vs. what remains; topology; failover/balance drivers) and its
  evidence drivers `docs/status/tasks/F-multiwan-host-evidence/{setup,baseline,failover,balance,cleanup}.sh` — your
  acceptance reproduces their results with the **agent** doing the work.
- `apps/agent/internal/subsystems/wanmon/{monitor,store}.go` — `Monitor.Configure/Tick/Run`, `Prober` interface (monitor.go:31-35),
  `ProbeSpec`; `apps/agent/internal/subsystems/wanmon_register.go` (today: an empty store, nothing runs `Run`).
- `apps/agent/internal/multiwan/health.go` — hysteresis `State.Observe`, `FailoverActive`, `BalancePaths` (reuse; never reimplement).
- `apps/agent/internal/agent/rpc_wan.go` — `WanState` (R7 #15: empty 200 hides "nothing monitored").
- Contract (exists, no change): `packages/schema/src/domains/ext/multiwan.ts`, `packages/schema/src/semantic/multiwan.ts`,
  proto `WanGroup`/`WanMember`/`WanMonitor`/`WanState*` (`packages/proto/ngfw/v1/dataplane.proto` "----- F-multiwan -----").
- Route family: `apps/agent/internal/descriptors/core/route.go` (weighted paths, `core.RouteKey`), how `projection.go` adds
  static routes (`apps/agent/internal/agent/projection.go` routing.static block) and feature builders under anchors (:344-375).
- Probing constraints: VPP's ping API has no source interface/table, blocks the binary API for count×interval and is refused
  with worker threads (`apps/agent/internal/actions/vrf-static-ecmp/ping.go:19-23,55-57`); `apps/agent/binapi/arping/`
  (`Arping{Address, SwIfIndex, Repeat, Interval}` → `ReplyCount`) is interface-scoped. Names only from `apps/agent/binapi/`.
- Resync hook: `Wiring.RequestResync()` (`apps/agent/internal/subsystems/seams.go:190-200`).
- `docs/lab/shared-host-rules.md` §1, §2, §11, §12; `docs/user/network/multi-wan.md`.

## Contract changes
None expected. If you find a gap, follow the FEATURE-TEMPLATE contract rule (commit `contract(schema|proto): …` first on
`contract/F-multiwan-wiring-a`, `docs/status/tasks/F-multiwan-wiring-a-contract.md`, tell the manager in your questions file, keep going).

## Scope — build exactly this
1. **Prober** (`subsystems/wanmon/prober*.go`): a `wanmon.Prober` that tests a member link through VPP, sourced from that
   member's interface, bounded by `TimeoutMs`, never blocking the agent's apply path. Pick the mechanism, log the decision
   with its options in your status file (candidates: binapi `arping` on the member `sw_if_index` for the next hop; a probe
   from the member's linux-cp tap when one exists; others you find in binapi). The API-blocking ping binapi is not allowed
   in a periodic loop. A target the mechanism cannot test → member stays "unprobeable" (up), exactly as `probeable()` does.
2. **Run the monitor from desired state** (`wanmon_register.go`, `subsystems/wanmon/**`): on every reconcile hand the desired
   `routing.wanGroups` to `Monitor.Configure` (state of surviving members is preserved — already built), start one
   `Monitor.Run` per owner (stopped on agent shutdown/owner change; ctx-bounded; no goroutine leak — test it), and on a member
   up/down flip call `RequestResync()` so the route projection below re-runs. Members with `nextHop: dhcp|pppoe` have no
   static gateway: monitor them only when the group has an explicit monitor target, and do not route via them in this row
   (warning `multiwan.nexthop-runtime` with a pointer — the DHCP/PPPoE gateway hand-off is out of scope).
3. **Health-driven default route** (`desired/multiwan.go`, one call under a new `// wave-BC: F-multiwan-wiring-a` anchor in
   `agent/projection.go`): for each group, in the VRF of its members (all members of a group must share one VRF — agent-side
   error with pointer `/routing/wanGroups/<i>/members/<j>/interface` otherwise): failover → `0.0.0.0/0` (and `::/0` when the
   gateways are IPv6) via the `FailoverActive` member's gateway/interface; balance → one multipath default over
   `BalancePaths` with the configured weights; no healthy member → no default route from multiwan (state says so). Use the
   existing route family (core route KV). A `routing.static` default in the same VRF conflicts → projection error with the
   pointer, never two writers of one FIB entry. The objects you emit must not come back as `routing.static` drift
   (`GET /api/v1/state/drift` clean after commit). Unit tests with the fake VPP: failover selection, restore-to-primary
   (lower priority returns after `upAfter`), balance weights 3:1 and 1:1, member flip → resync → route paths change, no-healthy.
4. **WanState honesty** (`agent/rpc_wan.go`, R7 #15): distinguish "no groups configured" (empty, OK) from "groups configured
   but the monitor is not running" (`codes.Unavailable` with a clear message). The API already surfaces agent errors as
   `agentError` in `GET /api/v1/state/wan` (`apps/api/src/features/multiwan/multiwan.controller.ts:70-77`) — no API change.
5. **Slot evidence driver** (`test/topology/multiwan/`, Go module like `test/topology/host-acl-nftables`, `NGFW_INTEGRATION=1`):
   LAN + two WAN members in a slot VRF (table in N000–N999, never table 0). Use `tools/lab rig up w<N>` for LAN/WAN1; build
   WAN2 without a hand-made af_packet if you can (e.g. an agent-created VLAN sub-interface of the rig WAN with a VLAN in the
   WAN netns) — never `vppctl delete host-interface` (V24/D-101); classify reset on anything you create (D-185). Config goes in
   through the slot API (candidate → commit), never vppctl writes. Cut links at the netns end.
6. **Docs**: `docs/user/network/multi-wan.md` — replace the "does not happen yet" note for monitor + routing (NAT/ABF stay
   "pending F-multiwan-wiring-b"); CLI equivalent line.

## Acceptance (paste the evidence; logs as `.txt` under `docs/status/tasks/F-multiwan-wiring-a-evidence/`)
- [ ] Failover: cut WAN1 → `vppctl show ip fib table <T> 0.0.0.0/0` moves to WAN2 within `downAfter × interval + 2 s`
      (agent log + timestamps), LAN → test target replies via WAN2 (per-interface tx counters, no trace)
- [ ] Restore-to-primary: WAN1 back → route returns to WAN1 after `upAfter`; LAN replies via WAN1 (this FAILED in the host row — prove it)
- [ ] Balance: weighted ECMP 3:1 and 1:1 — ~1000 flows split within ±10 % (per-interface tx), done by the agent's route
- [ ] `GET /api/v1/state/wan` on the slot API shows live up/down/loss/latency and the active member (and `agentError` while no monitor runs)
- [ ] Agent-restart simulation (stop your agent, delete your prefixed default route via binapi, start it) → route back within 30 s
- [ ] Rollback of the wanGroups commit removes the multiwan default route (Retrieve output); `/state/drift` clean
- [ ] Validation failure: members of one group in two VRFs → 400 problem+json with the `pointer`
- [ ] `systemctl show vpp -p NRestarts` before/after every host step (pasted); own unit tests output; `tools/ci-slot.sh --base main` tail

## Out of scope (do not build)
Per-member SNAT, sticky sessions, dead-link session clear and ABF/PBR pinning (→ F-multiwan-wiring-b) · DHCP/PPPoE-learned
gateways for members (warning only) · new binapi or `apps/agent/binapi/` edits · schema/proto/UI changes (the Routing →
Multi-WAN screen exists) · alarms/webhooks on link flips · IPv6 RA/DHCPv6-PD per member · tuning probe rates or claiming
failover performance beyond the acceptance budget · anything under `apps/web/**`, `packages/ui-kit/**`.

## Open questions to surface, not to decide silently
- Which prober mechanism you chose and why (and what it cannot test, e.g. "internet beyond the gateway" with arping).
- If `RequestResync()` is too coarse for the failover budget on the shared host, say so with numbers before inventing a
  targeted re-apply path.

## Files you own
`apps/agent/internal/subsystems/wanmon/**` · `apps/agent/internal/subsystems/wanmon_register.go` ·
`apps/agent/internal/desired/multiwan*.go` (new) · `apps/agent/internal/agent/rpc_wan*.go` · `apps/agent/internal/agent/projection.go`
(one call line under `// wave-BC: F-multiwan-wiring-a`, listed under `## Shared hunks`) · `test/topology/multiwan/**` ·
`docs/user/network/multi-wan.md` · `docs/status/tasks/F-multiwan-wiring-a*`.

## Rules
- Files you own: the list above. Everything else is read-only; a needed edit elsewhere → `docs/status/tasks/F-multiwan-wiring-a-questions.md`.
- Shared VPP: slot prefix `w<N>` on every object, tables N000–N999 (`eval "$(tools/lab env <N>)"`); never restart or kill VPP;
  `timeout 10` on every vppctl; packet trace banned (D-128: no `trace add` / `show trace` / `clear trace`). Global VPP steps
  (none expected here) only inside the exclusive globals lock window (D-167). Daemons: none.
- D-210a: write tests for your change and get them passing in your package; paste the output; no full suite, no lint, no other
  packages' tests. Run them through `tools/heavy.sh` (D-224), e.g. from `apps/agent`: `../../tools/heavy.sh go test ./internal/subsystems/wanmon/... ./internal/desired/...`.
- Contracts: additive only — `contract(schema|proto): …` commits first, numbers only from `docs/status/wave-BC-numbers.md`;
  renaming/reshaping = PENDING (decision-policy #1).
- Finish: `tools/ci-slot.sh --base main` green (compile-only gate, D-220/D-222; never `tools/ci.sh` directly), commit on your
  branch, `docs/status/tasks/F-multiwan-wiring-a.md` with pasted real output (D-175 evidence rules), stop every process you started.
