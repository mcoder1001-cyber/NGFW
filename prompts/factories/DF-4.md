# Task: DF-4 — Descriptors for VPP plugins: acl (incl. macip), acl stats   (prepend 00-CONTEXT.md)

## Goal
Write the reconciler **descriptors** (Create/Update/Delete/Retrieve/Dependencies) for the VPP `acl` plugin — L3/L4 ACLs (stateless and
stateful/reflect), per-interface in/out binding, L2 MACIP ACLs, ethertype whitelists and hit counters (WBS D5.2) — against the scheduler
interface published by P05a and the generated bindings in `apps/agent/binapi/`. No API, no UI — pure agent-side building blocks that the
firewall F-* tasks wire up later. Your `acl/<name>` key is consumed by DF-2 (ABF) and later by NAT/policer wiring — publish it early.

## Inputs to read first
- `apps/agent/internal/scheduler/descriptor.go` (P05a — the interface; do not change it, request changes via a question file)
- `apps/agent/internal/descriptors/README.md` and one merged example (e.g. `interface/`) — key scheme `interface/<name>`
- `apps/agent/binapi/acl/`, `binapi/acl_types/`, `binapi/ethernet_types/`, `binapi/ip_types/` — **the only source of message names and fields**;
  verify every name below in the package, never guess
- `apps/agent/internal/vpp/` stats-segment reader (P05; if not merged yet, the P05a `Client` interface + a stats fake) — hit counters live in the
  stats segment, not in the binary API
- VPP 26.06 docs: https://s3-docs.fd.io/vpp/26.06/ (acl plugin, "ACL-based forwarding" for how ABF consumes acl indices)
- `docs/lab/host-vrx-a.md` — `acl_plugin.so` is loaded; nothing here needs an unloaded plugin
- `docs/lab/shared-host-rules.md` — tags `w<N>-*`, prefixed loopbacks, no unprefixed objects

## Scope — build exactly this
Enumerate the object types in `docs/status/tasks/DF-4.md` first, then build (estimate: 8 object types × Create/Update/Delete/Retrieve
+ unit test with the fake client + one integration check under the shared lock with your slot prefix):

### Object types (binapi package → messages to look for)
- **acl** (`binapi/acl`): `acl` — an ordered rule list (acl_add_replace with `acl_index = ^uint32(0)` to create, the stored index from `Meta` to
  update in place; rules: is_permit permit/deny/permit+reflect, src/dst prefix, proto, port ranges, tcp flags mask/value, icmp type/code ranges;
  `tag` = `w<N>-<name>` — the tag is how Retrieve attributes ACLs to owners; acl_del). Retrieve: acl_dump (all, then filter by tag prefix).
  Update never recreates: a replaced ACL keeps its index so interface bindings and ABF policies stay valid.
- `acl-interface-binding` (acl_interface_set_acl_list: full ordered in/out lists per interface, `n_input` split; empty list = unbind. Prefer this over
  acl_interface_add_del because the whole list is the unit of desired state). Retrieve: acl_interface_list_dump.
- `acl-etype-whitelist` (acl_interface_set_etype_whitelist: ethertypes per interface in/out). Retrieve: acl_interface_etype_whitelist_dump.
- **macip** (`binapi/acl`): `macip-acl` (macip_acl_add_replace to create/update by index; rules: permit/deny, src mac + mask, src prefix; tag
  `w<N>-<name>`; macip_acl_del). Retrieve: macip_acl_dump. `macip-acl-interface-binding` (macip_acl_interface_add_del — one MACIP ACL per interface,
  input only). Retrieve: macip_acl_interface_list_dump (or macip_acl_interface_get — verify which is generated).
- **acl stats**: `acl-stats-enable` (acl_stats_intf_counters_enable — global singleton; read-modify-restore in tests), `acl-stats` (Retrieve-only:
  per-ACL, per-rule packet/byte hit counters read from the stats segment — discover the exact paths, e.g. under `/acl/…`, with the stats client's
  directory listing on the host and record them in the doc table; never hard-code a path you did not list). Expose a typed reader the F-* tasks
  and `StreamStats` can call at 1 Hz; no descriptor Create/Delete for it.
- Health/limits (Retrieve-only helpers, not descriptors): acl_plugin_get_version, acl_plugin_get_conn_table_max_entries — read and log; the
  conn-table size itself is a `startup.conf` knob and out of scope.

### Dependencies (declare in `Dependencies()`, test ordering with the fake)
acl → none · macip-acl → none · acl-interface-binding → interface + every `acl/<name>` it lists (Optional=false, so an ACL is created before it
is bound and unbound before it is deleted) · acl-etype-whitelist → interface · macip-acl-interface-binding → interface + `macip-acl/<name>` ·
acl-stats-enable → none · acl-stats → the ACLs it reads (Optional). Publish the key strings `acl/<name>` and `macip-acl/<name>` and the
`Meta` layout (acl_index) in `docs/agent/descriptors/acl.md` in your first commit — DF-2 builds against them.

### Deliverables per object type
1. Descriptor in `apps/agent/internal/descriptors/acl/<object>.go`: `KeyOf`, `Dependencies`, `Create`, `Update` (or `ErrRecreate`), `Delete`,
   `Retrieve` (full dump, decoded into the same proto type used for desired state, including metadata such as acl_index / sw_if_index).
   Rule encoding/decoding lives in one `rules.go` shared by acl and macip with exhaustive round-trip tests (prefix lengths, port ranges 0–65535,
   tcp flags, icmp ranges, "any" wildcards) — a decode that is not byte-identical breaks idempotency.
2. Registration in the plugin's `Register(scheduler)` function; add to the descriptor registry list.
3. Unit tests with the fake VPP client (table-driven: create, idempotent re-apply, update-in-place keeps index, delete, dependency ordering,
   Retrieve decoding, binding list reorder = update not recreate).
4. Integration test against the host VPP (`/run/vpp/api.sock`, `VRX_INTEGRATION=1`, `flock -s /run/lock/vrx-lab.lock`): create → Retrieve shows it →
   delete → Retrieve shows nothing. **Every object name/tag/table id carries your `VRX_TEST_PREFIX` / slot range**; Retrieve-based assertions
   filter by your tag prefix (other workers' ACLs exist on the same VPP — never assert on total counts or on index values). Bind only to your
   prefixed loopbacks; never touch `local0` or anything unprefixed; clean up in `t.Cleanup` (unbind before delete). One 50-rule ACL to prove the
   encoder at size; one stats read that shows zero counters with the expected shape (no traffic on this host).
5. `docs/agent/descriptors/acl.md`: table object type ↔ VPP messages ↔ notes/limitations (+ stats paths, + the reflect/stateful caveats).

## Rules
- Message names come from binapi. `apps/agent/binapi/` is **manager-owned** (generated for all plugins by P04, D-014): if a message is missing,
  write `docs/status/tasks/DF-4-questions.md` naming the plugin and continue with the rest; never edit `tools/binapi-gen.sh` or
  `apps/agent/binapi/` in your branch.
- Retrieve must decode *everything* the diff needs (every rule field, list order, n_input split, tag); a descriptor without Retrieve is not done.
- No shelling out to `vppctl`. No C. No changes to `startup.conf`. No VPP restarts (D-012).
- ACL rule order is semantic: desired order == applied order == retrieved order; test it.

## Acceptance (paste the evidence)
- [ ] `go test ./internal/descriptors/acl/...` green (unit + integration on the host)
- [ ] `grep -rn "vppctl\|exec.Command" internal/descriptors/acl` is empty
- [ ] Applying the same desired state twice yields an empty plan (log excerpt) — including for a 50-rule ACL and a reordered binding list
- [ ] Object ↔ message table committed; `acl/<name>` key contract documented in `docs/agent/descriptors/acl.md`
- [ ] `vppctl show acl-plugin acl` / `show acl-plugin interface` / `show acl-plugin macip acl` pasted for your tagged objects, then empty after delete

## Out of scope (do not build)
API endpoints, UI screens, schema changes (firewall rule model, zones, aliases), F-* feature wiring, performance, startup.conf changes
(conn-table sizes, hash params). Not yours: ABF policies (DF-2 — they consume your key), classifier-based `input_acl_set_interface` (DF-2),
NAT (DF-3), interfaces (DF-1/P05), policers (DF-7), packet-level tests (firewall F-*), session/conn-table browsing UI. No binapi regeneration,
no VPP restart, no `local0`, no `vppctl clear acl-plugin` on the shared VPP.
