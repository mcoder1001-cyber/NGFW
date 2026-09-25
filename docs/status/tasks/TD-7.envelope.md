# TASK ENVELOPE — TD-7
id: TD-7   branch: task/TD-7   worktree: /root/ngfw-wt/TD-7   base: task/TD-6@621d1e2 (SPECULATIVE, D-114 — TD-6 approved, merge pending)   started: 2026-09-24T16:09
title: apply-startup.sh follow-ups from the TD-6 review (D-116)
read first: docs/status/tasks/TD-6-review.md (F1, F2 — your whole scope), TD-6.md, docs/agent/renderers/vppstartup.md; LOG D-103, D-116
scope:
  F1 two concurrent `--stage rollback` runs of one apply must be impossible: take a per-apply lock at the start of stage_rollback (the lock file keyed by the apply id), and cancel/stop the armed dead-man timer before a manual rollback; the refusal message points the operator at the safe command. New harness scenario(s) proving a second concurrent rollback is refused (VPP stopped/started exactly once) — it must FAIL on TD-6's script.
  F2 scenario 40: replace its load-scaled time bound with a load-independent check (count the fake's `ip neigh` invocations / poll iterations). It must FAIL on main's pre-TD-6 script and pass now, independent of load.
out of scope: F3–F9 (docs/tech-debt.md), any real --apply, /etc/vpp, vpp.service, tools/ci.sh except if the scenario list needs registering.
slot: 6 → eval "$(tools/lab env 6)"; daemon-owner: none. Fake-host harness only.
files you own: deploy/vpp/** docs/agent/renderers/vppstartup.md docs/status/tasks/TD-7*
time box: 2 h. WIP commit every 45 min; docs/status/tasks/TD-7-wip.md
CI: `TMPDIR=/tmp/g-td7 VRX_CI_CACHE_DIR=/tmp/g-td7/cache tools/ci.sh --base main` (fresh cache dir so the harness really runs). Ports 3000/8080/9101 belong to tools/app.
finish: docs/status/tasks/TD-7.md with pasted output (new scenarios failing on TD-6's script, passing on yours; full harness; CI), all committed, final message = 6-line summary.
never: merge · a real --apply · restart/kill VPP · pkill · edit /etc/vpp
