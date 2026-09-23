# nat44-ed descriptors (DF-3)

Package `apps/agent/internal/descriptors/nat44ed`, binapi `apps/agent/binapi/nat44_ed` (plugin `nat_plugin.so`,
loaded on vrx-a). Entry point `nat44ed.Register(registry, client, owner)`; `nat44ed.New` returns the typed
descriptors for consumers that page state (`Users`, `UserSessions`, `DeleteSession`).

Desired state is carried as `*structpb.Struct` built from the typed specs below with `natcommon.Encode`
(one field per spec field, canonical IPs/protocols) until the NAT proto message lands
(F-nat44-ed-sessions contract). The F-nat44-ed-sessions object names map 1:1:
`nat44-enable`→`nat44-ed.enable`, `nat44-interface-feature`→`nat44-ed.interface-feature`,
`nat44-address-pool`→`nat44-ed.address-pool`, `nat44-static-mapping`→`nat44-ed.static-mapping`,
`nat44-timeouts`→`nat44-ed.timeouts`.

## Object types

| Descriptor | Key id | Create / Delete | Update | Retrieve | Dependencies | Notes / limitations |
|---|---|---|---|---|---|---|
| `nat44-ed.enable` | `global` | `nat44_ed_plugin_enable_disable` (enable / disable) | `ErrRecreate` (disable+enable) — refused with `ErrForeignObjects` when objects of another owner exist | `nat44_show_running_config` (`sessions == 0` ⇔ disabled; VPP zeroes rconfig on disable) | optional `vrf/<inside>`, `vrf/<outside>` | **Global singleton.** Create reads first: already enabled with the same config ⇒ converged (no call); different config ⇒ `ErrForeignObjects`. `sessions: 0` is reported as VPP's default 64512, so state it explicitly. `static-mapping-only`/`connection-tracking` are `UNSUPPORTED` in 26.06 and not modelled. Retval `FEATURE_ALREADY_ENABLED/DISABLED` is idempotent success. |
| `nat44-ed.timeouts` | `global` | `nat_set_timeouts` / Delete restores VPP defaults 300/7440/240/60 | in place (`nat_set_timeouts`) | `nat44_show_running_config.timeouts` (only while enabled) | `nat44-ed.enable` | Singleton; `nat44_ed_set_timeouts` does not exist in 26.06 — `nat_set_timeouts` is the message. |
| `nat44-ed.forwarding` | `global` | `nat44_forwarding_enable_disable` / Delete = disable | in place | `nat44_show_running_config.forwarding_enabled` | `nat44-ed.enable` | Singleton. |
| `nat44-ed.interface-feature` | `<interface>/<inside\|outside>` | `nat44_interface_add_del_feature` with `NAT_IS_INSIDE` or `NAT_IS_OUTSIDE` | recreate | `nat44_interface_dump` (+ `sw_interface_dump` for names/tags); one object per set flag | `nat44-ed.enable`, `interface/<name>` | Interface resolved by exact name (VPP's filter is a substring). Meta `IfMeta{SwIfIndex}`. |
| `nat44-ed.output-feature` | `<interface>` | `nat44_ed_add_del_output_interface` | recreate | `nat44_ed_output_interface_get` (cursor; `EAGAIN` continues) | `nat44-ed.enable`, `interface/<name>` | `nat44_interface_add_del_output_feature` no longer exists in 26.06. |
| `nat44-ed.interface-address` | `<interface>` | `nat44_add_del_interface_addr` (flag `NAT_IS_TWICE_NAT`) | recreate | `nat44_interface_addr_dump` | `nat44-ed.enable`, `interface/<name>` | |
| `nat44-ed.address-pool` | `<first>-<last>/<vrf>[/twice-nat]` | `nat44_add_del_address_range` (≤ 1024 addresses, flag `NAT_IS_TWICE_NAT`) | recreate | `nat44_address_dump` — single addresses merged back into maximal contiguous ranges per VRF / twice-NAT class | `nat44-ed.enable`, optional `vrf/<id>` (not for `vrf: 4294967295` = any) | Two adjacent desired pools of one class would be retrieved as one range → schema must merge/forbid adjacent pools. Ownership by address scope (slot `10.<N>.0.0/16`). |
| `nat44-ed.static-mapping` | `<name>` (tag `<owner>:<name>`) | `nat44_add_del_static_mapping_v2` | recreate | `nat44_static_mapping_dump`, tag filtered | `nat44-ed.enable`, optional `vrf`, `interface/<external.interface>` when set | Flags `addr_only`, `twice_nat`, `self_twice_nat`, `out2in_only`; `external.interface` ⇒ `external_sw_if_index`, address blanked. `match_pool`/`pool_ip_address` are write-only in 26.06 (not in details) → not modelled. Untagged / foreign-tagged mappings are invisible. Pools are registered before mappings so the plan orders them first. |
| `nat44-ed.identity-mapping` | `<name>` (tag) | `nat44_add_del_identity_mapping` | recreate | `nat44_identity_mapping_dump` | `nat44-ed.enable`, optional `vrf`, `interface/<name>` when set | |
| `nat44-ed.lb-static-mapping` | `<name>` (tag) | `nat44_add_del_lb_static_mapping` | backend set in place via `nat44_lb_static_mapping_add_del_local`; anything else recreate | `nat44_lb_static_mapping_dump` | `nat44-ed.enable`, optional `vrf/<local.vrf>` | Locals sorted (vrf, ip, port). |
| `nat44-ed.vrf-table` | `<table>` | `nat44_ed_add_del_vrf_table` + `nat44_ed_add_del_vrf_route` per route | route set in place | `nat44_ed_vrf_tables_v2_dump` | `nat44-ed.enable`, optional `vrf/<table>`, `vrf/<route>` | Ownership by table range (slot `N000–N999`). |

Retrieve-only state / actions (not descriptors): `Users` (`nat44_user_dump`), `UserSessions` (`nat44_user_session_v3_dump`,
per user, paged in the agent with offset/limit), `DeleteSession` (`nat44_del_session`, `NAT_IS_INSIDE` [+ `NAT_IS_EXT_HOST_VALID`]).
Not used: `nat44_add_del_static_mapping` (v1), `nat44_set_session_limit`, `nat_set_workers`, `nat_set_mss_clamping`,
`nat_ipfix_enable_disable` (DF-8), `nat44_ed_set_fq_options`, `nat44_user_session_dump`/`_v2_dump`.

## Ownership on the shared VPP (docs/lab/shared-host-rules.md)

`natcommon.ScopeFor(owner)`: owner `w<N>` owns addresses in `10.<N>.0.0/16`, tables `N000–N999`, interfaces tagged
`w<N>:*` and mappings tagged `w<N>:<name>`; any other owner (production `vrx`) owns everything. Singletons (enable,
timeouts, forwarding) are global: the integration test reads them first, enables only when nobody has, refuses to
disable while foreign objects exist and restores what it changed in `t.Cleanup`. ED and EI are mutually exclusive;
this slot's nat44ed and nat44ei packages serialise on `/run/vrx-test/w<N>/nat44.lock` (`nattest.SlotLock`) and skip
when the other mode is enabled by a foreign owner.

## Tests

- Unit: `nat44ed_test.go` — stateful fake VPP shared with owner `w3`; create, idempotent re-apply (empty plan),
  update in place, `ErrRecreate`, delete with Meta, Retrieve filtering, dependency ordering, VPP errors.
- Integration: `nat44ed_integration_test.go` (`VRX_INTEGRATION=1 VRX_TEST_PREFIX=w9`): loopbacks `loop901/902`,
  table `9001`, pool `10.9.1.1–10.9.1.4`, twice-NAT pool `10.9.2.1`, mappings tagged `w9:*`; every object
  Retrieve == desired, then deleted, Retrieve empty; plugin disabled again in Cleanup when this test enabled it.
