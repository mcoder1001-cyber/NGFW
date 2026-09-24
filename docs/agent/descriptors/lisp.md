# lisp / lisp-gpe descriptors (DF-6, WBS D6.8 — minimal set, T3)

Package `apps/agent/internal/descriptors/lisp`. Messages only from `apps/agent/binapi/lisp`, `lisp_gpe`, `lisp_types`.
Shared rules: [df6.md](df6.md). LISP objects have no owner tag; an object is ours only through the ClaimStore record
of our own Create (D-071). `lisp.enable`, `lisp-gpe.enable` and `lisp.pitr` are VPP-global: setters only on the
globals owner (never deleted on absence; disabled only when `lisp.SafeToDisable` finds no LISP object of any owner),
require variants elsewhere.

| Object type | Descriptor / key | Create / Delete | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| LISP enable (global) | `lisp.enable` · `lisp.enable/global` | `lisp_enable_disable` (Delete disables) | `show_lisp_status` | — | — |
| GPE enable (global) | `lisp-gpe.enable` · `…/global` | `gpe_enable_disable` | `show_lisp_status` | — | — |
| Locator set | `lisp.locator-set` · `…/<name>` | `lisp_add_del_locator_set` | `lisp_locator_set_dump` (local) | `ErrRecreate` | `lisp.enable/global` |
| Locator | `lisp.locator` · `…/<set>/<interface>` | `lisp_add_del_locator` | `lisp_locator_dump` per local set | `ErrRecreate` | `lisp.locator-set/<set>`, `interface/<if>` |
| Local EID | `lisp.local-eid` · `…/<vni>/<eid>` | `lisp_add_del_local_eid` | `lisp_eid_table_dump` (local) | `ErrRecreate` | locator set, `lisp.enable/global`, `lisp.eid-table-map/l3\|l2/<vni>` (vni ≠ 0) |
| Map resolver / map server | `lisp.map-resolver` / `lisp.map-server` · `…/<address>` | `lisp_add_del_map_resolver` / `_map_server` | `lisp_map_resolver_dump` / `_map_server_dump` | `ErrRecreate` | `lisp.enable/global` |
| Remote mapping (static) | `lisp.remote-mapping` · `…/<vni>/<eid>` | `lisp_add_del_remote_mapping` | `lisp_eid_table_dump` (remote) + `lisp_locator_dump` of its remote set | `ErrRecreate` | `lisp.enable/global`, EID-table map |
| Adjacency | `lisp.adjacency` · `…/<vni>/<reid>/<leid>` | `lisp_add_del_adjacency` | `lisp_eid_table_vni_dump` + `lisp_adjacencies_get` | `ErrRecreate` | remote mapping, local EID |
| EID-table map | `lisp.eid-table-map` · `…/l3/<vni>` or `…/l2/<vni>` | `lisp_eid_table_add_del_map` | `lisp_eid_table_map_dump` (l2 + l3) | `ErrRecreate` | `vrf/<dp_table>` or `bridge-domain/<dp_table>` (DF-1) |
| PITR (global) | `lisp.pitr` · `lisp.pitr/global` | `lisp_pitr_set_locator_set` | `show_lisp_pitr` | set in place | locator set, `lisp.enable/global` |
| GPE forwarding entry | `lisp-gpe.fwd-entry` · `…/<vni>/<reid>/<leid>` | `gpe_add_del_fwd_entry` | **partial: write-only** (V13) | `ErrRecreate` | `lisp-gpe.enable/global`, `vrf/<dp_table>` |

EIDs are strings: an IP prefix (canonical, masked) or a MAC; NSH EIDs are not modelled. The map-register HMAC key is not
modelled (secret; map-server authentication out of scope). RLOCs / locator pairs are compared as sorted lists.

VPP quirks handled
- `lisp_eid_table_details` carries a non-src/dst EID in `seid` (not `deid`).
- It reports `locator_set_index = ~0` when the set has no locators; Retrieve then shows an empty `locator_set` (diff
  until a locator is added); Delete still works (VPP only needs some valid set name).
- `lisp_enable_disable` also switches LISP-GPE: desired state with `lisp.enable` should include `lisp-gpe.enable`.
- Remote-mapping delete also removes adjacencies with that reid (the scheduler deletes adjacencies first anyway).
- **V13**: `gpe_fwd_entry_path_details` is sent with the message id *without* the plugin base, so the client receives a
  different message (`memclnt.GetFirstMsgIDReply` here) — locator pairs cannot be read back; `lisp-gpe.fwd-entry` is
  write-only with an existence probe (`Present`, via `gpe_fwd_entry_vnis_get` + `gpe_fwd_entries_get`). Entries the
  LISP control plane programs for its adjacencies are excluded from that probe.
- **V14 (leak)**: deleting a remote mapping leaves its auto-created `<remote-N>` locator set (`show lisp locator-set`).
- Disabling LISP leaves the down `lisp_gpe0` / `lisp_gpe<vni>` interfaces VPP created (reused, not deletable by API).

Host test: opt-in `VRX_DF6_LISP_HOST=1` (turns the global LISP switch on only if it was off and restores it), with
`VRX_DF6_LISP_UPTO=<n>` for stepwise bring-up. Per the manager rule after the review it is **not run on the shared
VPP** any more (LISP is a global only the globals owner may switch); the evidence from the first round stands, the
fix-round behaviour is covered by the unit tests (claims, resync without re-add, require variants, emptiness check).
