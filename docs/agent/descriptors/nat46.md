# nat46 — NAT46 (IPv4 clients → IPv6-only servers) over MAP-T 1:1

Package `apps/agent/internal/descriptors/nat46` (F-nat46). **No descriptors of its own**: a projection
library over `mapnat` (DF-3, `map.md`). Reachability table: `library`.

## Design spike

VPP 26.06 has no NAT46 plugin (binapi NAT families: nat44_ed, nat44_ei, nat64, nat66, map, cnat, pnat).

**(a) Stateless SIIT (RFC 7915), 1:1 — feasible with the map plugin.** One MAP-T domain per mapping:

| NAT46 | `map_add_domain` field | value |
|---|---|---|
| IPv4 service address `A` | `ip4_prefix` | `A/32` |
| IPv6 server `S` | `ip6_prefix` | `S/128` |
| client prefix (RFC 6052) | `ip6_src` | the /96 (e.g. `64:ff9b::/96`) |
| — | `ea_bits_len`, `psid_offset`, `psid_length` | 0 |
| mtu | `mtu` | 0 = VPP default |
| name | `tag` | `<owner>:nat46-<name>` |

Interfaces: `map_if_enable_disable is_translation=1` (ip4-map-t + ip6-map-t) on the IPv4-facing and the
IPv6-facing interfaces. IPv4 → IPv6: dst `A` → `S` (EA bits 0, the whole /128), src `c` → `/96 + c`.
IPv6 → IPv4 reverses it. An IPv4 client `192.0.2.33` appears on the IPv6 side as `64:ff9b::c000:221`
(`nat46.ClientAddress`); the IPv6 server needs a route for the /96 back to the router.

Rules (`nat46.Validate`, paths relative to the NAT46 object): client prefix exactly /96, network address;
IPv4 unique unicast; IPv6 unique unicast and not inside the client prefix; names unique, no `# / :` or
spaces, ≤ 60 bytes with the prefix; MTU 0 or 1280..65535; interfaces required when mappings exist.

Not host-verified yet (cloud container, no VPP): whether 26.06 accepts `ip6_prefix` /128 with
`ea_bits_len 0` and translates both ways, and the interaction with `map.params` security check
(`TestNat46OnHost` answers the first; packet test is the host row's).

**(b) Stateful NAT46** (IPv4 pool with port sharing in front of IPv6 servers) has no VPP implementation.
Not built; surfaced in `docs/status/tasks/F-nat46-questions.md` (out-of-scope decision vs. VPP code track).

## Ownership

NAT46 domains live in the same `map.domain` tag space as `nat.map` domains; the `nat46-` name prefix
(`nat46.DomainPrefix`) separates them and `nat46.Assemble` picks only `nat46-*` domains of the 1:1 shape.
A `nat.map` domain must not use that prefix (rule for F-det44-map-dslite-cnat's builder — questions file).
MAP-T interfaces are shared: `map.interface/<if>/map-t` is one key whether NAT46 or `nat.map` wants it.

## API

`Project(Config) (Projection, error)`, `Validate(Config) []*FieldError`, `Assemble(domains, interfaces)`,
`IsNAT46Domain`, `ClientAddress`, `DomainName`.

## Wiring (F-nat46 part B)

`nat.nat46` (NatConfig 28) → `desired/nat46.go`, called from `desired.Nat` after `pnatBuild`:
`nat46.Validate` refusals (rule `nat.nat46-valid`, pointers under `/nat/nat46`), plus two cross-checks with
`nat.map` — a service /32 inside a MAP domain's `ipv4Prefix`, and a NAT46 interface bound `map-e` in `nat.map`.
The name guard holds both ways: `desired/map.go` refuses `nat46-*` MAP domains; the schema also refuses a MAP domain
named `nat46-<mapping>`.

Shared MAP-T interface: when `nat.map` binds the interface `map-t` too, only `nat.map`'s builder emits
`map.interface/<if>/map-t` (one object, no duplicate key). On Retrieve, `assembleNat46` (after `assembleMap`) splits
retrieved map-t interfaces with the owners of the **stored** desired state (`desired.SetNat46Owners`, called from the
service snapshot refresh — never from a projection): under `nat.nat46` if the stored nat46 lists it, under `nat.map`
if the stored `nat.map` lists it or nat46 does not. A shared interface therefore appears under both and a commit reads
back without false drift; a foreign map-t binding stays visible as `nat.map` drift.
