# DF-2 — re-review after the fix round

Reviewer: an independent re-review agent (did not write this code). Branch `task/DF-2` @ eed9066. Fix commits: f3d1833, f5a5ab2, 5ca42e4 and 88205a4, plus the merge f70aff1. Base: `main` @ c7b28e3.
I ran directly on the host with the slot-3 env (`eval "$(tools/lab env 3)"`, `VRX_INTEGRATION=1`), one package at a time. `VRX_DF2_PROXY_ND` was not set.
`systemctl show vpp -p NRestarts` read **2** before and after every host run.

## Evidence I reproduced

- **Unit tests:** `go test -count=1 ./internal/descriptors/...` passed in all 11 packages. This includes the new review-fix tests: `TestStoreDropsRecordsOfAnotherVPPInstance`, `TestStoreRejectsReusedIndexWithOtherGeometry`, `TestPruneWaitsForCreate`, `TestTableCreateRollsBackOnStoreError`, `TestOutputACLSwitchesTables`, `TestOutputACLUnknownBinding`, `TestBindingsOnUntaggedInterfaces`, `TestSessionRefusesRedirectMatch` and `TestDuplicateACLTagsFollowDF4`.
- **Host tests:** I reran `ip_neighbor`, `arp`, `ip6_nd`, `urpf`, `adl`, `abf`, `classify`, `ip_session_redirect`, `df2` and `df2/idempotency`. All passed. The Retrieve lines match DF-2.md; the only difference is VPP-assigned indices.
  - `adl.interface Retrieve = interface:"loop306"`
  - `output-acl after A→B = … ip4_table:"w3-t2"`
  - Idempotency plans:
    - apply #1: create=17
    - apply #2: empty
    - apply #3 (fresh descriptors, reopened classify store and claim store): empty
  - Three write-only types are excluded.
  - After the runs: `show classify tables` returns "No classifier tables configured", and no `loop3xx` interfaces remain.
- **Probes on the host:** I wrote a temporary probe test in the classify package, ran it on slot 3 and deleted it. It was never committed. Results:
  - **P1:** I created our table and deleted it out of band. A "foreign" table with the **same** geometry then reused index 0, and `TableDescriptor.Retrieve` claimed it. This is the residual in N3.
  - **P2:** The same setup with a **different** mask. Not claimed, and the stale record was pruned. H1 is fixed.
  - **P3:** `TableDescriptor.Delete(obj, TableMeta{Index: <foreign>})` returned `nil` **and deleted the foreign table**. This is N2.
  - **P4:** `InputACL` Create by owner `w3other` on the `w3`-tagged `loop360` was refused with `ErrForeignInterface`. H3 is fixed.
  - **P5:** 8 concurrent `TableDescriptor.Create` calls ran against a tight `Retrieve` loop. All 8/8 tables were recorded and retrieved afterwards. M6 is fixed.
- **CI:** `tools/ci.sh --base main` gave `CI GATE PASSED`, EXIT 0, with the contract guard OK. The only warnings are the two non-conventional subjects (the merge commit and the manager's review commit), the same as pasted.
- **Contract and scope:**
  - `git diff --name-only main...HEAD -- packages/schema packages/proto apps/agent/gen packages/proto/gen packages/api-client/src/generated apps/agent/go.mod apps/agent/go.sum apps/agent/binapi tools/binapi-gen.sh` is empty.
  - `git merge-tree main HEAD` is clean.
  - There are no `vppctl` or `exec.Command` calls in non-test code.

## Original findings

| # | Verdict | Evidence |
|---|---|---|
| H1 stale classify record claims another owner's table | **FIXED** (residual: N3) | `classify/table.go` `snapshot`:<br>- The Store is bound to `control_ping_reply.vpe_pid`; any other pid resets it.<br>- A record is live only if `classify_table_ids` lists it **and** `classify_table_info` skip/match/mask equal the record.<br>- Unit tests cover both.<br>- Host probe P2: a different geometry on a reused index is not claimed. |
| H2 write-only types by default; output-acl A→B keeps A | **FIXED** (edge cases: N4) | - `adl.interface` reads presence with `feature_is_enabled(device-input, adl-input)` (host: Retrieve shows loop306).<br>- `classify.output-acl` reads `ip4-output/ip4-outacl` and `ip6-output/ip6-outacl`. Table names come from the Store's `OutputRecord`; an unrecorded bound table is refused on Create. Create unbinds the recorded binding first. The host A→B switch shows `w3-t2`. L2 output is refused.<br>- `adl.allowlist`, `interface-ip-table` and `interface-l2-tables` are only in `RegisterWriteOnly`, which matches D-063.<br>- Docs updated. |
| H3 untagged interfaces invisible; no foreign-interface check | **FIXED** | - `df2/claims.go`: DF-4's `acl.ClaimStore` plus `FileClaimStore`, claimed by object key.<br>- `Interfaces.Resolve` refuses foreign tags and `local0`.<br>- `OwnsObject` is applied in every Retrieve: neighbor, proxy-arp interface, RA config/prefix, proxy-ND, uRPF, adl, abf attach, input/output ACL.<br>- Host: idempotency apply #3 finds the objects on the untagged `loop352` only through the reopened claim store. Probe P4 confirms the refusal.<br>- This satisfies the D-071 claim rule for interface-attached objects. |
| H4 proxy-ND registered by default after a VPP abort | **FIXED** | - `ip6nd.Register` no longer registers it; `RegisterProxyNd` is opt-in.<br>- The host test is gated by `VRX_DF2_PROXY_ND` (SKIP in my run).<br>- The doc says "Unverified on the host, opt-in only (D-064)".<br>- Recorded as V12 in `docs/vpp-code-track.md` on main. |
| M5 ABF duplicate-tag resolution differs from DF-4 | **FIXED** | - `abf/policy.go:101` Create uses `acl.LookupIndex` (lowest index is canonical).<br>- `abf/acl.go` `DumpACLs` names non-canonical indices `name#idx`, so the policy diffs, and Update returns `ErrRecreate` on the ACL change.<br>- The dependency is `acl.KeyACL`; `TestDuplicateACLTagsFollowDF4` covers it. |
| M6 Retrieve mutates the store and races Create | **FIXED** | - `LiveTables`/`snapshot` are read-only, and the records are read before the VPP dump.<br>- `Prune` runs only under the Store's transaction lock (`sync.Locker`) on a fresh snapshot.<br>- Table Create, Delete and session Create/Delete hold the same lock.<br>- `TestPruneWaitsForCreate` covers it; host probe P5 gave 8/8. |
| L7 go.mod conflict with main | **FIXED** | The merge took main's go.mod/go.sum. The diff against main for go.mod/go.sum is empty and `merge-tree` is clean. |
| L8 (a) orphan table when Put fails | **FIXED** | `table.go:151-157` deletes the new VPP table on a Put error; `TestTableCreateRollsBackOnStoreError` covers it. |
| L8 (b) session overwrites a redirect's match | **FIXED** | `session.go:114-122` refuses a match that is already a redirect session; `TestSessionRefusesRedirectMatch` covers it. |
| L8 (c) global singletons without an owner | **NOT FIXED — now required by D-071** | Only documented (`ip_neighbor.md:15`, questions #8). D-071 now decides the question. See N1. |
| INFO restart pass in the committed test; arp nil-range doc | **FIXED** | Idempotency apply #3 (fresh descriptors, reopened stores); `arp.md` has the nil-range text. |

## New findings, ranked by severity

### N1. MEDIUM — the global singletons are still registered and set by every owner (D-071)
`ip_neighbor/register.go:11-14` (`r.Register(NewConfig(c))`) and `ip6_nd/register.go:13-17` (`r.Register(NewDad(c))`). Both are unconditional.
`ip_neighbor/config.go:75` and `ip6_nd/dad.go:84`: Delete **resets to VPP defaults**.

- **D-071** says VPP-global singletons are set only by the globals owner (`globalsOwner: true`). Non-owners may only *require* a global, never set or reset it.
- **Failure scenario:** any second agent or test-slot reconciler that calls the default `Register` has `ip-neighbor.config` / `ip6-nd.dad` in its descriptor set. Its desired state either:
  - overrides the neighbour limits and DAD for every other owner, or
  - (absent desired, present actual) Deletes the global, which resets it to VPP defaults under everyone.
- **Precedent:** DF-3 was BLOCKed for the same class (DF-3 review H1/M4, the reason D-071 exists).
- **Fix:**
  - Move `NewConfig`/`NewDad` out of the default `Register` into `RegisterGlobals(...)`, or register them only when a `df2.WithGlobalsOwner(true)` option is set.
  - Have a non-owner Retrieve not report them, so the scheduler never plans a Delete.
  - Add a unit test showing that the default `Register` does not include them.
  - Update `ip_neighbor.md`, `ip6_nd.md` and questions #8.
  - The tests already restore previous values, not defaults. That part is compliant.

### N2. MEDIUM — deletes by index do not re-verify identity (D-071)
D-071 says "Deletes by index must re-verify identity (tag/tuple) in the same call sequence immediately before deleting".

- **Classify tables.** `classify/table.go:174-191` `TableDescriptor.Delete` deletes `meta.Index`, or the Store's index, without checking the VPP instance or geometry.
  - **Host probe P3:** with a stale `TableMeta` pointing at an index now held by another owner's table, Delete returned `nil` and **removed the foreign table**.
  - **Window:** VPP restarts, or our table is deleted out of band, between the Retrieve that produced the meta and the Delete. Another owner's table then takes the index.
  - **Fix:** under the Store lock, run `snapshot` and delete only if the record is live (same `vpe_pid`, index listed, geometry equal) **and** `meta.Index == rec.Index`. Otherwise drop the record and return nil, because our table is gone.
- **ABF policy.** `abf/policy.go:175-190` `PolicyDescriptor.Delete` dumps the policy by id but never checks that its `ACLIndex` is still an ACL tagged by this owner before removing its paths. With the production nil range, a policy id reused by another owner or the operator is emptied.
  - **Fix:** compare `det.Policy.ACLIndex` with `meta.ACLIndex` and the owner-tagged set. Skip the delete if they differ.
- **Interface-attached objects.** uRPF, neighbour, adl, RA, proxy-arp interface, abf attach, input/output ACL Delete all act on `meta.SwIfIndex` without re-dumping the interface. A `sw_if_index` reused by a foreign interface would be modified.
  - **Fix:** one `df2.DumpInterfaces` plus an `OwnsObject` check (tag ours, or untagged and claimed) immediately before the call.

### N3. LOW — residual of H1: a same-geometry table on a reused index within one VPP instance is still claimed
Host probe P1 reproduced it: our table was deleted out of band, a foreign table with an identical mask and vectors got the same index, and Retrieve reported it as ours.
- Classify tables have no tag, so this cannot be closed completely. The probability can be reduced by also fingerprinting `nbuckets`, `next_table_index` and `miss_next_index`, which the record does not store today.
- **Fix:** extend the fingerprint and document the residual in `classify.md`.

### N4. LOW — output/input ACL recovery paths can get stuck
- **Output ACL, stuck record.** `classify/bindings.go:466-482` (`unbind`) and 544-560 (`Delete`) unbind with the **recorded** indices.
  - VPP rejects a delete whose table index is free (`/root/vpp/src/vnet/classify/in_out_acl.c:92`) or does not match the interface's bound index (`:99-107`), with `NO_SUCH_TABLE`.
  - If the recorded table vanished out of band, Create and Delete both fail on every reconcile: Retrieve reports `#unknown`, which leads to recreate, then unbind fails. The same happens if the interface was recreated with a new `sw_if_index` while the record, keyed by name, stayed.
  - **Fix:** in `unbind`, first check `feature_is_enabled`. If the feature is off, or the recorded table is not live, drop the record instead of calling VPP.
- **Input ACL.** Create (`bindings.go:341`) does not detect an existing binding, and VPP's add is a silent no-op when one exists (`in_out_acl.c:111-114`). Delete of a Retrieved value that contains `#<idx>` fails in `tableIndex` with `ErrNoSuchTable` forever.
  - **Fix:** refuse on Create when `classify_table_by_interface` already shows a table, as output-acl does. On Delete, unbind the dumped indices.

### N5. LOW — claim-store hygiene
- **Unattributed object on Claim failure.** `df2.Claim` runs after the VPP call. If it fails (disk error), the object exists but is unattributed, so it is invisible and never deleted. Undo the VPP call on a Claim error, the same way table Create now does on a Put error.
- **Stale claims.** Claims are never pruned and are not bound to the VPP instance. A claim whose object vanished while it was also removed from the desired state stays forever, and later adopts an identical object configured by someone else on that untagged port.
- **Proxy-ARP ranges.** With the production `nil` range, ranges are attributed by table-id range, not by a ClaimStore record. That literally deviates from D-071's "untagged → only via ClaimStore". This is acceptable for the single product agent, but state it in `arp.md` or ask the manager for a D-071 exception.

### N6. INFO / follow-ups (not blocking)
- **D-069:** DF-2 resolves interfaces by the **VPP** name (`sw_interface_dump`). When DF-1's logical-name resolver lands, DF-2 must switch to it. The claim keys embed the interface name, so persisted claims need a one-off key migration. Add this to questions.
- **D-073(c):** alias `df2.ErrRetrieveUnsupported` to the scheduler's copy once P05 merges. The scheduler copy is not on main yet.
- **Outside the file set:** `apps/agent/internal/descriptors/acl/integration_test.go:201` has a one-space gofmt-only change in a DF-4 file. It is harmless; mention it in DF-2.md or drop it.

## What must change before merge

- N1: gate the global descriptors behind the globals owner (D-071).
- N2: re-verify identity immediately before deletes by index, at least for classify table Delete (reproduced on the host) and ABF policy Delete.

N3, N4 and N5 can go to a follow-up task. N6 is for P08/P05 sequencing. Every original HIGH and MEDIUM finding is fixed and verified on the host.

**APPROVE WITH CHANGES**
