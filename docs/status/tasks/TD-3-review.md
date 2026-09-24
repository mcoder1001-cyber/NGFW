# TD-3 review — V19 crash guard (independent reviewer)

Branch `task/TD-3` @ d001e5f, reviewed against `main`. VPP source checked read-only in `/root/vpp/src` (v26.06).

## What I ran (slot 2, on the host, no packets, VPP never restarted)
- `tools/ci.sh --base main` → **CI GATE PASSED** (mode quick, 3m41s, logs `/root/ngfw-wt/logs/ci/TD-3-20260924-052653-3036188`); matches the status file.
- `VRX_INTEGRATION=1 tools/lab lock shared go test -p 1 ./internal/vpp/ifsanitize -run OnHost` → `TestV19InheritanceClearedOnHost` PASS
  (the inherited `ip4-classify:[0]:table:1` DPO on `10.2.91.1/32` appears, and is gone after Sanitize). `NRestarts` 5 → 5.
- `vrx-vpp-preflight` against the live VPP, before and after: `V19 pre-flight ok … (0 warning(s))`.
- Contract guard: no changes under schema/proto/gen/api-client; `apps/agent/binapi/` and `tools/binapi-gen.sh` untouched.

## The key question: can a binding to an already-freed table really not be removed, and is it really dormant?

**Partly true for one call, false as a conclusion.**

1. The real crash vector of 04:50 — the **ip classify table** (`classify_set_interface_ip_table`) — *can always be reset*:
   `vnet_set_ip4_classify_intfc` (`src/vnet/ip/ip4_forward.c:2784-2800`) checks the table only when `table_index != ~0`. The sanitizer
   does reset it to ~0 blindly, before any address exists. **This part is correct and sufficient.**
2. For input/output ACL, policer classify and flow classify, an unbind naming the freed table is refused
   (`pool_is_free_index → NO_SUCH_TABLE`, `classify/in_out_acl.c:92-93`, `policer_classify.c:66-67`, `flow_classify.c:55-56`).
   But that does **not** mean it cannot be removed: the classify table pool is LIFO (`vppinfra/pool.h:153-161`, `_pool_get` pops
   `free_indices[n_free-1]`), so creating placeholder classify tables until `classify_add_del_table` returns the freed index T takes at
   most (number of free indices) creations; then the unbind of T is accepted and the placeholders are deleted.
3. "Dormant because feature arcs are cleared on delete" is **true for the ip4/ip6 slots only** (`feature/feature.c:698-735`). The **L2 slots
   are not vnet feature arcs** — L2 input ACL, L2 output ACL and L2 policer classify are bits in `l2input/l2output` feature bitmaps
   (`in_out_acl.c:27-34`, `policer_classify.c:20`). On delete, `l2_input_interface_add_del` (`l2/l2_input.c:510-527`) resets the bitmap
   **only if the interface was bridged or xconnected at that moment**. An interface that had an L2 ACL/policer binding while in L3 mode
   keeps the bit on its index. The new interface inherits the bit; when it is later put in a bridge, `set_int_l2_mode` ORs the bridge bits
   in without clearing it (`l2_input.c:360-372`), and `l2-input-acl` reads the stale table unconditionally:
   `l2/l2_in_out_acl.c:177-187` → `pool_elt_at_index(vcm->tables, <freed>)` → **same SIGSEGV family as 04:50, not dormant.**
4. Even the "dormant" ip4/ip6 case is **not harmless for a firewall**: every add path returns 0 early when the per-index slot is not ~0
   (`in_out_acl.c:112-114`, `policer_classify.c:84-86`, `flow_classify.c:73-75`) **without enabling the feature or storing the new table**.
   So a later `classify.input-acl` / `output-acl` / policer bind on the new interface reports success and filters nothing — a silent
   ACL bypass — and as soon as a new table lands on index T (very likely, LIFO) the stale binding silently points at someone else's table.
5. Delete-and-recreate does **not** help: `sw_interfaces` is also a LIFO pool, so the recreated interface gets the same index back.

**Safe behaviour to require** (in this order, all binapi, no C):
- a) Always call `sw_interface_set_l2_bridge{enable=0}` (→ `set_int_l2_mode(MODE_L3)`, `l2_input.c:315-331`: `feature_bitmap = DROP`,
  l2 output config zeroed) on the new index as part of Sanitize. This clears the L2 ACL / L2 policer / L2 classify bits even when the table is
  freed, which removes the crash path of point 3 unconditionally. (Verify on the host that it is a no-op for a fresh L3 interface of every type.)
- b) For any unclearable slot: **resurrect** — create untagged-then-owner-tagged placeholder classify tables (minimal mask, 1 bucket) until
  index T is returned, unbind T from the interface, delete all placeholders; re-read (`classify_table_by_interface` for input ACL, probe for
  the others) and require ~0.
- c) If (b) fails: **quarantine, don't report created** — keep the interface admin-down with tag `<owner>:quarantine:<idx>`, never use it,
  create the requested interface again (it gets a fresh index because the quarantined one holds T's index), expose a gauge
  `vrx_agent_iface_quarantined`, and fail Create if no clean index is obtained. A quarantined interface is released only when (b) later
  succeeds.

## Findings (by severity)

### H1 — Unclearable inherited bindings are accepted ("warn + metric"); L2 slots are a live crash vector, ip slots a silent ACL bypass
`apps/agent/internal/vpp/ifsanitize/sanitize.go:34-39` (doc/decision), `:221-230` (input ACL: freed table → `Unclearable`, continue),
`:118-125` (only a warn), status file "Decisions 1".
**Failure:** agent crash / external delete removes `ifA` (L3, L2 input ACL on table T) without unbinding; T is then deleted (see H2 — nothing
stops it); `ifB` reuses the index → Sanitize logs `unclearable`, Create succeeds → `ifB` is added to a bridge → first L2 frame →
`l2-input-acl` on freed T → VPP SIGSEGV. On the ip slots: a later input/output ACL on `ifB` is accepted by VPP and enforces nothing.
**Fix:** (a)+(b)+(c) above; `Unclearable` non-empty after (b) must be an error path (quarantine), not a warn. Add a host test for the
freed-table case (bind L2 input ACL on a loopback in L3 mode, raw-delete it, delete the table, create a loopback → assert it is clean or
quarantined) — no traffic needed; assert with `show inacl type l2` / `classify_table_by_interface`.

### H2 — Output ACL / policer / flow classify bindings to a freed table are invisible to the sanitizer (not even counted)
`sanitize.go:242-260` (`probe` iterates only live tables).
**Failure:** a stale output-ACL/policer binding to freed T answers `NO_SUCH_TABLE` for every live table, so the report is "clean",
`unclearable_total` stays 0, and the operator is told nothing — the L2 output ACL/L2 policer variant has the H1 crash path, the ip variants
the silent-bypass path.
**Fix:** the L3-mode reset (a) for the L2 bits; for the vectors themselves, resurrect *every* free classify index ≤ max(table ids) with
placeholders before probing (the pool is small), probe, then delete placeholders. Then there is no blind spot left and the count is honest.

### H3 — Interface Delete does not clear per-interface state while the tables still exist
`descriptors/core/loopback.go:92`, `descriptors/df6/ifdesc.go:214`, and the Delete of tap/memif/bond/af_packet/subinterface/mpls tunnel
(no `ifsanitize.BeforeDelete` in any product Delete; it is used only in test helpers).
**Failure:** any binding whose descriptor Delete was skipped (`skipDelete`, foreign/untagged binder, write-only record lost) survives on the
freed index; the table Delete (H4) then succeeds and the binding becomes unclearable. Delete time is the only moment all tables are
guaranteed alive.
**Fix:** call `ifsanitize.BeforeDelete` in every interface descriptor's Delete (and in lcp pair delete), right before the VPP delete.

### M1 — Table Delete refusal only sees live interfaces (input ACL) and own records; misses exactly the stale-index cases
`descriptors/classify/users.go:85-98` (input ACL only over `sw_interface_dump`), `:150-156` (records of deleted interfaces ignored),
no policer/flow check (documented), no foreign ip-classify check.
**Failure:** input ACL left on a deleted index → invisible → table deleted → H1. (Ignoring write-only ip-table/l2-tables records of deleted
interfaces is fine only because those two can be reset without the table.)
**Fix:** with H3 in place this narrows a lot; additionally keep the table (refuse) while *any* own record (input/output ACL, policer, flow,
ip-table, l2-tables) names an index that no longer exists **and** that kind needs the table to be cleared; resolve by running the
resurrect/unbind path, not by ignoring. Concurrency: the scheduler is sequential (no goroutines in `internal/scheduler`), so the
TableUsers→delete TOCTOU is only against other owners on a shared VPP; acceptable, note it in `docs/agent/descriptors/classify.md`.

### M2 — Pre-flight false negatives
`ifsanitize/preflight.go:83-172`.
- No check of chained tables: a live table whose `next_table_index` is a freed index crashes in the same `vnet_classify_find_entry` chain
  walk. Add `classify_table_info` for every id → FAIL if `next_table_index` is not ~0 and missing.
- L2 input/output classify tables (`classify_set_interface_l2_tables`) are not inspected; punt ACL (`punt_acl_get`) and classify
  pcap/trace filter (`show classify filter`) not inspected.
- API errors swallowed: `ClassifyTableByInterface` error → `continue` (`:114-116`), SPD dump stops on any error (`:178-181`).
- An ip classify binding on an address-less interface stays invisible (acknowledged; acceptable only because Sanitize resets it blindly).

### M3 — Pre-flight false positives / placement
- TOCTOU: `classify_table_ids` is read once (`:84-91`) before the CLI reads; a table created and bound by another slot in between is
  reported "does not exist" → spurious FAIL. The run happens after the lab lock is already converted to shared (`tools/ci.sh:492-495`).
  Fix: run `before-tests` while the exclusive lock is still held, and re-read `classify_table_ids` after the CLI reads (flag only indices
  missing in both snapshots).
- Not run **after** the suites (before `rig down`): pollution created by this run is only found by the *next* run — the 04:50 crash was a
  manual ping in exactly that window. Add `v19_preflight after-tests`.
- Exit 2 (VPP unreachable / API error) is reported with the "crash vector" message (`tools/ci.sh:468-469`); distinguish.
- Read-only: confirmed (only `*_dump`, `classify_table_ids/by_interface`, `cli_inband` with `show …`).

### M4 — Product and CI disagree on the same state
`preflight.go:150-151` FAILs a binding to a missing table on an existing interface; the agent's Sanitize declares the same state acceptable
(warn). After H1 is fixed (quarantine), make the pre-flight treat a quarantined (`:quarantine:`-tagged, admin-down) interface as WARN and
everything else as FAIL, so both apply one rule.

### M5 — Creator/state coverage
- `lcp` pairs (`descriptors/lcp/lcp.go:320,378`) create a VPP host-tap sw_if_index and are not sanitized; linux-cp is a product plugin
  even if not built here. Wire Sanitize for the host tap index (unit test with the fake is enough on this host).
- Handled by VPP itself (no action, but the docs should say so): ACL plugin (`plugins/acl/acl.c:2508-2525` resets in/out ACL lists),
  NAT44-ED/EI (`nat44_ed.c:2583-2629`), ADL (`adl.c` add/del callback re-initialises the per-index config on add — so the status file's
  "ADL allow-list config per index is not reset" risk is moot).
- Not handled, no crash path found (feature arcs cleared) but inherited state that mis-reports or blocks later config: ABF attachments,
  NAT64/NAT66/DET44 interface flags, cnat snat-if, flowprobe. List them in `docs/vpp-code-track.md` V23(b) as known, or probe-clear them.
- Ordering is right in every wired creator: Sanitize runs after VPP returns the index and before tag / admin-up / any address
  (tap's tag is in the create message, but a failed Sanitize deletes the tap).

### L1 — Metrics
`sanitize.go:378-381`: `BeforeDelete` goes through `Sanitize` and is counted in `vrx_agent_iface_sanitize_total/inherited_total`
(a before-delete run always "finds" the interface's own bindings). Count it separately (`phase="create|delete"`). Add a gauge for
currently tainted/quarantined interfaces — counters alone cannot drive an alert on current state.

### L2 — Probe cost/noise
~8 × (#classify tables) API calls and one `clib_warning` per miss for every interface create. Fine now; cap or batch once table counts grow.

### Info — verified correct
- SPD cleanup with the first existing spd_id: `ipsec_set_interface_spd` (`ipsec/ipsec_spd.c:194-222`) does not compare the id on delete;
  SPD delete clears every binding (`ipsec_spd.c` add_del loop) — reasoning holds.
- V23(a): the sanitizer never calls `feature_is_enabled` (confirmed); DF-2 `classify.output-acl` Create/Retrieve and `adl.interface`
  Retrieve still do — needs its own fix task (agree with the open question).
- Restart simulations delete dependents first via `BeforeDelete`; `TestClassifyOnHost` cleanup order fixed.
- No scope creep beyond D-095 (a)–(d); no security-pattern hits (no `exec.Command`, secrets, shell).

## Required before merge
H1 (L3-mode reset + resurrect + quarantine, host test for the freed-table case), H2 (no blind spot), H3 (BeforeDelete in every
interface Delete), M3 after-tests pre-flight run. M1/M2/M4/M5/L* may follow in a TD task.

**BLOCK**
