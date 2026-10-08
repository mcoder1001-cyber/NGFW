# FRR lock cleanup R1 correctness and tests

Reviewed source: `88d97a9ca9914d673499b275f7c19752ffe648fe`; no uncommitted product-source changes at final inspection.

No correctness finding in the scoped changes. Returning the acquired harness after failed base reset allows the existing registered cleanup to close the slot lock. The extracted root-harness acquisition closes its descriptor before returning a flock failure, preserving the prior nonblocking acquisition semantics.

Both regressions exercise real filesystem/flock behavior. The reset test deliberately causes `RemoveAll` to fail, asserts the exact cleanup handle survives, calls `Stop`, and verifies a contender can acquire the slot. The contention regression repeats failed acquisitions, counts descriptors referring to the lock file, then verifies release and reacquisition. Neither needs daemon execution. The additional holder-preservation regression verifies real contention followed by cleanup without an acquired lock preserves the holder sentinel and symlink. The final Stop guard also makes a second cleanup non-destructive after lock release.

Independent `git diff --check`: PASS (exit 0, no output). Independent Go race-test execution and the complete quick gate remain pending; this review does not claim either passed.

Verdict: BLOCK pending independent execution of the two added regression tests and the unchanged complete quick gate. No source defect identified.
