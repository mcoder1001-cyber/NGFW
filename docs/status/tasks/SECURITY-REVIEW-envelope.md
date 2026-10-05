# SECURITY-REVIEW task envelope
Branch: codex/security-final-20261005
Worktree: /root/ngfw-wt/security-final-20261005
Baseline: d314f0728 (origin/main at start).
Ownership: prompts/SECURITY-REVIEW.md and docs/status/tasks/SECURITY-REVIEW*.md only; coordinate product fixes before editing.
Scope: whole-tree adversarial review, auth delta after SEC-auth, existing abuse regression tests, secret scanning and complete unchanged quick gate. No shared-host mutations. Root owns board, independent review and integration.
Recovery: checkpoint and publish coherent changes immediately, at least every 15 minutes. No unexecuted lab acceptance called passing.
