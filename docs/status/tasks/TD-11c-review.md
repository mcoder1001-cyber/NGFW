# TD-11c review: alias-aware delete order, untagged-NIC claims in core, keyed claim batching

Reviewer: review agent, 2026-09-24. Branch `task/TD-11c` @ 853f1ea. The branch merged main twice, so I reviewed its own
changes as `git diff 869c580 task/TD-11c`, where 869c580 is the merge-base with main. That diff has 18 files: scheduler topo,
core `{core,ifaddr}.go`, the `core.Register` line, KeyedClaims batching, the Q1 agent hunk, tests and docs. I read it against
envelope, the §3 verify results, TD-11b's claim-first design (task/TD-11b @ 3b1931d) and TD-8 (task/TD-8 @ 8a96a9c; TD-9 has
no commits yet). I made no host runs and did not run `tools/ci.sh`, as the manager instructed. I rely on the pasted CI and host
evidence, and it is consistent with the code (claim key format, holders, table 10011).

**Verdict: APPROVE WITH CHANGES.** 3.1c (topo) and 3.1b (core claim path) are correct and well tested. The 3.2 batching
removes the claim durability that TD-11b's claim-first order depends on (F1), and it reports a failed flush as APPLIED (F2).
Both are in TD-11c's own hunks and small to fix. Fix them before merge; the alternative is a D-entry that explicitly
accepts N3, which I advise against. N4 needs a correction (F3).

## What I ran
```
$ cd apps/agent && go test -race -count=1 ./internal/scheduler/... ./internal/descriptors/core/... ./internal/subsystems/...
ok  	ngfw/agent/internal/scheduler	1.330s
ok  	ngfw/agent/internal/descriptors/core	1.244s
?   	ngfw/agent/internal/descriptors/core/coretest	[no test files]
ok  	ngfw/agent/internal/subsystems	1.770s
$ TMPDIR=/tmp/g-rv11c go test -race -count=1 ./internal/agent/      # short TMPDIR: unix socket path limit
ok  	ngfw/agent/internal/agent	9.448s
$ go vet ./internal/scheduler/ ./internal/descriptors/core/ ./internal/subsystems/ ./internal/agent/
go vet: clean
# base check: 869c580's reconciler.go dropped into a scratch copy, topo_through_test.go removed (uses the new func)
$ go test -count=1 -run TestDeleteOrderFollowsAliasToCreator ./internal/scheduler/
--- FAIL: TestDeleteOrderFollowsAliasToCreator (0.01s)
    --- FAIL: .../sub-interface_removed,_parent_stays
    --- FAIL: .../af_packet_interface_removed_with_address,_VRF_binding,_admin_state,_MTU
    --- FAIL: .../bond_removed_with_member_and_address
    --- FAIL: .../bridge_with_BVI_removed
FAIL	ngfw/agent/internal/scheduler	0.032s
```
(With the long scratchpad TMPDIR, three agent tests fail with `bind: invalid argument`. That is the 108-byte unix socket path
limit, not the branch.)

Contract: the diff has no changes under packages/schema, packages/proto, gen or binapi. `exec.Command` appears only in the
integration test's fixed-argv `vppctl show` and `ip` rig helpers.

## Findings, ranked

### F1: HIGH, fix before merge. Batching (N3) removes the durability of TD-11b's claim-first order for every KeyedClaims family
`subsystems/stores.go:206-217` (`setLocked` batch branch), `:370-395` (`Begin`, `ClaimsTxn`); `agent/service.go:338-341, 390-393`.
- **What happens.** TD-11b makes natcommon claim before the VPP write (3.3), so a crash can leave a claim on nothing but
  never an unrecorded object in VPP. natcommon's store is `KeyedClaims("nat")`, and ACL/df2 use `KeyedClaims("acl")`. With
  TD-11c, a claim reaches the file only at the end of the transaction.
- **Scenario.** F-nat44-ed-sessions commits 2000 static mappings. The agent dies after VPP wrote mapping *k* but before the
  flush: OOM, SIGKILL at the systemd stop timeout, or a panic before TD-9's recover.
  - On restart, the claims file lacks *k*, and the saved desired state is the old one (`st.save` runs after the flush).
  - Retrieve reports only claimed objects, so *k* is invisible. A static NAT that no committed revision contains stays live
    and outside drift detection.
  - Every retry of that commit fails at Create with VPP "exists" until VPP restarts. The reconciler cannot converge
    (architecture rule 2, DoD 2).
  - This is the 3.3 failure class again, with the whole transaction as the window.
- **N3 is not acceptable as written.** The fix still does not need an fsync per claim:
  - Claims are bound to the VPP boot identity (D-080).
  - VPP runs on the agent's kernel. Anything that loses un-fsynced page cache (kernel panic, power loss) also restarts VPP
    and voids every claim.
  - So claim-first needs durability only against the death of the *agent process*, and a plain `write(2)` gives that.
- **Fix, only in the stores.go KeyedClaims hunk; no scheduler hook, and TD-11b's order stays as it is:**
  - Inside a batch, `claim` and `release` append one record or tombstone line to `claims-<family>-<owner>.journal`
    (O_APPEND, no fsync) before they return. The claim is in the kernel before the descriptor's VPP write.
  - `Flush` writes the snapshot once (the current `atomicWrite`) and truncates the journal.
  - `openClaims` replays snapshot + journal and tolerates a torn last line.
  - A failed journal write fails the Claim, so Create fails and the transaction rolls back (real txn semantics).
  - The cost is one syscall per claim. Re-measure 2000 claims and paste the result; expect milliseconds.
  - New test: `ClaimsTxn` → `Claim` → reopen the store without `Flush` → the claim is present. It fails today.
- **Only if the manager keeps N3:**
  - A D-entry that explicitly amends TD-11b's guarantee for KeyedClaims families.
  - An fsync'd "txn-open" marker, written at the first claim that dirties a batch and cleared by the flush.
  - On a start with the marker present: DEGRADED plus an alarm that names the store ("keyed objects of an interrupted
    transaction may be orphaned in VPP; a VPP restart clears them").
  - I do not recommend this path.

### F2: MEDIUM, fix before merge. A failed claim flush is reported as APPLIED
`agent/service.go:390-395`; `agent/claimstxn_test.go:30` asserts APPLIED on "disk full".
- **What happens.** The API marks sync in-sync on APPLIED; only DEGRADED goes through lostTrack plus the running re-apply
  (`apps/api/src/commit/commit.service.ts:576-611`). Meanwhile the agent saves the new desired state (`st.save`, :395)
  without the claims. An agent restart before the next good flush lands in F1's state, and nobody was told.
- **Fix.**
  - Flush right after `ApplyWith`/`fillResponse` and before the outcome switch, so rollback releases are included.
  - On error, turn APPLIED into DEGRADED ("claim stores not persisted: …"), so `st.desired` is not merged and the API
    re-applies running.
  - With F1's journal, a failed snapshot write is harmless (the journal holds the records): log it and retry at the next
    flush. Either way, change the test.

### F3: MEDIUM, forward. 3.1c orders a delete correctly in the product only when the retrieved alias names its creator; N4 is wrong for tunnels
`descriptors/interface/alias.go:137-140` sets `Creator` only through `Table.KeyFor`. `KeyFor` knows only the device classes in
`dump.go:49-55` (Loopback, tap, af-packet, bond, memif) plus sub-interfaces by type, and `iface.RegisterKind` has zero callers
in any worktree.
- **Kinds that miss.** The df6 tunnel creators (`gre.tunnel` "GRE tunnel device", ipip "IPIP tunnel device", vxlan "VXLAN",
  and the other `df6.IfDescriptor` families) tag their interfaces but are not mapped and implement no KeyProvider.
- **Effect.** Their retrieved alias has an empty creator, so `targets()` returns nothing and the delete order falls back to
  registration order. df6 is registered after DF-1, so the tunnel is deleted before its admin state and MTU: the
  "Invalid sw_if_index" failure from F-vlan-qinq Q1.
- **Why the tests miss it.** The fake (`topo_alias_test.go`) puts the creator into the alias value, so it cannot see this gap.
- **Kinds that are covered.** Today's wiring: loopback (including the BVI role, which F-loopback-bvi models as a loopback),
  af_packet and sub-interface; also F-bonding's bond. wireguard and ipsec itf are KeyProviders.
- **Fix.**
  - Amend N4: the obligation is "map the VPP device class (`iface.RegisterKind`) or provide `interface/<name>` (KeyProvider)",
    not "none".
  - Add the obligation to the wave-B/C tunnel rows' envelopes and to the review checklist.
  - Best: a subsystems guard test. For each registered interface creator, create one interface on the fake VPP and assert
    that the retrieved alias carries the creator key.
  - Cheapest code fix: a `DevType` field in `df6.IfSpec`, with `NewIfDescriptor` calling `iface.RegisterKind`. Owner: the
    first row that wires df6.

### F4: LOW. The Q1 bracket is not panic-safe, and TD-8's second transaction boundary is not bracketed
`agent/service.go:338-341, 390`.
- **Panic path.** A panic between Begin and the flush (in `project`/`fillResponse`; descriptor panics become errors only
  after TD-9) leaves every KeyedClaims store in batch mode.
- **TD-8 path.** TD-8's `syncLocked` (dynsource.go, `sched.ApplyWith` outside `applyLocked`) is a second transaction
  boundary. Its claims write at once, which is correct but unbatched. After such a panic they would stay in memory only.
- **Fix.**
  - Add a `defer` that flushes if the flush has not run.
  - After TD-8 merges, use one begin/end helper in both `applyLocked` and `syncLocked`, or let `BeforeTxn` return an
    after-func.
- **Merge check.** Against TD-8 there is no textual conflict: TD-8's `applyLocked` edits are in the `else` branch
  (`applySources`), and TD-11c's are after "reconcile start" and after the switch. With TD-8, several `ApplyWith` attempts
  share one batch, which is correct.
- **TD-9 obligation (add to its envelope).** Keep the flush after the outcome and before `st.save` on every path, including
  timeout/DEGRADED. Never run it on the caller's cancellable ctx.

### F5: LOW. The table binding on an untagged NIC takes over a foreign binding
`descriptors/core/ifaddr.go:94-101, 147`. `p4` is read only for the restore path. If the NIC is already bound to a non-zero
table and we hold no claim (F-startup-gen, an operator), Create rebinds it and Delete resets it to 0 instead of `p4`. That
takes over foreign state and then destroys it, which goes against N1's own rule. Fix: when either family's table is non-zero
and there is no claim, refuse with a "bound to table %d by someone else" error, and add a test.

### F6: LOW. A claim bound to a stale sw_if_index is never released
`ifaddr.go:144`, `:293`. Delete returns nil when `logical()` is false, so a claim tied to a NIC that someone else re-created
stays in `claims-iface` until the next VPP boot's Prune. Fix: when the record exists but does not match, release it (it is
dead either way).

### F7: LOW, merge note. Core claims have no ctx
`core.ClaimStore` (`core.go:79-83`) has no ctx. TD-11b adds `ClaimContext`/`ClaimedContext` (the R2-stores ctx-bounded
refresh). When the second of the two merges, core should prefer those, as `iface.ContextClaimStore` does. Otherwise core's
claims keep the 5 s `legacyBound`.

## The five focus questions
1. **Topo through the alias.** Correct.
   - It checks nodes first, then node aliases, then non-node objects from the Retrieve snapshot. It walks them transitively
     with a seen-set, so non-node cycles terminate.
   - A cycle can only come from a real cycle in the after-state graph, and it is reported as a plan issue, not misordered.
   - Create order changes only where a true transitive edge was missing. P08's projection emits every alias, so product
     documents keep their create order.
   - `executor.dependents` already walks live aliases.
   - For every creator kind it is correct *if* the retrieved alias names the creator (F3).
2. **Per-address claims.**
   - N1 is correct. VPP refuses a duplicate address with `DUPLICATE_IF_ADDRESS` unless the address is flagged STALE
     (`ip4_forward.c:724-756`), so a foreign address is never adopted by an add that succeeds. The STALE residual is negligible.
   - Delete releases the claim, and releases it too when the NIC vanished.
   - Restart safety holds (boot + sw_if_index binding; the host proof shows an empty plan after restart).
   - Gaps: F5 and F6.
3. **Batching.** Not acceptable as N3 stands. It conflicts with TD-11b's claim-first order; F1 reconciles the two at almost
   no cost, because page-cache durability is enough here.
4. **Q1 hunk.** Accept it; the transaction boundary is the right hook. Add F2's status change and F4's `defer`. There is no
   textual conflict with TD-8. Semantically, dynsource transactions are not bracketed, which is harmless; unify the bracket
   when TD-8 lands. TD-9 has not started, so record its obligation (F4).
5. **Q2.** I recommend no change to the projection code.
   - Inferring "existing" from live VPP state would make the same document mean create or adopt depending on actual state.
     That is not declarative, and after a VPP restart the agent would create and own an interface it had only decorated
     before.
   - af_packet is always agent-created (D-010/D-105). The product case is a DPDK NIC (KindExisting), which is covered on
     the fake.
   - Recommended row:
     - Add to the acceptance of F-startup-apply's real run, the first DPDK run that TD-11c gates: `interfaces.<dpdk nic>.ipv4`
       + `vrf` committed through `POST /api/v1/config/commit` → applied, `vppctl show int addr`, the claims-iface records,
       agent restart → empty plan, removal releases the claims.
     - Add a docs/tech-debt line for TD-11a: "`host-<netdev>` always names an agent-created af_packet; an untagged
       af_packet cannot be named (by design)".
     - If it is ever needed, the fix is an explicit additive schema flag on a contract branch.
   - Q3: agreed.

## Scope
The `service.go`/`agent.go`/`service_test.go` hunk is outside the file list, and it is justified (Q1). The `plan()` call-site
line in reconciler.go belongs with the topo hunk. Nothing else is outside the envelope.

**APPROVE WITH CHANGES.** F1 and F2 before merge. At merge: correct N4 in the LOG and in the tunnel rows' envelopes (F3).
F4–F7 may go to tech-debt with owners (F4 → whoever merges second of TD-8/TD-11c, plus TD-9's envelope).
