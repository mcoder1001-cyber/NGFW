# TASK ENVELOPE — DF-6
id: DF-6   branch: task/DF-6   worktree: /root/ngfw-wt/DF-6   base: main@af83adb   started: 2026-09-23T15:28
title: Descriptors: gre, ipip, vxlan, vxlan_gpe, gtpu, l2tp, pppoe, sr (srv6 + mpls), lisp
prompt: prompts/factories/DF-6.md   (template: prompts/DESCRIPTOR-FACTORY-TEMPLATE.md)   wbs: D6.6, D6.7, D6.8, D2.8
scope: gre, ipip, vxlan, vxlan_gpe, gtpu, l2tp, pppoe, sr (srv6 + mpls), lisp
merged deps you can rely on: P05a, P04
slot: 11 → VRX_TEST_PREFIX=w11  VRX_HTTP_PORT=31100  VRX_WEB_PORT=51100  VRX_METRICS_PORT=91111  VRX_AGENT_SOCKET=/run/vrx-test/w11/agent.sock  VRX_PG_DATABASE=vrx_w11  VRX_VPP_TABLE_BASE=11000
daemon-owner: none
files you own exclusively: apps/agent/internal/descriptors/<plugins of DF-6>/** docs/agent/descriptors/<plugins>.md
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp, apps/agent/binapi (P04/manager-owned)
time box: 12 h — when exceeded: stop, commit WIP, write docs/status/tasks/DF-6.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/DF-6-wip.md current
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/DF-6.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
questions: docs/status/tasks/DF-6-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
