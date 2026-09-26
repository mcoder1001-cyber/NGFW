# P12 contract change (additive, P08 pattern: `contract(schema|proto): …` commits on `task/P12`)

Numbers from docs/status/wave-BC-numbers.md "Batch-2 follow-ons" (binding since D-109 e); nothing else is taken.

| where | change | number |
|---|---|---|
| schema `interfaces.<name>.lcp` (`domains/ext/frr-linuxcp.ts`, key line under `wave-A: P12` in `InterfaceSchema`) | `{hostIfName?: Linux name ≤ 15, hostIfType: tap\|tun = tap, netns?: ≤ 31}`; present = linux-cp pair | proto `Interface.lcp` **22** → `InterfaceLcp{host_if_name 1, host_if_type 2, netns 3}` |
| schema `routing.static[].tag` (key line under `wave-A: P12` in `StaticRouteSchema`) | optional uint32 ≥ 1 (RF-1 Q1) | proto `StaticRoute.tag` **8** |
| proto `EventKind` | `EVENT_KIND_ROUTING_CHANGED` | **14** |
| proto `EventKind` | `EVENT_KIND_BGP_NEIGHBOR_CHANGED` | **15** |
| proto `service Dataplane` (under `wave-A: P12`) | `rpc RoutingState(RoutingStateRequest) returns (RoutingStateResponse)` (RF-1 Q2) | — |
| proto `// ----- P12 -----` section | `InterfaceLcp`, `RoutingStateRequest`, `RoutingStateResponse`, `BgpInstanceState`, `BgpNeighborState`, `BgpNeighborAfiState`, `RoutingLcpPair`, `RoutingRibEntry`, `RoutingRibNextHop` (field numbers from 1) | — |
| semantic `semantic/bgp.ts` (spread under `wave-A: P12`) | `routing.bgp-lcp-host-name`, `routing.bgp-lcp-host-name-unique`, `routing.bgp-interface-has-lcp`, `routing.bgp-static-tag-via-frr` | — |
| `apps/api/src/testing/fake-agent.ts` (P5 anchor) | `routingState` UNIMPLEMENTED stub | — |
| `docs/contracts/proto.md` §11 | `### P12: RoutingState` | — |
| fixture `packages/proto/test/fixtures/frr-linuxcp-bgp.json` | round-trip corpus (lcp, tag, bgp, policy) | — |

`RoutingConfig` 12 (the routing-level LCP option) stays reserved and unused (P12-questions Q2). Why an interface leaf, and
why a `RoutingState` RPC rather than FRR state in `Retrieve`: P12-questions Q2, contracts §5/§11. `buf breaking` has
nothing to flag (new fields/values/messages only); the drift guard (`contracttest`) passes: every new schema leaf has
its proto field.
