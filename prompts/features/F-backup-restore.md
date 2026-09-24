# Task: F-backup-restore — backup/restore, scheduled export, templates, support bundle, upgrade UI   (prepend 00-CONTEXT.md)

## Goal
Operational safety net around the configuration datastore (WBS D8.5, D8.6, D8.7 in `plan/wbs.csv`): encrypted full backups
(config document + revisions + secrets), restore into the candidate, scheduled export to a target, config templates
(parameterised snippets merged into the candidate), a support bundle (sysdump) with secrets redacted, and the **upgrade UI**
that drives F-ab-upgrade's mechanism. Reference: TNSR "configuration backup/restore" and "sysdump". No agent/VPP work.

## Inputs to read first
- P06 on `task/P06` (read only via `git show`): `datastore/datastore.service.ts` (candidate/running, export/import to candidate),
  `commit/commit.service.ts`, `secrets/secrets.service.ts` (AES-256-GCM under `VRX_SECRET_KEY_FILE`), `db/schema.ts` (Drizzle tables),
  `audit/`, `actions/actions.controller.ts`
- `packages/schema` — `redactSecrets(doc)` / `secretPointers()` (D-046, D-070): backups carry secrets only inside the encrypted archive
- `docs/decisions/LOG.md` D-046, D-051, D-059 (have-nots); `prompts/P10-packaging-deb.md` (paths `/var/lib/vrx`, `/data`); `docs/09-os-packages.md` §7
  (`/data` holds backups and update bundles); `prompts/features/F-ab-upgrade.md` (the mechanism you call; it may not be merged yet)
- P07b UI flows (pending-change bar, commit dialog) — restore and template apply land in the candidate and go through the normal commit

## Scope — build exactly this
1. **Backup format**: `vrx-backup-<host>-<ts>.tar.zst.age`-style archive = `manifest.json` (schema version, revision id, hash, created-by) +
   running document + last N revisions + secret-store rows (still encrypted) ; the archive itself encrypted with a **user passphrase**
   (scrypt/argon2 KDF + AES-256-GCM via Node crypto, no shell). Restore refuses a schema major version it cannot parse; older minor → migrate via P02 parse.
2. **API** `apps/api/src/features/backup-restore/`: `POST /api/v1/actions/backup` (stream download), `POST /api/v1/actions/restore` (upload →
   candidate + secrets staged, then the normal commit/confirm path; never writes running directly), scheduled export
   (`management.backup{schedule(cron), target: local|sftp|https, retention}` — contract below; SFTP via `ssh2`, key/password by secret ref),
   templates (`GET/PUT /api/v1/config-templates/{name}`, apply = render parameters → merge-patch into candidate, validated per D-049),
   `POST /api/v1/actions/support-bundle` (redacted running config, last 1000 audit rows, agent `Health`/`Retrieve` summaries, service
   status from the agent, versions; secrets and hashes removed by `redactSecrets` + a final regex scan that fails the bundle on a hit),
   upgrade UI endpoints that call F-ab-upgrade's `vrx-upgrade` CLI **through the agent Action** (list slots, stage bundle, activate, confirm, rollback).
   OpenAPI; regenerate `packages/api-client`.
3. **Contract** (`contract/F-backup-restore`, additive): `management.backup{…}` and `management.templates` if templates live in the document
   (alternative: own table — choose, justify in the status file).
4. **UI** `apps/web/src/domains/system/backup-restore/`: Backup & Restore page (download, upload + diff preview before commit), schedules,
   templates editor (SchemaForm for parameters), Support bundle button, Upgrade page (current/other slot version, upload, staged rollout
   status, rollback); en + fa (`locales/*/backup-restore.json`).
5. **Support-bundle collector** `deploy/support-bundle/` — the host-side script list the agent Action may run (fixed argv, no user input), e.g.
   `journalctl -u vrx-* --since`, `vppctl show version/plugins/interface` read-only; allow-listed per `renderers/ALLOWLIST.md` style.
6. **Docs**: `docs/user/system/backup-restore.md` (backup, restore, templates, support bundle, upgrade UI).

Files you own: `apps/api/src/features/backup-restore/**`, `apps/web/src/domains/system/backup-restore/**`, `apps/web/src/locales/*/backup-restore.json`,
`deploy/support-bundle/**`, `docs/user/system/backup-restore.md`, `test/topology/backup-restore/**`. Shared files: one-line appends only
(`app.module.ts`, web router/nav, agent action registration) — the manager resolves at merge.

## Acceptance (paste the evidence)
- [ ] Backup → wipe candidate/running in a test DB → restore → commit → document hash identical (pasted); wrong passphrase → 400, nothing staged
- [ ] Archive bytes contain no plaintext secret (grep for the test PSK marker returns nothing); support bundle scan passes and contains no hash/secret
- [ ] Scheduled export to a local SFTP (127.0.0.1, slot port, only if `sshd` test instance can be started without touching the system unit) or to a local directory target; retention prunes
- [ ] Template apply produces the expected candidate diff; injection attempt (newline / `__proto__` key) → 400 with pointer
- [ ] UI screenshot of Backup and Upgrade pages against the real endpoint; `tools/ci.sh --base main` green

## Out of scope (do not build)
The A/B slot mechanism, boot flags and rollback watchdog (F-ab-upgrade); package building/signing (P10, F-vpp-debs, F-hardening-lite); bulk multi-device
provisioning/orchestration (D8.5 fleet part — have-not, list it); cloud storage targets (S3/Azure); HA config sync between peers (F-vrrp-config-sync);
RESTCONF export formats (F-restconf-yang); performing a real upgrade of this host.

## Open questions to surface, not to decide silently
Do backups include the audit log (size) — default last 30 days. Should restore of secrets require the original master key (default: no — secrets are
re-encrypted under the passphrase inside the archive).
