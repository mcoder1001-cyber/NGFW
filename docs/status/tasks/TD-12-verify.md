# TD-12 — verification

**Verdict: APPROVE**

Checked at fed09a5 (task/TD-12 @ base main@16b622a), worktree /root/ngfw-wt/TD-12.

## (1) Diff scope
`git -C /root/ngfw diff main...task/TD-12 --stat` touches exactly:
- `apps/agent/internal/agent/service_test.go` (test)
- `apps/agent/internal/renderers/strongswan/review_fixes_test.go` (test)
- `apps/api/vitest.config.ts`, `apps/web/vite.config.ts` (vitest configs)
- `docs/status/tasks/TD-12.envelope.md`, `docs/status/tasks/TD-12.md` (status docs)

No product code changed. Matches the envelope's "files you own" list. PASS.

## (2) Bounded waits, no timing-assumption regressions
- `TestGRPCRoundTrip`: fixed `time.Sleep(50ms)` replaced with `waitForSubscriber(t, s)`,
  a poll on `s.bus.subs` bounded at 10 s with `t.Fatal` on timeout — a real regression
  still fails fast. PASS.
- `TestWatchResync`: subscription wait widened from an unbounded-looking 200×10ms loop
  that **used to silently fall through** on exhaustion to an explicit 15 s bound that now
  `t.Fatal`s ("Watch never subscribed") if exceeded, and the overall `ctx` timeout raised
  20s→45s. Confirmed the final `select` still exits via `t.Fatalf` on `ctx.Done()` — a hang
  still fails within 45 s. PASS.
- vitest `maxWorkers: 4` + `testTimeout/hookTimeout/teardownTimeout: 30_000` (both apps/api,
  apps/web): these bound *concurrency*, not the timeout escape hatch — a genuinely hung
  test still fails at the 30 s `testTimeout`, `maxWorkers` doesn't suppress that. PASS.

## (3) TestPendingSurvivesRestartAndRevertsAfterDeadline still tests the same property
Read the full test body in the worktree. The fix removes the real-timer race (bumped
`ConfirmTimeoutSec` to 3600 s so `armTimerLocked`'s real `time.AfterFunc` can't plausibly
fire before `s.Close()`), then constructs the "deadline already passed" precondition
directly via `loadState`/backdated `meta.ConfirmDeadline`/`save()`. Verified in
service.go: `armTimerLocked` for the persisted pending txn is only invoked from `Resync`,
never from `NewService`/`refreshSnapshotLocked` — so after restart, `s2.Health()` reads the
persisted `PendingTxnID` deterministically (no race with a background timer) confirming
**pending survives restart**, and the explicit `s2.Resync()` call synchronously reverts
because `ConfirmDeadline` is in the past, confirming **reverts after the deadline** — same
two assertions as the original test, same code paths (`Resync`'s owed-revert branch), just
without depending on wall-clock elapse. PASS.

## (4) vitest config doesn't mask a real hang
Confirmed by reading both configs: `maxWorkers: 4` only caps fork/process count; the
`testTimeout`/`hookTimeout`/`teardownTimeout: 30_000` values are unchanged as the actual
failure trigger — a hung test/hook/teardown still throws at 30 s regardless of worker count.
PASS.

## (5) Test runs (this session, in the worktree)
```
$ cd apps/agent && go test -count=5 ./internal/agent/ -run 'TestGRPCRoundTrip|TestPendingSurvives' -v
--- PASS: TestGRPCRoundTrip (x5)
--- PASS: TestPendingSurvivesRestartAndRevertsAfterDeadline (x5)
ok  	ngfw/agent/internal/agent	1.355s

$ go test -count=3 ./internal/renderers/strongswan/ -run TestWatchResync -v
--- PASS: TestWatchResync (x3)
ok  	ngfw/agent/internal/renderers/strongswan	0.120s
```
10/10 and 3/3 passed, no flakes observed this run.

## Notes (non-blocking)
- TD-12.md's finding #5 (grep for other time.Sleep-then-assert patterns) correctly scopes
  `apply_test.go`'s matching pattern as out-of-scope (not in this task's owned files) and
  flags it as a follow-up — reasonable given the "never: product code" / narrow file-
  ownership constraint on a shared worktree.
- Envelope's synthetic-load proof (`stress-ng`) wasn't run (not installed on host, and a
  large synthetic CPU burn was declined as host-disruptive on a shared 12-agent box);
  TD-12.md documents host real-load sampling and a mechanistic repro (temporarily removing
  the safety margin) as the substitute. Acceptable given the constraints, and the checks
  above independently confirm the fixes are structurally sound regardless of how flakiness
  was originally reproduced.
