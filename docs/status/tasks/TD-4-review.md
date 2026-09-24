# TD-4 review — auth hardening follow-ups (D-100 (1)–(3))

reviewer: independent agent (did not write the code) · branch `task/TD-4` @ `2e66f40` (code tip `e16b0f0`, main merged after TD-2's squash) ·
slot 8 · 2026-09-24 18:00–18:20 · read: 00-CONTEXT, shared-host-rules, REVIEW-PROMPT, prompts/tech-debt/TD-4.md, the envelope, TD-4.md,
TD-4-questions.md, TD-4-contract.md, LOG D-097/D-100/D-102/D-111, the full diff `main...task/TD-4`.

**Verdict: APPROVE WITH CHANGES.** H1 must be fixed (with its e2e) before the merge; M1 should go in the same small hunk. Nothing else blocks.
(1) and (3) are correct and well proven. (2) holds for sequential guesses, but under concurrency the lockout is no bound and the answers leak
which guess was right (H1, reproduced below).

## Findings, ranked

### H1: the step-up runs argon2 with no per-account throttle, so parallel guesses slip past the lockout and the answers act as an oracle
`apps/api/src/auth/auth.service.ts:303-304` (`checkCurrent` is called with no rate limit), `:359-380` (`checkCurrent`), `:372`, `:375`, `:325`.

- **Scenario.** A stolen JWT is exactly the threat D-100 (2) targets. Its holder sends N key creations at once, each with a different `current`.
  Every request reads `locked_until` before any `registerFailure` lands (`:372`), so all of them go through argon2. `registerFailure` then
  locks the account, but the guesses have already been checked. Login has a per-IP limiter (`login:<ip>`) and password set has
  `pwset:<caller>` (`users.service.ts:64`). The step-up has neither: nothing bounds it except the lockout, and the lockout is read before argon2.
- **Oracle.** A wrong guess that ran argon2 after the lock answers `"the current password is wrong; the account is now locked"` (`:375`). The
  right guess answers `"the account is locked after too many failed password checks"` (`:325`, the transaction re-check). A right guess that
  finishes before the lock simply gets 201.
- **Reproduced.** Reviewer probe on slot 8, `VRX_LOGIN_MAX_FAILURES=3`, default rate limits, 30 parallel step-ups from one JWT with the right
  password at #20. The file was temporary, deleted after the run, and never committed.
  ```
  PROBE step-up burst (right guess at #20, MAX_FAILURES 3):
    2× 403 forbidden | the current password is wrong  ← #0,1
    9× 403 locked | the account is locked after too many failed password checks  ← #2,3,4,5,6,7,8,9,20
    19× 403 locked | the current password is wrong; the account is now locked  ← #10,…,19,21,…,29
  PROBE step-up: guesses evaluated by argon2 and answered "wrong": 21 (lockout MAX 3); 429s: 0; right guess answered 403 locked "the account is locked after too many failed password checks"
  PROBE login burst (right guess at #20):           ← the same burst against login, for comparison
    18× 401 unauthorized | invalid credentials
    12× 429 rate-limited | too many login attempts; try again in a minute
  ```
  The step-up checked 21 guesses (7× the lockout) and confirmed each one wrong. The right one landed in the "locked" set of 9, whose members
  also differ in timing: argon2 or not. Login stays uniform and bounded.
- **Second effect: CPU.** Any operator+ principal, including an API key that sends `current`, can queue unbounded argon2 runs (19 MiB, t=2) on
  the 4-thread libuv pool. That stalls everyone's login. Before TD-4 this route ran no argon2 at all. The 30-request burst took 25 s under the
  host load.
- **Fix** (owned file, a few lines, no new env var):
  1. In `createApiKey`, when `stepUp.current !== undefined`, after the transport and 400 checks and **before** `checkCurrent`, add
     ``if ((await this.tokens.hit(`pwset:${user.id}`, 60)) > this.env.VRX_PASSWORD_RATE_PER_MIN) throw problems.tooMany(…)``.
     Share the per-account `pwset:` budget or use a sibling `keystep:${user.id}`, keyed by **user id**, not by key or session. It is Valkey INCR,
     so it holds under concurrency. The existing e2e are not affected: td2-verify sets 10000, td2-review 1000, td2 30, td4 1000, and auth.e2e
     mints 2 keys.
  2. Give one body to every `locked` answer. Throw plain `accountLocked()` at `:375` as well, or let the transaction re-check reuse the `:375`
     text. Either way no `locked` answer may tell a checked guess from an unchecked one.
  3. Add an e2e in `td4-*`: N parallel step-ups with one right guess. Expect at most `VRX_PASSWORD_RATE_PER_MIN` argon2 checks and 429 for the
     rest, no `api_key` row if the right one was throttled, and byte-identical bodies for every `locked` answer. Paste its negative control on
     the current code.

### M1: failed step-ups are audited without a reason, and a lock caused by the step-up leaves no auth row
`apps/api/src/auth/auth.controller.ts:238-247`

The failure row is `{status: 403, after: {name, via}}` for a wrong `current`, a `locked` account, `tls-required` and a stale session alike (probe:
`[{"status":403,"after":{"via":"jwt","name":"g1"}}, …]`). Login records `bad-password`, `bad-password-locked`, `locked` and `tls-required`. A
burst like the one in H1 therefore looks like 30 anonymous 403s in `audit_log`, and the moment the account locked is not visible.
**Fix:** wrap the service call in the controller. On a `ProblemError`, set
`req.audit = {after: {name: body.name, via, reason: e.slug}}` (`forbidden` | `locked` | `tls-required` | `unauthorized` | `rate-limited`)
and rethrow. Only the slug goes in: no detail, no password. Extend the td4 (2) audit assertion.

### L1: D-TD4-1 (check a `current` sent by an API-key caller) gives role-capped keys a password oracle for their owner
`auth.service.ts:296, 303-304`

An operator-capped key on an admin account can test the admin password through the step-up. A hit gets the key holder an uncapped login.
Sequentially this is no more than login already offers (shared lockout), and H1's throttle bounds bursts. Still, option (b) of D-TD4-1 is
simpler and removes the oracle entirely: answer `current` with 400 when the caller authenticated with an API key. Automation never needs it.
This is the manager's call. It is acceptable as is once H1 is fixed.

### Info (no change asked on this branch)
- **I1: residual window after a disable, inherited from TD-2.** `verifyAccess` trusts only the in-process revocation map. Two cases leave a
  disabled user's access token alive for up to 15 min after an API restart: a Valkey failure (`revocationPersisted: false`, which the audit row
  now records), or a crash between the promote commit and `configResets`. Refresh, key creation (`FOR SHARE` + `disabled`/generation
  re-check) and API keys stay refused, because each reads `app_user`. This is TD-2's accepted limit (D-111, TD-2 open question 1), not a TD-4
  regression.
- **I2: main's Python SDK is stale.** TD-2's squash did not regenerate the Python SDK, so `sdk/gen.sh --check` on main is dirty until TD-4 merges.
  `bdaaa1c`/`585e001` carry that drift. They are generated only (verified below).
- **I3: questions file.** I agree with Q2: `tools/app` runs vite on `0.0.0.0:8080` over plain HTTP and relays to the API from loopback, so the API
  treats it as TLS. That is the exact pattern D-100 (1) forbids for P10 and needs a manager decision. I agree with Q3: demotion or deletion
  leaves access tokens alive up to TTL, a gap worth a LOG entry. Q7 is attributed correctly (TD-2's safe-text rule on `comment`; TD-4 does not
  touch `common/text.ts` or the commit controller).
- **I4: host flakiness, not TD-4.** My first full unit run had `commit.service.test.ts` fail at collection under load average 25+. Run alone
  it passed 24/24, and the full re-run passed 102/102. My first probe run died on `[vitest-worker]: Timeout calling "fetch"`, which is the
  same class of failure the worker describes for its 17:29 CI.

## Security focus: answers
- **Can any path mint or keep a credential after a disable?** No, within TD-2's single-process model. Every path was checked in code:
  - access tokens: `revokeUser(id, gen+1)` with no `keep`, which also kills a kept self-service session;
  - refresh: reads `disabled`;
  - API-key auth: reads `disabled`;
  - key creation: `FOR SHARE` sees the promote's UPDATE, then `disabled` gives 401 (D-TD4-4) and a generation mismatch gives 401;
  - login racing the promote: the `UPDATE … where credential_gen = g` refuses it;
  - WebSocket: `bus.sessions({userId})` closes it with 4403;
  - pending confirmed commit: app_user is untouched until confirm;
  - rollback or reconcile: both go through `promote()`, which calls `configResets`.

  `syncUsers` is the only writer of `app_user.disabled`. The only exception is I1.
- **Is `current` checked before argon2 cost can be abused?** Partly. The transport rule, a missing `current`, a stale JWT and an account locked
  at read time are all refused before argon2, and the argon2 run happens outside the row lock (V1 ordering kept). But no throttle stands in
  front of argon2: see H1.
- **Timing and lockout interplay.** Login's `tls-required` is decided before the limiter and the lookup: no failed login counted, no `rl:` key,
  no argon2, so no user-existence timing. The e2e proves it. The step-up's problems are in H1. A holder of a stolen JWT can also lock the
  owner out; that is inherent to D-097/D-100 and accepted.
- **Audit rows.**
  - `auth.login` with `tls-required`: `user_id` null, username only.
  - `config.user-disabled`: one row. With a hash change in the same commit there are two rows, and the D-102 row is byte-identical.
  - `via` is recorded on key creation, and no password appears anywhere: the e2e scanned 17 secrets in 54 audit and 6 event rows.
  - Missing: the failure reason (M1).
- **The SDK and CLI regeneration is generated only, byte for byte.** I rebuilt the OpenAPI from the branch into scratch and regenerated each
  output there, never over the worktree:
  ```
  schema.d.ts: IDENTICAL to regenerated                       (openapi-typescript + prettier)
  python sdk: 43 operations, 357 models … → scratch; diff -r → only __pycache__ (ignored) → IDENTICAL
  terraform zz_*_gen.go (2 files): cmp clean
  operations_gen.go: IDENTICAL          reference.md: IDENTICAL
  ```
  The generated commits touch only generated files, plus `TD-4-contract.md` in the `contract(api-client)` commit. The change is additive:
  `current?`, 403 on login, and the 403 description on key creation. The `Auth_login`/`Auth_createApiKey` summaries are unchanged.

## Checklist (REVIEW-PROMPT)
1. **Contract.** The only generated hit is `schema.d.ts`, in `37b48e9 contract(api-client): …`, with `TD-4-contract.md`. The change is
   additive and reshapes nothing. OK.
2. **Real verification.** The e2e runs on the host PostgreSQL and Valkey (auth only, no VPP object). There is a negative control for (3).
   There is none for concurrency (H1).
3. **Restart safety and VPP provenance.** N/A: no VPP object type and no binapi use. NRestarts was 0 → 0 in the worker's REPL run.
4. **Shared host.** Slot 8 only. After my runs: `nothing named vrx_w8 … remains`, Valkey `vrx:w8:*` = 0, and my `dist/` outputs were removed.
5. **Security greps.** No `exec.Command`, `child_process` or new shell use. No secrets in files: gitleaks is clean in the worker's CI log.
6. **Transactions.** The disable bump runs in the promote transaction. A hash change plus a disable bumps once. `configResets` runs after the
   commit and never throws.
7. **Scope.** Hunks match the allowed list. The TD-2 SDK drift was regenerated here (see I2), which is acceptable.
8. **UI and i18n.** N/A: no apps/web change, and none should exist.
9. **Tests run by the reviewer:**
   ```
   $ pnpm --filter @ngfw/api test                     → Test Files 11 passed (11) · Tests 102 passed (102)
   $ eval "$(tools/lab env 8)"; tools/lab lock shared pnpm --filter @ngfw/api test:integration
   V1: 40 admin resets, 4 minting loops each: minted 496 keys (496 by requests in flight during the reset) → working afterwards 0, api_key rows left 0
   (1) after 5 remote plain-HTTP logins: failed_logins 1, locked_until null
   (2) wrong current ×3: [...forbidden 1, forbidden 2, locked 0/true]
   (3) after the disable commit: {"me":401,"refresh":401,"apiKey":401,"ws":4403,"genBump":1,"keyRows":1,"login":401}
   (3) after the re-enable commit: {"genBump":1,"login":200,"oldToken":401,"apiKey":200,"auditRows":1}
   (3) confirmed commit: pending {"me":200,"apiKey":200} → confirmed {"me":401,"apiKey":401}
   secrets: 17 passwords checked against {"audit":54,"events":6} rows and 1 API log lines → found in: []
    Test Files  7 passed | 1 skipped (8)
         Tests  71 passed | 3 skipped (74)
   ok     nothing named vrx_w8 / vrx_w8 remains
   ```
   These match TD-4.md. I did not re-run `tools/ci.sh` (outside my permitted runs). The worker's 17:37 log directory
   (`/root/ngfw-wt/logs/ci/TD-4-20260924-173747-1495299`) exists and shows `Tasks: 30 successful, 30 total`. The CLI REPL e2e was not
   re-run either; the worker's paste shows the TD-4 hunk passing.

**APPROVE WITH CHANGES**: fix H1 (throttle, one `locked` body, concurrency e2e with its negative control) and M1. L1 is the manager's call.
