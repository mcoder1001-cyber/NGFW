# TASK ENVELOPE — DF-1
id: DF-1   branch: task/DF-1   worktree: /root/ngfw-wt/DF-1   base: main@030f404   started: 2026-09-23T15:10
title: Descriptors: bond, l2 (bridge, xconnect), memif, tap, host-interface/af_packet, subinterface, admin-state, mtu, rx-mode
prompt: prompts/factories/DF-1.md   (template: prompts/DESCRIPTOR-FACTORY-TEMPLATE.md)   wbs: D1.2, D1.4, D1.5, D1.6
scope: bond, l2 (bridge, xconnect), memif, tap, host-interface/af_packet, subinterface, admin-state, mtu, rx-mode
merged deps you can rely on: P05a, P04
slot: 2 → VRX_TEST_PREFIX=w2  VRX_HTTP_PORT=3200  VRX_WEB_PORT=5200  VRX_METRICS_PORT=9121  VRX_AGENT_SOCKET=/run/vrx-test/w2/agent.sock  VRX_PG_DATABASE=vrx_w2  VRX_VPP_TABLE_BASE=2000
daemon-owner: none
files you own exclusively: apps/agent/internal/descriptors/<plugins of DF-1>/** docs/agent/descriptors/<plugins>.md
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp, apps/agent/binapi (P04/manager-owned)
time box: 12 h — when exceeded: stop, commit WIP, write docs/status/tasks/DF-1.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/DF-1-wip.md current
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/DF-1.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
questions: docs/status/tasks/DF-1-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
