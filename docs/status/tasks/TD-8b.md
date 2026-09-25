# TD-8b — agent seams follow-up: per-key quarantine, refuse start-up without an id range, V2–V5, L7

Branch `task/TD-8b`, worktree `/root/ngfw-wt/TD-8b`, base `main@d3a7c266`, 2026-09-25 04:10–05:10, slot 5 (VPP side only).
The open questions are in `TD-8b-questions.md`.

| commit | what |
|---|---|
| `26bf0ae0` | V1 per-key quarantine, V3 SyncFunc doc, V4 fair verify blame, L7 sync deadline; tests G, H and others |
| `b5e42a90` | Q3 flip: start-up is refused without an id range (V5 citation pinned), V2 `df7.WithIDs`, `tools/app` env line, topology passthrough, §12, FEATURE-TEMPLATE rule, P10 note |
| `be0a1184` | topology harness: `VRX_P08_PG_NAME` override (Q2 of the questions file), questions file |
| (this commit) | self-review fix: a key retry while VPP is disconnected backs off and re-arms (§1b); TD-8b.md |

**How the pre-fix failures were produced.**
- Base: `git archive d3a7c266 apps/agent` in `/tmp/g-td8b/base`, plus this branch's test files. The code is untouched.
- The new test seams in `dynsource_seams_test.go` are replaced by `/tmp/g-td8b/shim/dynsource_seams_test.go`, the same
  helpers over the base API:
  - nothing is ever quarantined;
  - "retry" is the base's own `retrySource`;
  - `isQuarantinedErr` is always false;
  - `sourceSyncTimeout` is an unused variable.
- Every test in the agent package compiles on the base, so each failure below is behavioural.
- `df7.WithIDs` is a new API, so its two tests fail to build on the base.

## 1. V1 (MEDIUM): per-key quarantine — `26bf0ae0`

**What changed** (`apps/agent/internal/agent/dynsource.go`)
- `runQuarantining` replaces the single fallback rerun. When a transaction fails because of dynamic keys, `culprit`
  returns those keys, each with the value to hold:
  - Create: dropped;
  - Update: the planned op's `Old`;
  - Delete: `Old` kept;
  - a dependent that was re-created around its dependency's change: dropped, because it cannot follow the change.
- `mg.hold` swaps the key's KV and the transaction runs again, at most `maxKeyReruns = 3` times, with the source's
  descriptors still in scope.
- The whole-source fallback (R2) remains for what quarantine cannot settle: an invalid plan issue, a failed Retrieve of a
  source descriptor, a key failing again, or more than 3 keys.
- It is still declarative: only desired KVs change, and no imperative VPP call is made.
- The source stays in sync. `leaveOutLocked` records the key in `ds.quarantine` and reports it as before: a SKIPPED result
  with the source and cause, an ERROR event with source/reason/key, and
  `vrx_agent_dynamic_source_errors_total{reason="rejected"}`.
- Every later transaction desires the held value until a sync applies the source's own:
  - each key has its own backoff (`retryMin` doubling to `retryMax`) and a key-retry timer;
  - a sync also releases at once a key whose source value changed;
  - a failed release doubles the backoff, so the key never stays due.
- A sync that leaves keys quarantined ends APPLIED and returns an error wrapping the new `subsystems.ErrQuarantined`
  (the rest applied).
- `syncLocked`, whose shape TD-11c keeps, now runs the per-key loop inside the same block. See §6 for TD-11c.
- DryRun mirrors it: a WARNING `agent.dynamic-object-quarantined` for a key, and `agent.dynamic-source-skipped` for a
  source.
- Headline fixed in `seams.go` (the `DynamicSource` doc) and in `docs/agent/README.md` §S1: "a rejected dynamic object is
  quarantined on its own", with the one exception stated — a config change cannot delete what a live dynamic object
  depends on while that object cannot go.

**Tests** (`internal/agent/dynsource_test.go`) and their pre-fix failures:
```
--- FAIL: TestDynamicSourceOneRejectedObjectDoesNotHoldBackItsSource          (probe G)
    dynsource_test.go:54: resync: dynamic objects "", want loop701 (the creatable object restored, loop702 quarantined)
    dynsource_test.go:57: resync: loop702 reported … "dynamic source test-sync left out of this transaction (rejected): VPP: label already in use (-1)", source in sync false (want SKIPPED, in sync)
    dynsource_test.go:64: sync: dynamic source test-sync: APPLY_STATUS_ROLLED_BACK: create test.dyn/loop702: VPP: label already in use (-1), dynamic objects "" (want loop701 restored and an error naming loop702)
--- FAIL: TestDynamicSourceRejectedObjectDoesNotBlockAConfigDelete            (probe H)
    dynsource_test.go:79: status APPLY_STATUS_FAILED, want APPLY_STATUS_APPLIED: validation failed: interface.loopback/loop702: cannot delete: test.dyn/loop702 (not managed by this transaction) depends on it …
--- FAIL: TestDynamicSourceMissingDependencyQuarantinesTheKey
    dynsource_test.go:120: dry run issues [agent.dynamic-source-skipped agent.unimplemented-domain] (want the key quarantined, the source not skipped)
    dynsource_test.go:125: apply: orphan … "dynamic source test-sync left out of this transaction (rejected): mandatory dependency interface.loopback/loop709 is neither desired nor present", in sync false, dynamic "loop701"
--- FAIL: TestDynamicSourceRefusedDeleteKeepsTheObject
    dynsource_test.go:139: sync with a refused delete: dynamic source test-sync: APPLY_STATUS_ROLLED_BACK: delete test.dyn/loop702: VPP: label busy, dynamic "loop701,loop702", in sync false
--- FAIL: TestDynamicSourceQuarantineBackoffAndRelease
    dynsource_test.go:182: retry 0: backoff 0s (want 1h0m0s), dynamic "loop701", in sync false
--- FAIL: TestDynamicSourceQuarantineIsBounded
    dynsource_test.go:207: commit: loop706 true, in sync false, quarantined [] (want the config applied, the source left out, 3 keys quarantined)
```
- G: after a VPP restart the creatable object is restored by the resync and by the source's own sync.
- H: the commit ends APPLIED, and its results include both deletions, `test.dyn/loop702` and
  `interface.loopback/loop702`.

**TD-8's tests whose expectations V1 flips** (`seams_test.go`). They encoded the all-or-nothing behaviour. The pre-fix
run shows each old behaviour:
```
--- FAIL: TestDynamicSourceFailureDoesNotFailTheCommit        seams_test.go:725: after the commit: in sync false, quarantined [] (want the source in sync, loop703 quarantined)
--- FAIL: TestDynamicSourceFailureDoesNotRollBackTheResync     seams_test.go:773: dynamic objects "" (want loop701 rebuilt, only loop702 quarantined)
--- FAIL: TestDynamicSourceLeftOutRejoinsThroughTheRetry       seams_test.go:935: after the commit: in sync false, quarantined [] (want in sync, loop703 quarantined)
```
- Probe A's second half now goes through the key retry.
- The rejoin test covers both retries: the key retry, and the whole-source rejoin after invalid output.
- `memDesc` gains a `failDel` knob.
- No other assertion was weakened. Probes C, D and E, and every other dynamic-source test, pass unchanged.

### 1b. Self-review fix (last commit): a key retry while VPP is disconnected
- The gap: on `be0a1184`, a key-retry timer that fired while VPP was disconnected got UNAVAILABLE from `syncLocked` and
  was not re-armed. A resync does not re-arm it either, so the quarantined keys would have waited for the source's next
  Run sync.
- The fix: `retryKeys` now postpones the due keys (`postponeDueLocked`: backoff doubled, so the timer cannot spin) and
  re-arms.
- Also, the method `quarantined()` was renamed `quarantineSummary()`, because it shared its name with the type.
```
$ (task base + shim)   --- FAIL: TestDynamicSourceKeyRetryWhileDisconnected   dynsource_test.go:203: key retry while disconnected: backoff 0s (want 2h), timer armed false
$ (be0a1184's code)    --- FAIL: TestDynamicSourceKeyRetryWhileDisconnected   dynsource_test.go:203: key retry while disconnected: backoff 1h0m0s (want 2h), timer armed false
$ (this commit)        --- PASS
```

## 2. V3 (LOW): SyncFunc doc — `26bf0ae0`
- The `SyncFunc` doc (`seams.go`) now says: never from Desired or from a descriptor call, "or from any goroutine they
  start".
- The agent refuses a call from their own goroutine, but cannot recognise a goroutine they start, which then deadlocks.
  The same sentence is in README rule 2.
- Documentation only; no test.

## 3. V4 (LOW): culprit matching — `26bf0ae0`
- A failed verification blames a source only when every key it names (parsed from the scheduler's
  "actual state differs from desired: …" list) belongs to that one merged source.
- A failed Retrieve of a source descriptor still blames the whole source.
- Plan issues follow the same rule: every issue must be on a key of a merged source.
```
--- FAIL: TestDynamicSourceVerifyFailureBlamesTheSourceOnlyForItsOwnKeys
    dynsource_test.go:235: a config key and a dynamic key differ: source blamed true, want false
```

## 4. TD-9 review L7 (Q7): dynamic-source syncs get a deadline — `26bf0ae0`
- `syncLocked` runs under `context.WithTimeout(parent, sourceSyncTimeout)`, with the reruns included.
  `sourceSyncTimeout` is a local 5 min var, equal to TD-9's `DefaultTxnTimeout`. Rebase note: when TD-9 merges, use
  `s.txnTimeout`.
- A sync cut by its own deadline rolls back and arms the retry; only a caller that went away skips the retry.
- The `AfterResync` half of L7 is in `agent.go` `fullResync`, on the line TD-9 rewrites, so it is not done here
  (questions file Q3).
```
--- FAIL: TestDynamicSourceSyncHasADeadline
    dynsource_test.go:259: sync past its deadline: <nil> after 610.67143ms, dynamic "loop701,loop702" (want it cut after the stalled operation and rolled back)
```

## 5. Q3 flip + V2 + V5 — `b5e42a90`
- `ConfigFromEnv` no longer swallows `ErrNoIDRange`: `Validate` refuses start-up, and the error cites
  `shared-host-rules.md §12`.
  - V5 had already been fixed on main at TD-8's merge. The test now pins it: §12, never §11.
  - A `Config` built in code still owns no id (fail closed), as before.
- `df7.WithIDs(*IDRange)` takes a copy of the range: nil = every id, empty = none.
- `IDRange`'s doc shows `df7.WithIDs(ids.DF7())`.
- `tools/app`: `VRX_VPP_TABLE_BASE=13000` on the agent's env line. Only the file was edited: not run, nothing restarted,
  ports 3000/8080/9101 and `/run/vrx/agent.sock` untouched.
- `test/topology/interfaces` passes `VRX_VPP_TABLE_BASE` to the agent. The default is the slot's `N000`.
- `docs/lab/shared-host-rules.md` §12 is the TD-8.md text, renumbered, with "neither set: the agent refuses to start"
  and the family rule.
- `prompts/FEATURE-TEMPLATE.md`, Scope 2, has the rule: "an id-allocating family takes its range only from
  `w.IDRange()`, never `nil` or a missing option; its test asserts `NoIDs()` owns nothing".
- `docs/tech-debt.md`: a P10 row for the packaged unit (`VRX_VPP_ID_RANGE=all`, plus a packaging test).
- `docs/agent/README.md` id-range row updated.
- Other agent launchers already inherit the variable from `tools/lab env` / `tools/ci.sh slot_env`:
  `apps/api/test/integration/agent.int.test.ts` (`...process.env`), `apps/cli/test/devstack.sh`,
  `test/topology/sdk-terraform-ansible/live.sh`, and the Go integration tests (`os.Environ()`). Only the topology
  harness built a clean environment.
```
--- FAIL: TestConfigFromEnvIDRange
    seams_test.go:118: unset: none (fail closed) <nil> (want no id and start-up refused with ErrNoIDRange citing §12)
# ngfw/agent/internal/descriptors/df7 [ngfw/agent/internal/descriptors/df7.test]
internal/descriptors/df7/options_test.go:8:32: undefined: WithIDs                     (TestWithIDs)
# ngfw/agent/internal/subsystems [ngfw/agent/internal/subsystems.test]
internal/subsystems/seams_test.go:141:44: undefined: df7.WithIDs                     (TestDF7FamilyTakesItsRangeFromTheWiring)
```

## 6. Unit proof, TD-11c compatibility
```
$ cd apps/agent && env -u VRX_INTEGRATION go test -race -count=1 ./internal/agent/... ./internal/subsystems/... ./internal/descriptors/df7/...
ok  	ngfw/agent/internal/agent	14.084s
ok  	ngfw/agent/internal/subsystems	6.749s
ok  	ngfw/agent/internal/descriptors/df7	1.200s
?   	ngfw/agent/internal/descriptors/df7/df7test	[no test files]
ok  	ngfw/agent/internal/descriptors/df7/registry	1.141s
$ go test -race -count=5 -run 'TestDynamicSource|TestConfigFromEnvIDRange|TestStartIDRange|TestDF7Family|TestWithIDs' ./internal/agent/ ./internal/subsystems/ ./internal/descriptors/df7/
ok  	ngfw/agent/internal/agent	15.280s          (host load 25–30; no flake)
ok  	ngfw/agent/internal/subsystems	1.273s
ok  	ngfw/agent/internal/descriptors/df7	1.158s
```
**TD-11c.**
- `git merge-tree --write-tree task/TD-8b task/TD-11c` is clean (tree `257a8526`).
- In the merged `syncLocked`, `runQuarantining` (the per-key reruns) sits between TD-11c's `claimsBatch()` and
  `flushClaims()`, as addendum (b) asks. In `applyLocked`, TD-11c's bracket encloses the whole `applySources` call.
- On the merged tree: `go vet ./internal/agent/` is clean;
  `go test -race ./internal/agent/ ./internal/subsystems/` gives `ok … 13.605s` and `ok … 16.683s`.

## 7. The topology/interfaces run (slot 5, `tools/lab lock shared`, VRX_VPP_TABLE_BASE passed through)
The environment of the run. TD-10b owns `vrx_w5`, Valkey 5 and port 3500, so the API/DB side moved to free resources;
see questions Q2.
```
eval "$(tools/lab env 5)"; VRX_HTTP_PORT=3590 VRX_VALKEY_DB=15 VRX_AGENT_SOCKET=/run/vrx-test/w5/td8b-agent.sock \
VRX_METRICS_PORT=9159 VRX_P08_PG_NAME=w5_td8b TMPDIR=/tmp/g-td8b test/topology/interfaces/run.sh
```
Excerpts from `/root/ngfw-wt/logs/TD-8b-topology-2.log`:
```
before: NRestarts=2 at 04:50:44
    interfaces_test.go:211: systemctl show vpp -p NRestarts (before) = 2
    interfaces_test.go:224: rig up w5 (slot 5, path af_packet)
    interfaces_test.go:234: create role vrx_w5_td8b / create database vrx_w5_td8b (owner vrx_w5_td8b)
    interfaces_test.go:234: started vrx-agent pid 3788935 (log /run/vrx-test/w5/p08/agent.log)
    interfaces_test.go:345: host-w5w0 rx (echo replies in): +5 frames +6260 bytes → ok
    interfaces_test.go:512: agent started at +0s; interfaces back in VPP at +0.24s; ping OK at +1.57s (no config API call)
    interfaces_test.go:533: reconcile after simulated loss: … = 0.293s (agent log timestamps)
    interfaces_test.go:153: pg-test drop w5_td8b: <nil>   (drop database vrx_w5_td8b, drop role vrx_w5_td8b)
    interfaces_test.go:214: systemctl show vpp -p NRestarts (after) = 2
--- PASS: TestInterfacesVerticalSlice (29.80s)
    --- PASS: TestInterfacesVerticalSlice/topology (13.86s)
    --- PASS: TestInterfacesVerticalSlice/restart-safety (4.94s)
    --- PASS: TestInterfacesVerticalSlice/cleanup-through-api (0.40s)
ok  	ngfw/test/topology/interfaces	29.828s
exit=0
after: NRestarts=2 at 04:51:41
```
The agent under test read its range through the passthrough (`/run/vrx-test/w5/p08/agent.log`, both starts):
```
{"time":"2026-09-25T04:51:14.57…","level":"INFO","msg":"subsystems wired","owner":"w5",…,"vpp_ids":"5000-5999"}
{"time":"2026-09-25T04:51:36.80…","level":"INFO","msg":"subsystems wired","owner":"w5",…,"vpp_ids":"5000-5999"}
```
- NRestarts was 2 before and 2 after. The 1 → 2 restart at 04:27 happened before this task's run: the manager's note,
  a dns_plugin crash from another branch.
- No `show trace`, `trace add` or `clear trace`, no classify sweep, no VPP restart. The harness's V19 guard only reads
  and resets its own rig interfaces' bindings.
- An earlier attempt (`TD-8b-topology.log`, 04:48) stopped at `run.sh`'s `pnpm build` of `apps/api`: the fresh
  worktree had no built workspace packages. It stopped before the lab lock, VPP, PostgreSQL or Valkey, and NRestarts
  was 2 → 2. I built the dependencies (`turbo run build --filter=@ngfw/api^...`) and made the one real run above.
- Cleanup:
  - the database was dropped by the harness;
  - the 4 keys the API left in Valkey db 15 under this run's own prefix `vrx:w5:p08:126fd5:` were deleted (db 15 was
    empty before and after);
  - ports 3590 and 9159 were released;
  - the processes were stopped by PID by the harness.

## 8. CI — `TMPDIR=/tmp/g-td8b tools/ci.sh --base main`
The run on `be0a1184` (04:52) passed: `CI GATE PASSED`, wall time 11m18s. After the §1b fix, the final code at
`34448df8` gave:
```
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m03s
  install (pnpm --frozen-lockfile --prefer-offline)   0m00s
  generate + generated-output gate                   1m57s
  forbidden patterns (+ gitleaks)                    0m06s
  packet-trace ban on the shared VPP (D-128)         0m02s
  lint · typecheck · unit tests · build (turbo)   2m08s
  apps/agent: make lint test build                   0m56s
  apps/cli: make lint test build                     0m10s
  test/ Go modules, unit mode (test/integration/smoke test/topology/interfaces)   0m05s
  deploy/vpp: shellcheck + apply-startup fake-host harness   0m14s
  mode quick · wall time 5m42s · logs /root/ngfw-wt/logs/ci/TD-8b-20260925-050529-4025938

CI GATE PASSED
```
- The proof command on `34448df8` (`go test -race -count=1 ./internal/agent/... ./internal/subsystems/...
  ./internal/descriptors/df7/...`): `ok` 15.052s, 6.770s and 1.223s, plus registry 1.143s.
- `git merge-tree` with `task/TD-11c` is still clean at `34448df8` (tree `bc27b422`).
- No host run after 04:51. The manager's hold on host runs (the ifsanitize placeholder cap from about 05:05, TD-25)
  came after the topology run of §7 had finished.
- The last commit changes only this file; the CI and tests above ran on `34448df8`.

## Out of scope / left
- The `AfterResync` deadline (L7's second half): agent.go, which conflicts with TD-9 (questions Q3).
- R10b (freeze after Start) stays LOW, as in TD-8.
- The P10 packaged unit is P10's.
- **Known edge, by design (the verify's rule "Delete: keep Old").** Holds live in process memory and survive a VPP
  reconnect. So a resync re-creates a held value, including an object whose delete VPP refused before the restart. Its
  key retry then converges it within the backoff, 60 s at most. An agent restart clears the quarantine: the source
  starts out of sync, and its first sync finds rejected keys again.

## Decisions (for the LOG)
1. **Per-key quarantine (V1).**
   - The held value is the planned op's `Old` for Update and Delete. For Create, and for a dependent that has no op of
     its own, the key is left out.
   - At most 3 per-key reruns per transaction, then the R2 whole-source fallback.
   - The quarantine is process memory, like `inSync`.
   - Each key backs off on its own, 5 s doubling to 60 s. A sync releases a key at once when the source's value changed.
     The agent's key retry releases due keys, at most 3 per sync.
   - A key that fails again keeps its previous hold.
   - `SyncFunc` returns an error wrapping `ErrQuarantined` when it ended APPLIED with keys held back. No feature uses S1
     yet.
   - New DryRun rule `agent.dynamic-object-quarantined`.
2. **Verify blame (V4).** Only when every key named is one merged source's.
3. **Sync deadline (L7).** 5 min, local until TD-9's `txnTimeout` lands.
4. **Id range (Q3).** `ConfigFromEnv` refuses start-up without a range. A code-built `Config` stays fail-closed.
   `tools/app` takes the reserved 13000–13999. §12 is the host rule.
5. **Topology harness.** `VRX_P08_PG_NAME` (default: the prefix), so that a slot shared between an API/DB task and a
   VPP-side run keeps its database (questions Q2).
