# F-management-ui: Management screen (users, AAA, API TLS, remote syslog) + management.tls apply

Branch `claude/modest-keller-upaw4m` (cloud session modest-keller). D-152, WBS D0.9 / D0.10.
Contract: **additive, generated only** (`contract(F-management-ui)` commit). The new `GET /api/v1/state/management/tls`
route regenerates the api-client types and CLI opgen (`MgmtTls_state`). There is no schema or proto change.

## What was built
| layer | what |
|---|---|
| API `features/mgmt-tls` | `validator.ts`: PEM chain + key parse, key matches leaf (`checkPrivateKey`), validity window, `createSecureContext` with `minVersion`. Issues carry pointers `/management/tls/certificateRef` / `privateKeyRef` and rules `management.tls.*`. Messages never quote key material. `MgmtTlsService`: resolves `cert/…` and `key/…` refs from the secret store (current version, decrypted via SecretsService, loaded lazily to avoid the Secrets→Commit→Validation cycle). `validate(doc)` is called by `ValidationService` in the semantic tier, so a bad pair is a 400 before commit. It subscribes to `commit.events` (`applied`/`confirmed`/`reverted`), re-reads running and hot-swaps via `tls.Server#setSecureContext`. A bad pair keeps the previous certificate and records the error. Optional HTTPS listener when `NGFW_HTTPS_PORT` is set (fastify `routing` handler, same host as `NGFW_HTTP_HOST`). `GET /api/v1/state/management/tls` (any role) returns subject, issuer, SANs, validity, days left, SHA-256 fingerprint, loaded revision, listener, error, and never the key |
| shared lines (unanchored) | `app.module.ts` import + controllers + providers. `commit/validation.service.ts`: optional injection + 3-line semantic check (same pattern as F-licensing). `router.tsx`: `/system/management` route, `/system/users` → `Navigate` to `?tab=users`. `nav.ts` `BUILT_DOMAINS += 'management'`. `i18n.ts` namespace |
| web `domains/system/management` | `ManagementPage` = `DomainTabsPage` + `tabs.ts`: **Users** (the existing `UsersPage`), **AAA** (honest "not yet available": F-aaa not built), **API TLS** (`TlsTab`: SchemaForm of `management.tls`, replace-patch with `null` for cleared refs, uncommitted chip, pointer → field, loaded-certificate panel, "no HTTPS listener" note, load-error alert), **Remote syslog** (the existing Services › Logging tab, which already edits `management.syslog`). en + fa `management.json` |
| docs | `docs/user/system/management.md` |

## Tests run here (cloud sandbox: no PostgreSQL, no lab slot)
- API `tsc --noEmit` ok. Unit vitest 48 files / 294 tests ok, including the new `mgmt-tls.test.ts` (9 tests). Certificates are generated per run with `openssl`, so no key is committed. The tests cover: matching pair, mismatched key (pointer + key text absent), expired, not yet valid, garbage PEM, and a **real TLS handshake**: server started on cert A / 1.2, then `setSecureContext(cert B, 1.3)`, after which a 1.2 client is refused and a 1.3 client gets CN=b. Service tests cover `validate()` pointers, hot swap on a `commit.events` publish, a bad pair keeping the previous certificate, and the no-refs state. The route-guard test passes unchanged (the GET route is guarded, any role). **e2e (PostgreSQL) not run.**
- Finding while testing: a context handed out by `SNICallback` does **not** apply its `minVersion` (the handshake used TLS 1.2). The service therefore swaps the server's default context (`setSecureContext`).
- Web: `tsc`, eslint, logical-CSS check ok. Full vitest 73 files / 461 tests ok. New: `ManagementPage.test.tsx` (nav available, 4 tabs + AAA not-available, `/system/users` redirect, TLS panel + PATCH body, pointer → key field, load error) and the locale parity test. Changed because behaviour changed: `nav.test.ts` gains `'management'`. `App.test.tsx`'s "not yet available" example moves from `/system/management` to `/system/ha` (same assertions).
- cli `go test ./...` ok (opgen regenerated).

## Not tested / open
- **Acceptance evidence not produced**: no `openssl s_client` against a running API, no 400 through the real commit route (needs PostgreSQL), no screenshot, and `tools/ci.sh --base main` was not run (turbo cannot spawn here).
- The HTTPS listener is **opt-in** (`NGFW_HTTPS_PORT`, read directly from the process environment and not added to `config.ts`). Without a configured certificate it does not start: **no self-signed first-boot certificate** is generated (Node.js cannot mint X.509 certificates). WebSocket upgrades over the HTTPS listener are not wired (`/api/v1/stream` stays on the HTTP port).
- The secret's **current** version is used, not the version a revision pinned. After a rollback that re-activates older secret versions this is the same value. Replacing a secret without a commit is only picked up at the next commit event.
- Removing `management.tls` refs drops the context for future starts, but a running listener keeps its last certificate until restart.
- Open question (from the prompt): if the API sits behind the tools/app vite proxy or a P10 front end, the certificate should arguably apply there, not on the API. Coordinate with P10 / PENDING-tools-app-transport.
- AAA tab is a placeholder by design. F-aaa replaces its entry in `tabs.ts`.
