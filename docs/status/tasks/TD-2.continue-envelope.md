# TASK ENVELOPE — TD-2 (CONTINUE, respawned by host manager cycle 12, 2026-09-24 07:2x)
id: TD-2   branch: task/TD-2   worktree: /root/ngfw-wt/TD-2
CONTINUE: the previous worker died mid fix round 1 at ~05:45. Its uncommitted edits are salvaged as the top commit 8dde3e4 ("wip(TD-2): salvage …") — review that diff first, keep what is right, finish the fix round; do not restart.
read first: docs/status/tasks/TD-2-review.md (APPROVE WITH CHANGES cc70629: H1 reconcile restores old password hash, H2 refresh survives reset, M1 keys survive reset → D-097, M2 current-guessing), docs/status/tasks/TD-2.md, docs/decisions/LOG.md D-097 (password lifecycle — implement exactly: hashes only from app_user on every snapshot write-back path; admin reset revokes sessions, refresh chains AND API keys unless keepApiKeys:true (audited); self-change keeps keys; wrong current counts toward lockout)
merged deps you can rely on: P06, P07b, P13, DF-5
slot: 7 → eval "$(tools/lab env 7)" (VRX_TEST_PREFIX=w7, ports 3700/5700/9171, DB vrx_w7, Valkey db 7)
files you own exclusively: apps/api/** except apps/api/src/state/** (owned by P08) · packages/api-client/** (additive only, D-078) · docs/status/tasks/TD-2*
time box: 4 h
finish by: fix-round section appended to docs/status/tasks/TD-2.md answering every finding with pasted real output (unit + e2e) · tools/ci.sh --base main green · everything committed · vrx_w7 test data dropped · final message = 10-line summary
rules on main to honour: prompts/00-CONTEXT.md; docs/decisions/LOG.md (D-039…D-097); docs/lab/shared-host-rules.md
commit WIP at least every 45 min and keep docs/status/tasks/TD-2-wip.md current — you may be killed by a usage limit at any time and respawned with a CONTINUE envelope
questions: docs/status/tasks/TD-2-questions.md — write and keep going; never wait for a human
never: merge to main · touch /root/ngfw (main) or other worktrees · restart/kill VPP · edit /etc/vpp, startup.conf, vpp.service · Docker · pkill · secrets in files/logs/fixtures · install packages
