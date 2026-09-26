# F-neighbors-ra — review

Reviewer: independent review agent (did not write this code), 2026-09-24. Branch `task/F-neighbors-ra` @ `a69b6f2`.
Diff reviewed: `git diff df67a8e task/F-neighbors-ra` (78 files). df67a8e is the W-seed tip this branch merged (5a6fe26).
**During this review the manager rewrote `task/W-seed` to `a303f0b` (squash on main 73aa976)**, so
`task/W-seed...task/F-neighbors-ra` now shows 150 files, most of them W-seed/P08 content. The feature's own diff is the
df67a8e one (see "Merge note" at the end).

Inputs read: 00-CONTEXT, REVIEW-PROMPT, prompts/features/F-neighbors-ra.md, the envelope, wave-A-hotspots §0–§4,
F-neighbors-ra.md / -questions.md / -contract.md, LOG D-126/D-128, VPP 26.06 source (`/root/vpp/src/vnet/ip-neighbor/`).
No host runs (per instructions); unit gate re-run with main's `tools/ci.sh` copy (D-127), result under "CI".

**Verdict: APPROVE WITH CHANGES** — no blocker. One medium change (M1) before merge; the rest can follow.

## Checklist (REVIEW-PROMPT order)

| # | check | result |
|---|---|---|
| 1 | contract | OK. Every contract path is touched by `contract(schema)` 19935ee / f1fccf2 / 65fbe25 and `contract(proto)` 61412e5 / 744392e, plus `F-neighbors-ra-contract.md`. Only additions. Numbers are the §2 allocation: Interface 15 `ipv6_ra` / 16 `proxy_arp` / 17 `proxy_nd`, Subinterface 13–15 (allowed because the leaves are mirrored), Vrf 4 `proxy_arp_ranges`, RoutingConfig 10 `neighbors`, ActionRequest oneof 4 `arp_flush`, EventKind 10 `EVENT_KIND_NEIGHBOR_CHANGED`. Nothing else was numbered. No other `task/F-*` branch reuses any number or message name (checked). New RPC and messages sit under the anchors, in the `// ----- F-neighbors-ra -----` section. proto.md §11 has three sections. Every new schema leaf is optional and has no default at the leaf level (`ipv6Ra?`, `proxyArp?` boolean, `proxyNd?`, `proxyArpRanges?`, `neighbors?`), so existing interfaces, VRFs and routing keep their parsed shape, and TS `toJSON` omits empty repeated fields. The regenerated api-client in 6e633e6 (feat) is route regen covered by the branch's contract commits. `State_neighbors` → `NeighborsRa_neighbors` is the operationId change that P2 implies; nothing referenced the old id |
| 2 | real verification | OK on the pasted evidence. `TestNeighborsRaHost` runs on the host VPP with prefixed loopbacks and no packets (learned entries are injected with `ip_neighbor_add_del` flags NONE). It asserts on agent Retrieve, `show ip neighbors`, `show ip6 interface`, `show arp proxy`, the `arp-proxy` feature and `/state/drift`. `TestNeighborWatchOnHost` checks `show ip neighbor-watcher` (our pid on our loopback, never ~0). I did not re-run host tests |
| 3 | restart safety | OK: log excerpt pasted (agent stopped, dependents deleted via binapi, `created:6` back 0.25 s after start). No new object type (DF-2 descriptors, all with Retrieve). NRestarts 1 → 1 (the 1 predates the task, Q7) |
| 4 | VPP API provenance | OK: only `binapi/{ip_neighbor,interface,ip6_nd,ip6_dad,arp}`. `binapi/` and `tools/binapi-gen.sh` are untouched |
| 5 | shared host | OK: w9 prefix, table range from `SlotIDRange()` (9000–9999), `VRX_GLOBALS_OWNER=0`, `flock -s` only during the run, NRestarts checked, cleanup pasted. Never ~0: the new code refuses `sw_if_index` 0 and ~0 for dump/subscribe/flush, and never calls `ip_neighbor_flush`. VPP source confirms `ip_neighbor_watch` maps 0 to ~0 (`ip_neighbor_watch.c:109`) and that `ip_neighbor_flush` is `ip_neighbor_del_all`, which deletes statics too (`ip_neighbor_api.c:368`, `ip_neighbor.c:689`). D-128: no `show trace` / `trace add` / `clear trace` anywhere in the branch. D-126: no classify sweep, no 802.1ad, no packets |
| 6 | security | OK: no `exec`/`child_process` in product code (only in the `test/topology` harness, with fixed args). Authz: global `AuthGuard` (GET = readonly, POST `/actions/arp-flush` = operator). Audit: resource `arp-flush/<if\|*>`, before/after, failure rows included (pasted). Body is `strictObject` with a regex-bounded interface name. No secrets |
| 7 | transaction semantics | OK: rollback evidence (Retrieve, then vppctl empty). A validation failure leaves nothing (`TestNeighborsRaValidationFailureRollsBack`). Globals are projected only for the owner, and slot agents get `agent.unsupported-field` warnings |
| 8 | UI honesty | OK: real endpoints, screenshots en+fa (table, flush confirm, flush done, drawer group), no TODO/mock/stub in `apps/web/src/domains/routing/neighbors-ra` |
| 9 | scope creep | none: sub-interface mirroring is allowed by §2, and the `ip_neighbor` edit is the gap fix below |
| 10 | i18n | OK: no hardcoded JSX strings and no physical margin/padding in the feature's files; en/fa parity is tested |
| 11 | CI | see "CI" below |

### Focus items from the manager

- **Mapping onto DF-2, no duplicates.** `registerNeighborsRa` (`subsystems/neighbors_ra.go:55-79`) calls each DF-2
  `Register` exactly once: `ipneighbor`, `ip6nd` (ra-config + ra-prefix) and `arp` (range + interface). Every call gets
  `df2.WithClaims(w.KeyedClaims("acl"))`, which is the persisted store (D-080; §4 checklist line satisfied). No other
  package registers these families, and there is no new descriptor or claim store. `RegisterGlobals` is called only
  when `Env.GlobalsOwner` is set. `RegisterProxyNd` is called only with `VRX_DF2_PROXY_ND=1`, and the UI labels it
  "experimental". The `Domains` lines put one descriptor in one domain each: RA and proxy → interfaces, ranges → vrfs,
  statics/limits/DAD → routing. `NormalizeRaConfig` and `NormalizeRaPrefix` are used as they are (D-104).
- **DF-2 ip-neighbor Retrieve fix.** Correct and complete. `neighbor.go:142-196` dumps
  `ifs.Candidates()` (own tag or untagged, never local0) × {IPv4, IPv6}, ignores a detail row for another index, and
  keeps the old STATIC and ownership filters. It is the only `IPNeighborDump` in descriptor code, and the other new
  callers go through `neighborsra.dump`, which refuses 0 and ~0. `TestRetrieveNeverDumpsAllInterfaces` fails on the
  old code (negative control pasted). The descriptor doc is updated. Cost is 2 dumps per candidate per Retrieve, which
  is fine.
- **Event watcher.** Per interface: one `want_ip_neighbor_events_v2` per nameable index, with ip = 0.0.0.0. VPP's
  `ip_address_reset` wildcard key covers both families. Events arrive at most once per interface per second
  (`Coalescer`, `Tick` 1 s) and are bounded: the per-interface map lives only one tick, and above 16 interfaces one
  aggregate event is sent. The watcher is restart-safe: `Connected` stops the previous watcher (waiting up to 5 s)
  before it starts a new one; the old client's VPP registrations die with that client (`want_ip_neighbor_events_reaper`);
  duplicate watches are deduplicated in VPP (`ip_neighbor_watch.c:120`). Tested with the fake and on the host. With
  `Env.Publish == nil` the watcher is not started at all, so there is no VPP subscription and no events until TD-8
  (fine). Rough edges: L2 and N1.
- **arp-flush action.** Only learned entries are deleted, one by one with `ip_neighbor_add_del is_add=0`; statics
  stay, which is tested against the fake and on the host. Only configured interfaces are flushed when `interface`
  is empty: `configuredInterfaces()` ∩ nameable. **A named interface is not held to that rule (M1).** Foreign tags
  are refused (INVALID_ARGUMENT → API 400 with a pointer). The flush is audited and restricted to operator or above.
- **Out-of-anchor edits.** All five are justified; none changes behaviour outside the feature:
  - `subsystems.go:288-289` (comment + `w.neighborsRaConnected(ctx)` after DF-8's line in `Connected`): W-seed seeded
    no anchor in `Connected`, and the new W-seed a303f0b still has none, even though the envelope asks for exactly this
    line. Option (a) of Q2 is the right one.
  - The import line in `domains/{interfaces,vrfs,routing}.ts` (`:15`, `:4`, `:18`) and in `testing/fake-agent.ts:41`:
    a key line under an anchor cannot compile without its import, and neither W-seed has import anchors. There is no
    cycle, because `ext/neighbors-ra.ts` imports only primitives, ip and ui.
  - `coretest/fakevpp.go:96` `v.installNeighborsRa()`: without it every agent-level test that uses `coretest.New()`
    fails on the new families' dumps. A6 allows new files only and has no hook, so one line is the minimum.
  - `packages/proto/test/desired-state.test.ts:154`: ts-proto fills an absent repeated field with `[]`, so this is a
    test expectation, not contract (D-128b).
  - The manager should seed anchors for `Connected`, `coretest.New()` and the import blocks. F-bonding, F-bridge-l2
    and F-vrf-static-ecmp will add lines at the same spots, which gives adjacent-line conflicts that are trivial
    unions.
- **Q5 recommendation (examples).** Keep the branch as it is. `neighbors-ra-full.json` in `packages/proto/test/fixtures`
  is `RootConfig.parse()`d by `parsed-documents.test.ts`, round-tripped by `desired-state.test.ts` and strictly decoded
  by the Go drift guard, so the schema is covered. Manager, in a follow-up: turn `SIBLING` in
  `packages/schema/src/examples.test.ts:46` into an anchored alternation that also admits the wave-A slugs (or any
  `^(?:invalid-)?[a-z0-9-]+-` file whose slug has a `domains/ext/<slug>.ts`). Features can then add validated
  `examples/<slug>-*.json`; this one can move its document there after merge. Not a merge blocker.
- **Q9 recommendation (fake Action).** The diagnosis matches the observed 504. `fake-agent.ts` answers
  `action` with `call.destroy(err)`; that destroys the writable without the status reaching the client, so the API
  waits for its deadline and returns 504 instead of 501. Fix it in the fake (`call.emit('error', err)` or `call.end()` after setting the
  status). The fix belongs to whoever owns the Action dispatch in the fake: F-vrf-static-ecmp, or the manager in the
  anchor commit. Then add an `arp_flush` handler in `features/neighbors-ra/fake.ts` under the P5 anchor, and tighten
  `test/e2e/neighbors-ra.e2e.test.ts:205` (`expect([501, 504])`) to the real success path. Until then that e2e case also
  costs `VRX_AGENT_TIMEOUT_MS` of wall time per run. Not a merge blocker: the success path is unit-tested with a stub
  client and proven on the host agent.

## Findings (ranked)

### M1 — a named ARP flush reaches untagged interfaces outside this agent's configuration (medium, fix before merge)
`apps/agent/internal/agent/rpc_neighbors_ra.go:90` (`names := []string{req.GetInterface()}`) →
`apps/agent/internal/actions/neighbors-ra/neighbors.go:336-344` (`t.IndexByName(n)`, which resolves any **untagged**
interface by its VPP name).
*Scenario:* on the shared host an operator of slot 9 calls `POST /api/v1/actions/arp-flush {"interface":"<untagged
VPP interface used by another workload>"}`. The agent deletes that interface's learned ARP/ND entries, and the other
workload loses its neighbours until it re-resolves them. Q10 gave exactly this reason ("every nameable interface …
on the shared host includes other slots' untagged interfaces") for limiting the unnamed flush to configured
interfaces. The named path skips the rule, although the manager's brief says "configured interfaces only".
*Fix:* when `req.Interface != ""`, require the name to be in `s.configuredInterfaces()`, or to be an interface tagged
by this owner (`t.OwnedID`). Otherwise return INVALID_ARGUMENT ("not an interface of this configuration"). Add a case
to `TestListNeighborsAndArpFlushRPCs` with `v.AddInterface("loop555", "Loopback", "")` (untagged, not configured) →
InvalidArgument. Say it in proto.md §11 and docs/user/routing/neighbors-ra.md. About 10 lines.

### L1 — flush is not serialised with Apply/Resync (low)
`neighbors.go:351-366` dumps, then deletes by (interface, ip). VPP's `ip_neighbor_add_del is_add=0` deletes
whatever entry has that key, static or not (`ip_neighbor_del`).
*Scenario:* a commit adds static neighbour X while X is currently learned. VPP turns the entry static in place. The
flush dumped it as dynamic a moment earlier and now deletes it, so the configured static is missing (Retrieve drift)
until the next resync or commit.
*Fix:* run `arpFlush` under the service's apply lock, the one Apply/Resync hold (a `Service` accessor in the owned
`rpc_neighbors_ra.go`).

### L2 — events are lost for up to 30 s after every VPP (re)connect (low; moot until TD-8)
`agent.go:229-231` calls `Connected` (which starts the watcher) **before** `Resync`, which recreates this owner's
interfaces after a VPP restart. The watcher's first `rescan()` (`subsystems/neighbors_ra.go:197`) therefore finds
none of them, and they are subscribed only at the next `Rescan` (30 s). Interfaces created by a later commit also wait
up to 30 s.
*Fix:* run a short first rescan (for example 1 s and 5 s, then every 30 s), or trigger a rescan from the owned code
after each apply. `Wiring.AfterResync` would need another shared line. The UI's 5 s poll hides the gap in the
meantime.

### L3 — `Nameable` "ours wins" is not what the code does (low)
`neighbors.go:101-106` walks indexes in ascending order and keeps the **first** interface with a given logical name.
An untagged interface whose VPP name equals one of our logical names and has a lower `sw_if_index` therefore shadows
our own tagged interface. ListNeighbors (and its `interface=` filter) then shows the other interface's table under
our name, while Flush (`IndexByName`, own tag first) acts on ours: list and flush disagree.
*Fix:* two passes, owned (`t.OwnedID`) first and untagged second, the same as `iface.Table.IndexByName`. Add a fake
test with a twin.

### L4 — permanent `/state/drift` from list order and text form (low; same pattern as P08)
`/state/drift` compares arrays as leaves (`packages/schema/src/diff.ts`). The assembler reorders and canonicalises:
`routing.neighbors.static` sorted by (interface, address) (`desired/neighbors_ra.go:438-449`), `proxyArpRanges` by
(table, low, high), and `proxyNd` by address. MACs come back lower-case with colons and IPv6 compressed. The schema
accepts `AA-BB-…` MACs, non-canonical IPv6 text and any order, so a valid running document can show drift forever.
*Fix (either one):* canonicalise in the schema (a MAC transform to lower-case colon form, canonical IPs) and document
the order; or make the assembler follow the stored document's order, as it already does for "off" leaves. Do the same
for P08's address lists (cross-cutting, manager).

### L5 — use main's `safeText` for free-text query parameters (low, on the rebase)
`apps/api/src/features/neighbors-ra/neighbors-ra.controller.ts:21,25-30` (`vrf`, `search`: `z.string().max(64)`).
Since TD-2 (7082cc6, not in this branch's base), main validates the same kind of parameter with `safeText(64)`
(`/state/routes`). Adopt it when the branch moves onto main.

### N1 — nits
- `subsystems/neighbors_ra.go:192` says "gone interfaces took their watcher with them". VPP has no interface-delete
  cleanup for watchers: only unwatch or the client reaper removes them (`ip_neighbor_watch.c:66-93`), and an unwatch
  after deletion fails VALIDATE_SW_IF_INDEX. A reused index then sends another owner's events to us; the Go filter
  drops them correctly. Fix the comment.
- `desired/neighbors_ra.go:55-58`: the projection options are process-global (`atomic.Pointer`), set by
  `registerNeighborsRa`. Two Wirings in one process overwrite each other, and tests have to reset it
  (`project_neighbors_ra_test.go:233-236`). Pass the options through the Wiring or `project()` when A2 is next touched.
- `neighbors.go:93-123,228-240`: `Nameable` calls `sw_interface_get_table` ×2 for every nameable interface even when
  `interface=` is set. Filter by name first. It is cheap to fix, and the table polls every 5 s per open tab.
- Proto message names `Ipv6Ra`, `ProxyArpRange`, `StaticNeighbor`, `ArpFlushAction` do not carry the `Neighbor*`
  prefix of hotspots rule 5. No collision exists on any wave-A branch today; keep them, but check at each merge.
- `neighborWatches` (`subsystems/neighbors_ra.go:88`) is never pruned (tests only).

## CI
Run by the reviewer at 22:51–23:14 in this worktree, using **main's** `tools/ci.sh` (73aa976, D-127) copied to a
private path: `TMPDIR=/tmp/g-rv9 /tmp/g-rv9/ci-main-nra.sh --base main`. Step logs are in
`/root/ngfw-wt/logs/ci/F-neighbors-ra-20260924-225124-3379768/`.
```
branch    task/F-neighbors-ra @ a69b6f2   (base: main)
== contract guard: HEAD vs main ==            ok — contract commit(s) on the branch (19935ee 61412e5 f1fccf2 744392e 65fbe25 + W-seed/P08's)
== generate + generated-output gate ==        clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
== forbidden patterns (+ gitleaks) ==         ok (no shell/VPP/FFI in api/web/packages, no kill-by-pattern, no secrets; gitleaks no leaks)
== lint · typecheck · unit tests · build ==   Tasks: 30 successful, 30 total
== apps/agent: make lint test build ==        golangci-lint 0 issues; go test -race: 91 ok, 0 FAIL (actions/neighbors-ra, agent, desired,
                                              subsystems, descriptors/ip_neighbor all ok)
== apps/cli: make lint test build ==          ok (7 packages)
== test/ Go modules, unit mode ==             test/topology/neighbors-ra: gofmt ok · go vet ok · ok
== deploy/vpp: shellcheck + apply-startup fake-host harness ==
  shards 101/0, 99/2, 99/2, 99/2 (scenario 24) → serial rerun died in scenario 26 ("/run-pid: No such file or directory")
CI GATE FAILED — apply-startup fake-host harness failed (398 passed …)
```
**Assessment: green for this feature.** The only failing step is `do_deploy_vpp`, which main added after the branch's
base. The branch does not touch `deploy/vpp` (`git diff df67a8e task/F-neighbors-ra -- deploy/vpp` is empty), and its
old-base `deploy/vpp/test-apply-startup.sh` has no `VRX_TEST_SHARD`/`VRX_TEST_ONLY` support (0 occurrences; main's
copy has 5). Main's step therefore ran the **whole** harness 4× in parallel: scenario 24 has second-scale timeouts and
failed under that load. The "serial rerun" then ran everything again, ignoring `VRX_TEST_ONLY`, and died in scenario 26.
This goes away when the feature diff is placed on the new base (merge note). Every step the worker pasted matches this
run: 30/30 turbo, golangci 0 issues, 91 ok / 0 FAIL, cli 7 packages, the topology module. The worker's pasted run
used the branch's `ci.sh` with the D-127 hunk, which has no deploy/vpp step (Q8). The manager re-runs CI on the
squashed result anyway.

## Merge note (for the manager)
- The branch history contains the **old** W-seed commits (base 8b7558e, merge of df67a8e). `task/W-seed` is now the
  squash `a303f0b` on main. A read-only dry run of a plain merge onto a303f0b conflicts in about 25 files, all of them
  duplicated anchor hunks. Replaying only the feature diff onto a303f0b
  (`git merge-tree --merge-base=df67a8e a303f0b task/F-neighbors-ra`, also read-only) conflicts **only** in the two
  generated files (`packages/api-client/src/generated/schema.d.ts`, `apps/cli/internal/api/operations_gen.go`), which
  are regenerated under rule 3. A D-112 squash of `git diff df67a8e task/F-neighbors-ra` onto the new base is
  therefore clean, followed by `pnpm gen && make -C apps/cli gen docs` and a CI rerun on the result.
- Merge-order constraint (§4.5 / D-125): wave-A merges wait for TD-11a.
- Seed anchors for `Wiring.Connected`, `coretest.New()` and the import lines of the C1 domain files and `fake-agent.ts`
  before the next wave-A merges.
