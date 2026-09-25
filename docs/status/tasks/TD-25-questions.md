# TD-25 — questions for the manager (non-blocking; the work followed the envelope)

## Q1. `docs/agent/descriptors/interface.md` is stale and outside my owned files
Lines 131–193 describe the old resurrect-every-index algorithm: `FreshRun` (8), `PlaceholderCap`, and "a capped run fails
Create without a retry". I did not edit the file, because the envelope does not give it to me. Proposed replacement for the
resurrect and cap paragraphs, which the manager can apply or hand to a docs task:

> Sanitize reads the interface's bindings back exactly: input ACL (`classify_table_by_interface`), policer/flow classify
> (`policer_classify_dump` / `flow_classify_dump` with sw_if_index 0, the only safe value in VPP 26.06) and the SPD. The
> output ACL has no readback, so each slot is checked with one probe placeholder. When a binding names a deleted table, only
> that freed index is brought back: placeholders pop the classify pool's LIFO free list down to it, and are deleted in reverse
> order. The pool ends exactly as it was (TD-25). At most `MaxPlaceholders` (64) placeholders per run. A deeper binding is
> unclearable (`ErrCapped` + `ErrUnclearable`); the index is quarantined and the create retried on another index.
> `vrx_agent_iface_sanitize_placeholders_total{phase}` counts placeholders (one per run when nothing is inherited).

## Q2. There is no "placeholder-free method" of reading the free list
The envelope asked for a before/after probe "via the placeholder-free method you implement". VPP 26.06 has none:
- `classify_table_ids` and `show classify tables` list live tables only;
- `pool_is_free_index` is 1 for every index beyond the vector;
- no API reports the vector length.

A free-list entry at the tail and a fresh index are therefore indistinguishable by popping. The host proof in TD-25.md §3
uses the sanitizer's own logged pops instead: one pop per run, always 22, a proven free-list entry. It also uses the base
code's capped run as a before/after probe: it pops only from the free list, restores it, and reported the identical 122 freed
indices and 64-pop sequence three times. Please confirm that this is acceptable evidence.

## Q3. Decisions to log (draft D-entry in TD-25.md)
- **(a) The sw_if_index-0 dump.** `policer_classify_dump` / `flow_classify_dump` are used with sw_if_index 0. This relies on
  26.06's `vec_len(&v[sw_if_index])` quirk, the same code D-063 called broken. For 0 it is well defined and returns the whole
  vector. `TestV19InheritanceClearedOnHost` (reused index other than 0) fails if a VPP upgrade changes it; the V23 row
  documents it.
- **(b) The transient probe-table bind.** The output ACL check binds the run's empty probe placeholder for two API calls, and
  only on an **empty** slot of the given interface. Before, the sanitizer "never binds" (TD-3 re-review answer 2). This is
  the only binapi way to tell a bound output-ACL slot from an empty one. The alternative, `cli_inband "show outacl"` in the
  agent, would need a LOG entry: TD-3 decision 4 kept the CLI for CI only.
- **(c) Acquire retries after a capped quarantine.** Before, it failed without a retry, because the cap was a pool property.
  Now the cap is hit only when this index's binding names a freed index deeper than 64 pops.
- **(d) The delete phase resurrects on demand too.** Before, it never resurrected. It stays non-fatal.

## Q4. docs/tech-debt.md
The row "TD-3 re-review M3" (probing → exact per-interface readback) is done by this task, and so is re-review L1 (the
FreshRun blind spot). Please close them. The file is not in my owned set.

## Q5. The journal is 6 lines per run, not 0
The output-ACL check's first step (an unbind naming the probe table) logs "Non-existent intf_idx … for delete" once per slot
when the slot is empty. VPP logs each twice, which gives 6 lines per run (59 before). Swapping the order (bind first) would
make it 0 lines. But a stale binding that names the probe table's own index would then be removed without being reported as
Freed. This is exactly the common case after a raw delete, and `TestV19FreedTableOnHost` counts it. I kept the exact report.
