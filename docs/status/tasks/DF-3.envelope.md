# TASK ENVELOPE — DF-3
id: DF-3   branch: task/DF-3   worktree: /root/ngfw-wt/DF-3   base: main@030f404   started: 2026-09-23T15:10
title: Descriptors: nat44_ed (nat), nat44_ei, nat64, nat66, det44, map, cnat, pnat
prompt: prompts/factories/DF-3.md   (template: prompts/DESCRIPTOR-FACTORY-TEMPLATE.md)   wbs: D4.1, D4.2, D4.3, D4.4, D4.5, D4.6
scope: nat44_ed (nat), nat44_ei, nat64, nat66, det44, map, cnat, pnat
merged deps you can rely on: P05a, P04
slot: 9 → VRX_TEST_PREFIX=w9  VRX_HTTP_PORT=3900  VRX_WEB_PORT=5900  VRX_METRICS_PORT=9191  VRX_AGENT_SOCKET=/run/vrx-test/w9/agent.sock  VRX_PG_DATABASE=vrx_w9  VRX_VPP_TABLE_BASE=9000
daemon-owner: none
files you own exclusively: apps/agent/internal/descriptors/<plugins of DF-3>/** docs/agent/descriptors/<plugins>.md
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp, apps/agent/binapi (P04/manager-owned)
time box: 12 h — when exceeded: stop, commit WIP, write docs/status/tasks/DF-3.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/DF-3-wip.md current
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/DF-3.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
questions: docs/status/tasks/DF-3-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
