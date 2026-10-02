# Independent scheduler uncertain-outcome boundary review

Frozen source `05a5dd821e35f4568d0a1dafcb90ce9b49b85a89`. Scope only this commit's three scheduler files; pure punt adapter preceding it remains unregistered and is not approved as working P10 integration by this report. Independent R1/R2/R4 verdict: APPROVE narrow typed uncertainty boundary.

ErrUncertainOutcome is additive and recognized with errors.Is, including normal wrapping/errors.Join while preserving the original cause. Existing panic/cancellation/deadline and text compatibility paths remain intact. No planner, ordering, journal, compensation, protobuf/API or privilege contract changes. Existing operation failure stops subsequent operations, propagates uncertainty, and reports DEGRADED rather than falsely claiming complete rollback/APPLIED. Failed external deletion is not added to the successful deletion journal; dependence ordering retains the pair when admission outcome is unknown, as intended by the design. Ordinary failure is not automatically marked uncertain.

Actual independent commands:

```text
go test -race -count=1 ./internal/scheduler -run 'TestTypedUncertainOutcome|TestUncertainMarker|TestCommittedUnknownDelete'
ok ngfw/agent/internal/scheduler 1.022s
go test -race -count=1 ./internal/scheduler
ok ngfw/agent/internal/scheduler 2.237s
git diff --check
exit 0
```

New regression includes an actual mutation of fixture descriptor state before returning a joined non-timeout uncertain error. It checks degraded/uncertain result, actual admission absence, pair retained and exact single operation call log. Separate fixture checks errors.Is original cause preservation and wrapped marker vs ordinary rejection. These are real scheduler/descriptor unit flows, not nft/VPP integration proof.

No secret material, user-input shell, additional privilege, suppressed error or lowered test/CI requirement introduced. Descriptors must continue joining/wrapping their real cause with marker; future consumer must independently verify mutation/readback/compensation before claiming certainty. Real punt admission runtime integration, full hosted quick and whole P10 acceptance remain pending, not waived by this narrow approval.
