# F-vrf-static-ecmp — questions and notes for the manager

Written while working; nothing here blocks the task (defaults taken and stated).

## Q1 Contract commits are on the task branch (tell-the-manager item)
`contract(schema): vrfs source-select, next-hop vrf, viaFrr` and `contract(proto): ListRoutes` are on
`task/F-vrf-static-ecmp` (details: `F-vrf-static-ecmp-contract.md`). Numbers: Vrf 3, StaticRoute 7, NextHop 4 (the last
one is only "proposed" in wave-A-hotspots §2 — please add it there). ActionRequest 6 is not used.

## Q2 Traceroute (prompt's open question) — default taken: UNIMPLEMENTED + V-item
VPP has no traceroute API. The agent answers `UNIMPLEMENTED` with a message naming the V-item; `docs/vpp-code-track.md`
gets `### V-new (F-vrf-static-ecmp)` covering traceroute and VRF-aware ping. Option (b) "needs linux-cp (P12)" stays
open: once P12 has a linux-cp host path, traceroute can run as a fixed-argv `traceroute -n` in the VRF's netns.

## Q3 Ping in a non-default VRF / with a source / with a size — default taken: INVALID_ARGUMENT + the same V-item
`want_ping_finished_events{address, repeat, interval}` has no table, source or size (checked in `apps/agent/binapi/ping`
and VPP's `ping_api.c`: `table_id = 0`, `data_len = PING_DEFAULT_DATA_LEN` hard-coded).

## Q4 `packages/schema/examples/` cannot take a `vrf-static-ecmp-*.json` (C4)
`packages/schema/src/examples.test.ts` fails any file that is neither in its group (a) list nor matches the P02b/P02c
sibling regex `^(?:invalid-)?(?:nat|objects|acl|vpn|tunnels|services|ha)-…`. Default taken: the valid fixture lives in
`packages/proto/test/fixtures/vrf-static-ecmp-full.json` (both round-trip corpora read it); the invalid cases are unit
tests in `semantic/vrf-static-ecmp.test.ts`. Every wave-A feature hits this — the manager may want to widen the regex once.

## Q5 One import line per C1/C2 file sits outside the anchor
A key line under the anchor needs its identifier imported at the top of `domains/vrfs.ts` and `domains/routing.ts`
(one `import … from './ext/vrf-static-ecmp.js'` line after the last import). Listed under "Shared hunks"; expect a
trivial union conflict with F-neighbors-ra / F-rpf-adl-pbr, which add their own import at the same place.

## Q6 `packages/proto/test/desired-state.test.ts` one-word edit (not an owned file)
`toEqual` → `toMatchObject` on the `vrfs['customer-a']` assertion: any additive repeated field on `Vrf` breaks the exact
match (ts-proto materialises `[]`). F-neighbors-ra's `proxy_arp_ranges` needs the same; the identical edit merges.

## Q7 VPP crash 18:41:08 (manager incident, D-126) — slot 2's activity around it
Not the classify sweep: this task has no classify/policer code or test (`git grep -n classify` in the owned files: none).
What slot 2 did on the shared VPP between 18:34 and 18:41 (all API, all in table 2100 of the slot range):
- 18:34–18:38 `TestFIBBrowser100kOnHost` (apps/agent/internal/actions/vrf-static-ecmp/fib_integration_test.go): 100 000 API
  drop routes in table 2100, five ListRoutes pages, then its cleanup. **Defect in that first cleanup**: it listed the
  routes with the generated dump (govpp reply buffer 100 / 100 ms), govpp dropped one reply on the loaded host, so it
  deleted 99 999 routes and then table 2100 with one API route left → VPP logged `18:38:42 fib/entry: BUG: ipv4 table 2100
  (index 2) is not empty` (the V15 leak condition).
- ~18:39 I re-created table 2100 via the API, found the leaked `10.2.24.3/32` (src API), deleted it, dumped (5 default
  entries) and deleted the table again; `vppctl ip table add/del 2100` probes before and after (CLI source, slot range).
- Fix (commit "fix(agent): FIB lister reads its dump through a 64k-reply stream…", 18:39:53): the test cleanup re-dumps
  through a 64k-reply stream until no API route is left and never deletes the table while one remains.
Nothing of slot 2 was in VPP at 18:40–18:41. NRestarts was 0 before my runs; the tests now compare against 1.

## Q8 govpp drops dump replies on a loaded host (affects every descriptor's dump, not only this task)
govpp core `sendReply` drops a reply when the stream's reply channel (ReplyChanBufSize = 100) stays full for
ReplyChannelTimeout = 100 ms ("unable to send reply (reciever end not ready in 100ms)"). Measured here: a 100 000-entry
`ip_route_v2_dump` returned 99 987 entries once (load ≈ 25–57). The FIB lister now opens its stream with
`core.WithReplySize(1<<16)` (totals then exact: 100 005 = 100 000 + 5 defaults, repeatedly). The same loss can hit P05's
`RouteDescriptor.dumpTable` / every Retrieve dump of a big table (a missed entry reads as "absent" → re-create →
ErrRouteConflict or a duplicate add). Suggest (manager/TD): a larger reply buffer in `internal/vpp` Conn.NewStream by
default, or a dfkit dump helper with it. Not changed here (internal/vpp is not an owned file).

## Q9 CLI `ping <host>` / `traceroute <host>` send no body
`apps/cli/internal/cli/cmd_op.go` `action()` calls `Actions_run` without a body, so `vrx ping 10.2.2.2` now answers 400
(`/target` required) instead of the old 501. One-line fix for the CLI owner: send `{"target": args[0]}`. The operation
table (`operations_gen.go`, regenerated) already says `Body: true`. Docs name the REST call as the CLI equivalent meanwhile.

## Q10 The fake agent's Action handler
`apps/api/src/testing/fake-agent.ts`: TypeScript refuses a second `action` key next to the spread (TS2783), so the base
`action` stub (UNIMPLEMENTED) was removed and `vrfStaticEcmpFake(this)` provides `action` (ping/traceroute; every other
action still UNIMPLEMENTED) and `listRoutes`, plus one import line. F-neighbors-ra / F-nat44-ed-sessions serve their own
static action routes and call `AgentClient.runAction`; for their fake behaviour they need a case in
`features/vrf-static-ecmp/fake.ts` (or the manager lifts the dispatch into fake-agent.ts once).

## Q11 Routing page tabs live in the feature folder
`domains/routing/vrf-static-ecmp/tabs.ts` (the envelope owns only that folder). P12 will want BGP/OSPF tabs on `/routing`:
the manager may move it to `domains/routing/tabs.ts` (like vpn/services) at merge.

## Q12 CI on this base hits the D-127 contract-guard flake
This branch's `tools/ci.sh` predates D-127 (fixed on main): `git log | grep -q '^contract'` under pipefail fails ~55 % of runs
with "CONTRACT FILES CHANGED WITHOUT A CONTRACT COMMIT" although `contract(schema): …` and `contract(proto): …` are on the
branch (3 `tools/ci.sh check --base main` probes: fail / pass / fail). I re-ran the unchanged gate until the guard passed
(attempts logged in the status doc); nothing else in the gate is retried.

## Q13 D-128 (`show trace` crash) — nothing to remove here
No test or code of this task runs `show trace` or `trace add` (grep of the owned files and the topology module: none). Host
evidence uses `vppctl show ip fib …`, `show svs`, FIB dumps and ping replies only.
