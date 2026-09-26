# F-mpls-srmpls — questions and notes for the manager

Worker slot 5, branch `task/F-mpls-srmpls`. Nothing here blocks the task; each item says what I did meanwhile.

## Q1 — contract committed on the branch (please review)
`contract(schema): routing mpls` (2e57555c) and `contract(proto): routing mpls, MplsState` (df92f99c), described in
`F-mpls-srmpls-contract.md`. Numbers exactly as wave-BC-numbers.md: RoutingConfig 15; MplsConfig 1–6 used, 7–9 spare
(this task's), 10 left to F-mpls-ldp with a comment + `// wave-BC: F-mpls-ldp` anchor (no `reserved 10;`, so their
change stays an insert). No EventKind, no ActionRequest member. I kept building against the branch.

## Q2 — no example under `packages/schema/examples/` (test file not mine)
`packages/schema/src/examples.test.ts` fails on any file whose name is not in its sibling list
(`nat|objects|acl|vpn|tunnels|services|ha`), so a `mpls-srmpls-*.json` example (my C4 file pattern) cannot be added
without editing that test. The model is covered instead by `packages/schema/src/semantic/mpls-srmpls.test.ts` (18 tests)
and the proto round-trip/drift corpus `packages/proto/test/fixtures/mpls-srmpls-full.json`. Suggest: add `routing` (or a
per-feature prefix list) to the sibling regex in a manager/test-infra change.

## Q3 — who declares `mpls-table/0` in production (the answer F-mpls-ldp carries over)
Decided (for the LOG): **every agent declares `mpls-table/0` when its MPLS configuration needs it** — MPLS-enabled
interfaces, label bindings, SR-MPLS policies, or a label route of table 0 (`desired.MplsNeedsTableZero`); a configuration
with only extra tables and tunnels does not. **What the declaration does depends on the D-071 role**
(`mpls.NewTableFor` / `RegisterFor`): the globals owner (the product agent) creates it (named `<owner>:0`, exempt from the
id range) and deletes it when no longer needed; any other agent only **requires** it — Create succeeds while table 0
exists (whoever created it), otherwise the transaction fails with "MPLS table 0 does not exist in VPP; it is VPP-global and
only the globals owner creates it (D-071)"; it never creates or deletes it.
Options considered: (a) the globals owner declares it whenever `routing.mpls` is non-empty (the envelope's proposal) —
would make a slot agent with only slot tables fail on the shared host; (b) declare only when needed + role in the
descriptor (chosen); (c) the projection reads VRX_GLOBALS_OWNER — the projection runs in the service, which has no
wiring, and duplicating `ConfigFromEnv` would drift.
**For F-mpls-ldp:** LDP-learned labels live in table 0 — add `ldp` to `MplsNeedsTableZero` (anchor
`// wave-BC: F-mpls-ldp` in `desired/mpls_srmpls.go`).
**Open (product, not decided):** on a real box where table 0 already exists **unnamed** (created by VPP for linux-cp /
FRR label sync before the agent's first resync), the globals owner's Create refuses it as foreign (DF-7's rule for any
table). Proposal: the globals owner adopts an unnamed table 0 (only table 0, only unnamed). Left as DF-7 behaviour here.

## Q4 — TD-11c allowlist: `mpls-tunnel` is fixed
`mpls-tunnel` now provides `interface/<name>` (`scheduler.KeyProvider`, `TestTunnelProvidesInterfaceAlias`). TD-11c is not
on main, so its `creators_guard_test.go` is not on this branch: whichever of the two merges second must remove
`mpls.NameTunnel: "F-mpls-srmpls"` from `knownAliasCreatorGaps` (the guard fails "is fixed … remove it" otherwise).
I did not also call `iface.RegisterKind("MPLS tunnel device", …)`: an `interfaces.<tunnel>` entry is a KindExisting alias
without creator in P08's projection, so a mapped device class would make the retrieved alias differ from the desired one
on every resync (see Q8).

## Q5 — TD-23: coretest hook
`coretest/mpls_srmpls.go` (my A6 file) models MPLS + SR-MPLS; `fakevpp.go` got ONE line in `New()`
(`v.installMplsSrmpls()`), needed because the routing domain now retrieves MPLS objects in every agent test. When TD-23 is
on main this becomes `RegisterExtension("mpls-srmpls", (*VPP).installMplsSrmpls)` in my file and the line goes. The
extension hooks only `mpls_*` / `sr_mpls_*` messages (no collision with F-vrf-static-ecmp's `fib_source_dump` /
`ip_route_v2_dump`); SR steering tests call `UseSRFibSource()` explicitly.

## Q6 — id range when Env.IDs is empty
`registerMplsSrmpls` logs and continues with the empty range (owns no MPLS table id) on `ErrNoIDRange` instead of failing
the registration as `seams.go`'s comment suggests: every existing agent/subsystems unit test builds the wiring without
IDs, and the agent itself refuses a missing range (TD-8b). A malformed range still fails.

## Q7 — write-only leaves always show in the drift view
`routing.mpls.ipBindings` and `routing.mpls.sr` have no dump in VPP 26.06 (D-063): never in a Retrieve, so `/state/drift`
lists them as missing on the data-plane side forever. The UI says so (SR tab note). Suggest a generic "write-only leaves"
mask in the API's drift view (owner: API state, not this task).

## Q8 — an MPLS tunnel cannot also be an `interfaces` key
A semantic rule (`routing.mpls-srmpls-tunnel-name`) refuses it: P08's `desired.KindOf` treats any unknown name as an
existing interface (alias without creator), so an address on the tunnel would not be ordered after the tunnel's creation.
Addressing MPLS tunnels and IP static routes through them needs `KindOf` (A3) to know tunnel names — a follow-up (same
question as F-tunnels' interfaces).

## Q9 — no CLI `show mpls`
The generated operation table has `MplsSrmpls_fib` / `MplsSrmpls_tunnels`, but a `show mpls …` command belongs to
`apps/cli` (not my files). The user doc gives the REST calls and `show configuration routing mpls`.

## Q10 — one walk limiter per agent
`MplsState` serialises MPLS FIB walks with its own semaphore (one in flight, UNAVAILABLE after 3 s, D-132).
F-vrf-static-ecmp's `ListRoutes` has its own for IP FIB walks. An MPLS and an IP walk can therefore run at the same time.
Suggest one agent-wide walk limiter (a small shared helper) when both are on main.

## Q11 — for F-mpls-ldp: linux-cp MPLS sync
VPP 26.06 `lcp_mpls_sync.c`: enabling MPLS on an interface with an LCP pair also enables it on the host tap and writes
`net.mpls.conf.<tap>.input`. The tests here never enable MPLS on an LCP-paired interface; the user doc warns.

## Q12 — payload canonical form
The assembler omits a label route's `payload` when it equals the documented default (ip6 when a next hop is IPv6, else
ip4). A document that spells the default explicitly (`"payload": "ip4"`) shows in the drift view until it is written
without it; the UI writes whatever the form holds and shows the effective payload in the grid.

## Q13 — shared scratchpad
At 11:07 I copied my API e2e log to the session scratchpad as `e2e.log`, overwriting a file of that name if another task
had one there. My evidence now lives in `/tmp/g-w5/evidence/`; sorry if that `e2e.log` was someone else's.

## Host steps pending (host runs closed until TD-25; table-0 parts need a manager window)
1. After TD-25: `eval "$(tools/lab env 5)"; cd apps/agent; VRX_INTEGRATION=1 go test -count=1 -run TestMplsOnHost -v
   ./internal/agent/` — slot MPLS table 5001, label routes, tunnel, idempotent re-apply, MplsState, restart simulation,
   rollback, `vppctl show mpls fib table 5001` / `show mpls tunnel`. No packet is sent, no af_packet is used.
2. Manager window (D-071/D-082): the same with `VRX_DF7_GLOBALS=1` (the test takes `flock -x /run/lock/vrx-globals.lock`,
   creates MPLS table 0 only if none exists and deletes it after): MPLS on the loopback, a label binding, a table-0 label
   route, SR-MPLS policy + steering; `show sr mpls policies`, `show mpls interface`, `show mpls fib table 0`.
3. Optional: real-stack screenshots (the committed ones use the API's fake agent).
