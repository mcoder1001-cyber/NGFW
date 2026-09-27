# TD-8c — two-phase resync for dynamic sources (cloud session, 2026-09-27)

Scope: TD-8b review C1 (MEDIUM, gate before F-mpls-ldp and F-igmp-mfib). Owned files: `internal/agent/dynsource.go`,
`internal/agent/service.go` (resync call), `docs/status/tasks/TD-8c*`.

## Problem (review C1)
Every quarantine rerun of a transaction with dynamic sources was a full `ApplyWith` of the merged plan. The dynamic
descriptors register after the configuration's, so a failing dynamic Create ran last and rolled back everything created
before it. After a VPP restart the resync's plan is the whole configuration: with more than `maxKeyReruns` rejected
dynamic objects the resync made 4 full create-and-rollback passes plus a config-only run — interface churn, ifsanitize
work and TD-9's transaction clock spent on every pass (probe K: loop706 at sw_if_index 36 instead of 18).

## Change
`applySources(…, twoPhase)`; the resync (`modeResync`) passes `true`, commits and reverts keep the merged run (their
churn is limited to the commit's own changes, as the review says).

`applyTwoPhase` (`dynsource.go`):
1. **Phase 1** — the configuration alone, sources out of scope (as when they are out of sync). Write-only
   configuration objects are re-applied here, once.
2. **Phase 2** — the sources alone: their objects, only their descriptors in scope. The configuration that now exists
   satisfies their dependencies (the planner retrieves every descriptor, out-of-scope ones included), so reruns and
   rollbacks touch dynamic objects only. Per-key quarantine (`runQuarantining`) works as before.
   - Settled: the response joins both phases (`joinResults`: results, summary, reapply counts, duration).
   - Not settled (a whole-source culprit, a key failing again, too many keys): the configuration stays applied and the
     answer is phase 1's; the sources are left out as before (R2) — no extra config-only run.
- **Fallback:** when phase 1 fails at the plan stage only because it would delete a configuration object that a live
  dynamic object of a merged source depends on ("cannot delete … depends on it", `blockedByDynamic`), nothing was sent
  and the resync runs today's merged transaction. Any other phase-1 failure is the configuration's own and is answered
  as such.

## Evidence (fake VPP, this session)

Tests fail on the code before the change (two-phase disabled):
```
--- FAIL: TestResyncRejectedDynamicObjectsDoNotChurnTheConfiguration
    dynsource_test.go:327: resync created 30 loopbacks, want 6 (one configuration pass; the reruns touch dynamic objects only)
--- FAIL: TestResyncTwoPhaseQuarantinesInPhaseTwo
    dynsource_test.go:360: resync created 4 loopbacks, want 2
```
and pass with it, together with probes G and H and every TD-8/TD-8b test:
```
$ go test -race -count=1 ./...          # apps/agent: all ok
$ golangci-lint run ./...               # 0 issues
```
- `TestResyncRejectedDynamicObjectsDoNotChurnTheConfiguration` (probe K): 6 interfaces, VPP restart, 4 rejected
  dynamic objects → 6 `create_loopback_instance`, 0 `delete_loopback`, source left out, configuration rebuilt.
- `TestResyncTwoPhaseQuarantinesInPhaseTwo` (probe G through phase 2): loop701 restored, loop702 SKIPPED and
  quarantined, the source stays in sync, the summary joins both phases.
- `TestBlockedByDynamic`: the fallback triggers only for plan-stage deletes blocked by a merged source's object.

Not run: host runs (no dynamic source is registered in the product wiring yet; F-mpls-ldp and F-igmp-mfib are the
first users).

## Noticed, not mine
`internal/vpp/ifsanitize/sanitize.go` is not gofmt-clean on main (one field alignment, from TD-27's commit b56cb8ad);
left to the TD-27/TD-26 session that owns the file.
