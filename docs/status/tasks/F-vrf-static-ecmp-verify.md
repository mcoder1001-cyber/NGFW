# F-vrf-static-ecmp — verification of fix round 1 (focused)

Reviewer: review agent (vrx-bot), 2026-09-25. Branch `task/F-vrf-static-ecmp` @ `8e779aad`. The original review is
`4435fcf`. The branch merged main (`266d1dc`, `4431c24` = main `f13b744`); the fixes are in `b7d45081` (agent),
`5f619ae1` (API/web) and `9cdee9c8` (contract). Scope, per `F-vrf-static-ecmp.verify1.md`: my findings, the main merge
and the contract commit. I made no host runs and changed no product code. The mutation tests ran on a `git archive`
copy of `apps/agent` in my scratch directory, not in the worktree.

**Verdict: APPROVE.** Every finding is fixed as asked. The findings that have a behaviour test fail when the fix is
removed. The merge brings nothing outside the feature, and the contract commit changes only comments, descriptions and
the additive 409 response. Four small, non-blocking notes are at the end.

## Findings → verified

| finding | what I checked | result |
|---|---|---|
| **H1 (a)(b) UI polling** | `api.ts:18-51`: `FIB_ON_DEMAND` (`staleTime: Infinity`, no focus/reconnect refetch, `retry: 1`). `useFibQueryDefaults` sets it for every `['state','routes',…]` query. `ServerDataGrid` passes `refetchInterval = false` and no `staleTime` (`packages/ui-kit/src/data-grid/ServerDataGrid.tsx:123,137-141`), so the defaults apply to the grid too. `FibTab.tsx:58-65,142-150`: the prefix filter applies on Enter or blur. `VrfsPage.tsx`: the counts are read once, a refresh button re-reads them, and only a 404 shows as "not in the data plane" (any other error shows "unknown"). `StaticRoutesTab.tsx:75`: 60 s, which meets D-132's "≥ 30 s". `grep refetchInterval` finds only that one interval. | fixed |
| **H1 (c) one walk at a time** | `fib.go` `walkSem` (capacity 1), `acquireWalk` waits `WalkWait` = 3 s, then `ErrFIBBusy` → `UNAVAILABLE` (`rpc_vrf_static_ecmp.go` `actionStatus`). A started walk runs on `WithoutCancel` with a 2-min cap, and the slot is released after the walk. **Mutation** (semaphore removed): `TestListRoutesOneWalkAtATime` fails with `peak in-flight dumps 6 (want 1), dumps 12 (want 12)`, and `TestListRoutesBusyWhileAWalkRuns` hangs until the test timeout (FAIL). Both pass on the branch. | fixed, test proven |
| **H1 (d)(e)** | The OpenAPI texts say to name the VRF and not to poll. V-new (b)/(d) in `docs/vpp-code-track.md` now name the worker barrier; the user doc has the same text (`vrf-static-ecmp.md:58,72-73`). | fixed |
| **M1 ping with workers** | `ping.go:96-103`: `refuseWithWorkers` (`show_threads`: more than one thread → `ErrWorkers` → `FAILED_PRECONDITION`) runs after the ping mutex and **before** the watch and `want_ping_finished_events`. The API maps it to 409 (`agent.client.ts:243-256`, `@Protected(…409…)`). The fake has a `workers` knob. **Mutation** (call removed): `TestPingRefusedWithWorkerThreads` fails, and `TestVrfStaticEcmpRPCs` fails at `project_vrf_static_ecmp_test.go:243` with `ping with workers: <nil>`. Both assert that no ping request reached the model. VPP side: `show_threads` counts `vec_len(vlib_worker_threads)` (`vlibmemory/vlib_api.c:182-212`), which is 1 on a `cpu { }` VPP. | fixed, test proven |
| **M2 window** | `fib.go`: a slim `entry` plus `slimPath`, and `MaxWindow = 100_000`. Measured with `unsafe.Sizeof` on the branch code: **entry 64 B, slimPath 48 B** (it was 88 B + 252 B per path), so about 11 MB for a full window with one path per route. The API's `RoutesQuery.superRefine` rejects `page × pageSize > 100 000` → 400 `/page` before the uint32 conversion. The agent test covers the window edge (`MaxWindow − 999` + 1000 rejected, `MaxWindow − 1000` accepted). | fixed |
| **L1 best source** | Commit `9cdee9c8` changes the `ListRoutesRequest.source` comment in the proto and both generated stubs, plus proto.md, the API description and the status tooltip. | fixed |
| **L2 status beyond 1 000** | `model.ts` `routeStatus(row, installed, partial)` → `unknown` for a VRF whose page was cut. `model.test.ts` adds that case; the old 2-argument function would return `missing`, so the test fails on the old code. | fixed |
| **L3 selector at init** | `subsystems/vrf_static_ecmp.go`: `func init() { RegisterStaticSelector() }`, Once kept. A scratch test that links only `_ "ngfw/agent/internal/subsystems"` and never calls Register got `frr.StaticOwnedByFRR(viaFrr)` = true. Q15 tells P12 not to register a selector. | fixed |
| **L4 audit** | `actions.controller.ts`: `req.audit = {resource: 'actions/ping', after: {target, count, intervalMs, vrf}}` (traceroute too). The feature e2e checks the row. | fixed |
| **M3 TD-2 carry-over** | `SafeParamPipe('action', 64)` on `run` (`actions.controller.ts:108`); `safeText(63)` on ping/traceroute `vrf` (`:26,37`); `safeText` on `/state/routes` `vrf`/`prefix`/`source`. My run of `td2.e2e.test.ts` on slot 2: **14/14 passed**, which covers `/actions/ping%1B` → 400 and `?vrf=%E2%81%A6x` → 400 `/vrf`. `vrf-static-ecmp.e2e.test.ts` run alone: **5/5 passed**. | fixed |
| **L5 / L6 / H1 (f)** | Not owned. Recorded as tech debt in Q14 (CLI `show ip route`/`ping`/descriptions, svs record pruning, keyset cursor). | deferred, as agreed |

## Main merge (28 conflicted files) — nothing smuggled

- `git diff --name-only f13b744 8e779aad` (f13b744 is the main commit that was merged in) lists **94 files**. That is
  exactly the 93-file feature set of the reviewed tip (`df67a8e..89ccb9b`) plus my `-review.md`. No file outside the
  feature differs from main.
- Hotspot hunks against `f13b744` are the same as in the reviewed diff. Examples: `subsystems.go` +4 (3 descriptor names
  + 1 Register call), `fake-agent.ts` (1 import + 1 spread, the base `action` stub removed), `i18n.ts` +5, and
  `state.controller.ts` (the routes block moved out plus main's now-unused `safeText` import; nothing else).
- **Regeneration:** I ran `tools/ci.sh gen-check` in the worktree: `clean: packages/proto/gen apps/agent/gen
  packages/schema/dist packages/api-client/src/generated`, `gen-check PASSED (1m57s)`. `make -C apps/cli gen docs` then
  rewrote `operations_gen.go` and `docs/user/cli/reference.md`, and `git status --porcelain` stayed empty. The generated
  output is byte-identical.
- **Contract commit `9cdee9c8`** (`contract(proto,api-client): …`): in `dataplane.proto` and the two generated stubs it
  changes only the comment of one field. In `schema.d.ts` it adds JSDoc descriptions, the `page` description and a
  **409 response** on `POST /actions/{action}`. That response is additive, not a reshape.

## Tests run for this verification

```
go test -race -count=1 ./internal/actions/vrf-static-ecmp/ ./internal/agent/ ./internal/subsystems/ ./internal/descriptors/svs/ ./internal/descriptors/core/... ./internal/desired/ ./internal/renderers/frr/...
ok  ngfw/agent/internal/actions/vrf-static-ecmp  9.226s
ok  ngfw/agent/internal/agent                    9.851s
ok  ngfw/agent/internal/subsystems               1.297s
ok  ngfw/agent/internal/descriptors/svs          1.275s
ok  ngfw/agent/internal/descriptors/core         1.444s
ok  ngfw/agent/internal/renderers/frr            3.732s
ok  ngfw/agent/internal/renderers/frr/frrtest    1.171s
pnpm --filter @ngfw/web test          →  Test Files 15 passed (15) · Tests 103 passed (103)
API e2e, slot 2: td2.e2e 14/14 · vrf-static-ecmp.e2e 5/5 (each file run on its own)
mutations (scratch copy): walkSem removed → OneWalkAtATime FAIL (peak 6), BusyWhileAWalkRuns FAIL (timeout);
                          refuseWithWorkers removed → TestPingRefusedWithWorkerThreads FAIL, TestVrfStaticEcmpRPCs FAIL
```

Afterwards: no slot-2 database is left (`pg_database like 'vrx_w2%'` returns nothing), no vitest process is running,
and the worktree is clean.

## Notes (non-blocking)

1. **Test pins that are missing (optional):** no web test fails if a `refetchInterval` comes back on a FIB query (H1 a/b).
   No test pins `MaxWindow = 100 000`: the agent cases are relative to the constant, and the API case `page=4296` also
   exceeds 1 M. The L3 init behaviour has no product test; my scratch test above passes. A good follow-up is one
   assertion each: `FIB_ON_DEMAND` has no interval and `FIB_STATUS_POLL_MS ≥ 30 000`; `page=101&pageSize=1000` → 400;
   the init test.
2. **M1 on the real VPP is not yet exercised.** If VPP ever reported more than one thread on a main-core-only VPP, ping
   would be refused everywhere. The source says 1 on vrx-a, and the topology test's ping under `ci.sh full` confirms
   it at merge.
3. **Merge note for the manager (D-112 squash):** main has moved on since `f13b744` (TD-7, TD-8, TD-20). Three feature
   files overlap:
   - `subsystems.go`: TD-8's `Env.IDs` and `Wiring.seams` are hunks separate from the feature's anchor hunks.
   - `docs/contracts/proto.md`: a different section.
   - `docs/vpp-code-track.md`: **V25 (TD-20) and this branch's V-new both append at the end of the file**. That is a
     trivial conflict: keep the V25 row, then the V-new section, and number the new item (V26).

   The squashed commit's subject must start with `contract(` (the guard reads subjects).
4. On slot 2, running `td2.e2e` and `vrf-static-ecmp.e2e` together in **one** vitest invocation failed the second file
   in setup (`drop schema if exists drizzle` → PostgreSQL `28P01` auth failed). Each file passes alone, and the worker's
   5-file run passed. This looks like a harness contention on one slot database, not the feature. I mention it only in
   case it shows up again in CI.

**APPROVE**
