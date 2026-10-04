TASK ENVELOPE
id: S-alarms-restart-rebuild   branch: task/S-alarms-restart-rebuild   worktree: /root/ngfw-wt/S-alarms-restart-rebuild
repo of record: /root/NGFW (any prompt that says /root/ngfw means /root/NGFW). main belongs to the manager: never commit, merge, reset or check out anything in /root/NGFW; never touch another worktree.
merged deps you can rely on: F-dashboard-prom-alarms-host (97ad50cd) (base main@55a18e0f)
slot: 10 → `eval "$(tools/lab env 10)"` (prefix w10, ports, tables 10000-10999, DB ngfw_w10; docs/lab/shared-host-rules.md); daemon-owner: none
files you own exclusively: apps/api/src/features/dashboard-prom-alarms/** docs/status/tasks/S-alarms-restart-rebuild*   files you must not touch: everything else
API only on your slot port (NGFW_HTTP_PORT) and your DB ngfw_w10; stop it by PID when done.
build: shared on-disk GOCACHE — do not set GOCACHE, never put any cache under /tmp (tmpfs = RAM); TMPDIR=/tmp/g-w10 is allowed for ci runs only, delete it before you finish. CI gate = `tools/ci-slot.sh --base main` (2-slot semaphore, D-220) — never run tools/ci.sh directly; one gate run at the end (plus one after a fix), not after every edit.
tests: D-210 — no full suite, no lint, no tester. D-210a — write tests for your own change and get them passing (your package / files only); paste the output.
host: never restart or kill VPP, never edit /etc/vpp or vpp.service; prefixed objects only; `timeout` every vppctl; packet trace banned (D-128); no pkill/killall (kill only PIDs you started); stop every process you started before finishing.
permission refusals are final: do not look for another route to the same effect; write it in docs/status/tasks/S-alarms-restart-rebuild-questions.md and continue. No git push, ever. No secrets in any file (redact as <redacted>).
WORKER-OPS: you run ON the host — ignore its desktop/ssh/wt.sh parts and work directly in your worktree; its Finishing and Blocked sections apply (with tools/ci-slot.sh).
commit WIP at least every 45 min and keep docs/status/tasks/S-alarms-restart-rebuild-wip.md current — you may be killed by a usage limit at any time and respawned with a CONTINUE envelope
time box: 3h — when exceeded, stop, commit WIP, write docs/status/tasks/S-alarms-restart-rebuild.md with what is left
finish by: committing on your branch, writing docs/status/tasks/S-alarms-restart-rebuild.md (what, how verified — paste real output — out of scope, open questions, decisions), `tools/ci-slot.sh --base main` green in your worktree
questions: write them in docs/status/tasks/S-alarms-restart-rebuild-questions.md and keep going on everything not blocked by them
