# classify descriptors (DF-2, WBS D2.7)

Package `apps/agent/internal/descriptors/classify`. Messages from `apps/agent/binapi/classify` (+ `ip_session_redirect_dump` to tell redirect sessions apart).

| Object | Descriptor / key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| table | `classify.table` / `classify.table/<name>` | `classify_add_del_table` (nbuckets, memory_size, skip/match vectors, mask bytes, next table, miss_next, current_data flag/offset); Update → `ErrRecreate` (the API cannot modify a table) | `classify_table_ids` → `classify_table_info` (nbuckets, vectors, mask, next, miss) | next table (`classify.table/<name>`) | Meta `{Index}`. See "Ownership". |
| session | `classify.session` / `classify.session/<table>/<hex(match)>` | `classify_add_del_session` (match, hit_next, opaque, advance, action, metadata); Update `ErrRecreate` | `classify_session_dump` per owned table; sessions that `ip_session_redirect_dump` lists are excluded (they belong to `ip-session-redirect.redirect`) | `classify.table/<table>` | Match canonical = trailing zeros trimmed; `NormalizeSession`. |
| interface ip table | `classify.interface-ip-table` / `<ifname>/<ipv4\|ipv6>` | `classify_set_interface_ip_table` | **none — partial** (`ErrRetrieveUnsupported`) | table + interface | No dump in the API. |
| interface l2 tables | `classify.interface-l2-tables` / `<ifname>/<input\|output>` | `classify_set_interface_l2_tables` | **none — partial** | tables + interface | No dump. |
| input ACL | `classify.input-acl` / `<ifname>` | `input_acl_set_interface` (ip4/ip6/l2 tables) | `classify_table_by_interface` on owner-tagged interfaces | tables + interface | Interfaces deleted between the snapshot and the query (`INVALID_SW_IF_INDEX`) are skipped. |
| output ACL | `classify.output-acl` / `<ifname>` | `output_acl_set_interface` | **none — partial** | tables + interface | `classify_table_by_interface` reports input tables only. |

## Ownership and values VPP does not report
Classify tables have no tag/name in the API. The owner's `name ↔ index` mapping lives in a `classify.Store` (`FileStore` in the agent's state dir, `MemStore` in tests) — the "owner table in the state dir" of `internal/descriptors/README.md`. Retrieve uses a record only when `classify_table_ids` still lists its index (stale records are dropped), so after a VPP restart the tables are re-created. The Store also keeps create-time parameters VPP does not return (`memory_size`, `current_data_*`, session `action`/`metadata`); every other field is decoded from VPP. This is a documented limitation, not cached desired state for existence.

CLI: `show classify tables [verbose]`, `show inacl type ip4`.
