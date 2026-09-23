# TASK ENVELOPE — DF-4
id: DF-4   branch: task/DF-4   worktree: /root/ngfw-wt/DF-4   base: main@030f404   started: 2026-09-23T15:10
title: Descriptors: acl (incl. macip), acl stats
prompt: prompts/factories/DF-4.md   (template: prompts/DESCRIPTOR-FACTORY-TEMPLATE.md)   wbs: D5.2
scope: acl (incl. macip), acl stats
merged deps you can rely on: P05a, P04
slot: 10 → VRX_TEST_PREFIX=w10  VRX_HTTP_PORT=31000  VRX_WEB_PORT=51000  VRX_METRICS_PORT=91101  VRX_AGENT_SOCKET=/run/vrx-test/w10/agent.sock  VRX_PG_DATABASE=vrx_w10  VRX_VPP_TABLE_BASE=10000
daemon-owner: none
files you own exclusively: apps/agent/internal/descriptors/<plugins of DF-4>/** docs/agent/descriptors/<plugins>.md
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp, apps/agent/binapi (P04/manager-owned)
time box: 12 h — when exceeded: stop, commit WIP, write docs/status/tasks/DF-4.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/DF-4-wip.md current
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/DF-4.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
questions: docs/status/tasks/DF-4-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
