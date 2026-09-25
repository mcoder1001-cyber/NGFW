# TD-8b — open questions and hand-offs

## Q1. Files touched outside the envelope's list (each needed by an item in scope)
- `apps/agent/internal/agent/agent.go`: the Q3 flip itself. The TD-8 review says "drop the exception at `agent.go:80`".
  That is 3 lines removed in `ConfigFromEnv` plus the `Config.IDs` doc comment. TD-9 edits the lines around it (its
  `replyErr` hunk keeps the exception as context), so whichever of TD-9/TD-8b merges second rebases. The conflict is
  mechanical: keep TD-9's lines and drop the `if errors.Is(idsErr, subsystems.ErrNoIDRange) { … }` block.
- `apps/agent/internal/agent/seams_test.go`, TD-8's dynamic-source tests:
  - Probes A and B and `TestDynamicSourceLeftOutRejoinsThroughTheRetry` encoded the all-or-nothing behaviour that V1
    replaces, so their expectations change. Details are in TD-8b.md §1.
  - `memDesc` gains a `failDel` knob.
  - `TestConfigFromEnvIDRange` expects the refusal.
- New test files, which are merge-safe:
  - `internal/agent/dynsource_test.go`;
  - `internal/agent/dynsource_seams_test.go`, the quarantine test seams, kept separate so that the pre-fix run can shim
    them;
  - `internal/descriptors/df7/options_test.go`.
- `docs/agent/README.md`: the §S1 headline, the id-range row, and rule 2 (V3). The envelope names "README §S1".

## Q2. `test/topology/interfaces`: one env override beyond the passthrough (manager: keep or revert?)
- The envelope has the run use slot 5 but forbids `vrx_w5`, Valkey 5 and port 3500 (they belong to TD-10b).
- The harness creates **and drops** its database with `deploy/dev/pg-test.sh create|drop <VRX_TEST_PREFIX>`, which is
  `vrx_w5` on slot 5. Its final `drop` would have killed TD-10b's sessions and database. There was no env knob for it.
- I added `VRX_P08_PG_NAME`, which defaults to the prefix, so the default behaviour is unchanged (`stack_test.go` +2
  lines, `interfaces_test.go` 3 lines).
- The run used:
  - `VRX_P08_PG_NAME=w5_td8b` (database `vrx_w5_td8b`, created and dropped by the run);
  - `VRX_HTTP_PORT=3590`;
  - `VRX_VALKEY_DB=15` (empty before the run);
  - `VRX_AGENT_SOCKET=/run/vrx-test/w5/td8b-agent.sock`;
  - `VRX_METRICS_PORT=9159`.
  All five were free when I checked them. The VPP side stayed slot 5's: prefix `w5`, table base 5000.
- If you prefer strict passthrough only, revert that hunk. A shared slot then needs its own database name some other
  way.

## Q3. TD-9 review L7, second half: the `AfterResync` hook has no overall deadline (not done here)
- `agent.go` `fullResync` calls `a.wiring.AfterResync(ctx)` without a deadline. TD-9 rewrites that exact line
  (`a.safely("wiring after-resync hook", …)`) and adds `connectHookTimeout` for the connect hook.
- Doing it here would conflict with TD-9 on the same line.
- Proposal, for TD-9's rebase or TD-22 once TD-9 has merged:
  `actx, cancel := context.WithTimeout(ctx, connectHookTimeout); defer cancel(); a.safely(…, func() { a.wiring.AfterResync(actx) })`.
- The sync half of L7 is done: `sourceSyncTimeout`, tested in `TestDynamicSourceSyncHasADeadline`.

## Q4. Rebase notes
- **TD-9:** `sourceSyncTimeout` (`dynsource.go`) is a local 5 min var equal to TD-9's `DefaultTxnTimeout`. When TD-9
  merges, `syncLocked` should use `s.txnTimeout`. Keep the var, or a test hook, for `TestDynamicSourceSyncHasADeadline`.
- **TD-11c:** `git merge-tree` of `task/TD-8b` with `task/TD-11c` is clean. The merged `syncLocked` has the per-key
  reruns (`runQuarantining`) inside TD-11c's `claimsBatch()` … `flushClaims()` bracket. On the merged tree,
  `go test -race ./internal/agent/ ./internal/subsystems/` passes (TD-8b.md §6).

## Q5. V5 was already fixed on main
TD-8's merge (`6320e21d`) already cites §12 in `ErrNoIDRange`, so there was nothing left to fix. `TestConfigFromEnvIDRange`
now pins it: the refusal must cite `shared-host-rules.md §12` and not §11. On the base that test fails because of the flip,
not because of V5.

## Q6. Small follow-ups (TD-22?)
- `cmd/vrx-agent/main.go`: the environment list in its doc comment lacks `VRX_VPP_TABLE_BASE` / `VRX_VPP_ID_RANGE`,
  which are now mandatory (not my file).
- P10: the packaged unit needs `VRX_VPP_ID_RANGE=all`, with a packaging test. The row is in `docs/tech-debt.md`.
- DryRun has a new WARNING rule, `agent.dynamic-object-quarantined`, next to `agent.dynamic-source-skipped`. Neither is
  mapped in the API or UI (`grep` finds no TS use). It is additive and needs no contract change.
