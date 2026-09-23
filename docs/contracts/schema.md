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
- **Empty is valid.** `RootConfig.parse({})` succeeds and `validateConfig({})` is `ok` in both tiers (D-048); every
  domain accepts `{}` and fills its defaults (root keys use `.prefault({})`, D-017). "Required" settings are semantic
  rules, not schema `required` — e.g. once users exist, one of them must be a usable admin; the API seeds the first
  admin at install time.
- **Defaults are explicit** and emitted as JSON Schema `default`, so a parsed document is complete (renderers never guess).
  `RootConfig.parse(RootConfig.parse(x))` equals `RootConfig.parse(x)`.
- **Primitives** (`src/primitives.ts`): `ipv4Address`, `ipv6Address`, `ipAddress`, `ipv4Cidr`, `ipv6Cidr`, `ipCidr`,
  `ipv4Network`/`ipv6Network`/`ipNetwork` (host bits zero), `macAddress` (unicast, non-zero), `vppInterfaceName`,
  `vlanId`, `mtu`, `pciAddress`, `hostname` (RFC 1123), `hostOrIp`, `objectName`, `vrfName`, `username`,
  `macPattern` (any 48-bit value: MAC match values / masks), `descriptionText`, `multilineText(max)` (printable + LF/TAB),
  `portNumber`, `asNumber`, `routerId`, `uint32`, `cpuCore`, `secretRef` / `secretRefOf(kind)` (`<kind>/<name>`, kinds
  `psk key cert password token`, D-051), `passwordHash`, `timezone` (IANA, checked against ICU — the JSON Schema
  carries only the pattern, so other consumers cannot reproduce the existence check). Patterns contain no lookaround
  so Go/Python can reuse them.
- **No control characters** in free text that reaches a daemon config, a terminal or a log line (D-049): single-line
  fields reject C0/C1/DEL, banners allow LF/TAB only, the BGP AS-path regex is restricted to the FRR regex alphabet.
- **UI hints.** Every field is wrapped in `withUi()` → `title`, `description` and `x-vrx-ui: { widget, group, order,
  help, secret, itemKey }` (D-019). `withUi` **merges** with the hints already on the wrapped schema (also through
  `.optional()`/`.default()`), so re-wrapping a hinted primitive keeps its `widget`/`help` (D-043, P02b review H1).
  Domain schemas carry `x-vrx-ui.order` = navigation order (vdom.md guardrail #4).
- **Secrets** never sit inline: `secretRef` points into the secret store; `passwordHash` is `writeOnly` (never returned
  by GET). Fixtures use `$vrx-test$VRX_TEST_HASH_<id>`; valid `examples/*.json` carry no secret leaf at all (they are
  the protobuf drift corpus, D-040).
- **Secret round-trip rule (D-046).** Every document that leaves the write path (GET, `GET /config/diff` — diff the
  redacted documents —, revisions, export, `audit_log.before/after`, `DesiredState`) goes through `redactSecrets()`.
  On PUT/PATCH an **absent** write-only member keeps the stored value and an explicit `null` (merge-patch) clears it,
  so GET → edit → PUT never wipes a password hash. Arrays are matched by their `itemKey` for this (users by
  `username`), never by index.
- **VRF first-class** (vdom.md #1): every interface, route, protocol instance, NTP/DNS/AAA/syslog server carries `vrf`
  (default `default`). `default` is VPP table 0 and always exists; it may be declared under `/vrfs` only with `id: 0`.
- **Pointers** are RFC 6901 built with `jsonPointer()` — VPP names contain `/` (`TenGigabitEthernet0~10~10`).
- **Keyed collections are records** (D-045/D-053): anything with a natural key (interfaces, VRFs, BGP neighbours by
  address, IGP interfaces by name, OSPF areas by id, redistribution by source protocol, prefix lists, route maps) is
  a JSON object, so PATCH pointers are stable and duplicates are impossible. Ordered lists stay arrays.
- **Arrays are diff leaves** (D-021): `diff()` reports a changed list as one `replace`; arrays of objects declare
  `x-vrx-ui.itemKey` so the UI can pair items, and the owning domain has a semantic rule that the key is unique
  (canonical addresses/prefixes: `2001:DB8::1` = `2001:db8::1`, D-049).

## Validation and editing API (`@ngfw/schema`)

| export | purpose |
|---|---|
| `RootConfig`, `RootConfigInput`, `ROOT_KEYS` | root schema, its output/input types, the 13 keys in documented order |
| `validateConfig(doc)` | tier (a) schema + tier (b) semantic in one call → `{ ok, config }` or `{ ok: false, tier, issues }` |
| `pointerIssues(zodError)` | Zod issues → `{ pointer, message }[]` for RFC 9457 problem+json; an unknown key is reported at the key itself (`/interfaces/loop0/bogus`) |
| `validateSemantics(config, domains?)`, `SEMANTIC_VALIDATORS`, `semanticRegistry` | named validators `<domain>.<rule>` returning `{ pointer, message }[]` |
| `mergePatch(target, patch)`, `mergePatchAt(doc, pointer, patch)` | RFC 7386, whole document or at `PATCH /api/v1/config/{path}`; throws `MergePatchError { pointer }` (→ 400) on `__proto__`/`constructor`/`prototype` keys (`FORBIDDEN_KEYS`, D-049) or a bad array index |
| `diff(a, b)` | `{ op: add\|remove\|replace, pointer, from?, to? }[]`, deterministic (sorted keys); two parameters only — options come as a third later |
| `redactSecrets(doc)`, `secretPointers(doc)` | remove / list secret leaves (`x-vrx-ui.secret`, `writeOnly`) — D-046 |
| `canonicalIp`, `canonicalPrefix`, `ipKey`, `prefixKey`, `parseCidr`, `ipFamily`, … (`ip.ts`) | address arithmetic shared with groups (b)/(c) |
| `jsonPointer(...segments)`, `parsePointer(p)` | RFC 6901 helpers |
| `generateSchemas()` | what `pnpm gen` writes, as data |

## `system`

| field | type | default | notes |
|---|---|---|---|
| `hostname` | `hostname` | `vrx` | RFC 1123, ≤ 253 chars, last label not all digits |
| `timezone` | `timezone` | `UTC` | IANA name, existence checked with ICU |
| `banner.login` | `multilineText(4096)` | — | pre-login banner (SSH issue, web login); printable + LF/TAB |
| `banner.motd` | `multilineText(4096)` | — | after login |
| `dns.servers[]` | `ipAddress` | `[]` | ≤ 8, unique (canonical) |
| `dns.searchDomains[]` | `hostname` | `[]` | ≤ 6 |
| `dns.vrf` | `vrfName` | `default` | |

NTP is **not** here: it is modelled once, in `services.ntp` (D-050). TS name of the DNS schema: `SystemDnsSchema` (D-047).

Semantic: `system.vrf-exists`, `system.dns-server-unique`.

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
| `managementPci[]` | `pciAddress` | default `[]`, ≤ 4; always `dpdk { blacklist }`, never a DPDK device (F-startup-gen, D-081) |
| `devices` | record `pciAddress` → `{ name?, rxQueues?, txQueues?, rxDesc?, txDesc? }` | default `{}`; `name` = logical interface name `[a-z][a-z0-9_-]{0,14}` (D-069); queues 1–256; descriptors power of two 64–16384 |
| `buffersPerNuma` | int 1024–4194304 | `buffers { buffers-per-numa }` |
| `plugins` | optional `{ switches: record <name>_plugin.so → boolean }` | present = authoritative (exactly these switches; `true` = enable, `false` = disable); absent = the current start-up file's switches are kept (D-060, D-081, D-084) |

Semantic: `dataplane.workers-match-corelist`, `dataplane.corelist-unique`, `dataplane.main-core-not-worker`, `dataplane.pci-unique`,
`dataplane.devices-pci-unique`, `dataplane.management-not-dpdk`, `dataplane.logical-name-unique`, `dataplane.descriptors-power-of-two`.

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
| `dhcpClient` | `{ hostname?, clientId?, setBroadcastFlag = false }` | — | DHCPv4 client (D-050); present = enabled; not with `unnumbered` |
| `subinterfaces.*.{enabled, description, mtu, ipv4, ipv6, vrf, unnumbered, dhcpClient}` | as above | | |

Semantic: `interfaces.vrf-exists`, `interfaces.address-no-overlap` (both families, per VRF, across parents and
sub-interfaces; several addresses of one subnet on one interface are allowed, the same address twice is not),
`interfaces.vlan-unique` (tag combination per parent), `interfaces.unnumbered-target-exists`,
`interfaces.subinterface-mtu` (≤ parent MTU when both set), `interfaces.mac-unique`.

## `vrfs`

Record `objectName` → `{ id: uint32 (required), description? }`. `default` = table 0, implicit; declared only with `id: 0`.

Semantic: `vrfs.id-unique`, `vrfs.default-is-table-zero`.

## `routing`

`{}` is valid; each protocol object is **absent when disabled**. Shapes follow the FRR renderer contract of P12 §2
(D-045); prefix lists / route maps are objects rather than bare arrays because the document is projected 1:1 onto
protobuf, whose map values cannot be `repeated` (P02a-contract.md).

| field | type | notes |
|---|---|---|
| `static[]` | `{ prefix: ipNetwork, vrf = default, nextHops[] = []: { address?, interface?, weight = 1 }, blackhole = false, distance = 1, description? }` | `itemKey: [vrf, prefix]`; ≥ 1 next hop unless `blackhole` (then none); hop needs address or interface; hop family = prefix family |
| `policy.prefixLists` | record → `{ description?, family = ipv4, rules[]: { seq ≥ 1, action, prefix, ge?, le? } }` | unique `seq`; prefixes match `family`; FRR ranges `len < ge ≤ le ≤ 32\|128`, `le ≥ len` |
| `policy.routeMaps` | record → `{ description?, entries[]: { seq, action, description?, match {…}, set {…} } }` | unique `seq`; `match`: prefixList, nextHopPrefixList, interface, community, asPath (FRR regex alphabet only), metric, tag; `set`: **localPref, med**, weight, nextHop, community[], communityAdditive, asPathPrepend[], tag |
| `bgp` | `{ asn, routerId?, vrf, peerGroups{name → peer}, neighbors{address → neighbour}, networks[], redistribute{source → {metric?, routeMap?}}, gracefulRestart = false, ebgpRequiresPolicy = true }` | peer: `remoteAs`, `description`, `updateSource`, `ebgpMultihop`, `passwordRef` (`password/<name>`), timers (hold > keepalive), `bfd`, `afi{ ipv4Unicast?, ipv6Unicast? → { enabled, routeMapIn/Out, prefixListIn/Out, nextHopSelf, softReconfig, maximumPrefixes, defaultOriginate } }`; neighbour adds `peerGroup`, `shutdown`; needs `remoteAs` or `peerGroup` |
| `ospf` | `{ routerId?, vrf, areas{id → { type, noSummary }}, interfaces{name → { area, cost, passive, networkType, hello/dead, priority, bfd }}, redistribute{…}, defaultInformationOriginate }` | area ids are strings, decimal or dotted (`"0"` = `"0.0.0.0"`, unique after normalisation) |
| `isis` | `{ net (NET), level = level-1-2, vrf, interfaces{name → { passive, metric, circuitType, networkType, bfd }}, redistribute{…} }` | |
| `rip` | `{ vrf, networks[]: ipv4Network, interfaces{name → { passive }}, redistribute{…}, defaultMetric = 1 }` | |
| `bfd` | `{ sessions[]: { interface, localAddress, peerAddress, desiredMinTxUs = 300000, requiredMinRxUs = 300000, detectMultiplier = 3, enabled = true } }` | VPP native BFD; addresses in one family |

`redistribute` sources: `connected static bgp ospf isis rip` minus the protocol itself.

Semantic: `routing.vrf-exists`, `routing.static-nexthop-interface-exists`, `routing.static-unique` (canonical prefix),
`routing.prefix-list-exists`, `routing.route-map-exists`, `routing.bgp-peer-group-exists`,
`routing.bgp-neighbor-unique` (canonical address), `routing.network-unique` (BGP and RIP networks),
`routing.bfd-session-unique` (interface + canonical peer), `routing.interface-exists` (route-map match, BGP update
source, OSPF/IS-IS/RIP/BFD interfaces), `routing.ospf-area-exists`.

## `management`

| field | type | default | notes |
|---|---|---|---|
| `users[]` | `{ username, role: admin\|operator\|readonly, scope: '*', passwordHash?, sshKeys[], fullName?, disabled }` | `[]` | `itemKey: username`; `scope` literal `*` (vdom.md #3); `passwordHash` write-only |
| `aaa.order[]` | `local \| radius \| tacacs` | `['local']` | unique; listed methods need servers (refine) |
| `aaa.radius.servers[]` | `{ address, authPort = 1812, acctPort = 1813, secretRef (psk/…), timeoutSec = 5, vrf }` | `[]` | unique (address, authPort) |
| `aaa.tacacs.servers[]` | `{ address, port = 49, secretRef (psk/…), timeoutSec = 5, vrf }` | `[]` | unique (address, port) |
| `tls` | `{ certificateRef? (cert/…), privateKeyRef? (key/…), minVersion = 1.2 }` | | cert and key together (refine); absent = self-signed |
| `syslog[]` | `{ address, port = 514, protocol = udp, severity = info, vrf }` | `[]` | unique (address, port, protocol) |

Semantic: `management.admin-exists` (when `users` is non-empty: an enabled admin with a password or SSH key — an
empty list is valid, the API seeds the first admin, D-048), `management.username-unique`, `management.server-unique`,
`management.vrf-exists`.

## Examples (`packages/schema/examples/`)

Group (a) owns `minimal.json` (`{}` — the empty document is valid), `two-interfaces.json`, `group-a-full.json` (every
group (a) domain exercised: DNS, dataplane, VRFs, sub-interfaces, DHCP client, static/blackhole routes, policy, BGP,
OSPF, BFD, users, AAA, TLS, syslog), `invalid-*.json` (schema rejects) and `invalid-semantic-*.json` (schema accepts,
a validator reports the pointer listed in `src/examples.test.ts`). `examples.test.ts` claims only these; the other
groups' files are tested by their own suites, and a file owned by no group fails the suite. Valid examples contain no
secret leaf: they are also the corpus of the protobuf drift tests (`packages/proto/test`, `apps/agent/internal/contracttest`).
