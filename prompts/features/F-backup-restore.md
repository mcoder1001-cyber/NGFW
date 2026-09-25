# Task: F-backup-restore — backup/restore, scheduled export, templates, support bundle, upgrade UI   (prepend 00-CONTEXT.md)

## Goal
Operational safety net around the configuration datastore (WBS D8.5, D8.6, D8.7 in `plan/wbs.csv`): encrypted full backups
(config document + revisions + secrets), restore into the candidate, scheduled export to a target, config templates
(parameterised snippets merged into the candidate), a support bundle (sysdump) with secrets redacted, and the **upgrade UI**
that drives F-ab-upgrade's mechanism. Reference: TNSR "configuration backup/restore" and "sysdump". No agent/VPP work.

## Inputs to read first
- P06 (merged; read on main) under `apps/api/src/`: `datastore/datastore.service.ts` (candidate/running, export/import to candidate),
  `commit/commit.service.ts`, `secrets/secrets.service.ts` (AES-256-GCM under `VRX_SECRET_KEY_FILE`, secret name as AAD, versioned with
  revisions — D-091), `db/schema.ts` (Drizzle tables), `audit/`, `app.ts` (8 MiB JSON body limit). `actions/actions.controller.ts` is the
  generic `POST /actions/:action` bridge (F-vrf-static-ecmp's): serve your actions from static routes in your own controller (a static route
  wins over `:action`) and call the agent through the generic Action stream method in `agent/agent.client.ts`
- `packages/schema` — `redactSecrets(doc)` / `secretPointers()` (D-046, D-070): backups carry secrets only inside the encrypted archive
- `docs/decisions/LOG.md` D-046, D-051, D-059 (have-nots), D-091, **D-097** (every path that writes a snapshot back — restore/import included —
  takes password hashes from `app_user`, never from the snapshot), D-102; `prompts/P10-packaging-deb.md` (paths `/var/lib/vrx`, `/data`);
  `docs/09-os-packages.md` §7 (`/data` holds backups and update bundles); `deploy/upgrade/` + `docs/install/ab-upgrade.md` (F-ab-upgrade,
  merged before you start: the `vrx-upgrade` argv, exit codes and `status --json` you call)
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
   Agent side: new `ActionRequest` members (numbers in `docs/status/wave-BC-numbers.md`), handler code in `apps/agent/internal/actions/backup-restore/`,
   one case in the Action switch under your anchor, one fixed-argv row per binary in `apps/agent/internal/renderers/ALLOWLIST.md` (no user
   string ever reaches argv: the op is an enum, the bundle path is validated to lie under `/data/updates/`). VPP facts for the support bundle
   come from binapi (`show_version`, interface dump) and the agent's `Health`/`Retrieve`; a fixed-string `vppctl`/`cli_inband` use is an
   exception (D-090) — questions file first. Update bundles are GB-sized: never through the 8 MiB JSON body limit — stream the upload to
   `/data/updates/` (the api unit needs write access there: P10's unit, ask in the questions file). Backups/restores are admin-only
   (ADMIN_ONLY list in `apps/api/src/auth/route-guard.test.ts`). OpenAPI; regenerate `packages/api-client`.
3. **Contract** (additive, as `contract(schema)`/`contract(proto)` commits on YOUR task branch — workers never create `contract/` branches):
   `management.backup{…}` and `management.templates` if templates live in the document (alternative: own table — choose, justify in the
   status file). The schema⊆proto drift guard requires a `ManagementConfig` mirror for every non-secret leaf (numbers from wave-BC-numbers.md).
   Tables (schedules, run log, templates if not in the document) = one Drizzle migration generated on top of main, never hand-numbered.
   **Restore and D-097:** the archive's document never brings password hashes back into running — hashes come from `app_user`; restoring
   accounts themselves (`app_user`/API keys) is either an explicit admin option with D-102 reset semantics (sessions, refresh chains and keys
   revoked, audited) or out of scope — choose (default: out of scope, listed), log with options.
4. **UI** `apps/web/src/domains/system/backup-restore/`: Backup & Restore page (download, upload + diff preview before commit), schedules,
   templates editor (SchemaForm for parameters), Support bundle button, Upgrade page (current/other slot version, upload, staged rollout
   status, rollback); en + fa (`locales/*/backup-restore.json`).
5. **Support-bundle collector** `deploy/support-bundle/` — the host-side script list the agent Action may run (fixed argv, no user input), e.g.
   `journalctl -u vrx-* --since`, `vppctl show version/plugins/interface` read-only; allow-listed per `renderers/ALLOWLIST.md` style.
6. **Docs**: `docs/user/system/backup-restore.md` (backup, restore, templates, support bundle, upgrade UI).

Files you own: `apps/api/src/features/backup-restore/**`, `apps/web/src/domains/system/backup-restore/**`, `apps/web/src/locales/*/backup-restore.json`,
`apps/agent/internal/actions/backup-restore/**`, `deploy/support-bundle/**`, `docs/user/system/backup-restore.md`, `test/topology/backup-restore/**`
(the envelope has the full list). Shared files: one-line appends only under your anchor (`app.module.ts`, web router/nav/i18n, the Action
switch in `server.go`, ALLOWLIST rows, route-guard lists, `ManagementConfig`/`ManagementSchema` key lines, P10's install list for the
collector) — the manager resolves at merge. The SFTP test uses your own `sshd -D -f <cfg> -p <slot port>` under `/run/vrx-test/w<SLOT>/`
(your PID only) — never `ssh.service`, which carries everyone's management SSH.

## Acceptance (paste the evidence)
- [ ] Backup → wipe candidate/running in a test DB → restore → commit → document hash identical (pasted); wrong passphrase → 400, nothing staged
- [ ] Archive bytes contain no plaintext secret (grep for the test PSK marker returns nothing); support bundle scan passes and contains no hash/secret
- [ ] Scheduled export to a local SFTP (127.0.0.1, slot port, only if `sshd` test instance can be started without touching the system unit) or to a local directory target; retention prunes
- [ ] Template apply produces the expected candidate diff; injection attempt (newline / `__proto__` key) → 400 with pointer
- [ ] UI screenshot of Backup and Upgrade pages against the real endpoint; `tools/ci.sh --base main` green

## Out of scope (do not build)
The A/B slot mechanism, boot flags and rollback watchdog (F-ab-upgrade); package building/signing (P10, F-vpp-debs, F-hardening-lite); bulk multi-device
provisioning/orchestration (D8.5 fleet part — have-not, list it); cloud storage targets (S3/Azure); HA config sync between peers (F-vrrp-config-sync);
RESTCONF export formats (F-restconf-yang); performing a real upgrade of this host (tests drive a fake `vrx-upgrade` with the same argv, or
F-ab-upgrade's loop-image harness — never `stage/activate` against this host's disks or grubenv).

## Open questions to surface, not to decide silently
Do backups include the audit log (size) — default last 30 days. Should restore of secrets require the original master key (default: no — secrets are
re-encrypted under the passphrase inside the archive).
