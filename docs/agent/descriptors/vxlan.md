# vxlan descriptors (DF-6, WBS D6.6)

Package `apps/agent/internal/descriptors/vxlan`. Messages only from `apps/agent/binapi/vxlan`. Shared rules: [df6.md](df6.md).

| Object type | Descriptor / key | Create / Delete | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| VXLAN tunnel | `vxlan.tunnel` · `vxlan.tunnel/vxlan_tunnel<instance>` | `vxlan_add_del_tunnel_v3` + owner tag | `vxlan_tunnel_v2_dump` + tag | `ErrRecreate` | `vrf/<encap_vrf_id>`, `interface/<mcast_interface>` |
| VXLAN bypass (ip4/ip6) | `vxlan.bypass` · `vxlan.bypass/<interface>` | `sw_interface_set_vxlan_bypass` per family | **write-only** (no dump) | toggles changed families | `interface/<interface>` |

Model `vxlan.Tunnel`: `instance`, `src`, `dst` (unicast or multicast group), `mcast_interface` (mandatory iff dst is
multicast), `vni`, `encap_vrf_id`, `src_port`/`dst_port` (0 = 4789, canonicalised back to 0), `is_l3`.

Notes: `vxlan_offload_rx` (hardware) is out of scope; VXLAN interfaces are bridged by DF-1.
