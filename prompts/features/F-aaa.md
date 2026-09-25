# Task: F-aaa — external AAA for management login (RADIUS, TACACS+, LDAP, OIDC, SAML) + MFA   (prepend 00-CONTEXT.md)

## Goal
Let administrators log in to the API/UI with external identity sources and a second factor (WBS D10.2 in `plan/wbs.csv`),
on top of P06's local users (argon2id, JWT + rotating refresh cookie, API keys, roles admin/operator/readonly, lockout).
Reference: TNSR "RADIUS / TACACS+ / LDAP authentication". **Everything lives in vrx-api** — authentication material never
crosses the API↔agent boundary (D-040); the agent is not touched. Priority order inside the 10 h box:
**RADIUS → TOTP MFA → LDAP → OIDC → TACACS+ → SAML** — stop at the time box and list what is left.

## Inputs to read first
- `packages/schema/src/domains/management.ts` — existing: `aaa.order` (`local|radius|tacacs`, 1–3, unique),
  `aaa.radius.servers[] {address, authPort, secretRef(psk/…), timeoutSec}`, `aaa.tacacs.servers[] {address, port, secretRef, timeoutSec}`,
  `users[]` (`passwordHash` write-only, D-046), `tls`. **Missing** (add as `contract(schema)`/`contract(proto)` commits on YOUR task branch —
  workers never create `contract/` branches; additive only, D-078 contracts-v1; the `AuthMethod` enum and the `order` `.max(3)` are widened in
  place — only F-aaa touches `AaaSchema`):
  `aaa.order` values `ldap|oidc|saml`; `aaa.ldap{servers[] {url(ldaps://), bindDn, bindPasswordRef(password/…), baseDn, userFilter, groupAttr, startTls}}`;
  `aaa.oidc{issuer, clientId, clientSecretRef(token/…), scopes[], roleClaim}`; `aaa.saml{idpMetadataUrl|idpMetadata, spEntityId, roleAttr}`;
  `aaa.roleMap[] {group, role}` (external group → admin/operator/readonly, default none = reject); `aaa.mfa{required: none|admins|all, issuer}`;
  `aaa.fallbackLocal` (local login allowed when every external server is unreachable). **Proto mirror is required** even though the agent
  never reads these leaves: the schema⊆proto drift guard (`apps/agent/internal/contracttest/drift_test.go`, `packages/proto/test/parsed-documents.test.ts`)
  fails every non-secret schema leaf without a proto field → mirror them in `ManagementAaa` with the numbers from `docs/status/wave-BC-numbers.md`
  (new messages prefixed `Aaa*`); a leaf flagged `secret: true` must NOT get a proto field (D-040). Secret *references* (`bindPasswordRef`,
  `clientSecretRef`) are plain strings and are mirrored, like `RadiusServer.secret_ref`.
- P06 (merged) and TD-2/TD-4 (merged before you start) on main: `apps/api/src/auth/{auth.service,auth.controller,auth.guard,tokens.service}.ts`,
  `apps/api/src/users/**`, `apps/api/src/secrets/secrets.service.ts`, `apps/api/src/db/schema.ts` (Drizzle, `app_user` incl. the credential
  generation, `api_key`, `secret`, `audit_log`), `apps/api/migrations/` — reuse the session/JWT path; an external login ends in the same
  `session()` call with a principal whose role came from `roleMap`. Web: `apps/web/src/pages/{LoginPage,UsersPage}.tsx`, `apps/web/src/shell/UserMenu.tsx`,
  `apps/web/src/auth/{session.ts,AuthProvider.tsx}`.
- `docs/decisions/LOG.md` D-040, D-046 (redactSecrets, hashes write-only), D-051 (`<kind>/<name>` refs, existence checked by the API), D-003 (no tenants),
  D-097 (hashes only in app_user; reset revokes sessions + keys), D-100 (API-key creation from a JWT session needs the current password; login
  transport check), D-102 (a hash staged through the config API = admin reset; **the Users page password change moves from config staging to
  `POST /api/v1/users/{name}/password` in this task**)

## Scope — build exactly this
1. **Contract** (commits on your task branch, subjects `contract(schema): aaa ldap/oidc/saml/mfa` and `contract(proto): aaa mirror`): the fields above + semantic rules — every method in
   `order` has a configured backend; `ldaps://` or `startTls` required (no clear-text bind); `roleMap` roles valid; secret refs well formed.
2. **API** module `apps/api/src/features/aaa/`: a `AuthBackend` interface (`authenticate(user, password) → {groups} | reject | unreachable`)
   with RADIUS (PAP + Message-Authenticator, RFC 3579; a maintained npm client or a small RFC 2865 codec — no shell), LDAP (bind + search via
   `ldapts`), TACACS+ (single-connection PAP authen) behind the same interface; P06's login walks `aaa.order`; per-server timeout and failover;
   **TOTP MFA** (RFC 6238, enrolment with QR secret shown once, stored encrypted in the P06 secret store, recovery codes hashed);
   OIDC authorisation-code + PKCE (`openid-client`); SAML SP (`@node-saml/node-saml`) only if time remains. Shared secrets resolved from the
   secret store at use time, never cached in logs, never in problem+json details. Audit every external login (method, server, result — no password).
3. **Routes**: `POST /api/v1/auth/login` extended (returns `mfaRequired` + short-lived challenge), `POST /api/v1/auth/mfa/verify`,
   `POST /api/v1/auth/mfa/enroll`, `GET /api/v1/auth/oidc/start|callback`, `GET /api/v1/state/aaa/servers` (reachability of each server),
   `POST /api/v1/actions/aaa/test` (test a credential against one backend, admin only). OpenAPI; regenerate `packages/api-client`.
   The unauthenticated routes (MFA verify with the challenge, OIDC start/callback) go on the PUBLIC list and `actions/aaa/test` on ADMIN_ONLY in
   `apps/api/src/auth/route-guard.test.ts`. D-100 step-up for users without a local hash (external principals): re-authenticate against the
   backend that logged them in, or refuse API-key creation from their JWT session — pick, log with options. TOTP seeds and recovery-code hashes
   live in a new table (one Drizzle migration generated on top of main, never hand-numbered).
4. **UI** `apps/web/src/domains/system/aaa/`: AAA settings (SchemaForm), server reachability chips, "test login" dialog, MFA enrolment screen
   on the user menu, MFA step on the login page; the Users page password change switches to `POST /api/v1/users/{name}/password` (D-102);
   en + fa (`locales/*/aaa.json`).
5. **Tests**: unit tests per backend with in-process fake servers (RADIUS UDP responder, LDAP via `ldapts` test server or a mock, TACACS+ TCP
   stub) on your slot's ports; a real `freeradius`/`slapd` **only if already installed** (never `apt-get install`), bound to 127.0.0.1 on slot ports.
6. **Docs**: `docs/user/system/aaa.md` — each method, role mapping, MFA recovery, break-glass local admin.

Files you own: `apps/api/src/features/aaa/**`, `apps/web/src/domains/system/aaa/**`, `apps/web/src/locales/*/aaa.json`, `docs/user/system/aaa.md`,
`test/topology/aaa/**` — plus minimal, named hunks in the auth/login/users files listed above (the envelope has the exact list). Shared files:
one-line appends only under your anchor (`app.module.ts`, web router/nav/i18n, route-guard lists, management.ts key lines) — the manager
resolves at merge. freeradius, slapd and tac_plus are not installed on this host; new npm dependencies need the registry (questions file).

## Acceptance (paste the evidence)
- [ ] RADIUS login against the fake/local server → JWT with the mapped role; wrong password → 401; server down + `fallbackLocal` → local admin works (pasted)
- [ ] TOTP: enrolment, login needing the second step, replayed code rejected, recovery code single-use (test output)
- [ ] `grep -rn` over logs, fixtures, audit rows and GET `/config/management` shows no shared secret, bind password or TOTP seed
- [ ] Unmapped external user → 403 with problem+json; `aaa.order: [radius]` with no servers → 400 with `pointer`
- [ ] UI screenshot (AAA page + MFA login step) against the real endpoint; `tools/ci.sh --base main` green

## Out of scope (do not build)
VDOM/tenant scoping (D-003, D10.1 is a D-059 have-not); SSH/console login via PAM-RADIUS/TACACS for the OS shell (F-hardening-lite decides OS
login policy); RADIUS accounting; EAP for VPN users (F-ra-vpn); WebAuthn/FIDO2; changing P06's local password/lockout logic beyond the hook;
certificate management for LDAPS CAs (reference `cert/<name>` from F-pki's store, do not build a CA UI); SECURITY-REVIEW's adversarial test.

## Open questions to surface, not to decide silently
Should local login stay allowed when an external server answers "reject" (not unreachable)? Default: no — only on unreachable. Group→role
precedence when a user is in several mapped groups (default: highest privilege — flag it).
