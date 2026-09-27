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

F-tunnels (product wiring)
- Wired by `internal/subsystems/tunnels.go` and projected from `tunnels.<kind>` by `internal/desired/tunnels.go`
  (tunnel + `tunnels.meta` + `interface/<vpp name>` alias + admin state, MTU, VRF, addresses, bridge membership).
- TD-11b: the descriptor declares `RecordsNoOwnership()` — ownership is the `<owner>:<id>` tag VPP carries on the
  interface; no claim or boot store is written.
- TD-11c: the package maps its VPP device class to its creator (`iface.RegisterKind` in `register.go`), so the
  interface's attributes are deleted before the tunnel.
- TD-8b: the projection accepts only instances inside the agent's VPP id range (none configured → every tunnel is
  refused with `tunnels.instance-range`).
