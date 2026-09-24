# TASK ENVELOPE — TD-6 (host manager, 2026-09-24 07:58)
id: TD-6   branch: task/TD-6   worktree: /root/ngfw-wt/TD-6   base: main@HEAD at spawn
title: apply-startup.sh hardening before the first real --apply (D-103)
prompt: none — this envelope + docs/status/tasks/F-startup-apply-verify.md are the task
merged deps you can rely on: F-startup-gen, F-startup-apply (5a0ea5c), P05 (Go VPP client), TD-1 bootid
read first: docs/status/tasks/F-startup-apply-verify.md (V1–V9 + open L1), docs/status/tasks/F-startup-apply.md, docs/agent/renderers/vppstartup.md, docs/decisions/LOG.md D-060, D-103
scope: fix V1 (a dead run's still-armed dead-man must never revert a newer committed apply — bind the dead-man to its run id/sha and disarm on supersede), V2 (rollback write failure must not leave VPP stopped with locks held — write the rollback file before stopping, fail to a clear marker + console-needed + release locks), V3 (`systemctl show` failure right after restart → treat as unhealthy → rollback), V4 (compare NRestarts/MainPID from before the restart so an early crash+auto-restart is not accepted); then V5–V9 where cheap; L1: wire deploy/vpp/test-apply-startup.sh (fake-host harness) and shellcheck of deploy/vpp/*.sh into tools/ci.sh quick mode. Each fix gets a fake-host scenario that failed before and passes after.
slot: 6 → eval "$(tools/lab env 6)"
files you own exclusively: deploy/vpp/apply-startup.sh deploy/vpp/test-apply-startup.sh deploy/vpp/vpp-iface-check* apps/agent/cmd/vrx-vppcheck/** docs/agent/renderers/vppstartup.md docs/status/tasks/TD-6* · tools/ci.sh — ONLY a new, self-contained harness/shellcheck step (TD-3 edits the v19_preflight part of the same file on its branch; do not touch that)
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc, /root/vpp
NEVER run apply-startup.sh --apply for real; never restart/kill VPP; fake-host harness and read-only dry runs only
time box: 3 h — when exceeded: stop, commit WIP, write docs/status/tasks/TD-6.md with what is left
commit WIP at least every 45 min and keep docs/status/tasks/TD-6-wip.md current — you may be killed by a usage limit at any time and respawned with a CONTINUE envelope
finish by: `tools/ci.sh --base main` green · docs/status/tasks/TD-6.md (what, per-finding fixed/left, pasted harness + CI output, out of scope, open questions) · everything committed · final message = 10-line summary
questions: docs/status/tasks/TD-6-questions.md — write and keep going; never wait for a human
never: merge · Docker · pkill · secrets in files/logs/fixtures · install packages
