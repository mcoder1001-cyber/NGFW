# TASK ENVELOPE — TD-6 (CONTINUE; host manager 2026-09-24 13:2x, after manager restart)
id: TD-6   branch: task/TD-6   worktree: /root/ngfw-wt/TD-6
CONTINUE: the worker was killed at ~08:20 by a manager/host restart. Branch task/TD-6 has V1–V9 fixed + scenarios 32–41 (ff520d3). Read docs/status/tasks/TD-6-wip.md and continue — do not restart. The original envelope still applies: docs/status/tasks/TD-6.envelope.md (scope, ownership, NEVER --apply for real, never restart VPP).
left to do: full fake-host harness run, L1 (harness + shellcheck step in tools/ci.sh quick — self-contained step only, TD-3 edits the v19_preflight part of the same file), docs/agent/renderers/vppstartup.md, docs/status/tasks/TD-6.md with pasted harness + CI output, tools/ci.sh --base main green, all committed.
time box: 1.5 h   final message = 10-line summary
