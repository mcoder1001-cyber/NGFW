# F-det44-cnat-fix — CNAT 0/12 on the det44 rig (D-218): root cause, fix, host proof (slot 16)

Branch `task/F-det44-cnat-fix`, worktree `/root/ngfw-wt/F-det44-cnat-fix`, slot 16 (`w16`), base main@55a18e0f.
Source: `F-det44-map-dslite-cnat-host-questions.md` "Q2 update" (run 2: `det44-in2out` x6 on `host-w16l0`, CNAT 0/12).

## Result

- **Suspect (a) is the root cause, and it is a VPP 26.06 bug.** `det44_interface_add_del()` calls
  `vnet_feature_enable_disable(..., 1, ...)` on its DELETE path too, so every det44 interface delete puts one MORE
  `det44-in2out`/`det44-out2in` on the ip4-unicast arc (add → 1, del → 2, add → 3, del → 4). The nodes outlive det44's
  own record and drop every packet no det44 map matches. Run 2's six nodes = rev 2 add + raw loss delete + resync add
  + second-restart delete/add (scheduler re-creates det44 dependents, questions Q3) + rev 4a delete.
- **Suspect (b) is not needed.** On interfaces without det44 history the VIP connects 12 of 12 with no cnat interface
  feature (VPP serves the VIP through the FIB `cnat-client` DPO). `descriptors/cnat` is unchanged.
- **Fix (config-only fallback, `apps/agent/internal/descriptors/det44/det44.go`):** the det44 interface descriptor keeps
  the arc exact through `binapi/feature` (`feature_is_enabled`, `feature_enable_disable`): Delete = det44 delete + remove
  every det44 node of a side det44 no longer holds; Create never enables twice (no add when det44 already holds the
  interface on that side) and clears leftovers before its single add; Retrieve reports leftover nodes as
  `<interface>#leftover/<side>` (never desired → the reconciler deletes it; its Delete is the repair alone; it depends
  on the base interface). A "both det44 nodes on" answer is not trusted (feature_is_enabled casts "no such feature" to
  true; the agent's in-memory test VPP answers the same for unmodelled nodes) — the descriptor never produces that state.
- **CNAT round on the host after the fix: 12/12 in rev 4a (cnat only) and 12/12 in rev 4 (map-e + cnat)**; every phase
  of `test/topology/det44` passes; rollback and cleanup leave nothing of slot 16; NRestarts=0 throughout.

## Before / after (host, slot 16, af_packet rig; evidence in `docs/status/tasks/F-det44-cnat-fix-evidence/`)

### VPP alone (raw binary API on fresh rig interfaces, no agent) — `split-before.txt` lines 37-59

```
ARC fresh rig interfaces (tools/lab rig up):
    host-w16l0(sw_if_index=5) det44 pool: none; feature_is_enabled det44-in2out=false det44-out2in=false; ip4-unicast arc: []
ARC step 1: det44_interface_add_del_feature is_add=true (lan inside, wan outside):
    host-w16l0(sw_if_index=5) det44 pool: in=true out=false; feature_is_enabled det44-in2out=true det44-out2in=false; ip4-unicast arc: [ip4-sv-reassembly-feature det44-in2out]
    host-w16w0(sw_if_index=6) det44 pool: in=false out=true; feature_is_enabled det44-in2out=false det44-out2in=true; ip4-unicast arc: [ip4-sv-reassembly-feature det44-out2in]
ARC step 2: det44_interface_add_del_feature is_add=false (lan inside, wan outside):
    host-w16l0(sw_if_index=5) det44 pool: none; feature_is_enabled det44-in2out=true det44-out2in=false; ip4-unicast arc: [det44-in2out det44-in2out]
    host-w16w0(sw_if_index=6) det44 pool: none; feature_is_enabled det44-in2out=false det44-out2in=true; ip4-unicast arc: [det44-out2in det44-out2in]
ARC step 3: … is_add=true:  arc: [ip4-sv-reassembly-feature det44-in2out det44-in2out det44-in2out]   (wan: det44-out2in x3)
ARC step 4: … is_add=false: arc: [det44-in2out det44-in2out det44-in2out det44-in2out]               (wan: det44-out2in x4)
config-only repair: feature_enable_disable ip4-unicast/det44-in2out enable=0 x4 on host-w16l0, ip4-unicast/det44-out2in enable=0 x4 on host-w16w0
ARC after the config-only repair:
    host-w16l0(sw_if_index=5) det44 pool: none; feature_is_enabled det44-in2out=false det44-out2in=false; ip4-unicast arc: []
    host-w16w0(sw_if_index=6) det44 pool: none; feature_is_enabled det44-in2out=false det44-out2in=false; ip4-unicast arc: []
```

### Suspect split through the agent — before the fix (`split-before.txt`) / after (`split-after.txt`)

Before (agent built from main@55a18e0f):
```
rev 2 (cnat only, no det44 history, no cnat interface feature): CNAT VIP 10.16.2.100:80 → backends (SYNs per backend): map[10.16.2.2:3 10.16.2.3:9]; client lines: 12 connected
ARC after rev 4 (det44 removed from the document, cnat kept):
    host-w16l0(sw_if_index=6) det44 pool: none; feature_is_enabled det44-in2out=true det44-out2in=false; ip4-unicast arc: [det44-in2out det44-in2out]
    host-w16w0(sw_if_index=5) det44 pool: none; feature_is_enabled det44-in2out=false det44-out2in=true; ip4-unicast arc: [det44-out2in det44-out2in]
det44 removed from the document but the ip4-unicast arcs still hold det44 nodes: {lanIn:2 lanOut:0 wanIn:0 wanOut:2}
        connected 0 of 12
rev 4 (det44 added and removed again, cnat kept): CNAT VIP 10.16.2.100:80 → backends (SYNs per backend): map[]; client lines: 0 connected
--- FAIL: TestDet44ArcSplitOnHost (50.94s)
    --- PASS: TestDet44ArcSplitOnHost/vpp-det44 (0.17s)
    --- PASS: TestDet44ArcSplitOnHost/cnat-fresh (2.46s)
    --- FAIL: TestDet44ArcSplitOnHost/det44-churn (40.34s)
```
After (this branch):
```
rev 2 (cnat only, no det44 history, no cnat interface feature): CNAT VIP 10.16.2.100:80 → backends (SYNs per backend): map[10.16.2.2:6 10.16.2.3:6]; client lines: 12 connected
ARC after rev 4 (det44 removed from the document, cnat kept):
    host-w16l0(sw_if_index=6) det44 pool: none; feature_is_enabled det44-in2out=false det44-out2in=false; ip4-unicast arc: []
    host-w16w0(sw_if_index=5) det44 pool: none; feature_is_enabled det44-in2out=false det44-out2in=false; ip4-unicast arc: []
rev 4 (det44 added and removed again, cnat kept): CNAT VIP 10.16.2.100:80 → backends (SYNs per backend): map[10.16.2.2:6 10.16.2.3:6]; client lines: 12 connected
after the cleanup through the agent: NAT objects of the slot 0 ; host-w16l0 present=false host-w16w0 present=false
--- PASS: TestDet44ArcSplitOnHost (9.08s)
    --- PASS: TestDet44ArcSplitOnHost/vpp-det44 (0.17s)
    --- PASS: TestDet44ArcSplitOnHost/cnat-fresh (1.41s)
    --- PASS: TestDet44ArcSplitOnHost/det44-churn (1.55s)
    --- PASS: TestDet44ArcSplitOnHost/cleanup-through-agent (0.36s)
ok  	ngfw/test/topology/det44	9.127s
```

### Full topology test after the fix — `topology-after.txt` (before = predecessor's `topology-run2.txt`: CNAT 0/12)

`eval "$(tools/lab env 16)"; VRX_FDET44_DET44_HOST=1 VRX_EVIDENCE_TXT=<abs>/topology-after.txt /root/NGFW/tools/heavy.sh test/topology/det44/run.sh -run TestDet44MapDsliteCnatOnHost`

```
GLOBALS WINDOW start 2026-10-01T21:33:28+03:30 (flock -x /run/lock/vrx-globals.lock inside flock -s /run/lock/vrx-lab.lock)
ARC after the loss behind the agent's back (raw det44 delete, VPP 26.06 adds a node):
    host-w16l0(sw_if_index=5) det44 pool: none; feature_is_enabled det44-in2out=true det44-out2in=false; ip4-unicast arc: [det44-in2out det44-in2out]
    host-w16w0(sw_if_index=6) det44 pool: none; feature_is_enabled det44-in2out=false det44-out2in=true; ip4-unicast arc: [det44-out2in det44-out2in]
ARC after the agent restart (resync: leftover nodes repaired, one det44 add each):
    host-w16l0(sw_if_index=5) det44 pool: in=true out=false; feature_is_enabled det44-in2out=true det44-out2in=false; ip4-unicast arc: [ip4-sv-reassembly-feature ip4-map det44-in2out]
    host-w16w0(sw_if_index=6) det44 pool: in=false out=true; feature_is_enabled det44-in2out=false det44-out2in=true; ip4-unicast arc: [ip4-sv-reassembly-feature ip4-map det44-out2in]
ARC after the second restart (no loss): (same: one det44 node each)
DET44 packet path OK (path: af_packet): 10.16.1.2:40001 → 10.16.3.0:3729 → 10.16.2.2:8000; sessions 2
ARC after rev 4a (det44 removed from the document):
    host-w16l0(sw_if_index=5) det44 pool: none; feature_is_enabled det44-in2out=false det44-out2in=false; ip4-unicast arc: []
    host-w16w0(sw_if_index=6) det44 pool: none; feature_is_enabled det44-in2out=false det44-out2in=false; ip4-unicast arc: []
        connected 12 of 12
rev 4a (cnat + dslite, no map): CNAT VIP 10.16.2.100:80 → backends (SYNs per backend): map[10.16.2.2:7 10.16.2.3:5]; client lines: 12 connected
MAP-E BR: 3 packet(s) encapsulated by VPP (fd00:10::1 → fd00:10:65::1 carrying 10.16.1.2:40002 → 10.16.65.1:1100), 2 answered by the CE towards the BR
        connected 12 of 12
rev 4 (map + cnat + dslite): CNAT VIP 10.16.2.100:80 → backends (SYNs per backend): map[10.16.2.2:8 10.16.2.3:4]; client lines: 12 connected
CNAT packet path OK (path: af_packet): 10.16.1.2 → 10.16.2.100:80 → 10.16.2.2:8080 (8) + 10.16.2.3:8080 (4)
slot objects left in det44 / map / cnat / dslite after rollback: 0
ARC after the rollback (nat {}):
    host-w16l0(sw_if_index=5) det44 pool: none; feature_is_enabled det44-in2out=false det44-out2in=false; ip4-unicast arc: []
    host-w16w0(sw_if_index=6) det44 pool: none; feature_is_enabled det44-in2out=false det44-out2in=false; ip4-unicast arc: []
after the interfaces were deleted through the agent: host-w16l0 present=false host-w16w0 present=false
GLOBALS WINDOW end 2026-10-01T21:33:53+03:30 (held 25s)
21:33:53 after the test: systemctl show vpp -p NRestarts → NRestarts=0
--- PASS: TestDet44MapDsliteCnatOnHost (24.95s)
    --- PASS: TestDet44MapDsliteCnatOnHost/config (2.86s)
    --- PASS: TestDet44MapDsliteCnatOnHost/restart (1.77s)
    --- PASS: TestDet44MapDsliteCnatOnHost/det44-packets (6.01s)
    --- PASS: TestDet44MapDsliteCnatOnHost/map-cnat-packets (8.91s)
    --- PASS: TestDet44MapDsliteCnatOnHost/rollback (0.26s)
    --- PASS: TestDet44MapDsliteCnatOnHost/cleanup-through-agent (0.27s)
PASS
ok  	ngfw/test/topology/det44	24.996s
```
Agent log of that run (the resync removes the loss's leftovers before re-adding; `/run/vrx-test/w16/det44/agent.log`):
```
{"time":"2026-10-01T21:33:35.989180973+03:30","level":"INFO","msg":"deleted","owner":"w16","component":"scheduler","key":"det44.interface/host-w16w0#leftover/outside"}
{"time":"2026-10-01T21:33:35.992611023+03:30","level":"INFO","msg":"deleted","owner":"w16","component":"scheduler","key":"det44.interface/host-w16l0#leftover/inside"}
{"time":"2026-10-01T21:33:36.500266073+03:30","level":"INFO","msg":"re-creating dependents","owner":"w16","component":"scheduler","key":"det44.enable/global","dependents":3}
```

### Leftovers after all runs (read-only, `leftovers-after.txt`)

No slot-16 interface, netns, veth, det44 interface/map, MAP domain, cnat translation, DS-Lite pool or CE route; AFTR
back to `::`; `rig: down`; `NRestarts=0`. Only VPP-internal cnat state of the slot's own addresses remains
(`cnat-client:[10.16.2.100]`, `cnat-client:[10.16.1.2]` + sessions): no per-owner API removes it, it ages out
(questions Q2).

## Fake-VPP unit tests (D-210a: own package only)

`cd apps/agent && /root/NGFW/tools/heavy.sh go test -count=1 -v ./internal/descriptors/det44/` — the fake models VPP
26.06's arc (`add` and `del` both add a node), a fixed VPP, and a VPP whose feature_is_enabled always says "on":
```
=== RUN   TestDet44
    det44_test.go:188: det44.enable is write-only: Retrieve → ErrRetrieveUnsupported
--- PASS: TestDet44 (0.00s)
=== RUN   TestDet44InterfaceArcRepair
--- PASS: TestDet44InterfaceArcRepair (0.00s)
=== RUN   TestDet44InterfaceArcRepairFixedVPP
--- PASS: TestDet44InterfaceArcRepairFixedVPP (0.00s)
=== RUN   TestDet44InterfaceUntrustedArc
--- PASS: TestDet44InterfaceUntrustedArc (0.00s)
PASS
ok  	ngfw/agent/internal/descriptors/det44	0.038s
```
`TestDet44InterfaceArcRepair`: create → one node per interface, idempotent second Create sends no add, delete → arc
empty (4 disables), raw loss → `loop900#leftover/inside` + `loop901#leftover/outside` reported (leftover depends on
`interface/loop900`), resync → exactly one node each, unwanted leftovers deleted, both-sides leftovers not trusted,
rollback re-create of a leftover sends nothing, foreign (w3-tagged) arcs never touched, untagged NIC only with this
owner's claim, a gone interface is not an error. Mutation check: with the repair disabled the test fails
(`after delete: arc … = [2 0 0 2], want [0 0 0 0]`). `go vet ./internal/descriptors/det44/` and
`go vet` of `test/topology/det44` clean; `go build ./...` of apps/agent OK.

## Harness changes (`test/topology/det44/**`)

- `arc_test.go` (new): `TestDet44ArcSplitOnHost` (the suspect split above) and the arc helpers (`arcState`, `mustArc`:
  det44_interface_dump + feature_is_enabled + `show interface features` ip4-unicast after every phase of the main test).
- `det44_test.go`: arc assertions after rev 2, the loss, both restarts, rev 3, rev 4a, rev 4 and the rollback;
  per-run CNAT source ports (`cnatPorts`, stale cnat sessions, questions Q2); CE replies through the ip6tnl only from the
  CE's shared address (source rule, netns table 16065; the old route stole the backends' SYN-ACKs); cleanup names its
  domains in `ApplyRequest.subsystems` (an empty `interfaces` map alone is skipped, D-041); diag uses 26.06's
  `show cnat session verbose <n> ip <ip>` and greps det44 drops.
- `helpers_test.go`: `mustApplyDomains`; captures wait for tcpdump's `listening on` (a det44 handshake went by
  uncaptured at load 16 with the fixed 700 ms).

## Out of scope / not done

- The VPP fix itself (no C code): V-entry text in questions Q1. The scheduler's re-creation of det44 dependents on every
  agent restart (det44 sessions dropped): questions Q3, `internal/scheduler` is not mine.
- Duplicate det44 nodes on a side det44 still holds (only producible by an outside actor's raw add/del) cannot be seen
  through `feature_is_enabled` (boolean) and are not repaired; neither are leftovers of both sides on one interface.
- `TestDet44OnHost` (descriptor integration test) stays opt-in (D-064) and was not run; the topology test covers the
  descriptor on the host VPP. No full suite, no lint, no other packages' tests (D-210).

## Decisions

- D-fix-1: fallback through the generic feature API (`feature_is_enabled` / `feature_enable_disable`, both in
  `binapi/feature`) instead of disabling the det44 plugin or a CLI parse; the node names are det44.c's feature names.
- D-fix-2: leftovers are reported under a distinct key `<interface>#leftover/<side>` (the D-066 `#n` pattern) so the
  reconciler deletes them before it creates the desired object (deletes run first); the leftover depends on its base
  interface (`natcommon.BaseName`).
- D-fix-3: an interface on which both det44 nodes read as enabled is never reported or repaired (VPP casts
  feature_is_enabled errors to true; the shared coretest VPP answers true for unmodelled nodes).
- D-fix-4: CNAT rounds use per-half-minute source-port blocks (20000-31999), never fixed ports.

## Shared hunks

None — only `apps/agent/internal/descriptors/det44/**`, `test/topology/det44/**`, `docs/status/tasks/F-det44-cnat-fix*`.

## CI gate

2026-10-01 21:39, tree of bac1d8b5, `TMPDIR=/tmp/g-w16 /root/NGFW/tools/ci-slot.sh --base main` (D-224 wrapper;
plan/NO-TESTS → compile-only gate, D-210):

```
ci-slot: /run/lock/vrx-ci-1.lock + /run/lock/vrx-heavy-2.lock after 0s (canary 0.28s, MemAvailable 16 GB) — tools/ci.sh --base main (log /root/ngfw-wt/logs/ci/gate-F-det44-cnat-fix-1001-213900.log)
no contract files changed in the 9 commit(s) of HEAD since main (55a18e0f)
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m07s
  generate + generated-output gate                   2m59s
  forbidden patterns (+ gitleaks)                    0m09s
  packet-trace ban on the shared VPP (D-128)         0m01s
  ip classify reset after every shell interface create (D-185)   0m01s
  slot resource scheme (1..32, no collisions)        0m03s
  typecheck · build (turbo — tests OFF)           3m16s
  apps/agent: go build + go vet (tests OFF)          0m43s
  apps/cli: make build (tests OFF)                   0m06s
  test/ Go modules, unit mode (… test/topology/det44 …)   0m19s
  warnings:
    - TESTS OFF (plan/NO-TESTS, D-210): compile-only gate — typecheck · build · go vet; no lint, no unit/integration tests, no apply-startup harness
    - deploy/vpp harness and integration skipped (tests OFF, D-210)
  mode quick · wall time 7m48s · logs /root/ngfw-wt/logs/ci/F-det44-cnat-fix-20261001-213900-343741

CI GATE PASSED
```
After the gate only docs changed (this section, the questions file's Q2 source reference, Q6).

## Open questions

`F-det44-cnat-fix-questions.md`: Q1 VPP det44 V-entry, Q2 cnat stale sessions, Q3 scheduler re-creates det44
dependents on every agent restart, Q4 suspect (b) not needed, Q5 harness latent bugs fixed, Q6 `/tmp/g-w16` not
deleted (permission refusal; `dist/` and `apps/agent/bin` of the worktree are deleted, no process of mine is left,
`rig: down`).

## Fix round 1 (review R4+R1 @ 90418916, APPROVE WITH CHANGES)

- Finding 1 (D-211): the split test's failure cleanup uses `scope.lossNoPool` — every slot object is removed via binapi
  except DS-Lite pool addresses, which are only logged; no `dslite_add_del_pool_addr_range is_add=0` from that test.
- Finding 2 (granted shared hunk `apps/agent/internal/desired/det44.go`): `assembleDet44` skips det44.interface objects
  whose interface ends in `det44.LeftoverSuffix` (no `#leftover` name, no `enabled: true` from a leftover alone).
  Test `TestAssembleDet44SkipsLeftovers` (new file `internal/desired/det44_leftover_test.go`).
- Finding 4: Delete of a leftover with SwIfIndex 0 (a rollback "re-create") is a no-op.
- Finding 3 → Out of scope (tech debt): `leftovers()` costs 2 `feature_is_enabled` per owned interface per plan.
- Findings 5, 6 not done (optional).

```
$ go test -count=1 -run TestAssembleDet44SkipsLeftovers ./internal/desired/
ok  	ngfw/agent/internal/desired	0.120s
$ go test -count=1 ./internal/descriptors/det44/
ok  	ngfw/agent/internal/descriptors/det44	0.027s
$ (test/topology/det44) go vet .   → clean
```
No host re-run (cleanup-path filter only).
