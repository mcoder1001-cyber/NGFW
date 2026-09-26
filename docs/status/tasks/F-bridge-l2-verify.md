# F-bridge-l2 — focused verify of fix round 1

Reviewer: the same independent agent as `F-bridge-l2-review.md` (@ `fc4b6bd`). Checked: `fc4b6bd..51e65f5` (code) and `2597061`
(status). Read-only apart from this file. Only unit and web tests were run; there were no host runs.

## Verdict: **BLOCK** — on D-132 polling only

Every review finding the manager listed is fixed and tested. See the table below.

The block is narrow. It covers the auto-refresh intervals of the Bridging page: two constants in the web code. The fix is under
10 lines and needs only a glance to re-verify. If the manager rules that D-132 allows a 10 s poll of the list and a 5 s poll of
the MAC page, this becomes **APPROVE** with no further change.

## Findings verified

| review finding | fix | verified |
|---|---|---|
| #2 member removal turned off the MAC filter | `leaveBridgeL2()` (`bridge-l2/model.ts:105`). A port with `macFilter` gets the patch `{bridgeDomain, shg, bvi, uuFwd, tagRewrite: null}` and keeps its filter; any other port still gets `l2: null`. The decision uses the fresh candidate (`BridgeDomainDrawer.tsx:131`). | ✓ Read the code. `BridgingPage.test.tsx` "removing a member …" asserts both patches: the sub-interface without a filter gets `l2: null`; `host-w7l0` (`macFilter: true`) gets the membership-only patch. Passes. |
| #3 / Q10: saving P08's drawer on a bridged interface | New `bridge-l2/drawer-l2.test.tsx` renders the real App and opens P08's drawer on an interface with `l2 {bridgeDomain, shg 2, macFilter}`. It changes the MTU and saves. | ✓ Passes. The PATCH body is exactly `{"host-w7l0":{"mtu":1400}}` and contains no `l2`. The sub-interface-dialog defect (`saveSub`) is still P08's and was correctly left alone. |
| #4 drift note | `F-bridge-l2.md` "Notes:" now says drift does compare `/routing/l2`. The `changes: []` body is pasted: `/routing` appears in `ignored` as a domain-level note only. | ✓ Correct, and consistent with my `driftOf` probe. |
| #11 Q2 acceptance line | Reworded to "bridge member + L2 cross-connect rx → 400, pointer `/routing/l2/xconnects/host-w7l0`". It notes that "two bridge domains" cannot be expressed with D-109(c). | ✓ |
| #9 Q7 rename = recreate | `docs/user/interfaces/bridge-l2.md`, `bridgeDomains` row: renaming the record or changing its `id` deletes and re-creates the bridge domain; members re-join, learned MACs are lost, and traffic stops briefly. | ✓ |
| #5 streamed MAC counts + 10 s poll | `fibCounts()` (`rpc_bridge_l2.go:84`) counts while it streams and keeps no table. `BRIDGE_POLL_MS` went from 3 s to 10 s (`queries.ts:17`). | ✓ Code read. `TestBridgeL2OnFake` asserts the counts (static 2, learned 0). The agent and web tests pass. **The walk is still there**, see D-132 below. |
| #6 learned `mac-*` entries replaced only by the globals owner | `DeviceDescriptor.adopt` / `WithGlobalsOwner` (`device.go`). Registration: `mactime.Register(…, w.env.GlobalsOwner)`; `GlobalsOwner` comes from `VRX_GLOBALS_OWNER` through `agent.go:74` → `Env`. | ✓ `TestDevice`: a non-owner gets `ErrNotOurs`; the globals owner replaces the entry. `mactime.md` is updated. |
| #8 cross-connect removal drops the rx tag rewrite | `removeCrossConnect()` (`BridgingPage.tsx:292`) plus `dropXconnectRewrite()` (`model.ts:115`). It applies to L2 cross-connects only (not l3xc) and reads the fresh candidate. It removes the whole leaf when nothing else is set, otherwise only `tagRewrite: null`. | ✓ The test "removing an L2 cross-connect …" asserts the routing patch followed by `{"host-w7w0":{"l2":null}}`. Passes. |
| #8 part: BD removal | The worker points out that the remove button was already disabled while members exist (`BridgeDomainDrawer.tsx:232`, `members.length > 0`). | ✓ **My review was wrong on that half.** The stale `isError` read is now a try/catch. The tag-rewrite editor in the cross-connect dialog is openly left for later, which is fine. |
| #10 404 keyed on NOT_FOUND | `bridge-l2.controller.ts:191`: `e.extra['grpcCode'] === 'NOT_FOUND'`. | ✓ `agentProblem`'s default branch sets `grpcCode: GrpcStatus[err.code]`, and `@grpc/grpc-js` `status[5]` is `"NOT_FOUND"` (checked with node). API `tsc --noEmit` exit 0. The e2e "7999/macs → 404" is from the worker's paste and was not re-run here (it needs the slot DB). |
| #11 info | "Manager to confirm" was replaced with "confirmed by D-122". The lost-record mactime enable is documented as a known limit in `mactime.md`. | ✓ |
| #7 scope coupling | Left for later, and says so, with a sound reason: P08 tests apply `interfaces` alone, so a guard needs a rule. | accepted as a follow-up |

### Tests run by the reviewer
```
apps/agent  go test -count=1 ./internal/descriptors/mactime/ ./internal/desired/ ./internal/descriptors/l2/ ./internal/subsystems/  → ok ×4
            go test -count=1 -run TestBridgeL2 ./internal/agent/  → ok · go vet (agent, mactime, subsystems) ok
apps/web    vitest run src/domains/interfaces  → Test Files 4 passed (4) · Tests 18 passed (18)
            (drawer-l2.test.tsx 1, BridgingPage.test.tsx 5 incl. both new cases, InterfacesPage.test.tsx 7, model.test.ts 5)
apps/api    tsc -p tsconfig.json --noEmit  → exit 0
```
The web tests needed the package `dist/` directories, which the worker's cleanup had removed. I rebuilt them with
`turbo run build --filter=@ngfw/web^...`. They are git-ignored and the tree stays clean.

## D-132: is the 10 s list poll acceptable? Does it walk the MAC table?

I could not find D-132's text on main, in `docs/`, or in any worktree. The judgment below uses the manager's summary, "no fast
polling of full VPP walks".

**Yes, it walks the MAC table.** Every refresh of `GET /state/l2/bridge-domains` calls `BridgeDomainState`. That runs one
`l2_fib_table_dump` for every owned bridge domain, only to compute `learnedMacs` and `staticMacs`. On the VPP side,
`l2fib_table_dump(bd_index)` iterates the whole L2 FIB bihash and filters by `bd_index`, so N bridge domains means N walks of
the global table. As far as I know that API handler is not mp-safe, so it runs on the main thread under the worker barrier.
Fix #5 removed the agent-side buffering. It did not remove VPP's walk.

- **List:** polled every 10 s by `useBridgeDomains` (`queries.ts:27`) and also by the domains grid (`BridgingPage.tsx:238`,
  `refetchInterval={BRIDGE_POLL_MS}`, `fetchQuery` with `staleTime: 1000`). The two timers are not synchronised, so there are
  up to 2 refreshes per 10 s per open tab, each doing N full-FIB walks.
- **Open drawer, a faster poll than the list:** the MAC grid in `BridgeDomainDrawer.tsx:333` has `refetchInterval={5_000}`.
  Each refresh calls `BridgeDomainMacs`, which walks the whole FIB and then buffers and sorts it in the agent, just to show one
  25-row page.

**Judgment:** under "no fast polling of full VPP walks", neither the 10 s list poll nor the 5 s MAC-grid poll is acceptable.
Both are periodic full-table walks triggered by a page that is simply left open, and they scale with the size of the FIB, not
with what is displayed.

**Fix (web only, no contract change):**
1. `BridgeDomainDrawer.tsx:333`: `refetchInterval={false}`. The MAC table loads when the drawer opens, on a page change, and on
   a Refresh button.
2. `queries.ts:17`: list auto-refresh ≥ 60 s (or off, with a Refresh button). Drop the grid's own `refetchInterval`
   (`BridgingPage.tsx:238`) so there is a single poller.
3. Optional, later, additive: a `BridgeDomainStateRequest.with_mac_counts` flag, so that the polled list does no FIB walk at
   all.

## For the merger (review #1, not done by instruction)
The branch is still on `df67a8e`. I ran a read-only probe equivalent to `git rebase --onto main df67a8e task/F-bridge-l2`
against current main (`f13b744`, with the wave-B/C anchors). Conflicting files:
- the two generated files → regenerate them (`pnpm gen && make -C apps/cli gen docs`);
- `packages/proto/vrx/v1/dataplane.proto` and `packages/schema/src/domains/routing.ts`: the self-seeded `wave-A: F-bridge-l2`
  anchor plus `l2 = 20` sit at the same spot as the new `wave-BC:` anchors. The fix is a union that keeps both. `RoutingConfig`
  20 does not collide: wave-BC uses 13–17, 18–19 are spare, and D-122 made 21 the next free number;
- `docs/vpp-code-track.md`: V25 (TD-20) and the `V-new (F-bridge-l2)` append land at the same place → union.
