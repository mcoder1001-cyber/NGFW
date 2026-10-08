# PPPoE delegated IPv6 LANs

In **Interfaces**, open the logical PPP interface, set **IPv6** to **dhcpv6**,
and add entries under **Delegated IPv6 LANs**. Each entry names one enabled LAN
interface and a stable numeric **Subnet ID**. The Persian form uses
«شبکه‌های محلی IPv6 تفویض‌شده»، «رابط شبکه محلی» and «شناسه زیرشبکه».
Stage the change and commit through the normal configuration workflow.

```json
{
  "interfaces": {
    "wanraw": { "enabled": true, "mtu": 1500 },
    "pppwan": {
      "enabled": true,
      "pppoe": {
        "parent": "wanraw",
        "username": "isp-user",
        "passwordRef": "password/isp",
        "ipv6": "dhcpv6",
        "delegationTargets": [
          { "interface": "lan0", "subnetId": 1 },
          { "interface": "lan1", "subnetId": 2 }
        ]
      }
    },
    "lan0": { "enabled": true },
    "lan1": { "enabled": true }
  }
}
```

The enabled PPP client requires a separate explicit raw WAN parent. Do not put
static addresses, DHCP, Linux control-plane pairing or an L2 attachment on that raw
parent. The logical PPP interface is the routed interface used by policies.
Existing same-name PPP configurations need migration to this explicit topology.

With an ISP lease `2001:db8:1200::/56`, subnet IDs 1 and 2 assign
`2001:db8:1200:1::/64` and `2001:db8:1200:2::/64`; the router uses `::1` in each.
A /64 delegation supports only subnet ID 0. IDs must fit the actual lease, be
unique for that WAN, and be between 0 and 4294967295. Targets must exist, be enabled,
belong to the same VRF and have no static IPv6, explicit RA, L2 or unnumbered
configuration. A LAN cannot be assigned to two PPP WANs. Live delegated networks
must not overlap other static or delegated IPv6 networks in that VRF.

Addresses and advertisements are runtime state; they are not written into the
LAN's static configuration. Assignments appear only for a current verified carrier
and current DHCP lease admission generation. Renewal replaces the old assignments;
disable, removal, rollback, carrier loss and lease expiry withdraw them through
normal reconciliation. No LAN is selected automatically.

RA valid/preferred lifetimes never exceed the DHCP lease. Budgets are rounded down
to 30 seconds, and assignments are conservatively withdrawn up to 29 seconds before
preferred expiry. If VPP refuses a change, the error is reported and retried; a
failed withdrawal must not be reported as completed.

Carrier readiness and delegated LAN registration are integrated in the source.
Combined source review and final aggregate CI remain required. Native DHCPv6 renew/rebind, LAN RA/SLAAC, return traffic and
carrier-loss acceptance remain to be tested on the laboratory topology.
