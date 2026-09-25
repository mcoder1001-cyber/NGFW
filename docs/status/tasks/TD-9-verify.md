# TD-9 verify: fix round 1 (focused)

- Branch: `task/TD-9` @ `5417411d`
  - fixes `4409d1e8`, lint `fe2dd46f`, docs `5417411d`
  - not squashed
- Checked against: the review `b30547fb` (TD-9-review.md), findings M1–M3, L2, L3, L5 and L6.
- Scope: only those findings, plus a regression run. M4 (the API filter) and L1, L4, L7, L8 and L9 stay with the manager and the follow-up rows named in the review.
- Run on 2026-09-25, 04:40–05:00, with no host runs.

## Verdict: APPROVE, with one merge condition (X1: TD-11c)

- Every finding in scope is fixed, and each fix has a test.
- My three probes from the review, run again unchanged on the fixed code, now show the correct behaviour.
- The race run is green.
- **X1** is not a defect of this branch. TD-9 and the approved TD-11c merge without a textual conflict, and the combined tree builds and passes its tests. But the automatic merge leaves one semantic gap that git will not flag. **Whichever of the two branches merges second must add the change described under X1.**

## Per finding

| # | fix (branch file:line) | evidence | status |
|---|---|---|---|
| M1 | See "M1 in detail" below. | See below. | **fixed** |
| M2 | `agent/service.go:473` `retryable = res.Uncertain \|\| timedOut(res) \|\| …`. `timedOut` (`:591`) applies `vpp.IsTimeout` to `res.Err` and to every result's `Err`: plan, operation, verify, undo. proto.md §2 item 3 now reads "an outcome in which a timeout took part is never stored". | See below. | **fixed** |
| M3 | `service.go:516-520`: `if covers {clear} else if s.isDegraded() { s.oweResyncLocked() }`. After the supersede `Reverting` is false, so the resync is armed. `covers` is computed over `implementedOnly(Managed)` (`:497`). proto.md's DEGRADED row says a narrower Apply leaves DEGRADED while the owed resync keeps retrying. | See below. | **fixed** |
| L2 | `agent.go:421` `panicked("hook")`; `metrics.go:64` has 4 labels, and the HELP text is updated | `TestHookPanicsHaveTheirOwnLabel` (`td9_fix1_test.go:111`) | fixed |
| L3 | `service.go:537-546`: `FlushClaims` runs on `WithTimeout(WithoutCancel(ctx), flushClaimsTimeout = 30 s)` (`:587`). A failure answers DEGRADED and is not stored. | `TestFlushClaimsContextAndFailure` (`:123`): the deadline is under 1 min with a transaction deadline of 1 h, and the retry is APPLIED, not the stored DEGRADED. Compatibility with TD-11c: see X1. | fixed |
| L5 | `service.go:857` `driftPlanTimeout = 60 s`, used at `:886`, not the transaction's 5 min | `TestDriftPlanIsBoundedTightly` (`:152`): a 200 ms bound with 1 s dumps finishes in under 2 s | fixed |
| L6 | See "L6 in detail" below. | See below. | fixed |

### M1 in detail

**The fix:**
- `vpp/conn.go:264`: `Conn.Invoke` now runs `invokeStream` (`:275`), a stream opened with `core.WithReplyTimeout(c.opts.ReplyTimeout)`. govpp's process-wide `core.DefaultReplyTimeout` can no longer cut it short.
- `invokeWithin` also maps `core.ErrReplyTimeout` to `ErrTimeout` (`:310`).
- The scheduler's `uncertain()` also matches govpp's reply-timeout text (`reconciler.go:284`), and `vpp.IsTimeout` (`conn.go:34`) does the same for storing.

**The reply decode is stricter than govpp's.** It checks that the reply's type is the type the caller asked for, while govpp decodes whatever reply arrives without checking. The worker re-ran the real-VPP path on slot 1: `TestCoreOnHost`, `TestClaimRulesOnHost`, `TestRouteOverVPPEntriesOnHost`, `TestAgentOnHost` and `TestAgentProcessOnHost` all PASS, with `NRestarts 2 → 2`. The only direct `Invoke` caller outside binapi is a test helper.

**Evidence:**
- My probe from the review, run again unchanged (`core.DefaultReplyTimeout = 200 ms`, `Conn.ReplyTimeout = 1 s`, a VPP that never replies):
  - before: `took 201ms err=no reply received … isTimeout=false isDeadline=false`
  - now: `took 1.001s err=vpp: no reply in time: context deadline exceeded: show_version: 1s isTimeout=true isDeadline=true`
- The worker's `TestInvokeHonoursAReplyTimeoutAboveGovppsGlobal` (`conn_fix1_test.go:22`) uses the same parameters and asserts both the type and the duration, so it is equivalent to my probe.
- `TestGovppReplyTimeoutIsUncertain` (`reconciler_fix1_test.go:13`) shows that a govpp timeout, whether wrapped or formatted with `%v`, gives DEGRADED plus `Uncertain`.
- `TestInvokeDecodesTheReply` shows that the decode path still works.

### M2 in detail

- My stored-FAILED-replay probe, run again (the plan's dump returns `ErrTimeout`, then VPP recovers, then the same txn_id is sent again):
  - before: `retry … APPLY_STATUS_FAILED` (the stored answer)
  - now: `APPLY_STATUS_FAILED "…no reply in time…"`, then `retry, same txn_id, VPP answers: APPLY_STATUS_APPLIED`
- `TestTimeoutOutcomesAreNeverStored` (`td9_fix1_test.go:28`) covers the plan (FAILED), verify (ROLLED_BACK) and verify+rollback (DEGRADED). In each case the retry with the same txn_id is APPLIED.

### M3 in detail

- My probe from the review, run again. Managed is `[vrfs routing]`, a routing revert is owed, and an Apply sends only vrfs:
  - before: `resyncTimer=false`
  - now: `resyncTimer=true`, with `degraded=true`
- `TestNarrowerSupersedeOfAnOwedRevertStillResyncs` (`:80`) also proves that the agent converges: the baseline route comes back, and DEGRADED is cleared by the owed resync.

### L6 in detail

**The fix:**
- `agent.go:141` `MinVPPReplyTimeout = 15 s`.
- `Validate` (`:128`) refuses values from 0 to 15 s (both excluded).
- The floor is documented in `cmd/vrx-agent/main.go:14` and in `docs/contracts/proto.md:138`.

**Evidence:**
- `TestReplyTimeoutBelowTheHealthCheckWindowRefused` (`:167`): `5`, `1500ms` and `14s` are refused; `15`, `30s` and `2m` are accepted.
- **proto.md is documentation only.** `tools/ci.sh:50` sets `CONTRACT_PATHS=(packages/schema packages/proto apps/agent/gen packages/api-client/src/generated)`.
- The simulated D-112 rebase of TD-9 alone onto main `4f472cc7` (`git merge-tree --merge-base 8a96a9ce main HEAD` gives tree `314b2f1c`, with no conflicts) changes 27 files, and **none of them is a contract path**. So the squashed commit is `fix(agent): …`.

## X1: TD-9 × TD-11c (merge condition)

**Why the hook is not used.** TD-11c (approved, `task/TD-11c` @ `e83c6318`) does not use TD-9's `ServiceConfig.FlushClaims` hook. It adds its own `claimsBatch()` (`s.claimsTxn`, set by `Start`), which is flushed before the outcome switch (TD-11c review F2). So compatibility here means the two are compatible when merged, not that TD-11c calls this hook.

**The combined tree.** `git merge-tree --write-tree --merge-base 8a96a9ce task/TD-11c HEAD` gives tree `6e19e3b4`:
- There is no conflict.
- `go build ./...` and `go vet` are clean.
- `go test ./internal/agent/ ./internal/subsystems/ ./internal/scheduler/ ./internal/vpp/` is all ok, including TD-11c's `TestClaimStoresBracketEveryTransaction`.
- In the combined `applyLocked`, TD-9's `applied_at` deadline (1.5b), its rule for clearing DEGRADED and the M3 re-arm all survive.

**The gap.** In the combined tree, a DEGRADED answer from TD-11c's failed claim flush (`service.go:489-493` in that tree, `claimsNotPersisted`) **is stored under the txn_id**, because `retryable` is not set. That contradicts proto.md §2 item 3 and D-entry (a). Probe on the combined tree:

```
first: APPLY_STATUS_DEGRADED "claim stores not persisted: disk full" |
retry same txn_id after the disk recovered: APPLY_STATUS_DEGRADED "claim stores not persisted: disk full"   (repeated txn_id, stored response)
```

TD-9's own hook (`:551-560` in that tree) then becomes a second, always-nil flush path.

**Required change at the second merge.** This is the merger's add-on, or a one-line fix on whichever branch rebases second. It is about three lines plus a test update:
1. After TD-11c's `claimsErr := flushClaims()` block, add `retryable = retryable || claimsErr != nil`. This way a claims failure is never stored, whatever the answer's status.
2. Remove TD-9's now-dead hook: `ServiceConfig.FlushClaims`, `Service.flushClaims`, `flushClaimsTimeout` and the `if s.flushClaims != nil {…}` block.
3. Move `TestFlushClaimsContextAndFailure`'s not-stored assertion onto `s.claimsTxn`: a failed batch end gives DEGRADED, and after the store recovers, a retry with the same txn_id is APPLIED.
4. Its ctx-deadline assertion is dropped, because TD-11c's flush takes no ctx.

Git reports no conflict here, so **the manager must pass this condition to the merger**. Otherwise it will be lost.

## Regression run

`cd apps/agent && env -u VRX_INTEGRATION go test -race -count=1 ./internal/agent/... ./internal/scheduler/... ./internal/vpp/...` on `5417411d`:

```
ok  ngfw/agent/internal/agent        20.764s
ok  ngfw/agent/internal/scheduler     1.693s
ok  ngfw/agent/internal/vpp           3.172s
ok  ngfw/agent/internal/vpp/bootid    1.097s
ok  ngfw/agent/internal/vpp/fake      1.113s
ok  ngfw/agent/internal/vpp/ifsanitize 8.001s
ok  ngfw/agent/internal/vpp/vpptest   1.090s
```

That is 7 ok and 0 FAIL. The worker's quick gate was `CI GATE PASSED`: lint reported 0 issues, and `go test -race ./...` passed 89 packages. The host runs are cited under M1.

The VPP restart at 04:27:36 (`NRestarts` 1 → 2) happened between the worker's rounds, and the worker did not cause it. The manager should find out what caused it, separately from TD-9.

## D-entry texts: checked against the code after fix round 1

The texts in TD-9-review.md still hold, with these corrections. The manager copies the versions below.

### (a) Agent timeouts and "outcome unknown"

Scope: TD-9, review 1.1/1.1b/1.1e/1.4 and ARCH-01 (agent half). It amends proto.md §2 item 3, the Outcomes table, and AD-4 step 4.

**Bounds**
- One VPP reply: 30 s (`VRX_AGENT_VPP_REPLY_TIMEOUT`, in seconds or as a Go duration).
  - **At least 15 s**, which is govpp's health-check window. A lower or invalid value refuses to start.
  - It covers an Invoke round trip, on a stream with this reply timeout, and each message of a dump.
  - govpp's global default is set to the same 30 s, as a backstop.
  - govpp's own reply timeout counts as a timeout.
- One transaction: 5 min on the agent's own clock once it holds the lock. This covers Apply, resync and confirm revert. The caller's deadline or cancel bounds only the wait for the lock.
- One rollback: 2 min on its own clock.
- The wiring's connect hook: 30 s.
- The drift check's Plan: 60 s.

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
  - an answer turned DEGRADED by a failed state save (ARCH-01) or a failed claim flush (D-133; with TD-11c this needs X1).
- A retry with the same txn_id runs again. This is safe because Apply is declarative.

**DEGRADED owes a resync**
- The resync is retried through the Env.Resync path with backoff from 5 s, doubling to 60 s.
- This is a repair after a failure, not a poll. D-132's floor of 30 s applies to periodic walks, and the steady state here is 60 s.
- While a confirm revert is owed, that revert's own retry stands in for the resync.

**Clearing DEGRADED**
- DEGRADED is cleared only by an APPLIED resync, an APPLIED confirm revert, or an APPLIED Apply whose domains cover every managed domain that this build implements.
- A narrower Apply never clears it, and neither does a dynamic-source sync. When a narrower Apply supersedes an owed revert, the owed resync is armed.
- The API's commit and reconcile always send every implemented domain.

**Drift check**
- A Plan, bounded to 60 s, of the stored desired state, every 5 min, under the transaction lock. It is skipped while a transaction runs.
- It is reported as `vrx_agent_drift_objects` and as an ERROR event with `reason=drift` when the count changes.
- It never corrects anything.

**Panics** are counted in `vrx_agent_panics_total{where=grpc|descriptor|transaction|hook}`.

**AD-4 step 4**
- While DEGRADED, the agent now resyncs automatically to its stored desired state.
- "Restart the data plane" stays manual.

**Why**
- govpp's default of 0 parked the agent forever when VPP died mid-request.
- ROLLED_BACK for an operation that was never journaled was a lie.
- A cached timeout made the "safe retry" unsafe.

**Alternatives considered**
- A timeout per descriptor: rejected, because it means about 80 call sites.
- Skipping the rollback after the first timeout: rejected, because a VPP that answers late would stay half-applied.
- Auto-correcting drift: deferred, because it is a product decision.

### (b) Confirm deadline from applied_at

Unchanged, and still matches the code: `service.go` sets `deadline := applied.Add(…)`. `confirm_deadline = applied_at + timeout`, as proto.md §4.1 says. This is a code-only change.

### (c) Agent start-up refuses invalid settings

Scope: TD-9, review 1.5c/1.5e and L6. The agent exits with status 1 when:
- `VRX_LOG_LEVEL` is anything other than debug, info, warn or error;
- `VRX_AGENT_VPP_REPLY_TIMEOUT` is invalid or **below 15 s**;
- `VRX_METRICS_ADDR` is not a loopback address and `VRX_METRICS_ALLOW_REMOTE=1` is not set, because /metrics is unauthenticated.

All current callers use `VRX_METRICS_PORT` on loopback and no reply timeout, so nothing breaks.

## Scratch

The probe files were scratch files under the reviewer's scratchpad (`probe2/`, `m11c/`), run against `git -C … archive` copies. None was committed, and no product code was edited.
