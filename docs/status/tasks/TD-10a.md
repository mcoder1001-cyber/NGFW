# TD-10a — API commit engine correctness (REVIEW-2026-09-24, D-125)

Branch `task/TD-10a`, slot 5 (w5, DB `vrx_w5`). Base main@3a6c679; `main` merged in afterwards (P08, W-seed, 869c580-era main: no conflict).
Merge order: after TD-4 (the merger rebases; TD-4 touches another hunk of commit.service.ts — `configResets` — and of pg-repo.ts — `syncUsers`).
No OpenAPI/api-client change (`pnpm --filter @ngfw/api openapi` leaves `packages/api-client/openapi.json` unchanged), so no `contract(…)` commit.

## Items: verified on the current code, then fixed

Each item was checked on main first. None had been fixed already. Every behaviour change has a test that fails on main (evidence below).

| ref | on main (verified) | fix |
|---|---|---|
| 2.1 | `checkPending` compared only `pendingConfirmTxnId` and never read `lastTxnId`. `confirm()` turned every `FAILED_PRECONDITION` into `commit-reverted` and rethrew a timeout. The fake agent set `lastTxnId` on confirm-window applies and was not a faithful model of the agent. | `pendingFate()` reads Health: `last_txn_id == txn` means confirmed and the revision is saved; `pending_confirm_txn_id == txn` means still pending; anything else means reverted. It is used by the deadline watcher, by `confirm()` on FAILED_PRECONDITION and on a lost answer (checked at once; if Health is also unreachable → sync `unknown` + reconcile), and by `reconcile()`, which now knows about pending commits: it promotes a confirmed one and drops a reverted one before it re-applies. The fake agent now follows service.go: `lastTxnId` changes only on a non-window apply or on a confirm, never on a revert. New test hooks: `confirmDelayMs`, `confirmPending()`, `revertNow()`, `deadlines`. |
| 2.2 | `restoreSecrets` was kept only in memory; `setPending` did not persist it. A confirmed rollback (the UI default) therefore restored nothing and pinned the current versions. | Migration `0004_td10a_pending_restore_warnings` adds `config_pending.restore_secrets` and `config_pending.warnings` (jsonb). `PendingCommit.restoreSecrets`/`warnings` are stored by `setPending` and returned by `readPending`. All four promote paths (confirm, watcher, reconcile, ARCH-01 check) pass the value to `promote`. |
| 2.3f | `SecretsService.delete` ran 3 separate SELECTs, then 2 DELETEs, with no transaction and no commit lock. | Now `commits.exclusive(() => db.transaction(…))`: `config_candidate` and `config_pending` are read `FOR UPDATE`, then running is read, then `secret` and `secret_version` are deleted, all in one transaction. |
| 2.4a | Web deadline was 75 s and CLI 90 s, but the server's worst case was 5 + 60 + 60 s plus an unbounded mutex queue. The web called `trackPending` only in onSuccess. | `commit/budget.ts` sets the server budget: lock wait 1 s + health 5 s + DryRun ≤ 30 s + Apply ≤ 60 s + DB 15 s = at most 111 s. DryRun and Apply are capped even when `VRX_AGENT_TIMEOUT_MS` is larger, and the agent sees these as gRPC deadlines. Commit, rollback and confirm return **409 `commit-busy`** (`retryAfterSec: 2`) after the 1 s lock wait instead of queueing. Web `TIMEOUTS.apply` is now 130 s and the CLI `ApplyTimeout` 150 s; other CLI calls keep 90 s. When there is no answer in time, both look up the outcome: the web via `applyOutcome()` in net.ts plus the RevisionsPage rollback hunk (pending / applied / unknown / not-applied, with en+fa strings, and a pending commit goes to the countdown banner); the CLI via `Error.Outcome` (`GET /state/system` + newest revision). |
| 2.4b | `new Mutex()` was in-process only (D-111 known limit). | `CommitLock` in common/mutex.ts is an in-process FIFO plus a **PostgreSQL session advisory lock** (`pg_advisory_lock(0x56525841, 1)`, commit/pg-lock.ts) held on a dedicated pool connection for each section. `run` waits (reconcile, watcher, password set via `exclusive`, secret delete); `tryRun` gives up after the wait (user commit/rollback/confirm). Without a DSN it falls back to in-process only (OpenAPI generator, unit tests). |
| 2.5 (API) | `confirm()` returned `warnings: []`. The 422 `apply-failed` and the `running-unknown` problems had no warnings. | Warnings are stored in `config_pending.warnings` and returned by confirm (including after an API restart). The 422 and `running-unknown` extras now carry `warnings`. The agent/proto part is untouched (tech-debt line, D-125). |
| 5.7b | `http.Client{Timeout: 90s}` had no CheckRedirect. Go 1.26 strips Authorization only when the hostname changes. | `CheckRedirect: http.ErrUseLastResponse`. A 3xx is returned as an error that names the Location, so the header never goes to another scheme or port and a 307/308 body is never replayed. `Raw()` does the same. |
| ARCH-01 (API) | The API never compared Health.last_txn_id with running. | `checkAgentTxn(reason)` runs at **boot** (`resumeSync`), on **agent reconnect** (`AgentClient.watchReady`: gRPC channel READY after not-READY; it keeps reconnecting with backoff ≤ 5 s) and on **RECONCILE_DONE** whose txn is `''` (resync/revert) or not one the API sent (the last 32 own txn ids are remembered). The expected txn is `config_sync.txn_id` when it is in-sync and newer than the running revision, otherwise the revision's txn. The check makes one Health call outside the lock; only a suspected mismatch takes the lock and looks again. A pending commit the agent confirmed is saved; a real mismatch → sync `unknown` → `reconcile()` re-applies running. When nothing was ever applied from this database, there is nothing to compare, so the check skips. |

## Decisions (task-local, for the manager to fold into LOG)

- **D-TD10a-1: `commit-busy` after a 1 s lock wait, not at 0 ms.** Internal sections (the watcher dropping a reverted commit, the ARCH-01 re-check, a password set) hold the lock for milliseconds. A strict try-lock would turn those into spurious 409s, and tests that commit right after a revert already did. The budget counts the 1 s. Internal sections wait without a bound, but each holder is itself bounded by the budget.
- **D-TD10a-2: an advisory lock around each section, not a boot-time "only one API process" guard.** A second process (tools/app next to a unit, or an upgrade overlap) now serialises with the first instead of refusing to start. Promote, password set and secret delete are all covered, which closes the D-111 `syncUsers` before-read limit. The in-memory state (`inflight`, timers) is still per process: HA remains out of the plan.
- **D-TD10a-3: the budget.** `COMMIT_BUDGET_MAX_MS` = 111 s; web 130 s; CLI 150 s. Validate uses the apply deadline on both clients.
- **D-TD10a-4: ARCH-01 skips a database that never applied anything.** With no revision and no sync txn, the check does nothing, so a fresh or restored DB never wipes an agent's state at boot. A revision saved without a txn id causes exactly one re-apply.
- **D-TD10a-5: the secret delete still removes every `secret_version`.** The optional tombstone was not built. A later rollback to a revision that pins a deleted secret fails validation with 400 `secrets.ref-exists`; nothing is corrupted silently.

## Files outside `files_owned` (small hunks, no other owner; see TD-10a-questions.md)

- `apps/api/src/agent/agent.client.ts`: optional `timeoutMs` on `apply`/`dryRun` (the budget) and `watchReady()` (ARCH-01 reconnect). Placed away from the W-seed anchor blocks.
- `apps/api/src/datastore/pg-repo.ts`: `readPending` maps the two new columns. This is the free function, not TD-15's PgConfigTx hunk and not TD-4/TD-10b's syncUsers hunk.
- `apps/api/src/commit/validation.service.ts` (commit/**, owned): the DryRun deadline option.
- `apps/web/src/locales/{en,fa}/revisions.json`: `rollback.outcome.*` (6 keys each).
- `apps/api/src/testing/fake-agent.ts`: besides the Apply/revert hunk, 3 lines in `reset()` and a `deadlines` record in the DryRun handler.
- New files: commit/{budget,pg-lock,commit.engine.test}.ts, migration 0004 + snapshot, test/e2e/td10a.e2e.test.ts, apps/cli/internal/api/transport_test.go, apps/web/src/{net.test.ts,pages/RevisionsPage.test.tsx}.
- The salvage commit 5374c4a (manager, after the session-limit stop) carried a whole-file prettier reformat of RevisionsPage.tsx and net.ts. 5ed9589 undoes it: only the hunks remain (RevisionsPage +75/−11, net.ts +47/−1).

## Evidence

### Tests fail on main first

The main runs overlay only the new tests plus the fake agent's hooks onto `git archive main` (869c580) in the scratchpad. Full logs are in /root/ngfw-wt/logs/TD-10a-main-*.log.

Unit (`apps/api/src/commit/commit.engine.test.ts`) on main: **14 failed | 3 passed (17)**. The 3 that pass are regression guards: a real revert is still dropped, a matching RECONCILE_DONE changes nothing, and a reconcile's re-apply txn counts.
```
   × 2.1 … a confirm whose answer is lost is looked up at once: 200 confirmed, revision saved
   × 2.1 … a confirm retried after a lost answer (agent: FAILED_PRECONDITION) is confirmed, not "reverted"
   × 2.1 … the deadline watcher saves a transaction the agent confirmed instead of dropping it
   × 2.2 … rollback with a confirm window, then confirm: version 1 active and pinned
   × 2.2 … also when the confirm is found through Health (watcher) by a restarted API
   × 2.5 … kept with the pending commit and returned by confirm
   × 2.5 … the 422 apply-failed problem carries them
   × 2.5 … the running-unknown problem carries them
   × 2.4a … a commit while another is in flight is 409 commit-busy at once
   × 2.4a … DryRun and Apply carry the budget deadlines even when VRX_AGENT_TIMEOUT_MS is larger
   × ARCH-01 … RECONCILE_DONE from an agent resync with another last_txn_id → running re-applied
   × ARCH-01 … boot (resumeSync): an agent whose last_txn_id is not running’s is put back on running
   × ARCH-01 … boot: a confirm the agent completed while the API was down is saved at once
   × ARCH-01 … agent reconnect: an agent that comes back with another last_txn_id is put back on running
ProblemError: agent: Deadline exceeded after 0.301s …                      (lost confirm → 504 instead of confirmed)
ProblemError: transaction 02d2a709-… is no longer pending (it was reverted) (confirmed txn called "reverted")
AssertionError: expected undefined to deeply equal { 'psk/tac': 1 }        (restoreSecrets not persisted)
AssertionError: expected 2 to be 1 // Object.is equality                   (confirmed rollback restored nothing)
AssertionError: expected 120024 to be less than or equal to 31000          (DryRun deadline = VRX_AGENT_TIMEOUT_MS)
```

PostgreSQL e2e (`test/e2e/td10a.e2e.test.ts`, slot w5, `flock -s /run/lock/vrx-lab.lock`) on main:
```
   × 2.4b a second API process gets 409 commit-busy while the first one commits → expected 200 to be 409
   × 2.2 + 2.5 … → Failed query: select restore_secrets, warnings from config_pending (column "restore_secrets" does not exist)
(alone) × 2.1 a confirm retried after a lost answer is 200 confirmed … → expected 409 to be 200
(alone) 2.3f race: commit 200, delete deleted, running references it: true, secret exists: false
        × 2.3f … never leaves running on a deleted secret → expected true to be false
```
(In the full-file run on main, 2.1 and 2.3f instead hit the pending commit left behind by the failed 2.2 test, so both were also run alone. On the branch all four pass in one run.)

CLI (`apps/cli/internal/api/transport_test.go`) with main's client.go:
```
--- FAIL: TestRedirectToOtherSchemeAndPortOfSameHostIsNotFollowed
    redirect followed: the http:// target got 1 request(s), Authorization ["ApiKey vrxk_VRX_TEST_KEY_TD10A"]
--- FAIL: TestRedirect307And308NeverReplayTheBody
    307: body replayed to http://localhost:37061: ["{\"password\":\"VRX_TEST_PSK_TD10A\",\"username\":\"admin\"}"]
    308: body replayed to http://localhost:42955: […]
--- FAIL: TestApplyDeadlineIsAboveTheServerBudget
    /api/v1/config/commit: deadline in 1m29.999932592s, want ≥ 140 s (server budget 111 s)
--- FAIL: TestCommitTimeoutLooksUpTheOutcome
    error lacks "txn-td10a": API unreachable (POST /api/v1/config/commit): … context deadline exceeded (Client.Timeout exceeded while awaiting headers)
FAIL	ngfw/cli/internal/api
```

Web (`src/net.test.ts`, `src/pages/RevisionsPage.test.tsx`) on main: **8 failed (8)**: `expected 75000 to be greater than 121000`; `applyOutcome is not a function` (×4); `Unable to find an element with the text: /revision 3 was created: the rollback was applied/i` (and the pending and not-applied variants).

### On the branch (after merging main)

```
apps/api  vitest run (unit)                      Test Files 11 passed (11)   Tests 113 passed (113)
apps/api  commit.engine.test.ts                  Tests 17 passed (17)
apps/api  vitest -c vitest.e2e.config.ts test/e2e (w5, flock -s)
 ✓ test/e2e/td2.e2e.test.ts (14 tests)        ✓ test/e2e/td2-verify.e2e.test.ts (8 tests)
 ✓ test/e2e/config.e2e.test.ts (15 tests)     ✓ test/e2e/td2-review.e2e.test.ts (11 tests)
 ✓ test/e2e/auth.e2e.test.ts (10 tests)       ✓ test/e2e/td10a.e2e.test.ts (4 tests)
 ✓ test/e2e/interfaces.e2e.test.ts (2 tests)  ✓ test/e2e/stream.e2e.test.ts (3 tests)
 Test Files 8 passed (8)   Tests 67 passed (67)
 td10a: 2.3f race: commit 409, delete 409, running references it: false, secret exists: true
apps/web  tsc + eslint clean; vitest run          Test Files 16 passed (16)   Tests 106 passed (106)
apps/cli  go vet, gofmt, golangci-lint (0 issues); go test ./...   all ok
apps/api  tsc + eslint clean; openapi regenerated: no diff
```
CI (`TMPDIR=/tmp/g-w5 tools/ci.sh --base main`, log /root/ngfw-wt/logs/TD-10a-ci.log):
```
== contract guard: HEAD vs main ==   … == lint · typecheck · unit tests · build (turbo) ==   == apps/agent ==   == apps/cli ==
== test/ Go modules, unit mode ==   == deploy/vpp: shellcheck + apply-startup fake-host harness ==   == summary (quick) ==
CI GATE PASSED
```

## Out of scope

- The agent/proto half of 2.5 (ApplyResponse warnings) and of ARCH-01 (a failed save answers DEGRADED): TD-9 / tech-debt.
- `commit-busy` handling in the pending-change bar (CommitDialog / queries.ts are not in files_owned). The bar already shows the 409 problem as the server sends it, and the outcome lookup for commits from the bar is a one-line reuse of `applyOutcome` in CommitDialog, left for its owner. The CLI shows commit-busy as its problem detail.
- A tombstone or refusal for secrets still pinned by old revisions (D-TD10a-5).
- The users.service / auth / state files (TD-4, TD-10b, P08).

## Fix round 1 (TD-10a-review.md @ 3abfd0a: APPROVE WITH CHANGES)

Commits: 2809b1a (API + web), 2cfae25 (H1 e2e, CLI L3/L4, budget note, tech-debt). main merged twice (ba34e80, 574d7a6), no conflicts.

| finding | fix |
|---|---|
| **H1** the lock client is checked out with no `error` listener, so a PG restart or a dropped connection crashes vrx-api | `pg-lock.ts`: `watched(client)` attaches our own `error` listener from checkout to release. `release()` gives the client back first and only then removes the listener, so the client is never without one. A lost client is destroyed (`release(err)`) and never re-pooled. `mutex.ts`: `ProcessLock` returns `Held { release(), lost() }`, and `CommitLock` calls `opts.onLost` after the section. `commit.service.ts` `lockLost()` records the system event `COMMIT_LOCK_LOST`, sets sync `unknown`, and the reconcile re-checks the agent. The section itself finishes normally. |
| **M1** the web says "not applied, try again" while the server's budget can still be running | `net.ts`: `COMMIT_BUDGET_MS = 111_000` and `OUTCOME_WAIT` (budget + 5 s, poll every 3 s). `applyOutcome()` answers `running` rather than `not-applied` until the budget has passed. `RevisionsPage.tsx` `followOutcome()` polls `/state/system` and the newest revision until an outcome is decisive or the budget has passed; closing or resubmitting the dialog ends the follow-up. The `looking` string now says "do not start another one meanwhile". Tech-debt line for P10: nginx `proxy_read_timeout ≥ 130 s` on the apply paths. |
| **M2** secret delete waited without bound behind commits | `CommitService.userExclusive()` = `userSection`: after the 1 s wait it answers 409 `commit-busy` with `retryAfterSec: 2`. `SecretsService.delete` uses it. The same change for `users.service.ts:134` is handed to TD-10b (tech-debt line). |
| **L1** a second API process could undo the other's lost-answer recovery | `checkAgentTxn` re-reads `config_sync` from the database inside the lock and does nothing unless it says `in-sync`. |
| **L2** the lock wait was not bounded by pool exhaustion | `tryAcquire` races `pool.connect()` against the wait; a late client goes back to the pool. The cross-process wait gets at least 250 ms. `budget.ts` states that the DB 15 s margin is an assumption; tech-debt goes to TD-15 (pool `connectionTimeoutMillis`, `statement_timeout`). |
| **L3** outcome heuristics could name the wrong commit | Web: a pending commit counts only if `createdAt ≥ sentAt − 5 s`; an older one is someone else's. CLI: reads the newest revision id before a commit, rollback or confirm and reports "new since the request: revision N … it was applied" by id, not by the server clock. A pending commit is reported as a fact ("a commit IS pending: txn …, created …"), not as "yours". |
| **L4** test gaps | New unit test for the confirm path "no answer and no Health → `running-unknown` → reconcile finds it confirmed → revision saved". The CLI outcome test is now driven by `c.ApplyTimeout = 300ms` (the per-request deadline) instead of `http.Client.Timeout`. The H1 tests are below. |
| L5 | Confirm-banner retry on `commit-busy`: tech-debt line for the owner of `apps/web/src/config/**`, together with CommitDialog reusing `followOutcome`. |

D-132 does not apply: the follow-up polls `/state/system` (agent Health, an in-memory snapshot with no VPP walk) and `/config/revisions` (database).

### Evidence

Tests fail before the fix (log `/root/ngfw-wt/logs/TD-10a-fix1-prefix-fail.log`, run on 2809b1a's tests with the pre-fix code):
```
   × H1: losing the lock connection mid-section > run: the process survives, the section finishes, …
   × H1: losing the lock connection mid-section > tryRun: the process survives, the section finishes, …
   × H1: losing the lock connection mid-section > CommitService: a commit whose lock connection drops completes; …
   × fix round 1 > M2: a secret delete during a commit is 409 commit-busy after the lock wait, not queued      → Error: expected a problem
   × fix round 1 > L1: a second API process does not undo the first one’s lost-answer recovery                → expected 'unknown' to be 'in-sync'
   ✓ fix round 1 > L4: confirm with no answer and no Health → running-unknown; …                               (coverage for an existing path)
H1 failures: uncaught "Connection terminated unexpectedly" (pg Client error event without listener) → expected [ …(1) ] to deeply equal []
```
H1 on real PostgreSQL (`pg_terminate_backend` of the session that holds the advisory lock while the agent Apply runs), with the pre-fix API code (`TD-10a-fix1-e2e-H1-prefix.log`):
```
   × TD-10a commit engine on PostgreSQL > H1 (fix round 1) the PostgreSQL backend holding the commit lock is terminated mid-commit …
⎯⎯⎯⎯⎯⎯ Unhandled Errors ⎯⎯⎯⎯⎯⎯
error: terminating connection due to administrator command
Error: Connection terminated unexpectedly
      Tests  1 failed | 4 skipped (5)      Errors  2 errors
```
Web, new tests against the pre-fix net.ts and RevisionsPage.tsx (`TD-10a-fix1-web-prefix-fail.log`), then restored:
```
   × M1: nothing new while the server may still be working (early network error, proxy 504): running, never not-applied → expected { kind: 'not-applied' } to deeply equal { kind: 'running' }
   × L3: a pending commit older than the request is someone else’s, not this one → expected { kind: 'pending', …(2) } to deeply equal { kind: 'not-applied' }
   × the budget constant matches the server → expected undefined to be 111000
   × reports "not applied" when nothing changed — only after the server budget has passed → Unable to find … /following up what became of the rollback/i
   × M1: an early network error while the rollback still runs → never "not applied"; the late revision is reported → Unable to find … /revision 3 was created/i
      Tests  5 failed | 7 passed (12)
```
After the fix:
```
apps/api  vitest run (unit)                  Test Files 12 passed (12)   Tests 119 passed (119)   (tsc + eslint clean)
apps/api  td10a e2e (w5, :3500 free, flock -s)
 ✓ 2.4b a second API process gets 409 commit-busy while the first one commits
 ✓ H1 (fix round 1) the PostgreSQL backend holding the commit lock is terminated mid-commit: the API survives, running is re-checked
 ✓ 2.2 + 2.5 a confirmed rollback confirmed by a restarted API restores the secret versions; warnings survive
 ✓ 2.3f a secret delete with a commit landing between its reads never leaves running on a deleted secret
      Tests  5 passed (5)       (2.3f race: commit 409, delete 409, running references it: false, secret exists: true)
apps/web  net.test.ts + RevisionsPage.test.tsx  Tests 12 passed (12)   (tsc + eslint clean)
apps/cli  go vet + golangci-lint 0 issues; go test ./... all ok
```
(5 of 5 passed: the fifth, the 2.1 case, ran in under 300 ms, and vitest's default reporter lists only slower tests by name.)

CI (`TMPDIR=/tmp/g-w5a tools/ci.sh --base main` on 574d7a6, log `/root/ngfw-wt/logs/TD-10a-fix1-ci.log`):
```
no contract files changed in the 11 commit(s) of HEAD since main (ae359680)
== lint · typecheck · unit tests · build (turbo) ==  == apps/agent ==  == apps/cli ==  == test/ Go modules ==  == deploy/vpp ==
== summary (quick) ==
CI GATE PASSED
```
