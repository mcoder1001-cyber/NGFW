# TD-10a — review (API commit engine correctness)

Reviewer: independent review agent. Branch `task/TD-10a` @ 30381b6 (code @ aa1c7f3, main merged in). Diff base
`main...task/TD-10a` (merge-base 0ae3559). Read: 00-CONTEXT, REVIEW-PROMPT, docs/01, docs/04, the envelope, TD-10a.md,
TD-10a-questions.md, the whole diff, and the agent's `apps/agent/internal/agent/service.go` (what Health.last_txn_id and
pending_confirm_txn_id really mean).

**Verdict: APPROVE WITH CHANGES.** The confirmed-vs-reverted state machine is correct against the real agent's semantics.
It is merge-ready once H1 is fixed; M1 and M2 are small and belong in this branch too.

## What I ran (no host runs, no e2e, no VPP)

| check | result |
|---|---|
| `pnpm --filter @ngfw/api test` (branch) | `Test Files 11 passed (11)  Tests 113 passed (113)`, matches TD-10a.md |
| `go test ./internal/api/` in apps/cli (branch) | `ok ngfw/cli/internal/api 0.394s` |
| main-failure logs `/root/ngfw-wt/logs/TD-10a-main-*.log` | real: unit 14 failed / 3 passed (the 3 are regression guards), e2e 3/4 full-file + 2.1 and 2.3f run alone, CLI 4 FAIL, web 8/8 failed. They match the pasted evidence |
| contract guard `git diff --name-only main...task/TD-10a -- packages/ apps/agent/` | empty: no contract change |
| CI log `TD-10a-ci.log` | ran on aa1c7f3 (the last code commit), `CI GATE PASSED` |
| lock-connection loss repro (scratch script, fake PG wire server, no host DB) | **process crash**, see H1 |

## Findings (ranked)

### H1 — a PostgreSQL restart or dropped connection while any commit-lock section runs crashes vrx-api (must fix before merge)
`apps/api/src/commit/pg-lock.ts:20` and `:32` (`pool.connect()`), `:54` (`releaser`).
The advisory lock is held on a client that stays **checked out** for the whole section: up to 111 s for a commit (Apply 60 s),
up to 65 s for each reconcile retry, plus the watcher and the ARCH-01 check. pg-pool removes its idle `error` listener on
checkout (`pg-pool/index.js:344`). `db.ts` only covers idle clients with `pool.on('error')`. If the backend goes away
while no query is running on that client (PG restart, `pg_terminate_backend`, a network drop, `idle_session_timeout`), pg's
`Client._handleErrorEvent` emits `error` with no listener. Node throws, and the API process dies. I reproduced it with pg@8.23.0 /
pg-pool@3.14.0 and a minimal fake PG server that drops the socket 300 ms after `select pg_advisory_lock(...)`:
```
lock "held" on a checked-out client; the section awaits the agent Apply now...
process exit code 1
Error: Connection terminated unexpectedly … Emitted 'error' event on Client instance at: Client._handleErrorEvent (pg/lib/client.js:422)
```
Before this branch, a checked-out client was held only for a transaction's queries (milliseconds). Now the window covers the
whole agent Apply. ARCH-01 would repair the state after systemd restarts the process, but a DB blip must not kill the API
halfway through an Apply.
**Fix:** after each `pool.connect()` in `acquire`/`tryAcquire`, attach `c.on('error', onLost)` (log it, record a `COMMIT_LOCK_LOST`
system event, and set a flag the section can read). Remove the listener in `releaser` and in the `tryAcquire` null/throw paths
before `c.release(...)`. Add a unit test: a fake `Pool` whose client emits `error` during `fn()`. The process must survive,
the section must finish, and the release must destroy the client. Once the lock connection is lost, the section no longer
has cross-process exclusion. The promote still runs under `config_candidate FOR UPDATE`, so logging it is enough.

### M1 — the web concludes "not applied, you can try again" while the server may still be applying (double-rollback risk)
`apps/web/src/pages/RevisionsPage.tsx:141-152`, `apps/web/src/net.ts:86-96`.
`isUnreachable(e)` is true for every network error and for any non-JSON 5xx (`api-problem.ts:21`). That includes an early
connection reset and an nginx 502/504; nginx's default `proxy_read_timeout` is 60 s, which is below the 111 s budget. The
lookup runs at once. If the rollback is still inside its budget, there is no new revision, no pending row and sync is
`in-sync`, so `applyOutcome` returns `not-applied` and the UI says "you can try again". A retry then either gets 409
`commit-busy` (fine) or, once the first one finished without a confirm window, applies a second identical rollback revision.
"The outcome lookup never double-commits" holds for the lookup itself, but its message invites the double commit.
**Fix:** export the budget to the web (`COMMIT_BUDGET_MS = 111_000` next to `TIMEOUTS`). While
`Date.now() - sentAt < COMMIT_BUDGET_MS + 5 s`, never return `not-applied`: show `looking` and poll `/state/system` and the
newest revision every few seconds until the budget has passed, or until a pending commit or newer revision appears. Add a
test case "early network error → no not-applied before the budget". Also add a tech-debt line for P10 (nginx):
`proxy_read_timeout ≥ 130 s` on `/api/v1/config/{commit,rollback/*,commit/confirm,validate}`. Without it, 2.4a stops at 60 s
behind nginx.

### M2 — secret delete now queues without bound behind commits, but the web gives it 15 s
`apps/api/src/secrets/secrets.service.ts:154` uses `commits.exclusive` → `CommitLock.run`, which waits with no bound.
A `DELETE /api/v1/secrets/…` that arrives during a commit or reconcile waits up to 111 s (65 s behind a reconcile). The web gives
it `TIMEOUTS.write` = 15 s (`net.ts`, `timeoutFor`). It then reports "unreachable" for a delete that happens afterwards. This
is the queue that 2.4a and D-TD10a-1 remove for commit, rollback and confirm, and this branch creates it for delete.
**Fix:** add `CommitService.userExclusive(fn)` = `userSection` (1 s wait → 409 `commit-busy`, `retryAfterSec: 2`) and use it in
`SecretsService.delete`. The 2.3f e2e keeps its meaning, since the delete is the lock holder there. `users.service.ts:134`
(password set) has the same issue but is not TD-10a's file. Hand it to TD-4/TD-10b as a one-line change.

### L1 — a second API process can undo the other process's lost-answer recovery through ARCH-01
`apps/api/src/commit/commit.service.ts:1072` (`checkAgentTxn` guard) and `:1109` (`expectedTxn`).
Process A: Apply C times out → `lostTrack('unknown', inflight C)` → config_sync becomes `unknown` in the DB. Process B sees the
foreign RECONCILE_DONE(C). Its guard reads **its own in-memory** `this.sync` (`in-sync`), and `expectedTxn` falls back to the old
revision's txn ≠ C. B therefore declares a mismatch and re-applies running before A's reconcile can promote C. The final state is
consistent, but the M3 lost-answer recovery is defeated in exactly the two-process setup that D-TD10a-2 supports.
**Fix:** inside the locked re-check, `const s = await this.repo.getSync(); if (s.state !== 'in-sync') return;` (optionally adopt
`s` into `this.sync`). Add a test with two `CommitService`s on one `MemoryConfigRepo`.

### L2 — the lock wait is not fully bounded; the "DB 15 s" budget line is an assumption
`pg-lock.ts:32`: `tryAcquire` calls `pool.connect()` with no timeout (no `connectionTimeoutMillis`). An exhausted pool makes
a user section wait longer than `lockWaitMs`, and nothing enforces the 15 s DB margin (no `statement_timeout`).
**Fix (cheap):** race `pool.connect()` against the remaining wait in `tryAcquire` and answer `LockBusyError` on expiry. Write
the DB margin down as an assumption in `budget.ts`, or set `statement_timeout` for the API role (tech-debt).

### L3 — outcome heuristics can name the wrong commit
- CLI `client.go:181`: "created after this request was sent: it was probably applied" compares the server's `createdAt` with the
  **client** clock (2 s slack). A remote CLI with clock skew misreports. The web already records `beforeRevision`. The CLI could
  read `Config_revisions?limit=1` before an outcome op, or the heuristic could be dropped.
- Web `applyOutcome` (`net.ts:88`) and the CLI treat *any* pending commit as "yours". A pending commit that existed before
  (another user's; the lost answer was actually a 409 `commit-pending`) is reported as "your rollback is pending", and the web
  also tracks it in `confirmStore`. Compare `pending.createdAt >= sentAt − skew`.

### L4 — test gaps (not blocking)
- There is no direct unit test for the confirm path "no answer **and** no Health → 409/5xx `running-unknown` → reconcile finds
  `last_txn_id == pending` → revision saved". The new `reconcileLocked` branch (`commit.service.ts:326`) is only reached indirectly.
- `transport_test.go:152` drives the timeout through `http.Client.Timeout`. Production uses the per-request context deadline
  (`ApplyTimeout`). Set `c.ApplyTimeout = 300ms` instead, so the tested path is the real one.
- After H1: add the lock-loss test.

### L5 — note for the confirm banner owner
A confirm that gets 409 `commit-busy` (a reconcile or ARCH-01 section holds the lock, at most ~65 s) close to the deadline lets
the commit revert. The banner (not TD-10a's file) should retry a confirm automatically on `commit-busy` using `retryAfterSec`.

## Focus questions

1. **Confirmed/reverted state machine: correct.** I checked every path against `service.go`. `LastTxnID` changes only on a
   non-window apply or a confirm, and never on a revert or resync. `PendingTxnID` stays set while a revert is owed. Findings:
   - A txn id is a fresh UUID, and new commits are refused while `config_pending` exists (`assertNoPending`), so
     `last_txn_id == p.txnId` can only mean confirmed. Neither → reverted.
   - The FAILED_PRECONDITION + Health `pending` case correctly counts as "deadline passed / revert owed" (agent M1 check).
   - Every path that promotes a pending commit (confirm, watcher, reconcile, ARCH-01) re-reads `config_pending` under the lock,
     and the promote clears it in the same transaction, so there is no double promote. The reconcile's in-flight path is
     guarded by "latest revision txn ≠ f.txnId".
   - The watcher's fate is computed outside the lock but can only go stale in a safe direction: a txn the agent no longer
     holds as pending can never be confirmed later.
   - API restart mid-confirm: the boot ARCH-01 check or the re-armed watcher saves it. Agent reconnect: `watchReady`.
     A foreign RECONCILE_DONE: coalesced check, re-verified under the lock.
   - Under the real agent's semantics I found no path that records "reverted" for a confirmed commit or the other way round,
     and no double apply. The only cross-process hole is L1.
2. **Migration 0004: additive and reversible.** It adds two nullable jsonb columns, with no default and no backfill. The
   snapshot chain is fine (0004.prevId = 0003.id). main's code ignores the columns, so a binary downgrade works; reversing it
   is `DROP COLUMN` ×2. No other branch adds a 0004 today (checked every `task/*` branch), but the merger must renumber if one
   lands first. `restoreSecrets` is applied inside the promote transaction that clears the pending row.
   `restoreSecretVersions` is idempotent (`version <> $v`). **Applied exactly once.**
3. **Advisory lock.**
   - Key `(0x56525841, 1)`: the two-int form fits int4 and is scoped per database, so slots do not collide. Nothing else in
     the repo uses advisory locks.
   - Lock order is always advisory → rows (commit, secret delete and password set take the lock before opening a
     transaction). Row-lock holders that are not sections (candidate edits, key deletes) never wait on the advisory lock,
     so no deadlock cycle is possible. The in-process FIFO means each process holds at most one extra pool connection.
   - Crash: the session ends and PostgreSQL drops the lock. **Connection loss: crashes the process (H1).**
   - The 1 s wait then 409 is sensible (D-TD10a-1).
4. **Time budget.** 1 + 5 + 30 + 60 + 15 = **111 s** matches the code path: Health once in `validate`, DryRun capped at 30 s,
   Apply capped at 60 s even with a larger `VRX_AGENT_TIMEOUT_MS`. The confirm worst case is about 81 s. Web 130 s (19 s
   margin) and CLI 150 s (39 s) are right. The CLI looks up only on a real timeout (after 150 s), so it cannot race.
   The web can, see M1. The lookups are GET-only; nothing re-posts.
5. **Secret delete race: closed.** The only revision writer is `promote`, and it always runs under the commit lock. The
   candidate and pending rows are locked `FOR UPDATE`, and the checks and both deletes are one transaction. Remaining issue:
   the unbounded queue (M2).
6. **CLI redirect policy: correct.** `CheckRedirect: http.ErrUseLastResponse`, so a 3xx becomes an error that names
   `Location` (also in `Raw()`). This covers no follow, no same-host scheme/port downgrade and no 307/308 body replay. There
   is one `http.Client` in the CLI and no WebSocket or other client. The operation IDs in `applyOps`/`outcomeOps` match the
   OpenAPI.
7. **Out-of-list hunks: justified.** `agent.client.ts` gets an optional `timeoutMs` (the budget can only be enforced there)
   plus `watchReady` (the reconnect trigger). The W-seed anchor block is untouched. `pg-repo.ts` changes only `readPending`
   (not TD-15's or TD-4's hunks; TD-4 overlaps only on `configResets`, which does not conflict). The locale keys and the
   fake-agent `reset()` and `deadlines` lines are needed by the tests. The salvage reformat was undone in 5ed9589.
8. **Tests fail on main: yes.** See the table above. Overlaying the corrected fake agent onto main is legitimate: the old fake
   set `lastTxnId` on window applies, which hid the bug, and the new fake matches `service.go` line for line.

## Recommendations on the task decisions

- **D-TD10a-1 (commit-busy after a 1 s wait): accept.** Internal sections take milliseconds except reconcile (≤ 65 s), and a
  0 ms try-lock would give spurious 409s right after the watcher or an event. Extend it to every *user* section, including
  secret delete and password set (M2). Optionally also send a `Retry-After` header.
- **D-TD10a-2 (advisory lock per section, not a boot guard): accept, conditional on H1.** Also record in the LOG entry that
  in-memory state (in-flight doc, sync, timers) stays per process, so a second process is safe only for serialisation. Fix L1.
  It does close the D-111 `syncUsers` before-read limit, since every hash writer now takes the lock.
- **D-TD10a-3 (111 / 130 / 150 s): accept.** Add a tech-debt line: nginx (P10) `proxy_read_timeout ≥ 130 s` on the apply
  paths, and state that the DB 15 s margin is an assumption, not enforced (L2).
- **D-TD10a-4 (ARCH-01 skips a database that never applied anything): accept.** Not wiping an agent from a fresh or restored DB
  is the safer default. The one re-apply for a revision without a txn is harmless.
- **D-TD10a-5 (delete removes every secret_version, no tombstone): accept for now.** The failure mode is a visible 400
  `secrets.ref-exists` on rollback, not silent corruption. Keep the tombstone and "refuse while pinned" option as tech-debt.

## Answers to TD-10a-questions.md

1. Hunks outside `files_owned`: approve all three as they are (see focus item 7).
2. Fold D-TD10a-1…5 into LOG as above.
3. CommitDialog / queries.ts: give the one hunk to the row that owns `apps/web/src/config/**` (the pending-change bar), **after**
   M1, so it reuses the time-gated `applyOutcome`. The same row should add the confirm banner's retry on `commit-busy` (L5).
4. Salvage reformat: confirmed undone; the net diff is hunks only.

## Merge conditions

H1 fixed with a test (required). M1 and M2 fixed in this branch (small; recommended before merge). L1–L4 fixed or logged as
tech-debt. There is no need for a full second review round if the fixes are limited to those lines and the api unit suite,
the td10a e2e and CI are green.

**APPROVE WITH CHANGES**
