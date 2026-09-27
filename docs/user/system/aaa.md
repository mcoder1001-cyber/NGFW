# External authentication (AAA) — increment 1

Management logins can be authenticated against external identity sources in addition to local users. This is
configured under **Config → Management → AAA** (`management.aaa`) and enforced entirely in the API (the data plane
is never involved).

> **Status.** Increment 1 ships the **contract**, the **RADIUS (PAP)** and **TOTP** engines, and an **admin
> backend-test** endpoint. The login-order integration (actually logging in via an external source), MFA
> enrolment/enforcement, and the LDAP / OIDC / SAML / TACACS+ backends are increment 2 (`F-aaa-login`).

## Configuration
```
management.aaa:
  order: [local, radius]        # methods tried in order; keep `local` so you are never locked out
  radius:
    servers:
      - { address: 10.0.0.10, authPort: 1812, secretRef: psk/radius, timeoutSec: 5 }
  ldap:                         # configurable now; enforced in increment 2
    servers:
      - { url: ldaps://ldap.example.net, bindDn: "cn=svc,dc=x", bindPasswordRef: password/ldap-bind, baseDn: "dc=x", userFilter: "(uid=%s)", groupAttr: memberOf }
  roleMap:
    - { group: netadmins, role: operator }   # external group -> local role; no match = login refused
  mfa: { required: none, issuer: vrx }        # none | admins | all (enforced in increment 2)
  fallbackLocal: true                          # allow local login when every external server is unreachable
```
- RADIUS and TACACS+ shared secrets, and the LDAP bind password, are **secret references** (`psk/…`, `password/…`),
  never inline.
- An LDAP server must use `ldaps://` or StartTLS — a clear-text bind is refused at commit.
- An external identity whose groups match no `roleMap` entry is refused (least privilege).

## Testing a backend (admin)
Before putting a method in `order`, validate it:

```
POST /api/v1/actions/aaa/test
{ "method": "radius", "username": "alice", "password": "…" }
→ { "reachable": true, "authenticated": true, "groups": ["netadmins"], "role": "operator", "detail": "authenticated; role operator" }
```
No session is issued; the call only reports whether the server is reachable, whether the credential is accepted,
the groups returned, and the role the `roleMap` would assign. It is admin-only and audited. `ldap`, `tacacs`,
`oidc` and `saml` answer `501` until increment 2.

## Multi-factor (TOTP)
The TOTP engine (RFC 6238, authenticator-app compatible) ships in increment 1; per-user enrolment and enforcement
at login arrive in increment 2, gated by `mfa.required`.
