# Progress

Updated 2026-10-03 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 73.7% by hours (990.0/1342.5 h), 73.1% by tasks (114/156)**

| state | tasks |
|---|---|
| merged | 114 |
| review | 0 |
| running | 13 |
| ready | 10 |
| parked | 2 |
| failed | 0 |
| todo | 17 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 264 / 274 | 96.4% | 28/29 | 0 | 0 | 1 |
| S3 | 16 / 19 | 84.2% | 1/2 | 0 | 0 | 1 |
| S4 | 567.0 / 775.0 | 73.2% | 68/96 | 12 | 10 | 0 |
| S5 | 64 / 147.5 | 43.4% | 7/15 | 1 | 0 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- F-det44-map-dslite-cnat — Wave B (day 10-12): DET44 CGNAT, MAP-E/T, DS-Lite, LW4o6, 464XLAT, CNAT policies (running, awaiting resume)
- F-nat46 — Wave B (day 10-12): NAT46 — IPv4 clients to IPv6-only servers (stateless SIIT / stateful NAT46) (running, awaiting resume)
- F-dataplane-ui — Dataplane screen: VPP plugins, NIC queues/descriptors, workers/corelist, hugepages — startup.conf preview + gated apply (running, awaiting resume)
- F-management-ui — Management screen: tabbed shell (local users, AAA, API TLS, remote syslog) + apply of management.tls (running, /root/ngfw_manager/management_developer)
- P11 — Wave B (day 10-12): strongSwan+VPP build (staging sysroot) + IPsec S2S + tunnel dashboards (running, awaiting resume)
- F-tunnels — Wave B (day 10-12): GRE, IPIP, VXLAN(-GPE), GTP-U, L2TPv3, PPPoE (running, awaiting resume)
- F-ospf — Wave B (day 10-12): OSPFv2/v3 via FRR (running, awaiting resume)
- F-isis-rip — Wave B (day 10-12): IS-IS, RIPv2/RIPng via FRR (running, awaiting resume)
- F-vrrp-config-sync — Wave C (day 13-15): VRRPv3 (VPP plugin + keepalived path), config sync, cluster UI (running, awaiting resume)
- P10 — Debian packaging + systemd + install (26.04, our VPP debs) (running, /root/ngfw_manager/p10_developer)
- F-notifications — Notifications: email (SMTP), Telegram and webhook for alarms, commits and link/VPN state (running, awaiting resume)
- F-setup-wizard — First-boot setup wizard: language/time, admin password, WAN (DHCP/static/PPPoE), LAN + DHCP, safe defaults, one commit (running, awaiting resume)
- F-multiwan-host — Multi-WAN on the lab: two WAN netns, failover time, balance split, per-member NAT (running, awaiting resume)

## Parked

- LAB-vpp-per-slot — parked_on: PENDING-vpp-host-hardening
- P12-fib-proof — parked_on: PENDING-vpp-host-hardening (via LAB-vpp-per-slot)
