# FRR lock cleanup R1 correctness and tests

Reviewed source: `88d97a9ca9914d673499b275f7c19752ffe648fe`; no uncommitted product-source changes at final inspection.

No correctness finding in the scoped changes. Returning the acquired harness after failed base reset allows the existing registered cleanup to close the slot lock. The extracted root-harness acquisition closes its descriptor before returning a flock failure, preserving the prior nonblocking acquisition semantics.

Both regressions exercise real filesystem/flock behavior. The reset test deliberately causes `RemoveAll` to fail, asserts the exact cleanup handle survives, calls `Stop`, and verifies a contender can acquire the slot. The contention regression repeats failed acquisitions, counts descriptors referring to the lock file, then verifies release and reacquisition. Neither needs daemon execution. The additional holder-preservation regression verifies real contention followed by cleanup without an acquired lock preserves the holder sentinel and symlink. The final Stop guard also makes a second cleanup non-destructive after lock release.

Independent `git diff --check`: PASS (exit 0, no output). Independent Go race-test execution and the complete quick gate remain pending; this review does not claim either passed.

Source-only R1 verdict: APPROVE at the recorded source commit; no source defect identified. Merge eligibility remains BLOCKED pending independent regression execution and the unchanged complete quick gate. Source approval does not replace execution evidence.

## Narrow CI fixture correction verification

Final integration source reviewed independently: local `54a1187e771623b2310a69102ce74bdf165a9c7e`, tree `a9795785d8f30fa31c3c85af6ce6f2303c2147fc`; published matching PR209 head `9e4c78fe29b716cbe8a81d1205e1de759f501d81` on parent `417e8fcd0c82fae7906da0fba1c0622a8de017f7` (manager publication receipt).

The narrow change anchors temporary lock/sentinel operations with `os.OpenRoot` and fixed relative `slot.lock`/`holder.pid` names. Owner and contender still open the same lock inode. Cleanup ordering closes the lock before its root handle; temporary directories are test-owned. The sentinel read remains a real filesystem read and still fails if holder cleanup wrongly removes the file. Failed-base reset, contention/reacquisition and marker/symlink preservation assertions are unchanged. No product control flow, guard, deadline, skip or assertion was weakened.

Required test functions remain `TestFailedBaseResetRetainsSlotCleanup`, `TestRootFRRContendedLockClosesDescriptors`, and `TestRootFRRCleanupWithoutSlotPreservesHolderFiles`. They contain no integration skip. The unchanged quick script invokes agent `make lint test build`, whose test recipe is `go test -race -count=1 ./...`; this includes both affected packages and all three regressions. Inclusion is source evidence, not an execution PASS.

Independent `git diff --check`: PASS (exit 0, no output). Historical run `37807567473` failed with three G304 fixture lint findings and remains failed evidence. Replacement run `37812533974` is pending; no successful compilation, race test or complete quick result is claimed by this review.

Narrow source-only R1 verdict: APPROVE at the final integration tree. Required actual race/regression and complete unchanged quick success remain prerequisites for merge.

## Independent focused race regression execution

Frozen code remained local `54a1187e771623b2310a69102ce74bdf165a9c7e`, tree `a9795785d8f30fa31c3c85af6ce6f2303c2147fc` before and after execution. The following focused command was independently executed with Go 1.26.0, `GOTOOLCHAIN=local`, `GOMAXPROCS=2` and `GOFLAGS='-p=2 -mod=readonly'`:

```sh
go -C apps/agent test -race -count=1 -v -run '^(TestFailedBaseResetRetainsSlotCleanup|TestRootFRRContendedLockClosesDescriptors|TestRootFRRCleanupWithoutSlotPreservesHolderFiles)$' ./internal/renderers/frr/frrtest ./internal/agent
```

Actual output, exit 0:

```text
=== RUN   TestFailedBaseResetRetainsSlotCleanup
--- PASS: TestFailedBaseResetRetainsSlotCleanup (0.00s)
PASS
ok  ngfw/agent/internal/renderers/frr/frrtest 1.022s
=== RUN   TestRootFRRContendedLockClosesDescriptors
--- PASS: TestRootFRRContendedLockClosesDescriptors (0.00s)
=== RUN   TestRootFRRCleanupWithoutSlotPreservesHolderFiles
--- PASS: TestRootFRRCleanupWithoutSlotPreservesHolderFiles (0.00s)
PASS
ok  ngfw/agent/internal/agent 1.059s
```

All three selected offline regressions executed and passed under the race detector, with no skips. Independent post-run `git diff --check`: PASS; no product-source or module-file edits. This is focused regression evidence, not full CI or native FRR acceptance. The unchanged complete final gate remains required.

R1 verdict: APPROVE scoped source and focused race regressions on the exact recorded tree. Full final gate remains a separate merge prerequisite.
