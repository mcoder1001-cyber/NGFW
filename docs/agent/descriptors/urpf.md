# urpf descriptors (DF-2, WBS D2.4)

Package `apps/agent/internal/descriptors/urpf`. Messages from `apps/agent/binapi/urpf` only.

| Object | Descriptor / key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| uRPF on an interface | `urpf.interface` / `urpf.interface/<ifname>/<ipv4\|ipv6>/<rx\|tx>` | `urpf_update_v2` (mode loose/strict, AF, is_input, sw_if_index, table_id); Update in place; Delete = mode OFF; mode OFF as desired state is rejected (omit the object) | `urpf_interface_dump` (mode, AF, direction, table id), filtered to owner-tagged interfaces, mode OFF omitted | `interface/<ifname>`, `vrf/<table>` (non-zero) | Meta `{SwIfIndex}`. CLI: `show interface features <if>` (`ip4-rx-urpf-*`). |

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
