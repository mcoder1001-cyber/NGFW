# F-aaa-login — external login + MFA (cloud session eloquent-wozniak, 2026-09-27)

Increment 2 of `F-aaa` (increment 1: `docs/status/tasks/F-aaa.md` — contract, RADIUS/TOTP engines, admin
`actions/aaa/test`). This increment makes `management.aaa` actually decide logins, and adds MFA.
User page: `docs/user/system/aaa.md`. Board row state → `review`.

## Delivered — all in the API (D-040), fully in-container

### 1. The login-order walk (`apps/api/src/auth/login-order.ts`, `auth.service.ts`)
`POST /auth/login` walks `management.aaa.order`. The default order (`['local']`) is byte-for-byte the previous
behaviour, including the audit reason `unknown-user` — nothing changes until a backend is configured.

A method answers `accept`, `reject`, `absent` (it does not know the identity) or `unreachable` (no server answered).
**The first method that accepts or rejects ends the walk.** Two attacks this closes: a directory that knows a local
name with a different password cannot log in as that local user after `local` refused it, and a rejected password is
not replayed against every other backend in turn. `fallbackLocal` adds a closing `local` step that runs only when a
method was unreachable — it is "let local users in while the directory is down", not a second always-on local login.

The local step keeps the reviewed ordering of argon2, the unknown-user check and the two lockouts (D-097, TD-4,
TD-10b) **unchanged**. Its only change: it answers `absent` instead of failing the login when this box holds no local
credential for the name (no such user, or a row with no argon2 hash — a shadow account, or a `management.users` entry
committed without a `passwordHash`), so the walk can offer the name to the next method. argon2 has already run against
the timing equaliser at that point, so `absent` costs what a real check costs.

### 2. Shadow accounts for external identities
An accepted external identity gets a local account with `source: 'aaa:<method>'` and no password hash; the role comes
from `aaa.roleMap` (no match → refused, least privilege). Sessions, revocation, API keys and the audit log then work
exactly as for a local user. `syncUsers` (pg-repo) only ever deletes `source = 'config'` rows, so a commit leaves
shadow accounts alone and they never appear in `management.users`. A demotion bumps the credential generation, as the
config user sync does (TD-10b). An existing **local** account of the same name is never taken over (audited
`local-account`). An external `reject` counts toward the per-(user, client) lockout once the account exists.

### 3. MFA (`features/aaa/mfa.service.ts`, `auth/mfa-ticket.ts`, migration `0007_f_aaa_mfa`)
`mfa.required` (`none|admins|all`) covers local and external users alike.
- **Enrolment**: seed generated, handed out once, stored **AES-256-GCM encrypted** (AAD `mfa/<user id>`, so a blob
  cannot be moved between accounts) as `mfa_pending_secret`; it becomes active only when a code proves it, so an
  abandoned enrolment cannot lock anybody out. Confirming mints 10 single-use recovery codes, returned once and kept
  only as sha256 (their entropy was widened 40 → 80 bits for exactly that reason). An active factor is never replaced
  silently (409).
- **Two-step login**: the first factor answers a challenge carrying no token and no cookie. The ticket is a 256-bit
  bearer token stored only as a sha256 under a 180 s TTL and consumed with `GETDEL`, so it is single-use; a wrong code
  returns a *fresh* ticket with `attemptsLeft` one lower, and after 3 the login is over — the password path, which is
  rate-limited and lockout-counted, is what bounds code guessing.
- **Forced enrolment**: when the policy requires MFA and the account has no factor, the challenge is
  `mfa: 'enrol'` and that ticket buys enrolment and nothing else. So `mfa.required` can be switched on without locking
  anyone out, and is not merely advisory — there is no session until a factor exists and has been proved.
- **Routes**: `POST auth/login/mfa`, `POST auth/login/mfa/{enroll,verify}` (public: the ticket is the credential),
  `GET auth/mfa`, `POST auth/mfa/{enroll,verify}`, `DELETE auth/mfa` (self, readonly may — own credentials, like the
  password change; a current code is required to turn MFA off, so a stolen session cannot strip a factor it cannot
  produce), `POST actions/aaa/mfa/reset` (admin, for a lost authenticator).
- All four session/admin MFA routes are in `PRIVILEGED_ROUTES`: an MFA change is a credential change, so it
  fails closed if its audit row cannot be written first (TD-10b).

### 4. Two races closed while self-reviewing the diff
- `upsertShadow`'s first-login INSERT carries `setWhere source like 'aaa:%'` on its `onConflictDoUpdate`: a conflict
  there means a row appeared between the SELECT and the INSERT, possibly a LOCAL user from a concurrent commit, and
  without the guard that row would have been re-sourced as a shadow — silently defeating the takeover rule above and
  taking the account out of the config sync's reach. Now the update touches shadow rows only; for a local one no row
  comes back and the login is refused.
- `POST /auth/login/mfa` carries the same per-client budget the password route has, on its own bucket. It is not what
  bounds code guessing (the 3 attempts and the rate-limited password path are), but the second factor should not be
  hammerable any harder than the first.

### 5. Policy caching
`AaaService` caches `management.aaa` and reloads it on commit events (the `AutoBlockService` pattern), so a login does
not read the running document from PostgreSQL every time. Its new `authenticate()` answers `unreachable`/`unsupported`
for a misconfigured or not-yet-implemented backend instead of turning a login into a 500.

## Evidence (run in this container against real PostgreSQL 16.13 and Valkey — not mocks)

```
$ pnpm --filter @ngfw/api exec vitest run
 Test Files  56 passed (56)
      Tests  341 passed (341)
   Duration  29.35s

$ VRX_TEST_PREFIX=cw1 VRX_VALKEY_DB=1 pnpm exec vitest run -c vitest.e2e.config.ts test/e2e
 Test Files  51 passed (51)
      Tests  265 passed (265)
   Duration  221.16s
```

New suites inside those totals:

```
 ✓ test/e2e/aaa-login.e2e.test.ts (11 tests)     # order walk, shadow accounts, fallbackLocal
 ✓ test/e2e/aaa-mfa.e2e.test.ts   (12 tests)     # enrolment, two-step login, tickets, recovery, admin reset
 ✓ src/auth/login-order.test.ts   (7 tests)      # the order/fallback rules
 ✓ src/features/aaa/mfa.test.ts   (3 tests)      # who mfa.required covers
```

`tsc -p tsconfig.json` and `eslint src test` clean. `route-guard.test.ts` (P06 acceptance: no route without a guard)
passes with the three new public ticket routes and the admin reset added to its reviewed tables.

Two e2e assertions worth naming, because they are the security claims rather than the happy path:
- *"a local user is decided locally — the directory is never consulted"*: the fake RADIUS server records every
  username it is asked about, and the test asserts it saw **none** after a correct *and* an incorrect local password.
  The account is also configured on the RADIUS side with a different password, so a fall-through would have let it in
  with the wrong role.
- *"a wrong code costs one of three tries, and a ticket is single-use"*: the spent ticket is replayed with a **good**
  code and must still be refused.

## Decisions taken here (FAST MODE: decide and log)
1. **The first method that answers wins** (`reject` stops the walk). The alternative — try every method until one
   accepts — makes any configured directory able to override a local refusal. Documented in `docs/user/system/aaa.md`.
2. **A directory may not log in as a local account** of the same name; the login is refused (`local-account`) rather
   than the directory winning or the config role being overwritten by `roleMap`. The operator renames one of the two.
3. **`fallbackLocal` only ever adds `local` to an order that does not list it**, and only fires after an
   `unreachable`. Reading it as "always also try local" would make it a silent bypass of a directory's rejection.
4. **Forced enrolment via an `enrol` ticket** rather than either refusing the login outright (nobody could ever
   enrol) or issuing a normal session (`mfa.required` would be advisory).
5. **Recovery codes widened to 80 bits** (`totp.ts`, was 40): only a sha256 is stored, so they must be out of reach
   of an offline search if that hash leaks.

## Known and accepted
- With an external method in `order`, a name that is not a local user costs one extra backend round trip, so response
  time still tells a probe whether a name is a **local** user. Inherent to chaining (an unknown local name has to be
  offered to the directory) and bounded by the per-client login rate limit; password validity itself stays
  unobservable. Stated in the `login()` doc comment.
- A wrong MFA code is audited as `auth.mfa`, not `auth.login`, so it does not feed the auto-block detector
  (F-bruteforce-block counts `auth.login` failures). Deliberate: a user reaching for their phone is not a brute-force
  attempt, and code guessing is already bounded at 3 per ticket plus the rate-limited, lockout-counted password path.
- **A TOTP code can be reused inside its own validity window.** `verifyTotp` accepts the current step and one either
  side (90 s, increment 1's engine, for clock skew), and no last-used counter is kept, so a code observed and replayed
  within that window would verify again — but only together with the account's password, on a fresh ticket. Closing it
  needs a per-user last-used-counter column and a compare-and-set on it; worth doing when the web client lands
  (noted on `F-aaa-login-2`), not worth a schema change here for a window that already requires the first factor.

## Not done here → follow-up row `F-aaa-login-2` (increment 3)
- **LDAP** (`ldapts` bind + search, group → role). The contract landed in increment 1; the backend answers
  `unsupported` and the walk treats it as a method that did not answer, so configuring it changes nothing yet.
- **OIDC / TACACS+ / SAML**: these need the reserved envelope fields 5/6 → a `contract(proto)` change, and this
  container has no `buf`/`protoc`, so proto cannot be regenerated here (see below).
- **Web**: the login MFA prompt, the enrolment screen and the AAA test panel. The API surface they need is complete
  and in the generated client.
- **TOTP replay window**: a per-user last-used-counter column + compare-and-set, so a code cannot verify twice inside
  its 90 s window (see "Known and accepted").
- **D-102**: moving the Users password change to `POST /users/{name}/password` (that route already exists;
  the move is removing the old one, which is `apps/api/src/users/**` — not this row's files).

## Environment note for the manager (not a defect)
This ran as a cloud session, not on the dev host. `tools/ci.sh` cannot complete here: `buf`, `protoc`, `gitleaks` and
VPP are absent, and `packages/proto/gen.sh` deletes `gen/ts` and `apps/agent/gen` *before* calling `buf`, so
`pnpm gen` must not be run in this container. Nothing in this task touches `.proto`; `packages/api-client/src/generated`
was regenerated with the node-only api-client generator, and its diff is **purely additive plus reordering** (verified:
zero lines removed, `comm -23` on the sorted old/new files), so no generator-version drift was introduced. The gate
still has to run on the host before merge.
