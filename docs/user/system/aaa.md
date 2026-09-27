# External authentication (AAA) and multi-factor login

Management logins can be authenticated against external identity sources in addition to local users, and can be
required to present a second factor. This is configured under **Config → Management → AAA** (`management.aaa`) and
enforced entirely in the API (the data plane is never involved).

> **Status.** Increment 1 shipped the contract, the RADIUS (PAP) and TOTP engines and the admin backend-test route.
> Increment 2 (`F-aaa-login`) adds the **login-order walk** — you can now actually log in through an external
> source — and **MFA enrolment and enforcement**. The LDAP, OIDC, SAML and TACACS+ backends are still to come:
> they are configurable, and `order` accepts them, but they answer "not implemented" and the walk treats them as a
> method that did not answer.

## Configuration
```
management.aaa:
  order: [local, radius]        # methods tried in order; keep `local` so you are never locked out
  radius:
    servers:
      - { address: 10.0.0.10, authPort: 1812, secretRef: psk/radius, timeoutSec: 5 }
  ldap:                         # configurable now; the backend itself is not implemented yet
    servers:
      - { url: ldaps://ldap.example.net, bindDn: "cn=svc,dc=x", bindPasswordRef: password/ldap-bind, baseDn: "dc=x", userFilter: "(uid=%s)", groupAttr: memberOf }
  roleMap:
    - { group: netadmins, role: operator }   # external group -> local role; no match = login refused
  mfa: { required: none, issuer: vrx }       # none | admins | all
  fallbackLocal: true                        # allow local login when every external server is unreachable
```
- RADIUS and TACACS+ shared secrets, and the LDAP bind password, are **secret references** (`psk/…`, `password/…`),
  never inline.
- An LDAP server must use `ldaps://` or StartTLS — a clear-text bind is refused at commit.
- An external identity whose groups match no `roleMap` entry is refused (least privilege).

## How a login is decided

`POST /api/v1/auth/login` walks `order`. Each method can give one of four answers:

| Answer | Meaning | What the walk does |
|---|---|---|
| **accept** | the credential is good | stop; log in (subject to MFA below) |
| **reject** | the method knows this identity and refused it | **stop; refuse the login** |
| *absent* | the method has no such identity (no local user, or a user with no local password) | try the next method |
| *unreachable* | no server of that method answered | try the next method, and allow `fallbackLocal` |

**The first method that accepts or rejects ends the walk.** This matters: with `order: [local, radius]`, a local user
who types the wrong password is refused there and then — the password is *not* offered to RADIUS. Otherwise a
directory that happens to know the same name with a different password could let someone in as that local user, and
every rejected password would be replayed against every backend.

`fallbackLocal: true` adds a closing `local` step that runs **only** when a method was unreachable. It means "let
local users in while the directory is down", not "always also try local": if the directory answered and said no,
the fallback does not run. When `local` is already in `order` it is tried at its configured position and no extra
step is added.

Default configuration is `order: [local]`, which is exactly the plain local login.

### Externally authenticated users

The first time an external identity logs in, the box creates a local **shadow account** for it: the role comes from
`roleMap`, and the account has no password, so it can never be used through the `local` step. From then on sessions,
role changes, session revocation, API keys and the audit log work exactly as for a local user.

- Shadow accounts are **not** part of the configuration: they never appear in `management.users` and a commit does
  not remove them.
- If `roleMap` later maps the identity to a **lower** role, the sessions and API keys issued under the higher role
  end at the next login, as for a local demotion.
- A directory can never log in **as** an existing local account of the same name — the login is refused and audited
  with reason `local-account`. Rename one of the two.

## Multi-factor authentication (TOTP)

`mfa.required` decides who needs a second factor: `none`, `admins` (the admin role only) or `all`. It applies to
local and externally authenticated users alike.

### Enrolling

From a session (**your own account**, any role):

```
POST /api/v1/auth/mfa/enroll        → { secret, otpauthUri }     # scan otpauthUri in an authenticator app
POST /api/v1/auth/mfa/verify        { "code": "123456" }
                                    → { recoveryCodes: [ … 10 codes … ] }
GET  /api/v1/auth/mfa               → { enrolled, required, pending, recoveryCodesLeft }
DELETE /api/v1/auth/mfa             { "code": "123456" }         # a current code is required
```

The seed is stored **encrypted** (the secret store's master key) and is not active until a code proves it, so an
abandoned enrolment cannot lock you out. The **recovery codes are shown once** — store them somewhere safe. Each is
single-use. An account that already has a verified factor cannot silently replace it: reset it first.

### Logging in with MFA

When a second factor is owed, the login answers a **challenge** instead of a session — no token, no cookie:

```
POST /api/v1/auth/login   { username, password }
→ { "mfa": "code", "ticket": "…", "expiresIn": 180, "attemptsLeft": 3 }

POST /api/v1/auth/login/mfa   { "ticket": "…", "code": "123456" }
→ { accessToken, tokenType, expiresIn, user }
```

The ticket proves only the first factor: it is single-use and lives three minutes. A wrong code returns a *fresh*
ticket with `attemptsLeft` one lower; after three wrong codes the login is over and the password has to be presented
again (and that path is rate-limited and counts toward the lockout). A recovery code may be used in place of the
TOTP code; it is spent in the process.

**A code is single-use.** Once a TOTP code has been accepted it will not be accepted again, even though it is still
inside the 90-second window the clock-skew tolerance allows (RFC 6238 §5.2). Logging in twice in quick succession
therefore means waiting for the authenticator to show the next code.

### When MFA is required but not yet enrolled

The login answers `{ "mfa": "enrol", … }`. That ticket buys **enrolment and nothing else**:

```
POST /api/v1/auth/login/mfa/enroll   { "ticket": "…" }      → { secret, otpauthUri, ticket, expiresIn }
POST /api/v1/auth/login/mfa/verify   { "ticket": "…", "code": "123456" }
                                     → { recoveryCodes, accessToken, … }   # enrolled and logged in
```

This is why `mfa.required` can be switched on for users who have no factor yet without locking anybody out — and why
it is not merely advisory: there is no session until a factor exists and has been proved.

### Lost authenticator (admin)

```
POST /api/v1/actions/aaa/mfa/reset   { "username": "alice" }    → 204
```

Admin-only and audited. It grants nothing by itself: where the policy requires MFA, that user's next login asks for
enrolment again.

## Testing a backend (admin)
Before putting a method in `order`, validate it:

```
POST /api/v1/actions/aaa/test
{ "method": "radius", "username": "alice", "password": "…" }
→ { "reachable": true, "authenticated": true, "groups": ["netadmins"], "role": "operator", "detail": "authenticated; role operator" }
```
No session is issued; the call only reports whether the server is reachable, whether the credential is accepted, the
groups returned, and the role the `roleMap` would assign. It is admin-only and audited. `ldap` and `tacacs` answer
`501` until their backends land.

## Audit

| Action | When |
|---|---|
| `auth.login` success | a session was issued; `after.method` names the AAA method for an external login |
| `auth.login` failure | every refused login, with `after.reason` (`unknown-user`, `bad-password`, `rejected`, `no-role-mapping`, `local-account`, `all-methods-unreachable`, …) |
| `auth.mfa` | a challenge was issued, a code was wrong, or a factor was used |

Only `auth.login` failures feed the brute-force auto-block detector: a user reaching for their phone is not a failed
login.

## CLI equivalent

```
config management aaa order local radius
config management aaa mfa required admins
show management aaa                     # policy and configured backends
request aaa test radius <user>           # the admin backend test
request aaa mfa-reset <user>             # clear a user's second factor
```
