# TD-8 review: agent seams (Publish/Resync wiring, S1 dynamic sources, metrics hook, fail-closed id range, ARCH-1 table)

Reviewer: independent agent, 2026-09-24 ~19:35. Branch `task/TD-8` @ `0508a16`, diff base `task/W-seed@df67a8e`.
Unit-only review, as the brief asked: no host runs and no `tools/ci.sh`. The author's gate output (`CI GATE PASSED @d4c9701`)
was not re-run.

**Verdict: APPROVE WITH CHANGES.** Merge after a short fix round for R1–R4. The fixes touch only files TD-8 owns
(`service.go`, `agent.go`, `seams.go`).

The rest of the branch is sound:
- The event and resync wiring works and cannot deadlock.
- The dynamic source never mutates stored desired state.
- Every VPP write still goes through the scheduler, under the transaction lock, with a complete journal. A failed
  transaction that carries dynamic objects rolls back fully (probe A: `reverted:12`).

The problem is S1's failure semantics. As built, the dynamic source ties restart safety (rule 2) to the state of external
daemons (R1, R2, R3). S1 is inert until a feature calls `AddDynamicSource`, so nothing regresses today. Still, fix the
contract before F-mpls-ldp or F-igmp-mfib code against it, and before Q5 goes into LOG.md.

## Verification

```
$ cd apps/agent && env -u VRX_INTEGRATION go test -race -count=1 ./internal/agent/... ./internal/subsystems/...
ok  	ngfw/agent/internal/agent	11.714s
ok  	ngfw/agent/internal/subsystems	1.292s
$ go vet ./internal/agent/ ./internal/subsystems/        → clean
```

Reviewer probes: a throw-away test file injected with `go test -race -overlay`. The worktree is unchanged. The probes reuse
`seams_test.go`'s `memDesc`/`newSrcSvc` pattern with a `test.dyn` descriptor whose Create can fail.
```
A) user commit with a failing dynamic object: status=APPLY_STATUS_ROLLED_BACK msg="create test.dyn/loop701: VPP: label already in use (-1)" loop701-present=false
B) resync after config loss with a failing dynamic object: status=APPLY_STATUS_ROLLED_BACK … loop701=false loop702=false degraded=true
B') same resync once the dynamic object succeeds: status=APPLY_STATUS_APPLIED loop702=true
C) Desired panics inside Apply: recovered by the test only = nil map in the source cache   (txn lock released by defer)
D) sync from inside Desired (under the txn lock): rpc error: code = DeadlineExceeded desc = context deadline exceeded
E) agent restart: dynamic objects before="loop701,loop702" after first resync="" (status APPLY_STATUS_APPLIED, summary deleted:2 unchanged:12)
F1) 3 cooperative collectors at their deadline (100ms each): scrape took 310ms
F2) 1 collector ignoring ctx (sleeps 700ms, deadline 100ms): scrape took 710ms
```

## Findings (ranked)

### R1 HIGH: an agent restart flushes every dynamic object (cold source cache)
`agent.go:268-272`, `agent.go:307-316`, `service.go:367-373`
- `Run` starts only after the first resync. The first resync still merges the source (`Desired` over an empty cache)
  and puts its descriptors in scope, so the scheduler deletes every dynamic object that VPP still holds.
- Probe E: after an agent-only restart with VPP intact, the first resync is `deleted:2`. For F-mpls-ldp, every agent
  restart or upgrade blackholes all LDP label routes until FRR is polled and synced again.
- The same happens for any config Apply that lands before the source's first sync.
- This breaks the declarative restart property: restart with VPP intact should produce 0 operations.

**Fix:** a readiness gate.
- Until a source has completed one successful `sync`, the agent leaves it out of every transaction: no KVs, and its
  descriptors are out of scope. The scheduler then treats its live objects as out-of-scope and leaves them alone.
- Optionally, start `Run` before the first resync. `sync` already returns UNAVAILABLE while VPP is down.
- Test: probe E ends with the dynamic objects unchanged, and the first `sync` after `Run` starts reconciles them.

### R2 HIGH: a dynamic object's VPP failure fails the user's commit and rolls back the post-VPP-restart resync
`service.go:367-383`, `service.go:582-584`
- With option (a), dynamic KVs travel in the same transaction as the configuration. One dynamic Create that VPP
  rejects (label in use, a resource limit, a stale FRR entry) does the following:
  - (probe A) the user's commit comes back ROLLED_BACK, although the configuration is valid;
  - (probe B) the resync after a VPP restart rolls back every config object it had just recreated. The data plane stays
    empty and DEGRADED, and nothing retries until the next reconnect or `RequestResync`.
- The configuration in the datastore now depends on daemon-derived state that no commit pipeline ever validated. That
  contradicts rule 2 ("fully rebuild the data plane from the datastore after `kill -9 vpp`").
- The same applies to a source that emits a key outside its descriptors (`agent.dynamic-source`): every config
  transaction fails until the agent is fixed.
- Q5 calls this "the cost of (a)". I do not accept it as stated.

**Fix:** keep (a) and add a fallback inside `applyLocked`, under the same lock.
- Trigger: the result is not APPLIED/DEGRADED, and the cause is a dynamic key. That means a plan issue or the first
  failed `OpResult` whose key's descriptor is owned by a source, or an `agent.dynamic-source`/`agent.duplicate-object`
  issue raised by `addSources`.
- Action: re-run the transaction once without the sources (`scopeOf(domains)`, no dynamic KVs).
- Report it: a WARNING issue `agent.dynamic-source-skipped`, an `ERROR` event with `attributes.source`, and a counter.
- The source's next `sync` retries its own objects, where a failure touches only them.
- The rare remaining case is "config deletes what a dynamic object depends on". That still fails with the scheduler's
  "cannot delete … depends on it", which is the correct outcome.
- Tests: probes A and B end APPLIED with loop701/loop702 present and the source flagged.

### R3 MEDIUM: a panic in `Desired` (or `Run`/`Collect`) is not contained
`service.go:644`, `agent.go:316`, `metrics.go:117`
- `grpc.NewServer()` (`agent.go:185`) has no recovery interceptor. A `Desired` panic inside Apply therefore kills
  vrx-agent (probe C: only the test's `recover` caught it).
- On restart, the first resync calls `Desired` again, so the agent crash-loops and the configuration is never applied.
- A panic in `Run` (in its goroutine) also kills the process.

**Fix:**
- In `addSources`, wrap each `src.Desired` in `defer recover()` and turn a panic into `pj.errorf("", "agent.dynamic-source", "… panicked: %v")`. With R2 this falls back to a config-only transaction.
- Recover in the `startSources` goroutine: log it, and treat the source as stopped and not ready.
- Wrap `c.Collect` the same way and count a panic as a collector error.

### R4 MEDIUM: the "fail closed" id range fails open if a family ignores the error
`seams.go:120-128`
- `Wiring.IDRange()` returns `(nil, ErrNoIDRange)`. To the consumers, `nil` means "every id": `df2/scope.go:10`,
  `df7/options.go:41` (`r == nil || …`), and the zero `vpn.IDRange` (`vpn/keys.go:42`).
- A family that writes `ids, _ := w.IDRange()`, or logs the error and carries on, therefore owns every id on the shared
  host. That is the exact failure the envelope asked to prevent.

**Fix:**
- On `ErrNoIDRange`, return a non-nil empty range together with the error, e.g. `&IDRange{Lo: 1, Hi: 0}` ("owns
  nothing"). That value owns no id under df2/df7 `Owns` and under `vpn.IDRange.Contains`.
- Document it on `IDRange()`.
- Add a unit test that converts the returned value to `df2.IDRange`/`df7.IDRange`/`vpn.IDRange` and asserts that id 1 and
  id 13000 are not owned.

### R5 MEDIUM: the collector deadline is cooperative and additive
`metrics.go:114-121`, `agent.go:195`
- Collectors run one after another, each with a 5 s deadline. A collector that ignores ctx holds the scrape for as long
  as it runs (probe F2). The metrics `http.Server` has no `WriteTimeout`.
- Three collectors at their deadlines take 15 s, which is past Prometheus' default 10 s `scrape_timeout`. Then the
  agent's own families are lost too (probe F1, scaled).
- "Outside every agent lock" holds: `collectors()` copies under `seams.mu`, and `Collect` runs with no lock.

**Fix:**
- Run each collector in its own goroutine, into its own buffer. Wait with `select` on a per-scrape budget (for example
  4 s in total, all collectors in parallel), and abandon a late collector (count an error).
- Keep one flight per collector, so a stuck collector is skipped instead of piling up goroutines.
- Required before the first collector merges (F-dashboard-prom-alarms). It is not required for TD-8's merge.

### R6 LOW: `SyncFunc` is a blocking call on the non-reentrant transaction lock
`seams.go:158`, `service.go:668-720`
- Calling `sync` from `Desired` or from a descriptor deadlocks. Probe D blocks until the ctx deadline; with
  `context.Background()` it blocks forever and wedges every transaction.
- A `Run` loop that holds its cache mutex across `sync`, while `Desired` takes that mutex, is an ABBA deadlock:
  Run takes cache then txn, Apply takes txn then cache.

**Fix:**
- Minimum: state both rules in the `SyncFunc`/`DynamicSource` doc.
- Preferred: make `sync` a non-blocking, coalescing trigger (like `RequestResync`), served by one agent goroutine per
  source, with the result reported through events and metrics. That removes the whole class.

### R7 LOW: sync events and contract text
`service.go:690`
- A source sync emits `RECONCILE_START`/`RECONCILE_DONE` with an empty `txn_id`. It does so on every call, even when the
  plan is empty, and each call also writes an INFO log and a `reconcile_total` sample.
- `docs/contracts/proto.md:269-272` lists only Apply, resync and revert as sources of reconcile events. Consumers
  (`apps/api/src/telemetry/relay.service.ts:22`) cannot tell a sync from a resync except by `attributes.source`,
  which the contract does not mention.

**Fix:**
- Emit no events or metrics for an empty APPLIED plan.
- Manager (contract doc): one sentence in §StreamEvents saying that `attributes.source` marks a dynamic-source sync.

### R8 LOW: no storm guard on `RequestResync`
`agent.go:239-244`
- A descriptor that calls `RequestResync()` during every resync makes `watchVPP` resync back to back, forever.

**Fix:** after a requested resync, ignore requests for a minimum interval (e.g. 5 s), or log a WARN when a requested
resync requests another.

### R9 LOW: ARCH-1 table (10 rows spot-checked; 7 exact, 3 imperfect)
Exact:
- `interfaces` E2E list = `subsystems.Domains[Interfaces]` plus the registrations in `subsystems.go:202-214`.
- `vrfs` → `vrf`.
- `routing` covers `static[]` only, and the rest gets `agent.unsupported-field` (`projection.go:292`).
- `frr.StaticOwnedByFRR` is the agent's only renderer use (`projection.go:243`).
- `interface.rx-placement` is registered but in no domain (`subsystems.go:211`).
- UI `BUILT_DOMAINS` = interfaces only; `vpn` and `services` are page shells (`router.tsx:44`).
- `agent.unimplemented-domain` (`projection.go:236`). All 61 descriptor packages are listed (checked by script).

Imperfect:
- (a) The `dhcp` row leaves out `dhcp.dhcp6-client`, `dhcp.dhcp6-pd-client`, `dhcp.dhcp6-pd-address` and
  `dhcp.dhcp6-duid` (`descriptors/dhcp/register.go:28-31`). All four are package-only.
- (b) `mpls-route.ldp` does not exist. `mpls.RouteDescriptor`'s name and key prefix are fixed (`mpls.go:40,182,455`).
  F-mpls-ldp must add an instance-name option, so mark it "to be added by F-mpls-ldp".
- (c) Renderers: `rfkit` is missing. It is the shared kit, like `dfkit`.

### R10 LOW (docs): the S1 contract needs two more rules
`seams.go:160-186`
- (a) A source's descriptor must `Retrieve` only the objects it owns (df7 claims or an owner table, as D-072 does for
  routes), and must be disjoint from any config descriptor over the same VPP table. Otherwise each deletes the other's
  objects as "not desired" in scope.
- (b) `AddDynamicSource` after `Start` is silently ignored, because `NewService` copies the sources. Freeze the registry
  after `Start` and return an error.

### Note (pre-existing, next to TD-8's split)
`metrics.go:151-172`: `writeAgent` holds `m.mu` while it writes to the HTTP response. If the client stalls and the buffer
fills (ifsanitize per-interface lines), `metrics.observe` blocks inside `applyLocked` under the txn lock. Fix: render into
a `bytes.Buffer` under `m.mu` and write after unlocking. Not TD-8's bug. It is a one-line fix now that the function is
split.

## Architecture answers (the brief's questions)

- **Declarative, desired state authoritative:** yes.
  - The stored document is never handed out: a clone per `Desired` call (`service.go:630`), and `mergeDomains` clones.
  - `syncSource` never saves state, and Retrieve returns config only.
  - Dynamic keys are confined to the source's descriptors (`service.go:645-653`). Unregistered or domain descriptors
    are refused (`service.go:595-610`, `seams.go` `AddDynamicSource`).
- **Under the txn lock, journaling and rollback intact:** yes.
  - Apply, resync, revert and sync all run under `s.txn`. Dynamic operations are ordinary scheduler operations in the
    same journal, and probe A shows a full rollback.
  - DryRun calls `Desired` without the lock, over a snapshot `storedDoc` read under `s.mu`. That is documented and
    free of races.
- **No VPP outside descriptors:** yes, by construction. `Desired` gets no client, and all writes go through
  `sched.ApplyWith`.
  - `Run` and `Collect` are feature code. Their doc comments forbid binary-API I/O, which is as far as a seam can go.
- **Can a source's failure corrupt or partially apply a transaction?** It cannot partially apply one. It can fail
  whole config transactions and the restart resync (R2), crash the agent (R3), and flush state on an agent restart (R1).
- **Publish/RequestResync deadlocks and slow subscribers:** clean.
  - `bus.mu` is a leaf, released before `offer`, and the subscriber queue drops the oldest event.
  - `RequestResync` is a non-blocking send on a 1-slot channel. `watchVPP` takes the txn lock only in its own goroutine.
  - `vpp.Conn.States` drops the oldest change and never blocks (`vpp/conn.go:99-110`), so a long requested resync
    cannot wedge the connection manager.
- **Collectors outside locks, bounded time:** outside locks, yes. The time bound is only cooperative and it adds up (R5).
- **Id range, product vs lab:**
  - Lab slots (1–12, `VRX_VPP_TABLE_BASE=N000`) and CI slot 12 are unchanged.
  - `tools/app` gets 13000, which is free: slots are 1–12 (`shared-host-rules.md` §1).
  - The product box needs `VRX_VPP_ID_RANGE=all`. `both set`, a malformed base and a bad `VRX_VPP_ID_RANGE` all refuse
    start-up (`Config.Validate`). That is right.
  - The weak point is R4.
- **Out-of-list edits:** all justified.
  - `subsystems.go` +5 lines: `Env` lives there, and the envelope requires "through Env". They are struct-only lines,
    away from the anchors.
  - `internal/agent/seams_test.go`: a new file, so merge-safe, and required by "unit tests for each seam".
  - The `dialVPP` package var: production still calls `vpp.Dial`. It is a test seam only.

## Q1–Q5 recommendations

- **Q1 (`subsystems.go` hunk):** (a), accept as seam hunks. They are line-local struct fields and do not conflict with
  the A1 anchors.
- **Q2 (new test file, `dialVPP`):** accept both.
  - Optional: replace the mutable package var with an unexported `Config.dial` field, so `startFake` tests can run with
    `t.Parallel()`.
- **Q3 (unset = owns no id, not refusal):** accept the staged form, but only together with R4 (non-nil empty range).
  - Copy §11 into `docs/lab/shared-host-rules.md` now.
  - Put the flip on the board as one gated change that must merge before the first id-allocating family
    (e.g. F-acl, P11 SPD/SA ids, F-rpf-adl-pbr ABF policy ids, F-qos-flat egress-map ids). The change:
    - `tools/app` gets `VRX_VPP_TABLE_BASE=13000`;
    - the `test/topology/interfaces` harness passes `VRX_VPP_TABLE_BASE` through;
    - P10's unit ships `VRX_VPP_ID_RANGE=all` in its EnvironmentFile, with a packaging test;
    - then drop the exception at `agent.go:80`.
- **Q4 (`SlotIDRange` meaning):** accept. It has no callers.
  - Mark it `// Deprecated: families read Wiring.IDRange (Env), never the environment.` so that staticcheck SA1019 flags
    any feature that reaches for it. Or delete it.
- **Q5 (S1 design) for LOG.md:** accept (a), merged into every transaction. It is the only option that deletes a
  dynamic object and its config dependency in one ordered transaction, and the scheduler check at
  `reconciler.go:451-453` makes (b) fail user commits.
  - Record it with these amendments:
    - readiness gate (R1);
    - retry once without the sources when a dynamic key causes the failure (R2);
    - panic containment (R3);
    - sync re-entrancy and lock-order rules, or sync as a trigger (R6);
    - owned, disjoint `Retrieve` for source descriptors (R10a).
  - Do not record "a failing dynamic object rolls back the config transaction" as an accepted cost.
- **F1 (`ci.sh` guard):** already fixed on main by `7edac8c` (D-127). Nothing for TD-8.

## Required before merge
R1, R2, R3, R4, each with a unit test (probes A, B, C and E are the scenarios). R5 is required before the first metrics
collector merges. R6–R10 are docs or follow-ups, and fixing them in the same round is welcome.

**APPROVE WITH CHANGES**
