# TASK ENVELOPE — DF-5
id: DF-5   branch: task/DF-5   worktree: /root/ngfw-wt/DF-5   base: main@af83adb   started: 2026-09-23T15:28
title: Descriptors: ipsec, ikev2, wireguard
prompt: prompts/factories/DF-5.md   (template: prompts/DESCRIPTOR-FACTORY-TEMPLATE.md)   wbs: D6.1, D6.3, D6.5
scope: ipsec, ikev2, wireguard
merged deps you can rely on: P05a, P04
slot: 4 → VRX_TEST_PREFIX=w4  VRX_HTTP_PORT=3400  VRX_WEB_PORT=5400  VRX_METRICS_PORT=9141  VRX_AGENT_SOCKET=/run/vrx-test/w4/agent.sock  VRX_PG_DATABASE=vrx_w4  VRX_VPP_TABLE_BASE=4000
daemon-owner: none
files you own exclusively: apps/agent/internal/descriptors/<plugins of DF-5>/** docs/agent/descriptors/<plugins>.md
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp, apps/agent/binapi (P04/manager-owned)
time box: 12 h — when exceeded: stop, commit WIP, write docs/status/tasks/DF-5.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/DF-5-wip.md current
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/DF-5.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
questions: docs/status/tasks/DF-5-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
