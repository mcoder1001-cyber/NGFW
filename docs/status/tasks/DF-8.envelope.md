# TASK ENVELOPE — DF-8
id: DF-8   branch: task/DF-8   worktree: /root/ngfw-wt/DF-8   base: main@52bab0b   started: 2026-09-24
title: Descriptors: dhcp, dns, flowprobe, sflow, prom, pcap/tracenode, lcp
prompt: prompts/factories/DF-8.md   (template: prompts/DESCRIPTOR-FACTORY-TEMPLATE.md)   wbs: D7.2, D7.3, D7.6, D8.2, D8.3, D3.1
merged deps you can rely on: P05a, P04
slot: 5 → VRX_SLOT=5 VRX_TEST_PREFIX=w5 VRX_HTTP_PORT=3500 VRX_WEB_PORT=5500 VRX_METRICS_PORT=9151 VRX_AGENT_SOCKET=/run/vrx-test/w5/agent.sock VRX_PG_DATABASE=vrx_w5 VRX_VALKEY_DB=5 VRX_VPP_TABLE_BASE=5000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock  (source of truth: `eval "$(tools/lab env 5)"`)
daemon-owner: none
note: linux_cp, linux_nl and npt66 are LOADED since 2026-09-24 (D-060) — lcp is no longer skip-unless-loaded; test lcp pairs for real with prefixed host interfaces (w5-*), clean up in t.Cleanup; never pair or touch ens* NICs or local0
files you own exclusively: apps/agent/internal/descriptors/<plugins of DF-8>/** docs/agent/descriptors/<plugins>.md docs/status/tasks/DF-8*
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp, apps/agent/binapi (manager-owned; missing plugin → questions file)
time box: 12 h — when exceeded: stop, commit WIP, write docs/status/tasks/DF-8.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/DF-8-wip.md current
finish: `tools/ci.sh --base main` green · docs/status/tasks/DF-8.md with pasted real output · everything committed · final message = 10-line summary
questions: docs/status/tasks/DF-8-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
