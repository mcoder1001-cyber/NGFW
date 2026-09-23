# TASK ENVELOPE — RF-3
id: RF-3   branch: task/RF-3   worktree: /root/ngfw-wt/RF-3   base: main@179676a   started: 2026-09-24
title: Renderers: kea-dhcp4/6 + ctrl-agent, unbound, chrony
prompt: prompts/factories/RF-3.md   (template: prompts/RENDERER-FACTORY-TEMPLATE.md)
merged deps you can rely on: P05a, P03, P02c (services schema: dhcp/dns/ntp, D-050 NTP lives in services.ntp)
slot: 6 → VRX_SLOT=6 VRX_TEST_PREFIX=w6 VRX_HTTP_PORT=3600 VRX_WEB_PORT=5600 VRX_METRICS_PORT=9161 VRX_AGENT_SOCKET=/run/vrx-test/w6/agent.sock VRX_PG_DATABASE=vrx_w6 VRX_VALKEY_DB=6 VRX_VPP_TABLE_BASE=6000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock  (source of truth: `eval "$(tools/lab env 6)"`)
daemon-owner: kea-dhcp4, kea-dhcp6, kea-ctrl-agent, unbound, chrony — test-scoped processes under /run/vrx-test/w6/ only, bound to 127.0.0.1 or rig namespaces; system units stay stopped and disabled; chrony must NEVER step or slew the host clock in tests (use -x / no-clock-control mode)
files you own exclusively: apps/agent/internal/renderers/{kea,unbound,chrony}/** docs/agent/renderers/{kea,unbound,chrony}.md docs/status/tasks/RF-3*
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp, /etc/kea, /etc/unbound, /etc/chrony, apps/agent/binapi; the renderer framework pieces RF-1 is building in apps/agent/internal/renderers/frr (read-only reference once committed)
time box: 12 h — when exceeded: stop, commit WIP, write docs/status/tasks/RF-3.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/RF-3-wip.md current
finish: `tools/ci.sh --base main` green · docs/status/tasks/RF-3.md with pasted real output · everything committed · test daemons stopped · final message = 10-line summary
questions: docs/status/tasks/RF-3-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own · start/enable system daemon units
