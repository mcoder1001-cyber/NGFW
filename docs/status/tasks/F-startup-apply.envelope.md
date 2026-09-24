# TASK ENVELOPE — F-startup-apply
id: F-startup-apply   branch: task/F-startup-apply   worktree: /root/ngfw-wt/F-startup-apply   base: main@78539ec   started: 2026-09-24
title: Robust startup.conf apply tooling (detached, watchdog, hung-VPP, ifupdown restore, Go API checks, handover gate)
prompt: prompts/features/F-startup-gen.md
merged deps you can rely on: F-startup-gen (merged 9923526), P05 (Go VPP client), TD-1 bootid
slot: 6 → VRX_SLOT=6 VRX_TEST_PREFIX=w6 VRX_HTTP_PORT=3600 VRX_WEB_PORT=5600 VRX_METRICS_PORT=9161 VRX_AGENT_SOCKET=/run/vrx-test/w6/agent.sock VRX_PG_DATABASE=vrx_w6 VRX_VALKEY_DB=6 VRX_VPP_TABLE_BASE=6000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock  (source of truth: `eval "$(tools/lab env 6)"`; slot 12 is CI-only, D-087)
rules on main to honour: LOG D-039…D-091 (esp. D-051 secret refs, D-069 logical interface names, D-070 redactSecrets, D-071 globals owner, D-078 contracts-v1 — additive contract changes only on a contract/F-startup-apply branch, D-087 host tests one package at a time, D-091 P06 RBAC/secret/sync-state semantics)
scope: branch task/F-startup-apply already holds the old script (from afbe6ae) — first `git merge main`; fix re-review N1 (vppctl/API timeouts, hung VPP → rollback), N2 (ifupdown/networkd/netplan-aware restore of mgmt address + default route), N3 (replace vpp_papi checker with a small Go binary using the agent's VPP client), N6 (env through systemd-run), enforce the handover gate, drop the committed .pyc; NEVER run --apply for real — test only against the fake-host harness; read docs/status/tasks/F-startup-gen-rereview.md on main
files you own exclusively: deploy/vpp/apply-startup.sh deploy/vpp/vpp-iface-check* deploy/vpp/test-apply-startup.sh apps/agent/cmd/vrx-vppcheck/** docs/agent/renderers/vppstartup.md docs/status/tasks/F-startup-apply*
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc, /root/vpp
time box: 6 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-startup-apply.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-startup-apply-wip.md current
finish: `tools/ci.sh --base main` green · docs/status/tasks/F-startup-apply.md with pasted real output · everything committed · final message = 10-line summary
questions: docs/status/tasks/F-startup-apply-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files/logs/fixtures · edit files you do not own · install packages
