# Core descriptors (P05)

The minimum set that proves the reconciler end to end. Values are the agent-internal messages in
`core_model.proto`; `internal/agent/projection.go` maps the configuration document onto them and back.

| descriptor | key | value | VPP messages (binapi) | ownership | Update |
|---|---|---|---|---|---|
| `vrf` | `vrf/<table id>` | `Table{id, vrf}` | `ip_table_add_del` (IPv4 + IPv6), `ip_table_dump` | table name `<owner>:<vrf name>` | rename → `ErrRecreate`; a missing family is re-added in place |
| `interface.loopback` | `interface.loopback/loop<N>` | `Loopback{name, instance}` | `create_loopback_instance` (is_specified, user_instance N), `sw_interface_tag_add_del`, `delete_loopback`, `sw_interface_dump` | interface tag `<owner>:loop<N>` | instance change → `ErrRecreate` |
| `interface-ip.table` | `interface-ip.table/<if>` | `InterfaceTable{interface, table_id}` | `sw_interface_set_table` (IPv4 + IPv6), `sw_interface_get_table` | the interface's tag | always `ErrRecreate` (VPP refuses a rebind while addresses exist; the scheduler removes and re-adds them around it) |
| `interface-ip` | `interface-ip/<if>/<addr>/<len>` | `InterfaceAddress{interface, prefix}` | `sw_interface_add_del_address`, `ip_address_dump` | the interface's tag | value = key |
| `ip.route` | `ip.route/<table id>/<prefix>` | `Route{table_id, prefix, paths[], preference}` | `ip_route_add_del` (is_multipath=false: add replaces the path set, delete removes it), `ip_route_dump` | owner table `owned-<owner>.json` in the state dir (key added before create, removed after delete) | path set replaced in place |

Dependencies: `interface-ip.table` → interface + `vrf/<id>`; `interface-ip` → interface (+ optional
`interface-ip.table/<if>` for ordering); `ip.route` → `vrf/<id>` (unless table 0) + optional interface per egress path.
Interface references go through `Env.IfRef`: `DirectInterfaceRef` (creator key `interface.loopback/<n>` for
loopbacks, `interface/<n>` otherwise) until DF-1's alias descriptor `interface/<name>` (D-065) is registered, then
`AliasInterfaceRef`.

Canonical form (what Retrieve returns and the projection produces): addresses and prefixes through `net/netip`
(route prefixes with host bits cleared), paths sorted by (address, interface, weight), weight 0 → 1, preference = the
route's administrative distance (0 when unset). Table 0 (`default`) is VPP's own table and never an object.

Limitations (documented in docs/status/tasks/P05.md): VRF and route descriptions are not stored in VPP (DryRun warns
`agent.unsupported-field`, Retrieve leaves them unset); routes in the shared table 0 are attributed by the owner table
only; `interface-ip` manages addresses on interfaces this owner tagged (loopbacks here, DF-1 interfaces later).

Tests: `core_test.go` (unit, `coretest` fake VPP model) and `core_integration_test.go` (host VPP, `VRX_INTEGRATION=1`).
