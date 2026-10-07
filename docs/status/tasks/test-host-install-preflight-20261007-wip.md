# Installer artifact preflight checkpoint

Branch/worktree/base and owned files: see adjacent envelope.
Local SHA at start: `f6ae6e555ecb1edf5397ff1a8115076ebfc11d38`.
Remote task SHA: not yet published at this checkpoint creation; publication follows commit.

Required context, contributing policy, decision policy, P10 and TD-19 prompts read.
Remote main verified unchanged; open PR list empty. Main hosted quick run in progress,
previous db46e75 run successful (not proof for this head).
`/srv/ngfw-artifacts` absent. Bundle README absent; authoritative bundle instructions
are `docs/user/install/bundle.md`. Full artifact inventory still in progress.
Board TD-19 trust parking is stale: historical PENDING file points to answered D-238.
No target readiness or installation success claimed.

Actual checks: git ls-remote PASS; source inspection only. No full quick gate run:
it installs dependencies and is outside this read-only preflight authority.
Remaining: local artifact/builder evidence, concrete command and prerequisites.
Current failure: no published artifact directory at documented location.
Exact next command: `ls -ld /srv/ngfw-artifacts` then bounded artifact discovery.
