# TASK ENVELOPE — RF-1
id: RF-1   branch: task/RF-1   worktree: /root/ngfw-wt/RF-1   base: main@40ba948   started: 2026-09-24T00:30
title: Renderers: frr (renderer framework: files, vtysh -C, frr-reload.py, JSON state; protocol semantics in P12/F-*)
prompt: prompts/factories/RF-1.md   (template: prompts/RENDERER-FACTORY-TEMPLATE.md)
merged deps you can rely on: P05a, P03
slot: 12 → VRX_TEST_PREFIX=w12  VRX_HTTP_PORT=4200  VRX_WEB_PORT=6200  VRX_METRICS_PORT=9221  VRX_AGENT_SOCKET=/run/vrx-test/w12/agent.sock  VRX_PG_DATABASE=vrx_w12  VRX_VALKEY_DB=12  VRX_VPP_TABLE_BASE=12000  (source of truth: `eval "$(tools/lab env 12)"`)
daemon-owner: frr (test-scoped processes under /run/vrx-test/w12/frr only; system frr unit stays stopped and disabled)
files you own exclusively: apps/agent/internal/renderers/frr/** docs/agent/renderers/frr.md docs/status/tasks/RF-1*
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp, /etc/frr, apps/agent/binapi
consumers: P12 (BGP), F-ospf, F-isis-rip, F-bfd-redistribution, F-mpls-srmpls + F-igmp-mfib (V5 agent-side FRR JSON sync, D-056) — the Section registry and JSON Retrieve must serve them
time box: 12 h — when exceeded: stop, commit WIP, write docs/status/tasks/RF-1.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/RF-1-wip.md current
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/RF-1.md with pasted real output · everything committed · final message = 10-line summary
questions: docs/status/tasks/RF-1-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own · start system daemon units
