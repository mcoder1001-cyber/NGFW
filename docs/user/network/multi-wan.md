# Multi-WAN (failover and load balancing)

> Implementation status: the runtime implements health-driven failover/weighted routes, per-member IPv4 NAT and group PBR. Probes bind to Linux LCP interfaces in the default namespace and VRF; unsupported interfaces fail closed. DHCPv4 gateway observations feed forwarding without changing the stored configuration. PPPoE gateway handoff awaits a verified forwarding carrier and remains unavailable. Native dynamic-lease packet acceptance remains a laboratory check.

With two or more WAN links, **WAN groups** (`routing.wanGroups`) keep Internet access up when a link fails and,
optionally, share load across links. Configure them under **Config → Routing → WAN groups**; watch them live under
**Routing → Multi-WAN**.

## A group
```
routing.wanGroups:
  - name: internet
    mode: failover          # or balance
    stickySessions: true
    members:
      - { interface: TenGigabitEthernet0/0/0, nextHop: dhcp,    priority: 10, weight: 1 }
      - { interface: TenGigabitEthernet0/0/1, nextHop: gateway, gateway: 203.0.113.1, priority: 20, weight: 1 }
    monitors:
      - { type: icmp, target: 1.1.1.1, intervalMs: 1000, timeoutMs: 1000, lossPct: 50, latencyMs: 150, downAfter: 3, upAfter: 3 }
```

- **mode**: `failover` — the healthy member with the **lowest priority** carries the default route; `balance` —
  weighted ECMP over the healthy members (by `weight`).
- **members**: each names an interface and how it learns its next hop (`gateway` + a `gateway` address, `dhcp`, or
  `pppoe`). A member interface may appear once per group.
- **stickySessions**: an established flow stays on its member while the member is up; on failover the dead link's NAT
  sessions are cleared so clients reconnect quickly on the surviving link.
- **monitors**: health probes sourced from each member link. A check is unhealthy when probe loss reaches `lossPct`
  or average latency exceeds `latencyMs` (0 = no latency check). A member goes **down** after `downAfter` consecutive
  unhealthy checks and comes back **up** after `upAfter` consecutive healthy ones (hysteresis prevents flapping).

## Behaviour
- **Failover time** ≈ `downAfter × intervalMs` before the member is marked down, then the default route moves to the
  next-best healthy member.
- **Balance** spreads new flows across healthy members in proportion to their weights; a member leaving/rejoining
  re-weights the ECMP set.
- Policy rules (Routing → Policy routing, ABF) can pin specific traffic to a group or a member.
- Link up/down transitions can raise **alarms** (Config → Management → Alarms, metric `interface_link_down`).

## Watching it
**Routing → Multi-WAN** shows each group's members with their live health (up/down, loss, latency), and the active
member in failover mode. Commit a change and the group is applied like any other routing configuration.

## Notes
- A WAN member must not also carry a conflicting static default route; the group manages the default route for its
  members.
- LTE/5G modems and SD-WAN SLA-based application steering are out of scope (backlog).

### ICMP availability under the packaged service

ICMP monitoring uses Linux echo-only datagram sockets, never raw sockets. The existing host `net.ipv4.ping_group_range` policy must permit the agent service's group; some hosts deny all groups by default. The agent does not change that policy or acquire additional capabilities. The packaged capability boundary remains unchanged.

When the kernel denies ICMP datagram probes, live state reports the affected requested WAN group as unavailable under host policy, rather than presenting a local permission failure as 100% packet loss. Unaffected requested groups remain readable. Use HTTP or DNS monitoring where ICMP is unavailable; any host-policy change requires separate administrator authorization. An unavailable probe does not establish that the WAN link is down. Interface/network-namespace and appliance acceptance remains unverified.

The current monitor implementation uses the default namespace and VRF. ICMP probes support IPv4 only; HTTP and DNS use device-bound IPv4 or IPv6 connections. Missing LCP devices, unsupported address families, and local socket/device binding restrictions return unavailable; they are not interpreted as remote packet loss. No probe falls back to another interface or changes host privileges.

### Agent routing and translation

Only durably committed groups start probes. A healthy static-gateway member contributes a default route in its VRF; failover selects the lowest priority and balance uses member weights. Unknown, unobserved, and unavailable probes contribute no path. All members must share one VRF and gateway address family. A group cannot share its default FIB entry with a static default or another group. DHCPv4 members require an enabled DHCP client on the named interface and a current BOUND lease with a unicast router and address. The agent polls owned leases each second, withdraws the path on release/read failure, and retries on the next poll. Observations expire after five seconds without refresh and are invalidated by configuration changes. Renewals update routes, group PBR and the member NAT pool. PPPoE members remain unavailable until a verified forwarding-carrier snapshot is wired; a negotiated PPP peer alone never authorizes forwarding ordinary IP through the physical WAN.

For IPv4 translation, explicitly enable `nat.enabled: true` with `nat.mode: ed`. Groups with at least two members receive an output NAT feature and interface-address pool for each member. A member already mentioned by the explicit NAT configuration remains owned by that configuration. The agent does not implicitly enable a global NAT plugin. Dynamic routes and translation objects have separate durable ownership and are excluded from static-route/NAT configuration retrieval and drift.

A policy path `{wanGroup: internet}` follows healthy group paths. It is exclusive with an address, interface, or non-default VRF. When all group members are down, the policy performs a lookup in their VRF; the WAN default itself is absent. Direct member pinning remains `{address: 203.0.113.1, interface: WAN2}`. API/CLI equivalent: set the policy under `/api/v1/config/routing/pbr`, then commit the candidate.

With `stickySessions` enabled, healthy-to-down transitions clear only dynamic NAT sessions matching the dead member's configured or currently learned IPv4 addresses in the member VRF. Lease renumbering or withdrawal also queues the retired outside address for cleanup. Work is bounded and resumes across scans; static mappings and other outside addresses are preserved. VPP owns established translation state. Weighted ECMP rehash affinity has not been proven on the appliance; use failover mode when preserving a surviving member's sessions during membership changes is required. Weighted traffic distribution, actual affinity, and restart/rollback packet acceptance remain deferred laboratory checks, not demonstrated guarantees.
