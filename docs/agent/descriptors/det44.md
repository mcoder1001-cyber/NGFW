# det44 descriptors (DF-3)

> **Ownership, globals (D-071), claims, unique keys and write-only re-application: see [nat-common.md](nat-common.md)** — it overrides older wording below where they differ.

Package `apps/agent/internal/descriptors/det44`, binapi `apps/agent/binapi/det44` (plugin `det44_plugin.so`, loaded on
vrx-a). Entry point `det44.Register(registry, client, owner)`. No "is enabled" getter → `det44.enable` is **write-only**
(`ErrRetrieveUnsupported`, D-063), as for nat64/nat66.

| Descriptor | Key id | Create / Delete | Update | Retrieve | Dependencies | Notes / limitations |
|---|---|---|---|---|---|---|
| `det44.enable` | `global` | Create `det44_plugin_enable_disable(enable=1)` (`inside_vrf`, `outside_vrf`; retval 1 "already" tolerated). **Delete never disables** (see below) | `ErrVRFChangeUnsafe` (VRF change needs a disable) | `ErrRetrieveUnsupported` (write-only) | optional `vrf/<inside>`, `vrf/<outside>` | No getter for "enabled" or the VRFs. The re-apply on every resync is idempotent (retval 1 tolerated). |
| `det44.timeouts` | `global` | `det44_set_timeouts`; Delete restores 300/7440/240/60 | in place | `det44_get_timeouts` — object only when non-default | enable | Presence = non-default values. |
| `det44.interface` | `<interface>/<inside\|outside>` | `det44_interface_add_del_feature` (`is_inside`) | recreate | `det44_interface_dump` (`is_inside` / `is_outside`) | enable, `interface/<name>` | |
| `det44.map` | `<inside prefix>/<outside prefix>` | `det44_add_del_map` | recreate | `det44_map_dump` (sharing ratio / ports per host are derived, not part of the value) | enable | Ownership: inside or outside prefix inside the slot v4 block. |

Retrieve-only / actions: `Sessions(userIP, offset, limit)` over `det44_session_dump` (per user, paged),
`CloseSessionIn` (`det44_close_session_in`), `CloseSessionOut` (`det44_close_session_out`). Not used: the legacy
`nat_det_*` aliases (`nat_det_add_del_map`, `nat_det_map_dump`, `nat_det_session_dump`, `nat_det_close_session_*`,
`nat_det_forward`/`reverse`), `det44_forward` / `det44_reverse` (lookups, not state).

**VPP 26.06 bugs (DF-3-questions.md Q0).** `det44_plugin_disable` iterates `vec_dup(dm->interfaces)`, but that
field is a pool, so freed slots are iterated too. The failed delete is then logged with `unformat_vnet_sw_interface`
used as a format function, which causes a SIGSEGV. As a result, disabling det44 after any det44 interface was ever
removed crashes VPP (this happened twice on vrx-a). `det44.enable` Delete therefore only releases the singleton in the
agent: the plugin stays enabled but idle until the next VPP restart, and a later Create finds it "already enabled".
A second bug: `det44_interface_add_del(is_del)` calls `vnet_feature_enable_disable(..., 1)`, so the
`det44-in2out/out2in` node stays on the interface after the det44 interface is deleted. The dump is correct, but the
feature node lingers until the interface is deleted.

Tests: `det44_test.go` (fake), `det44_integration_test.go` (loopbacks `loop920/921`, map `10.9.44.0/24 → 10.9.45.0/30`).
The integration test never disables det44, and it touches the det44 timeouts only when they are at VPP's defaults.
Per D-064 it is **opt-in** (`VRX_DF3_DET44=1`), because its first host run crashed the shared VPP (before the fix).
