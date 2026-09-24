# TD-3 — guard against VPP V19 (D-095)

Branch `task/TD-3`, slot 2 (`w2`), run directly on the host. VPP was never restarted; `NRestarts` stayed **5** before
and after every host run (below). No packet was sent through any interface.

## What was built

### (a) Create-time sanitizer — `apps/agent/internal/vpp/ifsanitize`
`Sanitize(ctx, client, swIfIndex, name)` clears, **binapi only**, the per-interface state VPP 26.06 keeps on a
sw_if_index after the interface is deleted (source-verified in `/root/vpp`: `vnet_feature_add_del_sw_interface`
clears feature arcs on delete, but not these per-index vectors):

| state | how | why |
|---|---|---|
| ip4/ip6 ip classify table (`classify_set_interface_ip_table`) | reset to ~0 (no readback) | **the crash vector**: `ip4_add_interface_routes` reads it directly and puts a classify DPO on the new interface's /32 |
| l2 input/output classify tables | reset (all ~0) | no readback |
| input ACL | `classify_table_by_interface` → unbind if the table exists | readback exists |
| output ACL, policer classify, flow classify | probe: unbind per live classify table; NO_SUCH_TABLE (-65) = not bound | no readback (`policer/flow_classify_dump` read out of bounds, DF-7) |
| ADL (`adl-input` on device-input) | disable (no-op when off) | `feature_is_enabled` is unreliable (V23 a) |
| vxlan bypass ip4/ip6 (V21) | disable (no-op when off) | bitmap survives delete |
| IPsec SPD binding (manager add-on, DF-5 review M3) | `ipsec_spd_interface_dump` → `ipsec_interface_add_del_spd(is_add=0)` | blocks the next SPD bind on the index |

A binding to an already **freed** table cannot be removed by any API call (VPP checks the table); it is dormant (its
feature arc was cleared by the delete) — reported as `unclearable`, logged at warn. Every run is logged at info
(`interface sanitized (VPP V19/V21 inherited state) cleared=[…] reset=[…]`) and counted:
`vrx_agent_iface_sanitize_total`, `_errors_total`, `_inherited_total`, `_cleared_total{state}`,
`_unclearable_total{state}` (exported by the agent's `/metrics`). Plugins that are not loaded are skipped.

Wired into every creator, after VPP returns the index and **before** the object is tagged / reported created (on
failure the interface is removed and Create fails): `interface.loopback` (core), `tapv2.tap`,
`af-packet.host-interface`, `memif.memif`, `bond.bond`, `interface.subinterface` (new `iface.SanitizeAndTag`), all DF-6
interface types via `df6.IfDescriptor` (gre, ipip, 6rd, vxlan, vxlan-gpe, gtpu, l2tp, pppoe), `mpls.tunnel`.
Not wired: `lcp` pairs (linux_cp is not built on this host; the host tap it creates is VPP-internal) — see open questions.

Test support: `ifsanitize/sanitizetest` models the VPP semantics on the fake (stateful `Model`, `Poison`, `Dirty`,
`Order`); `sanitizetest.Clean(f)` answers the sanitize messages in the shared fakes (coretest, ifacetest, df6test,
df7test) so existing descriptor tests are unchanged; `fake.Client.Handles` added.

### (b) Bindings before tables and interfaces
- Every classify-binding descriptor already had mandatory Dependencies on `classify.table/<name>` and
  `interface/<name>` (classify ip-table, l2-tables, input/output ACL; policer.classify; ipfix classify-table;
  ip-session-redirect; ADL on the interface) — now asserted by `TestBindingDependencies`.
- `classify.table` **Delete refuses while bound** (`ErrTableInUse`): `TableUsers` re-reads VPP right before the delete
  (chained tables, input ACL on every interface, punt ACL, ipfix classify, ip-session-redirect) and the Store's records
  for what VPP cannot report (output ACL; new persisted `bindings` records written by the write-only
  `interface-ip-table` / `interface-l2-tables`; a record whose interface no longer exists is ignored and logged).

### (c) Deletes behind the agent's back delete dependents first
`ifsanitize.BeforeDelete` (same clean-up, named for its purpose) runs before every raw interface delete in the restart
simulations and host fixtures: `agent` deleteOwned, core simulated loss, DF-1 `lose()` (tap, memif), alias test, and
the loopback/sub-interface helpers of ifacetest, df2test, df6test, df7test (+aligned), dfkittest, nattest (both), acl.
`TestClassifyOnHost` registers the tables' cleanup before the bindings' (old order deleted tableB while the output ACL
was still bound on a failure path); the policer classify subtest unbinds before deleting its table.

### (d) `tools/ci.sh full` pre-flight
`apps/agent/cmd/vrx-vpp-preflight` (read-only) + `v19_preflight` in `do_integration`, run **before any test** (after
`tools/lab status`) and again **right after `tools/lab rig up`** (the rig's af_packet interfaces may land on a reused
index). It dumps interfaces, classify tables, input ACL (binapi), the bindings VPP only shows via its CLI (`show
inacl/outacl`, `show classify policer/flow` through `cli_inband` — output ACL/policer/flow have no binapi readback),
classify DPOs in `show ip fib`/`show ip6 fib` (the only trace of an ip classify binding), and SPD bindings. **FAIL** (exit 1,
gate fails naming the interface): a binding on an existing interface or a FIB classify DPO to a missing table.
**WARN**: dormant bindings of deleted indices, SPD bindings on deleted or untagged interfaces.

## How it was verified

### Unit (fake VPP)
```
$ go test ./internal/vpp/ifsanitize/... ./internal/descriptors/{core,tapv2,af_packet,gre,classify}/ -run '…' -v
--- PASS: TestPreflightNamesTheOffendingInterface (0.00s)
--- PASS: TestPreflightClean (0.00s)
--- PASS: TestCleanInterfaceOnlyResets (0.00s)
--- PASS: TestInheritedStateIsCleared (0.00s)
--- PASS: TestBindingToDeletedTable (0.00s)
--- PASS: TestPluginNotLoadedIsSkipped (0.00s)
--- PASS: TestErrorsFailAndAreCounted (0.00s)
--- PASS: TestMetrics (0.00s)
ok  	ngfw/agent/internal/vpp/ifsanitize	0.025s
--- PASS: TestLoopbackSanitizesReusedIndex (0.00s)
ok  	ngfw/agent/internal/descriptors/core	0.027s
--- PASS: TestTapSanitizesReusedIndex (0.00s)
ok  	ngfw/agent/internal/descriptors/tapv2	0.022s
--- PASS: TestHostInterfaceSanitizesReusedIndex (0.00s)
ok  	ngfw/agent/internal/descriptors/af_packet	0.024s
--- PASS: TestTunnelSanitizesReusedIndex (0.00s)
ok  	ngfw/agent/internal/descriptors/gre	0.020s
--- PASS: TestTableDeleteRefusesWhileBound (0.00s)
--- PASS: TestBindingDependencies (0.00s)
ok  	ngfw/agent/internal/descriptors/classify	0.030s
$ go test ./...   → 83 packages ok, 0 FAIL;   make lint → 0 issues.
```
Pre-flight on canned real VPP output (`show ip fib` captured on vrx-a):
```
FAIL  interface host-w9l0 (tag w9:host-w9l0): input ACL ip4 bound to classify table 7, which does not exist
FAIL  interface loop292 (tag w2:loop292): FIB ipv4-VRF:0 10.2.91.1/32 has a classify DPO to classify table 0, which does not exist (ip classify binding inherited or left behind)
WARN  interface DELETED (2): input ACL ip4 bound to classify table 0, which does not exist
WARN  interface DELETED (9): IPsec SPD (index 1) still bound: the next interface on this index cannot get an SPD
```

### Host (real VPP, slot 2, one package at a time, lab lock shared by the tests, no traffic)
V19 reproduced **without packets** and cleared before use (`TestV19InheritanceClearedOnHost`): table bound
everywhere on loop291, loop291 deleted raw, loop292 gets the same index and inherits; the classify table exists the
whole time and is deleted last:
```
integration_test.go:208: loop291 sw_if_index 2: ip4/ip6 classify, input/output ACL, policer, flow classify → table 0; vxlan bypass; SPD 2091
integration_test.go:233: INHERITED on loop292 (sw_if_index 2, reused): input ACL ip4 = table 0, SPD bound, 10.2.91.1/32 has a classify DPO:
        10.2.91.1/32 fib:0 index:14 locks:3
          classify refs:1 src-flags:added,contributing,active,
                [@0]: ip4-classify:[0]:table:0
         forwarding:   unicast-ip4-chain
            [0] [@13]: ip4-classify:[0]:table:0
integration_test.go:237: Sanitize(loop292): cleared=[input-acl ip4 table 0 output-acl ip4 table 0 policer-classify ip4 table 0 flow-classify ip4 table 0 ipsec-spd spd-index 1] reset=[ip-classify ip4 ip-classify ip6 l2-classify input l2-classify output adl adl-input vxlan-bypass ip4 vxlan-bypass ip6] unclearable=[] skipped=[]
integration_test.go:261: after Sanitize: no classify DPO on 10.2.91.1/32, input ACL none, SPD none, output/policer/flow probes NO_SUCH_TABLE
integration_test.go:297: loop294 (reused sw_if_index 1 of loop293) created clean by the loopback descriptor; metric inherited 1→2, cleared map[flow-classify:2 input-acl:2 ipsec-spd:2 output-acl:2 policer-classify:2]
integration_test.go:201: classify table 0 deleted after every binding to it was gone
--- PASS: TestV19InheritanceClearedOnHost (0.05s)
```
(b) on the host (`TestClassifyOnHost`):
```
integration_test.go:186: Delete of bound table refused: classify.table: classify table still in use: "w2-t1" (index 1) is still referenced by [classify.interface-ip-table/loop209/ipv4 (record) classify.interface-l2-tables/loop209/input (record) input-acl ip4 on loop209 (sw_if_index 2)]
--- PASS: TestClassifyOnHost (0.07s)
```
All touched host packages, sequentially, `NRestarts` before/after each:
```
### ./internal/vpp/ifsanitize NRestarts(before)=5   ok 0.077s   NRestarts(after)=5
### ./internal/descriptors/classify NRestarts(before)=5   ok 0.144s   NRestarts(after)=5   (TestClassifyOnHost, TestTableDeleteStaleIndexOnHost PASS)
### ./internal/descriptors/core NRestarts(before)=5   ok 0.421s   NRestarts(after)=5   (TestCoreOnHost incl. simulated loss PASS)
### ./internal/descriptors/policer NRestarts(before)=5   ok 0.095s   NRestarts(after)=5
### ./internal/descriptors/tapv2 NRestarts(before)=5   ok 0.122s   NRestarts(after)=5
### ./internal/descriptors/memif NRestarts(before)=5   ok   NRestarts(after)=5
### ./internal/descriptors/bond NRestarts(before)=5   ok   NRestarts(after)=5
### ./internal/descriptors/gre NRestarts(before)=5   ok   NRestarts(after)=5
### ./internal/descriptors/interface NRestarts(before)=5   ok   NRestarts(after)=5   (TestRestartSimulationOnHost 1.67s PASS)
### ./internal/descriptors/af_packet NRestarts(before)=5   ok   NRestarts(after)=5
### ./internal/agent NRestarts(before)=5   ok   NRestarts(after)=5   (TestAgentOnHost, TestAgentProcessOnHost PASS)
```
Pre-flight against the live VPP afterwards: `V19 pre-flight ok: no classify binding or classify DPO points at a missing table (0 warning(s))`;
`vppctl show inacl/outacl type ip4`, `show classify policer/flow type ip4`, `show classify tables`: nothing left.

(d) the ci.sh function itself, extracted and run with the real binary and with a stand-in that reports a crash vector:
```
V19 pre-flight (before-tests): interfaces + classify/SPD bindings on the shared VPP
  V19 pre-flight ok: no classify binding or classify DPO points at a missing table (0 warning(s))
---
CI GATE FAILED — V19 pre-flight (before-tests): a classify binding on the shared VPP points at a deleted classify table — a crash vector for the next packet (VPP V19). The offending interface is named in the log below. No integration test was started; do NOT send traffic through that interface — remove the binding or the interface first (or report it to the manager)
FAIL  interface host-w9l0 (tag w9:host-w9l0): FIB ipv4-VRF:0 10.9.1.1/32 has a classify DPO to classify table 4, which does not exist
```
`tools/ci.sh full` itself was not run by me (slot 12 / exclusive lock are the manager's, D-087).

### CI gate
See "CI" at the end.

## Incident during this task (own mistake, repaired, no crash)
The first run of the host test failed on an assertion (the /32 is only installed while the loopback is admin-up) and
its cleanup deleted loop292 **before** sanitizing it and then deleted the table: index 2 kept an ip classify binding to
freed table 0 (no address, no traffic; `show inacl` showed `2 → 0 DELETED (2)`). Repaired within a minute: a loopback
was created on index 2 and sanitized (ip classify reset; the dormant ACL entries were cleared by the next run once a
table was back at index 0), then deleted. The test now sanitizes in every raw delete and keeps the table on purpose
(`keep`) if an index is taken by someone else. `NRestarts` stayed 5.

## Decisions (for the LOG)
1. Sanitize fails a Create only on an API error; an unclearable binding to a freed table is dormant (feature arcs are
   cleared on delete) → warn + metric, not an error. The first draft refused on "feature still enabled", but
   `feature_is_enabled` answers true for VPP errors (V23 a, verified on vrx-a), so it is not used at all.
2. Output ACL / policer / flow classify are cleared by probe-unbinding every live table (VPP logs a
   `clib_warning` per miss: ~8 calls × tables per new interface). Accepted: no readback exists.
3. SPD stale binding is removed with the first existing spd_id: VPP's unbind does not check the id against the bound
   one, and the dump reports the pool index, not the id.
4. The pre-flight reads the CLI (`cli_inband`) for what binapi cannot show; it is a CI diagnostic, never agent code.
5. Table Delete ignores write-only binding records of deleted interfaces (it would otherwise refuse forever); the
   reused index is sanitized by the next creator.

## Out of scope / left
- Interfaces created by non-agent tooling (`tools/lab rig up`, other workers' raw test helpers outside this tree) are
  not sanitized at create; the ci.sh pre-flight after `rig up` catches the FIB manifestation. `tools/lab` was not
  edited (not in my allowed scope).
- ADL allow-list config per index is **not** reset: `adl_allowlist_enable_disable` with a non-matching fib drops the
  whole per-interface config (`vnet_config_del_feature` → ~0), which would make a later `adl-input` read config ~0.
- An ip classify binding on a deleted index or on an interface without addresses is invisible to any readback, so the
  pre-flight can only see it once it becomes a FIB classify DPO (which is also exactly when it becomes dangerous).
- `mpls.tunnel` sanitize path has no dedicated unit test (shares the same call); lcp pairs not wired (plugin not built).

## Open questions
- V23 (a): DF-2 `classify.output-acl` Create/Retrieve and `adl.interface` Retrieve trust `feature_is_enabled`; they
  should treat `true` as "unknown" unless confirmed. Separate fix task?
- Should `tools/lab rig up` call the sanitizer (e.g. via a tiny `vrx-vpp-preflight --sanitize <if>` mode)?

## CI
`tools/ci.sh --base main` at 1a53f5c (code identical to the final commit; only this file and TD-3-wip.md changed after):
```
  contract guard: HEAD vs main                       0m00s
  generate + generated-output gate                   1m25s
  forbidden patterns (+ gitleaks)                    0m04s
  lint · typecheck · unit tests · build (turbo)   1m25s
  apps/agent: make lint test build                   0m50s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  mode quick · wall time 3m55s · logs /root/ngfw-wt/logs/ci/TD-3-20260924-052206-3012706

CI GATE PASSED
```

---

## Fix round 1 (after BLOCK 46fafdb) — 2026-09-24, slot 2 (w2)

The first fix-round worker died at ~05:45; its uncommitted edits were salvaged as d5116b8 and reviewed: kept (L3-mode
reset, resurrect, `Acquire`/quarantine, `BeforeDelete` in every Delete, per-phase metrics, lcp wiring). This round
finished them: exact resurrect of input-ACL tables + `FreshRun`, `Release`, unit tests, pre-flight M2/M3/M4, ci.sh M3,
a host test for the freed-table case, docs.

| finding | status | how verified |
|---|---|---|
| H1 unclearable accepted; L2 bits crash path | **fixed**: `Sanitize` first sets L3 mode (clears the l2-input/l2-output bitmaps whatever table they name), then resurrects freed table indices with placeholder tables (exactly for input-ACL tables, read back; `FreshRun`=8 fresh indices otherwise), unbinds through them, deletes the placeholders. Anything left is `ErrUnclearable` in the create phase → `Acquire` deletes the interface, parks the index under an admin-down `quarantine:<owner>` loopback and creates the interface again on a fresh index (`ErrNoCleanIndex` after 4 tries). `Release` frees holders that can be cleaned later | unit: `TestBindingToDeletedTable`, `TestFreedInReverseOrder`, `TestUnclearableIsAnError`, `TestAcquire*`; host: `TestV19FreedTableOnHost` (below) |
| H2 output ACL / policer / flow to a freed table invisible | **fixed**: the probe runs over live + placeholder tables, so a binding to a freed index is found and removed (counted as `freed_table_total`) | unit `TestBindingToDeletedTable` (output-acl ip4 → deleted 5); host: `output-acl ip4/l2 table 0 (deleted table, removed through a placeholder)` |
| H3 Delete does not clear bindings | **fixed**: `ifsanitize.BeforeDelete` right before the VPP delete in loopback, tap, memif, bond, af_packet, sub-interface, all DF-6 types, mpls tunnel, lcp pair (host tap) | unit suites of those descriptors pass with the sanitizing fake (`sanitizetest.Clean`); host: `phase=delete` log lines in both host tests |
| M1 table-delete refusal misses stale indices | **narrowed, documented** (`docs/agent/descriptors/classify.md`): VPP refuses every call on a deleted index, so an input ACL there can be neither read nor unbound; with H3 + resurrect such a table delete is no longer a crash vector for our interfaces; output-ACL records keep refusing; TOCTOU note (sequential scheduler) added | docs |
| M2 pre-flight false negatives | chained tables (`next_table_index` missing → FAIL) and API errors no longer swallowed (`classify_table_by_interface` other than INVALID_SW_IF_INDEX, SPD dump) **fixed**; L2 classify tables / punt ACL / pcap filter not inspected (noted) | unit `TestPreflightReviewM2M3M4` |
| M3 pre-flight placement / TOCTOU / exit 2 | **fixed**: `before-tests` now runs while the exclusive lock is still held; second `classify_table_ids` snapshot after the reads (only tables missing in both count); new `after-tests` run after the suites, before rig down (rig down runs either way, then the gate fails); exit 2 has its own message ("could not inspect VPP", not "crash vector") | unit `TestPreflightReviewM2M3M4`; `bash -n tools/ci.sh`; quick gate green (full not run by me) |
| M4 product and CI disagree | **fixed**: pre-flight reports a quarantine holder (tag `quarantine:*`, admin-down) as WARN, everything else as FAIL — the agent's rule | unit + host (`WARN interface loop0 (tag quarantine:w2): …`) |
| M5 lcp host tap not sanitized; docs | **fixed** (salvage): lcp pair create goes through `Acquire` on `host_sw_if_index`, Delete calls `BeforeDelete`; VPP-handled (ACL plugin, NAT44, ADL) and known-unhandled (ABF, NAT64/66/DET44, cnat, flowprobe) listed in `interface.md` and V23(b) | lcp unit tests (5 sanitize runs logged) |
| L1 metrics | **fixed**: `phase="create|delete"` label on every counter, `freed_table_total{phase,state}`, gauge `vrx_agent_iface_quarantined`, `vrx_agent_iface_quarantine_total` | unit `TestMetrics` |
| L2 probe cost | not changed; cost grew: every create now makes ≥ 8 placeholder tables (+ holes) and probes 8 × (tables + placeholders) unbinds. Fine at today's table counts; cap/batch later | — |

### Unit

```
$ cd apps/agent && go test -count=1 ./internal/vpp/ifsanitize/ -v | grep -E '^(---|ok)'
--- PASS: TestAcquireQuarantinesAnUnclearableIndex (0.00s)
--- PASS: TestAcquireFailsWithoutACleanIndex (0.00s)
--- PASS: TestAcquireOtherErrorsDeleteAndFail (0.00s)
--- SKIP: TestV19InheritanceClearedOnHost (0.00s)
--- SKIP: TestV19FreedTableOnHost (0.00s)
--- PASS: TestPreflightNamesTheOffendingInterface (0.00s)
--- PASS: TestPreflightClean (0.00s)
--- PASS: TestPreflightReviewM2M3M4 (0.00s)
--- PASS: TestCleanInterfaceOnlyResets (0.00s)
--- PASS: TestInheritedStateIsCleared (0.00s)
--- PASS: TestBindingToDeletedTable (0.00s)
--- PASS: TestFreedInReverseOrder (0.00s)
--- PASS: TestUnclearableIsAnError (0.00s)
--- PASS: TestPluginNotLoadedIsSkipped (0.00s)
--- PASS: TestErrorsFailAndAreCounted (0.00s)
--- PASS: TestMetrics (0.00s)
ok  	ngfw/agent/internal/vpp/ifsanitize	0.036s
$ go test -count=1 ./internal/descriptors/... ./cmd/...   → 61 packages ok, 0 FAIL
```

### Host (slot 2, `eval "$(tools/lab env 2)"`, `tools/lab lock shared`, no packets)

Run 1, 07:27:23 (`NRestarts=5` before): `TestV19InheritanceClearedOnHost` PASS (the original repro, now with the L3 reset and
placeholders in the log: `placeholders=9 reset="[l2-mode l3 …]"`), `TestV19FreedTableOnHost` FAIL at its first readback —
`l2_flags_get` answers INVALID_SW_IF_INDEX for an interface that is neither bridged nor xconnected (`l2_api.c:428`); the test
now reads the bits with `show interface <idx> feat`. **Then VPP restarted: NRestarts 5 → 6 at 07:27:32, SIGSEGV PC 0x0** —
see `TD-3-questions.md` (cause not established; the classify state in VPP then pointed only at a live table and no packet was
sent; slot 1 had just deleted af_packet interfaces with the known EBADF lines, and my read-only `vppctl show interface
features` was running). No further host runs after I noticed.

Run 2, 07:28:55 (`NRestarts=6` printed before it — noticed only afterwards; `NRestarts=6` after):

```
=== RUN   TestV19FreedTableOnHost
    loop285 sw_if_index 2 (L3 mode): L2 input ACL, L2 output ACL, L2 policer classify, ip4 output ACL → table 0; show interface feat:
        l2-input:
             POLICER_CLAS (l2-policer-classify)
                      ACL (l2-input-acl)
        l2-output:
                      ACL (l2-output-acl)
    after the raw delete of the loopback and of table 0 — show inacl type l2:
                 2                   0		DELETED (2)
    … show outacl type l2:            2                   0		DELETED (2)
    … show classify policer type l2:  2                   0		DELETED (2)
INFO interface sanitized (VPP V19/V21 inherited state) interface=loop286 sw_if_index=2 phase=create cleared=[]
     freed="[input-acl l2 table 0 (deleted table, removed through a placeholder) output-acl ip4 table 0 (…) output-acl l2 table 0 (…)
     policer-classify l2 table 0 (…)]" placeholders=8 reset="[l2-mode l3 ip-classify ip4 ip-classify ip6 l2-classify input
     l2-classify output adl adl-input vxlan-bypass ip4 vxlan-bypass ip6]" skipped=[]
    loop286 created on the planted sw_if_index 2: input ACL none, l2 features "l2-input:\n\n" "l2-output:\n  none configured\n",
    no row in show inacl/outacl/classify policer; classify tables back to []; freed map[] → map[create/input-acl:1 create/output-acl:2 create/policer-classify:1]
    loop287 sw_if_index 1 (L3 mode): … → table 0 (same plant)
ERROR interface sanitize failed (VPP V19) interface=loop288 sw_if_index=1 phase=create err="inherited binding to a deleted classify
      table cannot be removed (VPP V19): [input-acl l2 table 0]"            (MaxPlaceholders=0: resurrection disabled on purpose)
ERROR sw_if_index quarantined: … held by an admin-down loopback so it is never reused sw_if_index=1 tag=quarantine:w2
INFO interface sanitized … interface=loop288 sw_if_index=3 phase=create … placeholders=0
    quarantine: loop288 created on fresh sw_if_index 3; dirty 1 held by loop0 (tag "quarantine:w2", admin-down); gauge vrx_agent_iface_quarantined=1
    pre-flight: WARN  interface loop0 (tag quarantine:w2): input ACL l2 bound to classify table 0, which does not exist
    pre-flight: WARN  interface loop0 (tag quarantine:w2): output ACL ip4 bound to classify table 0, which does not exist
    pre-flight: WARN  interface loop0 (tag quarantine:w2): output ACL l2 bound to classify table 0, which does not exist
    pre-flight: WARN  interface loop0 (tag quarantine:w2): policer classify l2 bound to classify table 0, which does not exist
INFO interface sanitized … interface="quarantine holder 1" sw_if_index=1 phase=create freed="[input-acl l2 table 0 (…) output-acl ip4
     table 0 (…) output-acl l2 table 0 (…) policer-classify l2 table 0 (…)]" placeholders=8
INFO quarantined sw_if_index released: its stale bindings are gone (VPP V19) sw_if_index=1 tag=quarantine:w2
    Release: holder of 1 sanitized through placeholders (table 0 resurrected) and deleted; classify tables []
INFO interface sanitized … interface=loop288 sw_if_index=3 phase=delete …
INFO interface sanitized … interface=loop286 sw_if_index=2 phase=delete …
--- PASS: TestV19FreedTableOnHost (0.22s)
ok  	ngfw/agent/internal/vpp/ifsanitize	0.239s
```

Pre-flight against the live VPP after the restart and after run 2: `V19 pre-flight ok: no classify binding or classify DPO
points at a missing table (0 warning(s))`.

**NRestarts: 5 before run 1 → 6 (VPP crash 07:27:32, not attributed, written down in TD-3-questions.md) → 6 after run 2.**

### CI

```
$ tools/ci.sh --base main
  contract guard: HEAD vs main 0m01s · generate + generated-output gate 1m34s · forbidden patterns (+ gitleaks) 0m03s
  lint · typecheck · unit tests · build (turbo) 1m37s · apps/agent: make lint test build 0m49s · test/ Go modules 0m02s
  warnings: commit subject(s) not in Conventional Commits form: review(TD-3): findings   (the reviewer's commit)
  mode quick · wall time 4m09s · logs /root/ngfw-wt/logs/ci/TD-3-20260924-073128-3199816
CI GATE PASSED
```

`tools/ci.sh full` was not run (it restarts nothing, but it runs every suite under the lab lock; left to the manager).

### Out of scope / open

- Quarantine release is a library call (`ifsanitize.Release`), not yet called by the agent loop — `internal/agent` is P08's
  (TD-3-questions.md Q2).
- Pre-flight does not inspect L2 input/output classify tables (`classify_set_interface_l2_tables`), the punt ACL or the pcap/trace
  classify filter (M2 remainder); `Sanitize` resets the l2 classify tables blindly.
- `FreshRun` is a bound, not a proof, for the write-only kinds: ≥ 8 tables freed in exact reverse creation order above the highest
  live table could hide one; the input-ACL kind is exact.
- `apps/agent/internal/descriptors/core/coretest/fakevpp.go` import order is not gofmt-clean on main already; untouched.
