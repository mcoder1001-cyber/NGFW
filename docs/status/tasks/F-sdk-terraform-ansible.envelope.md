# TASK ENVELOPE — F-sdk-terraform-ansible
id: F-sdk-terraform-ansible   branch: task/F-sdk-terraform-ansible   worktree: /root/ngfw-wt/F-sdk-terraform-ansible   base: main@78539ec   started: 2026-09-24
title: Terraform provider + Python SDK (Ansible cut, see prompt)
prompt: prompts/features/F-sdk-terraform-ansible.md
merged deps you can rely on: P06 (OpenAPI + api-client)
slot: 5 → VRX_SLOT=5 VRX_TEST_PREFIX=w5 VRX_HTTP_PORT=3500 VRX_WEB_PORT=5500 VRX_METRICS_PORT=9151 VRX_AGENT_SOCKET=/run/vrx-test/w5/agent.sock VRX_PG_DATABASE=vrx_w5 VRX_VALKEY_DB=5 VRX_VPP_TABLE_BASE=5000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock  (source of truth: `eval "$(tools/lab env 5)"`; slot 12 is CI-only, D-087)
rules on main to honour: LOG D-039…D-091 (esp. D-051 secret refs, D-069 logical interface names, D-070 redactSecrets, D-071 globals owner, D-078 contracts-v1 — additive contract changes only on a contract/F-sdk-terraform-ansible branch, D-087 host tests one package at a time, D-091 P06 RBAC/secret/sync-state semantics)
note: no network downloads of providers/SDK deps except via pinned, checksummed package managers already configured on the host; if a toolchain (terraform CLI) is missing, do not install it — test with the plugin SDK's own test harness and say so
files you own exclusively: sdk/** docs/user/system/sdk-terraform-ansible.md test/topology/sdk-terraform-ansible/** docs/status/tasks/F-sdk-terraform-ansible*.md docs/status/tasks/F-sdk-terraform-ansible*
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc, /root/vpp
time box: 10 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-sdk-terraform-ansible.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-sdk-terraform-ansible-wip.md current
finish: `tools/ci.sh --base main` green · docs/status/tasks/F-sdk-terraform-ansible.md with pasted real output · everything committed · final message = 10-line summary
questions: docs/status/tasks/F-sdk-terraform-ansible-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files/logs/fixtures · edit files you do not own · install packages
