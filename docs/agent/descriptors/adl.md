# adl descriptors (DF-2, WBS D2.4 allow/deny lists)

Package `apps/agent/internal/descriptors/adl`. Messages from `apps/agent/binapi/adl` only.

| Object | Descriptor / key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| ADL on an interface | `adl.interface` / `adl.interface/<ifname>` | `adl_interface_enable_disable`; Update → `ErrRecreate` | **none — partial** | `interface/<ifname>` | The adl API has no dump; Retrieve returns `df2.ErrRetrieveUnsupported` (never cached desired state). |
| allow-list binding | `adl.allowlist` / `adl.allowlist/<ifname>` | `adl_allowlist_enable_disable` (fib_id, ip4, ip6, default_adl); Update in place | **none — partial** | `interface/<ifname>`, `vrf/<fib_id>` | Same limitation. |

Write-only descriptors cannot detect drift or leftovers; the reconciler must treat `ErrRetrieveUnsupported` as "unknown actual state" (DF-2-questions.md Q2).
