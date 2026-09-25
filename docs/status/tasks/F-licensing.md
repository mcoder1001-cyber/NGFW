# F-licensing — simple licence and entitlement enforcement

Branch `task/F-licensing` (base be53867). Cloud container run: no PostgreSQL, Valkey, VPP, agent or systemd.

## What was built
- **Format + CLI** `tools/license/`: `.vrxlic` = `{format:"vrxlic/1", license:{version, licenseId, customer, issuedAt,
  notBefore, expiresAt, binding{machineIdHash?, serial?}, entitlements{features[], limits{}}}, signature}`; detached
  Ed25519 over canonical JSON (keys sorted recursively). `vrx-license` (`vrx-license.mjs`, node:crypto only, shell shim):
  `keygen` (refuses to write inside a git repo, key 0600, prints paths), `issue`, `verify`, `inspect`. Canonical JSON is
  duplicated in `apps/api/src/features/licensing/format.ts`; `cli.test.ts` proves the API verifies what the CLI signs.
- **API** `apps/api/src/features/licensing/`: `PUT /api/v1/system/license` (`@MinRole('admin')`, audited via the global
  interceptor, `req.audit` = licenseId/status/expiresAt only), `GET /api/v1/state/license` (status community / valid /
  grace / expired / invalid, entitlements in force, days left, customer name; never signature or binding values).
  Validation stage `license` in `commit/validation.service.ts` between secret-existence and agent DryRun → 403
  problem+json `type …/license-required`, `tier: "license"`, `errors[].pointer` = first offending node. Grace 30 days
  (warning in `warnings[]`, commit proceeds). No licence / invalid / expired past grace = `COMMUNITY` (one table,
  `entitlements.ts`, marked SAMPLE). **Grandfathering**: a node already in running never offends — expiry never forces
  removal of running config. Expiry: `system_event` subsystem `licensing`, codes `license.grace` / `license.expired`
  (on transition; hourly timer + every status read). Licence stored as a file (`VRX_LICENSE_FILE`, default
  `/var/lib/vrx/license.vrxlic`, atomic write 0640) — no DB table/migration. Public key embedded in
  `licensing.config.ts` (placeholder; see questions); `VRX_LICENSE_PUBKEY_FILE` adds a dev/test key.
- **UI** `apps/web/src/domains/system/licensing/`: page (status chip, details, entitlement table, upload for admins),
  `LicenseBanner` in the shell (grace / expired / invalid; silent on errors), en + fa `locales/*/licensing.json`,
  nav item System → Licence, route `/system/licensing`.
- **Docs** `docs/user/system/licensing.md`. OpenAPI → `packages/api-client` regenerated; `make -C apps/cli gen docs`
  (operations_gen.go +2 ops; reference.md unchanged).

## Review fixes (coordinator BLOCK)
1. `COMMUNITY` is now permissive (every gated feature, no limits); the restrictive set is `SAMPLE_COMMUNITY` (tests via
   `LicensingOptions.community`, docs table). `docs/decisions/PENDING-licensing-matrix.md` (matrix + real signing key;
   nothing parked).
2. Unreadable / non-verifying stored licence: `Logger.warn` with path + error class only (no content/signature);
   state `invalid` with reason (test asserts the log has no signature).
3. `wiring.test.ts`: offline AppModule — `ValidationService.licensing` is the `LicensingService` singleton.
4. LOG.md D-142 (file storage), D-143 (403), D-144 (grandfathering), D-145 (invalid → community fallback).

Re-run: api `tsc` ok, eslint 0 errors, `Test Files 14 passed (14) · Tests 126 passed (126)` (licensing 24);
web `tsc` ok, eslint 0 errors, `Test Files 15 passed (15) · Tests 102 passed (102)`;
`tools/ci.sh check --base be53867` → `check PASSED`.

## Shared hunks
- `apps/api/src/app.module.ts` — under the three `// wave-BC: F-licensing` anchors (import, controllers, providers).
- `apps/web/src/i18n.ts` — under the four anchors.
- **Unanchored** (no F-licensing anchor exists; appended at the block end, each line marked `wave-BC: F-licensing (unanchored)`):
  `apps/api/src/commit/validation.service.ts` (tier union, import, `@Optional()` ctor param, stage call, warnings merge),
  `apps/web/src/router.tsx`, `apps/web/src/nav/nav.ts` + `nav.test.ts`, `apps/web/src/shell/AppShell.tsx`.
- Generated: `packages/api-client/src/generated/schema.d.ts`, `apps/cli/internal/api/operations_gen.go` (contract commit
  `contract(api-client): …` + `F-licensing-contract.md`).

## Verification (pasted)
API unit (`npx vitest run src/features/licensing`):
```
 ✓ src/features/licensing/cli.test.ts (4 tests) 188ms
 ✓ src/features/licensing/licensing.test.ts (17 tests) 166ms
      Tests  21 passed (21)
```
Covers: valid, tampered byte (payload and signature), wrong key, malformed, not-yet-valid, grace (20 days left),
expired; serial + machine-id binding match/mismatch; predicates on `packages/schema/examples` (ha-vrrp → `/ha/vrrp/lan-v4`,
vpn-site-to-site → first tunnel + `ipsecTunnels` limit, minimal → none); grandfathering; ValidationService rejects with
403 + pointer and DryRun not called, licensed → DryRun called; grace → ok + `license.grace` warning + system_event;
past grace → new VRRP instance rejected (`/ha/vrrp/customer-a`) while the running document itself still validates;
RBAC metadata (upload admin); HTTP (Nest+Fastify inject): upload → `GET` valid + entitlements, signature absent from the
body; tampered → 400 `application/problem+json` "licence signature is invalid"; CLI keygen/issue/verify/inspect in-process.

Full suites: `apps/api` `Test Files 13 passed (13) · Tests 123 passed (123)`; `apps/web` `Test Files 15 passed (15) ·
Tests 102 passed (102)` (incl. LicensingPage.test.tsx: status/entitlements/upload, 400 problem shown, readonly has no
upload, grace banner en + fa). eslint api/web: 0 errors; `tsc` api/web clean; `pnpm -C apps/api build` ok;
`pnpm -C apps/web build`: `bundle-budget: OK`, `check-no-dev-routes: OK`; api-client lint/typecheck/test ok;
`go vet`/`go test` apps/cli ok.

CLI transcript (temp dir shown as `<tmp>`):
```
signing key: <tmp>/keys/vrx-license-signing.pem
public key:  <tmp>/keys/vrx-license-public.pem
vrx-license: refusing to write a signing key inside a git repository (/home/user/wt/F-licensing/apps/x)
issued LIC-DEMO-1 → <tmp>/a.vrxlic
signature OK; status valid (licence LIC-DEMO-1, expires 2027-09-25T09:04:39.898Z)
INVALID: signature does not verify        (one byte of the customer changed)
exit 1
```
No private key committed: `git grep -n "PRIVATE KEY" -- tools/license apps/api/src/features apps/web/src/domains/system docs/user/system`
→ no match (exit 1); repo-wide hits are pre-existing test fixtures of other tasks. Test keys are generated in memory or
under `mkdtemp(os.tmpdir())` and deleted in `afterAll`. The service logs only `licence <id> installed (<status>)`.

CI: `TMPDIR=/tmp/g-flic tools/ci.sh check --base be53867` → `check PASSED` (contract guard ok, `ok: no secret-shaped
strings`, `gitleaks … no leaks found`). `tools/ci.sh --base be53867` (quick) **fails in this container before any of my
code runs**: `'pnpm gen' failed — @ngfw/schema#gen: unable to spawn child process: Exec format error (os error 8)` (turbo
cannot spawn tasks here; same failure in another worker's log). Local `main` is a stub commit (`7b2f438 first`), so
`--base main` is meaningless here; be53867 (board tip) was used.

## NOT done here (owed on the lab host)
- UI screenshot against the real endpoint (headless Chrome) — no API stack (PG/Valkey) in this container.
- "Running config stays applied": agent Retrieve before/after expiry on the slot agent, `NRestarts` — no VPP/agent here.
  By construction the licence code never calls the agent (D-040) and only runs inside validation; covered at unit level.
- `apps/api/test/e2e/licensing*` e2e (needs PG) — not written.
- `tools/ci.sh --base main` green (turbo spawn failure above).

## Out of scope (not built)
Online activation / call-home, dongles/TPM, VPP rate limiting (no agent/VPP work), billing, tamper-resistance, CLI-side
enforcement, VDOM licences.

## Obligations
D-040: no licence data to the agent (no proto/schema change; stage runs before DryRun; grandfathering). Secrets rule 10:
no private key in repo/fixtures/logs; signature never returned or logged. D-046 minimisation: GET returns name, status,
entitlements, days left, `bound` booleans only. RBAC: upload admin-only; audited. RFC 9457: 403 with pointer; tampered 400.

## Decisions taken (logged D-142..D-145)
- Licence storage: file vs PostgreSQL table → **file** (no migration conflicts with F-dashboard/F-aaa; single blob).
- Rejection status: 403 vs 422 → **403** `license-required` (authorisation-like; 422 is the apply-failure code here).
- Grandfathering against the running revision (vs rejecting any unlicensed node) → so expiry never forces removal.
- Expired-past-grace and invalid licences fall back to the community set (not "reject everything").

Open questions: `docs/status/tasks/F-licensing-questions.md`.
