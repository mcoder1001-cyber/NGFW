# arp (proxy-ARP) descriptors (DF-2, WBS D2.3)

Package `apps/agent/internal/descriptors/arp`. Messages from `apps/agent/binapi/arp` only.

| Object | Descriptor / key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| proxy-ARP range | `arp.proxy-range` / `arp.proxy-range/<table>/<low>-<high>` | `proxy_arp_add_del` (is_add, table_id, low, hi); Update = `ErrRecreate` (everything is key) | `proxy_arp_dump` | `vrf/<table>` (not for table 0) | No tag in the API: attributed by table id inside the agent's `df2.IDRange` (lab: slot range `N000–N999`; production: nil = **every** proxy-ARP range on the VPP, intended for the single production agent; `proxy_arp_dump` reports the table id, not the fib index, so the range filter is exact). |
| proxy-ARP interface | `arp.proxy-interface` / `arp.proxy-interface/<ifname>` | `proxy_arp_intfc_enable_disable`; Update → `ErrRecreate` | `proxy_arp_intfc_dump`, filtered to owner-tagged interfaces | `interface/<ifname>` | Meta `{SwIfIndex}`. |

`vppctl show arp proxy` is the CLI equivalent.

## Ownership on untagged interfaces (review H3)
Objects on interfaces tagged `"<owner>:<name>"` are ours. Physical ports (DPDK NICs) carry no tag: an object created on
an untagged interface is recorded by its **key** in the claim store (`df2.WithClaims(store)`, DF-4's `acl.ClaimStore`
interface; `df2.FileClaimStore` persists it in the agent state dir) and Retrieve reports it only while claimed. Interfaces
tagged by another owner — and `local0` — are refused with `df2.ErrForeignInterface`. Dependencies use the interface
alias key `interface/<name>` (D-065, DF-1).

## Deletes re-verify identity (D-071, fix round 2)
A Delete that acts on a stored `sw_if_index` first re-dumps the interfaces (`df2.SkipDelete`): the index must still name the
object's interface and that interface must still be ours (own tag, or untagged and the key claimed). Otherwise nothing is
sent to VPP — our object went with the interface, or the index now belongs to someone else — and only the claim is dropped.
