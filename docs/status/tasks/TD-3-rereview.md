# TD-3 re-review after fix round 1: V19 crash guard (independent reviewer)

Branch `task/TD-3` @ 175c676, reviewed against `main`. The merge-base is 26a70f3, and main is 88 commits ahead. VPP source was read
without changes in `/root/vpp/src` (v26.06). The previous review is `TD-3-review.md` (BLOCK). The worker's answers are in
`TD-3.md` under "Fix round 1". The 07:27:32 crash is D-101/V24 (af_packet), not TD-3's code.

## What I ran
- `tools/ci.sh --base main` gave **CI GATE PASSED** (quick, 3m27s, logs `/root/ngfw-wt/logs/ci/TD-3-20260924-074148-3256227`). It
  matches the status file's run at 07:31 (4m09s, PASSED); the only commits since then are docs. Contract guard: no changes under
  schema/proto/gen/api-client. `apps/agent/binapi/` and `tools/binapi-gen.sh` are untouched. No new `exec.Command`.
- Host tests on slot 2 (`eval "$(tools/lab env 2)"`, `tools/lab lock shared`, `-p 1`, one package at a time). No packets were sent,
  VPP was not restarted and no af_packet interface was deleted. `NRestarts` was **6 before and after every package**, 07:54:06–07:54:32:
  `ifsanitize` (TestV19InheritanceClearedOnHost, TestV19FreedTableOnHost), `core`, `tapv2`, `gre`, `memif`, `mpls`, `bond`,
  `classify` (incl. TestTableDeleteStaleIndexOnHost), `policer`, and `interface` (TestAttributesOnHost, TestAliasOnHost): all PASS.
  This is the first run on the host of the round-1 `Acquire` (L3 reset + placeholders) for tap, gre, memif, mpls, bond and
  sub-interface. The worker ran only `ifsanitize` on the host in this round. I did **not** run `interface/TestRestartSimulationOnHost`
  or the `af_packet` package: both delete af_packet interfaces while their veth is up, which D-101 forbids.
- Log lines from these runs: `placeholders=8..10` on every create. The quarantine holder was named **`loop0`** (`dirty 5 held by
  loop0 (tag "quarantine:w2")`).
- `vrx-vpp-preflight` against the live VPP after the runs: `V19 pre-flight ok … (0 warning(s))`, rc 0.
- VPP journal from 07:54:05 to 07:54:33: **3118** lines of `Non-existent intf_idx=… with table_index=… for delete` /
  `…_classify_intfc`. That is about 25 create and 28 delete sanitize runs, so ≈ 59 journal lines per sanitize run with only 1–2 live
  classify tables.
- Scratch tests on the fake model (`sanitizetest.Model`). They ran in a copy of `apps/agent` in my scratchpad and were **not committed**:
  - hole taken by another client after the `classify_table_ids` snapshot → `placeholders=256`, 777+777+518 probe unbinds,
    **2854 API calls for one create**, `err=nil`
  - 12 tables above the live ones freed in reverse creation order, with an output ACL left on table 13 → `err=nil freed=[]
    unclearable=[] dirty="output acl"`
  - API calls, create/delete, by live table count: 1 table → 110/21 (72 probes); 10 → 182/93 (144); 50 → 502/413 (464)

## Answers to the focus questions

1. **Can an interface still come up on a recycled index with a binding that crashes on the first packet?**
   - **Creators in this branch:** no crash path is left that I can find.
     - The ip classify table is reset blindly; `vnet_set_ip4_classify_intfc` checks the table only when it is not ~0.
     - The l2 classify tables are reset to ~0 together with their bit.
     - The L2 input ACL, L2 output ACL and L2 policer bits are cleared by `set_int_l2_mode(MODE_L3)`: `feature_bitmap = DROP` and the
       l2-output config is zeroed (`l2_input.c:315-330`). Entering a bridge later only ORs bits in (`:360-366`), so a stale slot can no
       longer re-arm them.
     - The ip in/out ACL, policer and flow slots do not crash. Their feature arcs are cleared on delete, and every *add* path returns 0
       early while the slot is not ~0, without enabling the feature (`in_out_acl.c:111-114`, same in `policer_classify.c` and
       `flow_classify.c`). A missed stale slot is therefore a **silent ACL bypass, not a crash**.
     - ADL and vxlan-bypass disables are no-ops when the feature is off (`feature.c:265`).
     - Verified on the host for loopback, tap, gre, memif, mpls, bond and sub-interface.
   - **After merging into main: yes, see H1.** DF-5 put two creators on main (`ipsec.itf`, `wireguard.interface`) that this branch
     does not wire. An IPsec or WireGuard interface with an address is exactly the 04:50 path.
   - **Non-agent creators** (`tools/lab rig up`) are covered only by the pre-flight (after rig up).
2. **Is resurrect safe on a shared VPP?**
   - It **never binds**. It only unbinds on its own sw_if_index and resets.
   - It deletes only indices it created in this run, after re-checking mask and geometry with `classify_table_info`. VPP hands out a
     live index only once, and other owners cannot adopt a placeholder, because `classify.table` `sameGeometry` compares mask bytes and
     the signature is unique.
   - While a placeholder sits on a freed index, any foreign stale binding to that index points at a valid empty table, which is safer.
     Dropping the placeholder restores the previous state, so **no new crash vector**.
   - It does disturb other owners in two ways: the runaway to 256 placeholders (M1) and blind spots between concurrent sanitizers (L2).
   - VPP's table delete succeeds silently on a free index (`vnet_classify_delete_table_index`, "tolerate multiple frees"). A client
     that deletes a stale index can therefore remove a placeholder. The mask check then refuses our delete ("not ours any more") and
     the Create fails; it is not a crash. The fake returns -6 there instead (Info).
3. **Does quarantine leak without bound?** No. Each holder pins one dirty index, there are at most `MaxAcquireAttempts` (4) per
   Create, and holders never exceed the dirty indices ever produced. But `Release` is not wired (Q2 → P08), so holders live until the
   next VPP restart. Each holder takes the **lowest free loopback instance** (M2), and the gauge resets when the agent restarts (L6).
4. **Does every Delete clear bindings first?** Yes, in the branch.
   - Every product delete call site is either a `BeforeDelete`-guarded Delete (loopback, tap, memif, bond, af_packet, sub-interface,
     DF-6, mpls, lcp) or a rollback right after a successful or failed Sanitize (Acquire `del`, tag-failure rollbacks). I grepped all
     `Delete*`/`*AddDel(false)` sites.
   - Caveats: the new creators on main (H1); an unverified meta index (L3); the af_packet quiesce needed by D-101 (L7).
5. **Is the ci.sh pre-flight correct and free of false positives?**
   - Placement is correct: `before-tests` runs under the exclusive lock, then `after-rig-up`, then `after-tests` before rig down, and
     exit 2 has its own message. The two-snapshot rule removes the create-and-bind TOCTOU.
   - Residual false positives and gaps are in L5: a table created, bound and deleted entirely inside one run under the shared lock;
     a dormant binding on a non-agent interface reported FAIL with no way to repair it; `after-tests` skipped on failing runs; FAIL
     lines able to scroll out of view.
6. **Is the per-create cost acceptable?** For the lab, today, yes. Before production it is not: see M3 (unbounded in the number of
   classify tables, ~60 VPP journal lines per create or delete). The runaway (M1) must be fixed now.

## Findings (by severity)

### H1: main has two interface creators this branch does not guard; merging reopens the V19 crash vector
On main, not on the branch:
- `apps/agent/internal/descriptors/ipsec/itf.go:67` (`IpsecItfCreate`) with Delete at `:99`
- `apps/agent/internal/descriptors/wireguard/interface.go:89` (`WireguardInterfaceCreate`) with Delete at `:121`

Both came from DF-5 (c1c1c88/fed8827) after the branch point 26a70f3.

**Failure:**
1. An agent crash or external delete leaves an index with an ip classify binding to a table that is later deleted.
2. After the merge, an `ipsec.itf` (route-based VPN) takes that index.
3. It gets its address, so the classify DPO lands on the /32 (`ip4_add_interface_routes`).
4. The first packet to the tunnel address → SIGSEGV in `vnet_classify_find_entry`, the 04:50:27 crash.

The same applies to `wireguard.interface`. Their Delete paths also skip `BeforeDelete`. The claim in `interface.md` and the status
file that "every interface creator" is covered becomes false on main.

**Fix (before merge):**
- Rebase on main.
- Create both through `ifsanitize.Acquire` (or `iface.AcquireAndTag`), with `IpsecItfDelete` / `WireguardInterfaceDelete` as `del`.
- Call `BeforeDelete` in both Delete paths.
- Add a guard so the next creator cannot forget. For example, a Go test in `ifsanitize` (or a `tools/ci.sh check` grep) that fails
  when a known interface-create message (`CreateLoopback*`, `TapCreate*`, `*AddDelTunnel*`, `IpsecItfCreate`,
  `WireguardInterfaceCreate`, `LcpItfPairAddDel*`, `CreateSubif`, `BondCreate*`, `MemifCreate*`, `AfPacketCreate*`, …) is used in
  `internal/descriptors` outside an `Acquire` closure or a `_test.go` / `*test/` helper.
- P08 (slot 1, wiring interface creation in `internal/agent`) must go through the descriptors, not raw creates. Tell the manager at
  merge time.

### M1: resurrect runs to MaxPlaceholders (256) when another client takes a hole
`ifsanitize/sanitize.go:253-289`: `holes` is computed once from the snapshot and the loop exits only when `len(holes) == 0`.

**Failure:**
1. Another slot's test, or another sanitizer's placeholder, pops a hole between the `classify_table_ids` snapshot and the loop. The
   pool is LIFO, so the top free index goes first.
2. That index never comes back, and the loop creates 256 placeholders.
3. The fake measured 2854 API calls and ~2070 VPP `clib_warning`s for **one** interface create, plus 256 × 64 KiB classify heaps.
4. A concurrent sanitizer then sees those 256 placeholders as "live" and probes 8 × 256 more.
5. If the stolen hole was the table an input ACL names, the unbind that would now succeed is never tried: `s.exists` has only
   snapshot and placeholder ids (`:419-421`). The result is a spurious `ErrUnclearable` and a quarantine.

This is not a crash, but it is plausible on the shared lab VPP, where slots create interfaces in parallel.

**Fix:**
- Once `consec >= FreshRun` and holes remain, re-read `classify_table_ids`, drop holes that are now live, and add them to `s.ids`
  (they can be probed as live tables).
- Lower `MaxPlaceholders` (e.g. holes + 2 × FreshRun, capped at 64).
- Count and log a run that hits the cap (`vrx_agent_iface_sanitize_capped_total`) instead of returning silently at `:290`.
- Add a unit test with a foreign pop between snapshot and loop (the scratch test above is ~20 lines).

### M2: the quarantine holder takes the lowest free loopback instance (`loop0`) and blocks the user's loopback of that name
`ifsanitize/acquire.go:62` (`CreateLoopback{}` makes VPP pick the lowest free instance; `ethernet/interface.c:743-760`). The host
run above shows the holder as `loop0`.

**Failure:**
1. After agent churn, a tap or tunnel create quarantines a dirty index, and the holder becomes `loop0`.
2. The user's `loop0` (`core.LoopbackDescriptor` uses `create_loopback_instance(is_specified, 0)`) now fails with "instance in use"
   on every commit or reconcile.
3. `Release` is not wired, so this lasts until VPP restarts. In the product, `loop0`/`loop1` are the most likely user names.

**Fix:**
- Create the holder with `CreateLoopbackInstance{IsSpecified: true, UserInstance: <highest free below 16384>}`, scanning down from
  16383; `LOOPBACK_MAX_INSTANCE` is 16384.
- Document the reserved range. Reserving it in `packages/schema` is a later contract task.
- Keep the `quarantine:<owner>` tag.

### M3: per-create and per-delete cost grows with the classify table count; the VPP journal floods
`ifsanitize/sanitize.go:444-467` (probe: 8 unbinds per table, over live tables plus ≥ `FreshRun` placeholders on create) and
`:239` (`FreshRun` = 8 placeholders on every create, each `clib_mem_create_heap` + info + delete).

Measured:
- 110 API calls per create and 21 per delete with 1 table; 502 and 413 with 50 tables.
- On the host, ~59 journal lines per sanitize run. VPP logs every miss twice.
- A reconcile of 500 VLAN sub-interfaces with 20 tables is on the order of 10⁵ API calls and warnings at boot, and again at teardown.

This is acceptable in the lab now and was the reviewer's L2 before. Round 1 made it ~3× larger.

**Fix (follow-up TD row, due before any production image):** bound it. Options, best first:
- (a) Read the write-only kinds exactly. `show outacl type …` and `show classify policer/flow type …` through `cli_inband` give the
  exact per-index table, because they list deleted indices too. The pre-flight already parses them (`parseBindingTable`). Then
  resurrect only those indices and unbind only those slots. This also removes the `FreshRun` guess (L1). The worker's decision 4
  ("CLI only in CI") would need a LOG entry; the output format is pinned by VPP 26.06.
- (b) Skip placeholders and probes when the create returned an index above every index ever seen in this VPP instance.
- (c) Rate-limit by caching "clean" per (VPP boot id, sw_if_index) across the delete-phase run of the previous holder.

### L1: the FreshRun blind spot is silent
`sanitize.go:233-239` and `:269-289`. Tables freed in reverse creation order, in a run of ≥ `FreshRun` above the highest live table,
look fresh. `t.Cleanup` is LIFO, so this is the normal cleanup order of a test that creates ≥ 9 tables. A stale output ACL, policer
or flow binding to the 9th or later table is not resurrected. The fake shows `err=nil, unclearable=[], dirty="output acl"`.
Consequence: a silent ACL bypass on the new interface. No crash (see answer 1). The status file documents it as "a bound, not a
proof", but nothing counts it.

**Fix:** (a) of M3 removes it. Otherwise re-probe, after the drop, the slots the metric can see; or at least document it in
`interface.md` and make `FreshRun` configurable.

### L2: concurrent sanitizers (two slots or agents) blind each other
Sanitizer B's snapshot counts A's placeholders as live (`sanitize.go:198-206`).
- If A drops placeholder P (a resurrected freed index) before B probes it, B's unbind answers NO_SUCH_TABLE. B then treats the
  interface as clean while its stale write-only slot remains (silent bypass).
- For input ACL, B's unbind fails with NO_SUCH_TABLE → error → Acquire deletes the interface and Create fails. It is retried and not
  a crash.

**Fix:** treat NO_SUCH_TABLE on a table that is not in the snapshot's own live set (another owner's placeholder) like a hole: re-read
and resurrect. Or ignore tables that carry the placeholder signature when building `s.live` (one `classify_table_info` per id; B then
resurrects them itself once A drops them).

### L3: `BeforeDelete` trusts an unverified meta index in tap, memif, bond, sub-interface and af_packet Delete
`tapv2/tap.go:171`, `memif/memif.go:91`, `bond/bond.go:126`, `interface/subinterface.go:147`, `af_packet/host_interface.go:100`.

Loopback, DF-6, mpls and lcp re-establish the index from the tag or dump first. These five do not. On a stale meta (the index was
reused by another owner between Retrieve and Delete on the shared VPP), `BeforeDelete` puts **that owner's** interface into L3 mode
(out of its bridge) and removes its ACLs, policer, ADL, vxlan-bypass and **SPD**, before the typed delete fails or hits the wrong
object. Before TD-3, a stale meta could at worst make the delete fail.

**Fix:** check `sw_interface_dump(idx).tag == <owner>:<id>` before `BeforeDelete` (a helper next to `iface.Tag`), and skip it if the
tag differs.

### L4: `l2tp` (no delete message): a failed sanitize leaves an untagged tunnel that cannot be deleted
`df6/ifdesc.go:124-131` returns the error without tagging, so every retry adds or collides with the same l2tpv3 tunnel.

**Fix:** on `ErrUnclearable`, tag the tunnel `quarantine:<owner>` (visible to the pre-flight, never used) and fail. On other errors,
tag it with the owner tag and fail, so Retrieve and the next Delete-less plan see it.

### L5: ci.sh pre-flight rough edges
- `tools/ci.sh:524` and `:528-529`: a failing Go or TS suite calls `fail` before `v19_preflight after-tests` (`:532`). Pollution left
  by a failing run is exactly what the review's M3 asked to catch in the same run. **Fix:** run `v19_preflight after-tests`
  (report-only) in `cleanup()` before rig down when integration started.
- `:466` prints only `tail -n 20` of the pre-flight log, and FAIL lines are sorted first (`preflight.go:252`). With more than ~19
  findings, the interface named by the gate message has scrolled off the console. **Fix:** print `grep '^FAIL'` first, or sort FAIL
  last.
- The pre-flight's FAIL on an **existing non-agent interface** (the rig's af_packet on an index with a dormant slot) has no repair
  path. `rig down`/`up` gets the same index back (LIFO), so CI stays red until someone resurrects the table by hand. These dormant ip
  and L2 slots are not crash vectors (answer 1), yet the message says "crash vector". **Fix:** add an explicit, manager-only
  `vrx-vpp-preflight -repair <sw_if_index>` that runs `ifsanitize.Sanitize` on it (the worker's open question), or document the
  manual procedure.
- Residual false positive (accept, document): under the shared lock (`after-rig-up`, `after-tests`), a table that another slot
  creates, binds and deletes entirely between the two snapshots is reported FAIL.

### L6: quarantine observability
`ifsanitize/metrics.go:20-21` and `:63-67`: `vrx_agent_iface_quarantined` counts only this process. After an agent restart it drops
to 0 while the holders are still in VPP, so an alert goes quiet. `Release` is not called (Q2 → P08).

**Fix (with P08's wiring):** at start and on full resync, set the gauge from a dump of `quarantine:<owner>` holders, then call
`Release`.

### L7: af_packet: Acquire's rollback delete must quiesce the veth as well (D-101/V24)
`af_packet/host_interface.go:74-77`: the `del` closure calls `AfPacketDelete` with the veth possibly up. It is a second af_packet
delete path besides Delete (`:103`). **Fix:** TD-5's scope ("the agent's af_packet Delete quiesces first") must cover both paths.
Note it on the TD-5 row.

### Info: verified or noted
- Previous findings: H1 fixed (L3 reset + resurrect + quarantine; host-proven for L2 input, L2/ip4 output and L2 policer to a
  *deleted* table). H2 fixed (probe over live + placeholder ids). H3 fixed in the branch (every Delete). M3 fixed (placement,
  two snapshots, exit 2). M4 fixed (quarantine holder = WARN). M5 fixed (lcp wired; the VPP-handled and unhandled lists are in
  `interface.md` and V23(b)). L1 fixed (`phase` label, gauge). M1 narrowed and documented. M2 partly done (chained tables, errors);
  the remainder is documented.
- `set_int_l2_mode(MODE_L3)` on a fresh interface: `l2_if_adjust` = 0 (so no promisc/`l2_if_count` change), and for an ethernet port
  only the 1/2/3-tag match flags of an already-L3 port are cleared. On the host it was a no-op for every type I ran.
  `vxlan_add_del_tunnel` itself sets `feature_bitmap = DROP` before admin-up (`vxlan.c:511-519`), so vxlan has no create→sanitize
  window.
- The reset of the ip classify table on an addressed interface removes an absent `FIB_SOURCE_CLASSIFY`: `fib_table_entry_special_remove`
  tolerates this (`fib_entry.c:1071-1080`).
- The host test `TestV19FreedTableOnHost` deliberately plants a real L2 crash-vector state (L2 ACL bit + slot to a freed table) on the
  shared VPP for a few ms. It has rescue logic and needs bridge + frame to fire. It is acceptable, but it could hold the lab lock
  exclusively for its plant/Create window.
- The fake model's `classify_add_del_table(is_add=0)` of a free index returns -6. VPP returns success (tolerates multiple frees).
  `dropPlaceholders` re-checks with `classify_table_info` first, so no bug hides behind it. Align the fake when touching it.
- Not handled and not used by the agent: the per-interface pcap classify chain (`classify_pcap_set_table`,
  `cm->classify_table_index_by_sw_if_index`). It survives delete, and a later `classify_pcap_set_table(idx, ~0)` *deletes* the stale
  table index with its chain (`vnet_classify.c:1776-1784`), possibly another owner's table. Add it to V23(b) as known.
- Metrics: `Release` counts its holder sanitize as `phase="create"` (inflates `inherited_total{create}`); cosmetic.

## Required before merge
- **H1:** rebase on main; wire `ipsec.itf` and `wireguard.interface` through `Acquire` / `BeforeDelete`; add the creator guard test.
- **M1:** re-read on stuck holes; lower the cap; count cap hits.
- **M2:** reserved high instance for the quarantine holder.

Unit tests for each. Host check of `ipsec` itf / `wireguard` creation on slot 2: `NRestarts` before and after, no packets.

M3, L1–L7 can go to one TD row (cost bound + exact readback before any production image; L3/L4/L5/L6 small). L7 goes on TD-5's row.

**APPROVE WITH CHANGES**
