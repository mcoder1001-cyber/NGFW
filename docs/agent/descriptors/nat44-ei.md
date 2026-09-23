# nat44-ei descriptors (DF-3)

Package `apps/agent/internal/descriptors/nat44ei`, binapi `apps/agent/binapi/nat44_ei` (plugin `nat44_ei_plugin.so`,
loaded on vrx-a). Entry point `nat44ei.Register(registry, client, owner)`. Carrier and ownership rules as in
`nat44-ed.md` (`natcommon`). **nat44-ei and nat44-ed are mutually exclusive on one VPP** — the integration test skips
when ED is enabled and serialises with this slot's ED test on `/run/vrx-test/w<N>/nat44.lock`.

| Descriptor | Key id | Create / Delete | Update | Retrieve | Dependencies | Notes / limitations |
|---|---|---|---|---|---|---|
| `nat44-ei.enable` | `global` | `nat44_ei_plugin_enable_disable` (flags static-mapping-only / connection-tracking / out2in-dpo, inside/outside vrf) | `ErrRecreate`; refused (`ErrForeignObjects`) when foreign objects exist | `nat44_ei_show_running_config` (`sessions == 0` ⇔ disabled) | optional `vrf/<inside>`, `vrf/<outside>` | Global singleton, read-first, converged when already enabled with the same config. The API enable carries no session counts (startup.conf / CLI in nat44-ei) → not modelled. |
| `nat44-ei.timeouts` | `global` | `nat44_ei_set_timeouts`; Delete restores 300/7440/240/60 | in place | running config `timeouts` (object only when non-default) | enable | Presence = non-default values. |
| `nat44-ei.forwarding` | `global` | `nat44_ei_forwarding_enable_disable` | — (empty spec) | running config `forwarding_enabled` (object only when on) | enable | Presence = enabled. |
| `nat44-ei.ipfix` | `global` | `nat44_ei_ipfix_enable_disable` (enable / disable) | in place | running config `ipfix_logging_enabled` + cached domain/src-port | enable | VPP reports only on/off; `domain_id`/`src_port` are write-only → Retrieve echoes the last values this process set (after a restart they read as 0 until re-applied). The IPFIX exporter object is DF-8's. |
| `nat44-ei.interface-feature` | `<interface>/<inside\|outside>` | `nat44_ei_interface_add_del_feature` (`NAT44_EI_IF_INSIDE`/`_OUTSIDE`) | recreate | `nat44_ei_interface_dump` | enable, `interface/<name>` | |
| `nat44-ei.output-feature` | `<interface>` | `nat44_ei_add_del_output_interface` | recreate | `nat44_ei_output_interface_get` (cursor, `EAGAIN` continues) | enable, `interface/<name>` | VPP refuses in2out and output features on the same interface. The legacy per-side `nat44_ei_interface_add_del_output_feature` / `_output_feature_dump` pair is a shim that does not report the interface back → not used. |
| `nat44-ei.interface-address` | `<interface>` | `nat44_ei_add_del_interface_addr` | recreate | `nat44_ei_interface_addr_dump` | enable, `interface/<name>` | |
| `nat44-ei.address-pool` | `<first>-<last>/<vrf>` | `nat44_ei_add_del_address_range` | recreate | `nat44_ei_address_dump`, merged into contiguous ranges per VRF | enable, optional `vrf/<id>` | Adjacent pools of one VRF merge (see nat44-ed). |
| `nat44-ei.static-mapping` | `<name>` (tag `<owner>:<name>`) | `nat44_ei_add_del_static_mapping` (`NAT44_EI_ADDR_ONLY_MAPPING`, `external_sw_if_index`) | recreate | `nat44_ei_static_mapping_dump`, tag filtered | enable, optional `vrf`, `interface/<external>` | No twice-NAT / out2in-only / lb in EI. |
| `nat44-ei.identity-mapping` | `<name>` (tag) | `nat44_ei_add_del_identity_mapping` | recreate | `nat44_ei_identity_mapping_dump` | enable, optional `vrf`, `interface` | |

Retrieve-only / actions: `Users` (`nat44_ei_user_dump`), `UserSessions` (`nat44_ei_user_session_v2_dump`, per user, paged),
`DeleteSession` (`nat44_ei_del_session`). Not used: HA (`nat44_ei_ha_*`), `nat44_ei_set_addr_and_port_alloc_alg`,
`nat44_ei_set_workers`, `nat44_ei_set_mss_clamping`, `nat44_ei_set_log_level`, `nat44_ei_interface_add_del_output_feature` /
`nat44_ei_interface_output_feature_dump` (legacy shim), `nat44_ei_del_user`.

Tests: `nat44ei_test.go` (fake, shared with owner `w3`), `nat44ei_integration_test.go` (loopbacks `loop903–905`,
table `9002`, pool `10.9.3.1–2`, tags `w9:*`, plugin disabled again in Cleanup when this test enabled it).
