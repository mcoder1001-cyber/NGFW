# Independent punt lifecycle fix verification, round 2

Frozen `00cb2cd33fde9a60ba1cf5ccd8b384947b72bbd1`, remote `77b32a41902fa2108caf5e3bca6730fe4987c7d6`. Original missing-error-path MAJOR at 22e8cf55 preserved. Scope R1/R7 lifecycle/test correctness and R3 public contract scope; independent parallel reviewer owns product activation/layout security and dataplane compatibility. No product edits by reviewer.

Original MAJOR resolved by actual renderer+descriptor+scheduler flows:

- Pair-delete failure occurs after actual admission revocation and scheduler journaling. Rollback recreates admission; final fake kernel membership and retained pair state plus exact delete/add log are asserted.
- Actual adapter unknown admission deletion applies the delete then loses reply/readback/compensation; result is DEGRADED/Uncertain, owned pair remains, no pair-delete call occurs.
- Actual adapter unknown creation returns PartialCreate; scheduler deletes owned uncertain admission before rolling back the created pair. Exact call log and empty final fake kernel/pair state asserted; uncertainty remains DEGRADED instead of falsely claiming clean rollback.
- Mixed operation failure restores original pair before original admission in reverse journal order. Failed ordinary admission creation removes the created pair. Restart/resync repairs lost membership from actual readback.

Existing success tests retain exact mixed revokeA/deletePairA/createPairC/admitC order, host rename and same-host type recreation; orphan kernel membership is still exposed and deleted. Projection tests establish disabled-agent no-op, explicit owner gate, ambiguous namespace rejection and final desired namespace selection instead of stale observed namespace. Tests invoke production control flow and mutate fake kernel/pair state; fixture error injection is at actual runner/pair operations, not bypassed descriptor methods.

R1/R7/R3 APPROVE reviewed lifecycle checkpoint. No schema/protobuf/generated/public API contract change from foundation, no assertion/CI weakening. Separate dynamic set layout and registration source are newly integrated and still require the other reviewer's approval and full hosted gate. No whole P10 completion or real nft/VPP acceptance claim.

Actual independent commands:

```text
go test -race -count=1 ./internal/renderers/basepolicy ./internal/scheduler ./internal/subsystems -run 'Test.*(Admission|Membership|PairDelete|MixedFailure|RestartResync|BasePolicy|Projection|Uncertain)'
ok ngfw/agent/internal/renderers/basepolicy 1.045s
ok ngfw/agent/internal/scheduler 1.027s
ok ngfw/agent/internal/subsystems 1.085s
go test -race -count=1 ./internal/renderers/basepolicy
ok ngfw/agent/internal/renderers/basepolicy 1.059s
git diff --check
exit 0
```

17 basepolicy test functions inspected and whole package race-tested. Real nft/VPP, restart/replay under installed sandbox and final unchanged hosted quick remain pending; executed unit tests are not a lab substitute. Existing security/license decisions remain outside this lifecycle approval.
