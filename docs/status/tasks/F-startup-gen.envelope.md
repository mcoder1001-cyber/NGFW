# TASK ENVELOPE — F-startup-gen
id: F-startup-gen   branch: task/F-startup-gen   worktree: /root/ngfw-wt/F-startup-gen   base: main@ff6b91a   started: 2026-09-24
title: VPP startup.conf generator (D0.6)
prompt: prompts/features/F-startup-gen.md
merged deps you can rely on: P04, P02a (dataplane schema), RF-1 (renderer style)
slot: 12 → VRX_SLOT=12 VRX_TEST_PREFIX=w12 VRX_HTTP_PORT=4200 VRX_WEB_PORT=6200 VRX_METRICS_PORT=9221 VRX_AGENT_SOCKET=/run/vrx-test/w12/agent.sock VRX_PG_DATABASE=vrx_w12 VRX_VALKEY_DB=12 VRX_VPP_TABLE_BASE=12000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock  (source of truth: `eval "$(tools/lab env 12)"`)
daemon-owner: none
rules on main to honour: D-049 input hardening, D-051 secret refs, D-055 stand-ins, D-069 logical interface names (iface.ResolveName), D-071 globals owner + claim rule, D-072 static routes single programmer, D-076 idempotent write-only; renderer framework reference: apps/agent/internal/renderers/frr (merged RF-1) — ALLOWLIST.md edits are expected, keep them minimal
note: read /etc/vpp/startup.conf only; never write under /etc, never restart VPP (applying is a manager step)
files you own exclusively: apps/agent/internal/renderers/vppstartup/** apps/agent/cmd/vrx-startupgen/** docs/agent/renderers/vppstartup.md docs/status/tasks/F-startup-gen*
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp, apps/agent/binapi, system daemon config under /etc
time box: 8 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-startup-gen.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-startup-gen-wip.md current
finish: `tools/ci.sh --base main` green · docs/status/tasks/F-startup-gen.md with pasted real output · everything committed · test daemons stopped · final message = 10-line summary
questions: docs/status/tasks/F-startup-gen-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files/logs · edit files you do not own · start/enable system daemon units
