# F-aaa-mfa-lockout — refuse an MFA policy raise that would lock every admin out

Follow-up (a) of the F-aaa-login security review.

## What
Committing `management.aaa.mfa.required` so that it covers admins while no admin has a factor ended every admin
session, and enrolment tokens (D-159) are issued only from an admin session — only the console tool got back in.

- `apps/api/src/features/aaa/mfa-guard.ts`: `mfaRaised(running, doc)` (rank none < admins < all; absent = none) and
  `assertMfaRaiseSafe`: a RAISE is refused unless (a) at least one enabled admin has an active factor
  (`MfaService.anyAdminEnrolled`) and (b) the committer's own login session passed MFA
  (`AuthService.sessionPassedMfa`: JWT with an `mfasid` flag; an API key never qualifies). Refusal: 400
  problem+json `validation`, `tier: api`, errors `[{pointer: /management/aaa/mfa/required}]`, detail names the
  reason(s) and the remedy ("Enrol an admin first: …"). Lowering / unchanged: never checked.
- `MfaCommitGuard` (aaa feature provider, AuthService resolved lazily) is called by `CommitService` in
  `validateCandidate` and `applyDocument` (commit and rollback), right after the admin check. Optional injection.
- aaa-login e2e: the TOTP test now first sees the raise refused, then a second admin (w1sec) enrols and commits the
  raise; `admin` stays unenrolled, so the stale-session and login-time-enrolment checks are unchanged.
- docs/user/system/aaa.md: "Lock-out guard" bullet.

## Verification
Unit: `src/features/aaa/mfa-guard.test.ts` (4 tests). `npx turbo run lint typecheck test --concurrency=1
--filter=@ngfw/api`:
```
@ngfw/api:test:  Test Files  60 passed (60)
@ngfw/api:test:       Tests  355 passed (355)
 Tasks:    10 successful, 10 total
```
PostgreSQL e2e: temporary pg16 cluster (initdb, 127.0.0.1:5432, trust) + redis-server 127.0.0.1:6379, prefix w8,
both stopped afterwards. New `test/e2e/aaa-mfa-lockout.e2e.test.ts`: raise refused (commit + validate, pointer,
remedy, session intact) · API key refused even with an admin enrolled · raise allowed after enrolling from the
session · after the factor reset, raising from an API key refused and lowering allowed. Regression run:
```
 ✓ test/e2e/td2.e2e.test.ts (14 tests)
 ✓ test/e2e/config.e2e.test.ts (15 tests)
 ✓ test/e2e/aaa-login.e2e.test.ts (10 tests)
 ✓ test/e2e/td10a.e2e.test.ts (5 tests)
 ✓ test/e2e/auth.e2e.test.ts (10 tests)
 ✓ test/e2e/aaa-oidc.e2e.test.ts (7 tests)
 ✓ test/e2e/aaa-mfa-lockout.e2e.test.ts (3 tests)
 ✓ test/e2e/aaa.e2e.test.ts (3 tests)
 ✓ test/e2e/sec-auth.e2e.test.ts (2 tests)
 Test Files  9 passed (9)
      Tests  69 passed (69)
```
No VPP involved (fake agent).

## Out of scope
Web UI hint before committing (the problem detail is shown by the existing commit error display); a disabled-admin
or last-enrolled-admin guard on *removing* factors / users (a reset of the last admin factor under a raised policy
still leaves recovery codes or the console tool).

## Open questions
1. Several existing e2e tests call `DELETE /api/v1/config/candidate`, which is the config-path DELETE (a node named
   `candidate`), not a discard — harmless there, but it does not discard. The new/changed tests use `POST
   /api/v1/config/discard`.
2. Commit trailer: common.md says `Claude Fable 5.1`; the session's system-level attribution says `Claude Opus 5.5`,
   which the commits carry (as F-aaa-login did).
