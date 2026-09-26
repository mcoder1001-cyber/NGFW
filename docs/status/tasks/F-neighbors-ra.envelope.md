# TASK ENVELOPE — F-neighbors-ra
id: F-neighbors-ra   branch: task/F-neighbors-ra   worktree: /root/ngfw-wt/F-neighbors-ra   base: main@task/W-seed@8b7558e (SPECULATIVE, D-114/D-120: P08 fix round 2 still running — do NOT merge main until the manager tells you P08 has landed)   started: 2026-09-24T17:27
title: Wave A (day 7-9): ARP/ND table, proxy-ND, IPv6 RA, DAD
prompt: prompts/features/F-neighbors-ra.md   (template: prompts/FEATURE-TEMPLATE.md; checked against main + P08 in wave-A prep)   wbs: D2.3 (+ the ARP-flush tool of D2.6)
scope: glue on top of DF-2 + P08: projection of `ip-neighbor.*` / `ip6-nd.*` / `arp.*` (static neighbours, RA config + prefixes, proxy-ARP ranges/interfaces, proxy-ND opt-in only, neighbour limits + DAD for the globals owner only), a paged `ListNeighbors` RPC + change events, the ARP-flush action, API module, Neighbours screen + per-interface RA fields, docs. No new descriptor package, no second claim store.
merged deps you can rely on: P08, DF-2
  - P08: desired/ (Sink, `interface/<name>` aliases), subsystems.Register/Domains, Wiring.KeyedClaims("acl") (= the persisted df2 claim store), reconnect hook `Wiring.Connected`, Env.GlobalsOwner, projection.go, InterfaceState state-RPC pattern, interface drawer = SchemaForm over `interfaces.<if>` grouped by `withUi` group, test/topology/interfaces
  - DF-2: descriptors/ip_neighbor (`ipneighbor.Register`, `RegisterGlobals`), descriptors/ip6_nd (`ip6nd.Register`, `RegisterGlobals` = DAD, `RegisterProxyNd` = opt-in), descriptors/arp (`arp.Register(r, c, owner, tables *df2.IDRange, …)`), helper descriptors/df2 (`WithClaims`, `IDRange`); docs/agent/descriptors/{ip_neighbor,ip6_nd,arp}.md
  - also on main: TD-3 (V19 sanitizer apps/agent/internal/vpp/ifsanitize + cmd/vrx-vpp-preflight), TD-2 (API auth/users follow-ups)
read first: prompts/features/F-neighbors-ra.md · docs/status/wave-A-hotspots.md (§0 rules, §2 numbers, A1 A2 A4 A6 C1–C7 P1 P2 P4 P5 P6 W1–W3 W5) · docs/agent/descriptors/{ip_neighbor,ip6_nd,arp}.md · docs/status/tasks/DF-2.md · docs/status/vertical-slice.md · docs/vpp-code-track.md V12 · docs/decisions/LOG.md D-063, D-064, D-065, D-069, D-071, D-076, D-080, D-082, D-094, D-095, D-101, D-104
slot: 9 → VRX_SLOT=9 VRX_TEST_PREFIX=w9 VRX_HTTP_PORT=3000+100·9 VRX_WEB_PORT=5000+100·9 VRX_METRICS_PORT=9100+10·9+1 VRX_AGENT_SOCKET=/run/vrx-test/w9/agent.sock VRX_PG_DATABASE=vrx_w9 VRX_VALKEY_DB=9 VRX_VPP_TABLE_BASE=9000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env 9)"`
  - rig prefix w9 → 10.9.{1,2}.0/24; neighbour/RA/proxy objects only on w9-prefixed interfaces; proxy-ARP ranges only in tables 9000–9999 and addresses in 10.9.0.0/16
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none
obligations:
  - D-104: use, do not rebuild — DF-2's descriptors, keys and Normalize helpers (`NormalizeRaConfig`, `NormalizeRaPrefix`) as they are; `descriptors/df2` is read-only
  - persisted claims only: every DF-2 family gets `df2.WithClaims(<Wiring.KeyedClaims("acl")>)`; no in-memory default in the product agent (D-080)
  - ID range for `arp.Register`: `subsystems.SlotIDRange()` if the manager's anchor commit provides it; otherwise a private, slug-named helper (`neighborsRaIDRange`, so it never collides with F-rpf-adl-pbr's) in your owned `subsystems/neighbors_ra.go` reading `VRX_VPP_TABLE_BASE` (set → [base, base+999]; unset → nil = all ids, the product agent). Never edit agent.go (A5). F-rpf-adl-pbr needs the same range for ABF
  - D-071/D-082: `ip-neighbor.config` (limits) and `ip6-nd.dad` go through `RegisterGlobals` only when `Env.GlobalsOwner`; slot agents emit a warning and skip. Host tests of globals run only behind your own opt-in env var, hold `flock -x /run/lock/vrx-globals.lock`, restore the PREVIOUS value (never VPP defaults)
  - D-064/V12: proxy-ND only via `RegisterProxyNd` behind `VRX_DF2_PROXY_ND=1`, never in the gate; the product exposes it as "experimental, off by default"
  - shared-VPP safety: `ip_neighbor_flush`, `ip_neighbor_dump` and `want_ip_neighbor_events_v2` default to sw_if_index ~0 = every slot's interfaces. Never send ~0 on the shared host; flush only interfaces this agent may name (own tag or untagged; `iface.ResolveName` refuses foreign tags); filter events the same way; re-subscribe events from `Wiring.Connected` (one line under your A1 anchor, like DF-8's `Reconnected`); coalesce events at 1 Hz (prompt default)
  - D-063/D-076: every DF-2 type here has a dump; if you find one that does not, it is write-only (idempotent Create or boot-keyed record from `Wiring`) — never echo desired
  - D-095c: delete order in tests, rollback and restart simulation — neighbours, RA prefixes/config and proxy entries before the interfaces they sit on
  - UI: the P08 drawer already renders new `interfaces.<if>` leaves through SchemaForm by `withUi` group — no drawer rewrite; absent/default must mean "off" (drawer saves write defaults back; P08's `dropPhantomOptionals` drops only unchanged optional objects)
files you own exclusively:
  - apps/agent/internal/desired/neighbors_ra*.go (builder + assembler for the per-interface, vrfs and routing leaves; `desired/interfaces.go` stays P08's)
  - apps/agent/internal/subsystems/neighbors_ra*.go (registration helper, ID range, event subscription), apps/agent/internal/agent/rpc_neighbors_ra*.go (`ListNeighbors`; `server` embeds UnimplementedDataplaneServer)
  - apps/agent/internal/actions/neighbors-ra/** (flush + neighbour lister: pure functions + fake-client tests)
  - apps/agent/internal/descriptors/core/coretest/neighbors_ra*.go (A6: new file; existing coretest files are read-only)
  - apps/api/src/features/neighbors-ra/** (index.ts exports {controllers, providers}; `NeighborsRaController`; `GET /state/neighbors`; static route `POST /actions/arp-flush`, audited; real fake behaviour in fake.ts), apps/api/test/e2e/neighbors-ra*
  - apps/web/src/domains/routing/neighbors-ra/**, apps/web/src/locales/{en,fa}/neighbors-ra.json
  - packages/schema/src/domains/ext/neighbors-ra*.ts, packages/schema/src/semantic/neighbors-ra*.ts (rule ids `interfaces.neighbors-ra-…`, `vrfs.neighbors-ra-…`, `routing.neighbors-ra-…`), packages/schema/examples/neighbors-ra-*.json, packages/proto/test/fixtures/neighbors-ra-*.json
  - docs/user/routing/neighbors-ra.md, test/topology/neighbors-ra/**, docs/status/tasks/F-neighbors-ra*
  - gap-only (edit only for a proven defect or a missing helper, name the test): apps/agent/internal/descriptors/{ip_neighbor,ip6_nd,arp}/**, docs/agent/descriptors/{ip_neighbor,ip6_nd,arp}.md
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-A: F-neighbors-ra`. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-neighbors-ra.md
  - A1 apps/agent/internal/subsystems/subsystems.go: the DF-2 registrations, descriptor names in `Domains` where each config leaf lives (RA/proxy per interface → Interfaces, proxy-ARP ranges → VRFs, static neighbours/limits/DAD → Routing; one domain per descriptor), one line in `Connected`
  - A2 apps/agent/internal/agent/projection.go: one call in project(), one in assemble() (after `desired.Assemble`)
  - A4 apps/agent/internal/agent/server.go: the `ArpFlushAction` case in `Action` (F-vrf-static-ecmp turns it into a type switch; if you merge first, do it: default Unimplemented)
  - C1 packages/schema/src/domains/{interfaces,vrfs,routing}.ts: one key line each (`ipv6Ra`, `proxyArp`, `proxyNd`; `proxyArpRanges`; `neighbors`), sub-schemas in your ext file · C2 packages/schema/src/semantic/index.ts: one spread line · C3 packages/schema/src/index.ts if you export anything · C4 new fixture files only
  - C5 packages/proto/vrx/v1/dataplane.proto: `ListNeighbors` under the service anchor; new messages in a `// ----- F-neighbors-ra -----` section at the end
  - allocated numbers (wave-A-hotspots §2, a merge blocker if reused): **Interface 15 `ipv6_ra`, 16 `proxy_arp`, 17 `proxy_nd`** · Subinterface 13–15 only if you mirror the leaves on sub-interfaces · **Vrf 4 `proxy_arp_ranges`** · **RoutingConfig 10 `neighbors`** · **ActionRequest.action oneof 4 `arp_flush`** · **EventKind 10 `EVENT_KIND_NEIGHBOR_CHANGED`**; nothing else
  - C6 docs/contracts/proto.md: `### F-neighbors-ra: ListNeighbors` (+ arp_flush, the event kind) · C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`)
  - P1 apps/api/src/app.module.ts · P2 apps/api/src/state/state.controller.ts: move the `GET /state/neighbors` handler (today a 501 stub) into your controller and delete the old block in one hunk · P4 apps/api/src/agent/agent.client.ts (`listNeighbors`; the Action stream method is F-vrf-static-ecmp's — reuse it if merged, else add yours under your anchor) · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub under the anchor on the contract commit) · P6 apps/api/src/infra/bus.ts `TOPICS` + apps/api/src/telemetry/relay.service.ts: topic `neighbor.events` + its EventKind case
  - A5 event sink: the agent's event bus (`agent/events.go`) is unexported and `subsystems.Env` has no publish hook, so neighbour events cannot reach StreamEvents from your subsystems code without an agent-core edit. Use the `Env` event sink the manager seeds in the anchor commit (shared with F-object-model, P11, P12, F-wireguard); if it is absent at spawn, write the question, keep the publisher behind a small interface in your own files, and never edit agent.go (A5)
  - W1 apps/web/src/router.tsx · W2 apps/web/src/nav/nav.ts + nav.test.ts (a "Neighbours" NavItem, labelKey in your namespace) · W3 apps/web/src/i18n.ts · W5 apps/web/src/domains/interfaces/{InterfacesPage,InterfaceDrawer}.tsx: one line in the manager's registry, only if the RA fields need more than their SchemaForm group
contract: commit `contract(schema): neighbours, RA, proxy-ARP/ND` and `contract(proto): ListNeighbors, arp_flush, neighbour event` as separate commits on YOUR branch first (the ci.sh contract guard checks the subject), plus docs/status/tasks/F-neighbors-ra-contract.md; tell the manager in the questions file and keep building. No own branches (P08 pattern)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/vpp), /root/vpp
  - apps/agent/binapi + tools/binapi-gen.sh (P04/manager-owned), tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/** (a need → questions file)
  - agent core (A5): apps/agent/internal/agent/{agent,service,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/desired/interfaces.go (A3); apps/agent/internal/vpp/ifsanitize/** (TD-3); apps/agent/internal/descriptors/df2/** (read-only)
  - apps/api/src/actions/** (F-vrf-static-ecmp), apps/api/src/state/** beyond the P2 hunk, apps/api/src/{auth,users}/** (TD-4)
  - every other descriptor package (core/svs = F-vrf-static-ecmp, urpf/adl/abf/classify = F-rpf-adl-pbr, acl, nat*, bond/l2/l3xc/mactime/lldp/span/gso/nsim, af_packet = TD-5); apps/web/src/locales/*/{nav,common,interfaces}.json (W4)
host rules:
  - core `ip-neighbor`, `ip6-nd`, `arp` and `ip6_dad` APIs are present (DF-2 verified); `ip6_dad_autoremove` is NOT loaded → no auto-remove (out of scope; startup.conf = handover). No data NICs needed
  - V19 SAFETY (D-095, crash on first packet): before ANY packet through the rig (the live-table test pings to learn a dynamic entry), `go -C apps/agent run ./cmd/vrx-vpp-preflight` must exit 0 (TD-3). Stop if it does not
  - run `systemctl show vpp -p NRestarts` before and after every host run; stop and write the questions file if it rises (D-064)
  - D-101: bring the veth down before any af_packet delete (TD-5 may not be merged)
  - never flush or subscribe with sw_if_index ~0; with VRX_INTEGRATION=1, one Go package at a time; hold `flock -s` on the lab lock only during a run (D-094)
coordination: F-vrf-static-ecmp and F-nat44-ed-sessions share the A4 Action switch (oneof 4 is yours, 5 is NAT's); F-vrf-static-ecmp also appends to `Vrf` (3) and owns `apps/api/src/actions/**` · F-rpf-adl-pbr appends to `Interface` (18–19) and `RoutingConfig` (11) and needs the same slot ID range · F-bonding / F-bridge-l2 / F-rpf-adl-pbr also add interface-drawer fields
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL: live neighbour table, flush confirm, RA fields in the interface drawer) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so.
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-neighbors-ra.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-neighbors-ra-wip.md current
CI: `TMPDIR=/tmp/g-w9 tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-neighbors-ra.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite), by PID · lab lock released · vrx_w9 dropped · no w9 neighbour/RA/proxy objects left in VPP (Retrieve + `vppctl show ip neighbors` / `show ip6 interface <if>` pasted) · every global you changed restored to its previous value (pasted) · rig down · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-neighbors-ra-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
