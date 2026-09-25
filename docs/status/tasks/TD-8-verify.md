# TD-8 verify: fix round 1 (focused, reviewer of `d8891da`)

Branch `task/TD-8` at `8d451ca` (code `95d7dde`), 2026-09-24 ~23:15. Read-only apart from this file: unit tests and
throw-away probes, run through `go test -overlay` or in `git archive` copies in the scratchpad. No host runs, no
`tools/ci.sh`.

**Verdict: APPROVE.**
- R1–R4 and the lows are fixed as asked. Every probe fails on the pre-fix code and passes now.
- `dynsource.go` keeps the agent declarative, and its lock order is sound.
- V1 (MEDIUM) is a finding that fix round 1 did not introduce: its S1 failure handling is all or nothing per source.
  - The seam is inert until a feature registers a source, so V1 does not block TD-8's merge.
  - It must be fixed before the first S1 feature (F-mpls-ldp, F-igmp-mfib) merges. Put it on the board as a gate.

## Evidence

```
$ cd apps/agent && env -u VRX_INTEGRATION go test -race -count=1 ./internal/agent/... ./internal/subsystems/...
ok  	ngfw/agent/internal/agent	13.942s
ok  	ngfw/agent/internal/subsystems	1.115s
$ go test -race -count=5 -run 'TestDynamicSource|TestRequestedResyncs|TestMetricsCollectors|TestStartWires|TestWiringIDRange|TestSlotIDRange' ./internal/agent/ ./internal/subsystems/
ok  	ngfw/agent/internal/agent	17.673s          (load 22–30; the timing tests did not flake)
ok  	ngfw/agent/internal/subsystems	1.224s
$ go vet ./internal/agent/ ./internal/subsystems/   → clean
```

**The author's probe tests on the pre-fix code.**
- Source: `git archive ca435ec`. `git diff d4c9701 ca435ec -- apps/agent` touches only the two `seams_test.go` files, so
  this is the pre-fix code plus the probe tests.
- Between `ca435ec` and `95d7dde`, probes A–E are unchanged except probe C's nil-map write, which became an explicit
  `panic` (SA5000). No assertion was weakened.
- Result: all six fail.
```
--- FAIL: TestDynamicSourceFailureDoesNotFailTheCommit      status APPLY_STATUS_ROLLED_BACK, want APPLY_STATUS_APPLIED
--- FAIL: TestDynamicSourceFailureDoesNotRollBackTheResync  status APPLY_STATUS_ROLLED_BACK, want APPLY_STATUS_APPLIED
--- FAIL: TestDynamicSourcePanicIsContained                 Apply panicked: assignment to entry in nil map
--- FAIL: TestDynamicSourceSyncInsideATransactionRefused    DeadlineExceeded after 3.0018s (want an immediate refusal)
--- FAIL: TestDynamicSourceAgentRestartKeepsDynamicObjects  summary deleted:2 unchanged:12, dynamic ""
--- FAIL: TestWiringIDRangeFailsClosed                      zero Env.IDs: <nil> … (want a non-nil empty range)
```
On `95d7dde` all of them pass (the runs above).

**My own probes, independent of the author's tests.**
- A new descriptor, `rv.dyn`, a source cache, and assertions. The file uses only APIs that exist on both trees.
- Run on `git archive d4c9701` and, with `-race -overlay`, on `95d7dde`.

| probe | d4c9701 | 95d7dde |
|---|---|---|
| A (R2): commit + rejected dynamic object | FAIL `ROLLED_BACK`, loop703 absent | PASS `APPLIED` |
| B (R2): resync after a VPP restart + rejected dynamic object | FAIL `ROLLED_BACK`, loop701/702 absent, degraded | PASS `APPLIED`, config rebuilt, not degraded |
| C (R3): Desired panics in Apply / Resync / DryRun / sync | FAIL, all four panic | PASS, none panics |
| D (R6): sync called from inside Desired | FAIL `DeadlineExceeded` after 2.0 s | PASS, refused at once |
| E (R1): agent restart, cold cache, VPP intact | FAIL `deleted:2` | PASS `deleted:0`, objects kept |
| G (new): after a VPP restart, one object rejected, one creatable | "" | "": the creatable object is not restored either (V1) |
| H (new): in sync; the commit deletes loop702 and adds loop703, whose dynamic object VPP rejects | `ROLLED_BACK` | `FAILED`: "cannot delete: rv.dyn/loop702 (not managed by this transaction) depends on it" (V1) |
| I (new): out of sync; the commit deletes loop702, which a live dynamic object depends on | `APPLIED` (deleted both) | `FAILED`, same message (documented; see V1) |

## Per finding

- **R1 fixed.**
  - `inSync` gate: every source starts out of sync; `activeSources` merges only sources in sync
    (`dynsource.go:109-118, 262-270`).
  - A source without Run gets one agent sync after the first resync (`agent.go` `startSources`).
  - Probe E: `deleted:0`.
- **R2 fixed as specified.**
  - `applySources` + `culprit` (`dynsource.go:214-285`) run the transaction once more without the sources.
  - No rerun after DEGRADED or after a cancelled ctx.
  - Reporting: a SKIPPED `ObjectResult`, an `ERROR` event with `source`/`reason`/`key`, and
    `vrx_agent_dynamic_source_errors_total{source,reason}`.
  - Retry: backoff `retryMin` 5 s doubling to `retryMax` 60 s, one timer per source, stopped by `Close`.
  - Probes A and B: APPLIED. The granularity of this fix is V1.
- **R3 fixed.**
  - `desiredOf` recovers panics in Desired.
  - `syncSource` and `retrySource` recover a panic in a sync. Their recover runs before the deferred unlock, so the
    txn lock is always released.
  - The `startSources` goroutine recovers a panic in Run, and a Run that returns early stops its source.
  - `runCollector` recovers a panic in a collector.
  - A panic in a source's *descriptor* during a config Apply is still not recovered: that is TD-9's gRPC interceptor,
    per the fix envelope. The R1 gate prevents the crash loop after the restart: the source is out of sync, so the
    first sync recovers.
- **R4 fixed for everything the seam returns.**
  - `IDRange()` and `SlotIDRange()` return `NoIDs()` = `{1,0}` with the error (`seams.go:174`).
  - I read every reader: df2 `Owns` (arp `range.go`, abf `policy.go`/`attach.go`), df7 `Owns`/`CheckID` (qos), and vpn
    `Contains`/`Check` (ipsec `sa`/`spd`/`spd_interface`/`spd_entry`). None of them uses `Lo`/`Hi` except in a range
    test, so `{1,0}` owns nothing in each.
  - ipsec's `NewCharonSweeper` overlap check with `{1,0}` can only refuse, which is the fail-closed direction.
  - Keeping nil/zero = "every id" in the kits is right. The product agent's `all` needs it, and flipping the kits
    would rewrite every package test outside TD-8's files.
  - The manager's question, whether that is enough: for the seam, yes. As a system guarantee, no: see V2.
- **Lows (the fix envelope's numbering).**
  - R6: guarded, probe D.
  - R7: the lock-order rule is in the `SyncFunc` doc and README rule 3.
  - R8: `proto.md` §7 sentence. It was in the fix envelope. `docs/contracts/**` is not a contract-guard path.
  - R9: `watchVPP` defers a request within 5 s and coalesces the ones made meanwhile
    (`TestRequestedResyncsAreRateLimited`).
  - R10a: the `DynamicSource` doc and README rule 1.
  - R10b (freeze after Start) was not done. The author says so; it stays LOW.
- **ARCH-1 table: all three corrections are right.**
  - A `dhcp` row for the four DHCPv6 descriptors, package-only. Correct: the schema has only the DHCPv4 client,
    `interfaces.ts:57`, and DHCPv6 appears only in the Kea `services` domain.
  - `mpls-route.ldp` is gone, with a note that F-mpls-ldp adds its own instance.
  - `rfkit` is listed as a shared kit.
- **Review note.** `writeCtx` now renders the agent's families into a buffer before writing, so a stalled scraper can
  no longer hold `metrics.mu`.

## `dynsource.go`: lock order and the declarative model

**Lock order.**
- Transactions: txn (chan) → the source's cache lock (Desired) → `sched.mu` → descriptors. The leaves `s.mu`,
  `metrics.mu` and `bus.mu` are taken briefly and released.
- Nothing takes txn while holding `s.mu`, `metrics.mu`, `bus.mu` or `sched.mu`.
- DryRun (`planSources`) takes `s.mu` only to copy `storedDoc`, then calls Desired, then `sched.mu.RLock`. It never
  takes txn.
- Timers (`retrySource`) and `sourceStopped` take only txn, with `context.Background()`, like the existing revert
  timer. `Close` stops them under txn, and a timer that fires after that sees `closed`.
- `inSync` is written only under txn and read atomically by DryRun.
- `holder` and `inDesired` are atomic or a `sync.Map`.
- `-race ×5` is clean.
- The one remaining deadlock is Run holding its cache lock across `sync` (ABBA). It is documented (README rule 3); a
  seam cannot prevent it.

**Declarative model: kept.**
- The scheduler is still the only writer, under the txn lock.
- The fallback is a second declarative apply of the config-only desired state, in the same critical section.
- "In sync" is process memory only; a restart starts out of sync by design. Nothing is persisted beyond the stored
  document, and Retrieve is unchanged.
- `goid()`, which parses the `runtime.Stack` header, is unusual. It is contained to the R6 guard and cheap. It is
  best effort: a sync from a goroutine that a descriptor or Desired *spawns* still deadlocks (V3).

## Findings

### V1 MEDIUM (gate before the first S1 feature, not TD-8's merge): failure handling is all or nothing per source
`dynsource.go:262-285` (fallback), `dynsource.go:396-420` (`syncLocked`)

A culprit takes the whole source out of scope, and a sync is one transaction over the whole source. Three
consequences, all measured:

- **G: one rejected object holds back the whole source.** After a VPP restart with one persistently rejected label,
  no label of that source comes back: the retry sync rolls back every time. For F-mpls-ldp, one stale FRR label means
  an LDP outage that lasts as long as VPP keeps rejecting that label.
- **H: a valid commit can still fail because of a dynamic object.** The commit deletes loop702, whose dynamic object
  Desired correctly drops, and at the same time adds a dynamic object that VPP rejects. The fallback drops the whole
  source, and with it the deletion of the orphaned dynamic object. The commit then fails with "cannot delete".
- **I: an out-of-sync source blocks config deletes of its dependencies.** This is documented in `seams.go:231`.
  Combined with G, "until the source is back in sync" can mean indefinitely.

The documents' headline, "a source never costs the configuration its transaction" (`seams.go:225`, README §S1),
overstates the current behaviour.

**Fix: quarantine per key instead of per source, still declarative.**
- On a culprit key, rerun with the source's KVs *minus the culprit's change*, keeping the source's descriptors in scope:
  - a Create: drop the key;
  - an Update: desire the planned op's `Old` value;
  - a Delete: keep `Old`.
- Quarantine the key, report it as today, and retry only it with backoff. `syncLocked` does the same.
- Tests: G restores the creatable object, and H's commit ends APPLIED with both deletions.
- Until then, qualify the headline: "never costs a transaction that does not delete what a live dynamic object depends
  on".
- Owner: a TD-8 follow-up in `dynsource.go`, about 60 lines. Board it as a prerequisite of F-mpls-ldp and F-igmp-mfib.

### V2 LOW (the Q3 flip task): the kits' defaults still fail open for a family that never asks
`seams.go:57-94`
- The seam's range is safe, but these families own every id:
  - a df7 family registered without `WithIDRange` (`Options.IDs` nil);
  - ipsec or ikev2 without `ipsec.WithIDRange` (zero `Config.IDs`);
  - a df2 family given `nil`.
- There is no df7 option that takes a pointer, so `ids.DF7()` needs a hand-written
  `func(o *df7.Options){ o.IDs = ids.DF7() }`. `df7.WithIDRange(ids.Lo, ids.Hi)` panics when the range is nil
  (`all`).

**Fix, with the Q3 flip:**
- add `df7.WithIDs(*IDRange)`;
- add a rule to the feature prompt: an id-allocating family takes its range only from `w.IDRange()`, never `nil` or a
  missing option, and its test asserts that `NoIDs()` owns nothing;
- reviewers check the Register line.

### V3 LOW: the R6 guard is per goroutine
`dynsource.go:511-532`
- A `sync` from a goroutine spawned inside Desired or a descriptor is not recognised and still deadlocks.
- **Fix:** add one sentence to the `SyncFunc` doc: "… or from any goroutine they start".

### V4 LOW: a verify error can blame a source unfairly
`dynsource.go:241`
- `culprit` matches `"<descriptor>/"` in the verify text. When config keys and dynamic keys both differ, the source is
  left out and the config-only rerun fails anyway.
- The config outcome is unchanged; the source is only out of sync until its retry.
- **Fix:** accept, or match only when every key the verify error names belongs to a source.

### V5 LOW (manager): §11 numbering collision
D-128 assigns `shared-host-rules.md` §11 to the trace ban (TD-20). TD-8's id-range text, "§11" in `TD-8.md`, and the
`ErrNoIDRange` message (`seams.go:47`, "…shared-host-rules.md §11") must take the next free number (§12) when the
manager copies it.

## Recommendation
- Merge TD-8.
- Board V1 as `TD-8b` (S1 per-key quarantine + the headline fix), gating F-mpls-ldp and F-igmp-mfib.
- Fold V2 into the Q3 flip task.
- Fix the V5 section number when copying §11. The ErrNoIDRange string can be fixed in TD-8b.

**APPROVE**
