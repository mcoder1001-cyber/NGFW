# vxlan_gpe descriptors (DF-6, WBS D6.6)

Package `apps/agent/internal/descriptors/vxlan_gpe`. Messages only from `apps/agent/binapi/vxlan_gpe`. Shared rules: [df6.md](df6.md).

| Object type | Descriptor / key | Create / Delete | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| VXLAN-GPE tunnel | `vxlan-gpe.tunnel` · `vxlan-gpe.tunnel/<name>` | `vxlan_gpe_add_del_tunnel_v2` + owner tag | `vxlan_gpe_tunnel_v2_dump` + tag | `ErrRecreate` | `vrf/<encap_vrf_id>`, `vrf/<decap_vrf_id>`, `interface/<mcast_interface>` |
| VXLAN-GPE bypass | `vxlan-gpe.bypass` · `vxlan-gpe.bypass/<interface>` | `sw_interface_set_vxlan_gpe_bypass` | **write-only** | toggles | `interface/<interface>` |

Model `vxlan_gpe.Tunnel`: `name` (tag id; VPP names the interface), `local`, `remote`, `local_port`/`remote_port`
(0 = 4790), `mcast_interface`, `vni`, `protocol` IP4/IP6/ETHERNET/NSH, `encap_vrf_id`, `decap_vrf_id`.
iOAM-over-GPE is out of scope.
