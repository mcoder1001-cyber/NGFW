# TD-8b review: per-key quarantine, refuse start-up without an id range, V2–V5, L7

**Reviewed:** branch `task/TD-8b` @ `699156dc` (code `34448df8`), base `main@d3a7c266`, 2026-09-25.
**Reviewer:** the TD-8 reviewer (`d8891da`, `d74bd49`).
**How:** unit tests, and throw-away probes run with `go test -overlay` or in `git archive` copies in the scratchpad.
Merges were simulated with `git merge-tree`. No host runs, no `tools/ci.sh`, nothing touched outside this file.

**Verdict: APPROVE with three merge conditions (M1–M3) for whichever of TD-8b and TD-9 merges second.**
- Every item in scope is done, and the agent stays declarative.
- My V1 probes G and H fail on the base and pass here.
- One MEDIUM follow-up (C1, rollback churn of the reruns) must land before the first S1 feature (F-mpls-ldp,
  F-igmp-mfib). It does not block this merge, because the seam is inert until a source is registered.

## Evidence

```
$ cd apps/agent && env -u VRX_INTEGRATION go test -race -count=1 ./internal/agent/... ./internal/subsystems/... ./internal/descriptors/df7/...
ok  	ngfw/agent/internal/agent	14.716s
ok  	ngfw/agent/internal/subsystems	6.712s
ok  	ngfw/agent/internal/descriptors/df7	1.170s
ok  	ngfw/agent/internal/descriptors/df7/registry	1.129s
```

**My probes**
- All of them, the TD-8 verify's A–I and the new G/H/K, ran on both trees: `git archive d3a7c266` and `-race -overlay`
  on `699156dc`.

| probe | base d3a7c266 | TD-8b |
|---|---|---|
| G: VPP restart, one rejected and one creatable dynamic object | FAIL: resync leaves dynamic `""`; the sync ends `ROLLED_BACK`, dynamic `""` | PASS: resync APPLIED with dynamic `loop701`; the sync keeps `loop701` and returns `ErrQuarantined` naming `loop702` |
| H: commit deletes loop702 and adds loop703, whose dynamic object VPP rejects | FAIL: `FAILED` "cannot delete: rv.dyn/loop702 … depends on it" | PASS: APPLIED; loop702 gone from config and dynamic; loop703 is config only |
| A–E (TD-8 probes) | — | PASS, no regression |
| I: out of sync, commit deletes a dependency | — | FAILED "cannot delete" (the documented exception) |
| K: 4 rejected objects in one resync after a VPP restart | 2 runs; loop706 sw_if_index 18 | APPLIED after 5 runs; loop706 sw_if_index **36** (C1) |

**Tests and code**
- The author's pre-fix run (TD-8b.md §1–§5, base plus a shim) matches what I see. TD-8's tests in `seams_test.go` were
  changed only where V1 reverses the old all-or-nothing behaviour: in sync, the key is quarantined. No assertion was
  weakened.
- Probe A's second half now goes through the key retry.

## Per item

**V1 is correct and declarative.** Code: `runQuarantining` (`dynsource.go:645-682`), `keyCulprit` (`:592-598`),
`held` (`:159-177`).
- What is held:
  - a rejected Create is left out (hold nil);
  - an Update or Delete keeps the plan's `Old`;
  - a dependent without an op of its own is left out.
- Scope and reruns:
  - The source's descriptors stay in scope, so the scheduler leaves a held object exactly as it is.
  - `again` stops a key from settling twice.
  - `reruns < maxKeyReruns (3)` bounds the loop: a transaction is at most 4 merged runs plus 1 config-only run.
  - There is no rerun after a cancelled or expired ctx, or after DEGRADED.
- Retry and backoff:
  - Each key has its own backoff: `retryMin` doubling to `retryMax`. A failed release, or a retry while VPP is down,
    doubles it (`settleReleasedLocked` :264, `postponeDueLocked` :288), so the key-retry timer cannot spin.
  - `keyGen` makes a superseded timer a no-op.
  - At most 3 keys are released per sync, "changed" before "due". Every sync makes progress: a changed key either
    applies or is re-quarantined with its new `want`, after which it is no longer "changed".
- No livelock:
  - `retrySource` runs only when the source is out of sync, and `retryKeys` only when it is in sync. The two retries
    cannot race each other.
- Lock order:
  - `qmu` is a leaf. It is taken under txn (Apply, sync, the timers) or alone (DryRun's `held`), and never held across
    another lock.
  - The timers take only txn, like the revert timer.
  - `-race` is clean.
- TD-11c:
  - `git merge-tree HEAD task/TD-11c` is clean (tree `b190e5ee`).
  - In that tree, `runQuarantining` sits between `claimsBatch()` and `flushClaims()` in `syncLocked` (merged
    `dynsource.go:866-882`), and `applySources` sits inside `applyLocked`'s bracket (merged `service.go:376-406`).
    Addendum (b) holds.

**V3:** done. The `SyncFunc` doc (`seams.go`) and README rule 2 now include "or from any goroutine they start".

**V4:** done.
- `errCulprits` (`dynsource.go:555-586`) blames a source only when every key the verify error names belongs to one
  merged source.
- The parse splits on `"; "`. `DiffSummary` joins fields with `", "` (`scheduler/reconciler.go` `DiffSummary`), so a
  field list cannot split a key.
- Tested by `TestDynamicSourceVerifyFailureBlamesTheSourceOnlyForItsOwnKeys`.

**L7:** done for the sync.
- `context.WithTimeout(parent, sourceSyncTimeout)` (`dynsource.go:857`) covers the reruns. A sync cut by its own
  deadline is retried; a caller that went away is not.
- The `AfterResync` half is left for TD-9's line (Q3). Agreed.

**Q3 flip:** correct.
- `ConfigFromEnv` keeps `ErrNoIDRange` (`agent.go:81`), and `Validate` refuses start-up.
- The message cites §12 (`seams.go` `ErrNoIDRange`), pinned by `TestConfigFromEnvIDRange`.
- The `Start` warning (`agent.go:167`) is now reachable only for a code-built `Config`. It is harmless.
- Every other launcher inherits `VRX_VPP_TABLE_BASE` from `tools/lab env` or from `ci.sh slot_env` (checked):
  - `devstack.sh`, `live.sh`, `agent.int.test.ts`;
  - the two Go host tests (`os.Environ()`).

**V2:** done.
- `df7.WithIDs(*IDRange)` (`df7/options.go:86`) copies the range: nil means every id, empty means none. Tested.
- The FEATURE-TEMPLATE rule is present, and so is the §12 family rule.

**tools/app:** the product agent still starts. Only the file changed.
- `cmd_up` always stops the agent and starts it again with the new `VRX_VPP_TABLE_BASE=13000` (`tools/app:108-109`);
  `up` is the only path that starts the agent.
- `env VAR=…` overrides a table base inherited from a `tools/lab env` shell.
- The two files `cmd_up` sources with `set -a` do not set `VRX_VPP_ID_RANGE`:
  - `/var/lib/vrx-app/secrets.env` contains no `VRX_VPP_*` key (count 0; I read no values);
  - `pg.env` holds only `VRX_PG_*`.
  So no "both set" refusal.
- The running product agent (pid 2502639, started without the variable) keeps running. The binary and the env line
  land together on main.

**Note for the manager, restarts after the merge:**
- Restart the product agent only with `tools/app up` from main.
- A hand-started `apps/agent/bin/vrx-agent` without `VRX_VPP_TABLE_BASE` now exits with `ErrNoIDRange`. That is
  intended.

**Topology passthrough, §12, P10 note:** done.
- `stack_test.go:66` defaults to the slot's `N000`.
- `interfaces_test.go:161` passes it to the clean agent environment.
- The author's slot-5 run logs `vpp_ids":"5000-5999"`.
- §12 is TD-8's §11 text with "refuses to start".
- The `tech-debt.md` P10 row is there.

**Q2 (`VRX_P08_PG_NAME`): keep it.**
- The default is the prefix, so behaviour is unchanged (`stack_test.go:67`, `interfaces_test.go:150-155`).
- It prevented a real hazard: the harness's final `pg-test.sh drop vrx_w5` would have dropped TD-10b's database on the
  shared slot.
- The name reaches `pg-test.sh` through argv, not a shell, and `pg-test.sh` validates it against `^[a-z][a-z0-9_]{0,15}$`.
- Follow-up: one line in `shared-host-rules.md` §1 or the harness README saying that a slot shared by an API/DB task
  and a VPP-side run sets it. A generic name, e.g. `VRX_TOPO_PG_NAME`, would be cleaner, but not worth a round.

## Merge fit with TD-9

**Method.** `task/TD-9` sits on the pre-D-112 W-seed line (merge-base with main `63178d29`). Its own commits are
`8a96a9ce..c850cbe4`: TD-8's squash, then TD-9's work on top. The realistic merge is TD-9 rebased onto main, so I
simulated TD-9's own changes with `git merge-tree --merge-base=8a96a9ce`:
- onto main: clean;
- onto TD-8b: 1 conflict;
- onto TD-8b + TD-11c: the same 1 conflict.

I resolved it in a scratch copy and ran the tests there.

- **M1, textual, `agent.go` `ConfigFromEnv`.** TD-8b drops the `ErrNoIDRange` exception; TD-9 adds the reply-timeout
  lines under it. Resolution: TD-8b's `ids, idsErr := subsystems.ResolveIDScope()` line, followed by TD-9's `reply,
  replyErr := parseTimeout(…)` block. Then `go build` and `go vet` are clean; `errors` is still used.
- **M2, semantic, not seen by git: a duplicate test helper.** TD-8b adds `func eventually(t, ok, why)`
  (`seams_test.go:964`), and TD-9 adds `func eventually(t, d, what, cond)` (`td9_test.go:192`). The merged package
  does not compile (`eventually redeclared in this block`).
  - Fix now in TD-8b, or on the second rebase: rename TD-8b's helper (e.g. `eventuallySrc`, 3 uses).
- **M3, semantic, depends on the environment: TD-9's config tests are not hermetic under the flip.** They are
  `TestReplyTimeoutBelowTheHealthCheckWindowRefused` (`td9_fix1_test.go:~168`) and `TestConfigReplyTimeoutAndMetricsOptIn`
  (`td9_helpers_test.go:~55`), which call `ConfigFromEnv().Validate()` without an id range.
  - After the merge they fail in a shell without `VRX_VPP_TABLE_BASE`: 15 `no VPP id range` failures in my scratch run.
  - They pass in a `tools/lab env` shell, so a gate run from a worker shell hides the failure, and the merger's shell
    decides.
  - Fix: `t.Setenv(subsystems.EnvTableBase, "7000")` at the top of both tests (TD-9's files; or TD-8b's rebase if TD-9
    merges first).
- **After M1–M3,** on the scratch tree TD-8b + TD-11c + TD-9:
  `go test -race ./internal/agent/... ./internal/subsystems/... ./internal/descriptors/df7/...` gives `ok` (21.7 s,
  16.8 s, 1.1 s).
  - With TD-9: `syncLocked` should take `s.txnTimeout` instead of the local `sourceSyncTimeout` (Q4 rebase note).
    TD-9's transaction clock (merged `service.go:787/857`) already bounds the config-transaction reruns.

## Findings

### C1 MEDIUM, gate before the first S1 feature: each rerun rolls back and re-creates the whole transaction
`dynsource.go:645-682`, `maxKeyReruns` `:53`
- Every rerun is a full `ApplyWith`. Dynamic descriptors register after the config ones, so a failing dynamic Create
  runs last and rolls back everything created before it.
- For a resync after a VPP restart the plan is the whole configuration. With more than 3 rejected dynamic objects, the
  resync makes 4 full create-and-rollback passes before the config-only run: every interface is created and deleted 4
  extra times.
- Probe K: loop706 ends at sw_if_index 36 on TD-8b, against 18 on the base (6 loopbacks per pass).
- On the real VPP that is interface churn and ifsanitize work on every pass. The placeholder cap is the live issue
  that stopped host runs at 05:05 (TD-25). It also spends TD-9's 5 min transaction clock.
- In modeTxn and modeRevert the churn is limited to the commit's own changes. The resync is the expensive case.
- **Fix (TD-8c or the first S1 feature's prerequisite):** a two-phase resync.
  - Run the config-only transaction first (the sources out of scope, as when they are out of sync).
  - Then run the merged transaction, now nearly all dynamic ops, so each rerun rolls back only dynamic objects.
  - If phase 1 fails with "cannot delete … depends on it", which happens only when stray config objects carry live
    dynamic dependents, fall back to today's merged run.
- Test: in probe K, loop706's sw_if_index stays at 12 (one config pass, the reruns touch only dynamic objects), and G and H stay green.

### L1 LOW: a held Delete resurrects after a VPP restart (documented by the author)
- A resync re-creates the `Old` value of an object whose delete VPP refused. The key retry removes it within 60 s.
- Accept. It is in TD-8b.md "Known edge".

### L2 LOW: the `Start` warning is now reachable only for a code-built `Config`
`agent.go:167`
- Its text, "families … refuse to register", still fits. It could say "(Config built in code)".
- Optional.

## Summary for the manager
- Merge TD-8b.
- Whoever merges second of TD-8b and TD-9 applies M1 (a one-block resolve), M2 (rename one test helper) and M3 (two
  `t.Setenv` lines).
- Keep `VRX_P08_PG_NAME`.
- Board C1 as a gate of F-mpls-ldp and F-igmp-mfib, next to the TD-9 `s.txnTimeout` swap and the `AfterResync`
  deadline (Q3).

**APPROVE**
