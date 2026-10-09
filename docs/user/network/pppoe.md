# PPPoE client (ISP dial-up)

Configure a **distinct logical PPP interface** in Interfaces and select its explicit
raw WAN parent. The parent can be a dedicated Ethernet interface or an explicit
VLAN sub-interface such as `eth0.100`. Store the ISP password in System → Secrets
and reference it as `password/<name>`.

The product runs the packaged `pppd` kernel PPPoE plugin in a private network
namespace. VPP connects the raw parent to that namespace at layer 2 for PPPoE
discovery and session frames. A separate plain-IP transit interface carries routed
traffic between VPP and the kernel PPP session. VPP remains responsible for firewall,
NAT and routing; the carrier does not use Linux NAT or disable VPP's PPPoE plugin.

This source implementation still requires final combined review and the deferred
laboratory packet/restart acceptance. Unit and mock controls are not live ISP or
LAN-transit evidence.

## Settings (`interfaces.<name>.pppoe`)

| Field | Meaning |
|---|---|
| Enabled | Dial the session; disable to retain settings while offline. |
| Dial over interface | Required dedicated parent, different from the logical PPP interface. Select an explicit VLAN child when the ISP requires tagging. |
| Username | ISP authentication name. |
| Password | Secret reference; plaintext is never stored in desired configuration or returned by config GET. |
| Service name | Requested Service-Name; empty accepts any access concentrator. |
| MTU | PPP payload MTU, normally 1492. Parent and VLAN root must accommodate this value plus eight bytes. IPv6 requires at least 1280. |
| Clamp TCP MSS | Clamp forwarded SYN MSS to the configured PPP MTU. |
| Default route from peer | Create the logical PPP default path after verified forwarding readiness. A Multi-WAN group owns this path while the PPP interface is a group member. |
| Use peer DNS | Request and report the ISP DNS servers. This does not automatically replace the system resolver configuration. |
| IPv6 | `off`, `slaac` for IPv6CP and ISP RA, or `dhcpv6` for address and prefix delegation. |
| Delegated IPv6 LANs | Explicit LAN targets and stable subnet IDs; see [delegated LAN configuration](../interfaces/pppoe-delegation.md). |
| Reconnect hold-off / max failures | Redial delay and failure limit; zero failures means keep retrying. |

The selected raw parent must be enabled and have no static IP, DHCP, Linux
control-plane pairing, unnumbered address, L2 attachment or bond membership.
A whole-port parent cannot also own VLAN children. For a VLAN parent, the selected
child is exclusive to PPP; unrelated children may remain on the physical root.
The physical root must not be paired to Linux, bridged, cross-connected or enslaved
to a bond. Live conflicting ownership is rejected before namespace creation.

The logical PPP interface must not carry static IP addresses. NAT44 outbound, ACLs,
Global Blocking and Multi-WAN membership reference its logical name, for example
`pppwan`, rather than the raw parent. IPv4 addresses are mirrored as /32 and global
IPv6 addresses as /128 on this logical interface. Routes use the private transit
peer; the ISP's point-to-point peer is not a VPP Ethernet next hop.

## Migration and runtime state

Legacy configurations that put PPP on its own raw parent, omit the parent, or use a
parent Linux control-plane tap must be explicitly migrated. Create a separate
logical interface, set its parent and move WAN policies to the logical name. The
agent rejects incompatible topology; it does not silently rewrite configuration.
Non-supervising development agents retain isolated renderer behavior and never
start product units.

The session panel reports phase, addresses, peer DNS, failure count and errors.
Reconnect uses `POST /api/v1/actions/interfaces/{name}/pppoe/reconnect`.
An `up` hook alone does not authorize forwarding: readiness also verifies the
current namespace, pppd process invocation, NCP generation, actual negotiated
addresses, VPP transport identity, MTU, links and VLAN classification. Readiness
expires and is withdrawn on replacement, disconnect or failed readback. Multi-WAN
probes run only through the verified owned namespace with bounded destinations and
lifetime. Delegated LAN addresses and RA are withdrawn on lease/carrier loss.

Private peer files live under `/var/lib/ngfw/agent/pppoe-carrier/<token>/ppp`;
passwords are confined to mode-0600 PAP/CHAP files. The fixed
`ngfw-pppoe-carrier@<token>.service` exposes that tree read-only, with only its own
`resolv.conf` output bound writable from `/run/ngfw/pppoe/<token>/resolv.conf`.
Use `systemctl show ngfw-pppoe-carrier@<token>.service -p ExecMainStatus -p NRestarts`
and VPP interface/FIB inspection for diagnosis. Do not edit generated files.

For authentication failures, check the reference and ISP credentials. For missing
routes, check carrier readiness and Multi-WAN monitor status before changing default
route settings. For large-packet failures, check both link MTUs and MSS clamping.
The fixed packaged pppd/plugin is trusted: kernel IP filtering prevents ordinary
namespace routing bypass, but is not a sandbox against a malicious daemon with raw
Ethernet privileges.

Laboratory acceptance still owes discovery with the product plugins loaded,
IPv4/IPv6 LAN traffic and NAT/ACL enforcement, VLAN tag symmetry, PMTU, PD renew/loss,
server restart, wrong-password behavior, VPP restart and appliance restart. No host
packages or services were changed while developing this source.
