# adl descriptors (DF-2, WBS D2.4 allow/deny lists)

Package `apps/agent/internal/descriptors/adl`. Messages from `apps/agent/binapi/adl` only.

| Object | Descriptor / key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| ADL on an interface | `adl.interface` / `adl.interface/<ifname>` | `adl_interface_enable_disable`; Update → `ErrRecreate` | **presence** via `feature_is_enabled` (arc `device-input`, feature `adl-input`, `plugins/adl/adl.c`) on this owner's interfaces | `interface/<ifname>` | Default `Register`. The adl API itself has no dump. |
| allow-list binding | `adl.allowlist` / `adl.allowlist/<ifname>` | `adl_allowlist_enable_disable` (fib_id, ip4, ip6, default_adl); Update in place | **none — write-only** (`df2.ErrRetrieveUnsupported`) | `interface/<ifname>`, `vrf/<fib_id>` | **Not in the default `Register`** — only `RegisterWriteOnly`, for a reconciler implementing D-063 (re-apply on resync, never delete on absence, skip verification). Removing it from the desired state does not disable it after an agent restart. |

## Ownership on untagged interfaces (review H3)
Objects on interfaces tagged `"<owner>:<name>"` are ours. Physical ports (DPDK NICs) carry no tag: an object created on
an untagged interface is recorded by its **key** in the claim store (`df2.WithClaims(store)`, DF-4's `acl.ClaimStore`
interface; `df2.FileClaimStore` persists it in the agent state dir) and Retrieve reports it only while claimed. Interfaces
tagged by another owner — and `local0` — are refused with `df2.ErrForeignInterface`. Dependencies use the interface
alias key `interface/<name>` (D-065, DF-1).

Registration: `adl.Register` = adl.interface; `adl.RegisterWriteOnly` = adl.allowlist (D-063 reconciler only).
