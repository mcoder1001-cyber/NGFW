# DF-6 review — tunnel / SR / LISP descriptors

Reviewer: independent review agent · branch `task/DF-6` @ 82d0e65 · slot 11 · 2026-09-24

## What I checked and reproduced

- **CI:** I ran `tools/ci.sh --base main` in `/root/ngfw-wt/DF-6`. Result: `CI GATE PASSED` (quick, 0m44s; logs
  `/root/ngfw-wt/logs/ci/DF-6-20260924-010603-1141821`). No contract files changed. All 10 descriptor packages `ok`.
  This matches the pasted output. The gate ran against the branch's old base `af83adb`, and main has moved on since (see L1).
- **Host tests:** I ran them myself on slot 11, one package at a time, with `VRX_INTEGRATION=1` and without the
  gtpu, LISP or l2tp-create opt-ins. `NRestarts` read 2 before and after every package.
  - gre, ipip, vxlan and vxlan_gpe passed. Each printed `re-apply plan … empty=true`.
  - sr: localsid, policy and steering passed, each with an empty re-apply plan.
  - sr_mpls: policy/steering passed; endpoint-color was skipped.
  - l2tp: interface-enable passed; tunnel create was skipped.
  - pppoe: I ran only `TestSessionOnHost`, which skipped. I did not run `TestCpOnHost` on purpose (see M2).
  - gtpu and lisp were skipped (opt-in).
  - Afterwards `show gre tunnel` and `show sr policies` were empty.
- **binapi provenance:** every imported `binapi/*` package exists on main. The branch does not change
  `apps/agent/binapi/` or `tools/`.
- **Forbidden patterns:** no `exec.Command`, `vppctl`, `pkill` or `killall` in DF-6 packages.
- **Slot prefix and cleanup:** fixtures carry the slot prefix and use slot tables, 10.11/16 and fd11::/16 addresses, and
  `w11-` names. Cleanup runs in `t.Cleanup`.
- **Tunnel Retrieve cannot claim a foreign tunnel.** `IfDescriptor.Retrieve` keeps only interfaces that carry our tag
  *and* whose tag id equals the id decoded from the object (`df6/ifdesc.go:191-217`).
- **V8 guard key is correct.** It matches VPP's decap key: `gtpu.c` key4 = (dst, teid), no FIB. The handler's
  pre-checks for NO_SUCH_FIB and the mcast interface `goto out` before the `get_combined_counters` line.
  `RequireTable` guards the unchecked `fib_table_find` in the SR paths.

## HIGH

### H1 — VPP-global switches are registered for every agent and not gated by a globals owner; one agent's Delete switches LISP off for all (D-071)
- **Where:**
  - `lisp/descriptors.go:37-53` (`lisp.enable`), `:56-72` (`lisp-gpe.enable`), `:75-97` (`lisp.pitr`), `:764-777` (`Register`)
  - `sr/globals.go:45,71-73`; `sr/register.go:13-18`
  - `l2tp/register.go:14`
  - `df6/singleton.go:100-127`
- **What is wrong:**
  - The singletons are always registered. `Delete`/`Unset` restore VPP defaults: LISP off, GPE off, PITR off,
    encap source `::`, hop limit 64.
  - `lisp.enable` and `lisp-gpe.enable` have a real `Get`, so `Retrieve` reports `lisp.enable/global` whenever *anyone*
    enabled LISP.
- **Failure scenario:** agent A (slot 3, no LISP in its config) resyncs while slot 7 runs LISP. A's Retrieve returns
  `lisp.enable/global`. It is not in A's desired state and deletes on absence by default, so A plans a Delete and sends
  `lisp_enable_disable(false)`. That wipes every owner's LISP and GPE state. It is the same class as DF-3 H1/M4.
  - `sr.encap-source` Delete resets the source for every SRv6 policy on the box.
  - `lisp.pitr` Delete disables PITR globally.
- **Fix (per D-071):**
  - Register the setters only when `globalsOwner` is true.
  - Non-owners get a "require" variant: Create checks the switch and fails clearly when it is off, Delete is a no-op,
    and `DeleteOnAbsence() == false` (P05's `AbsenceDeleter`).
  - Even the owner's `lisp.enable` should not be deleted on absence. Disable only after an emptiness check across owners.
  - Tests keep restoring the previous values (the LISP test already does).

### H2 — Untagged SR, SR-MPLS and LISP objects are claimed by address/ID ranges; production passes a nil scope and claims everything on the VPP (D-071 claim rule)
- **Where:**
  - `df6/scope.go:9-17` (the "In production every field is nil" comment)
  - `df6/keyed.go:163-179`
  - `sr/localsid.go:245-276`, `sr/policy.go:218-243`, `sr/steering.go:226-258`
  - `lisp/descriptors.go` (every `Owns:`)
  - Q10 ("Production passes a nil scope")
- **What is wrong:** D-071 says: own tag → ours; foreign tag → never touched; untagged → *only via a ClaimStore
  record* (DF-4 pattern). These objects have no tag. With a nil scope, `Retrieve` returns every localsid, policy,
  steering entry, locator set, EID, map resolver and eid-table map on the VPP.
- **Failure scenario:** P08 wires these descriptors as documented. Anything the agent did not create is retrieved as
  undesired and deleted on the first resync. That covers objects an operator made with vppctl, another controller's
  objects, and, on the shared host, another slot's objects whenever a slot runs with the documented nil scope.
  - `KeyedDescriptor.Create` and `Delete` never consult `Owns`.
  - The range-based scope is also only an approximation even in tests. For example, SR steering is attributed by BSID
    alone and ignores prefix and table.
- **Fix:**
  - Record a claim (descriptor, id) in the owner's ClaimStore on Create and release it on Delete.
  - `Retrieve` reports only claimed ids. A claimed id that is gone from VPP is reported absent, so it gets recreated.
  - Keep `Scope` as an extra test-only filter if useful.

### H3 — Write-only Creates are not idempotent, but P05 re-applies them on every resync (D-063)
- **Background:** P05 (`task/P05` `reconciler.go` `ErrRetrieveUnsupported` + `ApplyOptions.Resync`) re-applies every
  write-only desired object on agent start and on VPP reconnect, and states that "Create must be idempotent".
  The DF-6 fakes do not model duplicate adds, so the unit tests pass.
- **`ipip.sixrd`** (`ipip/sixrd.go:49-85`, `df6/ifdesc.go:94-121`):
  - A second `ipip_6rd_add_tunnel` fails with IF_ALREADY_EXISTS.
  - After an agent restart the scheduler has no Meta for it, so a later `Delete` fails with `ErrBadMeta`
    (`ifdesc.go:156`). The tunnel can never be removed.
- **`gtpu.bypass`, `vxlan-gpe.bypass`, `l2tp.interface-enable`, `pppoe.cp`** (`df6/bypass.go:91-114`: Create always
  applies from an assumed off/off state):
  - VPP has no "already enabled" guard for these. `gtpu.c:1137`, `vxlan_gpe.c:1017`, `l2tp.c:581` and `pppoe_cp.c:8`
    call `vnet_feature_enable_disable` directly.
  - `vnet_config_add_feature` does not deduplicate. Each resync therefore inserts the feature node once more into the
    arc and raises the feature count.
  - The node then runs twice per packet, and a single Delete no longer removes the feature.
  - Only `vxlan.bypass` is safe: `vnet_int_vxlan_bypass_mode` checks a bitmap.
- **`sr-mpls.policy`** (`sr_mpls/policy.go:120-141`): a second add returns -12 (`sr_mpls_policy.c:159`).
- **`sr-mpls.steering`**: same pattern.
- **`lisp-gpe.fwd-entry`** (`df6/keyed.go:86-95`): the second add is rejected with INVALID_VALUE ("don't support
  updates", `lisp_gpe_fwd_entry.c:517`).
- **Fix:** probe before adding. The probes already exist: `Present`, `BSIDPresent`, `KeyedSpec.List`.
  - 6rd: adopt the interface tagged `<owner>:<id>` as Meta. The tag is readable even though the 6RD prefixes are not.
  - Feature toggles: make Create send disable then enable. Disable at count 0 is a no-op (`feature.c:265`), so the
    pair is idempotent.
  - Add duplicate-add errors to the fakes and a "Create twice" case to every write-only unit test.

### H4 — Interface references are resolved and reported by VPP name, not by logical name, and other owners' interfaces are accepted (D-065/D-069)
- **Where:**
  - `df6/ifaces.go:25-58`: `byName` is keyed by `InterfaceName`, and `Index` ignores tags.
  - Callers: `df6/bypass.go:107` (the 5 toggles), `sr/localsid.go:143-147`, `sr/steering.go:126-130`,
    `lisp/descriptors.go:137`, and the mcast resolution in vxlan, vxlan_gpe and gtpu.
  - Retrieve side: `NameOrEmpty` in `sr/localsid.go:267`, `sr/steering.go:248`, `lisp/descriptors.go:190`,
    `vxlan/tunnel.go:133`, `vxlan_gpe/tunnel.go:149`, `gtpu/tunnel.go:151`.
- **Failure scenarios:**
  - P08 emits `interface/w2-tap40` (a logical name, D-069). DF-6 looks up VPP name `w2-tap40`, finds nothing, and
    Create fails. If the config uses `tap40` instead, the dependency key `interface/tap40` never matches DF-1's alias key.
  - Retrieve reports `tap40` while desired says `w2-tap40`, so every resync plans an update or recreate.
  - VPP names of created interfaces are index-based and change across VPP restarts. After a restart, `tap3` can be
    another slot's tap. Bypass, l2tp decap, PPPoE CP, SRv6 End.X/DX, L2 steering and LISP locators then land on a
    foreign interface. The resolver never refuses a foreign tag (DF-1 returns `ErrForeignInterface`).
  - This is the same class as the DF-2 H3 and DF-4 M1 fixes.
- **Fix:**
  - Use DF-1's `iface.ResolveName` / `Table.Logical` (D-069 "one resolver"), or a local copy until DF-1 merges.
  - Refuse interfaces tagged by other owners.
  - Report logical names in Retrieve.
- **How the tunnels themselves are named (for P08):**
  - gre, ipip and vxlan use id = VPP name (`gre<inst>`, `ipip<inst>`, `vxlan_tunnel<inst>`).
  - vxlan-gpe, gtpu, 6rd and gtpu-forward use id = config name.
  - The tag is `<owner>:<id>` in both cases, so DF-1's resolver can reach them as `interface/<id>`. See L2 for collisions.

## MEDIUM

### M1 — Deletes by index do not re-verify identity; the per-interface toggles do not check at all (D-071, DF-2 H1 pattern)
- **`IfDescriptor.present`** (`df6/ifdesc.go:180-186`): it checks only that the Meta sw_if_index is tagged by *this
  owner*. It does not check that the tag id equals the object's id, and it does not check the tuple.
- **Index reuse after a VPP restart:**
  - `ipip_del_tunnel` / `ipip_6rd_del_tunnel` delete by sw_if_index (`ipip/tunnel.go:83-84`, `ipip/sixrd.go:76-77`),
    so a stale index that now belongs to another of our tunnels deletes the wrong one.
  - The tuple-based deletes (gre, vxlan, vxlan-gpe, gtpu, pppoe) remove whatever tunnel holds the tuple. The gtpu
    guard checks only that (dst, teid) exists (`gtpu/tunnel.go:129-135`), not that it is ours or at the Meta index.
- **`BypassDescriptor.Delete`** (`df6/bypass.go:146-157`): it sends disable to the Meta index with no existence or
  ownership check. After an index reuse it disables vxlan/gpe/gtpu bypass, l2tp decap or PPPoE CP on another owner's
  interface. `pppoe_add_del_cp` does not even validate sw_if_index (`pppoe_api.c:134`).
- **`l2tp.tunnel` Update** (`l2tp/tunnel.go:100-109`): `l2tpv3_set_tunnel_cookies` is sent to the unverified Meta index.
- **Fix:**
  - `present()` should compare `OwnedID(idx) == id`.
  - Tuple deletes should match the dump record's sw_if_index against the Meta index.
  - Toggles should re-resolve the interface by (logical) name and compare it with the Meta index before sending.

### M2 — `pppoe.cp` is really a VPP-global singleton, and its host test runs by default
- **What is wrong:** `pppoe_add_del_cp` sets or clears the single `pem->cp_if_index` (`pppoe_cp.c:20-27`).
  - Create overwrites another owner's CP interface, and Delete sets it to `~0` for everyone.
  - The descriptor is keyed per interface (`pppoe.cp/<iface>`), so two desired keys can exist while VPP holds one value.
- **Test:** `TestCpOnHost` (`pppoe/pppoe_integration_test.go:15-27`) is not opt-in. It changed and then reset this
  global on the shared VPP.
- **Fix:** make it a singleton `pppoe.cp/global` under the globals-owner rule (H1), and make the host test opt-in or
  read-only.
- **Similar:** `gtpu.forward` is one global per (forwarding type, family) (`gtpu.c:452-455`). Document it under the
  same rule; its test is already opt-in.

### M3 — Presence probes are not identity checks, and SR steering silently takes over existing entries
- **`sr.steering` Delete** (`sr/steering.go:184-210`) looks for (family, table, prefix) with a nil scope and ignores
  the BSID. It deletes an entry that now points at another owner's policy.
- **`sr.steering` Create** re-points an existing (possibly foreign) steering key without error, because VPP's add on
  an existing key re-points it. `Update` depends on that (`:161-181`).
- **`sr-mpls` probes:**
  - `BSIDPresent` (`sr_mpls/policy.go:171-186`) matches *any* end-of-stack local-label entry in MPLS table 0.
  - `routePresent` (`sr_mpls/steering.go:94-115`) matches *any* route for the prefix, from any source.
  - A DF-7 MPLS route or a static route therefore reads as "present".
- **Fix:**
  - Compare the BSID in steering Delete.
  - Refuse Create when the key exists with a BSID that is not ours.
  - For SR-MPLS, filter by path type or FIB source.

### M4 — `sr-mpls.endpoint-color` Delete fails forever while the policy exists
`sr_mpls/steering.go:256-266` returns `ErrNoDelete` whenever the BSID is present. The scheduler deletes dependents
before their parent, so removing a policy that has an endpoint-color from desired state always fails, and the policy is
never deleted. **Fix:** make Delete a documented no-op, because the assignment is cleared together with the policy.

### M5 — No agent-restart simulation evidence (checklist item 3)
The pasted evidence is "re-apply plan empty" from the same process. It does not show a fresh descriptor set seeing
missing prefixed objects and recreating them. This matters most for the write-only types, which fail across a restart
(H3: 6rd Meta, stacked features). **Fix:** add one host test for the stateless types — create, delete in VPP, then a
new descriptor instance plus `PlanFor` shows a create. Also add one fake test per write-only type for the
restart → re-apply → delete sequence.

## LOW

- **L1 — go.mod conflict with main.** `apps/agent/go.mod` and `go.sum` conflict (`git merge-tree`: CONFLICT). DF-6
  added `fsnotify` and `logrus` as indirect; main additionally has `ftrvxmtrx/fd`. Merge main, take main's files, run
  `go mod tidy`, and re-run CI against the current main (0cdb35f).
- **L2 — Logical-name collisions and unusable names.**
  - Tag ids are not namespaced per descriptor. A gtpu tunnel named `gre5`, a vxlan-gpe and a gtpu tunnel with the same
    name, or a DF-1 loopback id can all collide. DF-1's `IndexByName` then picks the first match.
  - The `pppoe.session` id `aa:bb:…/11500` (`pppoe/session.go:32-38`) is not usable as `interface/<name>`.
  - Fix: validate names against the VPP-style patterns and document the rule.
- **L3 — Duplicate Retrieve keys (DF-3 H2 pattern).**
  - `sr.localsid` is keyed by SID only, so SIDs that differ only in prefix length (`sr_localsid_add_del_v2`) produce
    two KVs with one key.
  - L2 steering on an unresolved interface gives `l2/` for every entry.
  - Fix: dedupe or skip those entries, and log them.
- **L4 — Crash window between add and tag.** An agent crash between add and tag (`ifdesc.go:107-119`) leaves an
  untagged tunnel. It blocks re-creation (instance or tuple in use) and is never claimed. Document it, or record the
  claim before the add (H2).
- **L5 — Duplicated helpers and sentinel.**
  - `df6` duplicates DF-2's helper package.
  - `df6.ErrRetrieveUnsupported` is recognised by P05 only through string matching. Alias
    `scheduler.ErrRetrieveUnsupported` once P05 merges (D-073c).
- **L6 — Documentation mismatch.** `docs/agent/descriptors/df6.md` says "no descriptor ever sends a delete for an
  object VPP no longer has". That does not hold for the toggles (M1) or for the singleton Unset. Correct the wording.

## Summary

- **Solid:** Retrieve for tagged tunnels, the V8 guard, the table pre-checks, test hygiene and CI.
- **Blocking:**
  - H1: globals are not gated, and one agent's resync can switch LISP off VPP-wide.
  - H2: range claims instead of ClaimStore; nil scope claims the whole VPP in production.
  - H3: write-only types are not idempotent under P05's resync re-apply; four toggles stack features in the VPP graph.
  - H4: interface references use VPP names and accept foreign interfaces, against D-069.
- **Fix round:** H1–H4, M1–M4, and L1 before merge.

**BLOCK**
