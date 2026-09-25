# FRR BGP + routing policy sections, the FRR stage and linux-cp (P12)

Packages: `internal/renderers/frr/bgp` (section `bgp`, order 500), `internal/renderers/frr/policy` (section `policy`,
order 850), `internal/lcpmap` (VPP → Linux names, S2 producer `lcp-addresses`), `internal/desired/{bgp,lcp}.go`
(builders/assemblers), `internal/subsystems/frr.go` (FRR runtime, `frr.config` descriptor, events, RoutingState),
`internal/agent/rpc_routing.go` (gRPC). Framework rules (validators, redaction, convergence): `frr.md`, the RF-1 README.

## Desired state → FRR

| document | FRR 10.7 (canonical `show running-config` form) |
|---|---|
| `routing.bgp{asn, vrf}` | `router bgp <asn> [vrf <vrf>]` … `exit` |
| `routerId` | ` bgp router-id A.B.C.D` |
| `ebgpRequiresPolicy: false` | ` no bgp ebgp-requires-policy` (FRR's default is on, RFC 8212) |
| always | ` no bgp default ipv4-unicast` — a family is active for a peer exactly when `afi.<family>` is set |
| `gracefulRestart` | ` bgp graceful-restart` |
| `peerGroups.<g>` | ` neighbor <g> peer-group` + ` neighbor <g> remote-as N` + the peer lines |
| `neighbors.<addr>` | ` neighbor <addr> remote-as N` (when set) + ` neighbor <addr> peer-group <g>` + peer lines + ` … shutdown` |
| peer: `description` / `ebgpMultihop` (255 → no number) / `passwordRef` / timers / `updateSource` / `bfd` | ` description …`, ` ebgp-multihop [N]`, ` password <secret>`, ` timers K H` (the missing one derived: H = 3K, K = H/3), ` update-source <addr\|linux-if>`, ` bfd` |
| `afi.ipv4Unicast` / `ipv6Unicast` (enabled ≠ false) | inside ` address-family ipv4\|ipv6 unicast` … ` exit-address-family`: `  neighbor X activate`, `next-hop-self`, `default-originate`, `soft-reconfiguration inbound`, `prefix-list P in\|out`, `route-map R in\|out`, `maximum-prefix N` |
| `networks[]` | `  network P [route-map R]` in the family of P, address order |
| `redistribute.<src>{metric, routeMap}` | `  redistribute <src> [metric N] [route-map R]` (IPv4 always, IPv6 when the family is in use) |
| `routing.policy.prefixLists.<n>{family, description, rules[]}` | `ip\|ipv6 prefix-list <n> description …` + `… seq S permit\|deny P [ge G] [le L]` |
| `routing.policy.routeMaps.<n>.entries[]` | `route-map <n> permit\|deny <seq>` + ` description`, matches (`ip address prefix-list`, `ip next-hop prefix-list`, `interface`, `community`, `as-path`, `metric`, `tag`), sets (`as-path prepend`, `community … [additive]`, `ip next-hop` / `ipv6 next-hop global`, `local-preference`, `metric` (= MED), `tag`, `weight`) + `exit` |
| route-map `match community C` / `match as-path RE` | generated lists `bgp community-list standard vrx-<map>-<seq> seq 5 permit C`, `bgp as-path access-list vrx-<map>-<seq> seq 5 permit RE` (FRR matches list names, the document carries values) |
| `interfaces.<n>.lcp` + `ipv4`/`ipv6` | `interface <linux-name>` + ` ip address A/L` / ` ipv6 address …` (S2, `lcp-addresses`) |
| `routing.static[i]` with `viaFrr` (+ `tag`) | the framework `static` section: `ip route P NH [IF] [tag T] [D]` (D-072: the agent then does not program it) |

Validation beyond the schema (render-time, reported by DryRun through the projection's render check, pointers under
`/routing`): route-map sequence 1–65535 (FRR's range), unique sequences, prefix-list family / ge / le, AS-path regex
without `"| "` (FRR's CLI pipe), communities 0–65535, unmapped interfaces (update-source, `match interface`), neighbour
addresses unique canonically, `redistribute.bgp` refused.

Secrets: `passwordRef` → `rc.Secret` (D-051/D-072). Until PENDING-secret-channel the product agent has no resolver: a
neighbour or peer group with `passwordRef` is a validation error `routing.bgp-password-unavailable` at the reference's
pointer (DryRun and Apply). The redaction path (frr.conf `Secret`, `<redacted>` in Show/Retrieve/DryRun/errors) is
proven by `bgp/integration_test.go` with the fixture resolver (`VRX_TEST_PSK_P12_1`).

## The FRR stage: `frr.config/vrx` (D-109 d, P12-questions Q3)

One singleton scheduler object in `Domains["routing"]`, value = the FRR-relevant subset of the document
(`desired.FRRDoc`: bgp, policy, viaFrr statics, paired interfaces' lcp/addresses/description — references, never secret
values) plus a status. It exists while any of these exists — **a paired interface alone counts**: FRR keeps the VPP
addresses on the Linux side for as long as the pair exists, because with linux-nl listening in FRR's netns a removal of
the tap address would be mirrored into VPP and take the address off the VPP interface. Removing a pair is safe: the
scheduler deletes first (the pair and its tap go), then updates FRR. The object belongs to the `routing` domain, so an
address change of a paired interface reaches the tap in a transaction that includes `routing` (the API sends every
domain; an `interfaces`-only Apply leaves the tap addresses to the next routing transaction).

- **Create/Update**: render (all registered sections + interface-line producers, linux-cp mapping of the document's own
  pairs) → `vtysh -C` → `frr-reload.py --reload` → convergence check (`--test` must show no diff). The daemons are never
  restarted; `bgp/integration_test.go` and the topology test compare the daemon PIDs before/after.
- **Delete**: the framework-only configuration (every protocol and filter goes; FRR keeps running).
- **Retrieve**: `applied` when `frr-reload.py --test` of the last applied files is empty, `drift` otherwise (→ Update),
  `unreachable` when FRR has no vty socket, `unknown` after an agent restart when FRR holds configuration (→ Update:
  the re-apply is an empty diff for an unchanged document; sessions stay up).
- **Dependencies**: `lcp.itf-pair/<n>` of every paired interface in the document (taps exist before FRR configures them).
- Which FRR: owner `vrx` → `/etc/frr` (product, no pathspace); `VRX_FRR_PATHSPACE=<slot>` → that slot's frrtest
  instance; otherwise, or `VRX_FRR=off`, none — FRR content is then an `agent.unsupported-field` warning, not applied.
- Assemble: `routing.bgp`, `routing.policy` and the viaFrr statics come back from the object when its status is
  `applied` (contract §5: a leaf the backend cannot vouch for stays unset).
- TD-11b: `RecordsNoOwnership()` (no claim or boot records).

## State and events

- `RoutingState` RPC (contracts §11 P12): `show bgp vrf all summary json` (instances, neighbours: state, uptime,
  prefixes rx/tx, flaps = `connectionsDropped`), `show ip[v6] route vrf all summary json` (RIB counts), registered
  readers by key, a RIB lookup per prefix (≤ 100, `show ip[v6] route vrf V P json`), this owner's pairs. Never a table dump
  (RF-1 review M3). FRR down is `frr_running=false`, not an error.
- Events (TD-8 `Wiring.Publish`): a 1 Hz poller (after the first apply, while FRR carries this agent's configuration):
  `bgp-neighbors` → `EVENT_KIND_BGP_NEIGHBOR_CHANGED` (15), RIB counts → `EVENT_KIND_ROUTING_CHANGED` (14); the API relays
  both on the `routing.events` WebSocket topic.

## FRR → VPP routes

linux-nl's (AD-2): zebra installs the routes in the kernel of FRR's netns, linux_nl (listening in the netns that was the
lcp default when the first pair was created) programs them into the VPP table of the same id (254 → 0) with source
`lcp-rt-dynamic`. The agent programs no FRR-learned route and registers no dynamic desired source (TD-8 V1 does not
apply: linux_nl handles every route on its own). `/state/routes` shows those routes with `origin: frr`; `proto=<p>`
filters them by FRR protocol (API `features/bgp/routes.ts`, one RIB lookup per page prefix).

## Tests

| test | what |
|---|---|
| `bgp/render_test.go` | golden `testdata/full.golden` (every rendered form), render errors, policy errors, summary parser + poller |
| `bgp/integration_test.go` (VRX_INTEGRATION) | two slot FRR instances (`Instance: "p1"`), MD5 session, 100 prefixes, route map → 50, withdraw → 0, poller event, removal, redaction |
| `subsystems/frr_test.go` | frr.config lifecycle with a recording runner (not running → no exec, create/validate/reload/converge, drift, delete, unknown after restart), events, paths, tap gate |
| `agent/project_p12_test.go` | projection with/without FRR, passwordRef refusal, render check, assemble, S3 table |
| `agent/p12_topology_integration_test.go` (`test/topology/bgp/run.sh`) | VPP + linux-cp + 3 FRR instances: 200 → 100 → withdraw < 5 s → agent restart with pair loss → link down → rollback |
