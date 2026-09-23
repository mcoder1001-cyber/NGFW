# DF-3 — independent review

Reviewer: review agent (did not write this code). Branch `task/DF-3` @ 6417446, base `main` (now at 7d9be25).
Ran directly on the host with the slot-9 env (`eval "$(tools/lab env 9)"`, `VRX_INTEGRATION=1`), one package at a time.
`systemctl show vpp -p NRestarts` was `2` before and after every run (ActiveEnterTimestamp 00:26:04, unchanged), so VPP did not restart.
`VRX_DF3_DET44` was **not** set, so `TestDet44OnHost` was SKIP in my runs, as intended by D-064.

## Checklist

| # | Check | Result |
|---|---|---|
| 1 | Contract | `git diff --name-only main...task/DF-3 -- packages/schema packages/proto apps/agent/gen packages/proto/gen packages/api-client/src/generated` is empty. The specs are Go structs carried as `structpb` (the D-055 stand-in). OK. |
| 2 | Real verification | I reran all 8 host packages: nat44ed, nat44ei, nat64, nat66, mapnat, cnat and pnat PASS, det44 SKIP (opt-in). They assert on VPP state through `Retrieve`. `AssertPlan` hides duplicate keys, though (finding 2). Write-only types are checked only for "Create twice without error". That is the most D-063 allows. |
| 3 | Restart safety | No agent-restart simulation is pasted, because P05 has no agent yet. The descriptors are stateless: no caches, no stores, and Meta is rebuilt from dumps. So a fresh instance behaves like the tested one. The 7 write-only types have no Retrieve, which D-063 allows. The remaining gaps are index-based deletes (finding 3). |
| 4 | binapi provenance | Every message comes from `apps/agent/binapi/{nat44_ed,nat44_ei,nat64,nat66,det44,map,cnat,pnat,feature,interface,ip,memclnt}`. `binapi/` and `tools/binapi-gen.sh` are untouched. The raw-stream `nat44_ed_vrf_tables_v2_dump` workaround (Q4) uses the generated types only. OK. |
| 5 | Shared-host rules | Loopbacks `loop9xx` are tagged `w9:`. Addresses are in 10.9/16 and fd00:9::/32, tables are 9000–9999, and mapping/domain tags are `w9:`. Cleanup is in `t.Cleanup`. No pkill and no daemons. After my runs, `show nat44 addresses/interfaces` is empty and no `loop9xx` is left. **However**, the global singletons are neither owned nor restored correctly (findings 1 and 4). |
| 6 | Security | `grep -rn "vppctl\|exec.Command"` over the 9 packages is empty. The evidence hook only writes and stats files. No secrets. OK. |
| 7 | Transaction semantics | LB-mapping Update adds new locals before it removes old ones. Good. `pnat.attachment` Delete fails hard when no pnat interface exists (finding 7). |
| 8/10 | UI / i18n | n/a. |
| 9 | Scope | `natcommon` + `nattest` are shared helpers inside the owned tree. npt66 and dslite were not built. The factory prompt lists them but the envelope does not (Q5). That was accepted as a follow-up, but the prompt's acceptance item "npt66 skip-unless-plugin-loaded" is therefore open. `go.mod`/`go.sum` are outside the owned files and **conflict with main** (finding 9). |
| 11 | CI | I ran `tools/ci.sh --base main` in the worktree: `CI GATE PASSED`, EXIT 0. This matches the pasted output. My per-package host results match the pasted log too: same plans, same "empty after re-apply" lines. |

## Findings, ranked by severity

### 1. HIGH — disabling a NAT plugin wipes other owners' objects: `hasForeignObjects` misses 5 object kinds, and "enabled but still empty" is invisible
`nat44ed/nat44ed.go:122-180` (`hasForeignObjects`), `nat44ed/enable.go:96-108` (Delete), `nat44ed/enable.go:86-95` (Update → ErrRecreate). The same pattern is in `nat44ei/nat44ei.go:122-178` and `nat44ei/objects.go:214-237`.
- **What the check covers:** only `nat44_interface_dump`, `nat44_address_dump` and `nat44_static_mapping_dump`.
- **What `nat44_ed_plugin_disable` also destroys** (`/root/vpp/src/plugins/nat/nat44-ed/nat44_ed.c` ~2667-2694): output-feature interfaces (a separate dump, not in `nat44_interface_dump`), identity mappings, LB mappings, interface-address pools, VRF tables and forwarding.
- **Host probe (slot 9, temporary test, not committed):**
  - I enabled nat44-ed as `w9` and put an output feature on a loopback tagged `w9probe:foreign` (another owner).
  - I then called `p.Enable.Delete` → `err=<nil>; plugin enabled after=false`. The foreign output interface was wiped with the plugin.
- **Second hole, no check can catch it:** slot A enables nat44-ed and has not created its pools yet. Slot B's Create then finds the plugin "already enabled with a compatible mode" and treats it as converged. B's test cleanup or reconciler Delete sees no foreign objects and **disables NAT for A**. Because Retrieve claims `nat44-ed.enable/global` for every owner, *any* slot agent whose desired state lacks NAT plans that Delete.
- **Same class:** `nat64.enable` / `nat66.enable` Delete use `inventory()`, which has the same "enabled but empty" hole.
- **Fix:**
  - Extend `hasForeignObjects` (ED and EI) to the output-feature interfaces, identity/LB mappings, interface-address, VRF tables and the ED↔EI state.
  - For slot owners (`!scope.All`), never Retrieve or Delete the enable singleton unless this owner provably enabled it. A host-wide lease is the simplest way: a `flock` file per plugin under `/run/lock/vrx-nat-<plugin>.lock` held shared by every user and taken exclusive to disable. The per-slot `SlotLock` does not protect other slots.
  - Tests should disable only under that exclusive lock.
  - Add the probe above as a unit test with the fake (foreign output interface → `ErrForeignObjects`).

### 2. HIGH — interface-bound static and identity mappings are retrieved twice under one key, with different values
`nat44ed/mapping.go:237-273` (static) and `:344-373` (identity). The same applies to `nat44ei/objects.go:720-…` and `:821-…`.
- **Cause:** VPP's `nat44_static_mapping_dump` sends each resolved `sm->static_mappings` entry (external = the resolved address, `external_sw_if_index = ~0`) **and** the `sm_to_resolve` record (external = the interface) (`nat44_ed_api.c:719-730`). The identity dump behaves the same way (`:859-875`, one detail per local/VRF as well), and so does EI (`nat44_ei_api.c:910-920, 1051-1066`). Both carry the same tag.
- **Host probe:** a mapping with external `loop902` whose interface has 10.9.20.1 gives:
  - `nat44-ed.static-mapping/ifmap external.ip=10.9.20.1 meta={ExternalSwIfIndex:4294967295}`
  - **and** `nat44-ed.static-mapping/ifmap external.interface=loop902 meta={ExternalSwIfIndex:5}`
  - The pasted evidence shows the same twin lines (`external 10.9.20.1:2222` + `external loop902:2222`; EI: `external 10.9.40.1` + `external loop904`).
- **Why the tests pass:** `nattest.AssertPlan` and `Apply` put the KVs into a map, so the last value wins and `len(have)` counts keys. The duplicate is masked (`natcommon/nattest/plan.go:26-28, 90-95`).
- **Failure scenario:** the scheduler contract says keys are unique. A reconciler that keeps the first KV, or verifies by list, sees `external.ip≠""`, which differs from desired, on every pass. Update is nil → ErrRecreate → Delete + Create on every resync, a NAT outage window each time. Alternatively, verify fails and the transaction rolls back.
- **Fix:**
  - In Retrieve, drop the resolved `static_mappings` twin when a `to_resolve` record with the same tag exists. Keep the interface form and its sw_if_index Meta.
  - Merge multi-local identity details per tag.
  - Make `AssertPlan`/`Apply` fail on a duplicate key (`len(kvs) != len(have)`).
  - Add a unit test with the fake that returns both details.

### 3. HIGH — pnat binding indices are recovered non-atomically and used for deletes without verification, so the wrong binding can be deleted
`pnat/pnat.go:379-437` (`bindings`), `:474-486` (Delete by `meta.Index`), `:545-566` (attach by recovered index).
- **How recovery works:** it sends N·log(k) separate `pnat_bindings_get` calls. Any add/delete between them shifts the answer. The contract allows Retrieve concurrently with a Create of the same agent, and the VPP is shared.
- **Concrete case:** the fast path `count(n)==0 ⇒ indices 0..n-1` is wrong if a binding below n is deleted after the first get. Every later binding is then labelled one index too high, or too low after a concurrent add into a hole.
- **Consequence:** `pnat_binding_del(index)` / `pnat_binding_attach(index)` act on **another binding**, possibly another owner's. VPP allows identical match tuples (`pnat.c pnat_binding_add` has no duplicate check), so a retried Create also yields two bindings with one key. Retrieve then reports a duplicate key and one binding is never cleaned up.
- **Related:** a binding deleted while attached leaves a flow entry that points at a freed pool index. A later binding that reuses the index inherits that interface's traffic. The test comment acknowledges this, but the descriptor does not guard it.
- **Same class, smaller window:** `cnat.translation` Delete (`cnat/cnat.go:357-366`) and `map.domain` Delete (`mapnat/mapnat.go:239-247`) delete by a bare id from Meta with no identity check. After a VPP restart between plan and apply, or with index reuse, they hit another owner's object.
- **Fix:**
  - Take a before/after `getFrom(0)` snapshot and retry the recovery until the two are equal.
  - Before `pnat_binding_del`/`attach`, verify that `getFrom(idx)[0]` equals the spec and `count(idx)-count(idx+1)==1`.
  - Refuse Delete of a binding that `pnat_flow_lookup` still finds attached.
  - Report duplicate match tuples as `<id>#<index>` extras for deletion (the DF-4 D-066 pattern).
  - For cnat and map, re-dump by id and compare VIP/tag before deleting.

### 4. MEDIUM — global timeouts, forwarding and parameters are claimed by every slot owner, and the tests do not restore the prior values
- **Retrieve is not scoped by owner** for `nat44-ed.timeouts`/`.forwarding` (`nat44ed/enable.go`), `nat44-ei.timeouts`/`.forwarding` (`nat44ei/objects.go:264-298`), `nat64.timeouts` (`nat64/nat64.go` newTimeouts), `det44.timeouts` (`det44/det44.go:171-192`) and `map.params` (`mapnat/mapnat.go:387-…`).
- **Failure scenario:** a `w3` agent whose desired state lacks them plans Delete → resets `w9`'s (or production's) timeouts to the defaults and turns forwarding off.
- **The tests break the prompt rule "restore every global you changed":**
  - `nat44ei_integration_test.go` (Timeouts/Forwarding via `CreateAll`, ~l.61-65) resets to the **defaults** and turns forwarding **off**, even when EI was already enabled by another owner with other values.
  - `nat64_integration_test.go:46-48` overwrites nat64 timeouts unconditionally and "restores" them to the defaults.
  - `nat44ed_integration_test.go:63-73` restores timeouts only in the foreign-enabled case. Otherwise it relies on the disable, which finding 1 may refuse, leaving 299/7439/239/59 behind.
- **Fix:**
  - Scope Retrieve of these singletons to the owner that set them (same lease or claim as finding 1), or never report them for `!scope.All`.
  - In the tests, save the prior values and restore them exactly (the det44 and map tests already skip when the value is non-default; do the same).

### 5. MEDIUM — production scope (`All`) claims everything, including interfaces tagged by other owners; Create accepts foreign interfaces
- `natcommon/scope.go:98-107`: `OwnsInterface` returns true for **any** interface when `All`, including `w<N>:`-tagged loopbacks. `hasForeignObjects`/`inventory` short-circuit to "no foreign" for `All`.
- **Failure scenario:** on this shared host, any run of the real agent with the default owner `vrx` (manager smoke test, E2E, a P05 restart-safety check) Retrieves every slot's NAT features, pools, translations and pnat bindings as "owned, not desired" and deletes them. It disables nat44/nat64/nat66 for all 12 slots.
- **Also:** `ResolveInterface` (`natcommon/ifaces.go:89-116`) is used by every Create without an ownership check. A slot's desired object on another slot's loopback is created there, is invisible to its own Retrieve (leak), and is re-created on every pass.
- **Fix:**
  - For `All`, still reject interfaces whose tag parses as another owner's (`<x>:…` with `x ≠ owner`).
  - In Create, refuse foreign-tagged interfaces (DF-4 `ErrForeignInterface` precedent).
  - For untagged objects in production, use the owner table the README requires ("an owner table P05 keeps in the state dir"), or record a decision in the LOG that `All` is only allowed on a dedicated VPP.

### 6. MEDIUM — write-only enables silently ignore a VRF change (det44, nat66)
`det44/det44.go:140-148`, `nat66/nat66.go:160-166`.
- **Cause:** Create treats "already enabled" as success whatever the VRFs. The type is write-only, so the reconciler never has an actual value and never calls Update. `ErrVRFChangeUnsafe` (det44) and the nat66 foreign guard are therefore unreachable.
- **Failure scenario:** a changed `inside_vrf`/`outside_vrf` is reported as applied but never takes effect.
- **Fix:** where VPP exposes the VRF, compare it before claiming success (nat66 static-mapping/interface dumps do not expose it). Otherwise have Create return an explicit "already enabled, VRFs not verifiable" error when the agent's own last-applied value differs. At minimum, document the limitation in det44.md and nat66.md and raise it with P05 as a D-063 caveat.

### 7. LOW — `pnat.attachment` Delete errors when nothing can be attached
`pnat/pnat.go:578-582`. When `pnat_interfaces_get` is empty, nothing is attached, so the detach is already done.
- **Problem:** returning `ErrFlowHashUninitialised` makes a rollback or a stale-Meta delete fail, and the agent goes DEGRADED (AD-4).
- **Fix:** return nil there. Keep the guard, which is correct, so the crashing message is still never sent. Also, a second reply batch from `pnat_interfaces_get` is a hard error (`:528-529`); follow the cursor as `getFrom` does.

### 8. LOW — cnat crash guards are check-then-act across processes
`cnat/cnat.go:522-535, 628-650`.
- **Race:** the guard reads `cnat_get_snat_addresses`, then sends `cnat_set_snat_policy` / `cnat_snat_policy_add_del_exclude_pfx`. Another owner deleting the default entry in between (its test cleanup, `cnat/cnat_integration_test.go:85`) crashes the shared VPP (V10).
- **Probability:** low today, because only slot 9 runs cnat.
- **Fix:** serialise every default-entry mutation under a host-wide cnat lock (the same mechanism as finding 1), and say so in cnat.md.
- **Also:** `cnat.snat-policy` Delete resets the policy of a default entry that may belong to another owner (`:545-550`). Check `ownedSnat` first, as `snat-addresses` Delete does.

### 9. LOW — the branch does not merge cleanly
`git merge-tree main task/DF-3` → CONFLICT in `apps/agent/go.mod` and `go.sum`. Main already has `fsnotify`/`logrus`/`ftrvxmtrx/fd` (DF-4). Rebase and keep main's lines. The go.mod change is not mentioned in DF-3.md.

### 10. INFO
- **det44 guard is complete:** no code path sends `det44_plugin_enable_disable(enable=0)`. The only calls are the Create enable and the unit-test fake. The integration test is opt-in (`VRX_DF3_DET44`), and V9 is recorded. Because det44 interfaces are removed by the test and the plugin stays enabled, any *manual* det44 disable on this VPP will now crash it. Keep the V9 warning prominent for the manager's sweep.
- **pnat lazy-init guard is sound:** `pnat_disable()` frees the flow hash only when the translation pool is empty, and `pnat_binding_detach` needs a live binding, so "≥1 pnat interface ⇒ hash initialised" holds. The cnat `n_paths=0` guard is correct.
- **D-063 compliance:** no write-only type echoes cached desired state. The cnat exclude-prefix refcount growth on re-apply is documented.
- **Ownership by address range after a VPP restart:** pools, prefixes and translations have no index-reuse exposure. Meta always comes from the live dump, and there is no persisted store (unlike DF-2 finding 1).

## What unblocks

Findings 1, 2 and 3 must be fixed, with unit tests: probe 1 as a fake test, the duplicate twin in the fake, and a pnat recovery test with a concurrent delete. The branch must also be rebased (finding 9). Finding 4 (scoping plus exact restore in the tests) and finding 5 (reject foreign-tagged interfaces) should land in the same round, or be settled by a LOG decision. Findings 6–8 can follow.

**BLOCK**
