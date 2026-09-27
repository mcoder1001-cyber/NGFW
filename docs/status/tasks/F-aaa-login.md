# F-aaa-login — AAA increment 2: external login + MFA enforcement + LDAP/OIDC

Branch `task/F-aaa-login` (not pushed). Builds on F-aaa increment 1 (contract, RADIUS/TOTP engines, admin test).
User page: `docs/user/system/aaa.md`. Contract: `docs/status/tasks/F-aaa-login-contract.md`.

## What
- **Login order** (`apps/api/src/auth/auth.service.ts` `login()`): walks `management.aaa.order` (running config).
  `local` answers for local users with a password (unknown name falls through); RADIUS/LDAP: per-server failover on
  unreachable only, an answer is final; `fallbackLocal` only when every external method was unreachable and `local`
  is not in `order`. With the default `[local]` the P06 path is unchanged (refactored into `localLogin()`).
- **External → session**: groups → `roleMap` (highest privilege) → shadow `app_user` (source `external`, NULL hash)
  → the existing `session()` path. Unmapped → 403 `no-role-mapping` problem+json (and the shadow user's sessions end);
  a local user's name is never taken over; a lower role than before bumps the credential generation; identity link
  in `aaa_external_identity`.
- **Rate limiting**: the P06 per-client bucket plus a per-login-name bucket for external attempts
  (`extlogin:<name>`, VRX_LOGIN_RATE_PER_MIN); external rejects of a known shadow user count toward the lockout; a
  locked shadow user is not sent to the directory. OIDC start/callback and MFA verify share the per-client bucket.
- **LDAP** (`features/aaa/ldap.ts`, new dep `ldapts@9.2.0`): service bind → RFC 4515-escaped filter → exactly one
  entry → user bind; empty password refused before any I/O; no clear-text bind.
- **OIDC** (`features/aaa/oidc.ts`, `oidc.controller.ts`, on `jose` + fetch — no `openid-client`): code flow + PKCE
  S256 + nonce, single-use state in Valkey (10 min, GETDEL), discovery issuer check, ID token verified against the
  JWKS (iss/aud/exp/nonce), client secret from the secret store at use time; only fixed error slugs reach the browser.
  Contract: `management.aaa.oidc` + `AuthMethod += oidc` + proto `ManagementAaa.oidc = 5`.
- **MFA** (`features/aaa/mfa.service.ts`, `mfa.controller.ts`, migration `0007_f_aaa_mfa`): seed AES-GCM encrypted
  (AAD `mfa/<uid>`), recovery codes argon2id, single use; replay guard = one conditional UPDATE of `last_step`.
  Login answers `{mfaRequired, challenge, enrolled}` (no cookie) when `mfa.required` covers the role or the user
  enrolled; challenge single use (DEL wins), 5 min, 5 tries, bound to the credential generation; wrong codes count
  toward the lockout. Login-time enrolment when the policy requires a factor not set up. Voluntary set-up needs the
  current password; activation ends the user's other sessions. Admin reset `DELETE /auth/mfa/users/{name}`.
- **No stale-session bypass**: sessions that passed MFA are flagged (`mfasid:<sid>`); every JWT request
  (`authenticate`, policy cached ≤ 5 s) and every refresh of a role the policy covers needs the flag.
- **D-100 for external principals**: option (b) *refuse* — API-key creation / MFA set-up from a session without a
  local hash → 403 `external-principal`, never counted toward the lockout. Option (a) (re-authenticate against the
  backend that logged them in) is the alternative; refusing is simpler and removes a directory-bound oracle.
- **Web** (`apps/web/src/domains/system/aaa/`): MFA login step (code / recovery code / login-time enrolment,
  recovery codes shown once), OIDC handover via URL fragment, SSO button, `/system/aaa` page (own factor; admin
  backend test), user-menu entry; Users page: *Set password* (`POST /users/{name}/password`, D-102) and *Reset
  second factor* for existing users, hash field no longer offered when editing an existing user. en + fa `aaa.json`.

## Routes added
Public (route-guard PUBLIC): `POST /auth/mfa/verify`, `POST /auth/mfa/enroll` (both authorised by the challenge),
`GET /auth/methods`, `GET /auth/oidc/start`, `GET /auth/oidc/callback`. Readonly-may: `POST /auth/mfa/setup`,
`POST /auth/mfa/activate`. Admin-only (ADMIN_ONLY): `DELETE /auth/mfa/users/:name`. Protected: `GET /auth/mfa`.

## Shared hunks (anchors)
`apps/api/src/db/schema.ts` (3 tables under `// wave-BC: F-aaa`), `apps/api/migrations/0007_f_aaa_mfa.sql` +
meta (generated — regenerate on top of main at merge), `apps/api/src/auth/route-guard.test.ts` (PUBLIC/READONLY_MAY/
ADMIN_ONLY), `packages/schema/src/domains/management.ts` (AuthMethod + one key line + 2 refines),
`packages/proto/vrx/v1/dataplane.proto` (field 5 + `AaaOidc`), generated `apps/agent/gen`, `packages/proto/gen/ts`,
`packages/yang/generated/vrx-management.yang`, `packages/api-client/src/generated/schema.d.ts`,
`apps/api/package.json` + `pnpm-lock.yaml` (ldapts), web `router.tsx`/`nav/nav.ts`(+test)/`i18n.ts` under the F-aaa
anchors, named hunks in `pages/LoginPage.tsx`, `pages/UsersPage.tsx`, `shell/UserMenu.tsx`, `auth/session.ts`,
`apps/api/src/auth/dev-weak-warning.test.ts` (constructor arity).

## Verification
See the "Gates" section below (pasted output). PostgreSQL: this container has no running PostgreSQL service, but the
PostgreSQL 16 binaries and `redis-server` are installed; for the e2e run I started a throw-away cluster
(`initdb` under `/var/lib/postgresql/faaa-e2e`, 127.0.0.1:5432, trust auth) and `redis-server` on 127.0.0.1:6379,
slot prefix `w7`, and stopped/removed both afterwards. No freeradius/slapd/IdP: RADIUS is a live in-process UDP
responder, LDAP goes through the ldapts client seam (fake directory), OIDC against an in-process fake IdP (real
HTTP discovery/JWKS/token endpoint, RS256 ID tokens).

## Gates (pasted)
`npx turbo run lint typecheck test --concurrency=1 --filter=@ngfw/api --filter=@ngfw/web`:
```
@ngfw/api:test:  Test Files  58 passed (58)
@ngfw/api:test:       Tests  348 passed (348)
@ngfw/web:test:  Test Files  90 passed (90)
@ngfw/web:test:       Tests  523 passed (523)
 Tasks:    19 successful, 19 total
```
PostgreSQL e2e (throw-away cluster, prefix w7): aaa-login 8/8, aaa-oidc 4/4, aaa 3/3; the auth regression suites
(auth, td2*, td4*, td10b*, sec-auth, dev-weak) are green. auto-block "expires a block" failed once and passed on a
rerun (a timing flake in a test this branch does not touch). Schema 1558/1558; the agent contracttest is ok.
packages/proto desired-state "13 root keys" already fails on main (the `security` key). `tools/ci.sh check`:
gitleaks finds 5 generic-api-key hits, all in commits already on main, none in this branch.

## Out of scope (not built)
TACACS+ and SAML backends (`order: [tacacs]` is skipped at login; test route 501); `GET /state/aaa/servers`
reachability chips; SchemaForm AAA settings page (settings stay in the Management domain editor); QR-code rendering
(secret + otpauth URI are shown as text); LDAPS CA pinning via `cert/<name>` (system trust store is used);
WebAuthn; OS/SSH login; RADIUS accounting; screenshots (no headless browser run in this session).

## Open questions
1. Local login after an external **reject**: implemented default *no* (only on unreachable). Confirm.
2. Several mapped groups → **highest** privilege (implemented; increment 1's test route used first-match). Confirm.
3. Login-time enrolment is trust-on-first-use: whoever holds the first factor of a not-yet-enrolled user when the
   policy switches on can bind their authenticator. Alternative: admin-issued enrolment. Decide.
4. `pg-repo.syncUsers` (not owned): a config user created with the same name as an existing external shadow user is
   upserted onto that row but keeps `source = 'external'`, so later removal from the config does not delete it. A
   one-line fix in pg-repo (set `source: 'config'` on conflict) belongs to its owner.
5. `users.service.setPassword` (not owned): an external user changing their own password hits
   `verifyPassword(null)` → counts a failure (can lock). Should refuse with `external-principal` like API keys.
6. Shadow users are not listed/removable in the UI (they are not config). Needs a small admin list/delete route?
7. The OIDC identity and a RADIUS/LDAP identity with the same login name share one shadow user (both sources are
   admin-trusted). Keep, or key shadow users by (method, subject)?
8. Commit trailer: common.md says `Claude Fable 5.1`, the session's system instructions say `Claude Opus 5.5`; the
   system-level instruction was followed.
