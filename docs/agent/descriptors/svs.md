# Descriptors — svs plugin (F-vrf-static-ecmp, source VRF select)

Package `apps/agent/internal/descriptors/svs`, model `svs_model.proto` (agent-internal, D-055). `svs.Register(r, svs.Env{…})`
(wired by `internal/subsystems/vrf_static_ecmp.go` with the persisted BootStore, `IfTableRef: core.InterfaceTableKey`);
domain `vrfs` (`Domains[VRFs]`). Projection: `internal/desired/vrf_static_ecmp.go` (`vrfs.<vrf>.sourceSelect[]{prefix, interface}`).

| Object type (descriptor) | Key | Depends on | VPP messages | Update | Readback |
|---|---|---|---|---|---|
| `svs.table` | `svs.table/<id>` | — | `ip_table_add_del` (IPv4 + IPv6, name `<owner>:svs:<id>`); Retrieve `ip_table_dump` | missing family re-added; Reapplier re-asserts the API lock (idempotent) | real: tables named `<owner>:svs:<id>` (`missing_ip4/6` on partial loss) |
| `svs.interface` | `svs.interface/<if>` | `interface/<if>` (alias), `svs.table/<id>`, optional `interface-ip.table/<if>` | `svs_enable_disable` (IPv4 + IPv6); Retrieve `svs_dump` + `sw_interface_dump` | table change → ErrRecreate; a missing family is enabled | real: `svs_dump` entries whose table is ours, by logical interface name (D-069) |
| `svs.route` | `svs.route/<table>/<prefix>` | `svs.table/<table>`, `vrf/<source table>` (unless 0) | `svs_route_add_del`; Retrieve `fib_source_dump` + `ip_route_v2_dump` (src = the `svs` source) | always ErrRecreate (delete + add) | existence real (entries of the `svs` source except 0/0); **the selected table from an applied-once record** (below) |

## Model

`vrfs.<vrf>.sourceSelect[k] = {prefix, interface}`: a packet arriving on `interface` whose source address is in `prefix` is
looked up in `<vrf>`. VPP (`plugins/svs/svs.c`) binds one svs FIB table to an interface (`svs_enable_disable`, which also adds
`0/0 → the interface's own table`), and every svs route is a source-prefix entry in that table whose DPO is a lookup in the
selected table. So the projection makes **one svs table per ingress interface**, an enablement per interface and one route per
entry (`source_table_id` = the VRF's table id, 0 for `default`).

**Table ids** (`svs.Allocate`): interfaces in name order each take the first free id probing downward from
`Hi − fnv32a(name) mod size` in the svs range, skipping declared VRF ids — stable while other interfaces come and go. Range:
`svs.DefaultRange` 4294967040–4294967294 on the product agent; a test slot (`VRX_VPP_TABLE_BASE=N000`) uses the top 100 ids of
its range (`svs.RangeIn`, e.g. 2900–2999).

## Ownership and readback (D-063/D-071/D-076/D-080)

- Tables are named `<owner>:svs:<id>` by `ip_table_add_del` (the name is set by the first creator; ours is created first). The
  VRF descriptor skips names with `:` after the owner (a VRF name never contains one), so an svs table is never a VRF.
  A table id that exists under another name fails Create with `svs.ErrTableConflict` before anything is sent.
- `svs_table_add_del` is **not** used: its lock is counted per add (a repeated add leaks, an unbalanced delete underflows the
  per-source lock count); `svs_route_add_del` / `svs_enable_disable` only need the table to exist, which `ip_table_add_del`
  guarantees idempotently.
- Enablements and routes are ours when their table is. Enablement is not idempotent in VPP (a second enable stacks the
  feature and the 0/0 entry): Create enables only the families `svs_dump` does not list; another table on the interface is
  `svs.ErrInterfaceConflict`.
- **svs routes**: a second `svs_route_add_del` add only bumps the entry's source reference count and keeps the first lookup
  table, and the dump does not carry the selected table (an exclusive lookup DPO is dumped as a bare path). Create therefore
  removes an entry that is already there and adds it, then stores a BootRecord `{key, VPP boot identity, source table}` in the
  persisted BootStore (`boot-<owner>.json`). Retrieve reports the entries VPP has and takes the selected table from that record
  when it belongs to the running VPP instance; otherwise it reports `UnknownTable` (2^32−1), the diff sees a mismatch and the
  reconciler re-programs the entry (ErrRecreate). The assembler leaves unknown entries out of `sourceSelect`. Nothing is ever
  echoed from desired state.

## Ordering

Deletes run entries before tables (V15): routes and the enablement depend on the table. `svs_enable_disable` reads the
interface's table once, when it is enabled (and VPP's rebind callback is buggy, V-new): the optional dependency on
`interface-ip.table/<if>` makes the reconciler delete and re-create the enablement around a VRF binding change.

## Tests

`svs_test.go` (coretest model with `InstallVrfStaticEcmp`: svs handlers modelling VPP's duplicate-add behaviour,
`fib_source_dump`, the `src` filter of `ip_route_v2_dump`): apply/retrieve/idempotency/delete, record loss and a new VPP
identity, foreign table, rebind re-creation, allocation. `svs_integration_test.go` (`VRX_INTEGRATION=1`, host VPP): weighted
ECMP, next hop in another table, blackhole and source VRF select; `vppctl show ip fib …`, `show svs`; loss → resync; rollback
and the V15 probe (the table ids re-created hold only VPP's 5 default entries).
