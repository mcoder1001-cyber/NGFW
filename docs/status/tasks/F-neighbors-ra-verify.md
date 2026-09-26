# F-neighbors-ra — focused verify of fix round 1

Reviewer: the same independent reviewer as `F-neighbors-ra-review.md` (a7c9543), 2026-09-24 23:4x–00:0x. This is a
focused check of my own findings against `task/F-neighbors-ra` @ `8b96698` (code in `c8f8b00` + `1236448`), diff
`a7c9543..8b96698` (15 files). Read-only apart from this file. Tests used the fake agent only: Go fakes, and the API
e2e with host PostgreSQL + Valkey + the in-process fake agent on idle slot 9. No VPP, no `show trace`.

**Verdict: APPROVE**

| finding | status | evidence (this verify) |
|---|---|---|
| **M1** named flush reached unconfigured untagged interfaces | **fixed** | `rpc_neighbors_ra.go:99-113`: a named interface must be in `configuredInterfaces()` or own-tagged (`neighborsra.Owned` → `IndexByName` + `OwnedID`). Anything else gets `InvalidArgument` ("… is not an interface of this configuration …") and the API turns it into a 400 with pointer `/interface` (e2e asserts status, problem+json and pointer). `TestListNeighborsAndArpFlushRPCs`: untagged `loop555` is refused and its entry survives; own-tagged `loop777` (not in the document) is flushed. **Negative control** on a scratch copy of `apps/agent`, with `ok = true` forced before the check: `--- FAIL: TestListNeighborsAndArpFlushRPCs … project_neighbors_ra_test.go:376: unconfigured untagged flush: <nil>`. On the real tree: `ok` |
| **L1** flush not serialised with Apply/Resync | **fixed** | `rpc_neighbors_ra.go:93-96` takes `s.lock(ctx)` / `defer s.unlock()`. That is the `s.txn` channel semaphore Apply (`service.go:252`), confirm-revert (`:452`) and Resync (`:535`) hold, not `s.mu`, so `configuredInterfaces()` (which takes `s.mu`) cannot deadlock under it. The lock is held while lines stream to the API, bounded by the call deadline (fine). No dedicated test; the design is verified by reading and the existing flush tests still pass |
| **L2** events lost ≤ 30 s after (re)connect | **fixed** | `subsystems/neighbors_ra.go`: `Early` rescans at 1 s and 5 s after start by default (one timer re-armed from the pending list), then every 30 s. `TestRunNeighborWatchEarlyRescan` passes. **Negative control** with the `case <-early.C:` rescan removed: `--- FAIL: TestRunNeighborWatchEarlyRescan … timed out waiting for early rescan` |
| **L3** untagged twin shadowed our interface in the lister | **fixed** | `neighbors.go` `Nameable`: owned pass first, untagged second, the same rule as `IndexByName`, so list and flush name the same interface. `only` limits `sw_interface_get_table` to the named interface (the N1 item, 2 calls). `TestNameableOursFirst` passes. **Negative control** with the pass order swapped to untagged-first: `--- FAIL: TestNameableOursFirst … Nameable(lan) = [{Name:lan Index:1 …}], want our tap240 (sw_if_index 2)` |
| **L5** `safeText` for free-text query params | **fixed** | `vrf` and `search` are `safeText(64).min(1)`, and the OpenAPI query schema is `openapi(safeText(64))`. `apps/api/src/common/text.ts` is **byte-identical** to main's: `sha256 c51e4957…c4067ebcd` for both `git show main:apps/api/src/common/text.ts` (main @ a54b493) and the branch file, so the squash adds an identical file and cannot conflict. The controller test (ESC, LF, U+202E, 65 characters refused) is part of the 7 passing tests |
| **Q9** fake Action / e2e | **fixed** | `features/neighbors-ra/fake.ts` answers `arp_flush` for real (lines + `done`, family validation, M1's INVALID_ARGUMENT for unconfigured names) and every other action with `emit('error', UNIMPLEMENTED)`, which reaches the client. The e2e accepts **only** real answers: 200 with an exact body, then `GET /state/neighbors` showing the flushed IPv4 entry gone and IPv6 kept, the 400 `/interface` for `loop555`, flush-all 200, and the three audit rows (success/failure/success with before/after). The case no longer waits for the 5 s deadline |
| **L4** drift from list order / text form | **left as tech debt: agreed** | This is the same pattern as P08's sorted address lists and static routes, and the fix is cross-cutting (canonicalise in the schema, or assemble in stored order), not this feature's. The manager should give it a tech-debt row so it is not lost |
| N1 nits | watcher comment corrected, get_table narrowed; the rest are deferred as listed in the status file | fine |

Runs (this verify):
```
$ go test -race -count=1 ./internal/actions/neighbors-ra/ ./internal/agent/ ./internal/subsystems/ ./internal/descriptors/ip_neighbor/ ./internal/desired/
ok  ngfw/agent/internal/actions/neighbors-ra 1.119s · ok internal/agent 10.626s · ok internal/subsystems 2.478s
ok  internal/descriptors/ip_neighbor 1.158s · ok internal/desired 1.212s
$ npx vitest run src/features/neighbors-ra            ✓ neighbors-ra.controller.test.ts (7 tests)
$ eval "$(tools/lab env 9)"; npx vitest run -c vitest.e2e.config.ts test/e2e/neighbors-ra.e2e.test.ts
create role vrx_w9 · create database vrx_w9 · ✓ test/e2e/neighbors-ra.e2e.test.ts (4 tests) 3434ms · Tests 4 passed (4)
drop database vrx_w9 · drop role vrx_w9
```
The worker's CI claim is corroborated by the step logs in `/root/ngfw-wt/logs/ci/F-neighbors-ra-20260924-232824-533285`:
turbo 30/30, `07-agent.log` 91 ok / 0 issues, and all four apply-startup shards `101 passed, 0 failed`.
Cleanup: the `packages/{schema,proto}/dist` I built for the API tests were removed; `vrx_w9` was dropped by the
harness; the scratch copy for the negative controls lives only under `/tmp/g-rv9`; the worktree is clean apart from
this file.

## Notes for the merge (manager, not blockers)
- **Semantic merge hazard in the fake agent's Action handler.** F-neighbors-ra, F-vrf-static-ecmp
  (`...vrfStaticEcmpFake(this)`), F-nat44-ed-sessions and F-nat44-ei-64-66-nptv6 each **spread an `action`
  handler** under their own P5 anchors. The merge is textually clean, but the **last spread silently wins**. The
  optional-typed `action?` in `neighborsRaFake` also hides TS2783, so typecheck will not flag it. After the second of
  these merges one feature's e2e fails (this branch's now accepts only real arp_flush answers, so it would catch it).
  Fold them into one dispatcher in `fake-agent.ts`, one case per `ActionRequest` member, mirroring the agent's A4
  switch, and drop the per-feature `action` keys.
- Residual (info): the event watcher (`subsystems/neighbors_ra.go` rescan) still maps both twin indexes to our logical
  name. Events from an untagged twin would be counted under our interface. This is harmless (it only triggers a
  refresh) and rare.
- Squash `git diff df67a8e task/F-neighbors-ra` onto the new base and regenerate `schema.d.ts` / `operations_gen.go`
  (review merge note). Seed the `Connected` / `coretest.New()` / import anchors. TD-11a merge order (D-125) still
  applies.
