# DF-3 — independent re-review after fix round 1

Reviewer: re-review agent (did not write this code). Branch `task/DF-3` @ 6982b9b. The original review is 8317a71
(`DF-3-review.md`, verdict BLOCK). I read `git diff 8317a71..HEAD` over `apps/agent/internal/descriptors/{natcommon,nat44ed,nat44ei,nat64,nat66,det44,mapnat,cnat,pnat}`
(37 files, +2782/−1149), plus `nat-common.md`, the "Review fixes" section of `DF-3.md`, `DF-3-questions.md`, and LOG D-063/D-069/D-071/D-076.

How I ran everything:
- Directly on the host with slot 9 (`eval "$(tools/lab env 9)"`, `VRX_INTEGRATION=1`), one package or probe at a time.
- `VRX_DF3_DET44` was never set, so det44 stayed SKIP.
- `systemctl show vpp -p NRestarts` was **2 before and after every run** (ActiveEnterTimestamp 00:26:04, unchanged).
- No VPP restart and nothing left behind afterwards: no `loop9xx`, and nat44 static mappings, pnat, cnat translations and map domains are all empty.

## Verification summary

| Check | Result |
|---|---|
| Unit | `go vet` clean. `go test -race -count=1` on all 9 DF-3 packages: ok. |
| Host integration (per package) | nat44ed, nat44ei, nat64, nat66, cnat and pnat PASS; det44 SKIP (opt-in). **mapnat FAIL in 4 of 6 runs** (new finding N1). |
| Adversarial host probes | A temporary test package, deleted and not committed, ran 5 probes. Results are in the table below. |
| `tools/ci.sh --base main` | `CI GATE PASSED`, rc=0, wall time 1m04s (log `/root/ngfw-wt/logs/ci/DF-3-20260924-013615-1451589`). Matches the pasted gate. |
| Merge with main (74f35f9) | `git merge-tree --write-tree main HEAD` is clean. No contract, binapi or `binapi-gen.sh` paths changed. |

### Host probe results (slot 9, w9 = this owner, w9b = a second owner, vrx = production owner)

**P1: non-owner vs globals (H1/M4).** nat44-ed was enabled by the probe, and a **w9b-tagged identity mapping** was the only foreign object. As non-owner w9 I tried:
- `Enable.Delete`, and `Enable.Update` to 2048 sessions, which returned `ErrGlobalMismatch`;
- `Timeouts.Create{1,2,3,4}`, which returned `ErrGlobalMismatch`, and `Timeouts.Delete`;
- `Forwarding.Create/Delete`.

Running config before and after: identical (sessions on, timeouts `{300 7440 240 60}`, forwarding unchanged). The **globals-owner** `Enable.Delete` was skipped (nat44-ed still enabled), and the owner `Enable.Update` → `ErrNotEmpty`. Identity mappings were not in the original check at all.

**P5: production owner / foreign interfaces.**
- `vrx` Create on the w9b-tagged `loop981` → `ErrForeignInterface`, and w9 gets the same result.
- `vrx` Retrieve of interface-feature, static mapping, pool and identity mapping, while w9 objects existed → **0 / 0 / 0 / 0**.

**D-069: logical names.** I re-tagged `loop982` as `w9:uplink`. Create on `"uplink"` gave the key `nat44-ed.interface-feature/uplink/inside`, and the re-apply plan was empty.

**H2: interface-bound mappings.**
- Identity mapping on `uplink`, which has 10.9.82.1: one key `identity-mapping/idif`, empty plan.
- Static mapping with external `uplink:2022`: one key `static-mapping/smif`, empty plan.

**P3: pnat stale-index delete.** b1 and b2 were created and b2's Meta (index 0) retrieved. I deleted b2 raw, then added b3, which **reused index 0**. `Binding.Delete(b2, {Index:0})` returned nil, and b3 remained (`[0] …10.9.52.73`). A duplicate b1 was reported as `…/10.9.52.71/80#2`, and `Apply(b1)` deleted only the extra.

**P4: cnat and MAP stale delete.**
- cnat: I deleted w9's translation (id 1) raw, and w9b's translation 10.9.47.72 got id 1. `Translation.Delete(t1, {ID:1})` left it in place.
- MAP: I deleted w9's domain (index 0) raw, and a `w9b:foreign` domain got index 0. `Domain.Delete` left it in place.

**D-076: cnat excluded prefix.**
- 3× `SnatExcludePfx.Create` on one instance → **1** `cnat_snat_policy_add_del_exclude_pfx` add (the probe counted messages through a wrapped client).
- A fresh instance with the default in-memory store (= agent restart) → a 2nd add.
- After the default SNAT entry was deleted and recreated on the same VPP process, Create sent **no add**, and `show cnat snat-policy` no longer contains 10.9.50.0/24 (new finding N2).

## Original findings

| # | Finding | Status | Evidence |
|---|---|---|---|
| H1 | Disable wipes other owners' objects; "enabled but empty" hole | **FIXED** | `natcommon.Global` (`config.go:161-212`): a non-owner never sets, resets or disables anything, and its Retrieve is `ErrRetrieveUnsupported`, so it never plans a Delete. So the "enabled but empty" hole is closed for slots. The owner's disable runs only after `Plugin.Empty` covers all 8 ED kinds (`nat44ed.go:334-397`), with equivalents for EI, nat64 and nat66. det44 is never disabled. Unit `TestDisableNeedsCompleteEmptiness` (9 cases) passes, and so do the host regression and probe P1. The remaining check-then-act gap in the test fixture is N4. |
| H2 | Interface-bound static/identity mappings twice under one key | **FIXED** (see N3 for over-collapse) | `dedupeByName` (`nat44ed/mapping.go:562`, `nat44ei/objects.go:981`). The generic Retrieve fails with `ErrDuplicateKey` (`descriptor.go:611`), and `Apply`/`AssertPlan` fail on duplicate keys (`plan.go`). Host: `ifmap`/`eiif`, and probe `idif`/`smif`, give one key each. |
| H3 | pnat index recovery non-atomic; index/id deletes unverified | **FIXED** | pnat: recovery is accepted only when the before and after `get(0)` snapshots are equal (`pnat.go:405-424`). `bindingAt` re-verifies right before del and attach, `bindingIDAt` before detach, and an attached binding is refused (`ErrAttached`). Duplicates become `#index` extras. cnat `translationAt` and map `domainAt` re-verify identity. Probes P3/P4 confirm this on the host. The window between verify and act (two binapi calls) remains, and VPP has no atomic alternative; that is acceptable. |
| M4 | Globals claimed by every slot; tests reset to defaults | **FIXED** | Globals are owner-only (D-071), and the tests treat them as requirements with before == after. Plugins are fixtures that are disabled only if the test enabled them and they are empty. Probe P1 confirms no change. **But** the MAP "before == after" assertion compares uninitialised reply bytes and fails most runs (N1). |
| M5 | Production scope claims everything; Create accepts foreign interfaces | **FIXED** | `Scope.InterfaceOwnership` (`scope.go:369-380`) never claims a foreign tag, including for `All`. Untagged objects need a `ClaimStore` record (`NeedsClaim`). Create resolves through DF-1 `iface.ResolveName` → `ErrForeignInterface`. Probe P5 shows vrx sees 0 of w9's objects and foreign Creates are refused. |
| M6 | Write-only enables ignore a VRF change (det44, nat66) | **FIXED** (documented limitation) | Same-process VRF memory → `nat66.ErrVRFChange` / `det44.ErrVRFChangeUnsafe`, with unit tests "vrf change via create". A plugin enabled before the process started is documented as unverifiable (`nat-common.md`, Q11). |
| L7 | pnat attachment Delete errors when nothing is attached; single batch | **FIXED** | `pnat.go` Delete returns nil when `pnatInterfaces` is empty, and the crashing detach is still never sent. `pnatInterfaces` follows EAGAIN cursors. |
| L8 | cnat guards check-then-act; policy Delete on a foreign entry | **FIXED** | `HostLock(/run/lock/vrx-nat-cnat.lock)`: shared around the guard+policy/exclude sequence, exclusive around entry create/delete (`cnat.go` `lock`). Policy and entry are globals, so a non-owner Delete is a no-op (host: "a non-owner's Delete removed the default SNAT entry" is not triggered). |
| L9 | Branch does not merge | **FIXED** | It merges cleanly with current main (74f35f9). `go.mod`/`go.sum` are main's. |
| INFO 10 | det44 never disabled; pnat/cnat guards | Still holds | No `det44_plugin_enable_disable(enable=0)` path. The pnat detach/lookup guards are intact. |

D-069 (logical names): **done**. See probe D-069 above. Retrieve reports the tag id for own interfaces and the VPP name for untagged ones.
D-076: **partially done**. The idempotency itself works (3 resyncs → 1 add on the host). Two gaps are listed below (N2), and restart persistence depends on P05 passing a persisted `ClaimStore` (INFO I1).

## New findings, ranked

### N1. MEDIUM — the MAP host test is flaky: it compares uninitialised VPP reply fields
`apps/agent/internal/descriptors/mapnat/mapnat_integration_test.go:69` (`*after != *before`).
- **Cause:** VPP 26.06's `vl_api_map_param_get_t_handler` (`/root/vpp/src/plugins/map/map_api.c:421-458`) allocates the reply with `vl_msg_api_alloc`, which does not zero it. It never writes `ip4_lifetime_ms`, `ip4_pool_size`, `ip4_buffers` or `ip4_ht_ratio`, so those fields carry heap garbage. My runs showed, for example, `IP4LifetimeMs:33 IP4Buffers:50331648 IP4HtRatio:4.5e-318` → `0 0 1.1e-284` between two consecutive gets.
- **Failure:** `TestMapOnHost` failed with "a non-owner changed MAP params" in **4 of 6** runs, even though nothing changed. `tools/ci.sh full` will flake. The pasted "map params required only, unchanged" line is a lucky pass.
- **Fix:** compare only the fields the descriptor models: build a `mapnat.ParamsSpec` from both replies (the test already builds `cur`) and compare those, or zero the four unset fields before comparing. Add a one-line note to `map.md`, and optionally a vpp-code-track V-item (the reply leaves 4 fields uninitialised).

### N2. MEDIUM — the D-076 excluded-prefix record outlives the default SNAT entry, so a recreated entry silently loses the exclusion
`apps/agent/internal/descriptors/cnat/cnat.go:705-734` (record `<key>@vpp<pid>`, skip at `:717`).
- **Cause:** VPP drops the excluded prefixes together with the default SNAT entry (the code comment says so, `cnat.go` newSnatAddresses). The record, however, is keyed only by the VPP process.
- **Host probe:** add the prefix, then delete and recreate the entry on the same VPP process. Create is now **skipped** (the add count stays the same), and `show cnat snat-policy` no longer lists `10.9.50.0/24`. Retrieve is write-only, so nothing detects the drift until VPP restarts.
- **When this happens:**
  - For a non-owner slot (D-071), the entry belongs to the globals owner, so any owner-side recreate triggers it.
  - For the owner itself, it depends on whether P05's "re-create every dependent" (scheduler contract, `descriptor.go:34-36`) calls Delete before Create for each dependent. If it only re-Creates, the skip hits here too.
- **Consequence:** traffic to an excluded prefix gets SNATed again, silently.
- **Fix:** include the entry's generation in the record. There is no generation id in VPP, so the practical options are:
  - have the owner's `SnatAddresses` Set/Reset release every `cnat.snat-exclude-prefix/*@vpp<pid>` record in the shared ClaimStore;
  - key the record on `(pid, snat addresses, a per-process counter bumped by Set)`.

  For non-owners, document it as a D-063/D-076 limitation in `cnat.md` and `nat-common.md`. At minimum, add a unit test that models "entry recreated → prefix must be re-added".

### N3. LOW — `dedupeByName` collapses genuinely different mappings that share a tag, so one is hidden and never deleted in that pass
`apps/agent/internal/descriptors/nat44ed/mapping.go:562-577` (the same code is in `nat44ei/objects.go:981`).
- **Cause:** it keeps one item per *name* and drops every other detail with that tag, not only the resolved twin of an interface-bound mapping.
- **Host probe:** two raw ED static mappings tagged `w9:dup` (local :80→3080 and :81→3081). Retrieve returned **1** key with no `ErrDuplicateKey`. `DeleteAll` removed :80, and only the next Retrieve showed :81.
- **How it arises:** from a failed rollback of an Update=recreate (Delete old → Create new → the rollback's delete of the new one fails → old re-created), or from a manual or second instance.
- **Consequence:** the reconciler plans against the wrong mapping (Update → recreate → "Value already exists" → rollback), or leaves an orphan until another pass.
- **Fix:** collapse only details with the same local IP/port/proto/VRF (the resolved twin), plus the multi-local identity details of one mapping. Report any other same-tag entry as `ErrDuplicateKey` or as a `name#<n>` extra (the D-066 pattern already used for MAP and pnat). Add a unit case to the fake.

### N4. LOW — the test fixture's plugin disable is still check-then-act across slots
`apps/agent/internal/descriptors/natcommon/nattest/fixture.go:49-65`.
- **Race:** `EnsurePlugin`'s cleanup runs `Empty()` and then disables. If another slot's test finds the plugin "already enabled" and adds a pool between those two calls, the pool is wiped.
- **Scope today:** only slot 9 runs NAT host tests, so the window is milliseconds. The F-nat* tasks will run on other slots.
- **Fix:** the original review's recommendation, test-side only: hold a host-wide `HostLock(dir, "<plugin>", shared)` for the test's lifetime, and take it exclusive around `Empty`+`Disable` (`HostLock` already exists).

### INFO
- **I1:** restart persistence depends on P05. The default `ClaimStore` is in memory. After an agent restart:
  - every untagged object of a production owner (pools, prefixes, translations, bindings, objects on untagged interfaces) becomes invisible to Retrieve and is re-Created. That gives VPP "already exists" errors, or pnat duplicates that surface only as unclaimed `#index` extras and are never deleted;
  - the D-076 records vanish, so the probe saw a 2nd prefix add.

  This is the DF-4/DF-1 pattern, and `WithClaims` exists. P05 must wire a persisted store into all 8 families, and its restart-safety check should include an untagged NAT pool.
- **I2:** D-076 records from earlier VPP boots (`@vpp<oldpid>`) are never released, so a persisted store grows by one key per prefix per VPP restart. Prune records with a PID other than the current one on Create.
- **I3:** Q9 (`ErrRetrieveUnsupported` local copy) is still open until P05 merges. As stated.

## Verdict

H1, H2, H3, M4, M5, M6, L7, L8 and L9 are fixed and reproduced as fixed on the shared host. No VPP restart occurred, and CI passes. None of the new findings lets one owner damage another owner's state in the product configuration. N1 (flaky MAP host test) and N2 (D-076 record outliving the SNAT entry) should be fixed before or with the merge. N3 and N4 can follow in a small fix-up or in F-nat*.

**APPROVE WITH CHANGES**
