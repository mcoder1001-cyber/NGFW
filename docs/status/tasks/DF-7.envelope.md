# TASK ENVELOPE — DF-7
id: DF-7   branch: task/DF-7   worktree: /root/ngfw-wt/DF-7   base: main@c791087   started: 2026-09-24
title: Descriptors: policer, qos, lb, span, lldp, bfd, vrrp, igmp, mpls
prompt: prompts/factories/DF-7.md   (template: prompts/DESCRIPTOR-FACTORY-TEMPLATE.md)   wbs: D7.8, D7.9, D1.10, D1.7, D3.8, D9.1, D2.9, D2.8
merged deps you can rely on: P05a, P04, DF-4 (acl — merged example of a finished factory)
slot: 10 → VRX_SLOT=10 VRX_TEST_PREFIX=w10 VRX_HTTP_PORT=4000 VRX_WEB_PORT=6000 VRX_METRICS_PORT=9201 VRX_AGENT_SOCKET=/run/vrx-test/w10/agent.sock VRX_PG_DATABASE=vrx_w10 VRX_VALKEY_DB=10 VRX_VPP_TABLE_BASE=10000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock  (source of truth: `eval "$(tools/lab env 10)"`)
daemon-owner: none
rules added 2026-09-24: interface references use the alias key `interface/<name>` (D-065); descriptors without a VPP dump return ErrRetrieveUnsupported and are write-only (D-063); before/after the first host run of each plugin check `systemctl show vpp -p NRestarts` — a crash → stop, gate the test behind an opt-in env var, report the message in questions (D-064)
files you own exclusively: apps/agent/internal/descriptors/<plugins of DF-7>/** docs/agent/descriptors/<plugins>.md docs/status/tasks/DF-7*
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp, apps/agent/binapi (manager-owned; missing plugin → questions file)
time box: 12 h — when exceeded: stop, commit WIP, write docs/status/tasks/DF-7.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/DF-7-wip.md current
finish: `tools/ci.sh --base main` green · docs/status/tasks/DF-7.md with pasted real output · everything committed · final message = 10-line summary
questions: docs/status/tasks/DF-7-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
