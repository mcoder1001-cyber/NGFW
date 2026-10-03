# P10 parallel portable bundle checksum slice

Base: origin/main 19aa88a5. Branch: codex/p10-resume-parallel-20261003.
Worktree: /root/Documents/Codex/2026-10-03/check-out-latest-code-from-git/work/NGFW-p10-resume.
Worker slot: 1, no daemon or lab activity.
Owned files: deploy/debian/bundle/**, docs/user/install/bundle.md,
docs/status/tasks/P10-parallel-* only. Developer never merges.

Recover the existing bundle verifier/exporter; correct the observed newer-dpkg
fixture error with a valid rebuilt payload and malformed-input refusal test.
Add whole saved transport tar SHA-256 and byte size to the export report,
keeping existing payload byte count. No authentication/security-boundary change.
Requires independent R1 correctness, R2 security and R8 packaging review plus
unchanged hosted quick CI before manager integration.
