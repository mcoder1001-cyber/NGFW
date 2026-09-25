# TD-23 fix round 1 — focused verify

Verifying `task/TD-23 @ 39e07a0f` against `docs/status/tasks/TD-23-review.md` (`85a7d12e`) only.
Time-boxed 25 min.

## Verdict: APPROVE

- **F1** — fixed and correct. `docs/status/tasks/TD-23.md` now records F-bonding's actual mechanism
  (its own redeclared `var extensions []func(*VPP)` + loop, not a single call line) with tip
  `79a9ee1b`. Spot-checked 3 rows against live branch content: F-bonding (current tip `e30c49fb` —
  `git -C /root/ngfw diff 79a9ee1b e30c49fb -- .../coretest/` is empty, branch only moved by an
  unrelated doc commit, row still accurate), F-rpf-adl-pbr (`v.rpf().AdlInput[...]` matches the real
  `RpfAdlPbrModel.AdlInput` field and `(*VPP).rpf()` accessor), F-bridge-l2 (`v.l2()`'s
  `mtCount`/`reached` maps match the real model exactly). F-nat44-ed-sessions' recorded tip
  (`a0e2b155`) is similarly one commit behind its current tip (`4421baec`) for the same reason
  (another unrelated doc-only commit) — content unaffected.
- **F2** — fixed. All four `client.action(...)` calls in `fake-agent-action.test.ts` now pass
  `{ deadline: Date.now() + 2000 }`.
- **F3** — fixed, with a real test. Dispatcher checks `result instanceof Promise` and attaches
  `.catch()`; new test forces a post-`await` rejection (`await Promise.resolve()` then `throw`) and
  asserts `INTERNAL` with the message preserved.
- **feature_is_enabled seam (F5)** — matches the reviewed proposal closely: `RegisterFeatureIsEnabled`
  is installed once by `install()` (core, before `installExtensions()`), so it never itself claims a
  message name under `installingExt` and never trips `VPP.On`'s collision guard. Falls back to
  `IsEnabled: true` for an unregistered name, matching VPP's cast (per F-rpf-adl-pbr's own V23(a)
  comment). `TestFeatureIsEnabledCompose` genuinely models both real branches, not a strawman: one
  stub keyed like F-rpf-adl-pbr's real `installRpfAdlPbr` (per-`SwIfIndex`, `"adl-input"` only) and one
  stub keyed like F-bridge-l2's real `installBridgeL2` (per-`SwIfIndex`, `ArcName` ignored — verified
  the real branch code for both: `AdlInput map[uint32]int` / `mtCount map[uint32]int` +
  `reached map[uint32]bool` match field-for-field), then asserts no bleed-through in both directions
  (`mactime` at index 3 — ADL's enabled index — reads `false`) plus the unregistered-name default.
  `TD-23.md`'s two new registration lines for F-rpf-adl-pbr/F-bridge-l2 reproduce the real branches'
  actual `AdlInput`/`mtCount`+`reached` logic correctly (checked against
  `git -C /root/ngfw show task/F-rpf-adl-pbr:.../rpf_adl_pbr.go` and
  `task/F-bridge-l2:.../bridge_l2.go`).
- **F4/F6** — documented in both README comments (`fake-agent.ts` near `actionHandlers`, `fakevpp.go`
  near `RegisterExtension`/`RegisterFeatureIsEnabled`): module/package-level global, no reset,
  convention only, never reuse a real feature's name/slug from a test.
- **Scope** — unchanged: `fake-agent.ts`, `fake-agent-action.test.ts`, `fakevpp.go`,
  `extensions_test.go`, `docs/status/tasks/TD-23.md`. No feature handlers, no product code.

## Tests

```
$ cd apps/agent && go test -race -count=1 ./internal/descriptors/core/...
ok  	ngfw/agent/internal/descriptors/core          1.196s
ok  	ngfw/agent/internal/descriptors/core/coretest 1.092s

$ pnpm --filter @ngfw/api test -- testing
 ✓ src/testing/fake-agent-action.test.ts (5 tests) 274ms
 Test Files  11 passed (11)
      Tests  101 passed (101)
```

Both clean. Worker reports `tools/ci.sh --base main` passed after this round; not rerun here (nothing
observed contradicts it). No new findings this pass — clear to merge.
