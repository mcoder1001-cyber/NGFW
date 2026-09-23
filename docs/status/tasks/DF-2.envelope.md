# TASK ENVELOPE — DF-2
id: DF-2   branch: task/DF-2   worktree: /root/ngfw-wt/DF-2   base: main@030f404   started: 2026-09-23T15:10
title: Descriptors: ip_neighbor, ip6_nd (RA, DAD), urpf, abf, classify (ip tables/routes are P05 core)
prompt: prompts/factories/DF-2.md   (template: prompts/DESCRIPTOR-FACTORY-TEMPLATE.md)   wbs: D2.3, D2.4, D2.7
scope: ip_neighbor, ip6_nd (RA, DAD), urpf, abf, classify (ip tables/routes are P05 core)
merged deps you can rely on: P05a, P04
slot: 3 → VRX_TEST_PREFIX=w3  VRX_HTTP_PORT=3300  VRX_WEB_PORT=5300  VRX_METRICS_PORT=9131  VRX_AGENT_SOCKET=/run/vrx-test/w3/agent.sock  VRX_PG_DATABASE=vrx_w3  VRX_VPP_TABLE_BASE=3000
daemon-owner: none
files you own exclusively: apps/agent/internal/descriptors/<plugins of DF-2>/** docs/agent/descriptors/<plugins>.md
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp, apps/agent/binapi (P04/manager-owned)
time box: 12 h — when exceeded: stop, commit WIP, write docs/status/tasks/DF-2.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/DF-2-wip.md current
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/DF-2.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
questions: docs/status/tasks/DF-2-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
