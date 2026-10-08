# FRR slot lock cleanup checkpoint

Branch: `codex/frr-lock-cleanup-20261008`.
Base: `4d4723f78ab053d005015b2a7e41ba840fec06c5`.
Owned paths are declared in the envelope.

Implemented: failed base removal returns the acquired harness for cleanup; failed root-harness flock closes its opened descriptor. Root-harness cleanup refuses to modify a slot unless its lock was acquired. Added offline regressions for failed startup cleanup/reacquisition, repeated contention without descriptor growth, and preservation of holder files/symlinks after failed contender cleanup.

Actual validation: `git diff --check` passed; `tools/ci.sh check --base main` passed. Go compilation, race tests and complete quick results are pending. No source completion or native acceptance is claimed before those results.

Initial checkpoint: local `a24e9dadc2a37c35191290cb41697f2167fe67ee`, published `e4909890ef388d4375a18bbca227bcb66c452f4e`, equal tree `81a6c8527f273fd07e145a910a5240e0c8978e1f`, draft PR209. Independent review found that pre-acquisition cleanup could alter an existing holder; the lock ownership guard and holder-preservation regression correct that source defect. Fresh review and hosted results remain required.

Next command: `go -C apps/agent test -race -count=1 -run 'TestFailedBaseResetRetainsSlotCleanup|TestRootFRRContendedLockClosesDescriptors|TestRootFRRCleanupWithoutSlotPreservesHolderFiles' ./internal/renderers/frr/frrtest ./internal/agent`; then run the unchanged complete quick gate and obtain independent review.

## Current reviewed source
Local `88d97a9ca9914d673499b275f7c19752ffe648fe`; remote `ef301cbc5b4a9339df626d1a7eadf502e6b8e1ff`; equal tree `e906397df156dc44da74525b3657cbae50b9beb9`. PR209. R2/R4 source APPROVE; R1 identified no remaining source defect but requires executed regression/quick evidence. R7 evidence repair below; R8 review pending.

## Actual limited validation (2026-10-08)
Command: `tools/ci.sh check --base main`
```text
no contract files changed in the 2 commit(s) of HEAD since main (4d4723f)
ok: no secret-shaped strings
ok: ngfwtestsecrets only in test code
WARN gitleaks not installed — built-in secret grep only
ok: no packet trace (trace add / show trace / clear trace / tracedump API) outside docs and the generated bindings
ok: every shell interface create is followed by an ip4/ip6 classify reset
board valid: 212 tasks; read-only validation
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m03s)
```
Command: `git diff --check`
```text
(no output; exit 0)
```
These bounded checks are not the complete quick gate. Actual compiled/race regression evidence and hosted quick on the final integration tree remain mandatory before merge. Hosted current-source run 37806924992 in progress; no PASS claimed. No native startup or 200-route acceptance is claimed.
