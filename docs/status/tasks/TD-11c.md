# TD-11c: untagged NICs, delete order through the alias, claim stores at scale (REVIEW-2026-09-24 §3, D-125)

Worker: slot 10 (`w10`, tables 10000–10999). Branch `task/TD-11c`. Base: task/P08@998e391 (speculative, D-114). `main` was merged
in after P08 merged (1de0b3d) and again after W-seed merged (d364013). Questions and notes are in
[TD-11c-questions.md](TD-11c-questions.md): Q1 is a 13-line agent hunk outside my file list, Q2 is the host proof path, and N1–N4 are
the decisions I took. The agent stays declarative: nothing here changes what a transaction means, only which objects it can own and
in what order it deletes them.

## What

### 3.1c: topological order through the `interface/<name>` alias (scheduler/reconciler.go, topo hunk)
`topo` → `topoThrough(nodes, through)`. `plan` passes its Retrieve snapshot as `through`. Some dependencies name an object that is
not a plan node: the observe-only alias (D-065) of an interface this transaction deletes, which stops being a node once it leaves
the desired state. Such a dependency is now followed through that object's own dependencies (transitively, through the aliases
non-node objects provide, with a seen-set against cycles) to the nodes behind it. Attribute → alias → creator therefore becomes an
edge, and deletes run dependents-first for every interface kind, whatever the registration order. An alias without a creator (a
physical NIC) adds no edge. `executor.dependents` keeps the old `topo(nodes)` wrapper, and executor.create, ApplyWith and recover
are untouched (TD-11b and TD-9).

### 3.1b: addresses and VRF binding on an untagged NIC (descriptors/core/{core,ifaddr}.go, core.Register hunk)
- `core.Env.Claims` is a new `core.ClaimStore`, the same shape as `iface.ClaimStore`. `subsystems.Register` passes the persisted
  `Wiring.IfaceClaims()`, which binds each claim to the D-080 boot identity and the sw_if_index (D-071 claim path).
- `interface-ip.table` and `interface-ip` resolve an interface in this order: our tagged interface first; then, when a store is
  set, an untagged interface of that VPP name (its logical name, D-069). Another owner's interface and local0 are refused as
  before. So is an untagged interface when there is no store, and the core unit tests keep that behaviour.
- Create claims first and then writes; if VPP refuses, the claim is released. For a table binding where IPv4 was set and IPv6 was
  refused, IPv4 gets its previous table back first. Delete re-verifies the claim on the current sw_if_index, removes the
  address or binding, and then releases. A binding or address we do not hold is never touched. If the NIC has vanished, only the
  claim is released.
- Retrieve reports a tagged interface's objects as before. On untagged interfaces it reports only claimed objects.
- Holders: `interface-ip.table`, plus one `interface-ip|<canonical prefix>` per address (N1). An address somebody else put on
  the NIC is never adopted.

### 3.2: keyed claim stores flush once per transaction (subsystems/stores.go, KeyedClaims batching)
- `KeyedClaims.Begin/Flush` and `Wiring.ClaimsTxn()`. Inside a transaction, Claim, Release and Prune change only the in-memory set
  (Claimed reads it). The end of the transaction writes each store that changed exactly once, using the same atomic write as before:
  temp file, fsync, rename, directory fsync, 0600. An unchanged store is not written. If the write fails, the records stay in memory
  and dirty, and the next write persists them. Outside a transaction, writes still happen at once. `IfaceClaims` is unchanged.
- The agent brackets every transaction (apply, resync, revert) in `Service.applyLocked`. The claims are flushed after the outcome
  and before the new desired state is saved. A failed flush makes the agent DEGRADED until a later flush succeeds. This part is
  outside my files (Q1). Fix round 1 (below) replaced the N3 trade-off with a journal (D-133) and made a failed flush DEGRADED.

## How verified

### The tests fail on the base first
3.1c, scheduler shapes, on 998e391 (`go test -run TestDeleteOrderFollowsAliasToCreator ./internal/scheduler/`):
```
--- FAIL: TestDeleteOrderFollowsAliasToCreator (0.01s)
    --- FAIL: TestDeleteOrderFollowsAliasToCreator/sub-interface_removed,_parent_stays (0.00s)
        topo_alias_test.go:156: delete interface.admin-state/host-p0.100 must run before delete interface.subinterface/host-p0.100; ops:
              delete interface.subinterface/host-p0.100
              delete interface.admin-state/host-p0.100
              delete interface-ip/host-p0.100
    --- FAIL: TestDeleteOrderFollowsAliasToCreator/af_packet_interface_removed_with_address,_VRF_binding,_admin_state,_MTU (0.00s)
        topo_alias_test.go:156: delete interface-ip/host-p1 must run before delete af-packet.host-interface/host-p1; ops:
              delete interface.mtu/host-p1
              delete interface.admin-state/host-p1
              delete af-packet.host-interface/host-p1
              delete interface-ip/host-p1
              delete interface-ip.table/host-p1
    --- FAIL: TestDeleteOrderFollowsAliasToCreator/bond_removed_with_member_and_address (0.00s)
        topo_alias_test.go:156: delete interface.admin-state/BondEthernet0 must run before delete bond.bond/BondEthernet0; ops:
              delete bond.member/lan
              delete bond.bond/BondEthernet0
              delete interface.admin-state/BondEthernet0
              delete interface-ip/BondEthernet0
    --- FAIL: TestDeleteOrderFollowsAliasToCreator/bridge_with_BVI_removed (0.00s)
        topo_alias_test.go:156: delete l2.bridge-member/10-bvi10 must run before delete bvi.interface/bvi10; ops:
              delete l2.bridge-member/10-lan
              delete bvi.interface/bvi10
              delete l2.bridge-member/10-bvi10
              delete l2.bridge-domain/10
              delete interface-ip/bvi10
FAIL	ngfw/agent/internal/scheduler	0.035s
```
3.1b and 3.1c through the product wiring on the fake VPP, on the base agent code (`./internal/agent/`, before 0008a42):
```
--- FAIL: TestPhysicalNICAddressAndVRF (0.02s)
    untagged_test.go:69: status APPLY_STATUS_ROLLED_BACK, want APPLY_STATUS_APPLIED: create interface-ip.table/lan: core: interface is not owned by this agent: "lan" …
--- FAIL: TestSubinterfaceRemovedWhileParentStays (0.01s)
    untagged_test.go:135: removing the sub-interface: APPLY_STATUS_ROLLED_BACK delete interface.admin-state/host-zq11c0.100: sw_interface_set_flags: VPPApiError: Invalid sw_if_index (-2)
          interface.subinterface/host-zq11c0.100 APPLY_OPERATION_DELETE OBJECT_RESULT_CODE_REVERTED
          interface.admin-state/host-zq11c0.100 APPLY_OPERATION_DELETE OBJECT_RESULT_CODE_FAILED sw_interface_set_flags: VPPApiError: Invalid sw_if_index (-2)
          interface-ip/host-zq11c0.100/10.10.100.1/24 APPLY_OPERATION_DELETE OBJECT_RESULT_CODE_SKIPPED
--- FAIL: TestInterfaceRemovedDeletesAttributesFirst (0.01s)
    untagged_test.go:178: address-del never sent (deleted after the interface was gone?): mtu-default,admin-down,af_packet_delete
FAIL	ngfw/agent/internal/agent	0.086s
```
3.2 on the base: the API does not exist yet (`./internal/subsystems/`):
```
internal/subsystems/stores_batch_test.go:51:13: w.ClaimsTxn undefined (type *Wiring has no field or method ClaimsTxn)
internal/subsystems/stores_batch_test.go:70:9: nat.writes undefined (type *KeyedClaims has no field or method writes)
FAIL	ngfw/agent/internal/subsystems [build failed]
```
3.2, the cost on the host disk (2000 keyed claims in one transaction, a scratch test with TMPDIR=/var/tmp; the base run uses only
the existing API):
```
base:   500 keyed claims in one transaction: 2.652s  (500 whole-file rewrites, 2 fsyncs each)
base:  1000 keyed claims in one transaction: 6.594s
base:  2000 keyed claims in one transaction: 21.961s
TD-11c: 500 keyed claims in one transaction: 14ms (1 file write)
TD-11c: 1000 keyed claims in one transaction: 5ms (1 file write)
TD-11c: 2000 keyed claims in one transaction: 35ms (1 file write)
```

### After
```
--- PASS: TestDeleteOrderFollowsAliasToCreator (0.01s)
    --- PASS: TestDeleteOrderFollowsAliasToCreator/sub-interface_removed,_parent_stays (0.00s)
    --- PASS: TestDeleteOrderFollowsAliasToCreator/af_packet_interface_removed_with_address,_VRF_binding,_admin_state,_MTU (0.00s)
    --- PASS: TestDeleteOrderFollowsAliasToCreator/bond_removed_with_member_and_address (0.00s)
    --- PASS: TestDeleteOrderFollowsAliasToCreator/bridge_with_BVI_removed (0.00s)
--- PASS: TestTopoThroughNonNodes (0.00s)
ok  	ngfw/agent/internal/scheduler	0.049s
--- PASS: TestUntaggedInterfaceNeedsClaimStore (0.00s)
--- PASS: TestUntaggedInterfaceClaimPath (0.00s)
--- PASS: TestUntaggedPartialBindRestored (0.00s)
ok  	ngfw/agent/internal/descriptors/core	0.015s
--- PASS: TestKeyedClaimsFlushOncePerTxn (0.09s)
--- PASS: TestKeyedClaimsPruneInTxn (0.00s)
--- PASS: TestKeyedClaimsAndCorruptFile (0.00s)
ok  	ngfw/agent/internal/subsystems	0.134s
--- PASS: TestClaimStoresBracketEveryTransaction (0.05s)
--- PASS: TestPhysicalNICAddressAndVRF (0.03s)
--- PASS: TestSubinterfaceRemovedWhileParentStays (0.02s)
--- PASS: TestInterfaceRemovedDeletesAttributesFirst (0.00s)
ok  	ngfw/agent/internal/agent	0.152s
```
What the tests cover:
- `TestPhysicalNICAddressAndVRF`: Service + projection + product wiring on the fake. `lan` (untagged, DPDK-style) gets a VRF and
  IPv4/IPv6 addresses, and `wan` gets an address. The result is APPLIED, VPP has the binding and the addresses, Retrieve equals the
  canonical document, and the claims are on disk. Somebody else's address on `wan` is neither reported nor removed. A restart (a new
  Service over the same state dir) writes nothing to VPP. Removal unbinds and removes our objects and releases the claims; the NICs
  and the foreign address stay.
- `TestSubinterfaceRemovedWhileParentStays`: F-vlan-qinq Q1, without the sub-interface KeyProvider.
- `TestInterfaceRemovedDeletesAttributesFirst`: af_packet_delete is the last VPP write, after the address delete, the table
  unbind, admin down and the MTU default.
- `TestUntaggedInterfaceClaimPath` / `TestUntaggedPartialBindRestored`: claim-first, release when VPP refuses, IPv4 restored
  after an IPv6 refusal, another owner's interface refused, unclaimed addresses untouched.
- `TestClaimStoresBracketEveryTransaction`: one begin and one flush per transaction, including a transaction that fails
  validation. A failed flush degrades the agent and a later good one clears it.

Full agent module: `go vet ./...` is clean, and `golangci-lint run` on scheduler, core, subsystems and agent reports `0 issues.`
`go test -race -count=1 ./...` passes with every package ok.

### Host proof (slot 10, one package, shared lab lock; no packets, no trace/classify commands, D-126/D-128)
`VRX_INTEGRATION=1 go test -run TestUntaggedNICClaimsOnHost -v ./internal/subsystems/`. The product af_packet descriptor creates
`host-w10-u0` on the w10 veth (sanitized, TD-3/TD-5) and its owner tag is removed. From then on it is a pre-existing NIC. The
product wiring (`subsystems.Register` → core with the persisted IfaceClaims, the DF-1 alias, the scheduler) applies VRF 10011 + binding +
10.10.0.1/24 on it. Teardown re-tags the interface and deletes it through the descriptor (quiesced). The test cannot go through the
gRPC projection; Q2 says why.
```
NRestarts before: 1 (MainPID 2006833) 23:16:59
=== RUN   TestUntaggedNICClaimsOnHost
    untagged_integration_test.go:74: untagged NIC host-w10-u0 (sw_if_index 2); vppctl show interface:
                      Name               Idx    State  MTU (L3/IP4/IP6/MPLS)     Counter          Count
        host-w10-u0                       2     down         9000/0/0/0
    untagged_integration_test.go:135: after apply: Retrieve == desired: vrf/10011 id:10011  vrf:"td11c"
    untagged_integration_test.go:135: after apply: Retrieve == desired: interface/host-w10-u0 name:"host-w10-u0"
    untagged_integration_test.go:135: after apply: Retrieve == desired: interface-ip.table/host-w10-u0 interface:"host-w10-u0"  table_id:10011
    untagged_integration_test.go:135: after apply: Retrieve == desired: interface-ip/host-w10-u0/10.10.0.1/24 interface:"host-w10-u0"  prefix:"10.10.0.1/24"
    untagged_integration_test.go:136: vppctl show interface address:
        host-w10-u0 (dn):
          L3 10.10.0.1/24 ip4 table-id 10011 fib-idx 4
    untagged_integration_test.go:143: claims-iface-w10.json:
        [
         {
          "key": "host-w10-u0|interface-ip.table",
          "boot": "a93c0e7a-40b7-4755-9b0b-07eefa24137d/2006833/3203894",
          "sw_if_index": 2
         },
         {
          "key": "host-w10-u0|interface-ip|10.10.0.1/24",
          "boot": "a93c0e7a-40b7-4755-9b0b-07eefa24137d/2006833/3203894",
          "sw_if_index": 2
         }
        ]
    untagged_integration_test.go:147: after restart: Retrieve == desired: vrf/10011 id:10011  vrf:"td11c"
    untagged_integration_test.go:147: after restart: Retrieve == desired: interface/host-w10-u0 name:"host-w10-u0"
    untagged_integration_test.go:147: after restart: Retrieve == desired: interface-ip.table/host-w10-u0 interface:"host-w10-u0"  table_id:10011
    untagged_integration_test.go:147: after restart: Retrieve == desired: interface-ip/host-w10-u0/10.10.0.1/24 interface:"host-w10-u0"  prefix:"10.10.0.1/24"
    untagged_integration_test.go:151: after restart: plan empty (the claims were loaded; nothing to write)
    untagged_integration_test.go:163: after removal: vppctl show interface address:
        host-w10-u0 (dn):
--- PASS: TestUntaggedNICClaimsOnHost (0.71s)
ok  	ngfw/agent/internal/subsystems	0.752s
NRestarts after: 1 (MainPID 2006833) 23:17:03
```
Journal lines from the run: `af_packet quiesce: netdev down before af_packet_delete (D-101, VPP V24) netdev=w10-u0 … kind=veth`.
Afterwards the slot is clean: `vppctl show interface` has 0 lines with w10, there are 0 `w10` netdevs, and no w10 table exists.

### CI
`TMPDIR=/tmp/g-w10 tools/ci.sh --base main` on the final code tip (d364013, main with W-seed merged in):
```
branch    task/TD-11c @ d364013   (base: main)
logs      /root/ngfw-wt/logs/ci/TD-11c-20260924-232946-569908
no contract files changed in the 9 commit(s) of HEAD since main (869c580)
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~585328 bytes (585.33 KB) in 1.58s no leaks found
Tasks:    30 successful, 30 total Cached:    24 cached, 30 total Time:    2m39.352s
== apps/agent: make lint test build ==  (every package ok)
shellcheck ok: ./apply-startup.sh ./build.sh ./lib.sh ./test-apply-startup.sh ./verify.sh
CI GATE PASSED
```
An earlier run on c5d8521, before the W-seed merge, also passed (`CI GATE PASSED`).

## Fix round 1 (review TD-11c-review.md @ ac313a1c, D-133)
All changes are unit-tested on the fake. `main` was merged in at 50798421, after TD-8 merged, so F5 covers TD-8's `syncLocked`.

| item | done | where |
|---|---|---|
| **F1** claim-first durable (HIGH) | Inside a transaction every keyed Claim or Release appends one line to `claims-<family>-<owner>.journal` (O_APPEND, one write(2), no fsync) **before it returns**, so before the descriptor writes VPP. A failed append fails the Claim; a short write is cut back, so the journal holds only whole lines. The transaction end compacts: one atomic, fsync'd snapshot write, then the journal is emptied. `OpenKeyedClaims` replays the journal over the snapshot and compacts it. A torn last line is dropped; a bad line before it fails closed, like a corrupt snapshot. Prune within a batch is not journaled: a replayed record of another boot is ignored and pruned again at the next connect. | stores.go (KeyedClaims hunk), f87c6a60, 2f2e9ef0 |
| **F2** failed flush ≠ APPLIED | The flush runs after the transaction and **before** the outcome switch. On failure APPLIED becomes DEGRADED ("claim stores not persisted: …"), so the desired state is not merged and the API re-applies running. Other outcomes keep their status, and the agent is marked DEGRADED. The records stay in memory and in the journal; the next transaction end writes them. | service.go `applyLocked`, `claimsNotPersisted` |
| **F3** N4 amended + guard | The obligation for every interface creator is `iface.RegisterKind` **or** a KeyProvider for `interface/<name>`. `subsystems/TestEveryInterfaceCreatorNamesItsAlias` builds every creator, checks the device-class mapping (`iface.Kind`) and `scheduler.KeyProvider`, and fails for a creator with neither. A source scan fails for an interface-creating descriptor package that is not in the table. The known gaps are in a shrink-only allowlist, listed below. Mutation check: removing gre from the allowlist fails the test, and removing gre from the table fails the source scan. | creators_guard_test.go, cc44bbbc |
| **F4** panic-safe bracket | `claimsBatch()` returns end plus a deferred cleanup that ends the batch if the transaction panicked before end ran. | service.go |
| **F5** TD-8's second boundary | TD-8 merged, so `syncLocked` now uses the same `claimsBatch()` and `claimsNotPersisted` (a failed end means not in sync, DEGRADED). | dynsource.go (3-line hunk), a83c37ef |
| F6, F7 | Left for later: F6, a stale-index claim is released only at the next VPP boot's Prune; F7, core claims have no ctx; TD-11b has merged, so core can now switch to `ClaimContext`/`ClaimedContext` (the 5 s `legacyBound` applies until then). | — |
| Q2 | The projection stays as it is. The gRPC-on-DPDK proof goes to F-startup-apply's real run (manager note). | — |

**Interface creators without an alias creator today** (the F3 allowlist; each row fixes its own; output of the guard):
```
interface.loopback: device class mapped
af-packet.host-interface: device class mapped
interface.subinterface: device class mapped
tapv2.tap: device class mapped
bond.bond: device class mapped
memif.memif: device class mapped
wireguard.interface: KeyProvider
ipsec.itf: KeyProvider
mpls-tunnel: GAP → F-mpls-srmpls
gre.tunnel: GAP → F-tunnels
ipip.tunnel: GAP → F-tunnels
ipip.sixrd: GAP → F-tunnels
vxlan.tunnel: GAP → F-tunnels
vxlan-gpe.tunnel: GAP → F-tunnels
gtpu.tunnel: GAP → F-tunnels
gtpu.forward: GAP → F-tunnels (shares the GTPU class with gtpu.tunnel: needs a KeyProvider)
l2tp.tunnel: GAP → F-tunnels
pppoe.session: GAP → F-tunnels
lcp.itf-pair: GAP → P12 (untagged VPP-side host tap: needs a KeyProvider)
```
Device classes for F-tunnels' `RegisterKind` calls (VPP 26.06 `VNET_DEVICE_CLASS .name`): "GRE tunnel device", "IPIP tunnel
device", "ip6ip-6rd", "VXLAN", "VXLAN_GPE", "GTPU", "L2TPv3", "PPPoE", "MPLS tunnel device".

### The new tests fail before the fix
F1 on ac313a1c (`./internal/subsystems/`):
```
--- FAIL: TestKeyedClaimsSurviveAgentDeathMidTransaction (0.00s)
    stores_journal_test.go:131: claims of the interrupted transaction lost: 0 records
--- FAIL: TestKeyedClaimsJournal (0.00s)
    stores_journal_test.go:178: claim a not in the journal when Claim returned
FAIL	ngfw/agent/internal/subsystems	0.057s
```
`TestKeyedClaimsSurviveAgentDeathMidTransaction` runs the product wiring, a claim-first keyed family on an in-memory "VPP", and the
real scheduler. The transaction writes VPP, and the process "dies" before the transaction end (no compaction). A new agent over
the same state dir knows the claims: Retrieve reports both objects, re-committing the same document gives an empty plan
(idempotent), and removing them converges.

F2: the old `service.go` with the new test:
```
--- FAIL: TestClaimStoresBracketEveryTransaction (0.05s)
    claimstxn_test.go:38: status APPLY_STATUS_APPLIED, want APPLY_STATUS_DEGRADED:  results=[key:"interface.loopback/loop703" …]
FAIL	ngfw/agent/internal/agent	0.108s
```
F5: the pre-F5 `dynsource.go` with the new test:
```
--- FAIL: TestClaimStoresBracketSourceSync (0.04s)
    claimstxn_test.go:92: sync: begins 0 flushes 0, want 1 and 1
FAIL	ngfw/agent/internal/agent	0.102s
```
### After
```
--- PASS: TestEveryInterfaceCreatorNamesItsAlias (0.93s)
--- PASS: TestKeyedClaimsFlushOncePerTxn (0.16s)
--- PASS: TestKeyedClaimsPruneInTxn (0.00s)
--- PASS: TestKeyedClaimsSurviveAgentDeathMidTransaction (0.01s)
--- PASS: TestKeyedClaimsJournal (0.03s)
--- PASS: TestKeyedClaimsAndCorruptFile (0.00s)
ok  	ngfw/agent/internal/subsystems	1.193s
--- PASS: TestClaimStoresBracketEveryTransaction (0.10s)
--- PASS: TestClaimStoresBracketSourceSync (0.03s)
ok  	ngfw/agent/internal/agent	0.180s
```
`TestKeyedClaimsFlushOncePerTxn` changed its assertion: the claim of a transaction whose snapshot write failed is now on disk
(journal), where before it was only in memory. The whole agent module is green under `go test -race ./...`,
`go vet ./...` is clean, and golangci-lint on the touched packages reports `0 issues.`

### Performance (2000 keyed claims in one transaction, host disk, scratch test not committed)
```
base (998e391):          2000 claims: 21.961s  (2000 whole-file rewrites, 2 fsyncs each)
round 0 (batch only):    2000 claims: 35ms     (1 file write)
fix round 1 (journal):   2000 claims: 50ms / 51ms  (2000 journal appends 39ms / 37ms, 1 fsync'd snapshot write)
                         1000 claims: 13ms / 24ms   500 claims: 17ms / 23ms
```

### Merge notes
- **TD-11b: DONE (O1 from TD-11b's verify, 8106bd5c).** TD-11b merged to main at b5e74c08, and main was merged in at 2d1069cb
  without conflicts; the stores.go import block had been kept off TD-11b's line. As designed, TD-11b's tripwire
  `TestInterfaceObjectsRecordNoStore` then failed ("core.Env gained Claims …"). In `core/ownership.go`,
  `InterfaceTableDescriptor` and `InterfaceAddrDescriptor` now declare
  `CheckPersistent() error { return persist.Require("core interface-ip[.table]: claims", d.Claims) }` in place of
  `RecordsNoOwnership`. `Claims` is on the tripwire's field list, and the new `TestInterfaceObjectsRequirePersistedClaims` checks that
  nil and in-memory stores are refused (ErrVolatile) and a persisted one passes. The product wiring passes `subsystems.IfaceClaims`,
  which is persistent through fileClaims, so the agent's ownership guard accepts it (the agent and subsystems tests are green).
- **TD-9 (envelope obligation, review F4).** Keep `flushClaims()` after the outcome is known and before `st.save()` on every
  path, including timeout/DEGRADED. Never run it on the caller's cancellable ctx (it takes none today).

### Host
The optional host re-check was skipped. This round changes only the keyed claim stores and the transaction bracket, and the
round-0 host proof (core claims on an untagged af_packet NIC, `TestUntaggedNICClaimsOnHost`) does not exercise either. Slot 10
is also shared with the running F-unbound-chrony-syslog.

### CI (fix round 1)
Final, after TD-11b and O1:
`TMPDIR=/tmp/g-w10c tools/ci.sh --base main` at 8106bd5c (main with TD-11b merged in, plus O1):
```
branch    task/TD-11c @ 8106bd5c   (base: main)
logs      /root/ngfw-wt/logs/ci/TD-11c-20260925-035041-2355181
no contract files changed in the 19 commit(s) of HEAD since main (b5e74c08)
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~663143 bytes (663.14 KB) in 1.51s no leaks found
ok: no packet trace (trace add / show trace / clear trace / tracedump API) outside docs and the generated bindings
Tasks:    30 successful, 30 total Cached:    24 cached, 30 total Time:    2m23.864s
shellcheck ok: ./apply-startup.sh ./build.sh ./lib.sh ./test-apply-startup.sh ./verify.sh
CI GATE PASSED
```
Also: `go vet ./...` clean, `golangci-lint run ./...` reports `0 issues.`, and `go test -race -count=1 ./...` passes with every package ok.
After this run, main's d3a7c266 (status) and f03ec1a2 (board) were merged in; both are docs only, with no code.

Earlier, before TD-11b:
`TMPDIR=/tmp/g-w10c tools/ci.sh --base main` at 2f2e9ef0 (main with TD-8 merged in):
```
branch    task/TD-11c @ 2f2e9ef0   (base: main)
logs      /root/ngfw-wt/logs/ci/TD-11c-20260925-001500-1924018
no contract files changed in the 16 commit(s) of HEAD since main (8a633616)
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~649641 bytes (649.64 KB) in 1.32s no leaks found
ok: no packet trace (trace add / show trace / clear trace / tracedump API) outside docs and the generated bindings
Tasks:    30 successful, 30 total Cached:    24 cached, 30 total Time:    1m59.458s
== apps/agent: make lint test build ==  (every package ok)
shellcheck ok: ./apply-startup.sh ./build.sh ./lib.sh ./test-apply-startup.sh ./verify.sh
CI GATE PASSED
```

## Out of scope
- `core/README.md:64-68` (M5 is now done) and the `core.go:11-14` alias doc: these are TD-11a's hunks (Q3).
- The generic create-then-claim fix, the persistence guard and the ctx-bounded index refresh belong to TD-11b. A 3-way file merge
  of my files with task/TD-8 and task/TD-11b over main has no conflicts in my hunks (Q1 has the details).
- Batching for `IfaceClaims`: the envelope scoped 3.2 to KeyedClaims, and interface claims are few (N3).
- No packets, no DPDK. The DPDK NICs are still unbound (handover).

## Commits
0008a42 (3.1c) · 76739da (3.1b) · 1037660 (3.2) · c5d8521 (host proof) · 1c7ded2 (hunks moved off TD-8/TD-11b lines) · 1de0b3d and d364013 (main merged in) · the docs commit.
