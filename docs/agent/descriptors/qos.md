# qos descriptors (DF-7, WBS D7.8)

Package `apps/agent/internal/descriptors/qos` — VPP QoS marking infrastructure. Messages only from
`apps/agent/binapi/qos`. Conventions common to DF-7 (structpb specs, logical interface names, claims, re-resolving
deletes, write-only rules) are described in `policer.md`.

| Object type | Key | Create / Update / Delete | Retrieve | Notes |
|---|---|---|---|---|
| `qos.record` | `qos.record/<if>/<ext\|vlan\|mpls\|ip>` | `qos_record_enable_disable` enable; Update = ErrRecreate; disable until the reference count is 0 | `qos_record_dump` | VPP counts enables per interface+source; Delete disables until VPP answers `VALUE_EXIST` |
| `qos.store` | `qos.store/<if>/ip` | `qos_store_enable_disable`; Update (new value) = ErrRecreate (VPP keeps the first value); disable until 0 | `qos_store_dump` | VPP 26.06 implements the `ip` source only (`UNIMPLEMENTED` otherwise → rejected by Validate) |
| `qos.egress-map` | `qos.egress-map/<id>` | `qos_egress_map_update` (create and update in place — marks keep working) / `qos_egress_map_delete` | `qos_egress_map_dump` | rows ext/vlan/mpls/ip, 256 entries each; an omitted row is all zeros (an explicit all-zero row is not canonical and rejected); ids attributed by `df7.WithIDRange` (tests: slot range; production: all) |
| `qos.mark` | `qos.mark/<if>/<source>` | `qos_mark_enable_disable` (enable with map; Update = enable with the new map, in place) / disable | `qos_mark_dump` | `VALUE_EXIST` on disable = already gone |

Dependencies: record/store → `interface/<if>`; mark → `interface/<if>` + `qos.egress-map/<id>` (mandatory: VPP
frees a map still referenced by a mark, so the scheduler removes marks first).

Ownership: record/store/mark belong to the owner of the interface (claim rule for untagged interfaces); egress
maps by id range. FIB entries: none.

Reference counts (review L1): `qos.record` / `qos.store` enables are counted by VPP. Create enables only when the dump
does not list the interface/source (an existing one is accepted only when tagged or claimed by us); Delete sends one
disable — our one reference — and leaves other consumers' references alone.
