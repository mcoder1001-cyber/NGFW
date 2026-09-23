# ipip descriptors (DF-6, WBS D6.6)

Package `apps/agent/internal/descriptors/ipip`. Messages only from `apps/agent/binapi/ipip` (+ `tunnel_types`, `interface`).
Shared rules: [df6.md](df6.md).

| Object type | Descriptor / key | Create / Delete | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| IP-in-IP tunnel (p2p / p2mp) | `ipip.tunnel` · `ipip.tunnel/ipip<instance>` | `ipip_add_tunnel` / `ipip_del_tunnel` + owner tag | `ipip_tunnel_dump` + tag | `ErrRecreate` | `vrf/<table_id>` |
| 6RD tunnel | `ipip.sixrd` · `ipip.sixrd/<name>` | `ipip_6rd_add_tunnel` / `ipip_6rd_del_tunnel` + owner tag | **write-only** (`ErrRetrieveUnsupported`) | `ErrRecreate` | `vrf/<ip6_table_id>`, `vrf/<ip4_table_id>` |

Model `ipip.Tunnel`: `instance`, `src`, `dst`, `table_id`, `flags` (tunnel_encap_decap_flags), `mode`, `dscp`.
`ipip.Tunnel6Rd`: `name` (tag id), `ip6_prefix`, `ip4_prefix`, `ip4_src`, `security_check`, `ip6_table_id`, `ip4_table_id`, `tc_tos`.

Notes / limitations
- `ipip_tunnel_dump` lists 6RD tunnels without their 6RD prefixes, so `ipip.sixrd` cannot be read back (partial,
  DF-6-questions Q4). `ipip.tunnel` ignores 6RD records (tag ids never collide).
- P11 / DF-5 add tunnel protection on `ipip<instance>`.
