# Review R4 (data plane / VPP safety & shared-host rules): agent-allifs-unbound-nics

- **Reviewer:** panel R4 (Opus 5.5), read-only agent. No files were modified. Nothing was run against VPP, a lab slot, systemd or the host.
- **PR:** #241, branch `claude/agent-allifs-unbound-nics`.
- **Head reviewed:** `552b9bcce`. The product commit is `9a63c09ce`; `8d34740d4` and `552b9bcce` change only docs/status.
- **Base:** `origin/main` `99a774734` (merge-base confirmed).
- **Commits added during the review:** the branch moved to `736c45aa9` (`7afee7aef` quick-gate status, `736c45aa9` R7 hand-back). Both are docs only: `git diff --name-only 552b9bcce 736c45aa9 -- apps` is empty. This verdict therefore also covers `736c45aa9`.

## Verdict: APPROVE WITH CHANGES (0 BLOCKER, 0 MAJOR, 5 MINOR, 3 NIT)

**Summary.**
- The fix is correct and stays inside its scope: the diff touches only `apps/agent/internal/{desired,agent,subsystems}` and docs.
- The implicit all-interfaces target set (`allInterfaces` lists and the auto-block overlay) now uses the same `SplitUnboundPhysical` rule as the interfaces projection. Released rows are dropped before any lookup. Unbound rows are dropped when a kernel netdev of that name exists with no rtnetlink kind. A nil lookup keeps unbound rows in both projections, so the two stay consistent.
- No VPP message, `binapi/`, startup.conf, lab slot or daemon is involved. The binding descriptor still only adds or removes this owner's ACLs.
- Compared with before the fix, the data plane is strictly safer. The resync, every interfaces/security commit and every auto-block reconcile used to fail whenever an unbound or released row met an all-interfaces list or live auto-block entries. They now succeed and enforce on every interface VPP has.
- The findings are about:
  - doing the netlink decision twice, and doing it again in ACL-only transactions;
  - lab evidence that is not tracked on `main`;
  - follow-up wording that overstates what is left;
  - missing fake-VPP tests for Retrieve and for a NIC that gets bound later.
- I request MINOR 3 and MINOR 4 in this branch (both docs only). The rest may be answered with "won't fix" or a debt row. R4 needs no re-check round.

## Findings

### MINOR 1: two independent netlink splits per projection (inconsistency window and double cost)

**Where:**
- `apps/agent/internal/agent/projection.go:242` (`boundIfs, … := desired.SplitUnboundPhysical(ds.GetInterfaces(), netdev)`, which runs unconditionally);
- `apps/agent/internal/desired/global_blocking.go:108-114` (a second `SplitUnboundPhysical(ds.GetInterfaces(), x.netdev)`), reached through `acl.go:94,107` and `projection.go:411`.

**How it fails.**
- `subsystems.LinuxNetdevKind` (`subsystems/netdev.go:29-61`) does a fresh full RTM_GETLINK dump on every call, once per non-released physical row. The interfaces decision and the ACL decision are therefore two separate reads of kernel state, a few milliseconds apart.
- **(a)** A row's kernel netdev disappears between the two splits (driver unbind, rename). The alias is dropped but the binding is emitted. If VPP does not have the NIC, the plan fails with `CodeDependencyMissing`. This fails closed and is transient.
- **(b)** A kind-"" netdev with that name appears between the two splits. The alias and admin-state are projected but there is no `_gb.*` binding. That NIC forwards without block lists until the next projection. This fails open.
- Both cases need a kernel change inside the projection window, so this is MINOR.

**Fix.**
- Split once in `projectOwned`, which already does it at `:242`. Pass the kept names to `desired.ACL`, for example an `allIfs []string` or a `func() []string`. Test callers can pass nil to mean "every row except released ones".
- This also removes N RTM_GETLINK dumps per projection.
- D-250 rejected this option because "every caller must build it". That argument is weak: there is exactly one production caller (`projection.go:411`), and the test callers already had to be changed to pass a fourth argument.

### MINOR 2: ACL-only transactions re-decide bound/unbound on their own; on a name collision they now strip block lists from a VPP NIC that is still configured

**Where:**
- `apps/agent/internal/agent/rpc_autoblock.go:112`: the auto-block reconcile applies with domains `["acl"]`.
- `service.go:1385-1388`: `acl` is added to interfaces/security transactions, but `interfaces` is never added to ACL-only ones.
- `global_blocking.go:110`.
- `descriptors/acl/binding.go:201-211`: Delete removes this owner's ACLs.
- `scheduler/reconciler.go:504-512`: an observe-only alias satisfies the binding's mandatory dependency.

**How it fails.**
1. Row `eth1` is bound in VPP. The last interfaces projection configured it: alias, admin-up, addresses, `_gb.*` binding.
2. Later, a physical kernel netdev named `eth1` appears. Possible causes: custom udev names, or `net.ifnames=0` handing out the lowest free `ethN` on hot-add or driver re-probe. This is the same collision class as #238 R4 finding #9.
3. The next auto-block entry, expiry or ACL-only commit calls `lookup("eth1")`. It gets exists with kind "", so the row counts as unbound and no binding is desired.
4. Retrieve reports our binding on the VPP NIC, so Delete removes `_gb.*` and `_gb.pass` from it.
5. Its admin-up state and addresses are outside the transaction's scope and stay. The NIC keeps forwarding without block lists until the next interfaces commit or resync. That later transaction then drops its whole configuration, which is the known name-based-decision debt.
- Before the fix, the ACL-only projection kept the binding, because the VPP NIC's observe-only alias satisfied the dependency.
- The appliance's default predictable interface names cannot trigger this (there is no `net.ifnames` override in `deploy/`), so this is MINOR.
- The fix for MINOR 1 does not cover this case: at ACL-only time, both splits would run against the new kernel state.

**Fix.**
- Add this security consequence to the existing debt row "the agent decides 'unbound' by the row's logical name" (2026-10-17, on the #238 branch). Alternatively, add a row on `main` in this PR, since `main` does not carry that row yet.
- The real fix is to decide by `physical.pci` (the PCI function's driver, or a live DPDK interface), as `HostNics` already does. That removes both MINOR 1 and MINOR 2.

### MINOR 3: no DEFERRED-ACCEPTANCE row on `main` for the lab and packet-level evidence

**Problem.**
- The PR changes where a security feature is enforced in the data plane. It is proven only by projection tests and the fake VPP, which the PR states correctly.
- The only matching deferred case is row (e) of `docs/status/DEFERRED-ACCEPTANCE.md` on the unmerged #238 branch. It is still worded "(known agent defect, tech-debt)".
- The owner's rule is that lab-only acceptance is deferred into that file, so it must be tracked on `main`.

**Fix.** Add a row to `docs/status/DEFERRED-ACCEPTANCE.md` in this branch, run on an appliance or a per-slot lab and never on the shared host's real NICs. Evidence it should require:
- **(a) Commit and placement.** Use at least one DPDK-bound NIC, one registered kernel NIC (unbound) and one released row. Committing an `allInterfaces` block list is APPLIED with one `agent.nic-not-bound` and one `agent.nic-released`. `acl_interface_list_dump` / `show acl-plugin interface` shows `_gb.<list>.i00/o00` (+`_gb.pass`) only on the bound NIC and the other VPP interfaces. `Retrieve()` for acl equals desired. A second apply is empty and drift is 0.
- **(b) Auto-block enforcement.** `AutoBlockSet` with one live entry succeeds with `activeEntries` 1. A packet from the blocked source entering the bound NIC is dropped: the `_gb.auto-block.i00` hit counter rises, egress tx does not rise, and tcpdump in the peer netns sees nothing. An unblocked source is forwarded.
- **(c) NIC bound later.** The start-up apply binds the kernel NIC (VPP restart). The connect resync is APPLIED and the NIC carries the auto-block and all-interfaces ACLs. A blocked source entering it is dropped, and `Retrieve()` equals desired.
- **(d) Release.** Release a VPP-bound NIC before the start-up apply. The commit is APPLIED, our `_gb.*` binding is gone from it, it is admin-down, and another owner's ACLs on it are untouched.
- **(e) Agent restart.** Stop and start the agent with an unbound row, a released row and a live entry. The start-up resync is APPLIED with zero changes and the auto-block cache is replayed.
- On the #238 branch, reword row (e) to the target state and point it at #241.

### MINOR 4: the follow-up text overstates what is left ("the user-doc exception paragraph can then be dropped"; replacement debt row says "fixed")

**What remains in the same failure class.**
- An explicit reference to an unbound or released row still emits `acl.interface-binding/<row>`. Such references are:
  - a block list's `interfaces`;
  - an ACL attachment to the interface, or to a zone that contains it;
  - a MACIP attachment.
- That binding fails the plan. The auto-block reconcile projects the whole acl domain (`rpc_autoblock.go:112`), so in that case new automatic blocks are enforced on no interface. "Fails closed" holds for commits, not for auto-block.
- Worse, a reference to a released row is accepted while VPP still has the NIC, because the observe-only alias satisfies the dependency (`reconciler.go:504-512`). It then fails the connect resync once the start-up apply has removed the NIC. The restarted VPP stays unconfigured: degraded, retried with backoff.
- This is pre-existing (code reading), not introduced here.
- The #238 user doc's removed-NIC bullet (`default-dataplane-nics.md:176-178`) carries the same "except through an all-interfaces block list or automatic blocking" parenthetical.

**Fix.** Replace the paragraph rather than dropping it, for example:

> Explicit references (a block list's interfaces, ACL attachments, zones) to an awaiting-engine or released NIC are refused at commit; if one exists when the NIC leaves the engine, commits touching interfaces or security are refused and new automatic blocks are not enforced until the reference is removed. *All interfaces* and automatic blocking skip such NICs.

Also keep a residual debt row: either an API semantic rule that refuses references to `physical.owner: host` rows, or the agent skipping, with a warning, explicit references to rows the projection drops.

### MINOR 5: no fake-VPP proof of Retrieve, idempotence or the "bound later" transition for a user `allInterfaces` list

**Gap.**
- The new end-to-end test covers the runtime overlay, which Retrieve deliberately excludes.
- No test applies a user `allInterfaces: true` list with unbound and released rows on `coretest` and then checks any of:
  - `retrieveACL == desired` (the record fingerprint is built over the narrower parts, `acl.go:656-671` / `809-813`);
  - that a re-apply or `Resync` makes no changes;
  - that a later bind adds the binding.
- These are R4's restart and Retrieve proofs for the changed target set, and they are cheap on the fake VPP.

**Fix** (unit tests on the fake VPP, no host involved):
1. A user `allInterfaces` list plus the `nicsAutoBlockDoc` rows is APPLIED. `Retrieve(acl)` equals desired. A re-apply has summary 0/0/0 and `s.Resync` is APPLIED with no changes.
2. A service whose lookup reports `ens192` as absent, plus `v.AddInterface("ens192","dpdk","")` (bind plus VPP restart), then `Resync`: `ens192` carries `_gb.<list>.i00`.
3. `lan` released while the fake VPP still has it: its `_gb.*` binding is removed, it is admin-down, and a foreign ACL bound to it is untouched.

### NIT 1: test comment does not match the stub
`apps/agent/internal/agent/physical_nics_acl_test.go:25-26` says "lan is bound (VPP owns it)". But `fakeNetdevs` (`service_test.go:38-48`) returns `("veth", true)` for `lan`. So `lan` is kept because it looks like VPP-side plumbing, not because it has no kernel netdev. The production path for a bound NIC (`exists == false`) is exercised only by the stub in the desired-package test. Fix: wrap the lookup so it returns `("", false, nil)` for `lan`.

### NIT 2: imprecise "Out of scope" wording
`docs/status/tasks/agent-allifs-unbound-nics.md` ("Out of scope") and the WIP say a NIC bound later gets its binding at "the next auto-block reconcile". The tick (`rpc_autoblock.go:149-156`) reconciles only when the overlay fingerprint changes or the snapshot is dirty, and a NIC being bound changes neither. The binding actually arrives with the connect resync after the required VPP restart (`agent.go:471`), together with the NIC's own configuration. The conclusion holds; the wording does not.

### NIT 3: invalid test configuration
`apps/agent/internal/desired/global_blocking_test.go` (new test, `physical(...)`) gives all three physical rows the same PCI `0000:04:00.0`. The configuration is invalid but harmless: the agent does not read `pci`.

## Answers to the R4 questions

1. **VPP API provenance.** No change under `apps/agent/binapi/` or to `tools/binapi-gen.sh`, and no new VPP message. The change only alters which `acl.interface-binding` KVs are emitted.
2. **Restart safety, Retrieve and drift.** No new object type was added, so no new Retrieve is needed.
   - Binding Retrieve (`binding.go:217-259`) reports only this owner's tagged ACLs per interface.
   - `setList`/Delete (`binding.go:73-129`, `201-211`) keep foreign ACLs. Reconcile therefore never deletes a binding it does not own.
   - `recordGlobalBlocking` fingerprints exactly the emitted parts. The record keeps the applied set plus up to 8 others per config hash (`expansion.go:179,374-388`), so a different kept set for the same config evicts nothing. Because the parts equal what VPP holds, `AssembleACL` reports the configured list rather than a reconstruction (unit-proof requested in MINOR 5).
   - After an agent restart, the resync re-projects the same kept set: zero changes for kept rows. Unbound rows have nothing in VPP and nothing desired.
   - A released row that VPP still has: before the fix its `_gb.*` binding stayed applied (observe-only alias) and the NIC was admin-down. After the fix the binding is deleted once (our ACLs only) and admin-state Delete keeps it down.
3. **A NIC that becomes bound later.**
   - Binding requires startup.conf plus a VPP restart. The agent has no runtime attach path (no `dev_attach`).
   - On reconnect, `fullResync` (`agent.go:471`) projects all managed domains with a fresh lookup. Alias, admin-state and binding are planned in one transaction.
   - The only remaining window is inside that transaction (see O-1 below). It is pre-existing and the same for every interface.
   - Before the fix, the resync failed whenever any other row was still unbound or released while an all-interfaces list or live entries existed. VPP then stayed unconfigured after the restart and auto-block enforced nothing. After the fix the resync succeeds.
   - The auto-block tick does not react to kernel changes (NIT 2).
4. **Double lookup and inconsistency:** see MINOR 1 (within one projection) and MINOR 2 (ACL-only transactions versus the last interfaces projection).
5. **Nil-lookup path.**
   - It never occurs in production: `subsystems.go:295-297` defaults to `LinuxNetdevKind`, and `agent.go:322` passes `wiring.NetdevKind()`.
   - With nil, both projections keep unbound rows (consistent), and released rows are still dropped. The new desired test covers this.
6. **Released row ("agent.nic-released").** It is dropped before any lookup (`hostnics.go:22-25`) in both projections, with or without a lookup. It never binds through the implicit set; explicit references still bind it (MINOR 4).
7. **Shared host.**
   - The diff touches no VPP, lab slot, startup.conf, handover, daemon or `deploy/`/`tools/`/`test/` file. There is no trace, classify or VPP-global setting.
   - Tests use the in-memory `coretest` VPP and stub lookups (`fakeNetdevs`; `kernelNIC` in the desired test), never `LinuxNetdevKind`. This matters because ngfw-a really has kernel NICs named `ens161` and `ens192` (`docs/lab/host-ngfw-a.md:17`), yet the results do not depend on the host.
8. **Crash vectors.** None added. There are fewer VPP calls than before. Binding removals still run before ACL deletes because of the mandatory dependency (`binding.go:51-65`).
9. **Are the tests enough?** Yes for the projection logic. The data-plane proof must be deferred, with the evidence listed in MINOR 3.
10. **Host traffic on unbound NICs.** Unbound kernel NICs remain covered for traffic to the box by the `protectHost` nftables sets, which ignore interfaces (`renderers/nftables/blocking.go:31-60`). VPP does not forward on them.

## Out-of-scope observations (R4 aspect, pre-existing, not graded; suggest debt rows)

- **O-1: interfaces come up before block lists after a VPP restart.**
  - Topological ties break by descriptor registration rank (`scheduler/reconciler.go:700-706`). `interface.admin-state` is registered at `subsystems.go:326`; the acl descriptors at `:412`.
  - So after any VPP restart, the resync brings interfaces admin-up before `acl.acl` / `acl.interface-binding` are created. For up to 64 bucket ACLs per list and direction, traffic passes without block lists.
  - This applies to every interface and is not changed by this PR.
  - Suggested fix: make an interface's admin-state depend (optionally) on its desired binding, or stage the ACL objects first.
- **O-2: VLAN sub-interfaces are never targeted by "all interfaces".**
  - The all-interfaces set (before and after this PR) is the top-level `interfaces` rows only. VLAN sub-interfaces declared under `subinterfaces` (`desired/interfaces.go:185-205`) are never targeted.
  - VPP's acl plugin filters on the RX `sw_if_index` of the sub-interface. So the auto-block overlay and `allInterfaces` lists do not filter routed sub-interface traffic.
  - `docs/user/security/global-blocking.md:27` ("All interfaces also covers interfaces added later") has no such exception.
  - Basis: code reading plus acl-plugin semantics; not verified on VPP here.

## [other: R5]
`LinuxNetdevKind` dumps every link on each call. `projection.go:242` runs the split on every projection, including ACL-only ones. The ACL projection now repeats it whenever an all-interfaces list or live entries exist. The result is 2N full link dumps per Apply, DryRun, Resync, CheckDrift and auto-block reconcile. That is fine at 18 NICs. The fix for MINOR 1, or a single batch dump per projection, halves it.

## Evidence (commands run in `/root/NGFW/.claude/worktrees/funny-meninsky-c9ef19`, read-only)

```
$ git merge-base origin/main 552b9bcce
99a774734e2cf455f71563ff0dba5afb58051685
$ git diff --name-only 99a774734...552b9bcce
apps/agent/internal/agent/physical_nics_acl_test.go
apps/agent/internal/agent/projection.go
apps/agent/internal/desired/acl.go
apps/agent/internal/desired/acl_test.go
apps/agent/internal/desired/global_blocking.go
apps/agent/internal/desired/global_blocking_test.go
apps/agent/internal/desired/rule_expiry_test.go
apps/agent/internal/subsystems/acl_pbr_test.go
docs/decisions/LOG.md
docs/status/tasks/agent-allifs-unbound-nics-envelope.md
docs/status/tasks/agent-allifs-unbound-nics-wip.md
docs/status/tasks/agent-allifs-unbound-nics.md
$ git diff --name-only 99a774734...552b9bcce -- apps/agent/binapi tools deploy test .github
(empty)
$ git diff 99a774734...552b9bcce -- apps | grep '^+' | grep -nE 'vppctl|cli_inband|trace|systemctl|startup\.conf|exec\.Command|os/exec|LinuxNetdevKind|NetlinkRIB|pkill|killall|/run/vpp|api\.sock'
(no match)
$ grep -rn "desired\.ACL(" --include=*.go .
apps/agent/internal/agent/projection.go:411: desired.ACL(p, ds, in, netdev) …
apps/agent/internal/subsystems/acl_pbr_test.go:74: desired.ACL(sink, … , nil)
$ grep -rn "NewService(" --include=*.go apps/agent | grep -v _test.go
internal/agent/agent.go:322: … NetdevKind: wiring.NetdevKind(), …
$ grep -rn "GetAllInterfaces()" --include=*.go apps/agent/internal | grep -v _test
internal/desired/global_blocking.go:144
$ grep -rn "net.ifnames\|biosdevname" deploy/ tools/ docs/
(no match)

(apps/agent)
$ env -u NGFW_INTEGRATION go test -count=1 -run 'TestGlobalBlocking|TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs|TestAutoBlockWithUnboundAndReleasedNICsOnFake|TestProjectPhysicalNic' -v ./internal/desired/ ./internal/agent/
--- PASS: TestGlobalBlockingProjection (0.01s)
--- PASS: TestGlobalBlockingDriftReconstructs (0.00s)
--- PASS: TestGlobalBlockingAllInterfacesAndDisabled (0.00s)
--- PASS: TestGlobalBlockingBucketsAndScale (4.40s)
--- PASS: TestGlobalBlockingLimits (0.01s)
--- PASS: TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs (0.00s)
ok  	ngfw/agent/internal/desired	4.935s
--- PASS: TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs (0.02s)
--- PASS: TestAutoBlockWithUnboundAndReleasedNICsOnFake (0.07s)
--- PASS: TestProjectPhysicalNicAwaitingDataplane (0.00s)
--- PASS: TestProjectPhysicalNicBoundProjectsNormally (0.00s)
--- PASS: TestProjectPhysicalNicNoLookupProjectsNormally (0.00s)
--- PASS: TestProjectPhysicalNicLcpTapIsNotUnbound (0.00s)
--- PASS: TestProjectPhysicalNicUnboundHasNoLcpPair (0.00s)
--- PASS: TestProjectPhysicalNicReleasedToHost (0.00s)
--- PASS: TestGlobalBlockingOnFake (0.07s)
ok  	ngfw/agent/internal/agent	0.299s
$ env -u NGFW_INTEGRATION go test -race -count=1 -run 'AutoBlock|GlobalBlocking|PhysicalNic|Drift|Resync' ./internal/agent/
ok  	ngfw/agent/internal/agent	15.770s
$ env -u NGFW_INTEGRATION go test -race -count=1 -run 'GlobalBlocking|ACL|SplitUnbound|Physical' ./internal/desired/
ok  	ngfw/agent/internal/desired	26.238s

$ git status --short        → (clean)
$ git rev-parse HEAD        → 736c45aa9e173c597fd07f117307708f6bedb0f0
$ git log --oneline 552b9bcce..736c45aa9
736c45aa9 docs(status): save review R7 hand-back … ; 7afee7aef docs(status): quick gate PASSED …
$ git diff --name-only 552b9bcce 736c45aa9 -- apps      → (empty)
$ gh pr view 241 --repo mcoder1001-cyber/NGFW --json headRefOid,statusCheckRollup
736c45aa9e173c597fd07f117307708f6bedb0f0 · Mandatory quick gate IN_PROGRESS (no result claimed)
```

Everything else above comes from code reading, cited by file:line: the scheduler's observe-only alias and dependency check, topological order, the binding descriptor's Retrieve/Delete, the record's `putSet`, the auto-block tick, the projection wiring and the nftables host sets. No lab, VPP or packet-level evidence was collected, as the task's limits required. What it should contain is listed in MINOR 3.

**Verdict: APPROVE WITH CHANGES (0 BLOCKER, 0 MAJOR, 5 MINOR, 3 NIT).** Requested in this branch: MINOR 3 (DEFERRED-ACCEPTANCE row) and MINOR 4 (follow-up wording and residual debt row). MINOR 1, 2 and 5 and the NITs may be fixed or answered with "won't fix" or a debt row. R4 needs no re-check round.

## Verify round (head `873f1d946`)

- **Reviewer:** panel R4 (Opus 5.5), read-only agent. No files were modified and no temporary files were created. Nothing was run against VPP, a lab slot, systemd or the host.
- **Scope:** verify round only. I re-checked my round-1 findings, the product change since then (`git diff 9a63c09ce 856bf743a -- apps`, commit `856bf743a`) and the docs change (`git diff 20c3303fd 0b57dc070 -- docs`).
- **Head:** `873f1d946`. No file under `apps` changed after `856bf743a`.

### Verdict: APPROVE (0 BLOCKER, 0 MAJOR, 2 MINOR open, 2 NIT open)

The two open MINORs are the leftover of MINOR 4 (the PR description) and new finding N1. Both are optional under the shared rules. I recommend fixing them before merge because they are about wording, not code. The open NITs are N2 and N3.

### Status of the round-1 findings

**MINOR 1 (two separate netlink splits per projection): fixed.**
- `projection.go:242` splits once. `projection.go:411` passes the kept rows (`boundIfs`) to `desired.ACL`, whose new signature is at `acl.go:96`.
- `global_blocking.go` no longer takes a lookup. The `kept == nil` branch at `:114` only runs for test callers: `SplitUnboundPhysical` always returns a non-nil map (`hostnics.go:19`), and `projection.go:411` is the only production caller (grep below).
- `ds` is not reassigned and `boundIfs` is not changed between `:242` and `:411`. So the interfaces domain and the acl domain see the same kernel state, and both inconsistency cases (a) and (b) are closed.
- `TestProjectAutoBlockOverlayAddsNoNetdevLookups` (`physical_nics_acl_test.go:122`) proves the point. Projecting `[interfaces, acl]` makes the same lookups per name as projecting `[interfaces]`: one for each engine-owned row and none for a released row.
- D-251 records the choice.

**MINOR 2 (acl-only transactions re-decide bound/unbound by name): fixed (debt row).**
- `docs/tech-debt.md:226` describes the name-collision scenario, its fail-open effect on a VPP NIC that keeps forwarding, and the real fix (decide by `physical.pci`).
- The row's claim "since #241 the warning `acl.global-blocking` names the row, so the drop is not silent" is true for commits. It is not true for the auto-block reconcile, which is the first trigger the row lists. See N1.

**MINOR 3 (no DEFERRED-ACCEPTANCE entry on `main`): fixed.**
- `docs/status/DEFERRED-ACCEPTANCE.md:386` adds the section "All-interfaces block lists and auto-block with unbound and released NICs — 2026-10-17".
- It carries evidence (a)–(e) as I listed them. It adds the `acl.global-blocking` warning to (a) and keeps "never on the shared host's real NICs".
- The rewording of #238 case (e) is assigned to the #238 session, under the manager's merge order #240 → #242 → #241 → #238. This is recorded in the section's last sentence and in the status report's "Out of scope" list. Given that order, this is the right place for it.

**MINOR 4 (follow-up text overstated what is left): partially fixed.**
- In the branch it is fixed:
  - The status report ("Out of scope", around line 155) says "fail-closed for commits, fail-open for auto-block".
  - It quotes my replacement paragraph for the #238 session and says the paragraph is "replaced (not dropped)".
  - The residual debt row (`docs/tech-debt.md`, second new row) names both fixes: a schema refusal or an agent skip with a warning, and decoupling the host nftables auto-block from VPP binding errors.
- **Not fixed: the PR description on GitHub.**
  - `gh pr view 241` at head `873f1d946` (updatedAt 2026-10-17T12:09:43Z) still contains the sentence this finding was about: "The user-doc exception paragraph can then be dropped."
  - It also still describes the replaced D-250 design ("`desired.ACL` takes the Linux netdev lookup … once per projection, lazily", "Options considered (decision log D-250)"), the placeholder `#@@N@@`, and "`tools/ci.sh quick` on the head: **running**".
  - The status report's review table marks R7 "#7 PR body" as handled, which does not match GitHub.
  - Fix: replace the PR body with the current report text (D-251, the six tests, the gate results, the #238 hand-over with "replaced, not dropped"). [other: R7]

**MINOR 5 (no fake-VPP proof of Retrieve, idempotence or a later bind): fixed.**
- `TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake` (`:219`) covers a user `allInterfaces` list:
  - the list is bound to `lan` and `loop711` only;
  - `retrieveACL` equals the desired ACL;
  - a second apply has summary 0, and `Resync` is APPLIED with 0 results;
  - after a lookup flip plus `v.AddInterface("ens192")` and a `Resync`, `ens192` carries `_gb.bad.i00`;
  - `Retrieve` still equals the desired ACL afterwards.
- `TestAutoBlockWithUnboundAndReleasedNICsOnFake` (`:153`) now also covers:
  - a NIC bound later gets the auto-block ACL and comes up admin-up;
  - a second resync is empty;
  - releasing `lan` while VPP still has it removes our binding, takes it admin-down, and leaves the binding on `ens192` and `loop712`.
- Foreign ACLs untouched (part of my item 3): **this is covered, but not by the test the developer cited.**
  - `TestInterfaceScopePreservesExternalACLWithoutOverlay` proves that an interfaces-scoped transaction does not delete an unreferenced ACL *object*. That is not about another owner's ACLs bound to the released interface.
  - The right proof is `TestInterfaceBindingPreservesForeignACLs` (`descriptors/acl/binding_test.go:117-167`). It checks the exact `acl_interface_set_acl_list` request on Create, Update and Delete: foreign and untagged ACLs keep their order and `n_input`, and Delete leaves `[fIn, untagged, fOut]`.
  - The release path is the scheduler deleting `acl.interface-binding/lan`. That Delete is `setList` with an empty list (`binding.go:201-211`), so the descriptor test covers exactly the operation that runs.
  - I judge this enough for unit level. The composed check stays in deferred case (d).
  - I did not run the descriptor test myself, because `./internal/descriptors/acl/` is outside the allowed commands. The developer's `go test -race ./...` covered it (143 packages ok, pasted in the status report).

**NIT 1: fixed.** `nicLookup` (`physical_nics_acl_test.go:42-52`) returns `("", false, nil)` for `lan`, `ens192` and `ens161` unless they are listed as kernel NICs. The comment matches the stub.

**NIT 2: fixed.** The status report says the binding arrives "with the connect resync … not at an auto-block tick". The phrase "next auto-block reconcile" now appears only in my own round-1 text.

**NIT 3: fixed.** Each row has its own PCI in both the desired-package test and the agent test (`0000:04:00.0`, `0000:0b:00.0`, `0000:13:00.0`).

**O-1 and O-2: debt rows added** (`docs/tech-debt.md`, last row and third new row). Both are pre-existing and fine as rows.

### What I was asked to judge

**How released rows are labelled (`global_blocking.go:117-127`): it cannot mislabel anything.**
- A row missing from `kept` is labelled "released" when `physical.owner == "host"`, otherwise "not bound".
- `SplitUnboundPhysical` (`hostnics.go:20-36`) drops a row only when `KindExisting && physical != nil`, and then either because `owner == "host"` (released) or because the lookup reports a kernel netdev with no rtnetlink kind (unbound). The released test is the same `owner == "host"` predicate.
- The only production caller builds `kept` from the same `ds.GetInterfaces()` (`projection.go:242` → `:411`), and that map is not changed in between. So every missing row is exactly one of the two classes.
- A non-physical row, a row whose lookup failed and a row with no lookup are all kept, so they are never labelled.
- The contract is implicit: `kept` must be a `SplitUnboundPhysical` result over `ds.GetInterfaces()`. The `ACL` doc comment states it, and there is one caller. That is acceptable.

**The warning text from a data-plane operator's point of view.**
- The text is accurate and useful: "block list %q selects all interfaces; the physical NICs the data plane does not have are skipped (not bound to the data plane yet: …; released to the host: …)".
- Disabled and empty lists exit before the warning (`:135-143`), which is correct.
- Problems with where it is surfaced, its severity and its anchor: N1, N2 and N3 below.

**Shared host and provenance: unchanged.**
- No file under `apps/agent/binapi`, `tools`, `deploy`, `test` or `.github` changed.
- No added line matches the trace, vppctl, exec, systemctl, startup.conf, pkill or killall patterns.
- The tests use only the in-memory `coretest` VPP and stub lookups (`nicLookup` and `kernelNIC`), never `LinuxNetdevKind`.
- No new VPP message or object type. Retrieve is unchanged and still covered.

### New findings

**N1 (MINOR): the new warning is never shown during an auto-block reconcile; the debt row and the code comment claim it is.**
- **Where:**
  - `apps/agent/internal/agent/rpc_autoblock.go:112-116`;
  - `apps/agent/internal/agent/service.go:660`;
  - `apps/agent/internal/desired/global_blocking.go:106-108`;
  - `docs/tech-debt.md:226`.
- **What happens:**
  - `reconcileAutoBlockLocked` reads only `resp.GetStatus()`. The `Validation` report that carries `acl.global-blocking` is thrown away.
  - The "reconcile done" log logs status, summary and message, not the issues.
  - `AutoBlockSetResponse` returns only `ActiveEntries`.
- **Scenario:** the MINOR 2 case. A kind-less kernel netdev appears with the name of a NIC that VPP owns. The next auto-block entry or expiry runs an acl-only runtime reconcile, which removes `_gb.*` and `_gb.auto-block.*` from a NIC that keeps forwarding. No operator-visible warning, log line or RPC field names the row. It appears only at the next commit's DryRun or Apply report.
  - The debt row's sentence "since #241 the warning … names the row, so the drop is not silent" overstates this for the auto-block reconcile, which is the first trigger the same row lists.
  - The code comment "an acl-only transaction (auto-block reconcile, security commit) … the drop must never be silent" implies coverage the auto-block path lacks.
- **Fix, either:**
  - (a) In `reconcileAutoBlockLocked`, log at Warn the projection's `acl.global-blocking` issues whenever their text differs from the last reconcile. The reconcile already runs only when the fingerprint changes or the snapshot is dirty, so this adds little noise.
  - (b) Reword the debt row and the comment to "named in commit/DryRun reports; the auto-block runtime reconcile discards its report". [other: R2] (R2 #1 asked for "not silent".)

**N2 (NIT): released NICs are reported at WARNING severity on every commit, forever.**
- **Where:** `global_blocking.go:163-171`, compared with `projection.go:250`.
- **Why:** releasing a NIC is a deliberate, permanent state. It is also the documented way to handle a built-in NIC that was physically removed (`docs/user/interfaces/default-dataplane-nics.md:107-110`). The interfaces domain therefore reports it as INFO (`agent.nic-released`).
- **Effect:** the new warning puts a WARNING naming the released NIC on every commit that projects acl while an all-interfaces list or live auto-block entries exist, once per all-interfaces list. That teaches operators to ignore a warning whose other half (an unbound NIC: configuration not effective yet) matters.
- **Fix:** report the released part through the existing `infof` helper (`desired/cnat.go:43`), and keep `Warnf` for the unbound part only.

**N3 (NIT): the pointer and name used for the auto-block overlay.**
- **Where:** `global_blocking.go:171`; `physical_nics_acl_test.go:114`.
- **Why:** `/acl/globalBlocking/lists/auto-block/interfaces` points into a list name that can never exist in the user's document (reserved and refused at `autoblock/runtime.go:86-87`). The message also says "block list "auto-block"", but the operator configured *automatic blocking* at `/security/autoBlock`. A UI cannot attach this warning to a field.
- **Acceptable as is:** it follows the existing "selects no interface" warning, which uses the same `lp`.
- **Optional fix:** for `name == autoblock.ListName`, use the pointer `/security/autoBlock` and the wording "automatic blocking". For user lists, `/allInterfaces` would name the cause more exactly than `/interfaces`.

**Other (not graded): [other: R7/R1]**
- `gh pr view 241` shows `mergeable: CONFLICTING` (`mergeStateStatus: DIRTY`).
- Commit `873f1d946` has 0 check runs, so the hosted quick gate has not run on this head.
- The WIP already plans to rebase after #242 merges and to require a green hosted gate on the final head. The merge needs that.

### Commands run (in `/root/NGFW/.claude/worktrees/funny-meninsky-c9ef19`, read-only)

```
$ git rev-parse HEAD                      → 873f1d946a0c8e65f08603db4891025560bcaa66
$ git merge-base origin/main 873f1d946    → 99a774734e2cf455f71563ff0dba5afb58051685
$ git log --oneline 99a774734..873f1d946
873f1d946 docs(status): report and WIP after the review fixes and the gates on 0b57dc070 (PR #241)
0b57dc070 docs: D-251, deferred acceptance, tech-debt rows and the user-doc sentence for PR #241
856bf743a fix(agent): acl block lists reuse the projection's kept rows and name the NICs they skip
20c3303fd docs(status): save review R4 hand-back … ; c8f3a608a … R1 and R2 … ; 736c45aa9 … R7 … ; 7afee7aef … ; 552b9bcce … ; 8d34740d4 … ; 9a63c09ce fix(agent): all-interfaces block lists target only the rows the projection keeps
$ git diff --stat 9a63c09ce 856bf743a -- apps
 physical_nics_acl_test.go 205 ++-- · projection.go 6 · acl.go 14 · global_blocking.go 43 · global_blocking_test.go 76   (5 files, +266 −78)
$ git diff --name-only 856bf743a 873f1d946 -- apps                              → (empty)
$ git diff --name-only 99a774734...873f1d946 -- apps/agent/binapi tools deploy test .github   → (empty)
$ git diff 99a774734...873f1d946 -- apps | grep '^+' | grep -nE 'vppctl|cli_inband|trace|systemctl|startup\.conf|exec\.Command|os/exec|LinuxNetdevKind|NetlinkRIB|pkill|killall|/run/vpp|api\.sock'   → (no match)
$ grep -rn "desired\.ACL(" --include=*.go apps/agent
apps/agent/internal/agent/projection.go:411:		desired.ACL(p, ds, in, boundIfs) …
apps/agent/internal/subsystems/acl_pbr_test.go:74:	desired.ACL(sink, … nil)
$ grep -n "netdev" apps/agent/internal/desired/acl.go apps/agent/internal/desired/global_blocking.go
apps/agent/internal/desired/acl.go:94:// split without a netdev lookup, …            (comment only; no lookup left in the acl projection)

(apps/agent)
$ env -u NGFW_INTEGRATION go test -count=1 -run 'TestGlobalBlocking|TestProjectAutoBlockOverlay|TestAutoBlockWithUnboundAndReleasedNICsOnFake|TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake|TestProjectPhysicalNic|TestInterfaceScope' -v ./internal/desired/ ./internal/agent/ | grep -E '^(--- |ok|FAIL|PASS|panic)'
--- PASS: TestGlobalBlockingProjection (0.01s)
--- PASS: TestGlobalBlockingDriftReconstructs (0.00s)
--- PASS: TestGlobalBlockingAllInterfacesAndDisabled (0.01s)
--- PASS: TestGlobalBlockingBucketsAndScale (4.54s)
--- PASS: TestGlobalBlockingLimits (0.01s)
--- PASS: TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs (0.00s)
--- PASS: TestGlobalBlockingAllInterfacesNothingBound (0.00s)
PASS
ok  	ngfw/agent/internal/desired	5.006s
--- PASS: TestInterfaceScopePreservesExternalACLWithoutOverlay (0.04s)
--- PASS: TestInterfaceScopeReprojectsActiveOverlayAndSecurityRemoval (0.05s)
--- PASS: TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs (0.00s)
--- PASS: TestProjectAutoBlockOverlayAddsNoNetdevLookups (0.00s)
--- PASS: TestAutoBlockWithUnboundAndReleasedNICsOnFake (0.08s)
--- PASS: TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake (0.05s)
--- PASS: TestProjectPhysicalNicAwaitingDataplane (0.00s)
--- PASS: TestProjectPhysicalNicBoundProjectsNormally (0.00s)
--- PASS: TestProjectPhysicalNicNoLookupProjectsNormally (0.00s)
--- PASS: TestProjectPhysicalNicLcpTapIsNotUnbound (0.00s)
--- PASS: TestProjectPhysicalNicUnboundHasNoLcpPair (0.00s)
--- PASS: TestProjectPhysicalNicReleasedToHost (0.00s)
--- PASS: TestGlobalBlockingOnFake (0.06s)
PASS
ok  	ngfw/agent/internal/agent	0.413s
$ env -u NGFW_INTEGRATION go test -race -count=1 -run 'AutoBlock|GlobalBlocking|PhysicalNic|AllInterfaces|Drift|Resync|InterfaceScope' ./internal/agent/
ok  	ngfw/agent/internal/agent	16.358s
$ env -u NGFW_INTEGRATION go test -race -count=1 -run 'GlobalBlocking|ACL|SplitUnbound|Physical|RuleExpiry|Expir' ./internal/desired/
ok  	ngfw/agent/internal/desired	25.887s

$ gh pr view 241 --repo mcoder1001-cyber/NGFW --json updatedAt,headRefOid,mergeStateStatus,statusCheckRollup
2026-10-17T12:09:43Z · 873f1d946a0c8e65f08603db4891025560bcaa66 · DIRTY · statusCheckRollup length 0
$ gh pr view 241 --repo mcoder1001-cyber/NGFW --json body   (excerpts, verbatim)
"- `desired.ACL` takes the Linux netdev lookup (`NetdevKind`, the same one `desired.Interfaces` gets) and `globalBlocking` computes the all-interfaces set … once per projection, lazily …"
"- Options considered (decision log D-250): …"
"- `tools/ci.sh quick` on the head: **running** — result to follow in this description."
"… fixed by PR #@@N@@ … The user-doc exception paragraph can then be dropped."
$ gh api repos/mcoder1001-cyber/NGFW/commits/873f1d946a0c8e65f08603db4891025560bcaa66/check-runs --jq .total_count   → 0
$ git status --short   → (clean)
```

Everything else above comes from code reading, cited by file:line:
- the split and its single caller;
- the released predicate;
- the auto-block reconcile discarding its report;
- the "reconcile done" log fields;
- the binding descriptor's Delete and `setList`, plus `TestInterfaceBindingPreservesForeignACLs`;
- `infof` and `InfoSink`;
- the overlay's reserved list name;
- the API's `COVERAGE_RULES`. `acl.global-blocking` is not in it, so the new warning hides no drift.

No lab, VPP or packet-level evidence was collected. What it needs to contain is in `docs/status/DEFERRED-ACCEPTANCE.md:386`.

**Verdict: APPROVE (0 BLOCKER, 0 MAJOR, 2 MINOR open, 2 NIT open).**
- Fixed: MINOR 1, 2, 3 and 5 and NITs 1–3.
- Partially fixed: MINOR 4 (the PR description on GitHub still says "can then be dropped" and describes D-250).
- New: N1 (MINOR), N2 (NIT), N3 (NIT).
- Recommended before merge, optional under the rules: update the PR description, and either log the warning in the auto-block reconcile or reword the debt row and the comment (N1).
- R4 needs no further round.
