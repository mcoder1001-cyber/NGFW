# F-vrf-static-ecmp — contract changes (additive, for manager review)

Two commits on `task/F-vrf-static-ecmp` (the envelope's "no own branches, P08 pattern"), both additive (`buf breaking`
green, no field renamed or renumbered, numbers from `docs/status/wave-A-hotspots.md` §2 / the envelope):

## `contract(schema): vrfs source-select, next-hop vrf, viaFrr`

| schema (Zod) | proto mirror | number | meaning |
|---|---|---|---|
| `vrfs.<name>.sourceSelect?[]{prefix, interface}` (`domains/ext/vrf-static-ecmp.ts`, key line under the `VrfSchema` anchor) | `Vrf.source_select` → `repeated VrfSourceSelect{optional prefix = 1, optional interface = 2}` | Vrf **3** | Source VRF select (VPP `svs`): packets from `prefix` arriving on `interface` are routed in this VRF. `/0` rejected (it is the interface's own VRF). |
| `routing.static[].nextHops[].vrf?` | `NextHop.vrf` (`optional string`) | NextHop **4** (§2 "proposed", touched only by this task) | Resolve the next-hop address in another VRF (VPP path `table_id`). Absent = the route's VRF. |
| `routing.static[].viaFrr?: boolean` | `StaticRoute.via_frr` (`optional bool`) | StaticRoute **7** | D-072 flag: FRR (staticd) programs the route, the agent skips it. Selector registered by this task; FRR rendering is P12 / F-bfd-redistribution. |

`viaFrr` and `sourceSelect` are **optional without a default** so every existing document parses to the same value as
before (the `group-a.test.ts` exact-parse assertions and P08's Retrieve equality tests stay untouched).

Semantic rules (`packages/schema/src/semantic/vrf-static-ecmp.ts`, one spread line under the C2 anchor):
`routing.vrf-static-ecmp-nexthop-vrf` (next-hop VRF exists, differs from the route's VRF, not combined with an egress
interface), `routing.vrf-static-ecmp-single-path-weight` (weights only with ≥ 2 next hops),
`vrfs.vrf-static-ecmp-source-select-interface-exists`, `vrfs.vrf-static-ecmp-source-select-unique` (one VRF per
(interface, source prefix)). Existing rules reused and tested, not re-added: `vrfs.id-unique`,
`vrfs.default-is-table-zero`, `routing.vrf-exists`, `routing.static-nexthop-interface-exists`, `routing.static-unique`.

Fixture: `packages/proto/test/fixtures/vrf-static-ecmp-full.json` (valid document using all three fields; part of the
TS and Go DesiredState round-trip/drift corpora). **No** `packages/schema/examples/vrf-static-ecmp-*.json`:
`examples.test.ts` rejects any file that is neither group (a) nor a P02b/P02c sibling prefix (see questions Q3).

## `contract(proto): ListRoutes`

`rpc ListRoutes(ListRoutesRequest) returns (ListRoutesResponse)` under the service anchor; `ListRoutesRequest`,
`ListRoutesResponse`, `ListRoutesEntry`, `ListRoutesPath` (+ `VrfSourceSelect`) in the `// ----- F-vrf-static-ecmp -----`
section. Semantics: `docs/contracts/proto.md` §11 "F-vrf-static-ecmp: ListRoutes". `apps/api/src/testing/fake-agent.ts`
gets the UNIMPLEMENTED stub under the anchor (P5) so `pnpm typecheck` stays green.

One line outside an anchor, needed by the additive repeated field: `packages/proto/test/desired-state.test.ts` asserted
`ds.vrfs['customer-a']` with `toEqual({id, description})`; ts-proto materialises every repeated field as `[]`, so any
new repeated member of `Vrf` (this task's `source_select`, F-neighbors-ra's `proxy_arp_ranges`) breaks it. Changed to
`toMatchObject` (same intent, robust to additive fields; an identical edit by F-neighbors-ra merges cleanly).

Generated and committed: `apps/agent/gen/**`, `packages/proto/gen/ts/**`, `packages/api-client/src/generated/schema.d.ts`.
