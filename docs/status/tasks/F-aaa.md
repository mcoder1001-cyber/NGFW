# F-aaa — external AAA for management login (cloud session charming-johnson, 2026-09-27)

External management-login auth (RADIUS/TACACS+/LDAP/OIDC/SAML) + MFA, priority order RADIUS → TOTP → LDAP → OIDC →
TACACS+ → SAML. Delivered as **increment 1** this session (the prompt's "stop at the time box and list what is left").
User page: `docs/user/system/aaa.md`.

## Delivered (increment 1) — all in the API (D-040), fully in-container
- **Contract** (6ebdb266): `AuthMethod += ldap` (order.max 3→6); `aaa.ldap{servers[]{url, bindDn, bindPasswordRef,
  baseDn, userFilter, groupAttr, startTls}}`, `aaa.roleMap[]{group, role}`, `aaa.mfa{required, issuer}`,
  `aaa.fallbackLocal`; semantic (no clear-text LDAP bind, unique roleMap groups, ldap-in-order needs a server); proto
  mirror `ManagementAaa.ldap=4/role_map=7/mfa=8/fallback_local=9` (5/6 reserved for oidc/saml), AaaLdap*/AaaRoleMapping/
  AaaMfa. Drift guard + buf breaking clean.
- **RADIUS PAP** (3bd18dd2, `features/aaa/radius.ts`): RFC 2865 Access-Request over UDP with PAP + Message-Authenticator
  (RFC 3579), node crypto only — no external RADIUS library; Accept/Reject/unreachable + Class/Filter-Id groups.
- **TOTP** (`features/aaa/totp.ts`): RFC 6238 (HMAC-SHA1, ±1 step skew), base32 secret, otpauth:// URI, recovery codes.
- **Admin backend test**: `POST /api/v1/actions/aaa/test` (admin-only, audited) authenticates a test credential
  against the configured RADIUS server and reports reachable/authenticated/groups/role (roleMap) without issuing a
  session; LDAP/OIDC/SAML/TACACS+ answer 501.

## Evidence
- RADIUS unit tests vs a local UDP responder (accept with reply groups, reject, unreachable, multi-block password);
  TOTP RFC 6238 test vector + skew + URI/recovery; e2e (role mapping against a live local RADIUS server, reject, 501,
  operator 403); route-guard has actions/aaa/test as ADMIN_ONLY. API unit 313/313; drift + buf breaking clean.

## Not done here → follow-up row `F-aaa-login` (increment 2)
- **Login-order integration**: try `order` methods on `POST /auth/login`; on external success map role via roleMap,
  auto-provision a shadow local user (no local password), issue a session through the existing `session()` path;
  `fallbackLocal` when all external servers are unreachable. This touches the security-critical lockout/session code
  and deserves its own review — deliberately deferred rather than rushed in a long autonomous session.
- **MFA**: per-user TOTP enrolment (encrypted seed + recovery-code hashes, DB migration `f_aaa_mfa`, external identity
  link), a two-step login, enforcement by `mfa.required`; the web login MFA prompt.
- **Backends**: LDAP (ldapts, bind+search, group→role), then OIDC, TACACS+, SAML (envelope fields 5/6 for oidc/saml).
- **Web**: an AAA test panel on the config screen; move the Users password change to `POST /users/{name}/password`
  (D-102).
