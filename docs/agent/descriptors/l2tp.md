# l2tp descriptors (DF-6, WBS D6.6)

Package `apps/agent/internal/descriptors/l2tp`. Messages only from `apps/agent/binapi/l2tp`. Shared rules: [df6.md](df6.md).

| Object type | Descriptor / key | Create / Delete | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| L2TPv3 tunnel | `l2tp.tunnel` · `l2tp.tunnel/<name>` | `l2tpv3_create_tunnel` + owner tag / **no delete** (`df6.ErrNoDelete`) | `sw_if_l2tpv3_tunnel_dump` + tag | cookies in place (`l2tpv3_set_tunnel_cookies`); else `ErrRecreate` | `vrf/<encap_vrf_id>` |
| L2TPv3 cookies ("l2tpv3-cookies") | folded into `l2tp.tunnel` Update | `l2tpv3_set_tunnel_cookies` | via the tunnel dump | in place | — |
| L2TPv3 interface enable | `l2tp.interface-enable` · `…/<interface>` | `l2tpv3_interface_enable_disable` | **write-only** | — | `interface/<interface>` |
| L2TPv3 lookup key (global, **globals owner only**) | `l2tp.lookup-key` · `l2tp.lookup-key/global` | `l2tpv3_set_lookup_key`; Delete = no-op | **write-only** (no getter) | set in place | — |

Model `l2tp.Tunnel`: `name`, `client_address`, `our_address` (IPv6), `local_session_id`, `remote_session_id`,
`local_cookie`, `remote_cookie`, `l2_sublayer_present`, `encap_vrf_id`.

Limitations: VPP 26.06 has **no L2TPv3 tunnel delete message** — Delete returns `df6.ErrNoDelete`; a created tunnel
lives until VPP restarts, so the host test creates one only with `VRX_DF6_L2TP_CREATE=1`. The lookup key is global and
has no getter (tests never change it). DF-6-questions Q2.
