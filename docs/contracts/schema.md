# Configuration schema contract (`packages/schema`) — index

The whole VRX configuration is **one JSON document** validated by **one root schema** (`RootConfig`, Zod 4) and stored as
`config_revision.payload` (docs/04-api-datamodel.md). `pnpm gen` derives from it the TypeScript types, one JSON Schema
2020-12 per top-level key (`dist/json-schema/<key>.json`, consumed by the UI form renderer) and the OpenAPI 3.1
components (`dist/openapi-components.json`: `RootConfig`, `SystemConfig`, `InterfacesConfig`, …). One definition, three
consumers — never duplicate a type by hand.

> **Rule:** changing this package requires a `contract/<id>` branch with a `contract(schema): …` commit
> (`tools/ci.sh --base main` refuses `packages/schema` changes without one). Adding fields or domains as designed in
> docs/04 is an ordinary contract branch; **renaming or reshaping an existing field is always a PENDING decision**
> (docs/decisions/decision-policy.md #1). After `contracts-v1` is tagged, additive changes are merged by the manager.

Per-group detail documents (one table per modelled key):

| group | keys | document |
|---|---|---|
| (a) P02a | `system`, `dataplane`, `interfaces`, `vrfs`, `routing`, `management` | this file, below |
| (b) P02b | `nat`, `objects`, `acl` | `docs/contracts/schema-nat-objects-acl.md` |
| (c) P02c | `vpn`, `tunnels`, `services`, `ha` | `docs/contracts/schema-vpn-tunnels-services-ha.md` |

## Conventions every domain follows

- **Strict objects.** Every modelled object is `z.strictObject`: unknown keys are rejected with a pointer (a typo such
  as `mtuu` never disappears silently). Records (`interfaces`, `vrfs`, `prefixLists`, …) validate their keys with a
  primitive (`propertyNames` in JSON Schema).
- **Empty is valid.** `RootConfig.parse({})` succeeds; every domain accepts `{}` and fills its defaults (root keys use
  `.prefault({})`, D-017). "Required" settings are semantic rules, not schema `required` — e.g. an admin user.
- **Defaults are explicit** and emitted as JSON Schema `default`, so a parsed document is complete (renderers never guess).
  `RootConfig.parse(RootConfig.parse(x))` equals `RootConfig.parse(x)`.
- **Primitives** (`src/primitives.ts`): `ipv4Address`, `ipv6Address`, `ipAddress`, `ipv4Cidr`, `ipv6Cidr`, `ipCidr`,
  `ipv4Network`/`ipv6Network`/`ipNetwork` (host bits zero), `macAddress` (unicast, non-zero), `vppInterfaceName`,
  `vlanId`, `mtu`, `pciAddress`, `hostname` (RFC 1123), `hostOrIp`, `objectName`, `vrfName`, `username`,
  `descriptionText`, `portNumber`, `asNumber`, `routerId`, `uint32`, `cpuCore`, `secretRef`, `passwordHash`,
  `timezone` (IANA, checked against ICU). Patterns contain no lookaround so Go/Python can reuse them.
- **UI hints.** Every field is wrapped in `withUi()` → `title`, `description` and `x-vrx-ui: { widget, group, order,
  help, secret, itemKey }` (D-019). Domain schemas carry `x-vrx-ui.order` = navigation order (vdom.md guardrail #4).
- **Secrets** never sit inline: `secretRef` points into the secret store; `passwordHash` is `writeOnly` (never returned
  by GET). Fixtures use `$vrx-test$VRX_TEST_HASH_<id>`.
- **VRF first-class** (vdom.md #1): every interface, route, protocol instance, NTP/DNS/AAA/syslog server carries `vrf`
  (default `default`). `default` is VPP table 0 and always exists; it may be declared under `/vrfs` only with `id: 0`.
- **Pointers** are RFC 6901 built with `jsonPointer()` — VPP names contain `/` (`TenGigabitEthernet0~10~10`).
- **Arrays are diff leaves** (D-021): `diff()` reports a changed list as one `replace`; arrays of objects declare
  `x-vrx-ui.itemKey` so the UI can pair items.

## Validation and editing API (`@ngfw/schema`)

| export | purpose |
|---|---|
| `RootConfig`, `RootConfigInput`, `ROOT_KEYS` | root schema, its output/input types, the 13 keys in documented order |
| `validateConfig(doc)` | tier (a) schema + tier (b) semantic in one call → `{ ok, config }` or `{ ok: false, tier, issues }` |
| `pointerIssues(zodError)` | Zod issues → `{ pointer, message }[]` for RFC 9457 problem+json |
| `validateSemantics(config, domains?)`, `SEMANTIC_VALIDATORS`, `semanticRegistry` | named validators `<domain>.<rule>` returning `{ pointer, message }[]` |
| `mergePatch(target, patch)`, `mergePatchAt(doc, pointer, patch)` | RFC 7386, whole document or at `PATCH /api/v1/config/{path}` |
| `diff(a, b)` | `{ op: add\|remove\|replace, pointer, from?, to? }[]`, deterministic (sorted keys) |
| `jsonPointer(...segments)`, `parsePointer(p)` | RFC 6901 helpers |
| `generateSchemas()` | what `pnpm gen` writes, as data |

## `system`

| field | type | default | notes |
|---|---|---|---|
| `hostname` | `hostname` | `vrx` | RFC 1123, ≤ 253 chars, last label not all digits |
| `timezone` | `timezone` | `UTC` | IANA name, existence checked with ICU |
| `banner.login` | string ≤ 4096 | — | pre-login banner (SSH issue, web login) |
| `banner.motd` | string ≤ 4096 | — | after login |
| `ntp.enabled` | boolean | `true` | chrony |
| `ntp.servers[]` | `{ address: hostOrIp, prefer: false, iburst: true }` | `[]` | ≤ 16, `itemKey: address` |
| `ntp.vrf` | `vrfName` | `default` | |
| `dns.servers[]` | `ipAddress` | `[]` | ≤ 8 |
| `dns.searchDomains[]` | `hostname` | `[]` | ≤ 6 |
| `dns.vrf` | `vrfName` | `default` | |

Semantic: `system.vrf-exists`, `system.ntp-server-unique`.

## `dataplane`

All optional (absent = platform default); applied at VPP start-up, not live.

| field | type | notes |
|---|---|---|
| `workers` | int 0–255 | `cpu { workers N }`; must equal `corelist.length` when both given |
| `corelist[]` | `cpuCore` (0–1023) | `corelist-workers`; unique |
| `mainCore` | `cpuCore` | not a worker core |
| `rxQueues`, `txQueues` | int 1–256 | `dpdk { dev default { num-rx-queues } }` |
| `hugepagesGb` | int 1–1024 | |
| `pciWhitelist[]` | `pciAddress` | default `[]`, ≤ 64, unique (case-insensitive) |

Semantic: `dataplane.workers-match-corelist`, `dataplane.corelist-unique`, `dataplane.main-core-not-worker`, `dataplane.pci-unique`.

## `interfaces`

Record keyed by **parent** VPP interface name (`parentInterfaceName`, no `.sub` suffix). Sub-interfaces are named
`<parent>.<id>` on VPP and referenced that way (`unnumbered`, static next hops, NAT, ACL, OSPF …).

| field | type | default | notes |
|---|---|---|---|
| `enabled` | boolean | `false` | admin state; nothing forwards until enabled |
| `description` | `descriptionText` | — | |
| `mtu` | 68–9216 | — | absent = driver default |
| `mac` | `macAddress` | — | unicast, non-zero |
| `promiscuous` | boolean | `false` | |
| `rxMode` | `polling \| interrupt \| adaptive` | — | |
| `ipv4[]`, `ipv6[]` | `ipv4Cidr` / `ipv6Cidr` | `[]` | host bits kept; ≤ 32 each |
| `vrf` | `vrfName` | `default` | |
| `unnumbered` | `vppInterfaceName` | — | borrow addresses; `ipv4`/`ipv6` must be empty (refine) |
| `subinterfaces` | record `subInterfaceId` (decimal u32) → sub-interface | `{}` | |
| `subinterfaces.*.vlanId` | 1–4094 | required | outer tag |
| `subinterfaces.*.innerVlanId` | 1–4094 | — | QinQ inner tag |
| `subinterfaces.*.dot1ad` | boolean | `false` | 802.1ad (0x88a8) outer tag |
| `subinterfaces.*.{enabled, description, mtu, ipv4, ipv6, vrf, unnumbered}` | as above | | |

Semantic: `interfaces.vrf-exists`, `interfaces.address-no-overlap` (both families, per VRF, across parents and
sub-interfaces; several addresses of one subnet on one interface are allowed, the same address twice is not),
`interfaces.vlan-unique` (tag combination per parent), `interfaces.unnumbered-target-exists`, `interfaces.mac-unique`.

## `vrfs`

Record `objectName` → `{ id: uint32 (required), description? }`. `default` = table 0, implicit; declared only with `id: 0`.

Semantic: `vrfs.id-unique`, `vrfs.default-is-table-zero`.

## `routing`

`{}` is valid; each protocol object is **absent when disabled**.

| field | type | notes |
|---|---|---|
| `static[]` | `{ prefix: ipNetwork, vrf = default, nextHops[] ≥ 1: { address?, interface?, weight = 1 }, distance = 1, description? }` | `itemKey: [vrf, prefix]`; hop needs address or interface; hop family = prefix family (refine) |
| `prefixLists` | record → `{ description?, family = ipv4, rules[]: { seq ≥ 1, action, prefix, ge?, le? } }` | unique `seq`, prefixes match `family`, `ge ≤ le` (refine) |
| `routeMaps` | record → `{ description?, entries[]: { seq, action, description?, match {…}, set {…} } }` | unique `seq`; `match`: prefixList, nextHopPrefixList, interface, community, asPath, metric, tag; `set`: localPreference, metric, weight, nextHop, community[], communityAdditive, asPathPrepend[], tag |
| `bgp` | `{ asn, routerId?, vrf, peerGroups{}, neighbors[], networks[], redistribute[], gracefulRestart = false, ebgpRequiresPolicy = true }` | neighbour: `address`, `remoteAs` or `peerGroup`, `updateSource`, `ebgpMultihop`, `passwordRef` (secretRef), timers (hold > keepalive), `bfd`, `shutdown`, `ipv4Unicast`/`ipv6Unicast` `{ enabled, routeMapIn/Out, prefixListIn/Out, nextHopSelf, maximumPrefixes, defaultOriginate }` |
| `ospf` | `{ routerId?, vrf, areas[]: { id (int or dotted), type, noSummary }, interfaces[]: { name, area, cost, passive, networkType, hello/dead, priority, bfd }, redistribute[], defaultInformationOriginate }` | area ids unique after normalisation (`0` = `0.0.0.0`) |
| `isis` | `{ net (NET), level = level-1-2, vrf, interfaces[]: { name, passive, metric, circuitType, networkType, bfd }, redistribute[] }` | |
| `rip` | `{ vrf, networks[]: ipv4Network, interfaces[]: { name, passive }, redistribute[], defaultMetric = 1 }` | |
| `bfd` | `{ sessions[]: { interface, localAddress, peerAddress, desiredMinTxUs = 300000, requiredMinRxUs = 300000, detectMultiplier = 3, enabled = true } }` | VPP native BFD; addresses in one family (refine) |

Semantic: `routing.vrf-exists`, `routing.static-nexthop-interface-exists`, `routing.static-unique`,
`routing.prefix-list-exists`, `routing.route-map-exists`, `routing.bgp-peer-group-exists`, `routing.interface-exists`
(route-map match, BGP update source, OSPF/IS-IS/RIP/BFD interfaces), `routing.ospf-area-exists`.

## `management`

| field | type | default | notes |
|---|---|---|---|
| `users[]` | `{ username, role: admin\|operator\|readonly, scope: '*', passwordHash?, sshKeys[], fullName?, disabled }` | `[]` | `itemKey: username`; `scope` literal `*` (vdom.md #3); `passwordHash` write-only |
| `aaa.order[]` | `local \| radius \| tacacs` | `['local']` | unique; listed methods need servers (refine) |
| `aaa.radius.servers[]` | `{ address, authPort = 1812, acctPort = 1813, secretRef, timeoutSec = 5, vrf }` | `[]` | |
| `aaa.tacacs.servers[]` | `{ address, port = 49, secretRef, timeoutSec = 5, vrf }` | `[]` | |
| `tls` | `{ certificateRef?, privateKeyRef?, minVersion = 1.2 }` | | cert and key together (refine); absent = self-signed |
| `syslog[]` | `{ address, port = 514, protocol = udp, severity = info, vrf }` | `[]` | |

Semantic: `management.admin-exists` (an enabled admin with a password or SSH key), `management.username-unique`,
`management.vrf-exists`.

## Examples (`packages/schema/examples/`)

`minimal.json` (smallest committable document: one admin), `two-interfaces.json` (every group (a) domain exercised),
`invalid-*.json` (schema rejects), `invalid-semantic-*.json` (schema accepts, a validator reports the pointer listed in
`src/examples.test.ts`). All are run by `pnpm test`.
