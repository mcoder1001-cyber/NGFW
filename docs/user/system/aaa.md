# External authentication (AAA) and two-factor login

Management logins (web UI and API) can be authenticated against external identity sources in addition to local
users, and can require a second factor (TOTP). This is configured under **Config → Management → AAA**
(`management.aaa`) and enforced entirely in the API (the data plane is never involved).

Implemented: **local**, **RADIUS (PAP)**, **LDAP (bind + search)**, **OpenID Connect** (browser single sign-on) and
**TOTP** second factor. **TACACS+** and **SAML** are not built yet (a `tacacs` entry in `order` is skipped at login;
the admin test answers `501`).

## Configuration
```
management.aaa:
  order: [local, ldap, radius, oidc]   # see "Login order" below
  radius:
    servers:
      - { address: 10.0.0.10, authPort: 1812, secretRef: psk/radius, timeoutSec: 5 }
  ldap:
    servers:
      - { url: ldaps://ldap.example.net, bindDn: "cn=svc,dc=x", bindPasswordRef: password/ldap-bind,
          baseDn: "dc=x", userFilter: "(uid=%s)", groupAttr: memberOf }
  oidc:
    issuer: https://idp.example.net/realms/corp
    clientId: vrx
    clientSecretRef: token/oidc-client
    redirectUri: https://fw.example.net/api/v1/auth/oidc/callback
    # scopes: [openid, profile, email]   usernameClaim: preferred_username   roleClaim: groups
  roleMap:
    - { group: "cn=netadmins,ou=groups,dc=x", role: admin }
    - { group: noc, role: operator }
  mfa: { required: admins, issuer: vrx }  # none | admins | all
  fallbackLocal: true
```
- Shared secrets, the LDAP bind password and the OIDC client secret are **secret references** (`psk/…`,
  `password/…`, `token/…`) resolved from the secret store at the moment of use. They never appear in GET
  responses, logs, audit rows or error messages.
- LDAP must use `ldaps://` or StartTLS — a clear-text bind is refused at commit. The OIDC issuer and redirect URI
  must be `https://` (plain `http://` only for a loopback development IdP).

## Login order
`POST /api/v1/auth/login` walks `order`:
- **local** answers for users that exist locally with a password; a name unknown locally falls through to the next
  method.
- **radius** / **ldap**: each configured server is tried in turn until one *answers* (failover only when a server is
  unreachable). An answer is final: **a reject is not retried with another method** (and not with local).
- **oidc** is not a password method: it enables the "Sign in with single sign-on" button on the login page.
- **fallbackLocal**: when `local` is not in `order` and every external method was unreachable, local users can still
  log in (break-glass). Keep `local` in `order` or leave `fallbackLocal` on so you cannot lock yourself out.

An accepted external identity is mapped through `roleMap`; with several matching groups the **highest** role wins.
No match → `403 no-role-mapping` (and any earlier sessions of that user end). The first successful login creates a
*shadow* user (shown with source `external`, no local password — it can never log in with a local password). An
external name can never take over a local user of the same name. Every external answer is audited
(`auth.external`: method, server, result — never the password). External attempts are rate-limited per client and
per login name, and failures count toward the lockout.

External users cannot create API keys from their login session (there is no local password to confirm with:
`403 external-principal`).

## Two-factor authentication (TOTP)
- `mfa.required`: `none`, `admins` (admin role) or `all`. A user who enrolled voluntarily always needs the code.
- When a factor is needed, login answers `{ mfaRequired: true, challenge, enrolled }` instead of a session. The web
  UI then asks for the 6-digit code (`POST /api/v1/auth/mfa/verify`). The challenge is single use, lives 5 minutes
  and takes 5 wrong answers; each code is accepted only once.
- **Enrolment at login**: if the policy requires a factor the user has not set up, the login step shows a new secret
  once (`POST /api/v1/auth/mfa/enroll`); the first code enables it and ten **recovery codes** are shown once.
- **Voluntary enrolment**: user menu → *My second factor* (`/system/aaa`): current password, then the first code.
  Your other sessions are signed out.
- **Stale sessions**: raising `mfa.required` ends the use of sessions that never passed the second factor (checked on
  every request and every refresh).
- **Lost device**: sign in with a recovery code (each works once), or an admin uses *Reset second factor* on the Users
  page (`DELETE /api/v1/auth/mfa/users/{name}`) — the user's sessions end and they enrol again at the next login.
- TOTP secrets are stored encrypted (secret-store master key); recovery codes only as argon2id hashes. They are
  account data (tables `aaa_mfa`, `aaa_mfa_recovery`), not configuration: a configuration backup/restore does not
  carry them.

## Break-glass local admin
Keep at least one local admin and `local` in `order` (or `fallbackLocal: true`). The console break-glass tool
(`vrx-authctl`, root on the device) still unlocks local accounts; with `mfa.required: admins` the admin also needs the TOTP
code or a recovery code — keep the recovery codes offline.

## Testing a backend (admin)
`/system/aaa` → *Test an identity source*, or:
```
POST /api/v1/actions/aaa/test
{ "method": "ldap", "username": "alice", "password": "…" }
→ { "reachable": true, "authenticated": true, "groups": ["cn=netadmins,…"], "role": "admin", "detail": "authenticated; role admin" }
```
No session is issued; admin-only and audited.

## Users page
An existing user's password is set with **Set password** (`POST /api/v1/users/{name}/password`: argon2id on the
device, the user's sessions end, API keys are revoked unless *keep* is ticked) — the configuration no longer stages
a password hash for existing users (D-102).
