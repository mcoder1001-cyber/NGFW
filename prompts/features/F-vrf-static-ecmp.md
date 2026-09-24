# Task: F-vrf-static-ecmp — VRFs, static routes, ECMP, FIB browser, ping/traceroute   (prepend 00-CONTEXT.md)

## Goal
Complete L3 basics end to end in FAST MODE: VRF management incl. source-VRF select, static routes v4/v6 with weighted ECMP and
blackhole, a server-side paged FIB browser (1M+ routes) with DPO/adjacency detail, and ping/traceroute actions.
Reference: TNSR "Routing: static routes, VRFs, ping/traceroute"; VPP `ip`/`fib`, `svs`, `ping` (WBS D2.1, D2.2, D2.5, D2.6).

## Inputs to read first
- `apps/agent/internal/descriptors/core/` + its `README.md` (P05, merged): `vrf/<id>` (table name `<owner>:<vrf>`, `ErrTableConflict`),
  `ip.route/<table>/<prefix>` (paths sorted, weight, preference = distance, best-source claim rule, `ErrRouteConflict`),
  `interface-ip.table/<if>`; `apps/agent/internal/agent/projection.go` already maps `vrfs.*` and `routing.static[]`
- `packages/schema/src/domains/{vrfs,routing}.ts` — `vrfs.<name>{id, description}` and `routing.static[]{prefix, vrf, nextHops[{address,
  interface, weight}], blackhole, distance, description}` **exist**; no source-VRF-select field and **no next-hop VRF field**. Semantic
  rules that already exist (reuse + test, do not re-add): `vrfs.id-unique`, `vrfs.default-is-table-zero`, `routing.vrf-exists`,
  `routing.static-nexthop-interface-exists`, `routing.static-unique` (duplicate (vrf, prefix))
- `apps/agent/binapi/svs/` (`svs_table_add_del`, `svs_route_add_del`, `svs_enable_disable`, `svs_dump`), `apps/agent/binapi/ip/`
  (`ip_route_v2_dump`, `ip_route_lookup_v2`, `ip_table_flush`), `apps/agent/binapi/ping/` (`want_ping_finished_events` — confirm how
  a ping is started in binapi); traceroute has **no VPP API**
- `packages/proto/vrx/v1/dataplane.proto` — `Action` RPC with `PingAction`/`TracerouteAction` exists; `server.go` returns Unimplemented
- P06 (merged; the file was reworked by P08 — read P08's version): `apps/api/src/state/state.controller.ts` already serves `GET /state/routes?vrf&page&pageSize` by
  paging a full Retrieve in the API — **extend it, don't duplicate**; for 1M routes paging must move into the agent
- LOG D-072 (static routes: one programmer — VPP by default, FRR only when flagged), D-073b (descriptions kept in agent state),
  `docs/vpp-code-track.md` **V15** (table delete leaks API drop routes into the next table → fallback: remove own routes before deleting a
  table, clean leftovers by prefix — keep it and test it) and **V22a** (`ip_table_flush` crashed the shared VPP → never call it)
- binapi facts (checked 2026-09-24, prep-waveA): `ping` has exactly one request, `want_ping_finished_events{address, repeat, interval}`,
  answered by one `ping_finished_event{request_count, reply_count}` — no VRF/table, no source, no size, no per-reply output. `svs_dump`
  lists only interface enablements `{table_id, sw_if_index, af}`: there is no dump of SVS tables or SVS routes, so `svs.table` /
  `svs.route` must be read back from `ip_table_dump` / `ip_route_v2_dump` (the svs FIB source) if VPP exposes them there — verify on the
  host — else they are write-only (D-063/D-076: idempotent Create or a boot-keyed applied-once record); never echo desired state
- RF-1 `apps/agent/internal/renderers/frr/model.go`: `StaticOwnedByFRR` (already called by P08's projection) + `RegisterStaticSelector`
  — the owner of the D-072 schema flag registers the selector reading it, **exactly once** (a second call panics)

## Contract changes
Additive on `contract/F-vrf-static-ecmp`: `vrfs.<name>.sourceSelect?[]{prefix, interface}` (SVS), `routing.static[].nextHops[].vrf?`
(next hop resolved in another table; default = the route's VRF), `routing.static[].viaFrr?: boolean` (D-072 flag; leave the FRR
rendering to P12/F-bfd-redistribution), and a new read-only `ListRoutes` RPC (page/filter request, bounded response) for the FIB
browser — P08's live-state pattern; never put status or paging into `Retrieve` (docs/contracts/proto.md §5). Tell the manager; continue
against your branch.

## Scope — build exactly this
1. **Schema**: VRF ids unique (exists — test it); next-hop VRF exists (new rule for the new field); weights only meaningful with ≥ 2
   paths; `viaFrr` routes are not programmed by the agent (D-072); duplicates of the same (vrf, prefix) are already rejected by
   `routing.static-unique` (test it).
2. **Agent**: weighted multipath and blackhole **already exist** (P05 `core/route.go`: DROP path for an empty path set, per-path weight;
   P08's projection maps `nextHops`/`blackhole`/`distance`) — extend `core/route*.go` + `core_model.proto` only for
   next-hop-in-other-table; register the D-072 selector for `viaFrr`; new `svs` descriptor package
   (`svs.table/<id>`, `svs.route/<table>/<prefix>`, `svs.interface/<if>`); FIB lister with agent-side paging and filter (vrf, prefix,
   source) reading `ip_route_v2_dump` incrementally; ping via the ping plugin API in `apps/agent/internal/actions/vrf-static-ecmp/`
   (default table only — a non-default VRF, `source` or `size` answers `INVALID_ARGUMENT` with a clear message; output = one summary
   `line` + `done` with transmitted/received stats); traceroute → return `UNIMPLEMENTED` with a clear message unless a linux-cp path
   exists (P12), and write one V-item covering traceroute and VRF-aware ping. Unit tests with the fake client; ONE host check (prefixed
   VRFs/routes in your table range): Retrieve == desired, rollback empty, restart simulation.
3. **API**: config via pointer routes; `/api/v1/state/routes` (paged in the agent, filter `vrf,prefix,source`, detail = paths/DPO);
   `POST /api/v1/actions/ping` through `apps/api/src/actions/actions.controller.ts` (today a 501 stub with no agent call — you turn it
   into the Action RPC bridge; F-neighbors-ra and F-nat44-ed-sessions serve their own action routes from their feature modules).
4. **UI**: VRF list + form; static routes grid with ECMP path editor; FIB browser (ServerDataGrid, 1M-safe); Ping tool; en+fa.
5. **Docs**: `docs/user/routing/vrf-static-ecmp.md` (VRF + ECMP default route + ping; CLI equivalent).

Files you own: `apps/agent/internal/descriptors/core/{route,vrf}*.go`, `apps/agent/internal/descriptors/svs/**`, `docs/agent/descriptors/svs.md`,
`apps/agent/internal/actions/vrf-static-ecmp/**`, `apps/agent/internal/agent/project_vrf_static_ecmp*.go`,
`apps/agent/internal/desired/vrf_static_ecmp*.go` (P08 puts builders in `internal/desired/`), `apps/api/src/actions/**`,
`apps/api/src/features/vrf-static-ecmp/**`, `apps/web/src/domains/routing/vrf-static-ecmp/**`, `apps/web/src/locales/*/vrf-static-ecmp.json`,
`docs/user/routing/vrf-static-ecmp.md`, `test/topology/vrf-static-ecmp/**`.
Shared files (P08 layout; protocol and anchors in `docs/status/wave-A-hotspots.md`; list each hunk in your PR):
`apps/agent/internal/subsystems/subsystems.go` (register + `Domains`), `apps/agent/internal/agent/projection.go` (anchor calls, plus the
next-hop-VRF lines inside P08's `routing.static` project/assemble blocks — no other wave-A task edits those blocks),
`apps/agent/internal/agent/server.go` (`Action` type switch; the `ListRoutes` method goes in an owned `rpc_vrf_static_ecmp.go`),
`apps/agent/internal/descriptors/core/core_model.proto` (+ regenerated `.pb.go`), `apps/api/src/state/state.controller.ts` (move the
routes handler into your controller), `apps/api/src/agent/agent.client.ts`, `app.module.ts`, web router/nav/`i18n.ts`;
`packages/api-client` regenerated. `frr.RegisterStaticSelector` panics on a second call and `subsystems.Register` runs many times in
tests — call it through a `sync.Once`.

## Acceptance (paste the evidence)
- [ ] `vppctl show ip fib table <id> <prefix>` shows both weighted paths of the committed ECMP route; Retrieve == desired (pasted)
- [ ] FIB browser: 100k prefixes injected in your slot's table → page 1000 returns in < 1 s, no full-table gRPC message (timing pasted)
- [ ] Agent-restart simulation → VRFs + routes back within 30 s (log excerpt)
- [ ] Rollback removes routes then tables, no stray /32 left (V15 check pasted from `show ip fib table`)
- [ ] Next hop in an undeclared VRF → 400 problem+json with `pointer`
- [ ] Ping action from the UI returns replies from a rig address (screenshot); `tools/ci.sh --base main` green

## Out of scope (do not build)
BGP/OSPF/IS-IS/RIP and FRR static rendering (P12, F-ospf, F-isis-rip, F-bfd-redistribution); neighbour table / ARP flush (F-neighbors-ra);
uRPF, ABF/PBR (F-rpf-adl-pbr); MPLS labels on routes (F-mpls-srmpls); multicast routes (F-igmp-mfib); traceroute implementation without
a Linux path; VRF leaking via route-maps.

## Open questions to surface, not to decide silently
Traceroute: no VPP API — accept "needs linux-cp (P12)" or fund a V-item? Default: UNIMPLEMENTED + V-item.
Ping in a non-default VRF / with a source address: the ping API has neither — default: `INVALID_ARGUMENT` + the same V-item.
