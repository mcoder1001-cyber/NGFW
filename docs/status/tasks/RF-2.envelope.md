# TASK ENVELOPE — RF-2
id: RF-2   branch: task/RF-2   worktree: /root/ngfw-wt/RF-2   base: main@ff6b91a   started: 2026-09-24
title: Renderers: strongswan (swanctl/VICI path; vrx build lands in P11)
prompt: prompts/factories/RF-2.md
merged deps you can rely on: P05a, P03, P02c (vpn schema), RF-1 (framework reference)
slot: 3 → VRX_SLOT=3 VRX_TEST_PREFIX=w3 VRX_HTTP_PORT=3300 VRX_WEB_PORT=5300 VRX_METRICS_PORT=9131 VRX_AGENT_SOCKET=/run/vrx-test/w3/agent.sock VRX_PG_DATABASE=vrx_w3 VRX_VALKEY_DB=3 VRX_VPP_TABLE_BASE=3000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock  (source of truth: `eval "$(tools/lab env 3)"`)
daemon-owner: strongswan/charon — test-scoped charon under /run/vrx-test/w3/ only, never the system unit
rules on main to honour: D-049 input hardening, D-051 secret refs, D-055 stand-ins, D-069 logical interface names (iface.ResolveName), D-071 globals owner + claim rule, D-072 static routes single programmer, D-076 idempotent write-only; renderer framework reference: apps/agent/internal/renderers/frr (merged RF-1) — ALLOWLIST.md edits are expected, keep them minimal

files you own exclusively: apps/agent/internal/renderers/strongswan/** docs/agent/renderers/strongswan.md docs/status/tasks/RF-2*
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp, apps/agent/binapi, system daemon config under /etc
time box: 12 h — when exceeded: stop, commit WIP, write docs/status/tasks/RF-2.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/RF-2-wip.md current
finish: `tools/ci.sh --base main` green · docs/status/tasks/RF-2.md with pasted real output · everything committed · test daemons stopped · final message = 10-line summary
questions: docs/status/tasks/RF-2-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files/logs · edit files you do not own · start/enable system daemon units
