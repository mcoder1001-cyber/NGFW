# gre descriptors (DF-6, WBS D6.6)

Package `apps/agent/internal/descriptors/gre`. Messages only from `apps/agent/binapi/gre` (+ `tunnel_types`, `interface`).
Shared DF-6 rules (ownership, idempotent delete, typed errors): [df6.md](df6.md).

| Object type | Descriptor / key | Create / Delete | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| GRE tunnel (L3, TEB, ERSPAN; p2p / p2mp) | `gre.tunnel` · `gre.tunnel/gre<instance>` | `gre_tunnel_add_del_v2` (is_add 1 / 0) + `sw_interface_tag_add_del` | `gre_tunnel_v2_dump` + `sw_interface_dump` (owner tag) | `ErrRecreate` | `vrf/<outer_table_id>` (none for 0) |

Model `gre.Tunnel`: `instance` (the id; VPP names the interface `gre<instance>`), `type` L3/TEB/ERSPAN, `mode` P2P/MP,
`src`, `dst` (empty for multipoint), `outer_table_id`, `session_id` (ERSPAN), `flags`, `key`. Meta `df6.IfMeta{SwIfIndex}`.

Notes / limitations
- TEB tunnels are bridged by DF-1's `bridge-domain-member`, ERSPAN sessions by DF-7 (SPAN) — not here.
- Delete of a tunnel VPP no longer has (interface gone / not our tag) is a no-op and sends nothing.
