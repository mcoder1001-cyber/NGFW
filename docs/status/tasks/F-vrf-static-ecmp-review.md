# F-vrf-static-ecmp — review

Reviewer: review agent (vrx-bot), 2026-09-24. Branch `task/F-vrf-static-ecmp` @ `89ccb9b`. Diff base: `df67a8e` (the old
W-seed tip this branch merged): 93 files, +10 218 / −757. Main has meanwhile re-cut W-seed as a squash (`a303f0b`/`0ae3559`)
and carries TD-2 (`7082cc6`), so the manager's main merge is not a fast union (see M3).
Read: 00-CONTEXT, REVIEW-PROMPT, the feature prompt, the envelope, wave-A-hotspots (§0–§4), the status, contract, questions
(Q1–Q13) and WIP files, the whole agent/API/schema/proto diff and the web screens. I also checked the VPP 26.06 sources in
`/root/vpp`: `ip_api.c` (the dump handler and which messages are marked thread-safe), `api_shared.c` and `vnet/main.c` (the
barrier), `fib_table.c` (the walk with a source filter), `ping_api.c` and `svs.c`. I measured retained sizes with
`unsafe.Sizeof` on the pinned binapi and ran main's `tools/ci.sh` copy (below). No host runs.

**Verdict: APPROVE WITH CHANGES.** The architecture is sound. Descriptors are declarative and every new object type has
`Retrieve`. The contract is additive, and its numbers match wave-A-hotspots §2 with no collisions. Restart safety,
V15 ordering and rollback have real host evidence, the security posture is clean, and the evidence matches a re-run of the
gate. Fix H1 before merge. It is the same defect class as F-nat44-ed-sessions H1: every FIB-browser read costs a full VPP
table walk under the worker barrier, and the UI polls it every 5 s. Fix M1 and M2 in the same round. All three fixes are
in owned files with no contract change, about 2 h in total. P12's inputs (`ListRoutes`, `/state/routes`, the `viaFrr`
selector) do not change. M3 is a checklist for the manager's merge.

## Findings (ranked)

### H1 — every FIB read is a full-table walk under the worker barrier, polled every 5 s with no concurrency bound (fix before merge)
- **Agent:** `apps/agent/internal/actions/vrf-static-ecmp/fib.go:138-159` runs one complete `ip_route_v2_dump` per family on
  **every** `ListRoutes` call, whatever the page. `apps/agent/internal/agent/rpc_vrf_static_ecmp.go:21-48` does not limit concurrent calls.
- **VPP side:** `vl_api_ip_route_v2_dump_t_handler` (`/root/vpp/src/vnet/ip/ip_api.c:269-303`) walks the whole table and
  encodes and sends every entry before it returns. `ip_api_hookup` (`ip_api.c:2140-2164`) marks only route add/del and
  `ip_address_dump` as thread-safe. The dispatcher therefore wraps this handler in `vl_msg_api_barrier_sync()`
  (`vlibapi/api_shared.c:545-565` → `vpp/vnet/main.c:485-489` = `vlib_worker_thread_barrier_sync`). With workers, every
  worker stops for the whole dump. On vrx-a (main core only) the main thread polls no input during the dump either.
  The `src` filter does not help: `fib_table_walk_w_src` (`fib/fib_table.c:1280-1306`) walks every entry and compares the
  best source. Measured by the task: 0.79–1.12 s per page at 100 k routes on the shared host. The prompt's 1 M-route
  target is about 10× that per read.
- **Callers that multiply it (UI polls at `FIB_POLL_MS = 5_000`, `apps/web/src/domains/routing/vrf-static-ecmp/api.ts:19,30`):**
  - `VrfsPage.tsx:31-36,81`: `LiveRoutes` runs one `pageSize=1` query **per VRF row**. That is a full walk of every VRF's
    table every 5 s, only to show a count.
  - `StaticRoutesTab.tsx:69-75`: one `source=API` query per VRF in use, every 5 s. This is still a full walk (above).
  - `FibTab.tsx:150-152`: the open page refreshes every 5 s. `FibTab.tsx:126-131`: the prefix field starts a new query,
    and so a new walk, on every keystroke that matches the regex. There is no debounce.
  - The all-VRF listing, `vrf-static-ecmp.controller.ts:132-165`, costs one full walk per VRF per request. The CLI's
    `show ip route` (`apps/cli/internal/cli/cmd_op.go:323-337`, not owned) pages 500 at a time through the whole table,
    so it costs N/500 full walks. For 100 k routes that is 200 walks of about 1 s each, and it hits the agent's window
    cap at 1 M.
  - Every open browser tab adds its own load. A dump that outlives the API deadline keeps running in VPP while the next
    poll starts another one.
- **Fix** (owned files, no contract change):
  (a) UI: take `refetchInterval` off the FIB tab and add a refresh button next to the `retrievedAt` chip, or poll at
  ≥ 30 s only while the tab is visible. Debounce the prefix filter (apply it on Enter or blur).
  (b) UI: drop the per-row live count on the VRF list, or fetch it once on mount and on a refresh button (≥ 60 s if it
  must poll). The static-routes "installed" status: ≥ 30 s or on demand.
  (c) Agent: bound `ListRoutes` concurrency to one walk at a time per agent, with a semaphore. A second caller waits a
  short time, then gets `UNAVAILABLE` "a FIB read is in progress". Add a fake-client unit test: N concurrent calls give
  at most one in-flight `ip_route_v2_dump`.
  (d) API: the web UI always sends `vrf`. Document that the all-VRF form costs one walk per VRF.
  (e) `docs/vpp-code-track.md` V-new (d): add that the dump is not mp-safe and holds the worker barrier for the whole
  walk. The existing text mentions only the missing cursor.
  (f) Manager/CLI owner (TD): `vrx show ip route` should require `<vrf>` or cap its pages on a large table.

### M1 — the VPP ping API holds the worker barrier while it waits (a stall of up to 5 s per ping with workers)
- `want_ping_finished_events` is handled in `/root/vpp/src/plugins/ping/ping_api.c:44-134`. The handler suspends in
  `vlib_process_wait_for_event_or_clock` (`:113`) for count × interval. The message is not registered thread-safe
  (`ping_plugin_api_hookup`, `:139-148`), so the dispatcher holds `vlib_worker_thread_barrier_sync` across the suspension
  (`api_shared.c:545-565`). The CLI `ping` is `.is_mp_safe = 1` (`ping.c:1576`); the API is not. On a VPP with workers,
  every UI ping would stop all workers for up to 5 s. Echo replies that arrive on worker-polled interfaces could not be
  counted either. vrx-a runs no workers (`docs/lab/host-vrx-a.md:11`, `cpu { }`), so the host run could not show this.
  I derived it from the source and did not measure it.
- V-new (b) describes only the binary-API wait and the under-count.
- **Fix:** (a) In `ValidatePing`/`Ping` (`apps/agent/internal/actions/vrf-static-ecmp/ping.go:94-112`), refuse the API
  ping with `FAILED_PRECONDITION` or `UNAVAILABLE` and a clear message naming the V-item when VPP runs worker threads.
  `show_threads` exists in `apps/agent/binapi/vlib`; treat more than one thread as "has workers". Add a unit test with
  the fake. (b) Add the barrier to V-new (b), and to the fix estimate: mark the handler mp-safe and run the ping from its
  own process. (c) State it in `docs/user/routing/vrf-static-ecmp.md` next to the 5-s limit.

### M2 — the FIB lister's window keeps full binapi routes, up to about 340 MB for one deep page
- `fib.go:26-31` sets `MaxWindow = 1_000_000`, and `fib.go:52-56,148-154` keeps `entry{prefix, ip.IPRouteV2}` for the
  smallest offset + limit routes. Measured on the pinned binapi: `entry` is 88 B and each `fib_types.FibPath` is 252 B.
  One page at offset 999 000 therefore retains about 340 MB with one path per route, and more with ECMP. There is no
  concurrency bound (H1 c), and the UI's "last page" button reaches that offset on a 1 M FIB. The status says the lister
  is bounded by the window, not by the table. That is true, but the window scales with the offset, not with the page.
- `vrf-static-ecmp.controller.ts:19,133`: `page` has no upper bound. The offset `(page-1)·pageSize` becomes a proto
  `uint32`, which ts-proto truncates `>>> 0`. For example, `page=4296&pageSize=1000` wraps to offset 32 704 and returns
  a wrong page instead of a 400.
- **Fix:** keep a slim window entry: the prefix plus a compact path (type, next-hop address, sw_if_index, table, weight,
  preference, flags; about 40 B), converted at page time. Alternatively store the prefix only and re-read the page's
  entries. Lower `MaxWindow` to 100 000, with an `INVALID_ARGUMENT` that says "narrow with prefix/family/source". In the
  API, reject `page·pageSize > 1 000 000` with 400 `/page`. TD, with V-new (d): a keyset cursor (`after` prefix, an
  additive `ListRoutesRequest` field) so that deep pages cost O(limit).

### M3 — main-merge hazards the conflict resolution must not lose (manager checklist)
- **TD-2 input hardening:** TD-2 (`7082cc6`) is on main and not in this branch. Main sets `vrf: safeText(64)` on the
  old `RoutesQuery` (main `apps/api/src/state/state.controller.ts:34-35,331`) and
  `@Param('action', new SafeParamPipe('action', 64))` on `ActionsController.run`. `apps/api/test/e2e/td2.e2e.test.ts:351-355`
  expects 400 for `/actions/ping%1B` and 400 with pointer `/vrf` for `/state/routes?vrf=%E2%81%A6x`. This branch deletes
  the first block (P2) and rewrites the second (P3). If the conflict is resolved by taking the branch's side, the
  hardening disappears without warning. The branch would then answer 404 `unknown action 'ping\x1b'` and echo the control
  character into the problem detail. **Carry it over:** `RoutesQuery.vrf/prefix/source` =
  `safeText(64)` (`apps/api/src/features/vrf-static-ecmp/vrf-static-ecmp.controller.ts:12-34`),
  `PingBody.vrf`/`TracerouteBody.vrf` = `safeText(63)` (`apps/api/src/actions/actions.controller.ts:24-40`), and the
  `SafeParamPipe` on `run(@Param('action' …))` (`:108`). Then run `td2.e2e`.
- **`apps/api/src/testing/fake-agent.ts`:** F-nat44-ed-sessions edits the base `action` stub (`call.destroy` → `emit('error')`),
  and this branch deletes that stub (Q10). That is a modify/delete conflict. Keep the deletion: `vrfStaticEcmpFake`
  already answers every other action UNIMPLEMENTED through `emit('error')`. The Q10 answer below covers the longer-term
  fix.
- `apps/agent/internal/agent/server.go`: the `_` → `stream` rename is identical in F-neighbors-ra, F-nat44-ed-sessions and
  F-nat44-ei-64-66-nptv6, so it merges cleanly.

### L1 — the `source` filter is "best source", not "carries the source"
`fib_table_walk_w_src_cb` (`fib/fib_table.c:1280-1290`) keeps an entry only when `src` is its **best** source. The proto
comment on `ListRoutesRequest.source` (`packages/proto/vrx/v1/dataplane.proto`, the `// ----- F-vrf-static-ecmp -----`
section) and proto.md §11 say "routes that carry this FIB source". A static route shadowed by a better source for the
same prefix is missing from `source=API`, and `StaticRoutesTab` then marks it "not installed". **Fix:** a comment-only
contract edit ("whose best source is") in the proto, proto.md, the API `@describe` and the tab's tooltip. The comment in
`svs/route.go:207-208` says the same wrong thing; the re-check there is harmless.

### L2 — the static-routes status covers only 1 000 API routes per VRF
`StaticRoutesTab.tsx:71-72` reads `page 1, pageSize 1000` per VRF. On a VRF with more static routes, the rest show as
not installed. **Fix:** show "unknown" beyond the first page, or query the rows on screen by `prefix`.

### L3 — the `viaFrr` selector is registered in `Register()`, not at package init
`apps/agent/internal/subsystems/vrf_static_ecmp.go:25-36` runs the `sync.Once` only when `subsystems.Register` runs. A
process or test that projects or renders without it keeps `frr.FlaggedStatic` and ignores `viaFrr`. Examples are the FRR
renderer harness (`renderers/frr/frrtest/harness.go`) and P12's golden tests. **Fix:** add
`func init() { RegisterStaticSelector() }` in the same owned file and keep the Once. No product test registers another
selector in a binary that links `subsystems`, and `frr`'s own `review_fixes_test.go:408` does not import it. **Tell P12:**
never call `frr.RegisterStaticSelector`; the selector is `subsystems.ViaFrr`.

### L4 — the audit row does not say what was pinged
`AuditInterceptor` records `POST /api/v1/actions/:action` and the path, but not the target, count or interval.
**Fix:** in `ActionsController.run`, set `req.audit = { resource: 'actions/ping', after: { target, count, intervalMs } }`.

### L5 — the CLI regressed and its docs are stale (not owned; manager merge commit or CLI TD)
`vrx ping <host>` now gets 400 (`/target` required) instead of 501 (Q9). `docs/user/cli/reference.md:86,96,103,144` still
says "answers 501 until the agent implements actions" and "(connected + static)". Those descriptions come from
`apps/cli/internal/cli/cmd_op.go`, so a regeneration does not fix them. The status C7 row lists `reference.md` as
regenerated, but the branch does not change it. **Fix:** one line in `cmd_op.go` (`{"target": args[0]}`), new
descriptions, then `make -C apps/cli docs`.

### L6 — svs applied-once records are never pruned
`svs/route.go:137-149` removes a record only when a present entry is deleted. A record whose entry disappeared with a VPP
restart, and which is no longer desired, stays in `boot-<owner>.json` forever. It is harmless because the identity no
longer matches. **Fix, optional:** in `Retrieve`, drop records of this descriptor whose identity is not the current one
and whose key is absent from the dump.

### L7 — acceptance "page 1000 in < 1 s" is marginal
The pages at offsets 50 000 and 99 000 took 0.977 s and 1.118 s on the loaded host (status §2). The status records this
honestly, and FAST MODE forbids tuning. No action beyond H1.

## Focus points asked for

| topic | finding |
|---|---|
| declarative descriptors, Retrieve | `svs.table` (reads `ip_table_dump` and matches our names), `svs.interface` (`svs_dump`, logical names) and `svs.route` (existence from `ip_route_v2_dump` with the `svs` source, selected table from the record) all implement Create/Update/Delete/Retrieve/Dependencies. Nothing echoes desired state. The core route change is small: next-hop table on encode, decode, deps and sort. The VRF Retrieve skips `<owner>:svs:*` (a VRF name cannot contain `:`, per the `objectName` regex). |
| svs via `ip_table_add_del` + boot-keyed record, restart-safe? | **Yes.** `svs_route_add`/`svs_enable` only `fib_table_find` the table (`svs.c`), so `ip_table_add_del` is enough, and VPP re-locks it idempotently (`ip_api.c:946-962`). Agent restart on the same VPP: the record matches and Retrieve equals desired. VPP restart: the tables are gone and everything is recreated with a new record. Crash between add and `Put`: the entry reads as `UnknownTable`, which triggers a recreate. A missing identity reads as unknown, which triggers a re-program (fails safe). The store is the persisted `Wiring.boot` (review checklist 3.2 ok). Host evidence: descriptor-level loss → `Created:4 Unchanged:12`; topology agent restart → svs back in 359 ms. The records are kept only in `boot-<owner>.json` (L6). |
| agent-picked svs ids, collisions? | Product range `0xFFFFFF00–0xFFFFFFFE`; slot = top 100 of the slot range (2900–2999 for w2). Declared VRF ids are skipped. No other wave-A branch allocates in that range (grep of all 25 local branches). A foreign or other-owner table at the id fails loudly (`ErrTableConflict`, name check before any send). A new VRF that takes an svs id is safe because the reconciler runs deletes first (`scheduler/reconciler.go:182-183`). A hash collision or a VRF taking an id shifts an interface to another id, which means a recreate and a brief svs disable on that interface. That is acceptable and documented. |
| contract additive + numbered | Additive only (optional fields and a new RPC, nothing renamed). Commits `0435d87 contract(schema)` and `4fe7ae3 contract(proto)` exist, plus `-contract.md`. Vrf 3, StaticRoute 7 and ActionRequest (6 unused) match §2. **NextHop 4 `vrf`:** no collision. Among all 25 local branches, only this one defines a `NextHop` field above 3. Core `RoutePath.next_hop_table = 4` is on this branch only. `VrfSourceSelect`/`ListRoutes*` are unique (F-neighbors-ra has `ListNeighbors*`). The C4 `desired-state.test.ts` `toMatchObject` edit is acceptable (Q6). |
| FIB paging in the agent | Correct: sort, filter, `total` and page are right, and only the page crosses gRPC (35–38 KB per 1 000 routes). The 64 k-reply stream is a sound fix for govpp's 100-reply/100-ms drop (Q8). The VPP cost is H1 and the memory bound is M2. It is the same risk class as F-nat44-ed H1: bounded answers, unbounded VPP work per poll. |
| ping action | Default VRF only; vrf/source/size → `INVALID_ARGUMENT` with a body pointer; count × interval ≤ 5 s; serialised (`TryLock` → 503). No shell and no `cli_inband`. Authz is the operator role (403 for readonly, e2e). Audited by the global interceptor (e2e checks the row, L4). Traceroute → `UNIMPLEMENTED` → 501, and V-new covers (a)–(d). Barrier: M1. |
| `/state/routes` replaced in place | P2 was done as 4 hunks, not 1: the handler, its two consts, `routesOf` and the import. All are listed, all necessary, and the blank separator is kept. `operationId: 'State_routes'` is explicit, so the CLI still binds. The old item fields are kept and new ones added. **Behaviour change for clients:** items are now every FIB entry, not the config-level connected + static view: VPP's 0/0 drop, 224/4, 240/4, host /32s, adjacency and RR entries. `origin` widens from `connected | static` to any source name, and an unknown `vrf` → 404 (it was an empty list). The CLI decodes this fine but prints more rows; its docs are stale (L5). Main's TD-2 hardening must be carried over (M3). |
| viaFrr + selector once | `frr.RegisterStaticSelector(ViaFrr)` runs under a `sync.Once`, and a test calls it twice. P08's skip (`projection.go:243`) and the FRR renderer share `StaticOwnedByFRR`. `viaFrr` routes give an `agent.unsupported-field` warning and nothing is programmed. Registration timing: L3. |
| V15 route-before-table | Deps: `ip.route` → `vrf/<table>` and `vrf/<next-hop table>`; `svs.route`/`svs.interface` → `svs.table`; `svs.route` → `vrf/<selected>`. The rollback result order in the topology run is svs.route, svs.interface, svs.table … ip.route ×3, vrf ×2. The V15 probe (each table id re-created holds only VPP's 5 defaults) is pasted for 2021/2022/2960. The 100 k test cleanup re-dumps until no API route is left before it deletes the table. That fix came from the task's own V15 leak at 18:38 (Q7). `ip_table_flush` is never called (grep). |
| shared hunks | All listed (A1, A2 incl. the allowed in-place next-hop-VRF lines, A4, A6, A7, C1–C7, P1–P5, W1–W3) plus two unowned needed hunks (`config.e2e.test.ts`, `App.test.tsx`). Outside the anchors: one import line each in `vrfs.ts`/`routing.ts` (Q5) and the `action` stub removal (Q10). The C7 row wrongly includes `docs/user/cli/reference.md` (L5). |
| no `show trace` (D-128) | Confirmed: no `show trace`, `trace add`, `cli_inband` or `ip_table_flush` anywhere in the diff. Tests use only `vppctl show …`. Their `exec.Command` calls are fixed argv, and the topology test kills only PIDs it started. |
| security / UI / i18n | No `exec`/`child_process` in product code (the CI forbidden-pattern step passes). The screens call real endpoints (no stub/mock/TODO), screenshots are en + fa/RTL, the en/fa key sets are identical (102/102, every used key present), and there are no physical margin/padding properties. |

## Answers to the questions (Q1–Q13)

- **Q1 (manager): accepted.** NextHop 4 `vrf` does not collide with anything (checked above). **Manager:** move "NextHop 4 `vrf`
  (F-vrf-static-ecmp)" from "Proposed" into the §2 allocation table of `docs/status/wave-A-hotspots.md`. Also record core
  `RoutePath.next_hop_table = 4`, which is agent-internal and append-only by the envelope. ActionRequest 6 stays spare.
- **Q2 (manager): default accepted.** UNIMPLEMENTED + V-new. If P12 funds a linux-cp path, traceroute must be a fixed
  argv in the VRF netns, never a shell (rule 9).
- **Q3: default accepted,** plus M1 (the barrier) in the same V-item.
- **Q4 (manager):** widen `examples.test.ts`'s sibling regex once for all wave-A slugs. Until then the default (a proto
  fixture plus unit tests) is fine.
- **Q5:** accepted. The import lines outside the anchors cause a trivial union conflict at merge.
- **Q6:** accepted. The edit is identical to F-neighbors-ra's, so it merges cleanly.
- **Q7 (manager):** informational. The slot-2 V15 leak (`table 2100 is not empty`, 18:38:42) was real and has been fixed
  (the stream plus a cleanup that re-dumps). D-128 has the crash cause. Add one line to D-126 that slot 2 produced a V15
  leak 2.5 min before the crash.
- **Q8 (manager, TD):** agreed, and it is cross-cutting. On a loaded host, a big-table `Retrieve` in any descriptor (for
  example P05 `RouteDescriptor.dumpTable`) can miss entries and re-create them, which ends in `ErrRouteConflict`. Open a TD
  on `internal/vpp`: a default `WithReplySize` for dump streams, or a dfkit dump helper.
- **Q9 (manager):** L5. Fix the CLI body in the merge commit or as a TD, so that `vrx ping` does not ship as a 400.
- **Q10 (manager):** M3. Resolve the conflict by keeping the removal. Before F-neighbors-ra or F-nat44-ed need fake Action
  behaviour, lift a small dispatch table into `fake-agent.ts` (case → handler, one anchor line per feature).
- **Q11 (manager):** move `tabs.ts` to `domains/routing/tabs.ts` when P12 adds BGP/OSPF tabs. No change needed now.
- **Q12:** this review used main's `ci.sh` (with the D-127 fix), so the flake is moot (below).
- **Q13:** confirmed by grep.

## CI (reviewer run)

`TMPDIR=/tmp/g-w2rv bash <main's tools/ci.sh copy> --base main` in `/root/ngfw-wt/F-vrf-static-ecmp` @ `89ccb9b`, started 23:24
(logs `/tmp/g-w2rv/logs/F-vrf-static-ecmp-20260924-232437-374513`). The output below has the file lists trimmed:

```
== contract guard: HEAD vs main ==
ok — contract commit(s) on the branch:
  4fe7ae3 contract(proto): ListRoutes
  0435d87 contract(schema): vrfs source-select, next-hop vrf, viaFrr
  … (W-seed/P08 contract commits that main has as a squash)
WARN commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(W-seed): verify
== generate + generated-output gate ==
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
== forbidden patterns (+ gitleaks) ==
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~1062112 bytes (1.06 MB) in 3.11s no leaks found
== lint · typecheck · unit tests · build (turbo) ==
Tasks:    30 successful, 30 total Cached:    24 cached, 30 total Time:    2m17.962s
== apps/agent: make lint test build ==
ok  	ngfw/agent/internal/actions/vrf-static-ecmp	9.977s
ok  	ngfw/agent/internal/agent	11.079s
ok  	ngfw/agent/internal/contracttest	3.544s
ok  	ngfw/agent/internal/descriptors/svs	1.379s      (no FAIL/panic in 07-agent.log)
== apps/cli: make lint test build ==
ok  	ngfw/cli/internal/api	1.473s; ok  	ngfw/cli/internal/cli	2.265s; … ok  	ngfw/cli/test/e2e	1.118s
== test/ Go modules, unit mode (… test/topology/vrf-static-ecmp) ==
test/topology/vrf-static-ecmp: gofmt ok · go vet ok · ok  	ngfw/test/topology/vrf-static-ecmp	0.017s
== deploy/vpp: shellcheck + apply-startup fake-host harness ==
shellcheck ok: ./apply-startup.sh ./build.sh ./lib.sh ./test-apply-startup.sh ./verify.sh
```

Every step the branch's own gate has is green, and it matches the output pasted in the status (CI section). The branch
does not touch `deploy/` or `tools/` (`git diff df67a8e...task/F-vrf-static-ecmp -- deploy/ tools/` is empty). Main's
extra `deploy/vpp` step therefore runs the branch's older, pre-shard `test-apply-startup.sh`: each of the four "shards"
runs all 31 scenarios. At 23:59 every shard was at scenario 20 with 0 FAIL lines, and I stopped the run (my own task) to
avoid about 30 more minutes. The status recorded the same thing, and the main merge brings the sharded harness. No
retries. The D-127 flake cannot occur with main's guard.

## Required before merge
1. H1 (a)–(e): UI polling and debounce, agent concurrency bound with a unit test, and the V-new (d) text.
2. M1 (a)–(c): refuse the API ping when VPP has workers, the V-new (b) text and the user doc.
3. M2: a slim window entry or `MaxWindow` 100 k, and the API page bound.
4. Manager at merge: M3 (TD-2 hardening carried to the new controllers, fake-agent conflict), Q1 (§2 row), L5 (CLI).
L1–L4 and L6 can go into the same round or a TD.

**APPROVE WITH CHANGES**
