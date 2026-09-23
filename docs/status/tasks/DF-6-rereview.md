# DF-6 re-review — tunnel / SR / LISP descriptors (after the fix round)

Reviewer: independent re-review agent (did not write the code) · branch `task/DF-6` @ a9b6005 · slot 11 · 2026-09-24
Scope: `git diff fb83447..HEAD`. The DF-6 part is `407e074..HEAD`, 64 files. The rest is the main merge.

## What I ran

- **Unit tests:** `go test -count=1 ./internal/descriptors/{df6,gre,ipip,vxlan,vxlan_gpe,gtpu,l2tp,pppoe,sr,sr_mpls,lisp}/...`
  All 11 packages `ok`. `go vet ./internal/descriptors/...` is clean.
- **Host tests** (slot 11: `eval "$(tools/lab env 11)"`, `VRX_INTEGRATION=1`):
  - One package at a time, `-run OnHost`.
  - The gtpu, LISP and pppoe-cp opt-ins were **not** set.
  - `NRestarts=2` before every package and after the last one. VPP MainPID stayed 668679.
  - `TestAgentRestartOnHost`:
    - Agent 1 applies 4 objects; its re-apply plan is empty.
    - Agent 2 (new API connection, claim file reopened) plans nothing.
    - Losing the gre tunnel and the local SID makes agent 2 plan `create=1` for each.
    - Agent 2 reconciles; its plan is then empty again.
    - Identical to the pasted output.
  - gre, ipip, vxlan, vxlan_gpe and sr (localsid, policy, steering) all printed an empty re-apply plan.
  - sr_mpls: policy/steering PASS; endpoint-color skipped.
  - l2tp: interface-enable PASS; tunnel skipped.
  - gtpu, lisp and pppoe skipped (opt-ins).
  - Afterwards `show sr policies` and `show gre tunnel` were empty.
- **CI:** `tools/ci.sh --base main` → **CI GATE PASSED**.
  - Mode quick, 1m02s, logs `/root/ngfw-wt/logs/ci/DF-6-20260924-014019-1483055`.
  - No contract files changed.
  - Warnings only: two commit subjects that are not in Conventional Commits form (the merge commit and `review(DF-6)`).
  - `git merge-tree HEAD main` (main @ eedde04): no conflict.
- **Adversarial probes:** I wrote throw-away `zz_probe_test.go` files against the packages' own fakes. I ran them, kept
  copies outside the repo and deleted them; they are not committed. Results are cited under each finding below.

## Original findings

| # | Verdict | Evidence |
|---|---|---|
| H1 globals not gated | **FIXED** | See H1 below. |
| H2 range claims / nil scope | **FIXED** (see N3) | See H2 below. |
| H3 write-only not idempotent | **FIXED for repeated resyncs** (see N1, N2, N4) | See H3 below. |
| H4 VPP names / foreign interfaces | **FIXED** | See H4 below. |
| M1 delete by index w/o identity | **FIXED** | See M1 below. |
| M2 pppoe.cp global, host test | **FIXED** (see N5) | See M2 below. |
| M3 presence ≠ identity | **FIXED** | See M3 below. |
| M4 endpoint-color blocks policy delete | **FIXED** | `sr_mpls/steering.go` `Delete` only drops the per-boot claim. Create requires our claimed policy. |
| M5 restart evidence | **FIXED** | `df6/restart_integration_test.go`, which I reproduced on the host (above). Fake resync/restart tests exist per write-only type. Gap: the host test never loses the *interface* under a toggle (N1). |
| L1 go.mod conflict | **FIXED** | Merge `407e074`. No conflict with current main. CI passes. |
| L2 name collisions | **PARTIAL** (documented) | See L2 below. |
| L3 duplicate Retrieve keys | **FIXED** | `keyed.go:229` dedupes ids. L2 steering on an unresolved or foreign interface is skipped (`sr/steering.go` List). |
| L4 add→tag crash window | **PARTIAL** (documented) | Documented for tunnels. The same window exists in `KeyedDescriptor.Create` (Add, then Claim at `keyed.go:135-138`); see N7. |
| L5 helpers / sentinel | **PARTIAL** (documented) | Deferred to P05 (Q10). Acceptable. |
| L6 doc wording | **FIXED** | `df6.md` now describes the toggle/verify semantics. |

### H1 — globals not gated: FIXED
- `df6.Global` (`singleton.go`) registers the setter only with `WithGlobalsOwner(true)`. Everyone else gets
  `RequireDescriptor`: Create checks, Delete is a no-op, Retrieve returns `ErrRetrieveUnsupported`, and
  `DeleteOnAbsence()==false`.
- It is used in every Register that has globals:
  - `lisp.enable`, `lisp-gpe.enable`, `lisp.pitr`
  - `sr.encap-source`, `sr.encap-hop-limit`
  - `l2tp.lookup-key`
  - `pppoe.cp`
- On the owner, `lisp.enable` and `lisp-gpe.enable` have `KeepOnAbsence`, and before disabling they run `SafeToDisable`.
  That check covers local locator sets, EID-table entries, non-zero VNI maps and GPE VNIs of any owner.
- `TestLISP` asserts all of this:
  - a non-owner never sends `lisp_enable_disable`;
  - the owner leaves LISP on while another owner's set exists;
  - `DeleteOnAbsence()` is false.

### H2 — range claims / nil scope: FIXED (see N3)
- `df6.Scope` is gone. `KeyedDescriptor` enforces ownership through claims:
  - Create refuses a present, unclaimed id (`keyed.go:129-133`, `ErrNotOurs`).
  - Delete never touches an unclaimed id (`:177`).
  - Retrieve reports claimed ids only (`:229`).
- Probes: `sr TestClaims` and `sr_mpls TestResync` show that another owner can neither take over nor delete an
  object. I also confirmed that an unclaimed foreign SR policy is neither retrieved nor deleted.
- Weakness: a *stale* claim does not expire when VPP restarts (N3).

### H3 — write-only types not idempotent: FIXED for repeated resyncs (see N1, N2, N4)
- **6RD:** Create adopts the interface tagged `<owner>:<id>` (`ifdesc.go:111`). `TestSixrdResyncAndRestart` shows
  1 add after 2 creates, and a delete by tag after an agent restart.
- **Toggles:** a claim per VPP boot. `TestBypassIdempotentAcrossResyncs` counts: 3 applies → 1 instance; agent restart →
  0 re-adds; boot change → exactly 1 re-add.
- **SR-MPLS and GPE:** re-apply is a no-op when the object is present and claimed.
- **My probe (sr_mpls):** I sent 3 applies, wiped the fake's state and changed the boot id, then sent 3 more applies.
  VPP received `sr_mpls_policy_add` 2 times and `sr_mpls_steering_add_del` 2 times, with 0 rejected requests. So each is
  re-added exactly once after the simulated VPP restart.
- **LISP-GPE:** `TestLISP` shows a resync sends no second `gpe_add_del_fwd_entry`.
- The fakes now model duplicate adds (-65 / -7 / -12) and stacked features.

### H4 — VPP names / foreign interfaces: FIXED
- `df6.Interfaces` wraps DF-1's `iface.Table`:
  - `IndexByName` resolves logical names and refuses foreign tags.
  - `NameOrEmpty` goes through `Logical`.
- `TestBypassIdempotentAcrossResyncs` confirms:
  - `w3-tap1` (a foreign interface) is refused with `iface.ErrForeignInterface`;
  - an untagged `ens224` is claimed;
  - Delete on an unclaimed physical interface returns `ErrNotOurs`.
- SRv6 L2 steering and localsid, LISP locators and the mcast interfaces use the same resolver.

### M1 — delete by index without identity: FIXED
- `IfDescriptor.verify` (`ifdesc.go:135-169`) locates the object by tag and requires a dump record at that index
  that decodes to the same id. The delete then uses VPP's key fields.
- Toggles re-resolve the interface, compare it with Meta and check ownership.
- l2tp cookie Update goes through `verify`.

### M2 — pppoe.cp global and its host test: FIXED (see N5)
- `pppoe.cp` is now the singleton `pppoe.cp/global` under `df6.Global`.
- `TestCpOnHost` is opt-in (`VRX_DF6_PPPOE_CP_HOST`). I did not run it.
- `gtpu.forward` is documented as one global per type.

### M3 — presence probes are not identity checks: FIXED
- SR steering has `Identity` = BSID. In `TestClaims`, a foreign re-point is neither deleted nor taken over.
- The SR-MPLS BSID probe now requires every path to be a recursive MPLS path.
- The SR-MPLS steering probe uses the "SR" FIB source plus an MPLS path.

### L2 — name collisions: PARTIAL (documented)
- Documented in `df6.md:62`, and the schema's `tunnels.name-unique` prevents collisions across tunnel kinds.
- A DF-1 interface with the same id is still not caught, and Create now silently adopts it (N6).

## NEW findings

### MEDIUM

**N1 — Per-boot toggle claims are keyed by the logical name, not the sw_if_index: when the interface is recreated on the same VPP boot, the feature is never re-enabled.**
- **Where:**
  - `df6/bypass.go:94-99`: `claimID` = `<iface>/<family>`, and `ensure` at `:103-117` skips when the claim is held.
  - `pppoe/cp.go:38`: the same pattern.
  - `sr_mpls/steering.go:248`: endpoint-color, keyed by bsid/endpoint/color.
- **Failure scenario:**
  1. A bypass, l2tp decap or pppoe CP is enabled on `loop1171`.
  2. The loopback or tap is lost and DF-1 recreates it with a new sw_if_index. VPP did not restart. This is the
     DoD restart-safety step "delete prefixed objects via binapi → agent recreates", or any external loss.
  3. The write-only re-apply calls `Create`. The claim `loop1171/ip4 @ vpp-<same pid>` is still held, so nothing is sent.
  4. The new interface runs without the feature, with no error and no drift signal.
- The same happens to endpoint-color when the SR-MPLS policy is lost and re-added on the same boot: VPP cleared the
  assignment, but the claim says it is done.
- **Probe:** `TestZZProbeBypassIfaceRecreated`: `old idx 1 new idx 2 count(new)=0` → FAIL.
- **Fix:**
  - Include the sw_if_index in the claim id (`<iface>@<idx>/<family>`). A recreated interface is then a new claim.
    In `Delete`, release claims whose idx no longer exists.
  - For endpoint-color, key the claim by the policy's instance, or drop the claim whenever policy Create actually adds.
  - Add a fake test that removes and re-adds the interface on the same boot.

**N2 — The boot identity is the VPP PID alone; it is not robust across host reboots.**
- **Where:**
  - `df6/claims.go:70-75`: `BootID` = `control_ping.vpe_pid`.
  - Consumers: `bypass.go`, `pppoe/cp.go`, `sr_mpls/steering.go` endpoint-color.
- **Failure scenario:**
  - The claim store is persisted in the agent state dir (`OpenFileClaimStore`), so it survives a host reboot.
  - On an appliance with a deterministic boot sequence, VPP can get the same PID after a reboot. PID-namespaced
    deployments make that even more likely. The per-boot claims then look current.
  - Result: the l2tp decap enable, the PPPoE CP and the bypass toggles are never re-enabled after the reboot.
    l2tp decap and PPPoE CP are functionally required.
  - The same applies to PID wrap-around after VPP restarts, but that is unlikely with `pid_max` = 4194304.
- **Fix:**
  - Combine the PID with a per-boot marker: the kernel `boot_id` (`/proc/sys/kernel/random/boot_id`; the agent runs on
    the same host) or VPP's stats scalar `/sys/boottime` (`vlib/stats/stats.h:31`).
  - Keep the same helper for DF-1/DF-2, which use PID too. Log it in D-076.

**N3 — Keyed claims do not expire with the VPP instance, so after a VPP restart an object someone else creates at a formerly-claimed id is treated as ours (D-071 violated in that window).**
- **Where:** `df6/keyed.go:130` (Create treats present+claimed as ours), `:177-195` (Delete), `:229` (Retrieve).
- **Failure scenario:**
  1. The agent owns SR policy `fd11:b::7`.
  2. VPP restarts, and before the agent reconciles, an operator (vppctl) or another controller creates the same BSID.
  3. Retrieve reports it as ours, the diff plans an Update or recreate, and a later removal from config deletes the
     foreign policy.
- This affects every SR, SR-MPLS and LISP keyed type.
- **Probe:** `TestZZProbeStaleClaimTakeover`:
  - The owner's Retrieve returned the other owner's policy (`sid fd11:1::9`).
  - `Delete` removed it → FAIL.
- **Fix:**
  - Scope keyed claims to the boot as well (holder `<name>@<boot>`, the D-076 mechanism).
  - Alternatively, on the first Retrieve after a boot change, release claimed ids that are absent and treat present
    ones as unclaimed.

**N4 — Adopt-on-presence ignores parameter changes made while the agent was down (write-only types never converge).**
- **Where:**
  - `df6/ifdesc.go:111-113`: 6RD adopts by tag, with no parameter check (nothing is readable).
  - `df6/keyed.go:130`: SR-MPLS policy has no `Identity`; SR-MPLS steering's identity is the VPN label only, not the
    BSID; the GPE fwd-entry has no identity.
- **Failure scenario:**
  - Desired changes while no in-memory record exists (agent restart, crash between persist and apply, or rollback on
    start).
  - The re-apply `Create` finds our object and returns success. VPP keeps the old 6RD prefixes, old segment lists or
    old locator pairs forever.
  - D-063/D-076 require the re-apply to converge on desired, not just to be idempotent.
- **Probes:**
  - `TestZZProbeSixrdDrift`: VPP kept `fd11:6d::/32 / 10.11.1.1` after a re-apply with `fd11:77::/32 / 10.11.9.9`.
  - The sr_mpls probe: VPP kept labels `[11700]` after a re-apply with `[11799]`.
- **Fix:**
  - Persist a fingerprint (hash of the canonical desired object) with the claim or tag. On re-apply with a different
    fingerprint, delete and re-add (6RD: by tag; SR-MPLS: del + add; GPE: del + add).
  - Add a fake test per type.

### LOW

**N5 — `pppoe.cp` Update to another interface leaves pppoe-input enabled on the old one.**
- **Where:** `singleton.go:103-105` (Update = Create(new)) together with `pppoe/cp.go` `send`.
- **Failure scenario:** VPP's `pppoe_add_del_cp` enables the feature on the new interface, but nothing disables the
  old one.
- **Probe:** `TestZZProbeCpUpdate` → `a=1 b=1`. After Delete it is `a=1 b=0`, so the stale claim and feature on A leak.
- **Fix:** give the cp spec an Update that first sends `send(old, false)`.

**N6 — `IfDescriptor.Create` adopts any interface tagged `<owner>:<id>`, whatever its type.**
- **Where:** `ifdesc.go:111`.
- **Failure scenario:** a DF-1 loopback or tap with the same logical name as a 6RD, vxlan-gpe or gtpu tunnel. Create
  returns success with the loopback's index as Meta and creates nothing.
- **Probe:** `TestZZProbeCrossTypeAdopt`: `meta={1} adds=0`.
- The rule is documented, but the failure is now silent instead of a VPP error.
- **Fix:** before adopting, require a dump record of this type at that index. 6RD records do appear in
  `ipip_tunnel_dump`.

**N7 — Smaller items.**
- **Bypass Delete with stale Meta after a VPP restart:** `bypass.go:130` returns `ErrNotOurs` before checking whether
  anything was enabled on this boot. The Delete fails until a re-apply refreshes Meta. Check the boot claim first; when
  nothing was enabled, return nil.
- **Claims split across two stores:** `WithClaims(store)` affects keyed and per-boot claims only. Untagged-interface
  claims always go to `iface.Claims(owner)` (`names.go:207,215`). A caller that passes `WithClaims` without
  `iface.SetClaimStore` silently splits claims between a persisted store and a memory store. Drop `WithClaims`, or make
  it call `SetClaimStore`.
- **Claim store never pruned:** `FileClaimStore` never prunes `…@vpp-<old pid>` holders, so the file grows with every
  VPP restart.
- **Stale in-memory claim when the file write fails:** `Claim` mutates memory before `save()`. If the write fails,
  Create returns an error, but the retry sees the claim and reports success. The claim is then lost on restart and the
  object is orphaned as "not ours".
- **Keyed crash window:** the add→claim crash window in `KeyedDescriptor` (L4 class) leaves an object that blocks
  re-creation with `ErrNotOurs`. Claim before Add, and release on add failure.
- **`SafeToDisable` coverage:** `SafeToDisable` ignores other owners' map-resolvers/servers and PITR. Disabling does
  not delete them, so this is harmless but not "complete".

## Summary

- All four HIGH findings and M1–M5 are fixed and backed by tests. I reproduced them on the host with NRestarts stable,
  and CI passes.
- Remaining problems are in the new claim/boot mechanism:
  - N1: the claim is keyed by logical name, so a recreated interface never gets its feature back.
  - N2: the PID-only boot identity can collide across host reboots.
  - N3: stale keyed claims let the agent take over or delete a foreign object after a VPP restart.
  - N4: write-only adopt-on-presence never converges parameter drift.
- **Required before merge:**
  - N1 and N5 (small, local fixes).
  - N2 (add `boot_id` or `/sys/boottime` to `BootID`).
- **Board tech-debt, before P08 wires these descriptors:** N3, N4 and N6.

**APPROVE WITH CHANGES**
