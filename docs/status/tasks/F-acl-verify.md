# Verify — F-acl fix round 1 (task/F-acl @ 90a20193; code 122306d4)

Reviewer, 2026-09-25. This is a focused verify of my review (d58b9105): H1, M1/V7, M2, M3, L2, L5, L6, L8, L10 and L13,
plus a regression check. M4, Q3 and Q14 wait for the bases; Q2, Q11 and Q12 are the manager's. No host run.

## Verdict: **APPROVE**

H1 and M3 are fixed. Each has a test that fails on the old product code (re-run below) and passes on the fix. The docs,
the V7 row and the host-test hygiene now match shared-host rules §7. Nothing regressed. The five notes at the end are
Low, and none of them blocks the merge.

## What I ran

| check | result |
|---|---|
| `cd apps/agent && go test -race -count=1 ./...` on 90a20193 | **92 packages ok**, 0 failures (30 s wall) |
| Rebase simulation: `git merge-tree --write-tree main 90a20193`, exported to a scratch copy. Then `go test -race` of `subsystems agent actions/acl desired descriptors/acl`, with a scratch-only `RecordsNoOwnership()` on F-object-model's objects descriptor (the objects gap is outside F-acl, see review merge note 1) | all ok. TD-11b's guard on main accepts `acl.config` and the other six acl descriptors |
| My original probe P1, re-run on the fix, extended with the steps below | **passes**. Retrieve == A after every DryRun, and AclState keeps the counters on 10/20. After the Apply of B, Retrieve == B and the counters move to 100/200 without an `acl_add_replace` |
| New probe P3: plain agent restart with VPP intact (new Service on the same state dir, then Resync) | resync `created:2 unchanged:5`. The only creates are `acl.config/attachments` and `acl.config/list/l`, with nothing written to VPP. Retrieve == applied, the counter mapping is known again, and the ACL index is unchanged |
| The worker's two new agent tests against the **old** product code (d58b9105 tree plus the new tests and the fake's new helpers) | both **FAIL**: `rpc_acl_test.go:438 … after a DryRun of the candidate: Retrieve != the APPLIED configuration (false drift)` and `:507 … a binding to a missing interface must be refused at planning: … ok:true`. On 90a20193 both **PASS** |
| `go -C test/topology/acl vet ./...`; the fix-round CI logs (`logs/ci/F-acl-20260925-100259-760609`) | vet ok. CI: agent 92 ok, `golangci-lint` 0 issues, topology/acl unit mode ok, `CI GATE PASSED` |
| `apps/api`, `apps/web`, `packages/*` | not touched by the fix round (diffstat). My earlier web (38) and API (6) runs still apply |
| worktree after my runs | `git status` clean, no `apps/agent/bin` |

Extensions to probe P1 (Apply A, then DryRuns of B, C and B again):
- B has the same VPP content as A with renumbered rules and a different attachment sequence.
- C has **different** VPP content.
- After the DryRuns, a real Apply of B follows.

## Findings → status

| id | status | evidence |
|---|---|---|
| **H1** | **fixed** | See "H1 in detail" below the table. The regression test (`TestACLDryRunAndRollbackDoNotChangeTheAppliedView`) covers lists and attachments, DryRun, a rolled-back Apply and the hand edit (L10) |
| **M3** | **fixed** | See "M3 in detail" below the table |
| **M1 / V7** | **fixed** | See "M1 / V7 in detail" below the table |
| **M2** | **fixed (F-acl's part)** | See "M2 in detail" below the table |
| L2 | fixed | `actions/acl/runtime.go` `readCounters`: one `DumpStats(^/acl/(i|j|…)/matches$)` for all tracked lists, with the per-worker sums as before |
| L5 | fixed | `subsystems/acl.go:43` `aclResyncGap = 30 * time.Second`; the doc says "≥ 30 s apart, D-132" |
| L6 | fixed (docs) | acl.md: "a change of the time zone reaches them after the agent restarts" |
| L8 | fixed | Janitor: one fresh `acl_dump` / `macip_acl_dump` after the unbinds, and an index is deleted only if its tag is unchanged. The MACIP pass uses the same `w3:` / `w3-` predicate as the ACL pass |
| L10 | fixed | the hand-edit step of the H1 test |
| L13 | fixed | `F-acl-wip.md` is current (fix round 1, pending items) |

### H1 in detail
- **New descriptor `acl.config`** (`actions/acl/config.go`). It is agent-local: one object per list, one per MACIP list and one for the attachments.
- **When it changes:**
  - It changes only in a transaction's Create/Update/Delete; the scheduler reverts it on rollback.
  - It lives in memory and the resync rebuilds it (probe P3).
- **Record keys:** entries are keyed by name + VPP fingerprint + configuration hash. The applied configuration's entries are pinned.
- **Readers:** `AssembleACL`, `AclState` and the watcher look up the entry of the **applied** configuration (`desired/acl.go` AssembleACL; `runtime.go` `AppliedExpansion`; `subsystems/acl.go:191,222`).
- **Not a D-063 echo:**
  - `acl.config` is never reported as VPP state.
  - `AssembleACL` returns the applied list only when VPP's actual rules have the fingerprint that this configuration's recorded projection produced. Otherwise it reconstructs from VPP.
  - Its Retrieve returns its own applied record, like the objects family and D-073b. It is used only as the attribution key.
- **TD-11b:** it declares `RecordsNoOwnership` (it owns no VPP object) and passes main's guard.

### M3 in detail
- `descriptors/acl/register.go` adds `boundInterfaceDependency`, which is mandatory, for `acl.interface-binding` and `acl.macip-interface-binding`. The ethertype whitelist stays optional.
- `TestACLBindingAndInterfaceRemovedInOneCommit` checks that:
  - the binding is deleted before `interface.loopback/loop702`;
  - nothing stays bound to the freed index;
  - `loop799` (in neither the configuration nor VPP) → `agent.dependency-missing` at `/acl/attachments/0`, at **planning** time.
- **Scope:** the `interface/<name>` alias resolves for configured interfaces and, through the alias Retrieve, for untagged VPP interfaces (physical/DPDK NICs, rig host-interfaces). So product NICs are unaffected.

### M1 / V7 in detail
- **`docs/user/firewall/acl.md`** says:
  - hits count "since the list last changed in VPP", because VPP clears them on every `acl_add_replace`, including the watcher's re-projections and VPP restarts;
  - edits to descriptions, tags, sequences or attachments keep the counters (correct: `acl.acl` is not updated);
  - `enable=false` exists;
  - the flag is off after every VPP restart;
  - there is a per-packet cost.
- **`docs/agent/descriptors/acl.md`**: the same corrections, plus the `acl.config` and 30 s notes.
- **V7 row:** "no API getter; `enable=false` does switch it off; 0 after every VPP restart; CLI `show acl-plugin tables mask` prints it; host tests save and restore under `flock -x` (§7)". All match `acl.c:1824`, `:589` and `:3646`.

### M2 in detail
`countersScope` (`test/topology/acl/aclvpp_test.go`) implements every §7 step:
- **Change path** (opt-in `VRX_ACL_STATS_GLOBALS=1`): `flock -x`, save the current value (read with the CLI), switch on only if it was off, then `flock -s` while the test relies on it.
- **Cleanup:** `flock -x`, restore **exactly** the saved value (never the VPP default), then unlock.
- **Read-only path:** `flock -s` for the whole test.
- The screenshot run uses the same helper.
- DF-4's `integration_test.go:191-198`, the host-doc row and the resting value remain the manager's (review §2).

## Notes (Low; none blocks the merge)

| id | where | note |
|---|---|---|
| V1 | `actions/acl/expansion.go` `PutACL` / `trimLocked` | See "V1 in detail" below the table |
| V2 | `test/topology/acl/aclvpp_test.go` `countersScope` | Linux `flock` changes EX→SH non-atomically: it releases, then re-acquires. Another slot could change the flag in between. Re-read the flag after taking `-s` (one line). This is a test-only edge |
| V3 | `acl.config` in summaries | Every start-up / reconnect resync now reports `created:N` for the `acl.config` objects (probe P3: `created:2`, with no VPP write). The same keys appear in commit results and DryRun plans. This is cosmetic, but readers of RECONCILE_DONE summaries and the restart evidence should know it. A later option is to leave `acl.config` out of the counts |
| V4 | `descriptors/acl/register.go` `boundInterfaceDependency` | The alias Retrieve never reports another owner's interface, so on the shared host a binding onto another slot's interface is now refused at planning; the old `indexShared` path allowed it. This is intended, and the product has one owner |
| V5 | `docs/vpp-code-track.md` | A shared file outside the envelope, edited at the manager's request (listed under "Shared hunks"). The merger should keep the new V7 row if main changes that table |

### V1 in detail
- An Apply's projection records the new configuration's entry **before** `acl.config` pins it. The pin happens in the same transaction, but DryRuns do not take the transaction lock.
- So the entry can be evicted before it is pinned in either of two ways:
  - more than 4 concurrent DryRuns of other configurations of the same list fill the non-pinned slots;
  - at 100k rules, the rule budget trims it.
- Either way, Retrieve reconstructs, so the drift is false, until the next projection of the applied configuration (the next commit or resync).
- This is unlikely: it needs parallel validates during a commit.
- Fix options: mark the in-flight Apply's entry as protected, or let `acl.config`'s Create/Update re-put the entry from a staging slot.

## Open items (unchanged, owners as agreed)
- **At the rebase** (worker, reviewed): M4 (TD-23 `RegisterExtension`, PBR test through the agent), Q3 stand-in removal, the Q14 fold with F-host-acl-nftables.
- **Before F-acl can pass its gate on main:** F-object-model's objects descriptors must declare their TD-11b ownership.
- **Manager:** M5/Q2 gRPC limits (gates the 100k host step), M6/Q11 datastore row, Q12 after WEB-1, DF-4 test §7 fix + host-doc row, L1/L3/L4/L7/L9/L11/L12 tech debt.
- **After TD-25:** the screenshots and a host run of `countersScope` (the first real exercise of the save/restore path).
