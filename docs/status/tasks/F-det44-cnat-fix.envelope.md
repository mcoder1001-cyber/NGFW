TASK ENVELOPE
id: F-det44-cnat-fix   branch: task/F-det44-cnat-fix   worktree: /root/ngfw-wt/F-det44-cnat-fix
repo of record: /root/NGFW (any prompt that says /root/ngfw means /root/NGFW). main belongs to the manager: never commit, merge, reset or check out anything in /root/NGFW; never touch another worktree.
merged deps you can rely on: F-det44-map-dslite-cnat-host (e8fb9c57) (base main@55a18e0f)
slot: 16 → `eval "$(tools/lab env 16)"` (prefix w16, ports, tables 16000-16999, DB vrx_w16; docs/lab/shared-host-rules.md); daemon-owner: none
files you own exclusively: apps/agent/internal/descriptors/det44/** apps/agent/internal/descriptors/cnat/** test/topology/det44/** docs/status/tasks/F-det44-cnat-fix*   files you must not touch: everything else
host row (1 of max 2 on the shared VPP): det44 enable/globals only inside an exclusive lab-lock window as the det44 rig does; DS-Lite pool deletes banned (D-211); delete your own static routes via binapi ip_route_add_del (D-218); rig down w16 before and after.
build: shared on-disk GOCACHE — do not set GOCACHE, never put any cache under /tmp (tmpfs = RAM); TMPDIR=/tmp/g-w16 is allowed for ci runs only, delete it before you finish. CI gate = `tools/ci-slot.sh --base main` (2-slot semaphore, D-220) — never run tools/ci.sh directly; one gate run at the end (plus one after a fix), not after every edit.
tests: D-210 — no full suite, no lint, no tester. D-210a — write tests for your own change and get them passing (your package / files only); paste the output.
host: never restart or kill VPP, never edit /etc/vpp or vpp.service; prefixed objects only; `timeout` every vppctl; packet trace banned (D-128); no pkill/killall (kill only PIDs you started); stop every process you started before finishing.
permission refusals are final: do not look for another route to the same effect; write it in docs/status/tasks/F-det44-cnat-fix-questions.md and continue. No git push, ever. No secrets in any file (redact as <redacted>).
WORKER-OPS: you run ON the host — ignore its desktop/ssh/wt.sh parts and work directly in your worktree; its Finishing and Blocked sections apply (with tools/ci-slot.sh).
commit WIP at least every 45 min and keep docs/status/tasks/F-det44-cnat-fix-wip.md current — you may be killed by a usage limit at any time and respawned with a CONTINUE envelope
time box: 4h — when exceeded, stop, commit WIP, write docs/status/tasks/F-det44-cnat-fix.md with what is left
finish by: committing on your branch, writing docs/status/tasks/F-det44-cnat-fix.md (what, how verified — paste real output — out of scope, open questions, decisions), `tools/ci-slot.sh --base main` green in your worktree
questions: write them in docs/status/tasks/F-det44-cnat-fix-questions.md and keep going on everything not blocked by them
