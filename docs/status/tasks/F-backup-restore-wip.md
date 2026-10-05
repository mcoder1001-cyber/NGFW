# F-backup-restore work in progress

- Worker: /root/backup_restore, active 2026-10-05.
- Branch: codex/f-backup-restore-20261005
- Worktree: /root/ngfw-wt/f-backup-restore-20261005
- Base: origin/main d314f0728
- Slot: 26, prefix w26; no lab process started.
- Owned files: task-prompt feature directories, additive schema/proto contracts, anchored API/UI/agent wiring, generated contract output, feature tests and docs.
- Completed: scope and existing ownership inspected; no remote backup branch exists.
- Actual tests: none yet.
- Remaining: contracts, encrypted backups with candidate restore, schedules, templates, support, upgrade actions/UI, tests, quick gate, independent review.
- Current failure: none.
- Exact next command: inspect schema/proto conventions and implement additive contract.
- Local SHA: d314f0728; publication: initial worktree only, not published yet.

2026-10-05 checkpoint: contracts local151983a38 remote7f5e57174722dc32d55d0500219badbfa3d5792b exact tree verified by manager. Additional contract correction patchJson in progress (arbitrary JSON needs scalar mirror). Implemented encrypted bounded format, document+revision+secret/audit snapshot, candidate staging with inactive encrypted secret versions, normal commit pin integration and per-ref concurrent put locking; templates endpoints; local/SFTP pinned-key/HTTPS scheduling with durable minute claims; support export and fixed collector; streamed8GiB upgrade upload and dedicated root systemd enum units preserve agent sandbox. API-unit tests archive2 and auth routes8 pass. Initial contract Go drift reports arbitrary patch map vs Struct; corrected internal patchJson string currently generating. Complete quick gate not run/passed. UI separate developer remote546116dbb46b3d868802fb109414b927c6cdd54b not yet integrated. Remaining: tests, generated client, independent reviews, lab acceptance or explicit deferral, publication. Exact next command: pnpm gen, followed focused tests and tools/ci.sh --base origin/main.

Final backend hardening checkpoint: contract/generated locald037a0f70 remotely published6d86aed553b7792053e7b0aa4f096b476b1c6cd6. PostgreSQL e2e4/4 pass includes wiped running recovery SHA256ad27929269906644d14d795f096a6dd441780cbe241acf04821e88df696974e4, wrong passphrase400/no staging, inactive secret/live version safety, discard, confirm promotion, current accounts preserved, write-ahead audit refusal503 prevents restore/upgrade mutation, durable scheduled local run+retention, template injection/diff. Last generation13/13 pass; focused archive/templates5/5 pass; Go contract/agent pass; fixed upgrade action2tests pass. Single generated migration0010 contains both additions. Added database-side cumulative size metadata preflight before JSON/ciphertext selection; oversized-history test newly added, pending next e2e run. Remaining full quick gate/independent final review, combined UI integration/screenshots, dedicated SFTP/HTTPS transport fixture tests. Exact next command: tools/ci.sh --base origin/main in isolated backend worktree (manager also combines frontend and gates latestmain). No fullgate pass claimed.
