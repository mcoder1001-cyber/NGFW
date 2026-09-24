# TASK ENVELOPE — TD-3 (CONTINUE fix round 2; host manager 2026-09-24 13:2x, after manager restart)
id: TD-3   branch: task/TD-3   worktree: /root/ngfw-wt/TD-3
CONTINUE: the fix-round-2 worker was killed at ~08:15 by a manager/host restart. Branch task/TD-3 already has H1/M1/M2/L5 committed (cd21806, e73db90). Read docs/status/tasks/TD-3-wip.md and continue from there — do not redo finished parts.
The full round-2 envelope still applies: docs/status/tasks/TD-3.round2-envelope.md (read it first; scope, file ownership, never-list unchanged).
left to do: host runs (ifsanitize, ipsec, wireguard; slot 2; NRestarts before/after each run pasted — VPP was restarted at 13:03 by its owner and the host rebooted at 09:47, so the baseline is NRestarts=0 now), "Fix round 2" section in docs/status/tasks/TD-3.md with pasted real output, tools/ci.sh --base main green, all committed.
time box: 1.5 h   final message = 10-line summary
