# SEC-auth — auth security review (read-only; cloud session charming-johnson, 2026-09-27)

Scope (review 6.7): `apps/api/src/{auth,users,commit,secrets}/**`, route guards, WebSocket auth before the upgrade, API
keys, lockout, refresh chains, cookies/CSRF, the transport check, D-091/D-097/D-100/D-102. Read against `main@a48f857c`.
Three lenses — an unauthenticated attacker, a lower-privileged insider (readonly/operator, a leaked API key), and a
local user on the box — then an adversarial re-read of each finding against the code paths that would refute it.

## Verdict: two findings (1 MEDIUM, 1 LOW) → fix row `SEC-auth-fix`; nothing HIGH

## Findings

### M1 — a leaked expiring API key can mint a permanent one (`auth/auth.service.ts` createApiKey)
- An API-key caller may create keys without step-up (D-124, intended: automation). The new key's `expiresAt` comes
  only from the request's `expiresInDays`; absent → `null` (never expires). The caller's own expiry
  (`Principal.exp`, set from `api_key.expires_at`) is not consulted.
- The minted key is independent: deleting the caller's key does not delete it, and the owner's own password change
  keeps API keys (only an admin reset or a config password change revokes them, D-097/D-102).
- Scenario: a CI system gets a 7-day key; it leaks; the attacker calls `POST /api/v1/auth/api-keys` with it (no
  `current`, no `expiresInDays`) and holds a non-expiring key of the same role. Revoking the leaked key or waiting for
  its expiry does not end the access.
- Refuted? The role cannot grow (`scope = lowerRole(user.role, role)` where `user.role` is already the key-capped
  role) — only the lifetime escapes. Readonly keys cannot mint (the route's default role is operator). Confirmed.
- Fix: for `via === 'apikey'`, the new key's expiry is at most the caller key's (`expiresAt = min(requested,
  caller exp)`; a caller key without expiry keeps today's behaviour); optionally record the parent key and delete
  children with it. Test: an expiring key minting without `expiresInDays` gets the caller's expiry, never `null`.

### L1 — the secret-store master key file skips the key-file checks (`secrets/secrets.service.ts` masterKey)
- The JWT key ring is read through `readKeyFile` (O_NOFOLLOW, regular file, owner = API user or root, mode 0600;
  TD-10b). The master key (`VRX_SECRET_KEY_FILE`, AES-256-GCM for every stored secret) uses `existsSync` +
  `chmodSync` + `readFileSync`: it follows a symlink, checks no owner, and `chmodSync` also follows a symlink (it would
  chmod the link's target). key-file.ts itself says the master key "should call it too" (left to TD-10a).
- Needs write access to `/var/lib/vrx` (root/vrx) to exploit — hence LOW; it is the more valuable key of the two.
- Fix: read it with `readKeyFile` (create with `openSync(…, 'wx', 0o600)` when missing, as break-glass does), drop the
  `chmodSync`; a bad owner/mode/symlink is a 503 with the key-file message, never the content.

## Checked, no finding
- **Guard:** global `AuthGuard`, every route authenticated unless `@Public()` (login/refresh/logout/health, pinned by
  `route-guard.test.ts`); role defaults readonly for GET/HEAD, operator otherwise; `@MinRole('admin')` on secrets,
  lock break, audit, license, WireGuard keypair, log explorer. Denied mutations are audited (401 aggregated).
- **JWT:** HS256 pinned (`algorithms`), issuer/audience checked, `typ: access`, key ring by `kid`; revocation by
  credential generation (password reset, disable, demotion, deletion — PENDING-session-revocation option 1 in
  `syncUsers`) and by session id (logout, refresh reuse).
- **Refresh chain:** rotating, reuse detection revokes the family and its access tokens; refused when the user is
  deleted/disabled/locked or the generation moved; session maximum age.
- **Cookies/CSRF:** the refresh cookie is httpOnly, SameSite=Strict, path `/api/v1/auth`, Secure by default; every
  other route reads the `Authorization` header only (the docs cookie is scoped to `/api/docs`, GET/HEAD) — no ambient
  credential for CSRF.
- **Transport (D-100):** passwords only over TLS or from loopback; the client and its protocol come from the trusted
  proxy walk (`req.ips` stops at the first untrusted hop; X-Forwarded-Proto per hop) — a remote peer cannot forge
  either; the check runs before the rate limiter and argon2.
- **Login/lockout:** argon2 also for unknown users (no username oracle), per-client rate limit, per-(user, address)
  lockout, account lock only from inside a session, the last admin throttled instead of locked, atomic lock decisions.
- **Users/privilege:** `management.users`, AAA and every secret reference are admin-only at edit, stale-lock takeover,
  commit and rollback (`privilegedChanges`, `assertMayApply` in `applyDocument`); import goes through the same edit
  path. Revisions are stored redacted (no password hashes); the only inline secret leaf is `passwordHash`.
- **API keys:** 256-bit random, stored hashed, role capped by the key scope and the user's current role per request,
  disabled users refused; JWT step-up needs the current password (TLS only, per-account budget, counts to lockout).
- **WebSocket:** authenticated in `preValidation` before the upgrade; credential from the `Authorization` header or
  the `bearer.<jwt>` subprotocol, never the URL; closed at credential expiry and on session end; no cookie →
  no cross-site WebSocket hijacking.
- **Break-glass:** root-only CLI; key ring written 0600 via `wx` + fsync + rename, owner kept; audited.
- **Dev switch:** `VRX_DEV_WEAK_PASSWORDS` refused with `NODE_ENV=production`.

## Not covered
API e2e and a live attack run (needs PostgreSQL/Valkey); `apps/api/src/features/**` routes beyond their guard roles.
