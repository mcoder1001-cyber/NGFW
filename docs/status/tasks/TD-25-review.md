# TD-25 review: ifsanitize resurrects on demand only (classify-pool ratchet)

Reviewer: TD-25 reviewer, 2026-09-25. Branch `task/TD-25` @ 6841297f; the fix is fbb1af46. Base: `main`.
Read: the envelope; TD-25.md and TD-25-questions.md; `docs/vpp-code-track.md` V19, V21 and V23; LOG entries D-063, D-095,
D-105, D-126, D-128 and D-132; TD-3-rereview.md; TD-5-review.md; the evidence logs in `/root/ngfw-wt/logs/TD-25-evidence/`;
the full diff; and VPP v26.06 in `/root/vpp` (read only).

## Verdict: **APPROVE**

The fix is correct, and it is no less safe than the old sanitizer. Every binding kind is cleared at least as well as
before. The delete phase and the output-ACL verify are stronger than before. The ratchet is gone, the model matches
vppinfra, and the host evidence proves that the free list did not grow. There is no H or M finding.

Two Low items should follow the merge:
- **L2** (docs): the manager applies it, because the file is outside the worker's set.
- **L1** (hardening): a tech-debt row.

Neither blocks the merge. Please merge now: every af_packet create on the shared VPP is waiting for this fix.

## 1. Safety: nothing V19/V23 cleared before is missed now

| kind | old (main) | new | verdict |
|---|---|---|---|
| input ACL ip4/ip6/l2 | `classify_table_by_interface`; unbound if the table exists after resurrect-all | Same readback (`sanitize.go:589`). `named()` unbinds when the table exists, otherwise pops down to it (`:369-391`, `:433-505`). The input ACL is always re-read in `verify` (`:757-771`). | same or better |
| output ACL ip4/ip6/l2 | an unbind probe over every live table and every placeholder (all free indices, ≤ cap) | Probe table P (`:696-738`): (a) an unbind of P, (b) bind P then unbind P, (c) the existing tables, then pops until an unbind works (`:460-472`). `verify` binds and unbinds P again on every slot that was bound (`:772-792`). | same coverage; verify is stronger (the old verify accepted NO_SUCH_TABLE, which a freed binding also returns) |
| policer classify ip4/ip6/l2 | an unbind probe over every table | exact readback: `policer_classify_dump(sw_if_index 0)` (`:599-620`), re-read in verify | better (exact) |
| flow classify ip4/ip6 | the same probe | exact readback: `flow_classify_dump(sw_if_index 0)` (`:623-644`) | better |
| l2 feature bits | L3-mode reset first | unchanged (`:281-288`, first step) | same |
| ip / l2 classify tables | reset blindly | unchanged (`:796-824`) | same |
| IPsec SPD | `ipsec_spd_interface_dump` | unchanged (`:862-919`) | same |
| vxlan bypass | reset blindly | unchanged | same |
| ADL | disabled blindly | unchanged | same |
| delete phase | no resurrect, live tables only | the same on-demand resurrect as the create phase | better |

### The sw_if_index-0 dump is verified in VPP source
In `classify_api.c:473-507` (policer) and `:756-790` (flow):
- **Bounds check.** The handler returns early when `filter >= vec_len(v[type])`. So `~0` returns nothing, and an empty or
  NULL vector returns nothing for `0` as well.
- **Why 0 is the whole vector.** For `0`, `vec_tbl = &v[type][0]`, which is the vector pointer itself. `vec_len(vec_tbl)`
  therefore reads the real header, and `i` runs over every sw_if_index. The reply is a dump of all indices, not only
  interface 0.
- **Why other indices are unsafe.** For any `k > 0`, `vec_len(&v[k])` reads element memory as a vec header, which is out
  of bounds.
- **The type field.** `mp->type` is not validated by VPP, but Sanitize sends only 0–2 (policer) and 0–1 (flow). These
  match `POLICER_CLASSIFY_TABLE_{IP4,IP6,L2}` and `FLOW_CLASSIFY_TABLE_{IP4,IP6}`, and `aclSlots` uses the same order.
- **Only our own rows are used.** Rows are filtered on `d.SwIfIndex == s.idx`.
- **Host proof.** `TestV19InheritanceClearedOnHost` on the host (`host-ifsanitize.txt`) cleared policer-classify and
  flow-classify ip4 on a reused index other than 0.

### D-132 and D-126
- **D-132 (no fast polling).** The walk covers one small per-sw_if_index vector: its length is the highest index that
  ever had a binding. It is not a FIB or session table, it runs only on interface create and delete, and it is never
  polled. D-132 does not apply.
- **D-126 (no classify sweeps by index).** Every unbind names one of four things: the table a readback named, the run's
  own P, an existing table (only on an output-ACL slot proven bound), or a placeholder the run just popped. No unbind
  names an index that does not exist, and none walks a range of indices. The old code probed every table on every run;
  the new code narrows that. This is not a classify sweep.

### The output-ACL probe placeholder
In `in_out_acl.c:84-121`:
- An add checks `pool_is_free_index(P)` (P is live), then returns 0 unchanged when the slot is bound. It binds only an
  empty slot.
- An unbind returns NO_SUCH_TABLE unless the slot equals P.
- So (b) proves exactly "the slot was empty", and leaves it empty.

While P is bound (two API calls, only on an empty slot of the given sw_if_index), it is a live table with no sessions and
`miss_next_index = ~0`. `ip_in_out_acl.c:320/582` and `l2_in_out_acl.c:356` then take the feature's default next. P
therefore matches nothing, and packets pass unchanged. The 16-byte signature mask only feeds the hash of an empty table.
**No crash path hits P while it is live.** The crash needs a freed table; see L1 for the one theoretical way P could be
freed while it is still bound.

## 2. Ratchet

- **The LIFO model matches `vppinfra/pool.h`.** `_pool_get` pops `free_indices[n_free-1]` (`:153-161`) and grows the
  vector only when the free list is empty. `_pool_put_index` does `vec_add1(free_indices, index)` (`:275`). Nothing
  shrinks the vector. `vnet_classify_new_table` uses `pool_get_aligned_zero`, and the delete uses `pool_put`
  (`vnet_classify.c:130,171`). The model's handler (`sanitizetest.go:186-211`) pops `Free[n-1]`, appends on delete, and
  never lowers `Len`. That is exact.
- **Pops.** A run pops the probe table, which is the top of the free list. It pops further only while a pending binding
  waits (`resurrect`, `:433-505`): down to the named index, or for an unknown output-ACL slot, until an unbind works.
  `dropPlaceholders` deletes in reverse creation order (`:409-425`), which restores the free list exactly. A fresh pop is
  possible only when the free list is exhausted: once per VPP on an empty free list (probe), and in the race below.
- **Ratchet test.** The base failure (`base-ratchet.txt`) is the incident's exact message in case 1, and +8 vector/+8
  free list per create in cases 2 and 3. I could not re-run it on a base copy, because a `git archive` into the
  scratchpad was denied by the permission policy. The worker's paste is consistent with the old algorithm, which I read
  on `main`. On HEAD, `TestNoPoolRatchet` passes with pool equality **after every Acquire and every BeforeDelete** (vector
  and free-list order), which is stronger than start/end only.
- **Another client took the named index (the `taken` logic).**
  - The index is not on the free list, so the pops run until a pop above `top`. `top` includes the named index
    (`named()` → `see`). At that point `reread()` (`:509-526`) marks the new tables `taken` and tries them.
  - The unbind then names the other client's live table, **on our sw_if_index only**. It removes our stale slot, which
    is the TD-3 re-review M1 requirement (TestHoleTakenBySomeoneElse). It does not touch that table or any other
    interface's binding.
  - Nothing outside `s.holds` is ever deleted. `reread` excludes held indices, and every delete first checks the
    placeholder's geometry and mask.
  - Cost: at most one fresh pop per race. If the free list is deeper than 63, the run is Capped, and the index is
    quarantined as a false positive. That fails closed and is safe.
- **Concurrent sanitizers (other slots).** They can reorder the free list but not grow it, unless their pops together
  exhaust it. Any growth is then bounded by the peak simultaneous demand and does not ratchet. TD-3 re-review L2 (one
  sanitizer blinding another) is closed as well:
  - for a readback kind, a NO_SUCH_TABLE on an index that is live but not ours becomes pending (`:385-389`);
  - for the output ACL, a slot proven bound keeps popping until an unbind works.

## 3. Fail-closed semantics and the Acquire retry

The retry is bounded by `MaxAcquireAttempts` (4, `acquire.go:52`). Each attempt makes at most 64 placeholders, so the worst
case is 4 × 64.
- **Where the retry applies.** Only `ErrUnclearable` retries (`:64`), after `del` and `Quarantine`. A run that is Capped
  but not Unclearable (race only) deletes the interface and fails without a retry.
- **Holders.** Each holder pins one index proven dirty. A holder that lands elsewhere is deleted at once (`:86-88`), and
  the reserved range caps the number of holders. There is no loop and no leak beyond the dirty indices, as in TD-3
  re-review answer 3.
- **The error path still fails closed.** Capped with Unclearable wraps ErrCapped, ErrNoCleanIndex and ErrUnclearable
  (`sanitize.go:238-239`). `TestAcquireFailsWithoutACleanIndex` covers exhaustion of the attempts.

## 4. Host evidence

I checked `host-fix.txt`:
- **Pops.** All 40 runs are `placeholders=1 pops=[22]`: 20 create and 20 delete, on sw_if_index 1 and 6.
- **Runs.** Ten PASS results.
- **Classify tables.** "No classifier tables configured" before and after.
- **NRestarts.** 2 before and 2 after.

`host-base.txt` shows the base code's capped probe three times, with the identical 64-pop sequence and
`holes_seen=122 holes_left=58`.

**Is "no growth" proven?** Yes, even though the vector length cannot be observed:
- vppinfra grows the vector only when the free list is empty;
- the free list held 122 entries (the base probe popped 120 and 121, so the vector is at least 122);
- every run popped exactly one entry, the top one (22), and pushed it back last;
- a fresh index was therefore impossible, and the identical top-64 pop sequence after the runs confirms it.

Q2's argument is sound, and I accept it.

After the host run, commit 1e7af394 removed only a redundant first try of the probe table inside `resurrect`. `named()` and
outputACL step (a) already cover that index, and a clean run never enters `resurrect`. The host evidence therefore still
applies to HEAD.

## 5. Metrics and docs

- **Metrics.** `vrx_agent_iface_sanitize_placeholders_total{phase}` is new, and the help text of `capped_total` is
  updated (`metrics.go:120-121`). The names follow the convention.
- **`sanitize.go` header** (`:1-76`). It is accurate for every kind, the resurrection, the exceptions and the cap.
- **V19 and V23 rows.** Their fix columns are correct, and V23(b) documents the sw_if_index-0 quirk and why the pool is
  unobservable.
- **The D-entry draft.** It is accurate. Add one clause: it reverses TD-3 re-review answer 2, "never binds". The
  sanitizer now binds its own empty probe table on an empty output-ACL slot, for two calls.
- **Q1.** Correct in substance, but incomplete. See L2.

## 6. Tests

This is what I ran:

```
cd apps/agent && TMPDIR=/tmp/g-rv25 go test -race -count=1 ./internal/vpp/ifsanitize/... ./internal/descriptors/af_packet/... ./internal/descriptors/interface/...
ok  ngfw/agent/internal/vpp/ifsanitize          11.645s
ok  ngfw/agent/internal/descriptors/af_packet    6.835s
ok  ngfw/agent/internal/descriptors/interface    1.182s
```

The V19, V21 and V23 tests still exercise real stale bindings:
- **`TestInheritedStateIsCleared`** (unchanged) plants ip table, l2 table, InACL, OutACL, Policer, Flow, vxlan, SPD and ADL
  on a deleted index.
- **`TestBindingToDeletedTable`** now asserts the exact pops `[5 6 2]` and the restored pool.
- **`TestOnDemandPopsOnlyDownToTheNamedIndex`** covers every readback kind plus the output ACL.
- **`TestCappedFailsClosed`** and **`TestAcquireCappedAndUnclearable`** cover the fail-closed path.
- **The host tests** (`host-ifsanitize.txt`: V19InheritanceClearedOnHost, V19FreedTableOnHost) clear real stale bindings
  on VPP. The output ACL is cleared twice through the probe table, and quarantine and Release work.

## Findings

### L1 (Low, hardening; tech-debt row): P can be deleted while it may still be bound on the error path
- **Where.** `sanitize.go:714-722` (outputACL step b) and `:781-791` (verify) bind P and then unbind it. If that unbind
  returns an error other than NO_SUCH_TABLE, the run returns the error. `run()` then calls `dropPlaceholders`
  (`:233`, `:409-425`), which deletes P without checking the slot.
- **Why it matters.** In the delete phase the callers abort the Delete on that error (`af_packet/host_interface.go:150`,
  `df6/ifdesc.go:248`, `wireguard/interface.go:139`). A live interface would keep `ip4-outacl` enabled on a freed table,
  and `vnet_classify_delete_table_index` has already done `vec_free(t->buckets)`. That is the V19 crash signature.
- **Why it is Low.** In VPP 26.06 the unbind of a live, bound P cannot fail except on transport errors. VPP processes one
  client's messages in order, so the unbind always runs before the delete. A cancelled ctx also fails the delete, which
  leaves P live, and a live P is harmless.
- **Fix.** Keep a `probeBound` flag, set before the bind and cleared after a confirmed unbind. When it is still set,
  `dropPlaceholders` leaves P in VPP and logs it: a live empty table is a leak, not a crash.

### L2 (Low, docs; the manager applies it, because the file is not in the worker's set): `docs/agent/descriptors/interface.md:128-193` still describes the old algorithm
The stale text covers `FreshRun`, `PlaceholderCap`, "a capped run fails Create without a retry", the "Placeholder cap"
section and the metrics list. Q1's replacement text is correct but incomplete. When applying it:
- **(a)** Replace steps 2, 3 and 5 and the whole "Placeholder cap (fail closed)" section, including the TD-3 re-review M3
  sentence, which is now done.
- **(b)** Say that the output-ACL check **binds** the run's empty probe table on an empty slot for two calls. This
  reverses the "never binds" answer.
- **(c)** Qualify "the pool ends exactly as it was". There are two exceptions: one fresh index per VPP when the free list
  is empty (the probe table, reused afterwards), and one fresh pop when another client takes the awaited index.
  Concurrent sanitizers may reorder the free list.
- **(d)** Add `vrx_agent_iface_sanitize_placeholders_total{phase}` to the metrics list, and update the `capped_total`
  gloss to "the binding is unclearable; create: the index is quarantined and the create retried".

Also close the tech-debt rows "TD-3 re-review M3" and "L1" (Q4).

### L3 (Low, nit): the comment in `sanitizetest.go:136` is inverted
`growLocked` prepends implicit holes in ascending order, so the **lowest** implicit hole pops first, not the highest. This
matters only in tests that plant tables directly with gaps. No test result depends on it.

### L4 (Low, nit): `probeTable` does not call `s.see` on P
See `sanitize.go:311-314`. `top` then excludes P, so `resurrect` may re-read `classify_table_ids` on more pops than needed.
With the host's 122-entry free list that could be about 50 cheap re-reads in a deep output-ACL search. Correctness is
unaffected: missing `top` only causes extra re-reads, never fewer.

## Answers to the questions file
- **Q1.** Apply it, with the L2 amendments.
- **Q2.** Accepted (§4).
- **Q3.**
  - (a) The sw_if_index-0 dump: verified in source; log it.
  - (b) The transient bind: safe; log it as reversing "never binds".
  - (c) The retry: bounded (§3).
  - (d) Delete-phase resurrect: fine; it stays non-fatal.
- **Q4.** Close the tech-debt rows "TD-3 re-review M3" and "L1".
- **Q5.** I agree with keeping the exact report. Six journal lines per run is acceptable.
