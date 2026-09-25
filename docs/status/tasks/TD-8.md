# TD-8: agent seams (event publish/resync wiring, S1 dynamic desired source, metrics hook, fail-closed id range, ARCH-1 coverage)

branch `task/TD-8` · worktree `/root/ngfw-wt/TD-8` · slot none (unit tests only, no host runs) · base `task/W-seed@df67a8e`
(SPECULATIVE, D-114) · D-119 M2, D-123 · started 2026-09-24 18:08.
Code tip: `d4c9701 docs(agent): DynamicSource.Desired — DryRun calls it without the txn lock`. The CI gate ran on that tip (below). After it, only `docs/status/tasks/TD-8*` changed.

## What

The agent stays declarative through every seam. A feature publishes events and hands the agent desired state, which means KVs
of registered descriptors. Only the scheduler writes VPP, under the Service transaction lock. Features use the seams from
their own line in `subsystems.Register` (`internal/subsystems/seams.go`) and never edit `internal/agent`. Every seam does
nothing until a feature uses it.

1. **Event publish and Resync are wired (W-seed Q1).** `agent.Start` creates the bus before `subsystems.Register` and passes
   it to `NewService` (`ServiceConfig.Events`).
   - `Env.Publish` is `bus.publishFeature`. It publishes a copy of the event, so the publisher may reuse it. It clears `seq`,
     because each stream numbers its own events, and the bus sets `ts` when it is unset. `EVENT_KIND_UNSPECIFIED` and nil
     are dropped. So EventKind features (F-neighbors-ra 10, F-object-model 11, P11, P12, F-wireguard, F-acl) reach
     `StreamEvents` like the agent's own events.
   - `Env.Resync` is a non-blocking, coalescing send on a 1-slot channel, so it is safe inside a descriptor call, under
     the txn lock. `watchVPP` serves the request with `Service.Resync` and then `Wiring.AfterResync`. The connect path now
     uses the same `fullResync` helper. A request made while VPP is disconnected, including one where the state is stale
     and `conn.Connected()` is false, is dropped, because the reconnect resyncs anyway.
2. **S1 dynamic desired source.** `w.AddDynamicSource(DynamicSource{Name, Descriptors, Desired, Run})`.
   - The source's `Desired(doc)` is merged into the projection of every transaction (Apply, resync, confirm revert,
     DryRun) under the txn lock, and its descriptors are added to the scope. `doc` is a copy of the stored document as it
     will be after the transaction (`mergeDomains` for Apply).
   - `Run(ctx, sync)` starts once after the first resync and stops with the agent (`Stop` waits for it). `sync(ctx)` runs
     a transaction scoped to the source's descriptors only. It emits RECONCILE_START/DONE with the attribute `source`
     and counts in the reconcile metrics. It never changes the stored document or the confirm state, and its success
     does not clear DEGRADED. It returns UNAVAILABLE while VPP is down and ctx.Err() when cancelled.
   - Checks: `AddDynamicSource` refuses a malformed or duplicate name, a missing Desired, no descriptors, a descriptor
     that belongs to a domain, and a descriptor another source owns. `NewService` refuses an unregistered descriptor. A
     key outside the source's descriptors, or a duplicate key, fails the transaction (`agent.dynamic-source`,
     `agent.duplicate-object`, no pointer).
   - Dynamic objects are not configuration: Retrieve never returns them, and in results they have no pointer and no
     `subsystem`. Design options are in Q5.
3. **Metrics collector hook.** `w.AddMetricsCollector(MetricsCollector{Name, Collect(ctx, w) error})`.
   - Every `/metrics` scrape appends the collectors' families, sorted by name, after the agent's own. Collectors run
     outside `metrics.mu`, so a slow collector never blocks a transaction, with a 5 s deadline each on the request's
     context.
   - A collector's output is buffered. A collector that returns an error serves nothing and increments
     `vrx_agent_metrics_collector_errors_total{collector}`.
   - With no collector, the exposition is byte-identical to before.
4. **The id range goes through Env and fails closed.**
   - `subsystems.ResolveIDScope()` reads `VRX_VPP_TABLE_BASE=<base>` (base..base+999) or `VRX_VPP_ID_RANGE=all` (every
     id).
   - `ConfigFromEnv` stores the result in `Config.IDs`. `Start` passes it on as `Env.IDs`, and families read it with
     `w.IDRange()`, which never reads the environment.
   - Neither variable set: the zero scope, so `w.IDRange()` returns `ErrNoIDRange` and start-up warns.
   - Both variables set, a malformed base, or another `VRX_VPP_ID_RANGE` value: `Config.Validate` refuses to start.
   - `SlotIDRange()` is kept and now returns `ErrNoIDRange` when unset, instead of nil. Why "unset" does not refuse
     start-up yet: Q3.
   - The §12 text for `docs/lab/shared-host-rules.md` is below.
5. **ARCH-1 coverage table** in `docs/agent/README.md` (new file). It covers:
   - the 13 domains: agent status, wired descriptors, UI screen, and which features wire the rest;
   - every descriptor package: E2E, registered, or package;
   - the renderers;
   - a seams reference for feature authors.

   Today:
   - E2E: `interfaces`, `vrfs`, and `routing.static`. That is `core` + DF-1 minus rx-placement + `af_packet` +
     `dhcp.client`.
   - registered only: `interface.rx-placement`.
   - package only: every other family and every renderer. The agent uses only `frr.StaticOwnedByFRR`.

Files: `internal/agent/{agent,service,events,metrics}.go` (seam hunks), the new `internal/agent/seams_test.go`,
`internal/subsystems/{seams,seams_test}.go`, `internal/subsystems/subsystems.go` (+5 lines, Q1), `docs/agent/README.md`, and
`docs/status/tasks/TD-8*`.

### §12 for `docs/lab/shared-host-rules.md` (for the manager to copy)

```markdown
## 11. VPP numeric id ranges: explicit, fail closed (TD-8)
Every vrx-agent on this host has an explicit range for the VPP numeric ids its families allocate: FIB/VRF tables, SPD/SA ids,
policy, map and pool ids. No agent owns "every id" by default.

| agent | setting | ids |
|---|---|---|
| worker slot N (1–11) | `VRX_VPP_TABLE_BASE=N000` (`tools/lab env N`) | N000–N999 |
| CI slot 12 (`tools/ci.sh full`) | `VRX_VPP_TABLE_BASE=12000` | 12000–12999 |
| `tools/app`: the integrated main build, owner `vrx`, `/run/vrx/agent.sock` | `VRX_VPP_TABLE_BASE=13000` (reserved; there is no slot 13) | 13000–13999 |
| the product agent on a box of its own (P10's unit) | `VRX_VPP_ID_RANGE=all` | every id |

- `VRX_VPP_ID_RANGE=all` is never set on this shared host.
- Neither variable set: the agent owns no id. Start-up logs a warning, and a family that allocates ids refuses to register,
  so the agent then does not start.
- Both variables set, a malformed base, or any other `VRX_VPP_ID_RANGE` value: the agent refuses to start.
- A harness that starts `vrx-agent` with a clean environment passes `VRX_VPP_TABLE_BASE` through.
- Take VRF table ids typed by hand into tools/app from 13000–13999 too. The `vrf` descriptor does not check them.
```

## How verified

All runs are unit-only (`VRX_INTEGRATION` unset), in `/root/ngfw-wt/TD-8`, 2026-09-24 18:25–19:30, at host load 15–40.

### 1. The seam tests (`-race`)
```
$ cd apps/agent && env -u VRX_INTEGRATION go test -race -count=1 -v -run 'TestStartWires|TestStartIDRange|TestConfigFromEnvIDRange|TestMetricsCollectors|TestDynamicSource|TestNewServiceRefuses|TestWatchVPPResyncsOnEveryConnect|TestMetricsExposition' ./internal/agent/
--- PASS: TestStartWiresFeatureEventsAndResync (0.46s)
--- PASS: TestStartIDRangeFailsClosed (0.01s)
--- PASS: TestConfigFromEnvIDRange (0.00s)
--- PASS: TestMetricsCollectors (0.23s)
--- PASS: TestDynamicSourceMergedIntoEveryTransaction (0.24s)
--- PASS: TestDynamicSourceSyncIsScoped (0.07s)
--- PASS: TestDynamicSourceKeyOutsideItsDescriptorsFails (0.01s)
--- PASS: TestNewServiceRefusesUnregisteredSourceDescriptor (0.00s)
--- PASS: TestDynamicSourceRunLifecycle (0.06s)
--- PASS: TestMetricsExposition (0.00s)
--- PASS: TestWatchVPPResyncsOnEveryConnect (0.12s)
PASS
ok  	ngfw/agent/internal/agent	2.629s
$ env -u VRX_INTEGRATION go test -race -count=1 -v -run 'TestEventAndResync|TestSlotIDRange|TestWiringIDRange|TestAddDynamicSource|TestAddMetricsCollector' ./internal/subsystems/
--- PASS: TestEventAndResyncHooksDefaultInert (0.00s)
--- PASS: TestSlotIDRange (0.00s)
--- PASS: TestWiringIDRangeFailsClosed (0.00s)
--- PASS: TestAddDynamicSourceValidation (0.00s)
--- PASS: TestAddMetricsCollector (0.00s)
PASS
ok  	ngfw/agent/internal/subsystems	1.150s
```
What each test covers:

- **Default inert.**
  - `TestEventAndResyncHooksDefaultInert`: no hook set.
  - `TestAddDynamicSourceValidation`, `TestAddMetricsCollector`: empty registries by default.
  - `TestMetricsCollectors`: with no collector, the exposition has no collector family.
  - Every other test in both packages runs without sources, collectors or hooks and is unchanged. That is 89 Go packages in
    the gate below.
- **Wired path.**
  - `TestStartWiresFeatureEventsAndResync` runs `agent.Start` against a fake VPP (`dialVPP`).
    - `a.wiring.Publish` delivers to a subscriber as a copy: seq 1, ts set, interface and attributes kept. A change made
      after the publish does not leak. UNSPECIFIED and nil are dropped.
    - `RequestResync` ×5 while disconnected is dropped and never blocks. After connect, one request is one
      RECONCILE_START/DONE pair with message `resync …`.
    - `a.wiring.IDRange()` equals `cfg.IDs`.
  - `TestDynamicSourceMergedIntoEveryTransaction`:
    - an Apply creates the dynamic objects, with no pointer and no subsystem;
    - `Desired` gets a copy of the post-txn document;
    - Retrieve is the config only, and a repeat Apply is empty;
    - removing `loop702` deletes its dynamic object in the same txn;
    - a `routing`-only Apply keeps the dynamic objects;
    - DryRun plans the dynamic create;
    - a resync repairs a lost dynamic object;
    - a confirm revert re-merges against the baseline.
  - `TestDynamicSourceSyncIsScoped`:
    - a sync creates and deletes dynamic objects, with events that carry `source=test-sync`;
    - it does not repair a deleted config loopback, which a resync does;
    - the stored document and the last txn are unchanged;
    - it returns UNAVAILABLE while VPP is down and `context.Canceled` when cancelled;
    - an unknown source is an error.
  - `TestDynamicSourceRunLifecycle`: `Run` starts after the first resync, and its sync is APPLIED. A reconnect does not start it
    again, and a cancel stops it: 1 start, 1 stop.
  - `TestMetricsCollectors`:
    - the agent's families are a prefix, and a missing trailing newline is added;
    - a failing collector serves nothing and counts 1;
    - a collector that honours its deadline is dropped on a cancelled scrape and on a timeout through the HTTP handler.
- **Fail closed.**
  - `TestSlotIDRange` / `TestConfigFromEnvIDRange`:
    - unset → `ErrNoIDRange`, the zero scope, and start allowed;
    - `all` → every id;
    - base `3000` → 3000–3999;
    - both set, `ALL`, `1000-1999`, `0`, `x` → an error, no id, and `Validate` refuses.
  - `TestWiringIDRangeFailsClosed`: a zero `Env.IDs` gives `ErrNoIDRange` even with `VRX_VPP_TABLE_BASE` set in the environment.
    The returned range is a copy.
  - `TestStartIDRangeFailsClosed`: a Config built in code gives `ErrNoIDRange`.
  - `TestDynamicSourceKeyOutsideItsDescriptorsFails`: the sync errors, and the config Apply is FAILED with `agent.dynamic-source`
    and nothing applied.
  - `TestNewServiceRefusesUnregisteredSourceDescriptor`.

### 2. The tests catch a missing wiring (mutations with `go test -overlay`)
```
mutation 1 (Start without the Publish/Resync hooks):
--- FAIL: TestStartWiresFeatureEventsAndResync (5.01s)
    seams_test.go:73: waiting for 1 events, got []
mutation 2 (applyLocked without the S1 merge):
--- FAIL: TestDynamicSourceMergedIntoEveryTransaction (0.04s)
    seams_test.go:340: dynamic objects after the config apply: ""
```

### 3. `make -C apps/agent lint test` (`go vet`, golangci-lint 2.13.2, `go test -race ./...`)
```
go vet ./...
0 issues.
go test -race -count=1 ./...
… 89 packages "ok", 0 "FAIL" (incl. ok ngfw/agent/internal/agent 14.244s, ok ngfw/agent/internal/subsystems 1.170s)
real	2m42.505s
```

### 4. CI gate: `TMPDIR=/tmp/g-TD-8 tools/ci.sh --base main` on `d4c9701` → **CI GATE PASSED**
```
branch    task/TD-8 @ d4c9701   (base: main)
ok — contract commit(s) on the branch:
  6ce08c2 contract(api-client): … (P08) · 5c6e1f8 contract(wave-A): anchors · f6fbdf3 … · c02aa32 … · 51b7c42 contract(proto): … (P08)
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   1m51s
  forbidden patterns (+ gitleaks)                    0m05s
  lint · typecheck · unit tests · build (turbo)   3m17s
  apps/agent: make lint test build                   0m47s
  apps/cli: make lint test build                     0m19s
  test/ Go modules, unit mode (test/integration/smoke test/topology/interfaces)   0m08s
  warnings:
    - uncommitted changes in the worktree …: ?? docs/status/tasks/TD-8.md   (this file, committed afterwards)
    - commit subject(s) not in Conventional Commits form: review(W-seed): verify   (W-seed's, inherited)
  mode quick · wall time 6m31s · logs /root/ngfw-wt/logs/ci/TD-8-20260924-191916-2501408
CI GATE PASSED
```
Getting to that pass took several runs:

- **The contract guard.** It failed on some runs (F1 in the questions file: `grep -q` under `pipefail` makes git log exit
  141). It passed on attempt 3 of the final loop. The retry loop retried only on that exact message.
- **Two earlier gates** failed only in `@ngfw/web#test`, at host load 28–41, on web tests TD-8 does not touch.
  `git diff df67a8e HEAD -- apps/web packages` is empty.
  - `App.test.tsx › code-splits the developer routes …` timed out in 30 s.
  - `flows.test.tsx › redirects to /login?next=…` could not find `Sign in to VRX`. It also failed when run alone at load 37.
    It passed in the final gate.

  These are the base's load-sensitive tests. The manager may want a timeout pass like `5204b81`.

## Out of scope

- No feature uses the seams yet: no EventKind, no dynamic source, no collector, no family with an id range. They are the
  features' own lines under the wave-A/B/C anchors.
- `tools/app`, `test/topology/interfaces` and P10's unit get their id-range setting from the manager (Q3). The contract guard
  bug in `tools/ci.sh` is F1.
- No host runs. The seams talk to VPP only through the scheduler, so the fake VPP is the whole surface. Agent integration
  tests are unchanged.

## Questions

`TD-8-questions.md`:
- Q1: the 5-line `subsystems.go` hunk.
- Q2: the new test file and the `dialVPP` test seam.
- Q3: fail closed = owns no id, plus a one-line flip to refusing start-up once `tools/app` and the topology harness are updated.
- Q4: the `SlotIDRange()` change of meaning.
- Q5: the S1 design, for LOG.md.
- F1: the `ci.sh` contract guard fails intermittently under `pipefail`.

## Fix round 1 (review `d8891da`: APPROVE WITH CHANGES; manager answers D-129)

Code: `95d7dde fix(agent): TD-8 fix round 1 …`. It builds on `ca435ec` (the review's probes as tests) and the salvage
commit `09ce0f3`. The S1 code now lives in `internal/agent/dynsource.go`. `service.go` keeps only the calls into it.
Everything is unit-tested, no host runs. The agent stays declarative: the scheduler, under the txn lock, is still the
only writer, and no state is persisted beyond the stored document.

### What changed

| finding | fix | tests |
|---|---|---|
| R1 HIGH: cold source cache flushes dynamic objects on restart | **Readiness gate.** Every source starts out of sync. It is in sync after its first successful sync: Run's first sync, or, for a source without Run, the sync `startSources` runs once after the first resync. A source that is out of sync takes part in no Apply, resync, revert or DryRun: no KVs, and its descriptors are out of scope. | probe E `TestDynamicSourceAgentRestartKeepsDynamicObjects` (`deleted:0`); `TestDynamicSourceStartAndRunFailure/no_Run` |
| R2 HIGH: a dynamic object fails the commit or the post-restart resync | **Config-only fallback in `applySources`, under the same lock.** A source whose Desired panics, or returns a key outside its descriptors or a duplicate, is left out before the transaction runs. If the merged transaction then ends FAILED/ROLLED_BACK because of a dynamic key (plan issue, the first failed operation, or a Retrieve/verify error that names a source descriptor), the transaction runs **once** more without the sources. It does not rerun after DEGRADED or after a cancelled ctx. Each culprit is reported: a SKIPPED `ObjectResult` (key, source, cause; no pointer or subsystem), an `ERROR` event (`attributes.source/reason/key`, the txn id), and `vrx_agent_dynamic_source_errors_total{source,reason}` (`invalid`, `panic`, `rejected`, `stopped`; rendered only when sources exist). Every source left out is out of sync. The agent retries its sync with backoff (`retryMin` 5 s doubling to `retryMax` 60 s, one timer per source, stopped by `Close`). DryRun mirrors this at plan level with a WARNING issue `agent.dynamic-source-skipped`. | probe A `…DoesNotFailTheCommit`, probe B `…DoesNotRollBackTheResync`, `TestDynamicSourceKeyOutsideItsDescriptorsIsLeftOut` (was `…Fails`), `TestDynamicSourceLeftOutRejoinsThroughTheRetry` |
| R3 MEDIUM: panics | **Recovered.** Desired: `desiredOf` recovers and the source is left out (reason `panic`). A panic during a sync: recovered, and it also marks DEGRADED, because a descriptor that panics mid-transaction leaves no journal rollback. Run: recovered in the `startSources` goroutine. A Run that panics or returns before ctx is done stops its source until restart: out of sync, objects untouched, sync refused with FAILED_PRECONDITION. Collect: `runCollector` recovers, and the panic counts as a collector error. | probe C `TestDynamicSourcePanicIsContained`, `TestDynamicSourceStartAndRunFailure/{Run_panics,Run_returns_early}`, `TestMetricsCollectors` ("boom") |
| R4 MEDIUM: the id range fails open for a family that ignores the error | `Wiring.IDRange()` and `SlotIDRange()` return `NoIDs()` = `&IDRange{Lo: 1, Hi: 0}` with the error. Converters: `ids.DF2()`/`ids.DF7()` (nil → nil = every id) and `ids.VPN()` (nil → zero = every id; empty or 0..0 → `{1,0}`, owns nothing). The df2/df7/vpn packages are unchanged: their `Owns`/`Contains` already treat Lo > Hi as owning nothing, and nil/zero must keep meaning "every id" for the product agent. `SlotIDRange` is marked `Deprecated` (Q4). | `TestWiringIDRangeFailsClosed` (direct casts and the converters), `TestSlotIDRange`, `TestStartIDRangeFailsClosed` |
| R6 (low): sync from inside a transaction deadlocks | Refused at once with FAILED_PRECONDITION ("… inside a transaction …"). The guard compares goroutine ids (`goid()`, from the `runtime.Stack` header): the txn lock's holder, recorded only when sources exist, and the goroutines inside a Desired call, so DryRun is covered too. Documented on `SyncFunc`. | probe D `TestDynamicSourceSyncInsideATransactionRefused` (from Desired and from a descriptor Create) |
| R7 (low): lock order | `SyncFunc` doc and README rule 3: txn lock first, then the source's locks. Run updates its cache, unlocks, and only then calls sync. | docs |
| R8 (low): sync events | A sync that changes nothing emits no event, no metric and no `last_reconcile_at`. Any other sync emits `RECONCILE_START`/`RECONCILE_DONE` once it has finished. `docs/contracts/proto.md` §7 now says that `attributes.source` marks a source's sync, and that an `ERROR` event with `attributes.source` names a source that was left out or stopped. | `TestDynamicSourceSyncIsScoped` (the quiet sync) |
| R9 (low): resync storm | `watchVPP` defers a requested resync that arrives within `resyncMinInterval` (5 s) of the last one to the end of that interval, with a WARN. Requests made meanwhile coalesce into one. | `TestRequestedResyncsAreRateLimited` |
| R10a (low): disjoint objects | README rule 1 and the `DynamicSource` doc: a source's descriptors are instances of their own, and their Retrieve returns only the objects the source owns. | docs |
| ARCH-1 table | `dhcp`: a row for `dhcp.dhcp6-client`, `-pd-client`, `-pd-address`, `-duid` (package; no feature on the board, no schema field). `mpls`: no `mpls-route.ldp`; F-mpls-ldp adds its own instance. Renderers: `rfkit` (shared kit). | docs |
| review note (`metrics.go`) | `writeCtx` renders the agent's families into a buffer before it writes, so a stalled scraper never holds `metrics.mu`, which `observe` takes under the txn lock. | `TestMetricsExposition`, `TestMetricsCollectors` |

Behaviour notes:
- `vrx_agent_objects` counts only the configuration's objects of the last transaction. Dynamic ones are no longer
  included.
- R10b (freeze `AddDynamicSource` after `Start`) is not done. It was not in the fix envelope, and
  `TestAddDynamicSourceValidation` reads the registry before it adds to it. It needs an explicit `Seal()` that
  `Start` calls. That is ~10 lines, if the manager wants it.
- Q2's optional `Config.dial` (in place of the `dialVPP` package var) is not done.

### How verified

The review's probes fail on the pre-fix code. Command: `git archive ca435ec apps/agent` into the scratchpad, where
`ca435ec` = `d4c9701`'s code plus the probe tests, then `go test` there.
```
--- FAIL: TestDynamicSourceFailureDoesNotFailTheCommit      status APPLY_STATUS_ROLLED_BACK … create test.dyn/loop703: VPP: label already in use (-1)
--- FAIL: TestDynamicSourceFailureDoesNotRollBackTheResync  status APPLY_STATUS_ROLLED_BACK … create test.dyn/loop702: …
--- FAIL: TestDynamicSourcePanicIsContained                 Apply panicked: assignment to entry in nil map
--- FAIL: TestDynamicSourceSyncInsideATransactionRefused    … DeadlineExceeded after 3.001908875s (want an immediate refusal)
--- FAIL: TestDynamicSourceAgentRestartKeepsDynamicObjects  first resync after an agent restart: summary deleted:2 unchanged:12, dynamic ""
--- FAIL: TestWiringIDRangeFailsClosed                      zero Env.IDs: <nil> … (want a non-nil empty range and ErrNoIDRange)
```
On `95d7dde`:
```
$ cd apps/agent && env -u VRX_INTEGRATION go test -race -count=1 ./internal/agent/... ./internal/subsystems/...
ok  	ngfw/agent/internal/agent	13.575s
ok  	ngfw/agent/internal/subsystems	1.166s
$ env -u VRX_INTEGRATION go test -race -count=3 -run 'TestDynamicSource|TestRequestedResyncs' ./internal/agent/   → ok (9.4s)
$ make lint                                                                                 → go vet clean, golangci-lint 0 issues
```
- The probe-C nil-map write in the test is now an explicit `panic(...)` with the same message, because staticcheck
  SA5000 flagged it.
- CI gate: `TMPDIR=/tmp/g-td8 tools/ci.sh --base main` on `95d7dde` → **CI GATE PASSED** (quick mode, wall time
  6m10s, logs `/root/ngfw-wt/logs/ci/TD-8-20260924-225853-3811477`).
  - apps/agent `make lint test build`: 1m08s.
  - The only warnings are the subjects of the two `review(...)` commits, which are not Conventional Commits (TD-8
    review, W-seed).
  - After the gate, only the `SyncFunc` doc comment ("or the source is stopped") and this file changed.

### Manager answers (D-129), as applied

- **Q1 (a), Q2 both:** nothing to change.
- **Q4:** `SlotIDRange` is marked `// Deprecated: …`.
- **Q3, staged.** The flip "refuse start-up when no range is set" must merge before the first family that allocates
  ids. None of its pieces are in TD-8's files, so they are listed here for the manager:
  1. `tools/app` (the integrated main build): `VRX_VPP_TABLE_BASE=13000`.
  2. `test/topology/interfaces`: the harness must pass `VRX_VPP_TABLE_BASE` through to the `vrx-agent` it starts with a
     clean environment.
  3. P10's systemd unit: `VRX_VPP_ID_RANGE=all` in its EnvironmentFile, with a packaging test.
  4. Then `ConfigFromEnv` stops clearing `ErrNoIDRange` (`agent.go`, the `idsErr = nil` line) and start-up refuses.

  The §12 text above still applies.
- **Q5 (a), with the amendments.** Text for LOG.md:

  > S1 dynamic desired sources are merged into every transaction (option a) while in sync. Amendments: a readiness
  > gate (out of sync until the first successful sync; out of scope while out of sync), a config-only rerun when a
  > dynamic object fails the transaction, the source left out and retried with backoff, panic containment (Desired,
  > sync, Run, collectors), sync refused from inside a transaction, and source descriptors disjoint from config
  > descriptors (own instances, owned Retrieve). A failing dynamic object never rolls back a config transaction.

### R5, for `docs/tech-debt.md` (required before the first metrics collector merges; not TD-8)

```markdown
- 2026-09-24 (TD-8 review R5): /metrics collectors run one after another with a cooperative 5 s deadline each, and the
  metrics http.Server has no WriteTimeout. A collector that ignores ctx holds the scrape for as long as it runs, and
  three collectors at their deadlines take 15 s, past Prometheus' 10 s scrape_timeout, which then loses the agent's own
  families too. Fix before the first collector merges (F-dashboard-prom-alarms): run each collector in its own
  goroutine into its own buffer, wait on one per-scrape budget (e.g. 4 s in total), and abandon a late collector,
  counting an error. Keep one flight per collector, so a stuck collector is skipped instead of piling up goroutines.
  Add a WriteTimeout. (internal/agent/metrics.go writeCollectors, agent.go httpSrv)
```
