# Task: F-management-ui — Management screen (users, AAA, API TLS, remote syslog) + management.tls apply   (prepend 00-CONTEXT.md)

## Goal
Give the `management` schema domain one screen (WBS D0.9, D0.10, D-152). Its parts are owned by different rows: local users (P07b/TD-4,
page `/system/users`), remote syslog (F-unbound-chrony-syslog), AAA (F-aaa, todo). No row owns the page and nothing applies
`management.tls` today (no reader in `apps/api/src`). The Management nav entry shows "soon".

## Inputs to read first
- `packages/schema/src/domains/management.ts` — `users[]`, `aaa`, `tls {certificateRef, privateKeyRef, minVersion}`, `syslog[]`.
- `apps/web/src/domains/DomainTabsPage.tsx`, `apps/web/src/domains/services/{ServicesPage,tabs}.tsx` — the tab-shell pattern to copy.
- `apps/web/src/pages/UsersPage.tsx`, `apps/api/src/users/**`, `apps/api/src/secrets/**`, D-040, D-046, D-091, D-100, D-102,
  `docs/decisions/PENDING-tools-app-transport.md` (the web UI is served over plain HTTP on :8080 today).

## Contract changes
None expected.

## Scope — build exactly this
1. **UI shell** `apps/web/src/domains/system/management/`: `ManagementPage` = `DomainTabsPage` with `tabs.ts`; route under a
   `// F-management-ui` anchor in `router.tsx`, `management` added to `BUILT_DOMAINS`. Tabs: **Users** (reuse `UsersPage` content, keep
   `/system/users` working as a redirect), **Remote syslog** (SchemaForm over `/management/syslog`), **API TLS**. F-aaa adds its **AAA** tab
   later with one line in `tabs.ts`.
2. **API TLS** `apps/api/src/features/mgmt-tls/`: on commit of `management.tls`, resolve the certificate/key refs from the secret store,
   validate (key matches cert, not expired, minVersion) and hot-reload the API's TLS context; `GET /api/v1/state/management/tls` shows the
   active certificate (subject, SANs, expiry — never the key). A bad certificate fails validation before commit.
3. **Tests**: unit tests for the TLS validator (mismatched key, expired cert, 1.3-only), route-guard lists updated, UI tab tests.
4. **Docs**: `docs/user/system/management.md`.

## Acceptance (paste the evidence)
- [ ] Commit a new certificate → `openssl s_client` against the API shows it without a restart (pasted)
- [ ] Mismatched key → 400 problem+json with a `pointer`; the key never appears in logs, audit rows or GET responses (grep)
- [ ] Screenshot of the Management screen (3 tabs) against the real endpoint; nav entry no longer shows "soon"
- [ ] `tools/ci.sh --base main` green

## Out of scope (do not build)
AAA backends and MFA (F-aaa), the log explorer (F-unbound-chrony-syslog), fronting :8080 with TLS (PENDING-tools-app-transport / P10).

## Open questions to surface, not to decide silently
- If the API runs behind the vite preview proxy (tools/app), where does the certificate apply — API or proxy? Coordinate with P10.
