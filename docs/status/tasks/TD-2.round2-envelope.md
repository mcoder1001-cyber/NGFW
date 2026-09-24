# TASK ENVELOPE — TD-2 (FIX ROUND 2 (last), host manager 2026-09-24 08:1x)
id: TD-2   branch: task/TD-2   worktree: /root/ngfw-wt/TD-2
FIX ROUND 2 (the last one — after this the manager decides): branch task/TD-2 holds fix round 1 and the verify review ae52906. Keep everything that is fixed; fix only what the verify lists.
read first: docs/status/tasks/TD-2-verify.md (V1 High: API keys created while an admin reset runs survive it — fix with a credential generation column in app_user bumped in the reset transaction, key creation locks that row and re-checks the caller, e2e required; V2 Medium: config-API hash staging bypasses D-097 — implement /root/ngfw/docs/decisions/LOG.md D-102 exactly (admin-reset semantics on promote for every existing user whose hash changed, audit via: config), e2e required; V3 Valkey failure after DB commit, V4 replaceInflightHash before commit, V5 keepApiKeys not audited — fix if cheap, else list them in TD-2.md as left over), docs/status/tasks/TD-2.md. A new DB column is a migration in apps/api — additive only.
merged deps you can rely on: P06, P07b, P13, DF-5
slot: 7 → eval "$(tools/lab env 7)" (VRX_TEST_PREFIX=w7, ports 3700/5700/9171, DB vrx_w7, Valkey db 7)
files you own exclusively: apps/api/** except apps/api/src/state/** (owned by P08) · packages/api-client/** (additive only, D-078) · docs/status/tasks/TD-2*
time box: 3 h
finish by: fix-round-2 section appended to docs/status/tasks/TD-2.md answering every finding with pasted real output (unit + e2e) · tools/ci.sh --base main green · everything committed · vrx_w7 test data dropped · final message = 10-line summary
rules on main to honour: prompts/00-CONTEXT.md; docs/decisions/LOG.md (D-039…D-097); docs/lab/shared-host-rules.md
commit WIP at least every 45 min and keep docs/status/tasks/TD-2-wip.md current — you may be killed by a usage limit at any time and respawned with a CONTINUE envelope
questions: docs/status/tasks/TD-2-questions.md — write and keep going; never wait for a human
never: merge to main · touch /root/ngfw (main) or other worktrees · restart/kill VPP · edit /etc/vpp, startup.conf, vpp.service · Docker · pkill · secrets in files/logs/fixtures · install packages
