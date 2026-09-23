# urpf descriptors (DF-2, WBS D2.4)

Package `apps/agent/internal/descriptors/urpf`. Messages from `apps/agent/binapi/urpf` only.

| Object | Descriptor / key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| uRPF on an interface | `urpf.interface` / `urpf.interface/<ifname>/<ipv4\|ipv6>/<rx\|tx>` | `urpf_update_v2` (mode loose/strict, AF, is_input, sw_if_index, table_id); Update in place; Delete = mode OFF; mode OFF as desired state is rejected (omit the object) | `urpf_interface_dump` (mode, AF, direction, table id), filtered to owner-tagged interfaces, mode OFF omitted | `interface/<ifname>`, `vrf/<table>` (non-zero) | Meta `{SwIfIndex}`. CLI: `show interface features <if>` (`ip4-rx-urpf-*`). |
