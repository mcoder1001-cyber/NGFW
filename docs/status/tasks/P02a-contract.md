# P02a — contract changes of the review-fix round (schema group (a) + proto sync, D-061)

Branch `task/P02a`. Schema commits: `contract(schema): …`; proto commit: `contract(proto): sync group (a) messages …`
(`57f216e`), stubs regenerated with `packages/proto/gen.sh` after each `git merge main`. All changes land before
`contracts-v1`, so renames are allowed (D-061); each one is listed with the decision that asked for it.

## `packages/schema` (group (a))

| change | kind | reason |
|---|---|---|
| `system.ntp` removed | removal | D-050 — NTP lives once, in `services.ntp` |
| `DnsSchema` → `SystemDnsSchema`; `NtpSchema`/`NtpServerSchema` gone (TS identifiers only) | rename (TS) | D-047, review H2 — `export *` clash with group (c) |
| `system.banner.*`: `multilineText(4096)` (printable + LF/TAB) | tightening | review M1, D-049 |
| `interfaces.*.dhcpClient`, `….subinterfaces.*.dhcpClient` `{ hostname?, clientId?, setBroadcastFlag }` | additive | D-050 (D7.2) |
| `routing.prefixLists`, `routing.routeMaps` → `routing.policy.{prefixLists, routeMaps}` | reshape | D-045 (P12 §2) |
| prefix lists / route maps stay objects `{ description?, family, rules[] }` / `{ description?, entries[] }`, not P12's bare arrays | deviation from the P12 sketch | proto3 map values cannot be `repeated`; the document is projected 1:1 (D-042). Same content, one level of nesting |
| route-map `set.localPreference` → `set.localPref`, `set.metric` → `set.med` | rename | D-045 (P12 §2 names) |
| `match.asPath`: FRR regex alphabet only | tightening | review M1 (FRR config-line injection) |
| prefix-list rule ranges: `len < ge ≤ max`, `len ≤ le ≤ max`, `ge ≤ le`; `ge ≥ 1` | tightening | review L5 (FRR `lib/plist.c` rules) |
| `routing.static[].nextHops` default `[]`, new `blackhole = false`; ≥ 1 hop unless blackhole | additive + loosening | D-045, review M7 |
| `bgp.neighbors`: array with `address` → record keyed by address | reshape | D-045, review H4/M2 |
| BGP peer `ipv4Unicast`/`ipv6Unicast` → `afi.{ipv4Unicast, ipv6Unicast}`; new `softReconfig` | reshape + additive | D-045 (P12 §2) |
| `passwordRef` → `secretRefOf('password')` (`password/<name>`) | tightening | D-051 |
| `redistribute[]` of `{ protocol, metric?, routeMap? }` → record `{ connected?, static?, bgp?, ospf?, isis?, rip? }` (own protocol excluded) | reshape | D-045 (P12 names), review M2 |
| `ospf.areas[]` with `id` → record keyed by area id; `ospf/isis/rip.interfaces[]` with `name` → records keyed by interface | reshape | review M2 / L11, D-053 rule |
| OSPF area ids are strings (`"0"`, `"0.0.0.51"`), no more `number \| string` | reshape | 1:1 proto projection; review L11 |
| `secretRef` = `<kind>/<name>`, kinds `psk key cert password token`; `secretRefOf(kind)`; RADIUS/TACACS `psk/…`, TLS `cert/…` + `key/…` | tightening | D-051 |
| `sshKeys[]` comment part: no control characters | tightening | D-049 |
| new exports: `SECRET_KINDS`, `secretRefOf`, `multilineText`, `macPattern`, `redactSecrets`, `secretPointers`, `MergePatchError`, `FORBIDDEN_KEYS`, `inheritedHints`, `ip.ts` (`canonicalIp`, `canonicalPrefix`, `ipKey`, `prefixKey`, `parseCidr`, …), `BGP_AFIS`, `RoutingPolicySchema`, `DhcpClientSchema`, `bgpAsPathRegex`, … | additive | M4/M6/L4, D-046 |
| `diff(a, b, base?)` → `diff(a, b)` | signature | review L2 (the accumulator was never meant to be public) |
| `withUi` merges hints of the wrapped schema | behaviour | D-043, P02b review H1 |
| `acl.macip.rules[].sourceMac/sourceMacMask` use `macPattern` (group (b) file, 2 lines) | fix at merge | masks/wildcards are not unicast MACs; the group (a) `macAddress` correctly rejects them |

## `packages/proto/vrx/v1/dataplane.proto` (group (a) messages only; nat/vpn/tunnels/services/ha untouched)

| message | change | kind |
|---|---|---|
| `SystemConfig` | field 4 `ntp` → `reserved 4; reserved "ntp"`; `SystemNtp` deleted | removal (D-050) |
| `SystemDns` | `servers`, `search_domains`, `vrf` | additive (fills the placeholder) |
| `Interface` / `Subinterface` | `DhcpClient dhcp_client = 12` / `= 11`; new `DhcpClient { hostname, client_id, set_broadcast_flag }` | additive |
| `RoutingConfig` | fields 2/3 (`prefix_lists`, `route_maps`) reserved; new `RoutingPolicy policy = 9` | rename/move (D-045) |
| `StaticRoute` | `optional bool blackhole = 6` | additive |
| `RoutingPolicy`, `PrefixList(+Rule)`, `RouteMap(+Entry, Match, Set)` | filled | additive |
| `BgpConfig`, `BgpPeerGroup`, `BgpNeighbor`, `BgpAfi`, `BgpAddressFamily`, `BgpNetwork`, `Redistribute(+Options)` | filled; neighbours `map<string, BgpNeighbor>` | additive |
| `OspfConfig` (+`OspfArea`, `OspfInterface`), `IsisConfig` (+`IsisInterface`), `RipConfig` (+`RipInterface`), `BfdConfig` (+`BfdSession`) | filled; areas / interfaces are maps | additive |
| `ManagementAaa` (+`AaaRadius`, `RadiusServer`, `AaaTacacs`, `TacacsServer`), `ManagementTls`, `SyslogTarget` | filled; secrets only as `*_ref` | additive |

Every scalar is `optional` (explicit presence, D-039); `passwordHash` still has no field (D-040). `buf lint` clean.
`packages/proto/test/fixtures/all-domains.json` now exercises every new group (a) message; two contract tests lost
their `ntp` field (`apps/agent/internal/contracttest/desiredstate_test.go` `TestTypedConstruction`,
`packages/proto/test/desired-state.test.ts` typed construction) — the only edits to P03's test code.

## Evidence

- `go test ./internal/contracttest/` → `ok ngfw/agent/internal/contracttest` (strict protojson decode + key-path fidelity
  of every valid example incl. `examples/group-a-full.json`, and `fixtures/all-domains.json`).
- `pnpm --filter @ngfw/proto test` → `Tests 37 passed (37)`.
- Full gate: `docs/status/tasks/P02a.md` → "Review fixes".
