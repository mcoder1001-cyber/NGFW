# F-vlan-qinq — questions for the manager

Worker slot 5 (fix round 1: slot 12, D-128), branch `task/F-vlan-qinq` (speculative on `task/W-seed`@8b7558e). I keep
going on every item; nothing here blocks the task.

## Q0 — INCIDENT: VPP SIGSEGV during my second topology run (NRestarts 0 → 1, 18:41:08) — ANSWERED, root cause corrected
**Root cause (review 674b03e H1, from the core with the vpp-dbg symbols; manager D-128):** the crash was **`vppctl show
trace`**, which my test issued after each ping of its packet phase (the second call of run 2 crashed; the first one
returned). `format_vlib_trace` (`src/vlib/trace.c:159-162`) calls `node->format_buffer` without a NULL check. A trace
record keeps the `node_index` of a per-interface tx node; when that interface is deleted the node is renamed
`interface-N-tx-deleted` and recycled for the next hardware interface, and a reuser without a tx trace formatter (most
likely a Loopback, `ethernet_simulated_device_class`) leaves it with none. The crashing record was run 1's own
`host-w5w0-tx` (node 818, deleted and reused since); the shared-host rule "never clear trace" keeps such records
forever, so **any** slot's `show trace` after interface churn can jump to address 0. It is **not** the 802.1ad frames,
not the V19 classify reset and not V24: the fault is in the CLI process (`unix_cli_process` → `cli_show_trace_buffer` →
`format_vlib_trace` → `va_format` `call *(%rax)` with `%rax` = 0; see the review for the unwind), and 802.1ad frames take
the same af_packet input path as 802.1Q ones (`plugins/af_packet/node.c:367-383`, only the TCI differs) — run 1's trace
shows them dropped cleanly as `ethernet-input: unknown vlan`.

My first reading below ("V24 or the packet path, dot1ad frames") was wrong and is kept only as the timeline.

- **What changed on the branch (fix round 1):** no `trace add` / `show trace` in any test of mine (abe6d50); the packet
  phase proves the dot1q and dot1q-in-dot1q path with the answered ping plus the rx/tx packet counter deltas of exactly the
  pinged sub-interface (`vppctl show interface <sub>`, never cleared). It stays opt-in (`VRX_QINQ_PACKETS=1`, D-126/D-128),
  and it still sends no 802.1ad frame — because such a frame cannot reach a dot1ad sub-interface on af_packet (V-new
  (F-vlan-qinq), a lab limit), **not** as a safety measure.
- **The VPP-code item** (`format_vlib_trace` NULL guard) belongs to TD-20: its envelope owns that `docs/vpp-code-track.md`
  V-new, together with the ban step in `tools/ci.sh` and `shared-host-rules.md` §11. I did not add a duplicate. TD-20's
  ban patterns (from `task/TD-20:tools/ci.sh` `do_trace_ban`), run over this branch's non-doc files since W-seed: 0 hits.
- **Clean re-run done** (fix round 1, slot 12, packet-free, HEAD abe6d50, 19:43): `TestVlanQinqTopology` PASS, NRestarts
  1 → 1. Pasted in `F-vlan-qinq.md` "Fix round 1".

Original timeline (18:41, slot 5), kept as recorded:
- **Crash:** `vpp[8760]: received signal SIGSEGV, PC 0x0, faulting address 0x0` at 18:41:08; systemd restarted VPP at
  18:41:26 (new pid 2006833). Core kept by systemd-coredump (`coredumpctl list` → pid 8760, 38.6M); the manager keeps a
  copy in `/root/ngfw-wt/logs/crash-20260924-1841/`.
- **Timeline (run 2 started 18:38:40):** 18:40:38 rig hand-over (veths down, D-101, then `af_packet_delete host-w5w0`);
  18:40:43 agent starts; 18:40:55–59 commits (parent, then the three sub-interfaces); V19 guard + `vrx-vpp-preflight` OK;
  veths up; VLAN devices in `ns-w5-wan`; ping over `.100` answered and its `show trace` returned; then the dot1ad ping
  (no reply; run 1 showed such frames dropped as `unknown vlan`) and the phase's second `show trace` → VPP dead at
  18:41:08 (the core cannot name the CLI session; the review matches the timing to this call).
- **Also in the journal then, not mine:** 18:38:42 `fib/entry: BUG: ipv4 table 2100 (index 2) is not empty`; 18:40:59
  twelve `hw_add_del_mac_address: … Secondary MAC Addresses not supported for interface index 0`.
- Run 1 (18:20) and both screenshot runs were clean (NRestarts 0 → 0): the trigger needs a stale record whose tx node was
  reused by an interface without a formatter in between, hence not deterministic.

## Q1 — DEFECT (fixed on this branch, affects P08 and later interface features): removing a sub-interface rolls back
**Found by** `apps/agent/internal/desired/interfaces_qinq_test.go` `TestQinQDeleteWhileParentStays` (fake VPP, product wiring):
removing an **enabled** sub-interface (dot1q or QinQ) from the document while its parent stays — a rollback to a revision
without it, or *remove sub-interface* + commit in the P08 drawer — ends `APPLY_STATUS_ROLLED_BACK`:
```
delete interface.admin-state/host-w5w0.200: sw_interface_set_flags: VPPApiError: Invalid sw_if_index (-2)
results: interface.subinterface/host-w5w0.200 DELETE REVERTED · interface.admin-state/host-w5w0.200 DELETE FAILED · interface-ip/… SKIPPED
```
**Cause:** the sub-interface's attributes depend on the alias `interface/<parent>.<id>`; the alias is observe-only (D-065,
never deleted), so it is never a node of a delete plan and `scheduler.topo` drops the edge. The sub-interface itself depends
on the *parent's* alias, which is a node while the parent stays, so it sorts after its attributes and is deleted first.
**Fix (A-list file, proven defect):** `descriptors/interface/subinterface.go` implements `scheduler.KeyProvider` and
provides `interface/<parent>.<id>` (e771ecb). Both new tests fail without it and pass with it; `internal/{agent,scheduler,
subsystems,descriptors/interface}` stay green.
**For the manager (not changed — not my files):** the same gap exists for every creator whose attributes hang off an
observe-only alias. Today af_packet and loopback deletes come out right only by registration order (admin-state is ranked
after them), and interface addresses / VRF bindings of an af_packet interface are deleted *after* the interface (core
tolerates it: `ErrNotOwned` → nil). Options: (a) each creator provides its alias key (af_packet, loopback, bond, …) — one
method each; (b) `scheduler.topo` follows dependencies through non-node objects (observe-only aliases) to plan nodes —
one generic fix in P05's scheduler; (c) leave as is (only sub-interfaces were actually broken). I recommend (b), or (a) for
F-bonding's `bond.bond` at least.

## Q2 — the fake agent reports `innerVlanId: 0` for every sub-interface
`apps/api/src/testing/fake-agent.ts` `interfaceState()` hard-codes `innerVlanId: 0` (P08's file, P5 hotspot). My API e2e
(`apps/api/test/e2e/vlan-qinq.e2e.test.ts`) adds the real agent's row for `host-w5w0.200` through `liveExtra` (the API keeps the last live row of a
name, so it replaces the fake's own; the real agent reports the inner tag, proven on the fake VPP and on the host). Proposal: the fake takes `innerVlanId` from the applied sub-interface
(one line) in the next P08/F-* round that owns the file.

## Q3 — no `vlan-qinq-*.json` schema example
The envelope lets me own `packages/schema/examples/vlan-qinq-*.json`, but `packages/schema/src/examples.test.ts` fails on
any file outside `GROUP_A` / the `SIBLING` prefix list (`nat|objects|acl|vpn|tunnels|services|ha`), and that test is not
mine. I added no example file; the QinQ cases live in `semantic/interfaces-qinq.test.ts`. If the manager wants a corpus
file (it would also feed the proto drift tests), add `vlan-qinq` to `SIBLING` (or an `interfaces` group) first.

## Q4 — open question from the prompt: exact-match for every sub-interface
P08 hard-codes `exact-match` (`desired/interfaces.go`, routed L3 sub-interfaces). TNSR also offers non-exact-match (a
dot1q 100 sub-interface then also takes frames with more tags) for L2 use. **Recorded, not decided:** the product model
keeps exact-match only (default = no new field); F-bridge-l2 decides whether L2 sub-interfaces need a `exactMatch: false`
leaf (additive; its `Subinterface` proto number would come from the manager, wave-A-hotspots §2 has none for it).

## Q5 — stale help text in `interfaces.json` (P08's file, not mine): the QinQ dialog says "not supported by this release"
The generated sub-interface form takes its titles/help from `apps/web/src/locales/{en,fa}/interfaces.json` `field.*`
(`localizeSchema` in the drawer). Two helps now contradict this feature (visible in `docs/user/interfaces/img/vlan-qinq-dialog-en.png`):
- `field.vlanId.help` — en "802.1Q tag (single-tag sub-interfaces)" / fa «برچسب 802.1Q (زیراینترفیس تک‌برچسبی)»
  → proposed en "outer tag: 802.1Q, or 802.1ad when “802.1ad outer tag” is on" / fa «برچسب بیرونی: 802.1Q، یا 802.1ad وقتی «برچسب بیرونی 802.1ad» روشن است»
- `field.innerVlanId.help` — en "inner tag for QinQ (not supported by this release)" / fa «برچسب داخلی QinQ (در این نسخه پشتیبانی نمی‌شود)»
  → proposed en "inner 802.1Q tag of a QinQ (two-tag) sub-interface; empty = single tag" / fa «برچسب داخلی 802.1Q در زیراینترفیس QinQ (دوبرچسبی)؛ خالی = تک‌برچسبی»
The envelope forbids me that file; please apply at merge (like the `basics.md` "Not in this release" line), or allow the
four-string edit on this branch.

## Q6 — `tools/ci.sh` contract guard is flaky (SIGPIPE under `pipefail`), manager-owned file
`do_contract_guard` runs `git log --format=%s "$mb..$TIP" | grep -qiE '^contract(\(|:|!)'` under `set -o pipefail`.
`grep -q` exits at the first match; when that match is early (on this branch: line 23 of 74, P08's `6ce08c2 contract(api-client)`)
`git log` is still writing and dies of SIGPIPE, the pipeline returns 141 and the gate fails with "CONTRACT FILES CHANGED
WITHOUT A CONTRACT COMMIT" although the branch has five contract commits (P08's and W-seed's). Reproduced 5 × in a loop:
```
nomatch rc=141 · match · nomatch rc=141 · match · nomatch rc=141      (pipestatus=141 0)
```
Fix (one line, not mine to make): `git log --format=%s "$mb..$TIP" | grep -ciE '^contract(\(|:|!)' >/dev/null` or read the
log into a variable first. I re-ran the gate until the guard passed and say so in F-vlan-qinq.md.
