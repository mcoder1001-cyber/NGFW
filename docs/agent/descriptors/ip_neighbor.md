# ip_neighbor descriptors (DF-2, WBS D2.3)

Package `apps/agent/internal/descriptors/ip_neighbor` (Go package `ipneighbor`). Messages from `apps/agent/binapi/ip_neighbor` only.
Values: package-local proto `ip_neighbor/model.proto` (D-055 stand-in until P03b adds domain messages).

| Object | Descriptor / key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| static neighbour | `ip-neighbor.neighbor` / `ip-neighbor.neighbor/<ifname>/<ip>` | `ip_neighbor_add_del` (flags `STATIC` + optional `NO_FIB_ENTRY`); Update = re-add with the new MAC; a `no_fib_entry` change → `ErrRecreate` | `ip_neighbor_dump` per AF, `sw_if_index=~0`; keeps STATIC entries on interfaces tagged by this owner | `interface/<ifname>` | Meta `{SwIfIndex}`. IPs canonical via `net/netip`, MAC lower-case. |
| neighbour DB config | `ip-neighbor.config` / `ip-neighbor.config/<ipv4\|ipv6>` | `ip_neighbor_config` (max_number, max_age, recycle); Delete restores VPP defaults (50000/0/false) | `ip_neighbor_config_get` per AF (always both families) | none | Global, not owner-scoped: in production the agent owns it; integration test saves and restores the previous value. |

Limitations
- VPP 26.06 `ip_neighbor_flags` has only `STATIC` and `NO_FIB_ENTRY`; the "no-adj-fib" flag named in the task prompt does not exist in binapi → not modelled.
- Dynamic (learned) neighbours are never retrieved (not configuration). `ip_neighbor_flush` is an action, not a descriptor.
- Ownership: see below.
- `ip-neighbor.config` is a global singleton without an owner: on the shared lab VPP any slot whose reconciler registers it overrides the others (tests restore it); in production there is one agent (review L8).

## Ownership on untagged interfaces (review H3)
Objects on interfaces tagged `"<owner>:<name>"` are ours. Physical ports (DPDK NICs) carry no tag: an object created on
an untagged interface is recorded by its **key** in the claim store (`df2.WithClaims(store)`, DF-4's `acl.ClaimStore`
interface; `df2.FileClaimStore` persists it in the agent state dir) and Retrieve reports it only while claimed. Interfaces
tagged by another owner — and `local0` — are refused with `df2.ErrForeignInterface`. Dependencies use the interface
alias key `interface/<name>` (D-065, DF-1).
