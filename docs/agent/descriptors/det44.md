# det44 descriptors (DF-3)

Package `apps/agent/internal/descriptors/det44`, binapi `apps/agent/binapi/det44` (plugin `det44_plugin.so`, loaded on
vrx-a). Entry point `det44.Register(registry, client, owner)`. No "is enabled" getter → cache ∪ heuristic (any
det44 interface or map exists), as for nat64/nat66.

| Descriptor | Key id | Create / Delete | Update | Retrieve | Dependencies | Notes / limitations |
|---|---|---|---|---|---|---|
| `det44.enable` | `global` | `det44_plugin_enable_disable` (`inside_vrf`, `outside_vrf`; retval 1 "already" tolerated) | `ErrRecreate` (refused with `ErrForeignObjects` when foreign objects exist) | cache ∪ heuristic | optional `vrf/<inside>`, `vrf/<outside>` | VRFs have no getter: after an agent restart the heuristic reports zeros; a non-zero desired value plans one disable/enable cycle. |
| `det44.timeouts` | `global` | `det44_set_timeouts`; Delete restores 300/7440/240/60 | in place | `det44_get_timeouts` — object only when non-default | enable | Presence = non-default values. |
| `det44.interface` | `<interface>/<inside\|outside>` | `det44_interface_add_del_feature` (`is_inside`) | recreate | `det44_interface_dump` (`is_inside` / `is_outside`) | enable, `interface/<name>` | |
| `det44.map` | `<inside prefix>/<outside prefix>` | `det44_add_del_map` | recreate | `det44_map_dump` (sharing ratio / ports per host are derived, not part of the value) | enable | Ownership: inside or outside prefix inside the slot v4 block. |

Retrieve-only / actions: `Sessions(userIP, offset, limit)` over `det44_session_dump` (per user, paged),
`CloseSessionIn` (`det44_close_session_in`), `CloseSessionOut` (`det44_close_session_out`). Not used: the legacy
`nat_det_*` aliases (`nat_det_add_del_map`, `nat_det_map_dump`, `nat_det_session_dump`, `nat_det_close_session_*`,
`nat_det_forward`/`reverse`), `det44_forward` / `det44_reverse` (lookups, not state).

Tests: `det44_test.go` (fake), `det44_integration_test.go` (loopbacks `loop920/921`, map `10.9.44.0/24 → 10.9.45.0/30`;
plugin disabled again in Cleanup when this test enabled it).
