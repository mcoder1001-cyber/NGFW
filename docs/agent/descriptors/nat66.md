# nat66 descriptors (DF-3)

Package `apps/agent/internal/descriptors/nat66`, binapi `apps/agent/binapi/nat66` (plugin `nat66_plugin.so`, loaded
on vrx-a but **disabled by default** — `show nat66 …` prints "plugin disabled" until `nat66_plugin_enable_disable`).
Entry point `nat66.Register(registry, client, owner)`.

| Descriptor | Key id | Create / Delete | Update | Retrieve | Dependencies | Notes / limitations |
|---|---|---|---|---|---|---|
| `nat66.enable` | `global` | `nat66_plugin_enable_disable` (`outside_vrf`; already-enabled retval tolerated) | `ErrRecreate` (refused with `ErrForeignObjects` when foreign objects exist) | cache ∪ heuristic (any nat66 interface / mapping exists) | optional `vrf/<outside>` | No getter for `outside_vrf`: after an agent restart the heuristic reports `0`; a non-zero desired value then plans one disable/enable cycle. |
| `nat66.interface` | `<interface>/<inside\|outside>` | `nat66_add_del_interface` (`NAT_IS_INSIDE`/`_OUTSIDE`) | recreate | `nat66_interface_dump` — details carry only `NAT_IS_INSIDE`; flags 0 ⇒ outside | enable, `interface/<name>` | One VPP entry per interface (one side). Delete the nat66 interface before the interface itself: an entry whose interface is gone cannot be removed (retval −2) until a VPP restart. |
| `nat66.static-mapping` | `<local>/<vrf>` | `nat66_add_del_static_mapping` | recreate | `nat66_static_mapping_dump` (counters ignored) | enable, optional `vrf/<id>` | Ownership: local or external address in the slot v6 block, or vrf in range. |

Tests: `nat66_test.go` (fake), `nat66_integration_test.go` (loopbacks `loop912/913`, mapping `fd00:9::66 ↔
fd00:9::6600`; plugin disabled again in Cleanup when this test enabled it).
