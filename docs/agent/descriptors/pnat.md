# pnat descriptors (DF-3)

Package `apps/agent/internal/descriptors/pnat`. Bindings are in `apps/agent/binapi/pnat` (plugin `pnat_plugin.so`,
policy 1:1 NAT, loaded on vrx-a). Entry point: `pnat.Register(registry, client, owner)`. The carrier is
`*structpb.Struct` built from typed specs (D-055).

| Descriptor | Key id | Create / Delete | Update | Retrieve | Dependencies | Notes / limitations |
|---|---|---|---|---|---|---|
| `pnat.binding` | canonical match tuple `<proto>/<src>/<sport>/<dst>/<dport>` (`any` = wildcard) | `pnat_binding_add_v2` (match + rewrite; masks derived from the fields that are set) → Meta `{Index}` / `pnat_binding_del(index)` | recreate | `pnat_bindings_get` (cursor-paged, EAGAIN continuations followed); owned when any match/rewrite address is in the owner's scope | none | IPv4 only (the API is IPv4-only). Match ports require proto tcp/udp; an empty match or rewrite is rejected (as VPP does). Rewrite supports src/dst address, ports, copy-byte (from→to offset) and clear-byte (offset). **The index is not in the details:** it is recovered from the cursor semantics. `get(cursor=k)` returns every binding with index ≥ k in index order: one extra call when the pool has no holes, otherwise a binary search per binding (O(n log max-index) calls). VPP does not dedup bindings, so the match tuple is the stable id. |
| `pnat.attachment` | `<interface>/<input\|output>/<binding id>` | `pnat_binding_attach(sw_if_index, attachment, binding_index)` / `pnat_binding_detach` | recreate | `pnat_interfaces_get` for the attached interfaces, then `pnat_flow_lookup(sw_if_index, point, match)` for every owned binding on both points; attached when the lookup returns the binding's index | `pnat.binding/<id>`, `interface/<name>` | All bindings on one interface and point must share the same match mask (VPP rejects others with −2). |

**VPP 26.06 hazards (verified in `src/plugins/nat/pnat`, guarded in code and in the fake):**
- The flow hash is a `bihash_16_8` built with `BIHASH_LAZY_INSTANTIATE 0`. `pnat_flow_lookup` and
  `pnat_binding_detach` on a VPP where no binding was ever attached dereference an uninitialised table and **crash VPP**.
  The table is initialised by the first attach and never freed by a successful detach, so both messages are sent only
  while `pnat_interfaces_get` lists at least one interface. Otherwise Retrieve reports nothing and Delete returns
  `ErrFlowHashUninitialised` without calling VPP.
- `pnat_binding_detach` clears `enabled[point]` and turns the feature off on the interface even when other bindings
  remain attached there. Their flow entries stay, but traffic is no longer translated until the next attach. Retrieve
  ignores `enabled[]` and relies on the flow lookup. The scheduler deletes attachments before bindings (dependency),
  because `pnat_binding_del` on an attached binding leaves a dangling flow entry.
- `pnat_bindings_get` with cursor `~0` wraps to 0 in VPP (`cursor + 1` in u32). The recovery never sends it.

Tests: `pnat_test.go` runs on the fake. It covers index recovery over a pool with holes (with and without an EAGAIN
split), create, idempotent re-apply, invalid tuples, attachments on input and output, a foreign attachment kept, and
the crash guards (the fake fails the test if a lookup or detach is sent with no pnat interface).
`pnat_integration_test.go` runs on the host: bindings `udp 10.9.51.1 → 10.9.52.1:53` ⇒ `10.9.53.1:5353` and
`tcp *:80 → 10.9.52.2` ⇒ src `10.9.53.2` + clear-byte 3, attached on `loop950` input and `loop951` output. The test
checks create, Retrieve, idempotent re-apply and delete.
