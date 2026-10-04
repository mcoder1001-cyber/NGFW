# Recovery note — 2026-10-04

The historical report below preserves previous commands and outcomes. Its historical PASS/BLOCKED claims do not certify current main or current host readiness. This recovery adapts names to NGFW; current results and deferred acceptance are recorded in eight-review-recovery-20261004.md.

# F-ospf-host — host (lab VPP) evidence owed by F-ospf (RV-A R7/R4/R1 "owed on host" lists)

branch `task/F-ospf-host` · slot 11 (`w11`, table base 11000, API :4100) · daemon owner: frr (slot instances only) ·
started 2026-09-28 18:52, quota outage 20:50 (D-208), CONTINUE 2026-09-29 07:55

**Status: PARTIAL — blocked on D-211 (INC-vpp-hang-20260928, VPP hung since 2026-09-28 19:13:04; parked on the owner's
VPP restart)** — diagnosis in `docs/status/tasks/F-ospf-host-questions.md` Q1. This row changed **no VPP-global setting**
at any time (no §3 window held; the only VPP writes were slot-11 af_packet interfaces, addresses and LCP pairs, all deleted
through the agent at 19:12:23 before the hang — `topology-netns.txt` lines 80-97). My one stuck `vppctl show interface`
child (07:51, pid 2776545) was killed by PID at 07:58. The FRR-side
evidence and the tier-2 API 400 are done on the real host; every step that needs the VPP binary API (rig topology, FIB,
agent-restart simulation, rollback on the rig, tier-3 400) is written, vetted and committed, but could not run: the VPP
main thread has been spinning at 100 % since ~19:12:27 on 2026-09-28, `vppctl`, the stats segment and `api.sock` all hang,
and a worker may not restart VPP (D-012). Re-run after the manager's restart: `test/topology/ospf/run.sh` (+ `NGFW_OSPF_FIB=root`)
and `test/topology/ospf/api400.sh` — everything else in this file stays valid.

## Owed checklist (R7 line 30 · R4 B1 · R1 B1) → result

| # | owed item | result | evidence |
|---|-----------|--------|----------|
| 1 | frrtest (mgmtd, zebra, staticd, ospfd): golden accepted by FRR 10.7.1 | DONE — `TestOSPFLive` PASS 40.2 s | `F-ospf-host-evidence/frrtest-ospf.txt` |
| 2 | frr-reload DryRun empty after Apply; vtysh running-config matches the render | DONE — "no diff (canonical form)"; 16 ospf/interface lines matched (FRR reorders `interface` lines alphabetically, content identical) | same |
| 3 | state readers vs real `show ip ospf vrf all neighbor\|interface json` | DONE — parsed on live output; neighbours poller snapshot `Full/-`; ospf-neighbors event on withdrawal 2.6 s | same (lines 209-283) |
| 4 | two netns ospfd peers on the af_packet rig, 50 prefixes each in `show ip fib table <id>` via linux-nl, withdrawn ≤ 10 s, neighbours Full, 224.0.0.5/6 reaches the LCP tap | **BLOCKED (VPP wedged)** — test + driver committed; first run (netns mode, 19:09) reached the commit (11 objects created, pairs + FRR config applied) but no adjacency formed within 90 s; the diag (mfib 224.0.0.0/24, tcpdump on the tap, both sides' `show ip ospf interface/neighbor`) was added after that run and never ran — VPP wedged 4 s after its cleanup | `topology-netns.txt`, `topology-netns-diag.txt` |
| 5 | agent-restart ≤ 30 s (log), pairs deleted behind the agent's back | **BLOCKED** — step 3 of the test (`lcp_itf_pair_add_del_v3` del with the agent stopped, recovery timed) | `ospf_topology_integration_test.go:744-773` |
| 6 | rollback removes `router ospf` + routes (Retrieve) | DONE on FRR (frrtest: running-config back to bare `frr version` after removal, 0 routes) · **BLOCKED** on the rig (step 4 of the test) | `frrtest-ospf.txt` lines 275-305 |
| 7 | undefined area → 400 with pointer through the API | DONE — `POST /config/validate` and `POST /config/commit` → 400 problem+json, pointer `/routing/ospf/interfaces/host-w11l0/area`; running config unchanged | `api-400.txt` |
| 8 | area 0 stub → 400 with pointer through the API | **BLOCKED** — the rule lives in the agent's ospf renderer (tier 3, agent DryRun), and DryRun refuses while the VPP binary API is down: API answers 503 `agent-unavailable` ("agent: VPP binary API is not connected"); with VPP up the driver's step 2 expects 400 at `/routing/ospf/areas/0/type` | `api-400.txt` |
| 9 | ci.sh green | see "CI gate" below | |
| 10 | NRestarts before/after every host step | 2 → 2 on every step (below) | `nrestarts-*.txt` |
| 11 | cleanup | nothing of w11 left: no `ns-w11-*` netns, no w11 FRR pids, slot DB dropped, `/run/ngfw-test/w11/ospf-api` removed (VPP-side objects were deleted through the agent at 19:12:23 before the wedge) | `topology-netns.txt` lines 80-122, `api-400.txt` cleanup |

Not applicable / handover-gated: none of the owed items needs a VPP global (no §3 window was held); the root-netns zebra
mode (`NGFW_OSPF_FIB=root`, D-119 M3) holds the exclusive globals lock inside the shared lab lock and was not reached.

## What was built (test/evidence code only; no product code touched)

- `apps/agent/internal/renderers/frr/ospf/integration_test.go` — `TestOSPFLive` (R1 B1 asked for exactly this file):
  frrtest harness in `ns-w11-frr` (pathspace w11) + one peer ospfd in `ns-w11-p1` over a veth; Apply → `frr-reload --test`
  empty → running-config vs render → adjacency Full → 50 redistributed statics on the NGFW side → readers on live JSON →
  poller/event → peer withdraws → removal leaves a bare config. FRR only in slot namespaces; both instances stopped by PID.
- `apps/agent/internal/agent/ospf_topology_integration_test.go` — `TestOSPFTopologyOnHost` (skips unless
  `NGFW_INTEGRATION=1 NGFW_OSPF_TOPOLOGY=1`): in-process agent (owner w11, FRR pathspace w11, table base 11000), rig
  `host-w11l0/w0` with LCP pairs `w11-l0/w0`, NGFW FRR + two ospfd peers (50 blackhole statics each, area 0 p2p, hello 1 s),
  commit → Full → 100 routes → Retrieve == desired → idempotent apply → withdraw ≤ 10 s → announce → agent restart with both
  pairs deleted behind its back (recovery ≤ 30 s, no API call) → rollback → cleanup (peers down before the agent deletes
  its af_packet interfaces, D-101/V24). Two modes: netns (default, FRR RIB checked, nothing in the root netns) and
  `NGFW_OSPF_FIB=root` (root-netns zebra one slot at a time, D-119 M3: VPP FIB checked via `ListRoutes(lcp-rt-dynamic)` +
  `show ip fib`, fails closed on any existing LCP pair / set default netns / FRR-protocol root route / active system frr,
  exclusive globals lock inside the shared lab lock, every route withdrawn before the last pair is deleted). Vetted:
  `go vet` 0, `golangci-lint` 0 issues.
- `test/topology/ospf/run.sh` — driver: env check, `NGFW_INTEGRATION=1 NGFW_OSPF_TOPOLOGY=1`, `tools/lab lock shared go test
  -run TestOSPFTopologyOnHost`, NRestarts before/after. shellcheck clean.
- `test/topology/ospf/api400.sh` — driver for items 7-8: slot DB (`pg-test.sh`), slot agent (owner w11, `NGFW_FRR_PATHSPACE=w11`),
  slot API :4100 with a slot-local test licence (feature `ospf`; without it validate answers 403 `license-required` at
  `/routing/ospf`), PATCH candidate → validate/commit → problem+json, discard, kill only spawned PIDs (lock fds closed with
  `8>&- 9>&-`), DB dropped. shellcheck clean.

## How verified (real output, trimmed)

### 1-3, 6(FRR) — frrtest live test (2026-09-28 19:03-19:04)
```
$ eval "$(tools/lab env 11)"; cd apps/agent && NGFW_INTEGRATION=1 tools/lab lock shared go test -count=1 -v -run TestOSPFLive ./internal/renderers/frr/ospf/
=== RUN   TestOSPFLive
    integration_test.go:136: ngfw ns-w11-frr (pathspace w11), peer ns-w11-p1 (pathspace w11p1), veth w11v0 10.11.9.1 ↔ w11v1 10.11.9.2, dummy w11d0 10.11.8.0/24
    integration_test.go:173: ngfw rendered /run/ngfw-test/w11/frr/etc/w11/frr.conf:
        frr version 10.7.1
        …
        interface w11v0
         ip ospf network point-to-point
         ip ospf cost 10
         ip ospf hello-interval 1
         ip ospf dead-interval 4
         ip ospf priority 0
         ip ospf area 0.0.0.0
        exit
        !
        router ospf
         ospf router-id 10.11.9.1
         redistribute connected metric 20
         area 0.0.0.7 nssa
         area 51 stub no-summary
         default-information originate always
        exit
    integration_test.go:180: frr-reload --test of the applied config: no diff (canonical form, FRR 10.7.1)
    integration_test.go:206: 16 rendered ospf/interface lines checked against the running config
    integration_test.go:209: adjacency Full on the NGFW side after 100ms ({VRF:default RouterID:10.11.9.2 Address:10.11.9.2 Interface:w11v0 State:Full/- Priority:1})
    integration_test.go:227: 50 OSPF routes on the NGFW side after 5.1s (50 routes of 10.11/16)
    integration_test.go:232: connected (metric 20) and default-information originate on the peer after 2.1s (0.0.0.0/0 10.11.8.0/24)
    integration_test.go:244: show ip ospf vrf all interface json (keys [default]):
    integration_test.go:268: ospf-neighbors poller snapshot: map[default|10.11.9.2|w11v0:Full/-]
    integration_test.go:275: 0 redistributed prefixes after the peer withdrew after 100ms (0 routes of 10.11/16)
    integration_test.go:283: ospf-neighbors event for the peer after 2.6s (ospf-neighbors default|10.11.9.2|w11v0: Full/- -> -)
    integration_test.go:305: after removal, show running-config:
        …
        service integrated-vtysh-config
        !
        end
--- PASS: TestOSPFLive (40.22s)
ok  	ngfw/agent/internal/renderers/frr/ospf	40.276s
```
NRestarts: before `Mon Sep 28 07:03:19 PM +0330 2026 NRestarts=2` · after `Mon Sep 28 07:04:16 PM +0330 2026 NRestarts=2`.
FRR 10.7.1 line-order note: FRR canonicalises `interface` blocks alphabetically (`area, cost, dead-interval, hello-interval,
network, priority`); the renderer's order is accepted verbatim by `frr-reload --test` (no diff), so no renderer change.

### 4 — topology test, netns mode (2026-09-28 19:09:49-19:12:25) — reached the commit, no adjacency, then VPP wedged
```
$ eval "$(tools/lab env 11)"; test/topology/ospf/run.sh
run.sh: 2026-09-28T19:09:49 VPP NRestarts=2 before, mode netns
    ospf_topology_integration_test.go:561: ngfw-vpp-preflight: <nil>
        V19 pre-flight ok: no classify binding or classify DPO points at a missing table (0 warning(s))
    ospf_topology_integration_test.go:566: rig up w11 (slot 11, path af_packet)
          create vpp host-w11l0 (af_packet on w11l0)
          reset  ip/ip6 classify host-w11l0 (table-index ~0)
          …
2026/09/28 19:10:47 INFO created owner=w11 component=scheduler key=af-packet.host-interface/host-w11l0
2026/09/28 19:10:47 INFO created owner=w11 component=scheduler key=lcp.itf-pair/host-w11l0
2026/09/28 19:10:47 INFO created owner=w11 component=scheduler key=lcp.itf-pair/host-w11w0
2026/09/28 19:10:49 INFO FRR configuration applied owner=w11 component=subsystems component=frr conf=/run/ngfw-test/w11/frr/etc/w11/frr.conf bgp=false
2026/09/28 19:10:49 INFO reconcile done owner=w11 txn_id=w11-p12-commit mode=apply domains="[interfaces routing]" status=APPLY_STATUS_APPLIED summary=created:11
    ospf_topology_integration_test.go:641: adjacencies not Full within 1m30s: []
2026/09/28 19:12:23 INFO deleted owner=w11 component=scheduler key=lcp.itf-pair/host-w11w0
2026/09/28 19:12:23 INFO deleted owner=w11 component=scheduler key=lcp.itf-pair/host-w11l0
2026/09/28 19:12:23 INFO af_packet quiesce: netdev already down (D-101, VPP V24) netdev=w11w0 …
2026/09/28 19:12:23 INFO deleted owner=w11 component=scheduler key=af-packet.host-interface/host-w11w0
2026/09/28 19:12:23 INFO deleted owner=w11 component=scheduler key=af-packet.host-interface/host-w11l0
    ospf_topology_integration_test.go:616: cleanup apply: <nil> { "deleted":  9 }
    ospf_topology_integration_test.go:569: rig down: <nil>  (delete netns ns-w11-lan / ns-w11-wan)
    ospf_topology_integration_test.go:539: systemctl show vpp -p NRestarts (after) = 2
--- FAIL: TestOSPFTopologyOnHost (132.75s)
```
Second run 19:14:00 (with the diag added): `waiting for VPP at /run/vpp/api.sock: context deadline exceeded` — VPP already
wedged (`topology-netns-diag.txt`). NRestarts 2 → 2 on both runs. Why no adjacency is undiagnosed (the diag never ran);
candidates: 224.0.0.5 not punted to the tap in that netns (the open R4 question), or the peers' veth ends were still down
when the agent took the rig (`e.peers(true)` runs after the commit). Both are answered by the committed diag on the next run.

### 7-8 — API 400 through the slot stack (2026-09-29 08:05)
```
$ eval "$(tools/lab env 11)"; test/topology/ospf/api400.sh
08:05:06 VPP NRestarts=2 before
08:05:15 agent pid 2917444 (NGFW_FRR_PATHSPACE=w11 NGFW_VPP_TABLE_BASE=11000), API pid 2917451 health {"status":"ok"}
08:05:15 agent log: …"msg":"VPP connect round failed; backing off","owner":"w11","component":"vpp","err":"dial unix /run/vpp/api.sock: connect: resource temporarily unavailable"
08:05:15 === 1. undefined area: interface host-w11l0 in area 51, areas = {0} → POST /config/validate ===
08:05:15 PATCH /config/routing (ospf: routerId 10.11.1.1, areas {0}, interfaces {host-w11l0: area 51}) → HTTP 200 application/json; charset=utf-8
08:05:16 POST /config/validate → HTTP 400 application/problem+json; charset=utf-8
08:05:16 body: {"type":"https://ngfw.dev/problems/validation","title":"Validation failed","status":400,"errors":[{"pointer":"/routing/ospf/interfaces/host-w11l0/area","message":"OSPF area 51 is not defined under /routing/ospf/areas"}]}
08:05:16 POST /config/commit → HTTP 400 application/problem+json; charset=utf-8
08:05:16 body: {"type":"https://ngfw.dev/problems/validation","title":"Validation failed","status":400,"errors":[{"pointer":"/routing/ospf/interfaces/host-w11l0/area","message":"OSPF area 51 is not defined under /routing/ospf/areas"}]}
08:05:16 running config after the refused commit: routing.ospf = null
08:05:16 === 2. backbone stub: areas = {0: stub}, interface host-w11l0 in area 0 → POST /config/validate ===
08:05:16 PATCH /config/routing (ospf.areas.0.type = stub, interfaces {host-w11l0: area 0}) → HTTP 200 application/json; charset=utf-8
08:05:17 POST /config/validate → HTTP 503 application/problem+json; charset=utf-8
08:05:17 body: {"type":"https://ngfw.dev/problems/agent-unavailable","title":"Agent unavailable","status":503,"detail":"agent: VPP binary API is not connected","errors":[]}
08:05:17 === cleanup ===
08:05:17 candidate discarded
08:05:23 stopped pids: 2917444 2917451
08:05:24 database ngfw_w11 dropped
08:05:24 VPP NRestarts=2 after
```
(First attempt without the licence: step 2 answered 403 `license-required`, pointer `/routing/ospf`, "OSPF is not covered by
the licence (feature "ospf")" — the licence gate runs before tier 2 for that path; recorded here as a fact about the API.)

### Leftover check on CONTINUE (2026-09-29 07:55) and the VPP wedge
```
# ip netns list | grep w11            → (none)
# ps … | grep -E 'ospfd|zebra|frr'    → (none of w11)
# ls /run/ngfw-test/w11/               → frr-p1.lock frr-p2.lock frr.lock (lock files only)
# timeout 15 vppctl show interface    → rc=143 (hang) — see questions.md Q1 for the full diagnosis
```

## CI gate
D-210 (`plan/NO-TESTS` on main, merged into this branch): compile-only gate. Tail pasted below after the run.

`TMPDIR=/tmp/g-w11 tools/ci.sh --base main` on 124d7f46 (main merged at c50e4636^), 2026-09-29 08:05-08:23, load 28-38:
```
  slot resource scheme (1..32, no collisions)        0m04s
  lint · typecheck · unit tests · build (turbo)   2m40s
  apps/agent: make lint test build                   3m30s
  apps/cli: make lint test build                     0m20s
  test/ Go modules, unit mode (test/integration/reachability test/integration/smoke test/topology/acl test/topology/bonding test/topology/bridge-l2 test/topology/host-acl-nftables test/topology/interfaces test/topology/ipfix-sflow test/topology/kea-dhcp-relay test/topology/loopback-bvi-gso-lldp-span test/topology/nat44-ed-sessions test/topology/nat44-ei-64-66-nptv6 test/topology/neighbors-ra test/topology/object-model test/topology/qos-flat test/topology/system-identity test/topology/unbound-chrony-syslog test/topology/vlan-qinq test/topology/vrf-static-ecmp)   1m28s
  deploy/vpp: shellcheck + apply-startup fake-host harness   7m09s
  warnings:
    - control-plane lines exempted with 'ALLOW:' — reviewer, check each justification:
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:1:import { execFileSync } from 'node:child_process'; // ALLOW: test-only openssl cert
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:13:    execFileSync('openssl', ['version'], { stdio: 'ignore' }); // ALLOW: test-only openssl cert
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:20:const openssl = (args: string[]) => execFileSync('openssl', args, { stdio: 'ignore' }); // ALLOW: test cert
  mode quick · wall time 18m02s · logs /root/ngfw-wt/logs/ci/F-ospf-host-20260929-080547-2923556

CI GATE PASSED
```
(`ci-tail.txt` in the evidence directory.)

## Shared hunks
None in product code. Two test-only files outside the envelope's `files_owned` (questions.md Q2): `apps/agent/internal/renderers/frr/ospf/integration_test.go`
(new, R1 asked for it by name) and `apps/agent/internal/agent/ospf_topology_integration_test.go` (new; reuses P12's
`p12Env`, `applyFRR`, `hostConfig`, `dumpNames`, `nRestartsP12` helpers unchanged). No anchored lines were added to any shared file.

## Out of scope / not done
- Items 4, 5, 6 (rig), 8: blocked by the wedged VPP — drivers ready, re-run after the manager's restart (both modes of
  `run.sh`; `NGFW_OSPF_FIB=root` is the VPP-FIB proof and must run as the only FRR host row).
- The 224.0.0.5/6-to-tap question (R4) stays open until the topology diag runs.
- Screenshots: T4 after merge (envelope).
- No product-code change was needed so far; if the topology run shows that OSPF multicast is not punted to the LCP tap,
  that is a wiring fix outside this envelope (→ questions file, not here).

## Decisions
- D-host-1: the FRR-side owed items (1-3, 6) are proven with a frrtest live test in the ospf renderer package
  (`TestOSPFLive`), mirroring `bgp/integration_test.go`, instead of a driver script — the readers, poller and event path
  are agent-internal and only a Go test can exercise them on live vtysh JSON.
- D-host-2: the topology test's VPP-FIB proof is a separate `NGFW_OSPF_FIB=root` mode with fail-closed preconditions
  (P12 Q1 / D-119 M3), never the default: linux_nl hears only the lcp default netns, and a root-netns zebra beside another
  FRR row would sweep its routes.
- D-host-3: the API-400 evidence runs against a slot stack whose agent has no VPP (tier 1-2 need none); the tier-3 case is
  reported as blocked rather than faked with a unit test (D-210 forbids new unit tests anyway).

## Open questions
See `docs/status/tasks/F-ospf-host-questions.md`: Q1 VPP wedge (manager restart needed, then CONTINUE this row);
Q2 two test files outside `files_owned`; Q3 D-210 reading (host drivers are not unit tests).
