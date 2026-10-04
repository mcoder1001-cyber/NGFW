# Tunnel interfaces

**Where:** VPN → **Tunnels**: GRE, VXLAN, IPIP/6RD and VXLAN-GPE; enable **Show advanced kinds** for GTP-U, L2TPv3 and PPPoE sessions.
**REST:** generic configuration routes under `/api/v1/config/tunnels`; `GET /api/v1/state/tunnels` reports only this agent owner's live tunnel interfaces.
The live state joins configuration names to actual engine allocations and returns endpoints, separate IPv4/IPv6 interface FIBs, underlay FIB when readable, and counters as decimal strings. Null means unavailable, including 6RD parameters with no VPP getter and counters without the stats segment. Configuration columns and live columns are separate; candidate edits do not rename a live interface.
**CLI:** `ngfw set tunnels gre <name> …` / `ngfw show configuration tunnels`.

A tunnel is an interface. Its addresses, MTU, VRF and (L2 tunnels) bridge-domain membership are set on the tunnel
record itself; other features (routes, VRRP, LLDP …) reference it by its engine interface name.

## Common fields

| field | meaning |
|---|---|
| `instance` | **Required for GRE, VXLAN and ordinary IPIP.** Fixes the engine interface name: `gre<n>`, `vxlan_tunnel<n>`, `ipip<n>`. It must lie in the agent's VPP id range (on a shared lab host the slot's range, e.g. 7000–7999; `NGFW_VPP_ID_RANGE=all` on a box of its own). One of these tunnels without an instance, or outside the range, is refused with a pointer at `instance`. |
| `src` / `dst` | Outer addresses, same family, `src` configured on an interface in the underlay VRF |
| `underlayVrf` | FIB the encapsulated packets use (default `default`) |
| `vrf` | FIB the tunnel interface belongs to (overlay) |
| `enabled`, `mtu`, `ipv4[]`, `ipv6[]` | as on any interface |
| `bridgeDomain` | L2 tunnels (GRE `teb`/`erspan`, VXLAN `decap: l2`): numeric id of a bridge domain from Interfaces → Bridging |

## Per kind

- **GRE** — `type` `l3` (IP over GRE), `teb` (Ethernet over GRE), `erspan` (needs `sessionId` 0–1023). Point-to-point.
- **VXLAN** — `vni` 0–16777215, UDP ports (default 4789), unicast or multicast `dst` (multicast needs
  `mcastInterface`), `decap` `l2` (bridge the inner frame) or `ip4`/`ip6` (route it in the tunnel VRF).
- **VXLAN-GPE** — `vni`, source/destination, UDP ports (default 4790), `protocol` (`ip4`, `ip6`, `ethernet`, `nsh`). VPP allocates the interface name; configuration names remain stable across reads.
- **GTP-U** — `teid`, optional `tteid`, `pduExtension` and `qfi` (0–63; requires PDU extension), decapsulation including drop. Dump-first guards prevent unsafe add/delete calls; forwarding defaults to drop.
- **L2TPv3** — `src` / `dst` IPv6 endpoints, local/remote session IDs and cookies. Only default underlay VRF is supported because VPP does not report the encap table.
- **PPPoE** — VPP session record with `sessionId`, `clientIp` and `clientMac`; this is distinct from ISP PPPoE discovery/authentication on Interfaces.
- **IPIP** — `mode` `p2p` or `p2mp` (no `dst`; responder-only route-based IPsec), `dscp` (omit to copy the inner DSCP).

## Example: extend a LAN over VXLAN

```json
{
  "interfaces": { "loop7011": { "ipv4": ["198.51.100.2/24"] } },
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

- `instance` is mandatory for GRE, VXLAN and ordinary IPIP; VPP allocates advanced/6RD engine names.
- IPIP `sixrd` contains `ip6Prefix`, `ip4Prefix`, optional `securityCheck` and `tcTos`; it has no fixed destination or instance. VPP cannot read back its domain parameters; the live UI explicitly marks them unavailable.
- VPP cannot delete L2TPv3 tunnels. Removal/recreation fails with the existing no-delete boundary; only cookie changes update in place. Never use this as a reversible lab object.
- Dedicated host mutation tests remain opt-in: `NGFW_DF6_GTPU_HOST=1` for GTP-U and `NGFW_DF6_L2TP_CREATE=1` for irreversible L2TPv3 creation. No shared-host mutations were performed for this completion.
- Changing a tunnel's endpoints, VNI or type re-creates the tunnel interface (brief outage).
- Packet transfer, persistence on a real VPP restart and screenshots on a provisioned appliance remain deferred laboratory acceptance.
