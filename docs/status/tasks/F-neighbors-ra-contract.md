# F-neighbors-ra — contract changes (additive)

Two commits on `task/F-neighbors-ra` (the ci.sh contract guard checks the subjects), both additive; numbers from
docs/status/wave-A-hotspots.md §2 only.

## `contract(schema): neighbours, RA, proxy-ARP/ND`
Sub-schemas in `packages/schema/src/domains/ext/neighbors-ra.ts`, one key line per leaf under the F-neighbors-ra anchors:

| leaf | type | absent / default |
|---|---|---|
| `interfaces.<if>.ipv6Ra` (+ sub-interfaces) | `{suppress, managed, other, lifetimeSec, maxIntervalSec, minIntervalSec, prefixes{<ipv6 network>: {validSec, preferredSec, offLink, noAutoconfig}}}` | absent = not configured; the defaults are VPP's fresh-interface state (suppress true, 600/200/150 s, prefixes 2592000/604800 s) |
| `interfaces.<if>.proxyArp` (+ sub-interfaces) | optional boolean | absent / false = off |
| `interfaces.<if>.proxyNd` (+ sub-interfaces) | optional IPv6 address list (≤ 64) | absent / [] = off; experimental (V12, D-064) |
| `vrfs.<name>.proxyArpRanges` | optional `[{low, high}]` IPv4, low ≤ high | absent / [] = none |
| `routing.neighbors` | optional `{static[{interface, ip, mac, noFibEntry}], ipv4Limits?, ipv6Limits?{maxNumber, maxAgeSec, recycle}, dad?{transmits 1–10, delayMs 100–10000}}` | absent = none; limits/DAD absent = VPP defaults / DAD off |

No default is added to existing objects: a document that does not use the feature parses to exactly what it parsed to
before (tested), so Retrieve of every existing interface keeps its shape.

Refinements (single object, 400 at the field): RA `minIntervalSec ≤ 0.75 × maxIntervalSec`, `lifetimeSec = 0 or >
maxIntervalSec` (VPP `ip6_ra_config` rejects both), `maxIntervalSec` 4–1800, `lifetimeSec ≤ 9000` (VPP clamps above),
prefix `preferredSec ≤ validSec`, proxy-ARP range `low ≤ high` (IPv4 only: ARP), MAC unicast (existing primitive).

Semantic rules (`packages/schema/src/semantic/neighbors-ra.ts`, one import + one spread under the C2 anchors):
`interfaces.neighbors-ra-ipv6-required` (a non-default `ipv6Ra`, or `proxyNd`, needs an IPv6 address: VPP answers
IP6_NOT_ENABLED otherwise), `interfaces.neighbors-ra-slaac-prefix-length` (autonomous prefixes are /64),
`interfaces.neighbors-ra-proxy-nd-unique`, `vrfs.neighbors-ra-proxy-arp-range-unique`,
`routing.neighbors-ra-static-interface-exists`, `routing.neighbors-ra-static-connected` (the address lies in a connected
subnet of the interface — or of its unnumbered source; fe80::/10 when it has IPv6; never the interface's own address),
`routing.neighbors-ra-static-unique` (interface + canonical address).

## `contract(proto): ListNeighbors, arp_flush, neighbour event`
- Mirror fields: `Interface` 15 `ipv6_ra`, 16 `proxy_arp`, 17 `proxy_nd`; `Subinterface` 13–15 (same); `Vrf` 4
  `proxy_arp_ranges`; `RoutingConfig` 10 `neighbors`; messages `Ipv6Ra`, `Ipv6RaPrefix`, `ProxyArpRange`,
  `NeighborsConfig`, `StaticNeighbor`, `NeighborLimits`, `NeighborDad` in the `// ----- F-neighbors-ra -----` section.
  The drift guard (`apps/agent/internal/contracttest`) passes both directions.
- `rpc ListNeighbors(ListNeighborsRequest) returns (ListNeighborsResponse)`, `NeighborEntry`.
- `ActionRequest.action` 4 `arp_flush` (`ArpFlushAction{interface, family}`).
- `EventKind` 10 `EVENT_KIND_NEIGHBOR_CHANGED`.
- `docs/contracts/proto.md` §11: `### F-neighbors-ra: ListNeighbors`, `…: ActionRequest.arp_flush (4)`, `…: EventKind…(10)`.
- `apps/api/src/testing/fake-agent.ts`: the UNIMPLEMENTED `listNeighbors` stub under the P5 anchor.
- `packages/proto/test/desired-state.test.ts`: one expectation now lists `proxyArpRanges: []` (ts-proto fills an
  absent repeated field with `[]`; the exact-shape assertion on `vrfs['customer-a']` broke with any new repeated `Vrf`
  field). Not an owned file — the only non-anchor edit of the contract, listed under "Shared hunks".

Regenerated (never hand-edited): `apps/agent/gen/**`, `packages/proto/gen/ts/**`, `packages/api-client/src/generated/**`.
