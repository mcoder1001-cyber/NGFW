# gtpu descriptors (DF-6, WBS D6.6)

Package `apps/agent/internal/descriptors/gtpu`. Messages only from `apps/agent/binapi/gtpu`. Shared rules: [df6.md](df6.md).

| Object type | Descriptor / key | Create / Delete | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| GTP-U tunnel | `gtpu.tunnel` · `gtpu.tunnel/<name>` | `gtpu_add_del_tunnel_v2` + owner tag (**V8 guard**, below) | `gtpu_tunnel_v2_dump` (records with `is_forwarding=0`) + tag | tteid only: `gtpu_tunnel_update_tteid`; else `ErrRecreate` | `vrf/<encap_vrf_id>`, `interface/<mcast_interface>` |
| GTP-U tteid ("gtpu-tteid") | folded into `gtpu.tunnel` Update | `gtpu_tunnel_update_tteid` | via the tunnel's dump (`tteid`) | in place | — |
| GTP-U forwarding entry | `gtpu.forward` · `gtpu.forward/<name>` | `gtpu_add_del_forward` + owner tag | `gtpu_tunnel_v2_dump` (records with `is_forwarding=1`; VPP stores the destination in `src_address`) | `ErrRecreate` | `vrf/<encap_vrf_id>` |
| GTP-U bypass | `gtpu.bypass` · `gtpu.bypass/<interface>` | `sw_interface_set_gtpu_bypass` | **write-only** | toggles | `interface/<interface>` |

Model `gtpu.Tunnel`: `name`, `src`, `dst`, `mcast_interface`, `encap_vrf_id`, `decap_next` DROP/L2/IP4/IP6, `teid`,
`tteid` (0 = same as teid, canonical), `pdu_extension`, `qfi` (0–63, needs pdu_extension). `gtpu.Forward`: `name`,
`dst`, `forwarding_type` (mask 1 bad-header | 2 unknown-teid | 4 unknown-type), `encap_vrf_id`, `decap_next`.

**V8 — VPP 26.06 segfaults on any failed `gtpu_add_del_tunnel_v2`** (it reads the interface counters of
sw_if_index `~0`). The descriptor pre-checks with `gtpu_tunnel_v2_dump`: an add whose (dst, teid) key exists fails
with `gtpu.ErrTunnelExists` without being sent, a delete of a missing tunnel is a no-op, `src == dst` and
`decap_next > 3` are rejected before sending. The host test is opt-in (`VRX_DF6_GTPU_HOST=1`). See DF-6-questions Q1.
`gtpu_offload_rx` (hardware) is out of scope.

`gtpu.forward` is effectively one global per (forwarding type, address family) in VPP (review M2): only one
forwarding entry of a type can exist. It stays per-object (tagged interface) but the config must not declare two of
the same type/family; its host test is opt-in (`VRX_DF6_GTPU_HOST=1`, never run on the shared VPP).
