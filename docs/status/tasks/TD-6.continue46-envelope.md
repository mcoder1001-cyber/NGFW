# TASK ENVELOPE — TD-6 CONTINUE (manager ngfw-46, 2026-09-24 14:00)
id: TD-6   branch: task/TD-6 (already has commits — do not restart)   worktree: /root/ngfw-wt/TD-6   base: main
read first: docs/status/tasks/TD-6-wip.md, docs/status/tasks/TD-6.envelope.md, prompts listed there
slot: 6 → eval "$(tools/lab env 6)"; daemon-owner: none. Never a real --apply; fake-host harness only.
left: (1) decide whether the 24 failures under load (scenarios 4 7 12 14 15 16 20 21 22 23 24 28 30 32 37 40) are timing or real:
  run them serially with the command in TD-6-wip.md (host will NOT be quiet — 10+ agents; record `uptime` before/after). If a scenario still fails,
  run it alone 3× and inspect; fix real bugs; for pure timing make the bound load-tolerant (the harness must be green under load avg ≤ 50 on 32 cores —
  CI runs alongside workers), never by deleting the check. (2) exercise ci.sh's rerun logic once (force one failure, see warn/fail as designed).
  (3) `TMPDIR=/tmp/claude-0/ci-tmp-TD-6 tools/ci.sh --base main` green. (4) docs/status/tasks/TD-6.md with pasted real output.
time box: 2 h. WIP commits every 45 min; keep TD-6-wip.md current.
finish: all committed on task/TD-6, final message = 10-line summary (branch, last commit, CI result, evidence, open questions).
never: merge · restart/kill VPP · pkill · edit /etc/vpp · touch other worktrees or main
