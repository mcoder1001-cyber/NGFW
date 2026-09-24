# TD-4 verify — fix round 1 (review `d30d503` H1 / M1 / L1 → D-124)

verifier: independent agent (wrote neither the code nor the review) · envelope `TD-4.verify.md` (manager ngfw-46), focused, 45 min ·
branch `task/TD-4` @ `25a7574` (code `8180648`, contract `220bf32`, docs `30454b9`) · slot 8 · 2026-09-24 19:23–19:32 ·
read: REVIEW-PROMPT, the envelope, TD-4-review.md, TD-4.md "Fix round 1", TD-4-questions.md Q9/Q10, LOG D-100/D-124/D-127, the diff
`7adbb9a..25a7574` (round 1 only: `auth.service.ts`, `auth.controller.ts`, two td4 e2e files, `schema.d.ts`, `TD-4-contract.md`, one user doc).

**Verdict: APPROVE.** All three round-1 items are fixed as asked, and the proof is real: I ran it on the host PostgreSQL and Valkey,
not against mocks. The generated outputs regenerate byte-identical. Nothing regressed in the td4 suite. No new findings block the merge.

## Round-1 items

### H1: per-account budget before argon2, one `locked` body, concurrency e2e — FIXED
- **Order** (`apps/api/src/auth/auth.service.ts:306-329`). The request is checked in this order: transport → 400 `current-not-allowed-with-api-key` →
  400 missing `/current` → `tokens.hit('pwset:<user id>', 60) > VRX_PASSWORD_RATE_PER_MIN` → 429 → `checkCurrent`. The budget is spent
  before the stale-session check, before the `locked_until` read and before argon2. It is a Valkey `MULTI INCR` on
  `rl:pwset:<id>:<minute>`, so it holds for parallel requests. The key is the account id, the same key as `setPassword`
  (`users/users.service.ts:64`, `pwset:${caller.id}`). Both routes that check a current password therefore share one budget, and a
  stolen JWT cannot double its guesses by alternating between them. Nothing costly happens before the budget.
- **One body.** `accountLocked()` (`:20`) now takes no argument. All three `locked` sites (`:351` transaction re-check, `:398` locked at read,
  `:401` the lock a failure caused) return byte-identical bodies. `registerFailure` returns true for **every** failure while
  `locked_until` is in the future, not only for the one that crosses the threshold. Checked-after-lock guesses, right or wrong, therefore
  all answer `locked`. None of them answers `forbidden`, so the right/wrong oracle is gone.
- **The e2e is a real bound.** `test/e2e/td4-stepup-burst.e2e.test.ts` counts every `verifyPassword` call through a pass-through
  `vi.mock`. The API runs in-process, so the count is the API's own argon2 runs. The worker's before/after counts show the counter
  works: 21 before the fix, 5 after. The test asserts that checks ≤ `5 × minute windows touched`, that the rest are 429, that every
  `locked` answer has one raw body, that an `api_key` row exists only if #20 got 201, and that only known answers occur. It also
  checks that the M1 audit reasons match the answers and that no guess appears in `audit_log`.
- **Negative control: judged sound.** At `7adbb9a`, `createApiKey` had no `tokens.hit` (`git show 7adbb9a:…auth.service.ts` →
  only `login:${ip}` at :104). It could not answer 429, so `restRateLimited` (n429 ≥ 30 − budget ≥ 20) fails on the pre-fix code
  **deterministically**, however the burst is scheduled. `oneLockedBody` also failed there, because of the second text at the old :375.
  The worker's paste (`argon2Checks 21, rateLimited 0, distinctLockedBodies 2`, three properties false) matches this, and matches the
  review's own reproduction (21 checks, 2 texts). I did not patch the pre-fix code back: the worktree is read-only for me, and the
  deterministic argument above makes that unnecessary.
- **Re-run by me (slot 8, load 31):**
  ```
  $ eval "$(tools/lab env 8)"; tools/lab lock shared pnpm --filter @ngfw/api test:integration test/e2e/td4-stepup-burst.e2e.test.ts
  H1 step-up burst (right guess at #20, MAX_FAILURES 3, VRX_PASSWORD_RATE_PER_MIN 5, 1 minute window(s)):
    2× 403 forbidden | the current password is wrong  ← #0,1
    3× 403 locked | the account is locked after too many failed password checks  ← #2,3,4
    25× 429 rate-limited | too many password checks; try again in a minute  ← #5,…,29
  H1 summary: {"argon2Checks":5,"rateLimited":25,"lockedAnswers":3,"distinctLockedBodies":1,"rightGuess":"429 rate-limited","keyRows":0} (budget 5)
   ✓ test/e2e/td4-stepup-burst.e2e.test.ts (1 test) 4236ms
   Test Files  1 passed (1) · Tests  1 passed (1)
  ok     nothing named vrx_w8 / vrx_w8 remains                                 exit=0 (19:26:42–19:27:17)
  ```
  The output is identical to the worker's 18:42 and 19:16 pastes. #3 and #4 ran argon2 after the lock and answer with the same `locked` body as #2.

### M1: audit `after.reason` = problem slug on a failed creation — FIXED
`auth.controller.ts:248-263` wraps the service call. On a `ProblemError` it records only `{name, via, reason: e.slug}`: no detail and
no password. Anything else is rethrown untouched. The e2e assert each row shape:
- td4 (2) JWT: `bad-request`, `tls-required` ×2, then the 201 row without a reason.
- Lockout: `forbidden` ×2, then `locked` ×2.
- API-key: `tls-required`, then `current-not-allowed-with-api-key` ×2.
- Burst: all 30 rows match their answers, `rate-limited` included.

### L1 / D-124: `current` with an API key → 400, never checked — FIXED
`auth.service.ts:307-316`. The 400 is thrown before the budget, `checkCurrent`, argon2 and `registerFailure`, so it spends no budget and
adds no lockout count. The transport rule still answers first: a password over remote plain HTTP → 403 `tls-required`. This ordering is
deliberate, documented and asserted. The e2e shows that the right and the wrong password get byte-identical 400s with pointer `/current`,
that `failed_logins` stays 0 and that the key rows are unchanged. No caller hits the new 400: the CLI sends `current` only with a
login/session credential (`TestAPIKeyCreateStepUp`, unchanged this round), and `live.sh` uses a JWT.

### Contract `220bf32`: additive only — OK
The `schema.d.ts` diff has exactly two changes. The `current` description is new, and `Auth_createApiKey` gains a `429`
`application/problem+json` response. No field is reshaped or renamed, and the summaries are unchanged. `TD-4-contract.md` has a
"Fix round 1" section that names the intended behaviour change (D-124). **I regenerated everything into scratch, never over the worktree.**
I compiled the API with `tsc --outDir <scratch>`, then ran `node dist/openapi.js` → openapi.json (`cmp` equal to the worker's ignored
copy). Results:
```
schema.d.ts (openapi-typescript + prettier --config .prettierrc)  : IDENTICAL
apps/cli/internal/api/operations_gen.go (vrx-opgen)               : IDENTICAL
docs/user/cli/reference.md (vrx-docgen)                           : IDENTICAL
sdk/python/vrx/_generated (gen.py; diff -r -x __pycache__)        : IDENTICAL
sdk/terraform/internal/provider/zz_{interface_schema,secrets}_gen.go : IDENTICAL
```

## Regression check
- `td4-auth-hardening.e2e.test.ts` re-run on slot 8 (19:28:51–19:29:25): **10/10 passed**. Its (1)/(2)/(3) output lines match TD-4.md
  exactly, including `secrets: 17 passwords … {"audit":55,"events":6} … found in: []`.
- I did not re-run the whole API e2e or `tools/ci.sh` because the envelope scope is narrow. The worker's 19:16 full run (72 passed | 3
  skipped) is consistent with my two files. The CI log directory `/root/ngfw-wt/logs/ci/TD-4-20260924-190207-2206013` exists and shows
  `Tasks: 30 successful, 30 total`. After `30454b9` only `docs/status/tasks/TD-4*` changed.
- Scope: round 1 touched only the files the review named, plus the regenerated `schema.d.ts` and one paragraph in the user doc. No scope creep.

## Info (no change asked on this branch)
- **V-I1: residual timing.** A `locked` answer that ran argon2 is slower than one refused at read. This tells "checked" from "unchecked",
  never right from wrong, and the budget caps it at `VRX_PASSWORD_RATE_PER_MIN` per account per minute. It is acceptable, and the review
  asked only for one body.
- **V-I2: Q9 agreed.** `UsersService.setPassword` (TD-2 code) still has the two `locked` texts. It is already bounded by `pwset:` before
  argon2, so it is a follow-up row (a two-line change), not a TD-4 blocker. **Q10** is resolved on main by D-127 and arrives with the merge.
- **V-I3: `TokensService.hit` fails open on an EXEC-time reply error** (`auth/tokens.service.ts:281-286`). `[[, n] = [null, 0]]` → `Number(null|undefined)` →
  never `> limit`. A lost connection rejects the call (500, fail-closed). Only an in-transaction reply error such as WRONGTYPE on an `rl:`
  key would skip the limit. This is pre-existing TD-2 code shared with login and `setPassword`, so it could go in the tech-debt list at most.
- **V-I4: re-run hints for the manager.** When I started, every `dist/` in the worktree was absent (the worker's cleanup), so
  `test:integration` fails at collection until `pnpm --filter "@ngfw/api^..." run build`. The envelope's `test:integration -- td4-stepup` form
  does not filter: vitest ran all 9 files. Pass the path without `--`. I built `packages/{proto,schema}/dist` for my runs and removed
  them afterwards.

## Cleanup
Valkey `vrx:w8:*` = 0, `/run/vrx-test/w8` absent, no `vrx_w8` database or role, and the worktree has no tracked changes except this file.
No pkill was used, and ports 3000/8080/9101 were not touched.

**APPROVE**
