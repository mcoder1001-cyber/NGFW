# Task: F-multiwan — Multi-WAN failover and load sharing with link health monitors   (prepend 00-CONTEXT.md)

## Goal
With two or more WAN links (Ethernet, PPPoE, LTE later), keep Internet access up when one fails and optionally share load.

## Inputs to read first
- `F-vrf-static-ecmp`, `F-rpf-adl-pbr` (ABF policy routing), `F-bfd-redistribution`, `F-nat44-ed-sessions`, alarms (F-dashboard-prom-alarms).

## Contract changes
`routing.wanGroups[] { name, members: [{ interface, gateway|dhcp|pppoe, weight, priority }], mode: failover|balance,
monitors: [{ type: icmp|http|dns, target, intervalMs, timeoutMs, lossPct, latencyMs, downAfter, upAfter }], stickySessions: true }`.

## Scope — build exactly this
1. **Monitors** (agent): probes sourced from each member link (per-link source address/VRF), hysteresis up/down, state published.
2. **Routing**: failover = default route via the best healthy member (priority); balance = weighted ECMP over healthy members.
   Policy rules (ABF) can pin traffic to a group or a member.
3. **NAT**: per-member source NAT; sessions keep their link while it is up (sticky); on failover stale NAT sessions of the dead
   link are cleared so clients reconnect quickly.
4. **Alarms**: link down/up events to alarms/syslog.
5. **UI**: WAN groups page with live member state (up/down, loss, latency).
6. **Tests**: unit tests for the hysteresis; topology test with two WAN netns: cut link 1 → traffic moves to link 2 within
   the configured time; restore → back (failover mode); balance mode spreads flows by weight.
7. **Docs**: `docs/user/network/multi-wan.md`.

## Acceptance (paste the evidence)
- [ ] Failover time measured and within downAfter × interval + 2 s (pasted)
- [ ] Balance: flow split matches weights ±10 % over 1000 flows (pasted)
- [ ] `tools/ci.sh --base main` green

## Out of scope
SD-WAN SLA-based app steering (BL-OPS-02), LTE modems (BL-NET-07).
