# TASK ENVELOPE — F-backup-restore
id: F-backup-restore   branch: task/F-backup-restore   worktree: /root/ngfw-wt/F-backup-restore   base: main@<BASE>   started: <STARTED>
title: S5 system (day 16-18): backup/restore, scheduled export, templates, support bundle, upgrade UI
prompt: prompts/features/F-backup-restore.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main@11a175b)   wbs: D8.5, D8.6, D8.7
scope: encrypted full backup (document + last N revisions + secret-store rows, still encrypted) → restore into the candidate through the normal commit path; scheduled export (local/SFTP/HTTPS); config templates; a redacted support bundle; the upgrade UI that drives F-ab-upgrade's `vrx-upgrade` through two new agent Action members. No VPP configuration work
merged deps you can rely on: P06, P07b, P08, F-ab-upgrade (and through it P10, P14)
  - P06: datastore (`apps/api/src/datastore/`: candidate/running, export/import to candidate), commit engine, secrets service (AES-256-GCM under `VRX_SECRET_KEY_FILE`, the name as AAD, versioned with revisions, admin-only writes — D-091), audit, `db/schema.ts`, the 8 MiB JSON body limit in `app.ts`
  - F-vrf-static-ecmp: the generic Action bridge (`apps/api/src/actions/`) and the generic Action stream method in `apps/api/src/agent/agent.client.ts` — reuse it; your routes are static routes in your own controller
  - P08 + W-seed: the Action type switch in `apps/agent/internal/agent/server.go`, `internal/actions/<slug>/` pattern, the renderers' fixed-argv exec runner + `ALLOWLIST.md`
  - F-ab-upgrade: `deploy/upgrade/vrx-upgrade` (argv, exit codes, `status --json`: your contract, docs/install/ab-upgrade.md), `vrx-upgrade-health.service`, bundle format + signature check · P10: `/data/{backups,updates,support}`, the api/agent units · P14: partition layout
  - also on main by then: TD-2/TD-4 (D-097/D-102 account semantics, `route-guard.test.ts`)
read first: prompts/features/F-backup-restore.md · docs/status/wave-BC-numbers.md (section "S5 system": pack rules SY1–SY9 + "F-backup-restore") · docs/status/wave-A-hotspots.md (§0 rules; A4 C1–C7 P1 P4 P5 W1–W3) · docs/install/ab-upgrade.md · docs/install/bare-metal.md (P10) · apps/agent/internal/renderers/README.md + ALLOWLIST.md · docs/decisions/LOG.md D-046, D-051, D-059, D-063, D-090, D-091, D-097, D-102
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - test SFTP target: your own `/usr/sbin/sshd -D -f /run/vrx-test/w<SLOT>/sshd/sshd_config -p 3000+100·<SLOT>+82` (host keys generated there, 0600, your PID only); HTTPS target fixture on +83; local-directory target under /run/vrx-test/w<SLOT>/backups
  - "/data" in tests = /run/vrx-test/w<SLOT>/data (configurable path; never the real /data)
  - slots 1–11 only; 12 is CI
daemon-owner: none (the test sshd is a fixture; `ssh.service` carries everyone's management SSH — never touch it)
obligations:
  - D-046/D-051/D-091: secrets leave the box only inside the passphrase-encrypted archive (scrypt/argon2 KDF + AES-256-GCM via node:crypto, no shell). The archive bytes contain no plaintext secret (grep for the `VRX_TEST_PSK_FBR_*` marker). The support bundle runs `redactSecrets` plus a final regex scan that fails the bundle on a hit
  - D-097/D-102: restore and import never bring password hashes back from the archive; hashes come from `app_user`. Restoring accounts (app_user rows, API keys, F-aaa's TOTP table) is out of scope by default (listed as a have-not); if you build it, it is an explicit admin option with D-102 reset semantics. Log the choice with options
  - restore lands in the candidate and goes through validate → commit/confirm; it never writes running directly. A schema major version it cannot parse → refuse; older minor → migrate via `RootConfig.parse`
  - agent Action (numbers below): the op is an enum and the bundle path is validated to lie under `/data/updates/`, so no user string reaches argv (rule 9). One ALLOWLIST row per binary (`vrx-upgrade`, `vrx-support-collect`). Recommended default for the upgrade exec: the agent starts fixed-name units (`vrx-upgrade@<op>.service`) instead of exec'ing inside its own sandbox, because P10/F-hardening-lite's `ProtectSystem=strict` would block mounts/GRUB writes. Decide with P10's units in hand, log with options
  - support bundle: VPP facts via binapi only (`show_version`, interface dump) and the agent's `Health`/`Retrieve`. A fixed-string `vppctl`/`cli_inband` is an exception (D-090): questions file first. `journalctl` with fixed argv; `--since` is rendered from a validated timestamp, never passed as a string
  - update bundles are GB-sized: stream the upload to `/data/updates/`, never through the 8 MiB JSON body limit. Register the streaming parser from your own module (SY2)
  - never upgrade this host: tests drive a fake `vrx-upgrade` with the same argv, or F-ab-upgrade's loop-image harness
  - backups/restores/support bundles/upgrade actions are admin-only (SY1), audited, and rate-limited by the existing guard
files you own exclusively:
  - apps/api/src/features/backup-restore/** (index.ts exports {controllers, providers}; `BackupRestoreController`; scheduler provider; template renderer; bundle builder; real fake-agent behaviour in fake.ts)
  - apps/api/test/e2e/backup-restore*
  - apps/agent/internal/actions/backup-restore/** (upgrade + support-bundle handlers: pure functions + fake-runner unit tests)
  - packages/schema/src/domains/ext/backup-restore*.ts, packages/schema/src/semantic/backup-restore*.ts (rule ids `management.backup-restore-…`), packages/schema/examples/backup-restore-*.json, packages/proto/test/fixtures/backup-restore-*.json
  - apps/web/src/domains/system/backup-restore/** (Backup & Restore, schedules, templates editor, support bundle, Upgrade page), apps/web/src/locales/{en,fa}/backup-restore.json
  - deploy/support-bundle/** (the host-side collector `vrx-support-collect`, fixed argv, + its tests)
  - docs/user/system/backup-restore.md, test/topology/backup-restore/**, docs/status/tasks/F-backup-restore*
shared hotspots (append-only, conflicts resolved by the manager at merge; ids from docs/status/wave-A-hotspots.md §1 and docs/status/wave-BC-numbers.md "S5 system"):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-BC: F-backup-restore` (`#` in shell/debian files; the manager seeds it before spawn; if absent, at the end of the block). Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-backup-restore.md
  - SY4 packages/schema/src/domains/management.ts: `ManagementSchema` key lines `backup` (+ `templates` if in the document) under your anchor; sub-schemas in ext/backup-restore.ts · C2 semantic/index.ts one spread · C3 schema/src/index.ts one export if needed · C4 new fixture files only
  - C5 packages/proto/vrx/v1/dataplane.proto: `ManagementConfig` 7/8, `ActionRequest` 20/21 (+ `ActionOutput` 4 only if used) under their anchors; messages in a `// ----- F-backup-restore -----` section at the end · C6 docs/contracts/proto.md: `### F-backup-restore: ActionRequest.upgrade, support_bundle` under "Feature RPCs"
  - C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md
  - A4 apps/agent/internal/agent/server.go: the `upgrade` and `support_bundle` cases in the Action switch (one call each into internal/actions/backup-restore)
  - SY5 apps/agent/internal/renderers/ALLOWLIST.md: rows for `vrx-upgrade` (or `systemctl start vrx-upgrade@<op>`) and `vrx-support-collect`
  - SY1 apps/api/src/auth/route-guard.test.ts: ADMIN_ONLY += your action routes · SY2 apps/api/src/app.ts: only if the streaming upload parser cannot be registered from your module
  - SY3 apps/api/src/db/schema.ts + apps/api/migrations/**: tables under the anchor; `pnpm -C apps/api db:generate --name f_backup_restore`; regenerated on top of main at merge
  - SY6 deploy/debian/vrx/** (P10's install list): one line shipping `deploy/support-bundle/` under `# wave-BC: F-backup-restore`
  - SY8 pnpm-lock.yaml: e.g. `ssh2` (SFTP), a cron parser; questions file first
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts (only if the generic Action stream method is missing) · P5 apps/api/src/testing/fake-agent.ts (wire fake.ts's Action behaviour for the two new members under the anchor)
  - W1 apps/web/src/router.tsx · W2 apps/web/src/nav/nav.ts (+ nav.test.ts): non-domain system NavItems `backup-restore` and `upgrade` · W3 apps/web/src/i18n.ts
contract: `contract(schema): management backup/templates` and `contract(proto): backup mirror + upgrade/support-bundle actions` as separate commits on YOUR branch first (the ci.sh contract guard checks the subject), plus docs/status/tasks/F-backup-restore-contract.md. Tell the manager in the questions file and keep building. No own branches
contract numbers: **ManagementConfig 7 `backup`, 8 `templates` (only if templates live in the document) · ActionRequest 20 `upgrade`, 21 `support_bundle` · ActionOutput 4 `file_chunk` (only if the archive streams through the Action)** (docs/status/wave-BC-numbers.md "F-backup-restore"; reusing one is a merge blocker; 22–24 are the pack's spare, ask before using one)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/ssh), /boot, /data on this host, /root/vpp
  - deploy/upgrade/** (F-ab-upgrade — read-only; a change → questions file), deploy/debian/** beyond your anchor line, deploy/systemd/** (P10), deploy/image/** (P14/F-images)
  - apps/api/src/{secrets,commit,datastore,config,auth,users}/** (use the services; a gap → questions file), apps/api/src/actions/** (F-vrf-static-ecmp)
  - apps/agent/internal/agent/{agent,service,state,ifstate}.go (A5), apps/agent/cmd/**, apps/agent/internal/renderers/** beyond the ALLOWLIST rows, apps/agent/binapi (manager-owned)
  - packages/ui-kit/**, tools/ci.sh, tools/lab, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md
host facts:
  - sshd (OpenSSH 10.2) is present for a slot test instance; `age` is absent (use node:crypto); zstd is present (Node 22.23 has zlib zstd, or gzip)
  - the support-bundle and Action host tests talk to the shared VPP only read-only through your slot agent: check `systemctl show vpp -p NRestarts` before/after every stack run; hold `flock -s` on the lab lock only during a run (D-094)
coordination:
  - F-ab-upgrade (merged dep): the argv/exit-code/JSON contract; a mismatch → questions file, never a local fork of deploy/upgrade
  - P10 / F-hardening-lite: which unit runs `vrx-upgrade` and which `ReadWritePaths` the api unit has for `/data/*`. Record what you relied on
  - F-aaa: accounts and TOTP are not restored by default · F-restconf-yang: no RESTCONF export format here
evidence: backup → wipe (test DB) → restore → commit → document hash identical; wrong passphrase → 400 with nothing staged; archive grep; bundle scan; scheduled export + retention (local dir, and SFTP on the slot port); template diff + injection → 400 with `pointer`; upgrade Action against the fake `vrx-upgrade` (argv log pasted). Playwright is not installed: screenshots (Backup and Upgrade pages, en + fa/RTL) with the headless Chrome approach from P07a/P07b/P08, kept outside the product code, and say so
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-backup-restore.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-backup-restore-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-backup-restore.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite/sshd/HTTPS fixture), by PID · lab lock released · vrx_w<SLOT> dropped · /run/vrx-test/w<SLOT>/{sshd,data,backups} removed (host keys included) · no archive or bundle left outside your worktree's `.scratch/` · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-backup-restore-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
