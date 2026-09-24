# TASK ENVELOPE — F-vpp-debs
id: F-vpp-debs   branch: task/F-vpp-debs   worktree: /root/ngfw-wt/F-vpp-debs   base: main@87f84b2   started: 2026-09-24
title: VPP package build pipeline: pinned 26.06 source, patch series, reproducible .deb build script (D0.2)
prompt: prompts/features/F-vpp-debs.md
merged deps you can rely on: P01, P09 (CI)
slot: 2 → VRX_SLOT=2 VRX_TEST_PREFIX=w2 VRX_HTTP_PORT=3200 VRX_WEB_PORT=5200 VRX_METRICS_PORT=9121 VRX_AGENT_SOCKET=/run/vrx-test/w2/agent.sock VRX_PG_DATABASE=vrx_w2 VRX_VALKEY_DB=2 VRX_VPP_TABLE_BASE=2000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock 
daemon-owner: none
/root/vpp is OWNED BY THE BRING-UP AGENT (D-012): read-only for you (git log/status, file reads, `git clone --reference` / `git worktree add` into your own .build dir are fine; never checkout/reset/clean/build inside /root/vpp itself)
resources: a full VPP build is heavy — use at most 8 parallel jobs (`make pkg-deb` with -j8 / MAKEFLAGS=-j8) so 11 other workers keep running; disk budget ≤ 40 GB under your worktree's .build (git-ignored)
files you own exclusively: deploy/vpp/** docs/status/tasks/F-vpp-debs* .gitignore entries for deploy/vpp/.build and *.deb
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp (read-only), dpkg state (no apt/dpkg install)
time box: 8 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-vpp-debs.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-vpp-debs-wip.md current
finish: `tools/ci.sh --base main` green · docs/status/tasks/F-vpp-debs.md with pasted real output · everything committed (no .deb in git) · final message = 10-line summary
questions: docs/status/tasks/F-vpp-debs-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · install packages · Docker · pkill · edit files you do not own
