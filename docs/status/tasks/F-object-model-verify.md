# F-object-model — verify of fix round 1

Reviewer: the agent that wrote `F-object-model-review.md` (`47beb50`). 2026-09-24. Focused re-check of my own findings on
`task/F-object-model@c396593` (code `448ded6` + `c4dad58`, main merged at `68d4c5d`). Unit tests only, no host runs.

## Findings

| # | finding | status | evidence (this verify) |
|---|---|---|---|
| F1 | O(N²) per-object whole-store write | **fixed** | `store.go:118-138`: in-place change under the lock, a debounced timer (`FlushDelay` 200 ms), one marshal + atomic write in `Flush` (`:174-198`). `descriptors.go` `Retrieve` flushes first, so the scheduler's verification Retrieve writes a transaction once. `single()` is O(1). My original probe (`go test -overlay`, same scratch test + one final `Flush`) goes from **4 000 objects 2 m 34 s to 0.106 s**, and 1 000 from 9.0 s to 0.038 s. `TestStoreScale5000`: 5 000 in 0.176 s (limit 5 s). The whole scheduler transaction is 13.7 s for 4 000 objects according to the branch docs; the rest is scheduler cost (Q9, a separate row), below the 60 s API deadline |
| F1b | FQDN re-sync on every write | **fixed** | `store.change` calls `onFQDN` only when the entry's FQDN before and after differ (`fqdnOf`). The resolver's `track`/`untrack` are O(1) (`byHost` index), and the resolver state file is written coalesced by the loop. `TestStoreWritesCoalescedAndFQDNOnlyOnChange` passes |
| F2 | corrupt store stops the agent | **fixed** | `OpenStore` (`store.go:55-78`) renames the file to `.corrupt-<unix>`, logs ERROR, counts `vrx_agent_objects_store_corrupt_total` and starts empty. Only an I/O read error still fails. `TestStoreCorruptMovedAside` passes |
| F3 | consumers fall back to the applied store | **fixed** | `docs/agent/objects.md`: new "Project from the request" section. `Snapshot` is out of the stable table. The example tests `in["objects"]`, treats nil as "no objects", and reports `acl.objects-required` for a partial Apply. Out-of-band re-projection goes through a resync of the stored desired state. `runtime.go` `Snapshot` doc: diagnostics and tests only |
| F4 | picker guesses addresses | **fixed** | `model.ts` `pickerKinds` returns `[]` when a field is unclassified. `ObjectPicker.tsx` then renders `plainSchema` (no enum, `widget`/`objectKinds`/`dependsOn` stripped). `localizeSchema` no longer injects empty kinds. `ObjectPicker.test.tsx` runs against the real `domainSchemas.acl` attachments item: `list` is plain text, `target.zone` is a select of zones. Passes |
| F6 | clock-step trust | **fixed** | `sync(initial)` pulls a next refresh more than one interval ahead back to one interval. `dueLocked` treats more than `MaxRefresh` ahead as due. `TestFQDNNextRefreshBoundedAgainstClockSteps` passes |
| F7 | last-good forever | **fixed (D-129)** | Per-family `V4At`/`V6At`. A failing family drops its answers after `maxStale`: default 24 h, `VRX_OBJECTS_FQDN_MAX_STALE_SEC`, clamped to 60 s–30 d, a malformed value is an error. Expiry is logged as WARN, counted, and subscribers are told. Older state files fall back to `LastResolved`. `TestFQDNLastGoodExpiresAfterMaxStale` passes |
| F8 | fake handler can hang | **fixed** | `fake-agent.ts:655` adds `.catch((e) => cb(e as Error))` on the same line under the anchor |
| F9 | where-used duplicates acl references | follow-up (Q10) | The header comment is corrected to D-062. A single reference walker stays a P02b/manager follow-up |
| F5 | `service_test.go` hunks diverge across branches | manager action, unchanged | Branch hunk as before, still identical to F-acl/F-host-acl-nftables. Land one canonical version on main |
| R1 | stale base | **fixed** | main merged (`07301c4`, `68d4c5d`). `git merge-tree main c396593` → 0 conflicts against current main `a54b493` |

## Q4 close seam and the edits outside the anchors: acceptable
- `subsystems/lifecycle.go` is new and generic: `OnClose` / `Close`, reverse order, idempotent, stored beside the
  Wiring so `subsystems.go` is not edited. `registerObjectModel` registers `rt.Close`. `Runtime.Close` stops the loop,
  flushes the store and persists the FQDN state. `TestWiringCloseSeam` and `TestAgentStopClosesObjectsRuntimeAndMetrics`
  pass.
- `agent.go` Stop gets 3 lines (nil-guarded `a.wiring.Close()`) after `a.svc.Close()`. The ordering is right: no Apply
  can run any more, and the flush comes after. This is the single A5 line the review recommended and the manager
  directed (questions Q4). **Accept.**
- `metrics.go` gets 1 import and 1 call (`objects.WriteMetrics(w)`), beside the existing `ifsanitize.WriteMetrics`.
  **Accept for now.** Every feature with counters would add a line at the same spot, and agent core now imports a
  feature package. Recommended manager follow-up: a `subsystems` metrics seam like `OnClose`, or an anchor in `write()`.
- Nit, not blocking: if a `Flush` fails inside `flushLater` while `Close` is running, the 5 s retry timer is re-armed
  after `Close` (`store.go:155-160`). A later in-process `Open` of the same path could then be overwritten by the stale
  store. This happens only after a write error and only in tests that re-open. Fix: a `closed` flag checked in
  `flushLater`.

## Commands run
```
$ go vet ./internal/objects/ ./internal/subsystems/ ./internal/desired/ ./internal/agent/          vet-clean
$ TMPDIR=<short> go test -race -count=1 ./internal/objects/ ./internal/subsystems/ ./internal/desired/ ./internal/agent/
ok ngfw/agent/internal/objects 4.285s · ok ngfw/agent/internal/subsystems 1.229s · ok ngfw/agent/internal/agent 10.432s
--- PASS: TestStoreCorruptMovedAside · TestFQDNLastGoodExpiresAfterMaxStale · TestFQDNNextRefreshBoundedAgainstClockSteps
--- PASS: TestStoreScale5000 (scale_test.go:78: 5 000 objects: 175.591809ms) · TestStoreWritesCoalescedAndFQDNOnlyOnChange
--- PASS: TestWiringCloseSeam · TestAgentStopClosesObjectsRuntimeAndMetrics
review probe (overlay): N=1000 38.177048ms · N=4000 106.138884ms (was 9.0 s / 2m34.5s)
$ npx vitest run src/domains/firewall/object-model   (apps/web)   model 5 · ObjectPicker 2 · ObjectsPage 3 — 10 passed
$ npx vitest run src/features/object-model           (apps/api)   usage 4 passed
```
Package `dist/` outputs built for the TS tests were removed afterwards. I did not re-run `tools/ci.sh` or the topology
suite: the manager reports CI green and NRestarts 1→1.

**APPROVE**. Open items, none blocking this merge:
- manager: F5 (one canonical `service_test.go` version on main)
- manager: the metrics seam
- follow-up rows: Q9 (scheduler cost) and F9/Q10
- nit: the `flushLater` retry after `Close`
