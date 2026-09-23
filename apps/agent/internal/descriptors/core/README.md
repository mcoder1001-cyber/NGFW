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

## Claim rule (D-071, review H1/H2)

- **Loopbacks, addresses, table bindings:** ours only via the interface tag `<owner>:<name>`; every delete re-resolves
  the sw_if_index from the tag right before deleting (never a stale Meta index, L1); a vanished interface is not an error.
- **VRF tables:** Create dumps the id first. A family that exists under any name other than `<owner>:<vrf>` fails
  Create with `ErrTableConflict` before any message is sent (VPP would silently keep the foreign table and a later
  rollback would delete it). Create's cleanup removes only families this call created. Delete re-dumps and removes a
  family only while it is still named `<owner>:<vrf>`.
- **Routes:** decided by the FIB entry's **best source** (`ip_route_v2_dump` `src`, re-review N1). A route is claimed
  (owner-table record) unless the prefix already has a client-programmed entry in that table (API, CLI, DHCP, SR, 6RD,
  LISP, MAP, or any plugin-allocated source such as lcp-rt/nat) → `ErrRouteConflict`, nothing sent, nothing recorded.
  VPP-generated entries (recursive-resolution next-hop /32s, connected/interface, adjacency, default-route, special)
  never block: VPP stacks our API source next to them. Retrieve reports a claimed prefix only while its best source is
  API; Delete sends the delete only when an API source can be present (best source API, or a higher-priority source
  such as interface that may hide it) and never removes VPP-generated entries. Table 0 is shared; ownership is per prefix.
  Residuals: a foreign API route on a prefix whose best source is `interface` (a static route over a connected prefix)
  is invisible to the check; a stale claim (crash between recording and programming, or a route lost with VPP while the
  owner table survived) can adopt another client's later route on that prefix (re-review N3) — single-agent
  production is unaffected; on the shared host slot ranges keep prefixes apart.
- **D-072 (FRR-owned static routes):** the projection skips every `routing.static[i]` for which
  `frr.StaticOwnedByFRR(i, route, nil)` is true (wired; the default selector marks none for proto input). The schema
  flag and its `RegisterStaticSelector` belong to P08/P12 (+ contract change). A linux-nl route that FRR installs for a
  prefix the agent also programs makes our Create fail with `ErrRouteConflict` (plugin source `lcp-rt`) — the intended
  "never both" outcome.

## Owner table recovery (L2)

`owned-<owner>.json` is the claim record for routes. A corrupt file makes the agent refuse to start (fail closed):
move it aside and restart — the agent then owns no routes; the next Apply fails with `ErrRouteConflict` for every
route still present in VPP from before. Remove those prefixes once by hand (`ip_route_add_del`, or `vppctl ip route
del` by an operator) or restore the file from backup, then re-apply. Routes in owned VRF tables disappear with their
table; only table-0 routes need this.

## VPP quirk seen on the shared host

Deleting a FIB table (`ip_table_add_del` is_add=0) while it still holds API **drop** routes leaks those entries: the
next table that reuses the FIB index (any owner's!) starts with them. The reconciler always deletes routes before their
table; test helpers that simulate loss flush the table (`ip_table_flush`) before deleting it.

## Handoff to P08 (review M5)

`interface-ip` and `interface-ip.table` work only on interfaces this owner tagged (loopbacks today). Addresses on
physical/pre-existing NICs (untagged, e.g. `lan`) fail validation (`interface/<name>` has no provider) until DF-1's
alias descriptor is wired (D-065/D-073a) **and** a claim path exists for untagged interfaces (D-071 ClaimStore or the
alias' "physical interface" resolution with a claim record per address). P08 owns both.

Limitations: VRF and route descriptions are not VPP state; the agent service returns them from its stored desired state
(D-073b). Routes in the shared table 0 are attributed by the owner table only.

Tests: `core_test.go` (unit, `coretest` fake VPP model) and `core_integration_test.go` (host VPP, `VRX_INTEGRATION=1`).
