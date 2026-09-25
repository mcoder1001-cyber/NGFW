# TD-10a — focused verify of fix round 1

Reviewer: the author of TD-10a-review.md (3abfd0a). Branch `task/TD-10a` @ c365a95; the fixes are 2809b1a and 2cfae25,
and main was merged in at ba34e80 and 574d7a6. Scope: only the review's findings, the new tests, regressions and the
hand-offs. Time box 45 min.

**Verdict: APPROVE.** Every finding is fixed, and every behaviour fix comes with a test that fails on the old code. Nothing
regressed. The nits below are optional and do not block the merge.

## Runs (this verify)

| run | result |
|---|---|
| `pnpm --filter @ngfw/api test` | `Test Files 12 passed (12)  Tests 119 passed (119)` |
| `pnpm --filter @ngfw/web test` | `Test Files 16 passed (16)  Tests 110 passed (110)` |
| td10a e2e, slot 5 (`:3500` free, `flock -s /run/lock/vrx-lab.lock`) | `Tests 5 passed (5)`, including `H1 … backend … terminated mid-commit`; `2.3f race: commit 409, delete 409, running references it: false, secret exists: true` |
| `go test ./internal/api/` (apps/cli) | `ok` |
| negative control: new CLI test against the pre-fix `client.go` (3abfd0a) | `FAIL TestCommitTimeoutLooksUpTheOutcome/applied` + `/nothing_new` (lacks "new since the request: revision 8 …", "no new revision since the request") |
| pre-fix logs `/root/ngfw-wt/logs/TD-10a-fix1-{prefix-fail,e2e-H1-prefix,web-prefix-fail}.log` | real, and they match TD-10a.md: unit 5 failed / 1 passed (L4 is coverage of an existing path), e2e H1 `1 failed … Errors 2 errors` (unhandled `Connection terminated unexpectedly`), web 5 failed / 7 passed |
| `TD-10a-fix1-ci.log` | ran on 574d7a6, `CI GATE PASSED`, no contract files |

## Findings: verified

- **H1: fixed.**
  - `apps/api/src/commit/pg-lock.ts:64` (`watched`) attaches our own `error` listener from checkout until the client is
    given back.
  - `:77`: the client is released first (pg-pool re-adds its idle listener in `_release` on every path, including
    `release(err)`), and only then is our listener removed, so the client is never without one.
  - A lost client is destroyed and never goes back to the pool. `tryAcquire` stops polling once the lock is lost.
  - `common/mutex.ts`: `Held.lost()` → `onLost` runs after the section.
  - `commit.service.ts:284` `lockLost` records `COMMIT_LOCK_LOST`. `SystemEventsService.record` never throws, so the next
    step is reached even with PostgreSQL down. It then sets sync `unknown` and reconciles, which at worst re-applies
    running once, harmlessly.
  - Tests: `pg-lock.test.ts` (a fake wire server; run, tryRun, and a whole CommitService commit) and the e2e with a real
    `pg_terminate_backend` of the lock holder. Both fail on the old code with the unhandled error.
- **M1: fixed.**
  - `apps/web/src/net.ts:113` returns `running` instead of `not-applied` until `sentAt + 116 s` (`OUTCOME_WAIT`, `:69`).
  - `RevisionsPage.tsx:101` `followOutcome` polls every 3 s, and closing or resubmitting ends it (generation ref). The
    submit button stays disabled while the rollback is being followed.
  - The test "late revision reported as applied" (lands after 1.5 s) fails on the old code.
  - A test checks that `COMMIT_BUDGET_MS` equals the server's 111 000.
- **M2: fixed.** `secrets.service.ts:155` → `commits.userExclusive` (`commit.service.ts:477` = `userSection`) returns 409
  `commit-busy` after the 1 s wait, and the delete transaction never starts. The unit test fails on the old code (it queued).
  The 2.3f e2e keeps its meaning.
- **L1: fixed.** `commit.service.ts:1112` re-reads `config_sync` inside the lock. The two-service test fails on the old code
  (`expected 'unknown' to be 'in-sync'`, meaning B had re-applied).
- **L2: fixed.** `pg-lock.ts:101` `connectWithin` bounds `pool.connect()` by the wait, and a late client goes back to the
  pool. `budget.ts` now states that the DB margin is an assumption; the enforcement is handed to TD-15.
- **L3: fixed.**
  - CLI `client.go:314` reads the newest revision id before commit, rollback or confirm. `:185` decides "applied" by id, and
    reports a pending commit as a fact, not as "yours". The negative control above fails on the old code.
  - Web: the revision decision uses `beforeRevision` (the id). A pending commit counts only if
    `createdAt ≥ sentAt − 5 s` (`net.ts:106`).
- **L4: fixed.** The confirm path "no answer and no Health → running-unknown → reconcile saves it" has a unit test. It passes
  on the old code too: it covers a path that already worked and is not a behaviour fix. The CLI test is now driven by
  `ApplyTimeout`.

## Regressions and the main merge

- `git diff --name-only main...task/TD-10a` lists only TD-10a's files, the envelope's out-of-list hunks and
  `docs/tech-debt.md` (an appended section).
- Both merges of main are clean: `git diff-tree --cc` shows no conflict hunks.
- The merged agent code (TD-8 seams) keeps the `LastTxnID` semantics the state machine relies on: it is set only on confirm
  (`service.go:356`) and on a non-window apply (`:416`).
- Every suite above is green.

## Hand-offs (docs/tech-debt.md:90)

All five are real follow-ups, each with an owner. None is an obligation of TD-10a that was quietly dropped:

- P10: nginx `proxy_read_timeout ≥ 130 s`.
- TD-15: pool `connectionTimeoutMillis` and `statement_timeout`.
- TD-10b: password set → `userExclusive`. That file is outside TD-10a's scope; the manager should tell TD-10b directly.
- The owner of `apps/web/src/config/**`: CommitDialog follow-up, and the confirm banner's retry on `commit-busy`.
- Secrets tombstone (D-TD10a-5).

## Nits (optional, non-blocking)

1. `net.ts:106`: the pending check still compares the server's `createdAt` with the client clock. If the client clock runs
   more than 5 s ahead, a pending commit created by this rollback is ignored and, after 116 s, reported as "not applied".
   A retry is still safe (409 `commit-pending`), so no double commit follows. A cleaner test: any pending commit that was
   not there before the request is ours. A new commit is refused while one is pending, so the "before" snapshot from
   `usePending` decides this without clocks.
2. The tech-debt line says `followOutcome` is in `net.ts`, but it lives in `RevisionsPage.tsx:101`. The CommitDialog owner
   will need to move it to `net.ts` to reuse it.
3. `mutex.ts:47` `MIN_CROSS_WAIT_MS = 250`: the user lock wait can reach 1.25 s, not the 1 s that `budget.ts` states
   (total 111.25 s, still well below the web's 130 s). One comment line in `budget.ts` would cover it.

**APPROVE**
