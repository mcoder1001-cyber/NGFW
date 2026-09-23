# classify descriptors (DF-2, WBS D2.7)

Package `apps/agent/internal/descriptors/classify`. Messages from `apps/agent/binapi/classify` (+ `ip_session_redirect_dump` to tell redirect sessions apart).

| Object | Descriptor / key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| table | `classify.table` / `classify.table/<name>` | `classify_add_del_table` (nbuckets, memory_size, skip/match vectors, mask bytes, next table, miss_next, current_data flag/offset); Update → `ErrRecreate` (the API cannot modify a table) | `classify_table_ids` → `classify_table_info` (nbuckets, vectors, mask, next, miss) | next table (`classify.table/<name>`) | Meta `{Index}`. See "Ownership". |
| session | `classify.session` / `classify.session/<table>/<hex(match)>` | `classify_add_del_session` (match, hit_next, opaque, advance, action, metadata); Update `ErrRecreate` | `classify_session_dump` per owned table; sessions that `ip_session_redirect_dump` lists are excluded (they belong to `ip-session-redirect.redirect`) | `classify.table/<table>` | Match canonical = trailing zeros trimmed; `NormalizeSession`. |
| interface ip table | `classify.interface-ip-table` / `<ifname>/<ipv4\|ipv6>` | `classify_set_interface_ip_table` | **none — write-only** (`ErrRetrieveUnsupported`) | table + interface | No readback in the API. Not in the default `Register`: `RegisterWriteOnly` only (D-063). |
| interface l2 tables | `classify.interface-l2-tables` / `<ifname>/<input\|output>` | `classify_set_interface_l2_tables` | **none — write-only** | tables + interface | No readback. `RegisterWriteOnly` only (D-063). |
| input ACL | `classify.input-acl` / `<ifname>` | `input_acl_set_interface` (ip4/ip6/l2 tables) | `classify_table_by_interface` on owner-tagged interfaces | tables + interface | Interfaces deleted between the snapshot and the query (`INVALID_SW_IF_INDEX`) are skipped. |
| output ACL | `classify.output-acl` / `<ifname>` | `output_acl_set_interface` (ip4/ip6; L2 refused); Update → `ErrRecreate`; Create unbinds a recorded binding first | **presence** via `feature_is_enabled` (`ip4-output`/`ip4-outacl`, `ip6-output`/`ip6-outacl`, `vnet/classify/in_out_acl.c`); table names from the Store's `OutputRecord` for the enabled families, `#unknown` when not recorded | tables + interface | VPP returns 0 on an add while a table is bound **without switching** (`in_out_acl.c:111-115`), and refuses an unbind naming another table: the Store keeps the bound indices, Delete uses them, and Create refuses when an unrecorded table is bound. `classify_table_by_interface` reports input tables only. |

## Ownership and values VPP does not report (review H1, M6)
Classify tables have no tag/name in the API and VPP reuses indices. The owner's `name ↔ index` mapping lives in a `classify.Store` (`FileStore` in the agent's state dir, `MemStore` in tests) — the "owner table in the state dir" of `internal/descriptors/README.md`. A record is trusted only when all three hold:
1. the Store is bound to the running VPP instance — `control_ping_reply.vpe_pid` is stored with the records; a different pid (VPP restarted) makes every record stale and the store is reset;
2. `classify_table_ids` lists the index;
3. `classify_table_info` shows the recorded geometry (skip/match vectors and mask; the mask is stored per record).
Stale records are never reported. `LiveTables` is read-only; pruning happens only in `Prune` / `TableDescriptor.Create`, under the Store's transaction lock with a fresh snapshot taken inside it (Create holds the same lock from its prune to its `Put`), so a Retrieve running beside a Create cannot drop the new record. A table whose record cannot be stored is deleted again. Limitation: within one VPP instance an index freed by someone else's delete and reused for a table with identical geometry is indistinguishable; our own deletes remove the record under the lock first.
The Store also keeps create-time parameters VPP does not return (`memory_size`, `current_data_*`, session `action`/`metadata`, output-ACL tables); everything else is decoded from VPP.

`classify.session` refuses a match that is an `ip-session-redirect` session in the same table (it would silently overwrite it), and Retrieve leaves such sessions to the redirect descriptor.

CLI: `show classify tables [verbose]`, `show inacl type ip4`.

## Ownership on untagged interfaces (review H3)
Objects on interfaces tagged `"<owner>:<name>"` are ours. Physical ports (DPDK NICs) carry no tag: an object created on
an untagged interface is recorded by its **key** in the claim store (`df2.WithClaims(store)`, DF-4's `acl.ClaimStore`
interface; `df2.FileClaimStore` persists it in the agent state dir) and Retrieve reports it only while claimed. Interfaces
tagged by another owner — and `local0` — are refused with `df2.ErrForeignInterface`. Dependencies use the interface
alias key `interface/<name>` (D-065, DF-1).

Registration: `classify.Register` = table, session, input-acl, output-acl (all read back); `classify.RegisterWriteOnly` = interface-ip-table, interface-l2-tables (D-063 reconciler only).
