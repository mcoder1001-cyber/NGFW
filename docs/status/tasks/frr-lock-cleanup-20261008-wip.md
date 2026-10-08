# FRR slot lock cleanup checkpoint

Branch: `codex/frr-lock-cleanup-20261008`.
Base: `4d4723f78ab053d005015b2a7e41ba840fec06c5`.
Owned paths are declared in the envelope.

Implemented: failed base removal returns the acquired harness for cleanup; failed root-harness flock closes its opened descriptor. Added offline regressions for failed startup cleanup/reacquisition and repeated contention without descriptor growth.

Actual validation: `git diff --check` passed; `tools/ci.sh check --base main` passed. Go compilation, race tests and complete quick results are pending. No source completion or native acceptance is claimed before those results.

Next command: `go -C apps/agent test -race -count=1 -run 'TestFailedBaseResetRetainsSlotCleanup|TestRootFRRContendedLockClosesDescriptors' ./internal/renderers/frr/frrtest ./internal/agent`; then run the unchanged complete quick gate and obtain independent review. Publication and exact checkpoint SHA will be recorded after success.
