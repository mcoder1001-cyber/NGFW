# TD-9: agent core — bounded VPP calls and transaction semantics

branch `task/TD-9` · worktree `/root/ngfw-wt/TD-9` · slot 1 · base `task/TD-8@8a96a9c` (SPECULATIVE, D-114; TD-8 has since
merged as `8a633616`) · REVIEW-2026-09-24 §1 (verified), D-125 · started 2026-09-24 23:36.

Commits: `99f12d4b` (code and tests), `2445fcf5` (proto.md §2 and a test-cleanup fix), `ffe2b755` (lint), then this document.
The patch contains no contract file, and it applies to main without conflict (§ How verified 3).

## Summary

- **The finding the review rated HIGH (1.1) is fixed.** A VPP that dies or hangs mid-request can no longer park the
  agent.
  - Every VPP reply is now bounded: 30 s by default, set with `VRX_AGENT_VPP_REPLY_TIMEOUT`.
  - Every transaction runs on the agent's own deadline: 5 min for Apply, resync and revert, 2 min for a rollback.
  - An operation whose outcome is unknown answers DEGRADED, not ROLLED_BACK, and the agent owes a resync.
  - The owed resync is retried with 5 s → 60 s backoff until VPP is back in the stored desired state.
- **The caller's deadline bounds only the wait for the lock** (1.4). An outcome that a timeout decided is never
  stored under the txn_id.
- **The rest of the scope is done**, each item with a test that fails on the base: 1.1b, 1.1c, 1.1d, 1.1e, 1.2,
  1.3, 1.5a, 1.5b, 1.5c, 1.5e, ARCH-01 (agent half) and the tech-debt item "P05 owed revert dropped".
- The agent stays declarative, and every recovery path converges through the stored desired state: a resync, or
  the state reloaded from disk after a panic.
- The lock order is unchanged from TD-8: txn → the source cache → `sched.mu`.

Files:
- `internal/vpp/conn.go`, `internal/scheduler/reconciler.go` (the ApplyWith ctx, rollback ctx and descriptor-call
  guard hunks only), `internal/agent/{agent,service,state,server}.go`, `internal/ownertable/ownertable.go`,
  `internal/descriptors/dfkit/boot.go`, `cmd/vrx-agent/main.go`, `docs/contracts/proto.md` §2.
- `internal/agent/metrics.go`: an additive hunk (Q2).
- New tests: `internal/agent/td9{,_helpers}_test.go`, `internal/scheduler/reconciler_td9{,_helpers}_test.go`,
  `internal/vpp/conn{,_td9_helpers}_test.go`, `internal/ownertable/ownertable_td9_test.go`,
  `internal/descriptors/dfkit/boot_td9_test.go`.
- Updated test: `TestStateCrashInjection` (ARCH-01).

## What, per item (base file:line → change)

| ref | on the base `8a96a9c` | now |
|---|---|---|
| 1.1 | govpp `core.DefaultReplyTimeout = 0`; `vpp/conn.go:225-240` passes the caller ctx through | `vpp/conn.go`: `init` sets `core.DefaultReplyTimeout = 30 s` (backstop). `ConnOptions.ReplyTimeout` defaults to 30 s and is set from `VRX_AGENT_VPP_REPLY_TIMEOUT` (seconds or a Go duration; an invalid value refuses to start). `Invoke` bounds its round trip with `context.WithTimeout(ctx, ReplyTimeout)`: a ctx without a deadline gets it, an earlier caller deadline wins. `NewStream` prepends `core.WithReplyTimeout(ReplyTimeout)`, so every message of a dump is bounded and a caller's own option overrides it for a known-slow dump. A missed reply is `vpp.ErrTimeout`, which wraps `context.DeadlineExceeded`. `vpp.Bounded(c, d)` puts the same Invoke bound on a non-Conn client (the tests' fake VPP). |
| 1.1 | `service.go:497-499,534` revert on `context.Background()`; `:596` resync on the watchVPP ctx; `reconciler.go:710` `rbCtx = WithoutCancel(ctx)` | Apply runs on `txnContext` = `WithTimeout(WithoutCancel(caller), 5 min)`. Resync and the confirm revert each get their own `WithTimeout(ctx, txnTimeout)`, where ctx carries only the agent's stop. The rollback runs on `WithTimeout(WithoutCancel(ctx), Scheduler.RollbackTimeout = 2 min)`. |
| 1.1 | `agent.go:286` `a.wiring.Connected(ctx)`: P08's boot-identity ControlPing, unbounded | Runs under `WithTimeout(ctx, connectHookTimeout = 30 s)`, and a panic there is contained (`safely`). |
| 1.1 | a timed-out op ends ROLLED_BACK while VPP may still hold its effect | `reconciler.go` `ApplyWith`: the op that failed is classified by `uncertain(err)` (deadline, cancel or panic; matched by text too, for descriptors that format with `%v`). Such an op sets `TxnResult.Uncertain`: the journal is still undone, but the outcome is **DEGRADED**. An op that never started (a ctx already done before it) stays certain. The service appends "the outcome of that operation is unknown … the agent owes a resync" to `message`. |
| 1.1b | `service.go:597-599`: a failed resync only sets DEGRADED | `setDegraded(true)` owes a resync (`oweResyncLocked`): a timer with backoff `retryMin` doubling to `retryMax` (N2: 5 s → 60 s). It asks the agent through `ServiceConfig.RequestResync`, which is the TD-8 Env.Resync path (`watchVPP` → `fullResync`), so the wiring's after-resync hook runs too. Without an agent, the service resyncs itself. The timer is skipped while a confirm revert is owed, because that revert's own retry converges to the same baseline. It is paid, meaning stopped with the backoff reset, by an APPLIED resync, revert, or Apply over every managed domain (Q6). |
| 1.1b | no drift check | `Service.CheckDrift`: a **Plan**, never an apply, of the stored desired state of the managed domains (with the dynamic sources in sync) against VPP. It runs every 5 min (`watchDrift`, a.wg) and skips a round when a transaction holds the lock. It sets the gauge `vrx_agent_drift_objects`; when the count becomes non-zero or changes, it sends an `ERROR` event with `reason=drift,objects=N` and logs the first 5 keys. It never corrects anything; auto-correction would need a D-entry. |
| 1.1c | `agent.go:296-300`: the watcher goroutine exits for good after its first error and is not in a.wg | `runLinks`: restarts `watchLinks` with backoff (1 s → 30 s, reset after a watch that ran longer than the maximum) until VPP disconnects or the agent stops. It is in `a.wg`. `want_interface_events` goes through `Conn.Invoke`, so it is bounded as well; `telemetry.go` is unchanged. |
| 1.1d | `agent.go:186` `grpc.NewServer()`, no recover anywhere | `server.go` `newGRPCServer`: `ChainUnaryInterceptor` and `ChainStreamInterceptor` recover a panic → `INTERNAL`, with the stack in the log and `vrx_agent_panics_total{where="grpc"}`. `Service.containLocked` is deferred right after the txn lock in Apply, Resync, the revert, the resync retry and the drift check. A panic of the agent's own code there reloads the persisted state from disk (what a restart would do), leaves the agent DEGRADED with a resync owed, answers INTERNAL, and releases the lock. `watchVPP`'s wiring hooks and the link watcher run under `safely`. |
| 1.1e | no recover in `internal/scheduler` | `reconciler.go` `guard`: every descriptor call is wrapped — Retrieve (inside `retrieve`), the whole plan phase (Normalize, KeyOf, Dependencies, ProvidedKeys), each operation (`x.run`), verify, Reapply and each undo. A panic becomes an error wrapping `ErrDescriptorPanic` (key and call only; the value and stack go to the log). The normal rollback runs. The status is DEGRADED, because the panicking call's own effect is unknown (Q5). A panic in the plan phase answers FAILED; in Reapply it counts as a reapply error; in an undo it is REVERT_FAILED. `executor.create` and `topo` (TD-11b, TD-11c) are untouched. |
| 1.2 | `service.go:382-386,797-799`: Validation only for FAILED | An APPLIED or ROLLED_BACK answer carries the projection's warnings in `validation` (`ok = true`) when there are any. Without warnings the field stays unset, so earlier answers are unchanged. See proto.md §2. |
| 1.3 | `service.go:315` `confirmLocked()` runs before `:335` checks that VPP is connected | For an apply, the VPP-connected check runs right after the txn_id recall and **before** the confirm half, and before an owed revert is touched. |
| 1.4 | `service.go:339-340`: the txn runs on the caller ctx and its outcome is remembered unconditionally | The txn runs on `WithoutCancel(caller)` plus the agent's deadline. `applyLocked` returns `retryable` when an op was uncertain, when the txn deadline cut a non-APPLIED txn, or when the state or claims could not be saved. Such an outcome is **not** remembered, so a retry with the same txn_id runs again. proto.md §2 item 3 is amended. |
| 1.5a | `ownertable.go:199` renames without a directory fsync; `dfkit/boot.go:153-167` the same | `ownertable.SyncDir(dir)` is the shared helper (TD-16 can reuse it). `WriteAtomic` fsyncs the directory after the rename. The boot store's `flush` now calls `ownertable.WriteAtomic(path, raw, 0o600)`. `SetSyncDir` is a test seam. |
| 1.5b | `service.go:369,408`: `deadline = start + timeout` | `deadline = applied_at + timeout`, where `applied_at` is taken after the transaction, as proto.md §4.1 says. |
| 1.5c | `cmd/vrx-agent/main.go:29-30` ignores an invalid `VRX_LOG_LEVEL` | `agent.ParseLogLevel`. `Config.Validate` refuses anything but debug, info, warn or error (`""` = info), and `main` logs the error and exits 1. |
| 1.5e | `/metrics` has no authentication; `VRX_METRICS_ADDR` is not checked | `Config.Validate` refuses a non-loopback address, including `:9101`, `0.0.0.0` and `[::]`, unless `VRX_METRICS_ALLOW_REMOTE=1`. `localhost`, `127/8` and `::1` are fine. |
| ARCH-01 | `service.go:429-431,341-343`: a failed state save is only logged, and the answer is APPLIED | A failed write of `agent-state.json` after an APPLIED transaction answers **DEGRADED** ("applied to the data plane, but the agent could not save its state"). It sets a resync owed and is not stored under the txn_id. A failed write of only the `desired.pb` mirror (`errMirror`, after the state file is durable) stays APPLIED. `TestStateCrashInjection` was updated to match. |
| tech-debt | `service.go:321-334`: an owed revert is dropped before the superseding Apply runs, even when that Apply fails | The supersede happens in `applyLocked`'s APPLIED branch. A superseding Apply that fails leaves the revert owed and pending, and its retry timer keeps running. |

### Coordination hooks
- **TD-11c** (manager's note, 2026-09-24):
  - `ServiceConfig.FlushClaims func(ctx) error` is called in `applyLocked` **after the outcome is known** (after the
    status switch and `refreshSnapshotLocked`) and **before `st.save()`**.
  - An error turns APPLIED into DEGRADED through the same `notSavedLocked` path as a failed save, and the outcome is
    not stored.
  - It is nil (a no-op) on this base. TD-11c sets it in `agent.go`'s `NewService` call. Its ctx is the transaction's.
- **TD-8** (merged): the owed resync uses `requestResync(resyncs)`, the Env.Resync path. `dynsource.go` is untouched.
  The source-sync deadline is left to TD-8b (Q7).
- **TD-11b** (ownership declarations): TD-9 registers no descriptor, so there is nothing to declare (Q8).

### Timeouts
| what | value | where |
|---|---|---|
| one VPP reply (Invoke round trip / one stream message) | 30 s, `VRX_AGENT_VPP_REPLY_TIMEOUT` | `vpp.DefaultReplyTimeout`, `ConnOptions.ReplyTimeout`, `core.DefaultReplyTimeout` |
| one transaction (Apply after the lock, resync, confirm revert) | 5 min | `agent.DefaultTxnTimeout`, `ServiceConfig.TxnTimeout` |
| one rollback | 2 min | `scheduler.DefaultRollbackTimeout`, `Scheduler.RollbackTimeout` |
| wiring connect hook (boot-identity ControlPing) | 30 s | `connectHookTimeout` |
| owed-resync retry | 5 s doubling to 60 s (N2) | `retryMin`/`retryMax` |
| link-watcher restart | 1 s doubling to 30 s | `linkRetryMin`/`linkRetryMax` |
| drift check | every 5 min, Plan only | `driftInterval` |

With a VPP that stops answering entirely, an Apply returns after one reply timeout plus whatever rollback remains.
Each undo gets a reply timeout of its own, but govpp's health check (2 s × 5) marks VPP disconnected within about
10 s, and after that every call fails at once. The rollback bound caps the total at 30 s + 2 min.

### D-entry texts (for the manager to copy into LOG.md)
- **Agent timeouts and "outcome unknown" (TD-9, review 1.1, 1.1b and 1.4; amends proto.md §2 item 3 and Outcomes).**
  - Bounds: a VPP reply 30 s (`VRX_AGENT_VPP_REPLY_TIMEOUT`); a transaction 5 min on the agent's own clock, where the
    caller's deadline bounds only the wait for the lock; a rollback 2 min; the connect hook 30 s.
  - An operation cut off by a timeout, a cancel or a descriptor panic may have taken effect. It answers DEGRADED with
    a resync owed, retried 5 s → 60 s through the Env.Resync path, and the outcome is not stored under the txn_id.
    A failed state save answers DEGRADED in the same way (ARCH-01).
  - DEGRADED is cleared only by an APPLIED resync, revert, or Apply over every managed domain.
  - The drift check is Plan-only, every 5 min, reported as a gauge and an ERROR event. It never auto-corrects.
  - Why: govpp's default 0 parked the agent forever when VPP died mid-request. ROLLED_BACK for an unjournaled op was a
    lie. A cached cancel made "safe retries" unsafe.
  - Alternatives: a per-descriptor timeout, rejected as 80 call sites. Skipping the rollback on the first timeout,
    rejected: a VPP that answers late but answers would then stay half-applied until the resync. Auto-correcting
    drift, deferred: that is a product decision.
- **Confirm deadline from applied_at (TD-9, review 1.5b).** `confirm_deadline = applied_at + timeout`, as proto.md §4.1
  always said. The code used the transaction's start, so a slow apply shortened the window, and an apply longer than
  the timeout reverted at once. Code-only; no contract change.

## How verified
All runs are on this host, 2026-09-24 23:40 → 2026-09-25 04:01, at host load 15–48.

### 1. The TD-9 tests on the branch (`-race`, unit)
```
$ cd apps/agent && env -u VRX_INTEGRATION go test -race -count=1 -run 'TestApplyReturnsWhenVPPNeverReplies|TestCallerDeadlineDoesNotCutTheTransaction|TestResyncAndRevertHaveTheirOwnDeadline|TestOwedResync|TestConfirmAndApplyWhileVPPDown|TestApplyAnswersCarryWarnings|TestConfirmWindowStartsAtAppliedAt|TestFailedApplyKeepsTheOwedRevert|TestGRPCHandlerPanic|TestPanicInATransaction|TestLinkEventsWatcherRestarts|TestConnectHookHasItsOwnDeadline|TestDriftCheckIsPlanOnly|TestConfigRefuses|TestConfigReplyTimeout' -v ./internal/agent/
--- PASS: TestConfigReplyTimeoutAndMetricsOptIn (0.03s)
--- PASS: TestApplyReturnsWhenVPPNeverReplies (0.46s)
--- PASS: TestCallerDeadlineDoesNotCutTheTransaction (0.53s)
--- PASS: TestResyncAndRevertHaveTheirOwnDeadline (1.76s)
--- PASS: TestOwedResyncRetriedWithBackoff (0.66s)
--- PASS: TestOwedResyncTakesTheAgentsResyncPath (0.12s)
--- PASS: TestConfirmAndApplyWhileVPPDownConfirmsNothing (0.03s)
--- PASS: TestApplyAnswersCarryWarnings (0.11s)
--- PASS: TestConfirmWindowStartsAtAppliedAt (0.50s)
--- PASS: TestFailedApplyKeepsTheOwedRevert (1.31s)
--- PASS: TestGRPCHandlerPanicAnswersInternal (0.14s)
--- PASS: TestPanicInATransactionIsContained (0.03s)
--- PASS: TestLinkEventsWatcherRestarts (0.17s)
--- PASS: TestConnectHookHasItsOwnDeadline (0.28s)
--- PASS: TestDriftCheckIsPlanOnly (0.22s)
--- PASS: TestConfigRefusesUnknownLogLevelAndRemoteMetrics (0.00s)
$ env -u VRX_INTEGRATION go test -race -count=1 -run 'TestDescriptorPanic|TestRollbackIsBounded|TestTimedOutOperation' -v ./internal/scheduler/
--- PASS: TestDescriptorPanicRollsBack (0.03s)       (create, update, delete)
--- PASS: TestDescriptorPanicOutsideOperations (0.00s)
--- PASS: TestRollbackIsBounded (0.30s)
--- PASS: TestTimedOutOperationIsUncertain (0.01s)   (deadline, reply %v, cancelled, plain (sure))
$ env -u VRX_INTEGRATION go test -race -count=1 -v ./internal/vpp/ -run 'Reply|NeverReplied'
--- PASS: TestDefaultReplyTimeoutIsBounded (0.00s)
--- PASS: TestInvokeNeverRepliedTimesOut (0.26s)
--- PASS: TestStreamNeverRepliedTimesOut (0.82s)
ok (ownertable TestWriteAtomicSyncsTheDirectory, dfkit TestFileBootStoreSyncsItsDirectory)
```
What the key tests prove:
- **The acceptance (`TestApplyReturnsWhenVPPNeverReplies`).** The fake VPP applies blue's IPv6 table and never
  answers. The service uses `vpp.Bounded(fake, 300 ms)`, the same `invokeWithin` code that `Conn.Invoke` runs.
  - Apply returns after the reply timeout + ε, DEGRADED, with "the outcome of that operation is unknown".
  - The owed resync (backoff 50 ms in the test) deletes the table VPP made without answering, and DEGRADED clears
    although VPP still hangs on that request.
  - The retry of `t2` with the same txn_id runs again (nothing was stored) and is APPLIED once VPP answers, and the
    next Apply `t3` is APPLIED.
- **The real govpp path (`TestInvokeNeverRepliedTimesOut`, `TestStreamNeverRepliedTimesOut`).** A real
  `core.Connection` over govpp's mock adapter, whose replies carry an unknown message id, so govpp drops them:
  `Conn.Invoke` and a stream `RecvMsg` return `ErrTimeout` after `ReplyTimeout`. A caller's earlier deadline wins, and
  a caller's `WithReplyTimeout` overrides the default.

### 2. Every behaviour change fails on the base first
Method (as TD-8-verify):
- `git archive 8a96a9c apps/agent` into the scratchpad.
- Add the new test files unchanged (`td9_test.go`, `service_test.go`, `reconciler_td9_test.go`, `conn_test.go`,
  `ownertable_td9_test.go`, `boot_td9_test.go`).
- The new API is reached only through the `*_td9_helpers_test.go` files. For the base run they are replaced by shims
  that encode the **base's** behaviour, quoted in full here:
  - `bounded(c, _) = c` (nothing bounds a VPP call);
  - `setTxnTimeout`, `setLinkRetry`, `setConnectHookTimeout`, `withRollbackTimeout`: no-ops;
  - `resyncDelayOf = 0`, `uncertainOf = false`, `isTimeout = false`, `replyTimeoutOf = 0`,
    `errDescriptorPanic = errors.New(…)`;
  - `checkDrift`: no-op;
  - `newRecoveringServer = grpc.NewServer()` (base `agent.go:186`);
  - `testConn(conn, _) = &Conn{conn: conn}`;
  - ownertable: `SyncDir`/`SetSyncDir` defined but never called by the base's `WriteAtomic`.
- `TestConfigReplyTimeoutAndMetricsOptIn` tests settings the base does not have, so it lives in the helpers file and
  has no base run.
- One test per run (`go test -count=1 -timeout 90s -run '^T$' -v`):
```
internal/scheduler TestDescriptorPanicRollsBack => reconciler_td9_test.go:132: ApplyWith panicked: descriptor bug: delete|… create|… update
internal/scheduler TestDescriptorPanicOutsideOperations => --- FAIL|panic: descriptor bug in retrieve [recovered, repanicked]
internal/scheduler TestRollbackIsBounded => reconciler_td9_test.go:224: ApplyWith did not return within 10s
internal/scheduler TestTimedOutOperationIsUncertain => reconciler_td9_test.go:273: outcome ROLLED_BACK uncertain false, want DEGRADED (b/y may exist: true) (×3)
internal/vpp TestDefaultReplyTimeoutIsBounded => conn_test.go:52: core.DefaultReplyTimeout = 0s: govpp's 0 waits forever for a reply
internal/vpp TestInvokeNeverRepliedTimesOut => conn_test.go:66: the call did not return within 5s: VPP never replies and nothing bounds the wait
internal/vpp TestStreamNeverRepliedTimesOut => conn_test.go:102: the call did not return within 5s: VPP never replies and nothing bounds the wait
internal/ownertable TestWriteAtomicSyncsTheDirectory => ownertable_td9_test.go:28: directory fsyncs [], want [/tmp/…/001]
internal/descriptors/dfkit TestFileBootStoreSyncsItsDirectory => boot_td9_test.go:29: directory fsyncs [], want 2 × /tmp/…/001
internal/agent TestApplyReturnsWhenVPPNeverReplies => td9_test.go:201: Apply did not return within 5.3s
internal/agent TestCallerDeadlineDoesNotCutTheTransaction => td9_test.go:239: status APPLY_STATUS_ROLLED_BACK, want APPLY_STATUS_APPLIED: create vrf/7001: ip_table_add_del 7001 ipv6=false add=true: context deadline exceeded
internal/agent TestResyncAndRevertHaveTheirOwnDeadline => td9_test.go:280: Resync did not return within 5.3s
internal/agent TestOwedResyncRetriedWithBackoff => td9_test.go:320: waiting for 4 events, got [… RECONCILE_START message:"resync [vrfs]"]
internal/agent TestOwedResyncTakesTheAgentsResyncPath => td9_test.go:359: not within 5s: the owed resync repaired VPP through the agent
internal/agent TestConfirmAndApplyWhileVPPDownConfirmsNothing => td9_test.go:376: the confirm half ran although the apply half could not: … last_txn_id:"p1"
internal/agent TestApplyAnswersCarryWarnings => td9_test.go:399: APPLIED without the warning: <nil>
internal/agent TestConfirmWindowStartsAtAppliedAt => td9_test.go:424: confirm_deadline − applied_at = 1.696777321s, want 2s
internal/agent TestFailedApplyKeepsTheOwedRevert => td9_test.go:440: a failed apply dropped the owed revert: … last_txn_id:"base" degraded:true (no pending_confirm_txn_id)
internal/agent TestGRPCHandlerPanicAnswersInternal => panic: runtime error: invalid memory address or nil pointer dereference   (the test binary dies)
internal/agent TestPanicInATransactionIsContained => td9_test.go:508: Apply panicked: agent bug
internal/agent TestLinkEventsWatcherRestarts => td9_test.go:546: not within 3s: the link watcher came back
internal/agent TestConnectHookHasItsOwnDeadline => td9_test.go:575: waiting for 1 events, got []
internal/agent TestDriftCheckIsPlanOnly => td9_test.go:587: no drift expected:   (no vrx_agent_drift_objects family)
internal/agent TestConfigRefusesUnknownLogLevelAndRemoteMetrics => td9_test.go:624: log level "verbose": <nil> | "information": <nil> | "trace": <nil>
internal/agent TestStateCrashInjection => service_test.go:929: status APPLY_STATUS_APPLIED, want APPLY_STATUS_DEGRADED   (ARCH-01)
```
All of them pass on the branch (§1, §4).

### 3. TD-9 applied to main (TD-8 merged, TD-7 merged)
- The branch's base is speculative, and main has the squashed TD-8 plus later merges. So I applied
  `git diff 8a96a9c HEAD` (TD-9 only) to a `git archive main` copy: every hunk applied, with offsets only.
- In that copy, the vet and race run are green:
```
$ go vet ./internal/agent/ ./internal/scheduler/ ./internal/vpp/ ./internal/ownertable/ ./internal/descriptors/dfkit/ ./cmd/vrx-agent/   (clean)
$ env -u VRX_INTEGRATION go test -race -count=1 ./internal/agent/ ./internal/scheduler/ ./internal/vpp/ ./internal/ownertable/ ./internal/descriptors/dfkit/
ok  	ngfw/agent/internal/agent	18.115s
ok  	ngfw/agent/internal/scheduler	1.604s
ok  	ngfw/agent/internal/vpp	2.181s
ok  	ngfw/agent/internal/ownertable	1.091s
ok  	ngfw/agent/internal/descriptors/dfkit	1.107s
```
- `git merge-tree main HEAD` reports conflicts. They come from the history, not from TD-9: the branch still carries
  TD-8/W-seed/P08 unsquashed, while main has them squashed (D-112). `patch` applies TD-9's own diff cleanly.
- Main's changes to TD-9's files since the base are in other hunks: the `server.go` Action anchors, the
  `service_test.go` TD-12 de-flakes, and `projection.go`.

### 4. CI
- `TMPDIR=/tmp/g-w1 tools/ci.sh --base main` → `CI GATE FAILED — CONTRACT FILES CHANGED WITHOUT A CONTRACT COMMIT`.
  - The files it lists (`packages/proto/vrx/v1/dataplane.proto`, `packages/schema/src/**`, `apps/agent/gen/**`, …) are
    main's changes since the speculative base, not TD-9's.
  - `git diff --stat 8a96a9c HEAD -- packages/schema packages/proto apps/agent/gen packages/api-client/src/generated`
    is empty.
  - After the D-112 squash + rebase onto main, the guard sees no contract file.
- Quick gate:
```
$ TMPDIR=/tmp/g-w1 tools/ci.sh            (task/TD-9 @ ffe2b755)
  apps/agent: make lint test build    1m14s     ← golangci-lint "0 issues"; go test -race -count=1 ./... : 89 packages ok, 0 FAIL
  mode quick · wall time 6m41s · logs /root/ngfw-wt/logs/ci/TD-9-20260925-035334-2440353
CI GATE PASSED
```
- The first quick run found one revive finding (`conn_test.go` `within` returned the error first). It is fixed in
  `ffe2b755`.

### 5. Host: real VPP, slot 1, one package at a time, under `flock -s /run/lock/vrx-lab.lock`
- Nothing restarted or killed VPP: `NRestarts=1 → 1` (the 1 is the 18:41 crash from before TD-9), and
  `MainPID=2006833` is unchanged.
- The real `vpp.Conn`, with its reply bound and the stream wrapper, against VPP 26.06:
```
$ TMPDIR=/tmp/g-w1 VRX_INTEGRATION=1 flock -s /run/lock/vrx-lab.lock go test -count=1 -race -v -run 'TestCoreOnHost|TestClaimRulesOnHost|TestRouteOverVPPEntriesOnHost' ./internal/descriptors/core/
    core_integration_test.go:103: apply: {Created:10 …} in 338.944437ms
--- PASS: TestCoreOnHost (0.87s)
--- PASS: TestClaimRulesOnHost (0.20s)
--- PASS: TestRouteOverVPPEntriesOnHost (0.21s)
ok  	ngfw/agent/internal/descriptors/core	2.428s
```
- Restart-safety with the real agent binary: kill -9 of the PIDs this test spawned, loss of the prefixed objects
  simulated via binapi, restart, resync.
```
$ TMPDIR=/tmp/g-w1 VRX_INTEGRATION=1 go test -count=1 -race -v -run 'TestAgentOnHost|TestAgentProcessOnHost' ./internal/agent/
    agent_integration_test.go:282: restart after loss: converged in 470.965446ms
--- PASS: TestAgentOnHost (6.43s)
    agent_integration_test.go:407: kill -9 2686161
    agent_integration_test.go:414: restart after kill -9 + loss: converged in 1.24509395s
    agent_integration_test.go:436: {… "msg":"resync finished","owner":"w1","why":"connect","status":"APPLY_STATUS_APPLIED","summary":"created:12"}
--- PASS: TestAgentProcessOnHost (8.51s)
ok  	ngfw/agent/internal/agent	16.226s
```
- Afterwards, `vppctl show interface` has no slot-1 loopback left, and both tests' own leftover checks passed.

## Out of scope and follow-ups
- Q2–Q7 in `TD-9-questions.md`:
  - the metrics.go hunk;
  - the proto comment;
  - TD-10a's warning filter;
  - the DEGRADED-on-panic decision;
  - the "covers" rule;
  - the TD-8b source-sync deadline.
- The review plan's optional "run the resync outside the watchVPP select loop" is not done. A resync is now bounded
  (5 min), and the States channel is buffered with drop-oldest, so the loop blocks for a bounded time at most.
- govpp v0.13 property, not changed: after a reply timeout the stream's channel id goes back to govpp's pool, so a very
  late reply could reach a later user of that id. The owed resync re-reads VPP either way. With reply timeouts rare
  (VPP dead or hung), the agent accepts this.
- `docs/tech-debt.md:16` ("P05 verify: a new Apply that itself FAILS still drops an owed revert") can be ticked; I do
  not own that file.
