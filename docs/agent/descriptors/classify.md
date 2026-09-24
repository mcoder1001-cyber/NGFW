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
1. the Store is bound to the running VPP instance — the D-080 boot identity (`internal/vpp/bootid`: kernel boot_id, `control_ping_reply.vpe_pid`, VPP start time) is stored with the records as `vpp_boot`; a different identity (VPP or host restarted) makes every record stale and the store is reset. A pre-TD-1 file (`vpp_instance` = vpe_pid only) loads with an unknown instance and is reset on first use;
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

## Deletes re-verify identity (D-071, fix round 2)
- `classify.table` Delete runs the store snapshot under the transaction lock right before deleting: the index is deleted only if
  the record is live now (same VPP instance, index listed, same geometry) **and** equals the caller's Meta index. Otherwise the
  record is dropped and VPP is not touched (host regression `TestTableDeleteStaleIndexOnHost`).
- Binding deletes re-resolve the stored `sw_if_index` (`df2.SkipDelete`, see the other plugin docs).
- `classify.input-acl` Delete unbinds the tables `classify_table_by_interface` reports (only those that still exist); Create
  refuses an interface that already has input tables bound (VPP's add would be a silent no-op).
- `classify.output-acl` unbind only sends what VPP still has bound (feature enabled, recorded table still existing) and
  otherwise just drops the record, so a vanished table cannot wedge Create/Delete.
- Residual (follow-up N3): a table of another owner with **identical geometry** on a reused index within one VPP instance is
  indistinguishable from ours.

## A table is never freed while bound (TD-3, D-095 b)

`classify.table` Delete re-verifies, right before `classify_add_del_table(is_add=0)`, that nothing refers to the
index (`TableUsers`): another table chaining to it (`classify_table_info.next_table_index`), an input ACL on any
interface (`classify_table_by_interface` over `sw_interface_dump`), the punt ACL (`punt_acl_get`), the ipfix classify
tables (`ipfix_classify_table_dump`) and ip-session-redirect sessions (`ip_session_redirect_dump`). Bindings VPP cannot
report come from the Store: the output-ACL records and — new — `bindings`, the write-only `interface-ip-table` /
`interface-l2-tables` bindings this owner applied (persisted in the FileStore; a record whose sw_if_index no longer
exists is ignored: its index is cleared by the next creator's `ifsanitize.Sanitize`). While anything is found the
Delete fails with `ErrTableInUse` naming the users; the scheduler normally never gets there because every binding
descriptor depends on `classify.table/<name>` and on `interface/<name>`, so bindings are deleted first. Policer and
flow classify bindings have no usable readback (VPP 26.06 dumps read out of bounds); their descriptors' dependency on
the table orders them. Why: a binding to a freed table crashes VPP on the first packet (V19, 2026-09-24 04:50:27).

What this check cannot see (TD-3 review M1) and why it is acceptable now: an input ACL left on an index whose interface
was deleted behind the agent's back (VPP answers every call on a deleted index with INVALID_SW_IF_INDEX, so it can
neither be read nor unbound), and write-only records of deleted interfaces. Since fix round 1 every interface Delete
clears the bindings first (`ifsanitize.BeforeDelete`), and a creator that gets such an index resurrects the deleted table
and unbinds it (input ACL exactly, the write-only kinds through the pool's free list) or quarantines the index — so a
table deleted in that state is no longer a crash vector for our interfaces. Output-ACL records keep refusing the Delete
even when their interface is gone. Concurrency: the scheduler is sequential, so the `TableUsers` → delete window is only
open to other owners on the shared VPP (a table bound by another slot between the check and the delete); accepted.
