# TD-9 review: agent core, bounded VPP calls and transaction semantics

- Branch: `task/TD-9` @ `df4e4340`
  - code `99f12d4b`, proto.md `2445fcf5`, lint `ffe2b755`, docs `df4e4340`
- Base: `task/TD-8@8a96a9ce` (speculative, D-114). TD-8 is now on main as a squash.
- Reviewed diff: `git diff 8a96a9ce df4e4340`, which is TD-9 only (23 files, +2393/−106).
- The diff from `merge-base main HEAD` (`63178d29`) also contains the unsquashed P08, W-seed and TD-8 history (126 files), so it was not used as TD-9's diff.
- Reviewer run: 2026-09-25, 04:10–04:40. Host load 43–49. No host runs, as the envelope allowed.

## Verdict: APPROVE WITH CHANGES

The design is right and most of the scope is done well:

- Every Invoke and every dump message is bounded.
- The transaction runs on the agent's own clock once it holds the lock.
- The rollback runs on its own `WithTimeout(WithoutCancel(ctx), 2 min)`.
- A descriptor panic is recovered into `ErrDescriptorPanic` at every descriptor call site. The normal rollback runs, and the transaction lock is always released.
- ARCH-01 (a failed state save answers DEGRADED) is correct.
- The lock order is unchanged (txn → source cache → `sched.mu`).
- Tests: the race run and the merged-tree run are green. The base-first evidence is sound (spot-checked below).

Three defects in TD-9's own files break the outcome contract that TD-9 itself wrote into proto.md §2. Each fix is small and needs a test:

- **M1**: setting `VRX_AGENT_VPP_REPLY_TIMEOUT` above 30 s silently caps Invoke at 30 s. A timeout there is then classified as certain, so the answer is ROLLED_BACK and it is stored.
- **M2**: an outcome decided by a timeout in the plan, verify or rollback phase is still stored under the txn_id.
- **M3**: an Apply that supersedes an owed revert without covering every managed domain leaves the agent DEGRADED with nothing retrying.

**M4** is a regression in another component (the API), and the manager must decide how to handle it before the merge.

After the fixes, a focused re-verify is enough: the M1–M3 diffs, their tests, and the merge recipe below. A full re-review is not needed.

## Findings

| # | sev | where (branch file:line) | finding | required change |
|---|---|---|---|---|
| M1 | M | `vpp/conn.go:36`, `:251`, `:263`; `scheduler/reconciler.go:270` | See "M1 in detail" below. | Make Invoke honour `ReplyTimeout` in both directions, map `core.ErrReplyTimeout` to `ErrTimeout`, and add a test (see below). |
| M2 | M | `agent/service.go:471` against proto.md §2 item 3 | See "M2 in detail" below. | Widen `retryable` in the code (see below). Retries are safe because Apply is declarative. |
| M3 | M (latent) | `agent/service.go:486-494`, `:495`, `:512-514`, `:632-633` | See "M3 in detail" below. | Arm the owed resync when a narrower Apply supersedes the revert, and add a test (see below). |
| M4 | M (API) | main `apps/api/src/commit/commit.service.ts:586-591`; TD-9 `service.go:472-477` | See "M4 in detail" below. | Manager's decision (see below). |
| L1 | L | main `packages/proto/vrx/v1/dataplane.proto:194` (branch `:172`) | The comment on the validation field is stale: "Validation issues when status is FAILED … (empty otherwise)". The wire format is unchanged, and proto.md §2 is the authoritative description. The actual damage of the stale semantics is M4. | The next `contract(proto)` commit on any branch takes the Q3 wording, which is fine. Track it in tech-debt. |
| L2 | L | `agent/agent.go:415` | `safely()` counts panics in wiring hooks and in the link watcher as `vrx_agent_panics_total{where="transaction"}`. They are not transaction panics. | Give them a fourth label (for example `hook`) and update the HELP text. |
| L3 | L | `agent/service.go:531-536` | See "L3 in detail" below. | Optional now. Coordinate with TD-11c. |
| L4 | L | `service.go:632-642`, `:752`; `agent.go:456` | See "L4 in detail" below. | Record this exemption in the D-entry (text below). No code change. |
| L5 | L | `service.go:826-846` | `CheckDrift` holds the txn lock for a whole Plan, with the 5-min transaction deadline as its only bound. With a slow VPP, an Apply can wait minutes behind a drift walk. | Optional: bound the drift Plan more tightly, for example to 60 s. |
| L6 | L | `vpp/conn.go:244-306`; govpp v0.13 `core/stream.go:74-92`, `connection.go:335-350` | See "L6 in detail" below. | Refuse a value below the health-check window (for example 15 s) in `Config.Validate`, or document the floor. |
| L7 | L | `dynsource.go:396`, `:491` (TD-8's file); `agent.go` `fullResync` | See "L7 in detail" below. | Track in TD-8b (Q7's one-liner) or TD-22. |
| L8 | L (API, informational) | main `commit.service.ts:255-268` | See "L8 in detail" below. | A TD-10a follow-up. |
| L9 | L (arch docs) | `docs/01-architecture.md` AD-4 step 4 | See "L9 in detail" below. | Put a one-line note in the D-entry (text below). |

### M1 in detail

**What happens.** `init` sets govpp's global `core.DefaultReplyTimeout = 30 s`. `Conn.Invoke` calls govpp's `Connection.Invoke`, which opens its stream with that global (govpp `core/stream.go:53`). `ConnOptions.ReplyTimeout` reaches only `NewStream` and the outer `invokeWithin` ctx.

With `VRX_AGENT_VPP_REPLY_TIMEOUT` at 60 s:
- govpp's own 30 s timer fires first and returns `core.ErrReplyTimeout` ("no reply received within the timeout period 30s").
- `invokeWithin` maps only the expiry of its own ctx (`tctx.Err()` is still nil at 30 s), so the error is neither `ErrTimeout` nor `context.DeadlineExceeded`.
- So `uncertain()` returns false, the transaction answers **ROLLED_BACK** while VPP may hold the effect, and that answer is **stored** under the txn_id.
- The setting also does not do what `main.go`, proto.md and the D-entry say.

**Probe** (scratch only, not committed). `core.DefaultReplyTimeout = 200 ms`, `Conn.ReplyTimeout = 1 s`, and a VPP that never replies:

```
took 201ms err=no reply received within the timeout period 200ms isTimeout=false isDeadline=false
```

Before D-108 fixed the rings, af_packet creates stalled for 40 s to 5.5 min. Raising this setting is the obvious operator response to a slow VPP, so this path is realistic.

**Required change:**
- Make Invoke honour `ReplyTimeout` in both directions. Either run the round trip through `conn.NewStream(tctx, core.WithReplyTimeout(c.opts.ReplyTimeout))` plus SendMsg/RecvMsg in conn.go, or set `core.DefaultReplyTimeout = opts.ReplyTimeout` in `Dial` (the agent has one Conn).
- In `invokeWithin`, also map `errors.Is(err, core.ErrReplyTimeout)` to `ErrTimeout`, as `timeoutStream` already does.
- Add a test: a Conn with `ReplyTimeout` above `core.DefaultReplyTimeout` (scaled down) returns `ErrTimeout` after `ReplyTimeout`.

### M2 in detail

**What happens.** `retryable = res.Uncertain || (ctx.Err() != nil && status != APPLIED)`. That covers only an operation cut off mid-call and the transaction deadline. The following are still stored:
- a VPP reply timeout in the **plan** phase (Retrieve): FAILED;
- a timeout in **verify**: ROLLED_BACK;
- a **rollback** cut by the 2-min bound: DEGRADED.

proto.md §2 item 3, as amended by this branch, says: "An outcome that a timeout decided is never stored — a VPP call that did not answer in time …".

**Probe.** The dump returns `ErrTimeout`, then VPP recovers, then the same txn_id is sent again:

```
with the dump timing out: APPLY_STATUS_FAILED "retrieve vrf: ip_table_dump: vpp: no reply in time: context deadline exceeded: a dump"
apply: repeated txn_id, returning the stored response txn_id=t2
retry, same txn_id, VPP answers: APPLY_STATUS_FAILED "retrieve vrf: …"
```

Today the API never retries with the same txn_id (its reconcile uses `Health.last_txn_id` and a new id), so the practical impact is small. It is still a direct contradiction of the contract text this branch wrote.

**Required change.** Widen `retryable` in the code rather than narrowing the doc:

```go
retryable = res.Uncertain || timedOut(res) || (ctx.Err() != nil && status != APPLIED)
```

`timedOut(res)` is true when `res.Err`, or any result's `Err` (verify or undo), wraps `context.DeadlineExceeded`.

Add a test with the plan-phase timeout above.

### M3 in detail

**What happens.**
1. A failed confirm revert sets `Reverting` and DEGRADED. `oweResyncLocked` skips the timer on purpose, because the revert's own retry stands in for it.
2. An Apply over fewer than all managed domains then ends APPLIED. It supersedes the revert: `stopRetryLocked`, `Reverting = false`.
3. `covers` is false, so DEGRADED is not cleared, and nothing arms the owed resync.

**Probe.** Managed is `[vrfs routing]`, a routing revert is owed, and an Apply sends only vrfs:

```
managed=[vrfs routing] reverting=false pending="" resyncTimer=false retryTimer=false
health degraded=true
```

The agent stays DEGRADED with no retry until a reconnect, a restart or a covering Apply. That breaks proto.md's DEGRADED row ("retries it with backoff … until one succeeds").

The API cannot reach this today: every commit and reconcile sends every implemented domain (see "Verification notes", dimension 2). Any other gRPC client can.

**Required change:**
- In the APPLIED `modeTxn` branch: `if covers { s.setDegraded(false, "") } else if s.isDegraded() { s.oweResyncLocked() }`. After the supersede, `Reverting` is false, so the arm is not skipped.
- Optional: compute `covers` over `implemented()` domains only, as resync does.
- Add a test.

### M4 in detail

**What happens.** Since review 1.2, an APPLIED or ROLLED_BACK answer carries the projection's warnings. Every real document has a non-empty `system` domain, which this agent does not implement, so every ROLLED_BACK answer now carries `/system agent.unimplemented-domain` (a WARNING).

The API copies `res.validation.errors` into the 422 problem's `errors` without a severity field. The UI therefore shows that warning as an error on `/system` for every rolled-back commit. TD-9 cannot fix this in its own files (Q4).

**Manager's decision.** Pick one:
- (a) land the one-line filter (`severity === ERROR` into `errors`, the rest into `warnings`) before or together with TD-9's merge, as a TD-10a hunk or a manager add-on; or
- (b) add a tech-debt row with owner TD-10a, due before P10, and accept the noise until then.

The reviewer recommends (a).

### L3 in detail

`FlushClaims` has no test, because it is nil on this base. Two further points:
- It gets the transaction's ctx, which may already be past its deadline. A flush that honours ctx would then turn a genuine APPLIED into DEGRADED right at the deadline.
- The dynamic-source sync (`dynsource.go` `syncLocked`) runs claim-creating descriptors and has no flush.

Recommendation:
- Call it with `context.WithoutCancel(ctx)` and a short bound of its own.
- TD-11c must add the same flush to `syncLocked` (D-133), and TD-11c adds the tests.

### L4 in detail (D-132)

The owed-resync retry reuses N2's `revertRetryMin = 5 s`. After a DEGRADED answer there are three full walks in the first 35 s (at 5, 15 and 35 s), then one every 60 s while DEGRADED persists. D-132 allows automatic walks no more often than every 30 s.

The steady state (60 s) complies. The early burst is a repair after a failure, not a poll. When VPP is down, a resync fails before it walks anything.

The drift check complies: every 5 min (`driftInterval`), under the txn lock, skipped while a transaction runs.

### L6 in detail (residual govpp risk)

After a reply timeout, govpp returns the channel ID to its pool. Its `Invoke` decodes whatever reply arrives on that channel without checking the message ID, so a late reply can land on a later call.

With the default settings this is mitigated: govpp's health check (2 s × 5, about 10–15 s) reconnects the socket before the 30 s reply timeout fires, and that drops late replies. The risk becomes reachable only if `VRX_AGENT_VPP_REPLY_TIMEOUT` is set below the health-check window.

### L7 in detail

- Dynamic-source syncs still run without a transaction deadline (Q7). Each reply and the rollback are bounded, but a sync of N operations can take up to N × 30 s while it holds the txn lock.
- The wiring's `AfterResync` hook has no overall deadline, unlike the connect hook.

### L8 in detail

The caller's deadline no longer cuts the transaction (1.4). An Apply that outlives the API's `VRX_AGENT_TIMEOUT_MS` (default 60 s) is therefore finished by the agent. The API's reconcile then re-applies running over it:
- The end state is consistent.
- But the user's commit is lost instead of promoted.

The reconcile should wait while `Health.reconcile_in_progress` is true, then check `last_txn_id`.

### L9 in detail

TD-9 makes the "reload last good config" step automatic at the agent level: the resync converges to the stored desired state while DEGRADED. This agrees with the declarative rule. "Restart the data plane" stays a manual step.

## Verification notes, by review dimension

### 1. The bounds

**Invoke**
- `invokeWithin` wraps the ctx in `WithTimeout(ReplyTimeout)`, and a caller's earlier deadline wins. govpp's `recvReply` waits on the reply channel, its own timer, and `s.ctx.Done()`.
- `invokeWithin` starts no goroutines.
- On a timeout, govpp closes the stream (`releaseAPIChannel` deletes the channel from the map and returns its ID). A late reply to an ID that is no longer in use is dropped ("Channel ID not known"). `sendReply` never blocks for long (non-blocking send, then `receiveReplyTimeout`). No sender or receiver is leaked.
- For the channel-reuse case, see L6.

**Dumps**
- `NewStream` puts `core.WithReplyTimeout` first. `RecvMsg` is bounded per message and also by the stream's ctx, which is the transaction's 5 min.
- `timeoutStream` only wraps errors. Nothing type-asserts `*core.Stream`.

**Connect path**
- The connect hook's ControlPing runs under `WithTimeout(ctx, 30 s)` inside `safely`.
- `vppVersion` has 5 s.
- govpp's own handshake uses a socket read deadline.

**Event subscriptions**
- `want_interface_events` goes through Invoke.
- govpp's watcher goroutine ends with its ctx.
- `runLinks` is in `a.wg` and ends on disconnect or stop.

**Residual (accepted).** A govpp socket write has no deadline. It can block only once the unix-socket send buffer is full, which the health-check disconnect makes practically unreachable.

**Transaction clocks**

| path | clock | where |
|---|---|---|
| Apply | `WithTimeout(WithoutCancel(caller), 5 min)` | `service.go:357` |
| Resync | `WithTimeout(stop-ctx, 5 min)` | `:805` |
| Confirm revert | `WithTimeout(stop-ctx, 5 min)` | `:735` |
| Drift check | `WithTimeout(stop-ctx, 5 min)` | `:844` |
| Owed-resync retry | the Env.Resync path, or `resyncLocked` with its own deadline | |
| Rollback | `WithTimeout(WithoutCancel(ctx), 2 min)` | `reconciler.go:787-792` |

The worst case for one Apply is 5 + 2 min, and 5 + 2 + 2 min when `applySources` runs a second time without the dynamic sources. The owed-resync timer (`time.AfterFunc`) is stopped by `Close` and by a successful clear.

### 2. Outcome semantics

- A timeout or cancel after the request was sent, or a panic, sets `TxnResult.Uncertain` and the outcome is DEGRADED. `reconciler.go:744-748` and `:810`.
- An operation that never started (its ctx was already done) stays certain, and it is retryable through `ctx.Err()`.
- Correct, apart from M1, which is a classification gap.

**Storing:** an uncertain outcome or a transaction deadline is not stored. The gaps are listed in M2.

**Clearing DEGRADED (checked against the API):**
- The API's Apply always carries every implemented domain:
  - commit: main `commit.service.ts:553` takes `v.subsystems` = `ROOT_KEYS ∩ Health.subsystems` (`validation.service.ts:96-97`).
  - reconcile: `commit.service.ts:280` takes `validation.implemented().subsystems` (`validation.service.ts:66-72`).
  - The agent reports `Health.subsystems = implementedDomains()` (branch `service.go:990`).
- So once the stored Managed ⊆ implemented, the API's reconcile after a DEGRADED answer covers every managed domain and clears DEGRADED.
- A dynamic-source sync never clears it (`dynsource.go` `syncLocked`), which is correct.
- The gap is M3.

**ARCH-01:**
- A failed write of `agent-state.json` after APPLIED answers DEGRADED, owes a resync, and is not stored. A failure of only the `desired.pb` mirror stays APPLIED (`errMirror`, `state.go:43`). Correct.
- Spot-checked on the base (`8a96a9ce` with the branch's `service_test.go`): `service_test.go:929: status APPLY_STATUS_APPLIED, want APPLY_STATUS_DEGRADED`. This matches the worker's evidence.

**proto.md compared with the code:**
- §2 item 3's "never stored": M2.
- "bounded by `VRX_AGENT_VPP_REPLY_TIMEOUT`": M1 above 30 s.
- The DEGRADED row's retry promise: M3.
- Everything else matches: UNAVAILABLE before the confirm half (1.3); INTERNAL on a panic; warnings on APPLIED and ROLLED_BACK only.

**The dataplane.proto comment:** L1, but its consumer impact is M4.

### 3. Panic recovery (1.1d/1.1e)

- **gRPC** (`server.go` `newGRPCServer`): the unary and stream interceptors recover a panic into INTERNAL. The panic value is logged, never put in the status. The per-panic counter works.
- **Scheduler `guard`** covers every descriptor call site:
  - Retrieve, both the public one and the per-descriptor one;
  - the whole plan phase (Normalize, KeyOf, Dependencies);
  - each `x.run`, verify, Reapply, and each undo.
- **What a panic answers:**
  - in an operation: the journal is undone, the answer is DEGRADED (Q5; agreed), and `markPanicked` records the result;
  - in planning: FAILED;
  - in an undo: REVERT_FAILED;
  - the error carries only the call and the key, never the value.
- **The rollback after a panic** is bounded, because it runs on its own `rbCtx`.
- **Lock release:**
  - In the Service, the defers are registered in the order `unlock` → `containLocked` → `cancel`, so they run as cancel → recover/reload → **unlock**. The txn lock is released even if `containLocked` itself panics.
  - In the scheduler, `ApplyWith` holds `s.mu` with a defer'd `Unlock`.
- **The one lock a panic can leak** is a descriptor-internal mutex taken without `defer`. `guard` cannot fix that. main's claim stores (`subsystems/stores.go`) all use `defer`.

### 4. The owed-resync retry and D-132

- **Backoff:** `oweResyncLocked` starts at `retryMin` = 5 s and doubles up to 60 s. `resyncRetry` takes the lock and calls `requestResync`, a non-blocking send to the Env.Resync channel that watchVPP serves through `fullResync` (Resync plus `AfterResync`). It then re-arms in case the request is dropped. A success stops it.
- **The drift check** is Plan-only (`planSources` → `sched.Plan`). It never applies anything. It sets the gauge and sends an ERROR event only when the count changes. It walks the managed domains' descriptors (sw_interface/ip_table/ip_route dumps and so on), holding the txn lock.
- **Cadence:** every 5 min. There is no full walk every 5 s. The early retry burst is L4.
- **Not TD-9's (observation):** D-132's "one walk at a time" is not enforced between lock-free RPC walks (DryRun, Retrieve, InterfaceState) and the drift check.

### 5. Concurrency

- **Lock order.** TD-9 adds no new lock. `isDegraded` and `setDegraded` take `s.mu` inside the txn lock and release it before touching timers. That keeps the order txn → `s.mu`, and txn → source cache → `sched.mu` through `planSources` and `applySources`, the same as TD-8's `syncLocked`.
- **Env.Resync.** `requestResync` never blocks: it is a buffered send with a `default` branch. So `resyncRetry` holding the lock while watchVPP waits for the lock cannot deadlock.
- **TD-11b's claim store** is only reached inside descriptor calls under `sched.mu`, so the order is unchanged.
- **The FlushClaims hook** runs after the status switch and `refreshSnapshotLocked` (`service.go:483-528`) and before `st.save` (`:537`), as TD-11c's note asks. A failure there turns APPLIED into DEGRADED and is not stored (D-133). See L3.
- **Race detector:** clean (dimension 7).

### 6. Merge fit

- `git merge-tree --write-tree main HEAD` gives **27 conflicted files** (agent.go, service.go, server.go, projection.go, subsystems/*, api/*, web/*, schema/*, dataplane.proto, tech-debt.md, …). They all come from the history: the branch carries P08, W-seed and TD-8 unsquashed, while main has them squashed (D-112). None comes from TD-9's diff.
- **The simulated rebase of TD-9 alone** (`git merge-tree --write-tree --merge-base 8a96a9ce main HEAD`) is clean:
  - against main `10059d57`, tree `17c5aa80`;
  - again against main `e74ac23c` after the board commit, tree `a4c65bfa`.
  - The result differs from main only in TD-9's 23 files. It contains **no contract path** (`tools/ci.sh:50`: `packages/schema packages/proto apps/agent/gen packages/api-client/src/generated`).
- So the `--base main` contract-guard failure comes only from main's own files, and the squashed commit should be `fix(agent): …`, not `contract(…)`.
- **Merged tree** (`17c5aa80`, extracted to a scratch dir with `git archive`; nothing was checked out in /root/ngfw):
  - `go build ./...` and `go vet` on the six touched packages: clean.
  - `go test -count=1 ./internal/agent/... ./internal/scheduler/... ./internal/vpp/...`: 7 packages ok.
  - `go test -race -count=1` on agent, scheduler, vpp, ownertable, dfkit/... and subsystems: 8 packages ok.
- **The merger must not use `git merge-base main HEAD`** (`63178d29`) for the D-112 squash. That would squash P08, W-seed and TD-8 into TD-9 and then conflict. Use:

```
git update-ref refs/archive/TD-9 HEAD
git reset --soft 8a96a9ce && git commit -m "fix(agent): bounded VPP calls and transaction semantics (TD-9)"
git rebase --onto main 8a96a9ce
tools/ci.sh --base main
```

### 7. Tests

- **Branch** (`df4e4340`): `env -u VRX_INTEGRATION go test -race -count=1 ./internal/agent/... ./internal/scheduler/... ./internal/vpp/... ./internal/descriptors/...` gives **71 packages ok, 0 FAIL** (agent 20.5 s, scheduler 1.9 s, vpp 2.2 s, dfkit 1.1 s).
- **The base-first method is sound:**
  - The new API is reached only through the `*_td9_helpers_test.go` shims, and the shims encode the base's real behaviour.
  - The quoted failures are behaviour failures: the panic propagates; "did not return within 5.3s"; ROLLED_BACK instead of DEGRADED.
  - `TestConfigReplyTimeoutAndMetricsOptIn` and `TestDriftCheckIsPlanOnly` test features that do not exist on the base. That is acceptable.
  - Spot check: `TestStateCrashInjection` on `8a96a9ce` fails exactly as quoted.
- **Missing tests:** M1 (a `ReplyTimeout` above govpp's global), M2 (plan and verify timeouts not stored), M3 (a narrower supersede), and the FlushClaims error path (L3, possibly in TD-11c).

### 8. Scope

- `metrics.go`: +22/−0. It is additive only: the fields `drift` and `panics`, `panicked()`, `setDrift()`, and two families. No TD-8 collector hunk is touched.
- `service_test.go`: only `TestStateCrashInjection`, which the envelope names (ARCH-01).
- `reconciler.go`: only the ApplyWith ctx, the rollback ctx, the guard wrappers, and the `Uncertain` classification. `executor.create` (TD-11b) and `topo` (TD-11c) are untouched.
- Everything else is in the envelope's file list.
- No VPP message name is written by hand (binapi only). Nothing reaches a shell. Panic values and stacks go to the log only.

## D-entry texts (the worker's drafts, corrected)

The corrections to the drafts in TD-9.md:
- (a) extends "not stored" to every timeout (M2);
- (a) fixes the rule for clearing DEGRADED, including the narrower-supersede case (M3);
- (a) records the D-132 exemption and the AD-4 note;
- (c) is new.

### (a) Agent timeouts and "outcome unknown"

Scope: TD-9, review 1.1/1.1b/1.1e/1.4 and ARCH-01 (agent half). It amends proto.md §2 item 3, the Outcomes table, and AD-4 step 4.

**Bounds**
- One VPP reply: 30 s (`VRX_AGENT_VPP_REPLY_TIMEOUT`, in seconds or as a Go duration; an invalid value refuses to start).
  - It applies to a request/reply round trip and to each message of a dump.
  - govpp's global default is set to the same value, so no govpp path waits forever.
  - Keep the value above govpp's health-check window (2 s × 5).
- One transaction: 5 min on the agent's own clock once it holds the lock. This covers Apply, resync, confirm revert and the drift check. The caller's deadline or cancel bounds only the wait for the lock.
- One rollback: 2 min on its own clock. It runs even after the transaction's deadline has passed.
- The wiring's connect hook: 30 s.

**Outcome unknown**
- An operation whose VPP call timed out or was cut off after the request was sent, or whose descriptor panicked, may have taken effect.
- It answers **DEGRADED**, never ROLLED_BACK. The journal is still undone.
- A descriptor panic answers:
  - in planning: FAILED;
  - in an undo: REVERT_FAILED.
- A panic in the agent's own transaction code answers INTERNAL, reloads the state from disk, and leaves the agent DEGRADED.

**Not stored**
- Nothing is stored under the txn_id for:
  - an outcome in which a VPP reply timeout or the transaction deadline took part (in the plan, an operation, verify, or the rollback);
  - an answer turned DEGRADED by a failed state save (ARCH-01) or a failed claim flush (D-133).
- A retry with the same txn_id runs again. This is safe because Apply is declarative.

**DEGRADED owes a resync**
- The resync is retried through the Env.Resync path with backoff from 5 s, doubling to 60 s.
- This is a repair after a failure, not a poll. D-132's floor of 30 s applies to periodic walks, and the steady state here is 60 s.
- While a confirm revert is owed, that revert's own retry stands in for the resync.

**Clearing DEGRADED**
- DEGRADED is cleared only by an APPLIED resync, an APPLIED confirm revert, or an APPLIED Apply whose domains cover every managed domain.
- A narrower Apply never clears it, and neither does a dynamic-source sync. A narrower Apply that supersedes an owed revert arms the owed resync.
- The API's commit and reconcile always send every implemented domain, so its reconcile after a DEGRADED answer clears it.

**Drift check**
- A Plan of the stored desired state, every 5 min, under the transaction lock. It is skipped while a transaction runs, so there is one walk at a time.
- It is reported as `vrx_agent_drift_objects` and as an ERROR event with `reason=drift` when the count changes.
- It never corrects anything. Auto-correction would be a product decision.

**AD-4 step 4**
- While DEGRADED, the agent now resyncs automatically to its stored desired state, which is the last good configuration.
- "Restart the data plane" stays a manual operator action.

**Why**
- govpp's default of 0 ("wait forever") parked the agent for good when VPP died mid-request.
- ROLLED_BACK for an operation that was never journaled was a lie.
- A cached cancel or timeout made the "safe retry" unsafe.

**Alternatives considered**
- A timeout per descriptor: rejected, because it means about 80 call sites.
- Skipping the rollback after the first timeout: rejected, because a VPP that answers late would stay half-applied until the resync.
- Auto-correcting drift: deferred, because it is a product decision.

### (b) Confirm deadline from applied_at

Confirmed as the worker drafted it:
- Scope: TD-9, review 1.5b.
- `confirm_deadline = applied_at + timeout`, as proto.md §4.1 always said.
- The code counted from the transaction's start, so a slow apply shortened the window, and an apply longer than the timeout reverted at once.
- This is a code-only change; the contract does not change.

### (c) Agent start-up refuses invalid settings

Scope: TD-9, review 1.5c/1.5e. The agent exits with status 1 instead of running on a default nobody asked for when:
- `VRX_LOG_LEVEL` is anything other than debug, info, warn or error;
- `VRX_AGENT_VPP_REPLY_TIMEOUT` is invalid;
- `VRX_METRICS_ADDR` is not a loopback address and `VRX_METRICS_ALLOW_REMOTE=1` is not set, because /metrics is unauthenticated.

Every current caller uses `VRX_METRICS_PORT` (tools/app, the topology harnesses, devstack), which means 127.0.0.1, so nothing breaks. A future F-dashboard or remote Prometheus must set the opt-in explicitly.

## Probes

The probe tests were scratch files in the reviewer's scratchpad, run against `git archive` copies. None was committed, and no product code was edited:
- `zz_probe_test.go` in `internal/vpp` (M1);
- two `zz_probe*_test.go` files in `internal/agent` (M2, M3).

Their output is quoted in "M1 in detail", "M2 in detail" and "M3 in detail".
