# policer descriptors (DF-7, WBS D7.8)

Package `apps/agent/internal/descriptors/policer` — VPP 26.06 `policer` plugin (+ `classify` for classifier
policing). Message names only from `apps/agent/binapi/{policer,policer_types,classify}`.

Common to every DF-7 package (`internal/descriptors/df7`): values are `*structpb.Struct` documents of the typed
specs (D-055 stand-in; build with `df7.Encode(spec)`, read with `df7.Decode[T]`; every field `omitempty`, so zero
and absent are one canonical form), interfaces are named by their **logical name** and resolved with DF-1's
`iface` resolver (D-069; another owner's interface → `ErrForeignInterface`), objects on an untagged interface are
owned only through a claim of the object's key in DF-1's `iface.Claims(owner)` store (D-071), every Delete
re-resolves by logical name instead of trusting a stored index (D-071), write-only descriptors return
`df7.ErrRetrieveUnsupported` (D-063; text = the scheduler's sentinel) and are idempotent under resync (D-076).

## Object ↔ message table

| Object type (descriptor) | Key | Create / Update / Delete | Retrieve | Notes |
|---|---|---|---|---|
| `policer.policer` | `policer.policer/<name>` | `policer_add` / `policer_update` (in place, index kept) / `policer_del` | `policer_dump_v2` (all) + per-index `policer_dump_v2` to learn the pool index | VPP name = `vpp.OwnerTag(owner, name)` (`w10:gold`); Meta `{Index}`; cir/eir/cb/eb, rate type kbps/pps, round type, type 1r2c/1r3c-2697/2r3c-2698/2r3c-4115/2r3c-mef5cf1, color-aware, conform/exceed/violate action + dscp all decoded |
| `policer.interface` | `policer.interface/<if>/<input\|output>` | `policer_input` / `policer_output` (apply) — Update = un-apply old + apply new | **write-only** (no dump) | name-based v1 messages (no index needed); apply stacks the feature node → applied once per VPP boot identity (`df7.ApplyOnce`, D-076); un-apply only if applied in this VPP lifetime (VPP writes out of bounds otherwise) |
| `policer.bind` | `policer.bind/<policer>` | `policer_bind` (enable / disable) | **write-only** | idempotent; no workers on the host → `INVALID_WORKER` (test: `skip: no workers on host`) |
| `policer.classify` | `policer.classify/<if>` | `policer_classify_set_interface` is_add 1/0; Update = ErrRecreate | **write-only** | tables are DF-2 `classify.table/<name>` keys, resolved with `df7.WithClassifyTables` (P05 wires DF-2's Store); VPP keeps the first table while enabled |

Action helper: `policer.Reset(ctx, client, index)` (`policer_reset`). `policer.LookupIndex(ctx, client, owner, name)`
gives the pool index to other plugins.

## Dependencies

`policer.interface` → `policer.policer/<name>` + `interface/<if>` · `policer.bind` → `policer.policer/<name>` ·
`policer.classify` → `interface/<if>` + `classify.table/<t>` for each named table.

## VPP 26.06 findings (DF-7-questions.md)

- `policer_details` carries no pool index → Retrieve walks `policer_dump_v2(index)` (a free slot answers nothing)
  until every owned policer is found; Delete re-verifies `dump_v2(index).name` right before `policer_del` and
  falls back to a lookup by name (index reuse after a VPP restart).
- `policer_input_v2` / `policer_output_v2` handlers reply with the v1 reply message id (policer_api.c); govpp 0.13
  accepts it (host probe: `err=<nil>`), the descriptors use the v1 by-name messages anyway.
- `policer_input(apply=0)` writes `policer_index_by_sw_if_index[dir][sw_if_index]` without `vec_validate`
  (policer_op.c) → out-of-bounds write on an interface that never had a policer since VPP start.
- `policer_classify_dump`: sw_if_index ~0 is compared against the vector length and returns nothing; a single
  index calls `vec_len` on a pointer into the vector (out-of-bounds read) → never called; write-only.

## FIB entries

None.

## Review fixes

- `policer.interface`: the applied-once record holds the D-080 boot identity (kernel boot_id, VPP PID, start time)
  and `<sw_if_index>/<name>`; Delete sends `apply=0` only with a matching record — never on an interface that was
  re-created, after a VPP restart (also with a repeated PID) or after a failed apply (review H1).
- `policer.policer` Update re-verifies the stored pool index by name before `policer_update` (review M5); `Reset`
  takes the policer name and looks the index up.
