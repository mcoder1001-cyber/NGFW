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
  `users[]` (`passwordHash` write-only, D-046), `tls`. **Missing** (add on `contract/F-aaa`, additive only, D-078 contracts-v1):
  `aaa.order` values `ldap|oidc|saml`; `aaa.ldap{servers[] {url(ldaps://), bindDn, bindPasswordRef(password/…), baseDn, userFilter, groupAttr, startTls}}`;
  `aaa.oidc{issuer, clientId, clientSecretRef(token/…), scopes[], roleClaim}`; `aaa.saml{idpMetadataUrl|idpMetadata, spEntityId, roleAttr}`;
  `aaa.roleMap[] {group, role}` (external group → admin/operator/readonly, default none = reject); `aaa.mfa{required: none|admins|all, issuer}`;
  `aaa.fallbackLocal` (local login allowed when every external server is unreachable). Proto mirror only if the agent needs it — it does not.
- P06 on `task/P06` (read only: `git show task/P06:apps/api/src/auth/auth.service.ts`, `auth.guard.ts`, `tokens.service.ts`,
  `secrets/secrets.service.ts`, `db/schema.ts` (Drizzle, `app_user`, `api_key`, `secret`, `audit_log`)) — reuse the session/JWT path;
  an external login ends in the same `session()` call with a principal whose role came from `roleMap`.
- `docs/decisions/LOG.md` D-040, D-046 (redactSecrets, hashes write-only), D-051 (`<kind>/<name>` refs, existence checked by the API), D-003 (no tenants)

## Scope — build exactly this
1. **Contract** (`contract/F-aaa`, subject `contract(schema): aaa ldap/oidc/saml/mfa`): the fields above + semantic rules — every method in
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
4. **UI** `apps/web/src/domains/system/aaa/`: AAA settings (SchemaForm), server reachability chips, "test login" dialog, MFA enrolment screen
   on the user menu, MFA step on the login page; en + fa (`locales/*/aaa.json`).
5. **Tests**: unit tests per backend with in-process fake servers (RADIUS UDP responder, LDAP via `ldapts` test server or a mock, TACACS+ TCP
   stub) on your slot's ports; a real `freeradius`/`slapd` **only if already installed** (never `apt-get install`), bound to 127.0.0.1 on slot ports.
6. **Docs**: `docs/user/system/aaa.md` — each method, role mapping, MFA recovery, break-glass local admin.

Files you own: `apps/api/src/features/aaa/**`, `apps/web/src/domains/system/aaa/**`, `apps/web/src/locales/*/aaa.json`, `docs/user/system/aaa.md`,
`test/topology/aaa/**`. Shared files: one-line appends only (`app.module.ts` import, web router/nav, P06 login hook call) — the manager resolves at merge.

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
