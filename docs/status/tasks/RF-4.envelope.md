# TASK ENVELOPE — RF-4
id: RF-4   branch: task/RF-4   worktree: /root/ngfw-wt/RF-4   base: main@ff6b91a   started: 2026-09-24
title: Renderers: snmpd, keepalived, rsyslog
prompt: prompts/factories/RF-4.md
merged deps you can rely on: P05a, P03, P02a/P02c (management, ha, services schema), RF-1 (framework reference)
slot: 8 → VRX_SLOT=8 VRX_TEST_PREFIX=w8 VRX_HTTP_PORT=3800 VRX_WEB_PORT=5800 VRX_METRICS_PORT=9181 VRX_AGENT_SOCKET=/run/vrx-test/w8/agent.sock VRX_PG_DATABASE=vrx_w8 VRX_VALKEY_DB=8 VRX_VPP_TABLE_BASE=8000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock  (source of truth: `eval "$(tools/lab env 8)"`)
daemon-owner: snmpd, keepalived, rsyslog — test-scoped processes under /run/vrx-test/w8/ only; keepalived must never send VRRP on real host links (use a rig netns)
rules on main to honour: D-049 input hardening, D-051 secret refs, D-055 stand-ins, D-069 logical interface names (iface.ResolveName), D-071 globals owner + claim rule, D-072 static routes single programmer, D-076 idempotent write-only; renderer framework reference: apps/agent/internal/renderers/frr (merged RF-1) — ALLOWLIST.md edits are expected, keep them minimal

files you own exclusively: apps/agent/internal/renderers/{snmpd,keepalived,rsyslog}/** docs/agent/renderers/{snmpd,keepalived,rsyslog}.md docs/status/tasks/RF-4*
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp, apps/agent/binapi, system daemon config under /etc
time box: 12 h — when exceeded: stop, commit WIP, write docs/status/tasks/RF-4.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/RF-4-wip.md current
finish: `tools/ci.sh --base main` green · docs/status/tasks/RF-4.md with pasted real output · everything committed · test daemons stopped · final message = 10-line summary
questions: docs/status/tasks/RF-4-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files/logs · edit files you do not own · start/enable system daemon units
