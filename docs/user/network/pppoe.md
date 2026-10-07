# PPPoE client (ISP dial-up)

Some ISPs deliver service over **PPPoE**: you authenticate with a username and password and the ISP assigns your
WAN address. Configure it per interface under **Interfaces → (the WAN interface) → PPPoE client**.

Current support is limited to client configuration, hook state and address/route mirroring.
Product dial-up is blocked when VPP's PPPoE plugin owns discovery EtherType 0x8863;
the current client path also lacks VPP PPPoE encapsulation for forwarded IPv4 and IPv6.
A session status of `up` or a mirrored default route does not establish working LAN transit.
These are unresolved product limitations, not deferred laboratory checks.

## How it works
The box runs `pppd` (the `rp-pppoe` kernel plugin) on a Linux tap of the WAN interface, does PPPoE discovery and
PPP authentication (PAP/CHAP), and the agent mirrors the ISP-assigned IPv4/IPv6 address and the default route into
the data plane. VPP's own `pppoe` plugin is the *server/AC* side (terminating other people's sessions) and is not
used here.

## Settings (`interfaces.<name>.pppoe`)
| Field | Meaning |
|---|---|
| Enabled | Dial the session. Disable to keep the settings but stay offline. |
| Dial over interface | The interface the session runs on; leave empty to use this interface. Set it to dial over a **VLAN sub-interface** (some ISPs put PPPoE on a tagged VLAN). |
| Username | The PPP username from the ISP. |
| Password | A **secret reference** (`password/<name>`) — store the password in **System → Secrets** first; it is never kept in the configuration and only an admin may set or change it. |
| Service name | The RFC 2516 Service-Name to request; leave empty to accept any access concentrator. |
| MTU | PPPoE payload MTU; 1492 is the Ethernet default (1500 − 8 for the PPPoE and PPP headers). Must fit the parent link. |
| Clamp TCP MSS | Rewrite the MSS of forwarded TCP SYNs to fit the MTU (avoids large packets being black-holed). On by default. |
| Default route from peer | Install a default route via the session (the ISP is your gateway). |
| Use peer DNS | Use the DNS servers the ISP sends as the system resolvers. |
| IPv6 | `off`; `slaac` — negotiate IPv6 on the link and take the address and IPv6 default router from the ISP's Router Advertisements; `dhcpv6` — `slaac` plus a DHCPv6 address (IA_NA) and a delegated prefix (IA_PD). Needs MTU ≥ 1280. |
| Reconnect → Hold-off | Seconds to wait before redialling after the session drops. |
| Reconnect → Max failures | Give up after this many failed dials in a row; 0 = keep trying forever. |

A PPPoE interface must **not** carry static IPv4/IPv6 addresses — the peer assigns them, so the commit is refused if
you set both.

## IPv6
With IPv6 on, pppd negotiates IPv6CP (link-local addresses) next to IPCP. The kernel accepts the ISP's Router
Advertisements on the PPP link (SLAAC) — the global address and, when *Default route from peer* is on, the IPv6
default router (normally the ISP's link-local). With `dhcpv6` the box also runs `dhcpcd` (from the base OS
`dhcpcd-base` package) on the PPP link to request an address (IA_NA) and a delegated prefix (IA_PD). The agent
mirrors every global IPv6 address into the data plane as a /128 on the WAN interface (as IPv4 is mirrored as a /32)
and the `::/0` path via the router into the interface's IPv6 table, and withdraws them when IPv6 or the session goes
down. The delegated prefix is reported but not assigned to a LAN interface (there is no setting for that yet). The
MSS clamp covers IPv6 too (MTU − 60).

## Firewall, NAT and blocking
NAT44 outbound, ACL attachments and Global blocking can reference the WAN interface by name.
Their configuration does not supply the missing PPPoE encapsulation; forwarded traffic through
this client remains unsupported for both IP families.

## Live status and reconnect
Open the interface in **Interfaces** — the drawer shows a **PPPoE session** panel: the phase (up / dialing / down /
failed), the assigned local and peer addresses, IPv6 (addresses and, with `dhcpv6`, `delegated <prefix>`), the peer DNS, how long the session has been up, and, on a
failure, the consecutive-failure count and the last error. **Reconnect** redials immediately, ignoring the hold-off.

## Troubleshooting
- **Stuck dialing / auth failed** — check the username and that the `password/<name>` secret holds the right
  password; the panel shows the last error.
- **Large downloads stall** — leave *Clamp TCP MSS* on; a wrong MTU black-holes big packets.
- **No default route** — turn on *Default route from peer* (unless another interface provides the default route).

## Client configuration wiring

Enabled `interfaces.<name>.pppoe` is reconciled from the committed configuration through the `pppoe.client.config` daemon descriptor. Its parent must have a linux-cp tap; `parent` defaults to the configured interface. Username, service, MTU, route, DNS, IPv6 negotiation and reconnect policy are rendered from the schema. The existing encrypted, revision-pinned secret channel delivers only an enabled client's `password/<name>` reference. References remain in desired state and the private applied manifest; resolved passwords appear only in mode-0600 PAP/CHAP files. Missing material produces a passwordRef finding and prevents dialing.

The globals-owner agent supervises the per-tap pppd units. Other agents render under `/run/ngfw-test/<owner>/pppoe/` and report `pppoe.not-supervised`; they never call systemctl. Hook polling converges negotiated IPv4 and IPv6 addresses, single multipath-safe IPv4/IPv6 default-route paths and MSS clamp under the agent transaction lock. Down and configuration removal withdraw those effects before deleting dependencies. Credential edits invalidate previous hook state and restart the dialer even when peer options are unchanged; changing only MSS clamp keeps the negotiated hook state.

The interface-state endpoint includes phase, negotiated addresses, peer DNS and observed consecutive unit failures. Successful up hooks reset failure state. Reconnect uses the existing `POST /api/v1/actions/interfaces/{name}/pppoe/reconnect` action. Equivalent operator inspection is `systemctl show ngfw-pppoe-<tap>.service -p ExecMainStatus -p NRestarts` plus `vppctl show interface address`, `show ip fib` and `mss_clamp_get`. Configure the client with the normal candidate → commit workflow; do not edit generated files.

Actual live dialing needs the owner's PPP kernel/package provisioning and ISP or test AC,
and resolution of the discovery limitation above. Production supervision in an LCP network
namespace is rejected because the packaged pppd unit does not enter that namespace; slot
rendering and hook tests can use the slot tap without supervising host units. Laboratory
reruns of peer restart/holdoff, wrong-password exits and appliance restart remain owed.
NAT/transit acceptance requires implementation of the missing data path first.
No packages are installed by this development task.
