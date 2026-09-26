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

## Shared table 0 (review H2)

`mpls_route_details` carries no FIB source, and table 0 also holds SR-MPLS BSIDs (DF-6), `mpls-ip-bind` local labels
and FRR / linux-cp labels. In table 0 an `mpls-route` is therefore reported, updated and deleted only when this owner
recorded it (D-080 boot record written after its own successful add); Create refuses a label that exists without
our record (`dfkit.ErrNotOurs`). Other tables are owned by name (`<owner>:<id>`) and report every label ≥ 16.

## mpls-interface enable counter (review L1)

`sw_interface_set_mpls_enable` is a u8 reference counter. Create enables only when `mpls_interface_dump` does not list
the interface (an enabled interface is accepted only when tagged or claimed by us — never adopted); Delete sends one
disable, and only while the dump lists it (a disable at 0 wraps the counter). References of other consumers are left
alone.

## F-mpls-srmpls (gap-only changes, each with a named test in `f_mpls_srmpls_test.go`)

| Gap | Change | Test |
|---|---|---|
| D-071 role of table 0 | `NewTableFor(c, owner, globalsOwner)` / `RegisterFor`: the globals owner creates and deletes `mpls-table/0` (named `<owner>:0`, exempt from the id range: it is VPP's default table, not an allocated id); any other agent only **requires** it — Create succeeds while VPP has table 0 (whoever created it) and fails with `dfkit.ErrNotGlobalsOwner` otherwise, sending nothing; Delete never touches it; Retrieve reports `mpls-table/0` exactly while it is required and exists. `NewTable`/`Register` keep DF-7's (globals-owner) behaviour | `TestTableZeroRequiredByNonOwner` |
| Table-0 label routes of a non-globals-owner | `mpls-route` reads and deletes table-0 routes whenever table 0 exists, whoever named it (still only the labels this owner recorded, review H2). DF-7 counted table 0 only when it carried this owner's name: such routes were never reported and Delete forgot them without removing them from VPP | `TestRouteTableZeroOfTheGlobalsOwner` |
| TD-11b claim-first | `mpls-interface` claims an untagged interface before the enable (`Target.ClaimFirst`, `Undo`/`Adopt`); `mpls-route` writes its table-0 record before the add and drops it when the add fails | `TestInterfaceClaimFirst`, `TestRouteTableZeroRecordFirst` |
| TD-11b declarations | `mpls-table`, `mpls-ip-bind`, `mpls-tunnel`: `RecordsNoOwnership` (name / tag / write-only); `mpls-interface`: `CheckPersistent` over the owner's DF-1 claim store; `mpls-route`: `CheckPersistent` over the DF-7 BootStore (`ownership.go`) | `TestOwnershipDeclarations` |
| TD-11c creator obligation | `mpls-tunnel` provides `interface/<name>` (`scheduler.KeyProvider`): objects on or through the tunnel interface order after it on create and before it on delete. Remove `mpls.NameTunnel` from TD-11c's `knownAliasCreatorGaps` when both branches are on main | `TestTunnelProvidesInterfaceAlias` |
| Missing dependencies | `mpls-ip-bind` in the default VRF no longer depends on `vrf/0` (nothing provides it: never plannable); `mpls-route` / `mpls-tunnel` depend on `vrf/<id>` for paths that resolve or look up in IP table `<id>` ≠ 0 | `TestDependenciesOfBindingsAndLookupPaths` |

Product wiring (`subsystems/mpls_srmpls.go`): `mpls.RegisterFor(r, c, owner, Env.GlobalsOwner, df7.WithIDRange(<agent id range>))`
after `df7.SetBootStore(owner, Wiring.BootStore())` and `iface.SetClaimStore(owner, IfaceClaims)`. The projection
(`desired/mpls_srmpls.go`) declares `mpls-table/0` only when the configuration needs it (MPLS interfaces, bindings,
SR-MPLS policies or a table-0 label route).
