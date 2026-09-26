# F-vlan-qinq — review

Reviewer agent, 2026-09-24 19:15–19:45. Branch `task/F-vlan-qinq`@01b80ff, diff base `task/W-seed`@df67a8e
(`git diff task/W-seed...task/F-vlan-qinq`: 32 files, +3767/−65). I did not write this code. No host/VPP runs and no
`tools/ci.sh` run (the caller's scope). I ran unit tests, a read-only analysis of the Q0 core file, and read the VPP 26.06
sources in /root/vpp.

**Verdict: APPROVE WITH CHANGES.** H1, M1 and M2 must be fixed before the merge. The code (the delete-order fix, the
table, the tests, the docs) is sound. The main result of this review is **Q0's root cause**: the VPP crash at
18:41:08 came from **`vppctl show trace`**. The test calls it in its packet phase, and P08's topology test calls it too. It
did not come from 802.1ad frames, the classify reset or V24. D-126's rules therefore target the wrong trigger.

## Findings, ranked

### H1: `show trace` on the shared VPP crashed it (Q0 root cause); the opt-in packet phase still calls it
- **Where:** `test/topology/vlan-qinq/qinq_test.go:411` (`trace add af-packet-input 60`), `:423` (`show trace max 5000`,
  after every ping). The same pattern is in P08's `test/topology/interfaces/interfaces_test.go:291/297`, which `tools/ci.sh full`
  runs, and in F-nat44-ed-sessions `test/topology/nat44-ed-sessions/nat_test.go:409/422`.
- **Evidence:** I unwound the core without gdb. A Python ELF-core reader finds the kernel signal frame on the vpp_main stack
  (right after `__restore_rt` = libc+0x45cb0). I symbolized it with `addr2line` against the already-built
  `/root/vpp/build-root/vpp-dbg_26.06-release_amd64.deb`, whose build-ids match the installed libraries. Real output:
  ```
  signal frame: rip=0x0 rsp=0x72f2aa3718f8 rdi=0x72f2d1cd1d48 (the vec being formatted) rsi=0x72f2aa3719d0 (va_list)
  [fault rsp] = libvppinfra+0x266ef  → va_format (vppinfra/format.c:388): 266ed: ff 10  call *(%rax)   ← the %U function pointer is NULL
    format (format.c:449) ← format_vlib_trace (vlib/trace.c:164) ← cli_show_trace_buffer (trace.c:336)
    ← vlib_cli_dispatch_sub_commands (cli.c:608) ← vlib_cli_input (cli.c:712) ← unix_cli_process (unix/cli.c:2595)
  tail of the text being formatted (44407 bytes, record "Packet 21"):
    05:18:04:678270: af-packet-input … vlan 100 vlan_tpid 33024
    05:18:04:678710: ethernet-input   ARP: 02:4f:57:07:18:8f -> ff:ff:ff:ff:ff:ff 802.1q vlan 100
    05:18:04:678935: interface-7-output-deleted   sw_if_index: 3 …
    05:18:04:678978: interface-7-tx-deleted
  next va arg = h->data; trace header: node_index=818 n_data=24
  ```
- **Mechanism (source-verified):** `format_vlib_trace` (`src/vlib/trace.c:159-162`) calls `node->format_buffer` without
  a NULL check when `node->format_trace` is NULL. Trace records keep the `node_index` of per-interface output and tx nodes.
  When an interface is deleted, `vnet_delete_hw_interface` renames those nodes `interface-N-*-deleted`
  (`src/vnet/interface.c:1106`) and pushes them onto a LIFO recycle list. The next hardware interface to be created
  reuses them and sets `format_trace = dev_class->format_tx_trace` (`:943`). For `Loopback`
  (`ethernet_simulated_device_class`, `src/vnet/ethernet/interface.c:729`) that value is NULL. The crashing record is from
  **this test's own run 1**: VPP time 05:18:04 is run 1's `.100` ARP, and run 1's pasted trace shows the same netns MAC.
  Its tx node 818 was run 1's `host-w5w0-tx` (hw_if_index 7). Some interface without a tx formatter (a Loopback on
  another slot, most likely) later reused node 818 and was deleted again as hw_if_index 7, which left node 818 with no
  formatter. The shared-host rule "never `clear trace`" keeps such records indefinitely. After any interface churn, any
  slot's `show trace` can therefore jump to address 0. Run 2's first `show trace` (after the dot1q ping) survived and the
  second one crashed, so node 818 changed state in those few seconds. That explains why the crash is not deterministic.
  The core cannot show which CLI session issued the call, but the timing matches this test's second `show trace`.
- **What it rules out:**
  - **802.1ad frames:** the fault is in the CLI process, not in an input node. `af_packet` input handles dot1ad and dot1q
    on the same code path (`plugins/af_packet/node.c:367-383`). The only difference is the TCI value, and then
    `ethernet-input: unknown vlan` drops the frame, as the pasted run-1 trace shows.
  - **The test's V19 reset** (`vpp_test.go` `v19Guard`, writing ~0 on its own 4 interfaces): with ~0,
    `vnet_set_ip4_classify_intfc` calls `fib_table_entry_special_remove(…, FIB_SOURCE_CLASSIFY)`. CLASSIFY (prio 0x01) is a
    better source than INTERFACE (0x03), and `fib_entry_special_remove` returns ADDED for an absent better source
    (`fib_entry.c:1072-1080`), so nothing is removed. The L2 reset only clears feature bits. The guard also ran before the
    successful pings of runs 1 and 2.
  - **V24:** the fault is not in the epoll/file path.
  - The policer/flow/in-out-ACL "Non-existent intf_idx" sweeps at 18:21:33 and 18:40:57 are TD-3's `ifsanitize` inside
    the agent, not this branch's code, and they are not on the crash path.
- **Fix on this branch:**
  - Remove `trace add`/`show trace` from the packet phase. Prove the dot1q and dot1q-in-dot1q path with the ping result
    plus per-sub-interface rx/tx counters (`show interface <sub>`, or `sw_interface_dump`/stats), or with `tcpdump` in
    `ns-w5-wan`.
  - Correct the Q0 explanation in these places: the test header (`qinq_test.go:10-13`), `F-vlan-qinq-questions.md` Q0
    (lines 6-35), `F-vlan-qinq.md:41-43`, `:218-219`, `:391-395` and D-VQ-4 (`:412`, whose rationale is wrong).
  - Append a second `### V-new (F-vlan-qinq)` row to `docs/vpp-code-track.md`: "format_vlib_trace NULL `format_buffer`
    on recycled interface nodes". The VPP fix is a one-line NULL guard in `trace.c`, optionally plus dropping trace records
    of deleted nodes. The fallback until then: no `show trace` on the shared VPP.
  - The dot1ad ping exclusion can stay, because it cannot pass on af_packet (V-new), but it is not a safety measure.
- **For the manager (not this branch's files):**
  - Revisit D-126: its rules (no blind classify sweep, no 802.1ad) do not cover the trigger.
  - Ban `show trace` on the shared VPP now, or require `clear trace` directly before `trace add`, which only narrows the
    window. P08's test does this in `ci.sh full` on every run.
  - Re-examine the 07:27:32 crash (same PC 0x0), which was attributed to V24 without a core. V24 records that slot 1 had
    just deleted two af_packet interfaces, which is the P08 rig hand-over pattern. Check whether a P08-style packet phase
    with `show trace` was running then.
  - gdb is not needed for future cores: `vpp-dbg` exists in `build-root`. My reader script is
    `/tmp/claude-0/-root-ngfw/a859b866-b6bf-4dfb-af67-939e5bcdb4dd/scratchpad/corebt.py`, used as
    `zstd -d <core>.zst` + `dpkg-deb -x vpp-dbg….deb <dir>` + `python3 corebt.py <core> <dir>`.

### M1: the merge gate's contract guard will fail once W-seed is on main
- `packages/schema/src/semantic/interfaces-qinq.test.ts` is under `CONTRACT_PATHS` (`tools/ci.sh:50`). The guard passes
  today only because W-seed's and P08's `contract(…)` commits are in `mb..HEAD`. Simulated with W-seed as the base:
  `git diff --name-only $(merge-base W-seed F-vlan-qinq) F-vlan-qinq -- <CONTRACT_PATHS>` gives `…/interfaces-qinq.test.ts`,
  and the branch has **0** contract commits after W-seed. The pre-merge hook would then fail with "CONTRACT FILES CHANGED
  WITHOUT A CONTRACT COMMIT".
- **Fix:** the manager's choice.
  - (a) Preferred: exclude test files from the guard, e.g. `':!packages/schema/src/**/*.test.ts'`, in `tools/ci.sh`
    (manager-owned).
  - (b) The worker adds a commit `contract(schema): QinQ semantic test cases only, no schema change` and a
    `F-vlan-qinq-contract.md` that says so. The envelope owns the test file, so the conflict is in the tooling, not in the
    worker's scope.

### M2: the committed topology test has never completed a host run
- Run 1 (the pasted evidence) used the test at b8fa14e. The committed test (f764283 + 01b80ff: opt-in packets, CLI step,
  password-file removal) only ran as run 2, which crashed in the packet phase. The pasted CI run is at c06785c, before
  01b80ff's test change.
- I re-checked HEAD: gofmt clean, `go vet` ok and unit-mode `go test` ok (skips without `VRX_INTEGRATION`) in
  `test/topology/vlan-qinq`.
- **Fix:** after H1, one clean `test/topology/vlan-qinq/run.sh -run TestVlanQinqTopology` without packets, NRestarts
  before and after pasted, and a CI re-run at the final HEAD. The manager must grant it (Q0's ask).

### L1: stale and dead code in the test
`qinq_test.go:591-598`: after `if x.dot1ad { continue }` the `proto = "802.1ad"` branch is unreachable. Its comment and
`:414` cite Q0 as the reason. Remove the branch, or keep the dot1ad device code behind the opt-in with a V-new comment only.

### L2: hand-written view types beside the schema types
`apps/web/src/domains/interfaces/subinterfaces/tagStack.ts:14` (`TagStackFields`) and `:49` (`TagStackRow`) restate
fields of `SubinterfaceConfig` / `InterfaceItem`. They are structural, so they compile against both, but rule 5 prefers
`Pick<SubinterfaceConfig,'vlanId'|'innerVlanId'|'dot1ad'>` and `Pick<LiveState,'vlanId'|'innerVlanId'>`, so that a
contract rename breaks the build instead of silently reading `undefined`.

### L3: the API e2e's inner-tag assertion for `.200` is partly tautological
`apps/api/test/e2e/vlan-qinq.e2e.test.ts:84` injects the live `.200` row through `liveExtra`, because of Q2 (the fake
reports `innerVlanId: 0`). The real proof of the inner tag is `TestQinQRoundTripOnFake` (InterfaceState on the fake
VPP) and host run 1. Follow-up: the P5 owner fixes the fake (Q2). Nothing on this branch.

### Info
- **Acceptance wording:** `vppctl show interface` does not print tag stacks. The `sw_interface_dump` rows (flags, outer,
  inner, owner tag) are the proof instead, and the user page says so. Accepted.
- **Out-of-envelope but justified:**
  - `docs/vpp-code-track.md`: the 00-CONTEXT FAST MODE rule requires the entry; it is an append-only `### V-new` block,
    which follows A7.
  - Five new `docs/user/interfaces/img/vlan-qinq-*.png` files.
  - Both are listed under "Shared hunks".

## Q1: the delete-order fix (e771ecb), and whether the scheduler alternative Q1(b) is needed
- **Correct and sufficient for this task.** `SubinterfaceDescriptor.ProvidedKeys` = `interface/<parent>.<id>` follows the
  existing precedent: `wireguard.Interface` and `ipsec.Itf` already provide their `interface/<name>` alias. The subinterface
  is registered unwrapped (`subsystems.go:200`), so the type assertion sees it.
  - In create plans the real alias object is a node and wins (`topo` checks `nodes[target]` first), so nothing changes there.
  - In delete plans the provided key gives the missing edge.
  - `executor.dependents` (recreate) finds the same set as before.
- **Discrimination, re-run by me:** with the pre-fix `subinterface.go` through `go test -overlay`, `TestQinQRoundTripOnFake`
  and `TestQinQDeleteWhileParentStays` fail with `APPLY_STATUS_ROLLED_BACK … Invalid sw_if_index (-2)`, and pass with the fix.
- **My own probes on the fake VPP (overlay, not committed):**
  - Parent and all sub-interfaces removed together: APPLIED, 14 deletes, each sub-interface's attributes before its
    `delete_subif`.
  - A sub-interface with MTU, VRF and address removed while the parent stays: `interface.mtu`, `admin-state`,
    `interface-ip`, `interface-ip.table` are deleted before `interface.subinterface`. With the pre-fix file it rolls back.
- **Q1(b) is not needed to merge this branch.** The general gap is real:
  - Creators without `ProvidedKeys` (af_packet, core loopback) get the right order only through registration rank.
  - The parent's own `interface-ip` is deleted after `af_packet_delete` (tolerated through `ErrNotOwned` re-resolution).
  - DF-1's attribute `Delete`s act on the **cached** `Meta.SwIfIndex` without re-checking identity
    (`attributes.go:146-157`), so a wrong order hits a stale and possibly reused index on the shared VPP.
- F-bonding already adds `ProvidedKeys` for `bond.bond` (D-125), and TD-11c ("alias delete order") owns the general fix.
  Recommendation: TD-11c adds a registry guard test ("every descriptor that is the `creator` of an interface alias
  implements `KeyProvider`") and covers af_packet and loopback. (b) stays optional.

## Checks (REVIEW-PROMPT order)
| # | check | result |
|---|---|---|
| 1 | contract | No schema, proto or generated change on the branch. The only contract-path file is a test, which the guard miscounts (M1) |
| 2 | real verification | Host run 1 against `/run/vpp/api.sock` asserts `sw_interface_dump` rows, Retrieve through `/state/interfaces` `config` == desired, and `vppctl show interface [address]`. The fake-VPP tests use the product wiring |
| 3 | restart safety | Pasted: agent stopped, addresses then `delete_subif` through binapi (D-095c), agent started, all three back in 1.46 s, reconcile 1.332 s from the log. No new object type. The subinterface descriptor keeps DF-1's Retrieve |
| 4 | VPP API provenance | The test uses generated binapi only (`interface`, `af_packet`, `classify`, `acl`, `ipsec`, `ip`). No `binapi/` or `binapi-gen.sh` change |
| 5 | shared host | Slot prefix w5, rig 10.5/16, PIDs stopped, `vrx_w5` dropped, rig down, lock `-s`, NRestarts checked. **But `show trace` (H1)** |
| 6 | security | `exec` only in test code with fixed or derived arguments. The CLI password is in a 0600 file inside a 0700 directory and is removed. No secrets in the status files. No new routes |
| 7 | transaction semantics | Rollback deletes attributes, then sub-interfaces (the Q1 fix, verified on the host and in unit tests) |
| 8 | UI honesty | The table reads the real `/state/interfaces` and the candidate. Screenshots en and fa/RTL are real (headless Chrome, disclosed). No TODO, mock or stub |
| 9 | scope | Nothing extra. D-VQ-3 (`.300` dot1q-in-dot1q) costs nothing and proves the second flavour |
| 10 | i18n | en and fa `vlan-qinq.json` have identical keys (`locales.test.ts` 12/12). Logical `textAlign` start/end, symmetric `px`. Tag notation LTR by design (D-VQ-7) |
| 11 | tests | Pasted CI at c06785c. I did not re-run `tools/ci.sh` (scope). The unit results below match the pasted ones |

**Hotspots:** 
- **W3** `i18n.ts`: 5 lines, each directly under `// wave-A: F-vlan-qinq`. ✓
- **D1** `basics.md:28`: exactly the one line; line 102 untouched. ✓
- **W5** `InterfaceDrawer.tsx`: one block swap plus one import added and two now-unused imports removed (required by
  lint). No W5 anchor or registry was seeded, so the first-toucher rule applies and this branch should merge early. ✓
- **A3** `desired/interfaces.go`: untouched. ✓
- `subinterface.go`: +15 lines, defect-only, with the proving test named. ✓

**Architecture:** 
- Single schema source: the dialog keeps the generated `SubinterfaceSchema` form, and model types come from
  `@ngfw/schema` and the generated client (minor L2).
- The descriptor stays declarative, with DF-1's Retrieve.
- Only binapi names are used.
- No new projection, route or screen.

## What I ran (real output, trimmed to the result lines)
```
$ cd apps/agent && go test -count=1 ./internal/descriptors/interface/... ./internal/desired/... ./internal/scheduler/...
ok  ngfw/agent/internal/descriptors/interface 0.035s · ok  ngfw/agent/internal/desired 0.168s · ok  ngfw/agent/internal/scheduler 0.066s
$ go test -overlay <pre-fix subinterface.go + probes> -run 'QinQ|TestReview' ./internal/desired/
--- FAIL: TestQinQRoundTripOnFake   apply q8: APPLY_STATUS_ROLLED_BACK delete interface.admin-state/host-w5w0.300: … Invalid sw_if_index (-2)
--- FAIL: TestQinQDeleteWhileParentStays   apply d2: APPLY_STATUS_ROLLED_BACK delete interface.admin-state/host-w5w0.200: …
--- FAIL: TestReviewSubWithMtuVrf
$ go test -overlay <probes only> -run TestReview -v ./internal/desired/        (with the fix)
--- PASS: TestReviewParentAndSubsTogether   (status APPLY_STATUS_APPLIED, deleted:14, attributes before each delete_subif)
--- PASS: TestReviewSubWithMtuVrf           (0 mtu · 1 admin-state · 2 interface-ip · 3 interface-ip.table · 4 interface.subinterface)
$ cd packages/schema && npx vitest run src/semantic/interfaces-qinq.test.ts        →  Tests 9 passed (9)
$ cd apps/web && npx vitest run src/domains/interfaces src/locales                 →  Test Files 5 passed (5) · Tests 34 passed (34)
$ cd test/topology/vlan-qinq && gofmt -l . ; go vet ./... && go test ./...        →  (no files) · ok ngfw/test/topology/vlan-qinq 0.022s
```
Web deps were built for the vitest run (turbo `--filter=@ngfw/web^...`), and the `dist/` outputs were removed again
afterwards. The decompressed core (1.4 GB) and the vpp-dbg extraction were deleted from my scratchpad; the script stays.

## Required before merge
1. H1: no `show trace` in the test; Q0, status, header comment and D-VQ-4 corrected; a V-item for `format_vlib_trace`.
2. M1: the manager picks (a) the `ci.sh` exclusion or (b) a `contract(schema)` note commit.
3. M2: one clean packet-free host run of the committed test, plus CI at the final HEAD, both pasted.

L1–L3 can be done in the same round or deferred.
