# DF-7 — review (independent reviewer, 2026-09-24)

Branch `task/DF-7` @ 27a742a, reviewed in `/root/ngfw-wt/DF-7` on the host, slot 10 (`w10`), `VRX_DF7_GLOBALS` unset.
Paths below are relative to `apps/agent/internal/descriptors/` unless they start with `docs/` or `/root/vpp`.

## What I ran

- `tools/ci.sh --base main` → `CI GATE PASSED` (quick, 1m11s; only warning: the merge commit subject is not
  Conventional Commits). Matches the output pasted in `DF-7.md`.
- Host tests, one package at a time, `VRX_INTEGRATION=1 go test -count=1 -v -run OnHost`, and
  `systemctl show vpp -p NRestarts` before and after each package: **2 before and 2 after every package**. All PASS.
  Skips: `policer.bind` (no workers), `lldp.global` / `lb.conf` / `bfd.echo-source` / `igmp.group-prefix` / MPLS table 0
  (globals opt-in). `lldp.interface` was also **skipped in my run** ("no loopback with sw_if_index == hw_if_index").
- Leftovers after my runs: none of slot 10 apart from lb (see M3).

Checklist items with nothing to report: (1) contract: `git diff --name-only main...HEAD -- packages apps/agent/gen
apps/agent/binapi tools` is empty. (4) binapi provenance: every message compiles against `apps/agent/binapi/`, and the
branch does not touch `binapi/` or `tools/binapi-gen.sh`. (6) no `exec.Command`/`vppctl` in the descriptors (the only
`cli_inband` calls are read-only `show` commands in the test helpers). (8, 10) no UI. (9) no scope creep found.

## Findings (most severe first)

### H1 — D-076 "applied once" records use the PID alone and ignore the interface → features are silently skipped after a reboot or an interface re-creation; the OOB write it guards against can come back
`df7/applied.go:80-83` → `interface/identity.go:14` (the VPP main-thread PID from `show_threads`); records keyed by the
object key only: `policer/attach.go:77-86`, `policer/attach.go:131-139`, `lb/lb.go:604-609`, `lb/lb.go:632-640`.

D-080 says every persisted record uses (kernel `boot_id`, VPP PID, VPP start time), and per-interface records are
keyed on sw_if_index **and** logical name. This branch implements neither. P05 is told to install a persisted
`AppliedStore` (Q7), so both gaps matter in the product:
1. **Reboot:** VPP gets the same main PID after a reboot (early-boot PIDs repeat on the same image). The persisted
   record matches, so `policer.interface` and `lb.intf-nat` are **never re-applied**. These types are write-only, so
   nothing detects the drift. The policer (a security/QoS control) is simply absent. It gets worse: if the object is
   later deleted, `AppliedNow` says true and `Delete` sends `policer_input(apply=0)` on an interface that never had
   a policer. That is the out-of-bounds write the author documents in `attach.go:112-116`, on a shared VPP.
2. **Interface re-created in the same VPP lifetime** (tap recreated, interface lost and re-created by DF-1, the fast-mode
   restart simulation "delete prefixed objects, start agent"): the key is unchanged and the PID is unchanged, so the
   feature is skipped on the new sw_if_index (the V19 case in reverse).

Fix: store the D-080 triple plus the sw_if_index in the record. `ApplyOnce`/`AppliedNow` must match all of them.
Extend the fake to cover "same PID, different start time/boot_id" and "same key, new sw_if_index", and add those
unit tests.

### H2 — `mpls-route` Retrieve reports other features' labels in table 0 as its own → no convergence (or deletes them)
`mpls/mpls.go:535-579` (Retrieve walks every entry of every owned table), `mpls/mpls.go:515-531` (Delete = API-sourced
del with no paths). VPP's `mpls_route_dump` walks the whole table whatever the FIB source
(`/root/vpp/src/vnet/mpls/mpls_api.c:500-531`).

In the product the owner must own `mpls-table/0`: DF-6 `sr_mpls` depends on it, `mpls-interface` needs it (Q5).
Table 0 also holds:
- SR-MPLS BSIDs (DF-6),
- the local labels of DF-7's own write-only `mpls-ip-bind` (the author's note in `docs/agent/descriptors/mpls.md:27`:
  "cannot be told apart from routes"),
- any label from FRR/linux-cp.

All of them come back as `mpls-route/0/<label>/<eos>` objects that nobody desired. P05 plans a Delete, and the Delete
removes the API source. For an entry with another source that is a no-op, so the plan never empties and post-apply
verification fails on every resync. If the source does match, the Delete removes another feature's label. The host
test never covers table 0 (it is opt-in), so this is untested.

Fix: in table 0, report only labels this owner programmed. Use a persisted claim record per (table, label, eos),
tied to the D-080 identity and written only after a successful add. Or refuse `mpls-route` in table 0 and document
that. Add a unit test with a foreign-source entry in table 0.

### M1 — Claim before add, never released on failure (recurring pattern) → foreign objects on untagged interfaces get adopted, then updated or deleted
`df7/ifaces.go:61-70` (`Attach` records the ClaimStore claim), then the VPP add runs, and nothing calls `Release`
if the add fails. Affected: `vrrp/vrrp.go:357-364`, `bfd/bfd.go:310-321`, `bfd.go:476-483`, `span/span.go:119`,
`lldp/lldp.go:214-224`, `mpls/mpls.go:370-374`, the qos record/store/mark Creates, igmp (`igmp.go:264, 354, 540, 611`),
`policer/attach.go:82`, `policer/attach.go:306`, `lb/lb.go:605`.

Scenario (VRRP): untagged interface `X` already carries VR 5/ipv4, created by an operator or another tool. The
desired state adds VR 5 on `X`:
1. `Attach` claims `vrrp.vr/X/5/ipv4`.
2. `vrrp_vr_update` fails with `ENTRY_ALREADY_EXISTS` (`/root/vpp/src/plugins/vrrp/vrrp.c:888-893`), but the claim
   stays (persisted ClaimStore).
3. The next `Retrieve` reports the foreign VR as ours (`vrrp.go:255-275`, `Owned` accepts the claim).
4. P05 plans an Update, and the pool walk (`vrrp.go:405-416`) finds the foreign VR's index. VPP's key check passes
   because the key is the same, so its priority and addresses are overwritten, or the VR is deleted.

The same happens with `bfd_udp_add` EEXIST. Fix: claim only after the add succeeds, release on every error path.
Add a unit test (add fails → no claim → Retrieve reports nothing).

### M2 — lb enum byte-swap: documented, but no runtime guard
`lb/lb.go:342-347`, `lb/lb.go:365-367`. It is documented in `lb.md`, `DF-7-questions.md` Q1 and V20. The swap is
right for today's VPP (`/root/vpp/src/plugins/lb/api.c:83-111` compares `mp->encap`/`mp->type` without `ntohl`).
But it is unconditional, and V20 proposes exactly the upstream `ntohl` patch, which F-vpp-debs could ship. On a
patched VPP every non-zero encap/type becomes wrong. `lb_vip_dump` already reports the VIP *type* correctly (`htonl`,
`api.c:256`), and `Create` already calls `DumpVIPs` on `VALUE_EXIST`. Fix: after a successful add, dump the VIP and
check that the reported type matches the requested encap. On a mismatch, delete the VIP and fail with a clear error
("lb enum byte order changed — V20 patched?"). Add a unit test with a fake that decodes enums with `ntohl`.

### M3 — lb leftovers grow on every run; Q1 understates them
Host state after my single lb run: `show lb` → `#vips: 27 #ass: 24`. `show lb vips verbose` lists **26 removed slot-10
VIPs** (9× `10.10.30.1/32`, 8× `.2`, 8× `.3`, 1× `.6`). Table 0 `10.10.31.1/32` is a recursive-resolution drop with
`refs:8`. Q1 lists four VIPs. Each `TestLBOnHost` run adds three more removed VIPs and three ASes' RR references,
because VPP frees them only through the CLI GC (`cli.c:136/263/333`; the `lb_conf` API handler does not GC). In the
product, every `lb.vip` Update (`ErrRecreate`) leaks one pool entry and its FIB tracking until VPP restarts.

Fix:
- make `TestLBOnHost` opt-in (e.g. `VRX_DF7_LB=1`) until V20 is fixed, as done for the crash tests (D-064);
- correct the numbers in Q1 and V20;
- state the per-update leak in `lb.md`.
The global CLI GC is a manager decision (D-071).

### M4 — `lb.vip` / `lb.as` take over existing objects and fail on absent ones
- `lb/lb.go:388-401`: `VALUE_EXIST` is success when a VIP with the same prefix/port/encap exists.
- `lb/lb.go:520`: `lb.as` treats `VALUE_EXIST` as success.
- `lb/lb.go:410-416` and `544-550`: Delete sends the delete unconditionally.

VIPs are untagged global objects. D-071 allows them only through a claim record, but here an operator's VIP is
adopted and later deleted. Also, after a VPP restart, deleting a VIP that is no longer desired returns
`NO_SUCH_ENTRY`, which is an error, so the object is stuck. D-074 requires checking the object exists before
deleting it. Fix: Delete tolerates `NO_SUCH_ENTRY`, or checks with `DumpVIPs` first. Adoption requires a claim
record written after our own successful add.

### M5 — `policer` Update and `Reset` use a stored index without re-checking identity
- `policer/policer.go:79`: `policer_update(PolicerIndex: m.Index)`.
- `policer/policer.go:220`: `Reset(index)`.

Delete re-checks the index (`policer.go:98-114`); Update does not. If the Meta outlives a VPP restart (update before
resync), or the index was freed and reused by another owner, Update overwrites another owner's policer config.
Fix: same pattern as Delete — `dumpV2(idx)` and a name check, otherwise `LookupIndex` — immediately before the update.

### M6 — LLDP index mismatch leaves LLDP transmitting on the wrong interface
`lldp/lldp.go:229-238` returns `ErrIndexMismatch … disable it by hand`. At that point VPP has already enabled LLDP on
the interface whose **hw** index equals our sw_if_index. On the shared host that may be another slot's interface. In
the product, LLDPDUs carrying our system name, port description and mgmt IP go out another port. VPP's disable path
uses the same mapping, so the descriptor can undo the enable at once. Fix: on a mismatch, `set(ctx, idx, i, false)`,
then check with `lldp_dump` that the stray entry is gone, then return the error. Add a unit test.

### L1 — Reference-counted enables are removed in full on Delete
`mpls/mpls.go:396-409` (up to 64 disables until `mpls_interface_dump` shows the interface off); `qos/qos.go:217-227`
(`disableAll`). This protects against the u8 wrap, but it also removes enables taken by any other consumer of the
same counter. One disable, sent only while the dump shows the interface enabled, avoids the wrap without doing that.
Document which is intended.

### L2 — VRRP: VR addresses and ids are not bound to the owner's range; accept-mode masters add connected prefixes
`vrrp/vrrp.go:81-105`. Interface ownership is enforced: foreign-tagged interfaces are refused, and a tracked
interface may be referenced but never claimed. So a VR cannot be put on another slot's interface, and `Delete` finds
the VR by key after re-resolving it (`vrrp.go:423-441`). That answers the "can our VR take over another slot's
interface" question: no, except through M1.

Addresses, however, are unchecked. With `accept`, a master VR makes VPP add the virtual addresses to the interface,
with a default `/24` when no address on the interface matches (`vrrp.c:318-360`). On the shared host a mistyped
address injects a connected route into table 0. Also, the claim in `DF-7.md:338` ("no accept-mode master") depends on
test timing: VR 2 is accept+unicast and started. Fix: optional address-range option (tests: `10.<slot>.0.0/16`);
state in `vrrp.md` that accept mode adds interface addresses.

### L3 — VRRP pool walk: safe, but blind
`vrrp/vrrp.go:378-417`. Verified against VPP: `vrrp_vr_update` rejects an index whose VR has another
(sw_if_index, vr_id, af) with `INVALID_ARGUMENT` **before** any side effect, and a free index with `NO_SUCH_ENTRY`
(`vrrp.c:721-751`). The walk after an agent restart is therefore correct, and the host test proves it finds the same
index. Two caveats:
- an `INVALID_ARGUMENT` caused by our own VR's parameters looks the same as "another VR", so the walk goes through all
  4096 slots and ends with a misleading "pool index not found";
- after an agent restart every Update walks, because the Retrieve Meta has no index.
Fix: probe the stored index first (already done), and cache the found index back into the Meta.

### L4 — IGMP accepts every `-1 (UNSPECIFIED)` as "already there"
`igmp/igmp.go:268`, `290`, `615`. Mode drift (enabled as host, desired router) and any other `-1` failure are hidden,
and these types are write-only. At least document it in `igmp.md`, or re-enable with the desired mode when the
in-memory `modes` map disagrees.

### L5 — Proof gaps
- `lldp.interface` was skipped in my host run (no aligned loopback), so its only host proof is the author's run.
- `policer.interface` "write-only" subtest asserts nothing on VPP (`policer/integration_test.go:52-70`). The feature is
  shown only by the pasted `show interface features`.
- `RestartSimulation` removes objects with the descriptor's own Delete (`df7/df7test/host.go:327-340`), not with raw
  binapi as the fast-mode rule says.

Acceptable for write-only types, but P05's resync test should cover `policer.interface` and `lb.intf-nat` after
H1 is fixed.

### L6 — Key ambiguity
`scheduler.Join` does not escape `/`. `span.mirror/<src>/<dst>/<dev|l2>` is ambiguous when both names contain `/`
(VPP names of untagged physical interfaces or sub-interfaces). Low probability after D-069 logical names. Reject
`/` in names, or escape.

### Info
- **Codec (`df7/codec.go`)**: no int64 precision loss. The only 64-bit spec fields are the policer bursts, and they
  are bounded below 2^53 (`policer/spec.go:117,150`). Presence is collapsed (zero == absent) on both the desired and
  the retrieved side, which is consistent for D-055 structpb values. When P03b swaps in leaf messages with `optional`
  (D-039), re-check fields where 0 is meaningful: `qos.store.value` (DSCP 0), `lb.conf` 0 = keep current.
  `DisallowUnknownFields` is good.
- **BFD secrets**: resolved only through `Secrets` at Create. They never appear in a Value, in Retrieve, in errors or
  in logs, and the request copy is zeroed (`bfd/bfd.go:185-210`). Fixtures use `VRX_TEST_PSK_<id>`, and gitleaks is
  clean. The resolver's own slice is not zeroed. A new secret under the same conf-key id is never pushed (documented:
  rotate by id).
- **MPLS keys**: `mpls-table/<id>` and `mpls-interface/<name>` match what DF-6 uses (`df6/keys.go`,
  `sr_mpls` → `mpls-table/0`). `mpls-interface` → `mpls-table/0` is optional, so on a VPP without table 0 the Create
  fails with `NO_SUCH_FIB` instead of waiting. Q5 should be answered with a LOG entry saying table 0 is declared by
  the globals owner. Deleting a table re-checks ownership (`mpls.go:289-302`), and routes depend on their table, so
  V15 is covered.
- **Watch** (`df7/events.go`): when one watcher's ctx ends it unregisters the `want_*` registration shared by the
  whole client connection.
- **Slot prefix / cleanup**: loopbacks, tables, VR ids, labels and addresses are all from slot 10.
  `t.Cleanup` is in place. After my runs nothing of slot 10 was left except the lb items in M3.

## Required before merge
H1, H2, M1, M2, M4, M5, M6 fixed with unit tests. For M3: make the lb host test opt-in and correct the numbers in Q1.
L-items are optional but should be documented.

**APPROVE WITH CHANGES**
