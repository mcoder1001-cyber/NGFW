# Tunnels (GRE, VXLAN, IPIP)

**Where:** VPN → **Tunnels** (tabs **GRE**, **VXLAN**, **IPIP**). **REST:** the generic configuration routes under
`/api/v1/config/tunnels` (`tunnels.gre`, `tunnels.vxlan`, `tunnels.ipip`); live state of the tunnel interfaces in
`GET /api/v1/state/interfaces` (a tunnel is an interface named by its instance).
**CLI:** `ngfw set tunnels gre <name> …` / `ngfw show configuration tunnels`.

A tunnel is an interface. Its addresses, MTU, VRF and (L2 tunnels) bridge-domain membership are set on the tunnel
record itself; other features (routes, VRRP, LLDP …) reference it by its engine interface name.

## Common fields

| field | meaning |
|---|---|
| `instance` | **Required in this release.** Fixes the engine interface name: `gre<n>`, `vxlan_tunnel<n>`, `ipip<n>`. It must lie in the agent's VPP id range (on a shared lab host the slot's range, e.g. 7000–7999; `NGFW_VPP_ID_RANGE=all` on a box of its own). A tunnel without an instance, or outside the range, is refused with a pointer at `instance`. |
| `src` / `dst` | Outer addresses, same family, `src` configured on an interface in the underlay VRF |
| `underlayVrf` | FIB the encapsulated packets use (default `default`) |
| `vrf` | FIB the tunnel interface belongs to (overlay) |
| `enabled`, `mtu`, `ipv4[]`, `ipv6[]` | as on any interface |
| `bridgeDomain` | L2 tunnels (GRE `teb`/`erspan`, VXLAN `decap: l2`): numeric id of a bridge domain from Interfaces → Bridging |

## Per kind

- **GRE** — `type` `l3` (IP over GRE), `teb` (Ethernet over GRE), `erspan` (needs `sessionId` 0–1023). Point-to-point.
- **VXLAN** — `vni` 0–16777215, UDP ports (default 4789), unicast or multicast `dst` (multicast needs
  `mcastInterface`), `decap` `l2` (bridge the inner frame) or `ip4`/`ip6` (route it in the tunnel VRF).
- **IPIP** — `mode` `p2p` or `p2mp` (no `dst`; responder-only route-based IPsec), `dscp` (omit to copy the inner DSCP).

## Example: extend a LAN over VXLAN

```json
{
  "routing": { "l2": { "bridgeDomains": { "lan": { "id": 7010 } } } },
  "tunnels": { "vxlan": { "to-site-b": {
    "instance": 7010, "src": "198.51.100.2", "dst": "203.0.113.20", "vni": 7010, "bridgeDomain": 7010
  } } }
}
```

Add the LAN port to the same bridge domain (Interfaces → Bridging) and commit.

## GRE over IPsec

Protect a GRE or IPIP tunnel with IPsec on the IPsec page (P11, route-based: `routeBased.ipipInterface` names an IPIP
tunnel). The tunnel itself is configured here.

## Limits in this release

- `instance` is mandatory (see above).
- VXLAN-GPE, GTP-U, L2TPv3, PPPoE and IPIP 6RD have engine support but no configuration keys yet.
- Changing a tunnel's endpoints, VNI or type re-creates the tunnel interface (brief outage).
- A dedicated `GET /api/v1/state/tunnels` with per-tunnel counters is not available yet; use the Interfaces page.
