# Multi-WAN (failover and load balancing)

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
