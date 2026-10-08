# FRR slot lock cleanup checkpoint

Branch: `codex/frr-lock-cleanup-20261008`.
Base: `4d4723f78ab053d005015b2a7e41ba840fec06c5`.
Owned paths are declared in the envelope.

Implemented: failed base removal returns the acquired harness for cleanup; failed root-harness flock closes its opened descriptor. Root-harness cleanup refuses to modify a slot unless its lock was acquired. Added offline regressions for failed startup cleanup/reacquisition, repeated contention without descriptor growth, and preservation of holder files/symlinks after failed contender cleanup.

Actual validation: `git diff --check` passed; `tools/ci.sh check --base main` passed. Go compilation, race tests and complete quick results are pending. No source completion or native acceptance is claimed before those results.

Initial checkpoint: local `a24e9dadc2a37c35191290cb41697f2167fe67ee`, published `e4909890ef388d4375a18bbca227bcb66c452f4e`, equal tree `81a6c8527f273fd07e145a910a5240e0c8978e1f`, draft PR209. Independent review found that pre-acquisition cleanup could alter an existing holder; the lock ownership guard and holder-preservation regression correct that source defect. Fresh review and hosted results remain required.

Next command: `go -C apps/agent test -race -count=1 -run 'TestFailedBaseResetRetainsSlotCleanup|TestRootFRRContendedLockClosesDescriptors|TestRootFRRCleanupWithoutSlotPreservesHolderFiles' ./internal/renderers/frr/frrtest ./internal/agent`; then run the unchanged complete quick gate and obtain independent review.
