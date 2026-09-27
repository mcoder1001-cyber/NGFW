# dslite descriptors (F-det44-map-dslite-cnat, D4.5: DS-Lite AFTR / B4)

> **Ownership, globals (D-071), claims, unique keys and write-only re-application: see [nat-common.md](nat-common.md)** — it overrides older wording below where they differ.

Package `apps/agent/internal/descriptors/dslite` (new; DF-3 built none, DF-3-questions Q5). Bindings are in
`apps/agent/binapi/dslite` (plugin `dslite_plugin.so`). Entry point: `dslite.Register(registry, client, owner, opts…)`
with `natcommon.WithGlobalsOwner(env.GlobalsOwner)` and `natcommon.WithClaims(Wiring.KeyedClaims("nat"))`
(`subsystems/det44_map_dslite_cnat.go`). Carrier: `*structpb.Struct` from the typed specs via `natcommon.Encode`.

| Descriptor | Key id | Create / Delete | Update | Retrieve | Dependencies | Notes / limitations |
|---|---|---|---|---|---|---|
| `dslite.aftr` | `global` | owner: `dslite_set_aftr_addr(ip4, ip6)`; Delete writes `::` / `0.0.0.0` | same setter | owner: `dslite_get_aftr_addr`; absent when both addresses are unspecified | none | VPP global (D-071): a non-owner only requires the exact value (`ErrGlobalNotSet` / `ErrGlobalMismatch`), never writes, Retrieve → `ErrRetrieveUnsupported`. VPP has no delete: the reset writes the unspecified addresses (V-new (F-det44-map-dslite-cnat) a). |
| `dslite.b4` | `global` | owner: `dslite_set_b4_addr(ip4, ip6)`; Delete writes `::` / `0.0.0.0` | same setter | owner: `dslite_get_b4_addr` | none | Same rules as the AFTR. Only the B4 address is modelled (CE-side DS-Lite beyond it is out of scope). |
| `dslite.pool` | `<first>-<last>` | `dslite_add_del_pool_addr_range(start, end, is_add)`; delete of a missing range is a no-op | recreate | `dslite_address_dump` (single addresses) merged back into contiguous ranges, filtered by the slot's IPv4 range | none | Claim-store ownership like nat64 pools: a production owner (`All`) reports a range only when its key is claimed in the persisted `nat` store. |

Hazards: none known to crash VPP. The AFTR / B4 getters answer on a fresh VPP with all-zero addresses.
