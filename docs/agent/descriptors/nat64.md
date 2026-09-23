# nat64 descriptors (DF-3)

> **Ownership, globals (D-071), claims, unique keys and write-only re-application: see [nat-common.md](nat-common.md)** — it overrides older wording below where they differ.

Package `apps/agent/internal/descriptors/nat64`, binapi `apps/agent/binapi/nat64` (plugin `nat64_plugin.so`, loaded
on vrx-a). Entry point `nat64.Register(registry, client, owner)`. Carrier/ownership as in `nat44-ed.md`.

**No "is enabled" getter in VPP 26.06.** `nat64.enable` is **write-only** (D-063). Retrieve returns
`ErrRetrieveUnsupported`, and the reconciler re-applies the enable on every resync (VPP answers retval 1 "already
enabled", which is treated as success) and never disables on absence.

| Descriptor | Key id | Create / Delete | Update | Retrieve | Dependencies | Notes / limitations |
|---|---|---|---|---|---|---|
| `nat64.enable` | `global` | `nat64_plugin_enable_disable` (enable / disable; retval 1 = already, tolerated) | — (empty spec) | `ErrRetrieveUnsupported` (write-only) | — | Sizing (`bib/st buckets`, memory) has no getter → not modelled (VPP defaults). Disable (globals owner only) skipped while any object of any owner exists (nat-common.md). |
| `nat64.timeouts` | `global` | `nat64_set_timeouts`; Delete restores 300/7440/240/60 | in place | `nat64_get_timeouts` — object only when non-default | enable | Presence = non-default values. |
| `nat64.prefix` | `<prefix>/<vrf>` | `nat64_add_del_prefix` | recreate | `nat64_prefix_dump` | enable, optional `vrf/<id>` | Ownership: prefix in slot v6 block `fd00:<N>::/32` **or** vrf in slot table range. |
| `nat64.pool` | `<first>-<last>/<vrf>` | `nat64_add_del_pool_addr_range` | recreate | `nat64_pool_addr_dump`, merged into contiguous ranges per VRF | enable, optional `vrf/<id>` | Ownership by v4 address scope. |
| `nat64.interface` | `<interface>/<inside\|outside>` | `nat64_add_del_interface` (`NAT_IS_INSIDE`/`_OUTSIDE`) | recreate | `nat64_interface_dump` (+ `sw_interface_dump`) | enable, `interface/<name>` | `nat64_add_del_interface_addr` not modelled (no dump). |
| `nat64.static-bib` | `<proto>/<inside ip>/<inside port>/<vrf>` | `nat64_add_del_static_bib` | recreate | `nat64_bib_dump{proto: 255}` keeping `NAT_IS_STATIC` entries only | enable, optional `vrf/<id>` | Prefix and pool are registered before BIBs (plan order); ownership: outside v4 address in scope or vrf in range. |

Retrieve-only: `Sessions(protocol, offset, limit)` over `nat64_st_dump` (paged in the agent). Not used:
`nat64_add_del_interface_addr`.

Tests: `nat64_test.go` (fake incl. retval-1 "already enabled", write-only enable, foreign filtering),
`nat64_integration_test.go` (loopbacks `loop910/911`, tables `9010` v4+v6, prefix `fd00:9:40::/96`, pool
`10.9.64.1–2`, BIB `fd00:9::64:80 → 10.9.64.1:8080/tcp`; plugin is a test fixture: disabled again only if this test enabled it and it is empty).
