# Schema contract — group (b): `nat`, `objects`, `acl`

Part of `contracts-v1` (index: `docs/contracts/schema.md`, written by P02a). Source of truth:
`packages/schema/src/domains/{nat,objects,acl}.ts` (tier a — shape and single-field formats) and
`packages/schema/src/semantic/{nat,objects,acl}.ts` (tier b — cross-field rules returning `{ pointer, message }[]`).
Generated: `dist/json-schema/{nat,objects,acl}.json`, OpenAPI components `NatConfig`, `ObjectsConfig`, `AclConfig`.

**Changing this package requires a `contract/<id>` branch with a `contract(schema): …` commit** (enforced by
`tools/ci.sh --base main`). Adding fields/rules as designed in `docs/04-api-datamodel.md` is an ordinary contract
branch; renaming or reshaping an existing field is always a PENDING decision (`docs/decisions/decision-policy.md` #1).

Conventions shared by the three domains:

- Every field goes through `withUi()` → `title` + `x-vrx-ui { widget, group, order, help }` in the JSON Schema.
- Every domain accepts `{}` (root `prefault`, D-017); lists default to `[]`, records to `{}`, flags to `false`
  (`enabled` on ACL rules/attachments defaults to `true`), so a parsed document is fully populated and `diff()` never
  sees "absent vs empty" noise.
- Records are keyed by `objectName` (`^[A-Za-z0-9][A-Za-z0-9_.-]*$`, ≤ 63). Names are unique **within their record
  only** (vdom.md #2). API paths: `/config/nat/pools/<i>`, `/config/objects/<kind>/<name>`, `/config/acl/lists/<name>`.
- `vrf` is first-class wherever VPP scopes an object by FIB (vdom.md #1): NAT pools, mappings, BIBs, DET44 sides and
  ACL attachments. `default` always exists (VRF 0 is *named*, never implied numerically); every other name must be a
  key of `vrfs`.
- Interface references are VPP names (`vppInterfaceName`); a sub-interface `parent.N` resolves either as its own key
  in `interfaces` or as `interfaces[parent].subinterfaces[N]`.
- Local primitives (P02a owns `primitives.ts`; these live in the domain files and are candidates to move — see
  `docs/status/tasks/P02b-questions.md`): `ipPrefix` (v4|v6 CIDR), `l4PortNumber` (1–65535), `l4PortRange`
  (`"443"` | `"8000-8080"`), `timeOfDay` (`HH:MM`), `hexColor`, `ipv4AddressRange` (`"a.b.c.d[-a.b.c.e]"`),
  `ruleSequence` (1–2³¹−1), `linuxInterfaceName` (≤ 15). `ipv4Address`/`ipv6Address` are private to `nat.ts` to
  avoid an `export *` clash with P02a's primitives.

---

## `nat`

The top level of `nat` **is NAT44** (docs/04: `mode, inside, outside, static, portForwards, cgnat`; F-nat44-ed-sessions
§Contract: `mode, inside, outside, pools, staticMappings, timeouts, sessionLimit`). Other translators are siblings so
the API path stays `/config/nat/<translator>/…`. Docs/04's `static` + `portForwards` are one list, `staticMappings`
(a mapping without ports is 1:1, with ports it is a port forward — VPP's own model); `cgnat` is `det44` + `ipfix`.

| field | type | default | notes |
|---|---|---|---|
| `enabled` | boolean | `false` | NAT44 plugin enabled |
| `mode` | `ed` \| `ei` | `ed` | `nat44-ed` (endpoint-dependent) or `nat44-ei`; ED-only features are rejected in EI (`nat.mode-ed-features`) |
| `inside[]`, `outside[]`, `outputFeature[]` | `vppInterfaceName[]` ≤ 1024 | `[]` | interface NAT features; output-feature = post-routing path |
| `insideVrf?`, `outsideVrf?` | `objectName` | — | plugin-wide inside/outside FIBs |
| `forwarding`, `staticMappingOnly`, `connectionTracking` | boolean | `false` | plugin flags |
| `sessionLimit?` | int 1024–2³¹−1 | — | per worker; omit = VPP default |
| `pools[]` | **union** — range pool `{ name, description?, range: ipv4AddressRange, vrf?, twiceNat }` \| interface pool `{ name, description?, interface: vppInterfaceName, twiceNat }` ≤ 1024 | `[]` | discriminated by which key is present (both variants strict, no `kind` tag so the F-nat44 shape `{ name, range, vrf? }` stays verbatim). Range → `nat44_add_del_address_range`; interface → `nat44_add_del_interface_addr {sw_if_index, flags}` (the address VPP finds on the interface, follows DHCP; no `vrf` — the interface's FIB applies) |
| `staticMappings[]` | `{ name, description?, protocol?: tcp\|udp\|icmp, local { ip: ipv4, port? }, external { ip? \| pool? \| interface?, port? }, vrf?, twiceNat, selfTwiceNat, out2inOnly }` ≤ 65536 | `[]` | 1:1 (no ports) or port forward (ports + protocol); `external` names **exactly one** of a literal address, a pool from `nat.pools` (a range pool contributes its **start** address, an interface pool its interface → `external_sw_if_index`) or an interface whose address is used |
| `identityMappings[]` | `{ description?, ip? \| interface?, protocol?, port?, vrf? }` ≤ 4096 | `[]` | never-translate entries |
| `loadBalancedMappings[]` | `{ name, description?, protocol: tcp\|udp, external { ip, port }, locals[] { ip, port, probability 1–255, vrf? } (1–256), affinity, twiceNat, selfTwiceNat, out2inOnly }` ≤ 4096 | `[]` | ED only |
| `timeouts` | `{ udp 300, tcpEstablished 7440, tcpTransitory 240, icmp 60 }` seconds | VPP defaults | |
| `ipfix` | `{ enabled, domainId? 1–2³²−1, sourcePort? }` | disabled | NAT logging (CGNAT logging, D4.4) |
| `nat64` | `{ enabled, inside[], outside[], prefixes[] { prefix: ipv6Cidr, vrf? }, pools[] { range, vrf? }, staticBibs[] { protocol, inside { ip6, port }, outside { ip4, port }, vrf? }, timeouts }` | disabled | D4.3 |
| `nat66` | `{ enabled, inside[], outside[], staticMappings[] { local: ip6, external: ip6, vrf? } }` | disabled | D4.3 |
| `nptv6` | `{ bindings[] { interface, internal: ipv6Cidr, external: ipv6Cidr } }` ≤ 1024 | `[]` | RFC 6296, D4.3 |
| `det44` | `{ enabled, inside[], outside[], insideVrf?, outsideVrf?, mappings[] { inside: ipv4Cidr, outside: ipv4Cidr } ≤ 1024, timeouts }` | disabled | deterministic CGNAT, D4.4 |
| `dslite` | `{ enabled, aftr? { ipv6, ipv4? }, b4? { ipv6, ipv4? }, pools[] }` | disabled | D4.5 |
| `map` | `{ interfaces[] { interface, mode: map-e\|map-t } ≤ 1024, domains[] { name, mode: map-e\|map-t\|lw4o6, ipv4Prefix, ipv6Prefix, ipv6Source, eaBitsLength 0–64, psidOffset 0–16, psidLength 0–16, mtu? 1280–9216, rules[] { psid, ipv6Destination } } ≤ 4096, parameters { fragmentation, icmpSourceAddress?, icmp6Unreachables, securityCheck, tcpMss?, trafficClass, preResolve } }` | `[]` | MAP-E/MAP-T/lw4o6, D4.5. `interfaces[]` = VPP `map_if_enable_disable {sw_if_index, is_enable, is_translation}` — MAP processes packets only on bound interfaces; `mode: map-t` ⇔ `is_translation` (domains are shared, `map-e` also serves lw4o6 domains). `map_add_domain` has **no** mode field: a domain's `mode` fixes the meaning of `ipv6Source` — `map-e`/`lw4o6` → BR address `/128` (encapsulation source), `map-t` → DMR prefix `/64` or `/96` (VPP `ip4_map_t_embedded_address`). 464XLAT = a `nat64` PLAT + a MAP-T CLAT |
| `cnat` | `{ translations[] { name, protocol: tcp\|udp, vip { ip, port }, backends[] { ip, port } (1–1024), lbType: default\|maglev } ≤ 65536, snat { policy: none\|interface\|k8s, addresses { ipv4?, ipv6?, interface? }, interfaces[] { interface, table: include-v4\|include-v6\|pod\|host }, excludePrefixes[] } }` | `[]` | D4.6. `snat.interfaces[].table` = VPP `cnat_snat_policy_table` (`cnat_snat_policy_add_del_if.table`; one entry per (interface, table) pair); policy `interface` consults `include-v4`/`include-v6`, policy `k8s` consults `pod`/`host`. `addresses.interface` = `cnat_set_snat_addresses.sw_if_index` |

Semantic rules (`semantic/nat.ts`; pointer = the offending element):

| rule | rejects | pointer |
|---|---|---|
| `nat.interfaces-exist` | any inside/outside/outputFeature/binding/external interface not in `interfaces` | `/nat/inside/<i>`, `/nat/staticMappings/<i>/external/interface`, `/nat/nptv6/bindings/<i>/interface`, … |
| `nat.inside-outside-disjoint` | an interface listed twice or on both sides of nat44/nat64/nat66/det44 | `/nat/outside/<i>`, `/nat/<translator>/outside/<i>` |
| `nat.pools-valid` | range end < start; overlapping ranges **of the same twice-NAT class** within `pools` (VPP keeps normal and twice-NAT addresses in separate lists), within `nat64.pools`, `dslite.pools`; the same interface twice as an interface pool of one class; duplicate pool names. Interface pools take no part in the overlap check (VPP resolves them at run time) | `/nat/pools/<i>/{range,interface,name}` |
| `nat.static-mappings` | external not exactly one of ip/pool/interface; unknown pool; one-sided ports; **ports without `protocol`**; ports on ICMP; `twiceNat`+`selfTwiceNat`; duplicate names; duplicate external tuple (protocol, address, port, vrf) — `pool` is resolved to its start address / interface first, so `{ ip: X }` and `{ pool: p }` with `p` starting at `X` collide | `/nat/staticMappings/<i>/{external,external/pool,local/port,external/port,protocol,selfTwiceNat,name}` |
| `nat.identity-mappings` | not exactly one of ip/interface; port without protocol; port on ICMP | `/nat/identityMappings/<i>[/protocol]` |
| `nat.load-balanced-mappings` | duplicate names / external endpoints / local endpoints | `/nat/loadBalancedMappings/<i>/{name,external,locals/<j>}` |
| `nat.mode-ed-features` | twice-NAT pools or mapping flags, `out2inOnly`, load balancing while `mode = ei` | the flag / `/nat/loadBalancedMappings/<i>` |
| `nat.vrfs-exist` | any `vrf`/`insideVrf`/`outsideVrf` not in `vrfs` (`default` implied) | the `vrf` field |
| `nat.nat64-valid` | prefix length ∉ {32,40,48,56,64,96}; second prefix for one VRF; duplicate BIB inside/outside tuples | `/nat/nat64/prefixes/<i>[/prefix]`, `/nat/nat64/staticBibs/<i>/{inside,outside}` |
| `nat.nptv6-valid` | internal/external lengths differ; two bindings on one interface | `/nat/nptv6/bindings/<i>/{external,interface}` |
| `nat.det44-valid` | outside prefix shorter than inside; sharing ratio > 2¹⁵; overlapping inside/outside prefixes | `/nat/det44/mappings/<i>/{outside,inside}` |
| `nat.map-valid` | duplicate names; ipv6Prefix + EA bits > 64; psidOffset + psidLength > 16; rules with EA bits; PSID ≥ 2^psidLength; duplicate PSIDs; `mode` vs `ipv6Source` length (`map-e`/`lw4o6` ⇒ `/128`, `map-t` ⇒ `/64` or `/96`); an interface bound to MAP twice | `/nat/map/domains/<i>/{name,eaBitsLength,psidLength,rules,rules/<j>/psid,ipv6Source}`, `/nat/map/interfaces/<i>/interface` |
| `nat.cnat-valid` | duplicate names/VIPs; backend family ≠ VIP family; duplicate backends; an interface twice in one SNAT table; a table the selected `snat.policy` never consults (`interface` → include-v4/include-v6, `k8s` → pod/host; `none` accepts any) | `/nat/cnat/translations/<i>/{name,vip,backends/<j>[/ip]}`, `/nat/cnat/snat/interfaces/<i>[/table]` |
| `nat.dslite-valid` | AFTR and B4 both set; enabled with neither | `/nat/dslite/{b4,enabled}` |
| `nat.prefixes-are-networks` | a prefix whose address has host bits set, in every field VPP treats as a *network*: `nat64.prefixes[].prefix`, `nptv6.bindings[].{internal,external}`, `det44.mappings[].{inside,outside}`, `map.domains[].{ipv4Prefix,ipv6Prefix,ipv6Source}`, `cnat.snat.excludePrefixes[]` (`ipv4Cidr`/`ipv6Cidr` allow host bits because interface addresses need them; the message names the network, e.g. `'100.64.0.1/16' has host bits set; the network is 100.64.0.0/16`) | the prefix field |

`sessionLimit ≥ 1024` is a schema rule (`minimum: 1024`). Behaviour for outbound traffic matching no pool is VPP's
(`forwarding` flag) — not modelled beyond the flag.

Documented semantics that are **not** rules:

- `enabled: false` with pools/interfaces/mappings present is legal and means *NAT44 administratively down*: the
  renderer disables the plugin and renders none of the NAT44 objects (they stay in the document). The F-nat44 contract
  has no `enabled`; a document written verbatim to it must add `enabled: true` or nothing is applied. Whether to add a
  rule `nat.enabled-consistency` is left to the F-nat44 worker (see `docs/status/tasks/P02b-questions.md`).
- One interface on both `inside` and `outside` (one-armed NAT, VPP `NAT_IS_INSIDE|NAT_IS_OUTSIDE`) is rejected by
  `nat.inside-outside-disjoint` per F-nat44 §1; an additive `both[]` list can lift that later.
- `nat64.pools[]` and `dslite.pools[]` are range-only; the interface-address form (`nat64_add_del_interface_addr`)
  is not modelled yet (additive union later, same pattern as `nat.pools`).

---

## `objects`

Reusable firewall objects referenced by ACL rules (and later NAT policies). Every kind is a record keyed by name.
`fqdn` addresses are resolved by the agent when rendered (VPP ACLs match prefixes); an unresolvable FQDN matches nothing.

| record | value | notes |
|---|---|---|
| `addresses` | `{ type: host, address }` \| `{ type: network, prefix }` \| `{ type: range, start, end }` \| `{ type: fqdn, fqdn }`, each `+ description?, tags[]` | v4 or v6 |
| `addressGroups` | `{ members[] (1–4096), description?, tags[] }` | members are addresses or address groups |
| `services` | discriminated on `protocol`: `tcp`\|`tcp-udp` `{ destinationPorts[], sourcePorts[], tcpFlags? { mask, value } }`; `udp`\|`sctp` `{ destinationPorts[], sourcePorts[] }`; `icmp`\|`icmp6` `{ type?, code? }`; `any`; `other { number 0–255 }`, each `+ description?, tags[]` | ports are `l4PortRange` strings, ≤ 64 per list, empty = any |
| `serviceGroups` | `{ members[] (1–4096), description?, tags[] }` | members are services or service groups |
| `schedules` | `{ type: recurring, days[] (mon…sun, 1–7), start: HH:MM, end: HH:MM }` \| `{ type: once, start, end }` (RFC 3339 with offset), `+ description?, tags[]` | overnight window = two schedules |
| `zones` | `{ interfaces[] ≤ 1024, description?, tags[] }` | an ACL attachment may target a zone |
| `tags` | `{ description?, color?: #rrggbb }` | |

`ServiceSpecSchema` (the `services` value without `description`/`tags`) is what ACL rules embed inline.

| rule | rejects | pointer |
|---|---|---|
| `objects.names-disjoint` | a name present in both `addresses` and `addressGroups`, or `services` and `serviceGroups` (ACL rules reference either through one `name`) | `/objects/addressGroups/<name>`, `/objects/serviceGroups/<name>` |
| `objects.tags-exist` | a `tags[]` entry not in `objects.tags` (all six tagged kinds) | `/objects/<kind>/<name>/tags/<i>` |
| `objects.address-range-valid` | `range` with mixed families or end < start | `/objects/addresses/<name>/end` |
| `objects.address-group-members`, `objects.service-group-members` | unknown member; member listed twice; membership cycle (reported at the member that closes it) | `/objects/<kind>/<name>/members/<i>` |
| `objects.service-valid` | ICMP `code` without `type`; TCP flag `value` bits outside `mask`; overlapping port ranges in one list | `/objects/services/<name>/{code,tcpFlags/value,destinationPorts/<i>,sourcePorts/<i>}` |
| `objects.schedule-valid` | `end ≤ start` (recurring: same day; once: instant); a weekday listed twice | `/objects/schedules/<name>/{end,days/<i>}` |
| `objects.zone-interfaces` | unknown interface; interface listed twice; interface already in another zone | `/objects/zones/<name>/interfaces/<i>` |

All lookups are prototype-safe (`Object.hasOwn`): `constructor`, `toString` … are legal object names.

---

## `acl`

| field | type | notes |
|---|---|---|
| `lists` | record → `{ description?, tags[], rules[] }`, rule = `{ sequence, description?, enabled = true, action: permit\|deny\|reflect, ipVersion: ipv4\|ipv6\|any = any, source, destination: AddressMatch = any, service: ServiceMatch = any, schedule?, log = false }` | VPP `acl` plugin; `reflect` = permit + reflexive state |
| `macip` | record → `{ description?, tags[], rules[] }`, rule = `{ sequence, description?, action: permit\|deny, sourceMac, sourceMacMask = ff:ff:ff:ff:ff:ff, sourcePrefix? }` | VPP MACIP ACL, input only |
| `host` | record → `{ description?, tags[], rules[] }`, rule = `{ sequence, description?, enabled, action: accept\|drop\|reject, ipVersion, source, destination, service, interface?: linuxInterfaceName, log }` | management-plane nftables (D5.3) |
| `attachments[]` | `{ list, target: { kind: interface, interface } \| { kind: zone, zone }, direction: in\|out = in, sequence, vrf?, enabled = true, description? }` | order among lists on one target+direction = `sequence` |
| `macipAttachments[]` | `{ list, interface, vrf?, enabled, description? }` | one MACIP ACL per interface |
| `hostAttachments[]` | `{ list, chain: input\|output\|forward, priority −500…500 = 0, enabled, description? }` | |

`AddressMatch` = `{ kind: any }` \| `{ kind: prefix, prefix }` \| `{ kind: object, name }` (addresses ∪ addressGroups);
`ServiceMatch` = `{ kind: any }` \| `{ kind: object, name }` (services ∪ serviceGroups) \| `{ kind: inline, spec: ServiceSpec }`.
Hit counters are state (`/state/…`), not config.

| rule | rejects | pointer |
|---|---|---|
| `acl.rule-sequences-unique` | the same `sequence` twice in one list (lists, macip, host) | `/acl/<kind>/<name>/rules/<i>/sequence` |
| `acl.rule-references` | source/destination/service object or `schedule` not found in `objects` | `/acl/<kind>/<name>/rules/<i>/{source/name,destination/name,service/name,schedule}` |
| `acl.rule-consistency` | prefix family ≠ `ipVersion`; mixed source/destination families; `icmp` in an IPv6 rule or `icmp6` in an IPv4 rule; inline spec failing the service checks | `…/rules/<i>/{source/prefix,destination/prefix,service/spec/…}` |
| `acl.tags-exist` | list `tags[]` entry not in `objects.tags` | `/acl/<kind>/<name>/tags/<i>` |
| `acl.macip-rules` | source MAC with bits outside `sourceMacMask` | `/acl/macip/<name>/rules/<i>/sourceMac` |
| `acl.attachments` | unknown list/interface/zone/VRF; `vrf` ≠ the VRF declared by a target interface (or any zone member); same list twice on one target+direction; two attachments sharing a `sequence` on one target+direction — **zones are expanded to their member interfaces first**, so a list attached via a zone and again via one of its member interfaces (or a shared sequence that way) is rejected here rather than at apply time | `/acl/attachments/<i>/{list,target/interface,target/zone,vrf,sequence}` |
| `acl.macip-attachments` | unknown list/interface/VRF; VRF mismatch; a second MACIP list on one interface | `/acl/macipAttachments/<i>/{list,interface,vrf}` |
| `acl.host-attachments` | unknown list; same list twice on one chain | `/acl/hostAttachments/<i>/{list,chain}` |

---

## Examples (`packages/schema/examples/`)

| file | purpose |
|---|---|
| `nat-basic.json` | NAT44-ED: inside/outside, a range pool and an interface-address pool, 1:1, port forward, `external.pool`, `external.interface`, identity mapping, timeouts, session limit |
| `nat-cgnat.json` | DET44 + IPFIX, NAT64 (prefix, pool, BIB), NPTv6, lw4o6 MAP domain with PSID rules bound to an interface (`map.interfaces`), CNAT VIP + SNAT policy with `include-v4`/`include-v6` tables |
| `objects-basic.json` | every object kind, including tags, ranges, FQDN, TCP flags, recurring + one-time schedules, zones with a sub-interface |
| `acl-basic.json` | L3/L4 lists (reflect, inline ICMP, default deny), MACIP list, host list, interface/zone attachments with VRFs |
| `invalid-{nat,objects,acl}-*.json` | rejected by the **schema** (tier a): session limit 1023, CIDR as pool range, a pool with both `range` and `interface`, the pre-M3 CNAT `side` key, unknown key, port `0`, `24:00`, bad object name, unknown action, bad direction |
| `{nat,objects,acl}-semantic-invalid-*.json` | accepted by the schema, rejected by **`validateSemantics`** with exactly the documented pointer (`src/semantic/nat-objects-acl-examples.test.ts`) |

## Mapping to the F-nat44-ed-sessions contract

`nat{ mode: "ed", inside, outside, pools: [{name, range, vrf?}], staticMappings: [{name, local{ip,port?},
external{ip|pool, port?}, protocol?, vrf?, twiceNat?}], timeouts{udp,tcpEstablished,tcpTransitory,icmp}, sessionLimit }`
is exactly the top level of `NatSchema` (plus the interface-pool variant `{name, interface}`, `external.interface`
and the additional flags, all optional/defaulted; **`enabled: true` is required for anything to be rendered**).
Semantic rules required by that feature — inside/outside exist and are disjoint, pools valid and non-overlapping,
external port requires protocol, session limit ≥ 1024 — are `nat.interfaces-exist`, `nat.inside-outside-disjoint`,
`nat.pools-valid`, `nat.static-mappings` and the schema `minimum`. "Overlapping pools → 400 with pointer" is
`/nat/pools/<i>/range` (see `nat-semantic-invalid-overlapping-pools.json`).
