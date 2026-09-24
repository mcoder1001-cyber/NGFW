# F-wireguard — contract changes (additive only)

Branch `task/F-wireguard`, committed first as `contract(schema): …` and `contract(proto): …` (envelope: never a
`contract/` branch). Numbers from `docs/status/wave-BC-numbers.md` "Batch-2 follow-ons" (binding since D-109 e):
**WireguardInterface 12**, **EventKind 13** — nothing else numbered.

## Schema (`packages/schema`)

| change | where | notes |
|---|---|---|
| `vpn.wireguard.interfaces.<n>.routeAllowedIps: boolean = false` | `domains/vpn.ts` (one key line in `WireguardInterfaceSchema`, no anchor existed; single toucher) | TNSR does not auto-route allowed IPs → default off (prompt open question, flagged in the questions file) |
| `vpn.wireguard-public-key-unique` | `semantic/wireguard.ts` (new, owned) + one import + one spread under the `wave-A: F-wireguard` anchors in `semantic/index.ts` | VPP-wide key uniqueness (DF-5): the second peer is reported; same-interface duplicates stay `vpn.wireguard-unique`'s |
| `vpn.wireguard-allowed-ips` | same file | no host bits; no overlap between peers of one interface (the identical prefix stays `vpn.wireguard-unique`'s) |
| `vpn.wireguard-endpoint-family` | same file | an IP endpoint has the listen address family; hostnames are not checked |

Existing P02c rules (`vpn.wireguard-unique`, `vpn.wireguard-address-overlap`, `vpn.local-address-configured`,
`vpn.vrf-exists`) and P06's `secrets.ref-exists` are not re-added.

## Proto (`packages/proto/vrx/v1/dataplane.proto`)

| change | number | where |
|---|---|---|
| `WireguardInterface.route_allowed_ips` (`optional bool`) | **12** | in P02c's message (single toucher; drift guard mirror of the schema leaf) |
| `EventKind.EVENT_KIND_WIREGUARD_PEER_CHANGED` | **13** | below the `wave-A: F-wireguard` anchor in `EventKind`, framed by blank lines |
| `rpc WireguardState(WireguardStateRequest) returns (WireguardStateResponse)` | — | below the `wave-A: F-wireguard` anchor in `service Dataplane` |
| `WireguardStateRequest`, `WireguardStateResponse`, `WireguardInterfaceState`, `WireguardPeerState` | fields from 1 | the `// ----- F-wireguard -----` section |

No secret field anywhere (D-040): the state carries only the interface's public key. `buf lint` clean, `buf breaking`
against main clean (additive). Semantics: `docs/contracts/proto.md` §11 "F-wireguard: WireguardState".

## Generated (C7)

`pnpm gen`: `apps/agent/gen/vrx/v1/*`, `packages/proto/gen/ts/vrx/v1/dataplane.ts`,
`packages/api-client/src/generated/schema.d.ts`. Fake agent (P5): the contract commit adds the `wireguardState`
UNIMPLEMENTED stub under the anchor; the real fake lives in `apps/api/src/features/wireguard/fake.ts`.
