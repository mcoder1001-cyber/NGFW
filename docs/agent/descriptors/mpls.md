# mpls descriptors (DF-7, WBS D2.8)

Package `apps/agent/internal/descriptors/mpls` — core VPP MPLS. Messages only from `apps/agent/binapi/{mpls,
fib_types}`. DF-7 conventions: see `policer.md`. (SR-MPLS is DF-6.)

## Key contract (DF-6 SR-MPLS and F-mpls build against it)

| Object | Descriptor | Key | Helper | Meta |
|---|---|---|---|---|
| MPLS table | `mpls-table` | **`mpls-table/<id>`** | `mpls.KeyTable(id)` | none |
| MPLS on an interface | `mpls-interface` | **`mpls-interface/<logical name>`** | `mpls.KeyInterface(name)` | `{SwIfIndex}` |
| label route | `mpls-route` | `mpls-route/<table>/<label>/<eos\|neos>` | `mpls.KeyRoute` | none |
| label ↔ IP binding | `mpls-ip-bind` | `mpls-ip-bind/<mpls table>/<label>/<vrf>/<prefix>` | `mpls.KeyIPBind` | none |
| MPLS tunnel | `mpls-tunnel` | `mpls-tunnel/<name>` | `mpls.KeyTunnel` | `{SwIfIndex, TunnelIndex}` |

The descriptor names are hyphenated (not `mpls.table`) because DF-6 already depends on `mpls-table/<id>`.
A tunnel's interface is tagged `<owner>:<name>`, so its logical name (D-069) is the tunnel name and other objects
reference it as `interface/<name>`.

## Object ↔ message table

| Object type | Create / Update / Delete | Retrieve | Notes |
|---|---|---|---|
| `mpls-table` | `mpls_table_add_del` (name `vpp.OwnerTag(owner, "<id>")`) / del after re-verifying the name | `mpls_table_dump` (name = our tag) | an existing table with another name is refused; ids by `df7.WithIDRange`; table 0 is VPP-global (see below) |
| `mpls-interface` | `sw_interface_set_mpls_enable` / disable while `mpls_interface_dump` still lists it | `mpls_interface_dump` | needs MPLS table 0 (`NO_SUCH_FIB` otherwise); VPP decrements a u8 counter without checking — a disable is never sent for a disabled interface |
| `mpls-route` | `mpls_route_add_del` (not multipath: the path set is replaced; Update in place) / del | `mpls_route_dump` per owned table, labels ≥ 16 | paths `df7.Path`: next hop (+interface) or recursive via table, drop/local/icmp, out-label stack (label, ttl, exp; pipe mode — `is_uniform` is not reported by VPP's path encoder), weight, preference; an MPLS-proto "via label"/deag path's table is not reported either → not supported |
| `mpls-ip-bind` | `mpls_ip_bind_unbind` bind / unbind | **write-only** | the binding's label entries land in MPLS table 0 and cannot be told apart from routes (no source in `mpls_route_details`); table 0 must exist or is created by VPP (global) |
| `mpls-tunnel` | `mpls_tunnel_add_del` (sw_if_index ~0 creates with all paths) + interface tag; Update = ErrRecreate; delete sends the tunnel's own paths (VPP deletes the tunnel when the last path goes) | `mpls_tunnel_dump`, `mt_tag` = our tag | duplicate tags (lost reply + retry) → the lowest sw_if_index is `<name>`, others `<name>#<sw_if_index>` (never desired → deleted) |

Dependencies: route → `mpls-table/<table>` + next-hop `interface/<if>` (optional); ip-bind → `mpls-table/<t>` +
`vrf/<id>`; interface → `interface/<if>` + `mpls-table/0` (optional); tunnel → next-hop interfaces (optional).

## MPLS table 0 and D-071

VPP creates no MPLS table by default. Enabling MPLS on an interface requires table 0 and locks it; a label binding
creates/locks it. Table 0 is therefore VPP-global: the product's globals owner declares `mpls-table/0`; on the
shared host (no table 0) the host test shows `mpls-interface` Create → `NO_SUCH_FIB` and runs the enable and the
binding only with `VRX_DF7_GLOBALS=1`.

## FIB entries

`mpls-table` creates an MPLS FIB (with VPP's reserved-label special entries); `mpls-route` adds API-sourced label
entries to it (removed before the table — the scheduler deletes routes first; test cleanup does the same, V15);
`mpls-ip-bind` adds an MPLS-sourced local label to the IP prefix and entries in MPLS table 0; `mpls-tunnel` adds no
FIB entry of its own (its paths resolve through the FIB).
