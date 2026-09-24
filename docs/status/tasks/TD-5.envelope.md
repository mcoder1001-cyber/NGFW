# TASK ENVELOPE — TD-5
id: TD-5   branch: task/TD-5   worktree: /root/ngfw-wt/TD-5   base: main@bccb9e4   started: 2026-09-24T14:17
title: V24 guard: the agent's af_packet Delete quiesces the Linux netdev (link down via netlink) before af_packet_delete; restart-simulation fixtures do the same
prompt: prompts/tech-debt/TD-5.md   (template: none — tech-debt row)   wbs: D1.2
scope: D-101 (V24) + TD-3 re-review L7 (Create's rollback delete too); apps/agent/internal/descriptors/af_packet/** + its tests only; build on TD-3's merged af_packet code
merged deps you can rely on: TD-3 (ifsanitize, iface.AcquireAndTag rollback, BeforeDelete wiring in af_packet), DF-1, TD-1. P08 is NOT merged yet (its fixture on task/P08 quiesces via rig.peers(false)).
slot: 2 → VRX_SLOT=2 VRX_TEST_PREFIX=w2 VRX_HTTP_PORT=3200 VRX_WEB_PORT=5200 VRX_METRICS_PORT=9121 VRX_AGENT_SOCKET=/run/vrx-test/w2/agent.sock VRX_PG_DATABASE=vrx_w2 VRX_VALKEY_DB=2 VRX_VPP_TABLE_BASE=2000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock  (source of truth: `eval "$(tools/lab env 2)"`)
daemon-owner: none
files you own exclusively: apps/agent/internal/descriptors/af_packet/** apps/agent/internal/vpp/ifsanitize/** (placeholder cap only — scope item 7, D-105) docs/agent/descriptors/af_packet.md docs/agent/descriptors/interface.md (cap paragraph only) docs/status/tasks/TD-5*
shared hotspots (append-only, conflicts resolved by the manager at merge): docs/vpp-code-track.md (V24 row: append the agent-side status only)
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp, apps/agent/binapi (P04/manager-owned), apps/agent/go.{mod,sum} (no new dependency — golang.org/x/sys is already direct), apps/agent/internal/descriptors/interface/** (TD-3), tools/lab (manager), test/topology/** (P08 / features)
V24/V19 SAFETY: host tests on your slot prefix only, one package at a time, `VRX_INTEGRATION=1` under `flock -s /run/lock/vrx-lab.lock`, NO packets (test veths get disable_ipv6 before up); never af_packet_delete / `delete host-interface` with the veth up by any path other than the quiesced code under test; `systemctl show vpp -p NRestarts` before/after every host run pasted — stop host runs and write it down if it rises
time box: 3 h — when exceeded: stop, commit WIP, write docs/status/tasks/TD-5.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/TD-5-wip.md current
CI: `TMPDIR=/tmp/g-td5 tools/ci.sh --base main` (short TMPDIR — unix socket paths ≤ 108 chars; golangci-lint serializes itself since main fc0fe68). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them.
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/TD-5.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
questions: docs/status/tasks/TD-5-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
