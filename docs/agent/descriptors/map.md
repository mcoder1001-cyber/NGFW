# map descriptors (DF-3, D4.5: MAP-E / MAP-T / LW4o6)

Package `apps/agent/internal/descriptors/mapnat` (named `mapnat` because `map` is a Go keyword). Bindings are in
`apps/agent/binapi/map` (plugin `map_plugin.so`, loaded on vrx-a) plus `binapi/feature` for `feature_is_enabled`.
Entry point: `mapnat.Register(registry, client, owner)`. The desired-state carrier is `*structpb.Struct`, built from
the typed specs (`DomainSpec`, `RuleSpec`, `ParamsSpec`, `InterfaceSpec`) with `natcommon.Encode` (D-055 stand-in
until P03b adds NAT protos).

| Descriptor | Key id | Create / Delete | Update | Retrieve | Dependencies | Notes / limitations |
|---|---|---|---|---|---|---|
| `map.domain` | `<name>` (tag `<owner>:<name>`) | `map_add_domain` (ip4/ip6 prefix, ip6_src, ea_bits_len, psid_offset, psid_length, mtu, tag) → Meta `DomainMeta{Index}` / `map_del_domain(index)` | recreate (VPP has no modify) | `map_domain_dump`, filtered by owner tag | none | Prefixes are masked and canonicalised. `map_add_domain` in 26.06 has no flags field; the details' `flags` (MAP_DOMAIN_PREFIX, computed by VPP) is not modelled. MAP-E vs MAP-T is chosen per interface. Untagged and foreign-tagged domains are invisible. |
| `map.rule` | `<domain>/<psid>` | `map_add_del_rule(index, psid, ip6_dst)`; the domain index is resolved through the domain dump | destination change in place (re-add of the same PSID overwrites) | `map_rule_dump(domain_index)` for every owned domain, sorted by (domain, psid) | `map.domain/<domain>` | LW4o6 is a MAP-E domain with `ea_bits_len = 0` plus per-PSID rules. VPP rejects rules on domains with EA bits and PSIDs ≥ 2^psid_length (retval −1). |
| `map.params` | `global` | `map_param_set_fragmentation`, `_icmp`, `_icmp6`, `_security_check`, `_traffic_class`; Delete restores VPP's defaults (security-check on, tc-copy on, everything else off/0) | in place (same setters) | `map_param_get`; the object exists only when the values are non-default | none | Global singleton. `map_param_set_tcp` (TCP MSS) is write-only in 26.06 (not returned by `map_param_get`), so it is not modelled. The pre-resolve next hops (`map_param_add_del_pre_resolve`) are not modelled either (`map_param_get` always returns 0 for them, see the FIXME in map_api.c). |
| `map.interface` | `<interface>/<map-e\|map-t>` | `map_if_enable_disable(sw_if_index, is_enable, is_translation)` | recreate | No MAP interface dump exists: `feature_is_enabled` on arc `ip4-unicast` for `ip4-map` (MAP-E) / `ip4-map-t` (MAP-T), probed on owned interfaces only | `interface/<name>` | VPP keeps encap and translation in two separate bitmaps, so one interface can carry both; hence the mode is part of the key. |

Ownership: domains are identified by the owner tag, rules by their domain, and interfaces by the interface's owner
tag (`natcommon.Scope`). The params singleton is global, so a test slot touches it only when it is at the defaults.

Tests: `mapnat_test.go` covers the fake: create, idempotent re-apply, rule update in place, foreign and untagged
domains filtered, VPP errors, params presence semantics, and MAP-E plus MAP-T on one interface.
`mapnat_integration_test.go` runs on the host: domain `w9-lw` with `10.9.46.0/24` / `fd00:9:46::/48`, two rules,
MAP-E on `loop930` and MAP-T on `loop931`, and params set and then restored to the defaults.
