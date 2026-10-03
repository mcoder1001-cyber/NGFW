# F-aaa-hardening — status

The three non-blocking leftovers of the F-aaa-login security review. No new routes, no contract change.

## What
1. **API keys vs the MFA policy — refuse-at-use** (`api_key.mfa_verified`, migration 0008). At mint time the key
   records whether the caller was a login session that passed the second factor (or a key that itself was minted so).
   On use, while `management.aaa.mfa.required` covers the key OWNER's role (not the key's role cap) and the key has
   `mfa_verified = false`, the request is refused: 401 problem `mfa-required`. Fails closed when the policy cannot be
   read (like Bearer sessions). Chosen over revoke-on-raise: nothing is destroyed, lowering the policy revives the
   keys. The policy is read through the existing 5 s cache, so a raise takes effect within 5 s. Existing keys migrate
   as `false` (not MFA-minted). Consequence: after a raise, an unenrolled admin's recovery path is an MFA-minted key
   or the console break-glass tool; the F-aaa-mfa-lockout e2e now mints its recovery key from the MFA session.
2. **JWKS reuse**: `jwksFor(issuer, jwks_uri)` in `features/aaa/oidc.ts` keeps one `createRemoteJWKSet` per issuer
   (jose caches keys per instance); a different discovered `jwks_uri` replaces it.
3. **Case-insensitive uniqueness**: `app_user_username_lower_uq` = unique index on `lower(username)` (migration 0009).
   Total, not partial: local names are lower-case by the schema's `username` primitive (`^[a-z_][a-z0-9_-]{0,31}$`),
   external names are case-folded before insert, the bootstrap name is `admin` — existing data cannot violate it.
   Both migrations generated with `pnpm db:generate` (drizzle-kit); both are additive (undo: `DROP INDEX
   app_user_username_lower_uq`; `ALTER TABLE api_key DROP COLUMN mfa_verified`) — drizzle has no down migrations.

## Verification
Unit: `src/auth/apikey-mfa.test.ts` (4), `src/features/aaa/oidc.test.ts` (+1 JWKS reuse).
`npx turbo run lint typecheck test --concurrency=1 --filter=@ngfw/api`:
```
@ngfw/api:test:  Test Files  62 passed (62)
@ngfw/api:test:       Tests  365 passed (365)
 Tasks:    10 successful, 10 total
```
PostgreSQL e2e: temporary pg16 cluster (initdb in /tmp/ngfw-pg-w9, 127.0.0.1:5432, trust) + redis-server
127.0.0.1:6379, prefix w9, both stopped afterwards (data dir left in place). `aaa-mfa-lockout` extended (pre-MFA key
→ 401 mfa-required after the raise, MFA-minted key works, pre-MFA key works again after lowering); new
`aaa-username-ci` (index present, `W9Case` after `w9case` refused).
```
 ✓ test/e2e/td4-auth-hardening.e2e.test.ts (10 tests)
 ✓ test/e2e/td2.e2e.test.ts (14 tests)
 ✓ test/e2e/config.e2e.test.ts (15 tests)
 ✓ test/e2e/aaa-oidc.e2e.test.ts (7 tests)
 ✓ test/e2e/aaa-mfa-lockout.e2e.test.ts (3 tests)
 ✓ test/e2e/aaa-login.e2e.test.ts (10 tests)
 ✓ test/e2e/auth.e2e.test.ts (10 tests)
 ✓ test/e2e/aaa.e2e.test.ts (3 tests)
 ✓ test/e2e/sec-auth.e2e.test.ts (2 tests)
 ✓ test/e2e/aaa-username-ci.e2e.test.ts (1 test)
 Test Files  10 passed (10)
      Tests  75 passed (75)
```
`tools/ci.sh check`: `check PASSED`.

## Out of scope
Revocation of keys on a policy raise; showing `mfa_verified` in the key list/UI (would be an additive contract change).

## Open questions
1. ~~A key stays `mfa_verified` after its owner's factor is reset.~~ Resolved by S-aaa-key-reset: an admin factor
   reset (`DELETE /auth/mfa/users/:name`, the only way a factor is removed — there is no self-service disable) clears
   `mfa_verified` on all of the user's keys in the factor-delete transaction; audited as `apiKeysMfaCleared` (count).
   Unit `src/features/aaa/mfa-reset.test.ts` (2); e2e `aaa-key-reset` (mint from MFA session → 200, reset → 401
   mfa-required, re-enrol + new key → 200); `aaa-mfa-lockout` test 3 now lowers the policy from the MFA-minted key
   before the admin's own factor is reset (after the reset that key is refused while the policy covers admins).
2. Commit trailer: common.md says `Claude Fable 5.1`; the session's system-level attribution says `Claude Opus 5.5`,
   which is used (as in F-aaa-login).
