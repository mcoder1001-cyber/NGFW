# Progress

Updated 2026-10-01 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 70.4% by hours (945.0/1342.5 h), 67.3% by tasks (105/156)**

| state | tasks |
|---|---|
| merged | 105 |
| review | 14 |
| running | 8 |
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
| S4 | 528.0 / 775.0 | 68.1% | 60/96 | 7 | 10 | 0 |
| S5 | 58 / 147.5 | 39.3% | 6/15 | 1 | 0 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- F-det44-map-dslite-cnat — Wave B (day 10-12): DET44 CGNAT, MAP-E/T, DS-Lite, LW4o6, 464XLAT, CNAT policies (running, unassigned)
- F-nat46 — Wave B (day 10-12): NAT46 — IPv4 clients to IPv6-only servers (stateless SIIT / stateful NAT46) (running, unassigned)
- F-system-identity — Wave B: system identity — hostname, timezone, login/MOTD banners, DNS client + System screen (review, cloud session modest-keller)
- F-dataplane-ui — Dataplane screen: VPP plugins, NIC queues/descriptors, workers/corelist, hugepages — startup.conf preview + gated apply (review, cloud session modest-keller)
- F-management-ui — Management screen: tabbed shell (local users, AAA, API TLS, remote syslog) + apply of management.tls (review, cloud session modest-keller)
- P11 — Wave B (day 10-12): strongSwan+VPP build (staging sysroot) + IPsec S2S + tunnel dashboards (running, Codex manager delegated worker)
- F-tunnels — Wave B (day 10-12): GRE, IPIP, VXLAN(-GPE), GTP-U, L2TPv3, PPPoE (review, cloud session modest-keller)
- F-ospf — Wave B (day 10-12): OSPFv2/v3 via FRR (review, cloud session modest-keller)
- F-isis-rip — Wave B (day 10-12): IS-IS, RIPv2/RIPng via FRR (review, cloud session modest-keller)
- F-capture-trace — Wave C (day 13-15): pcap capture, BPF trace filter, trace node, Trace Path, PG (review, cloud session modest-keller)
- F-vrrp-config-sync — Wave C (day 13-15): VRRPv3 (VPP plugin + keepalived path), config sync, cluster UI (review, cloud session modest-keller)
- P10 — Debian packaging + systemd + install (26.04, our VPP debs) (running, unassigned)
- WEB-4a — Pre-built routing screens (OSPF, IS-IS/RIP, BFD+redistribution) merged UNROUTED; features add routes + status (review, cloud session modest-keller)
- WEB-4b — Pre-built HA/VRRP + cluster screens merged UNROUTED (review, cloud session modest-keller)
- TD-17 — apply-startup product mode: installed paths, appliance approval gate (sha256 + dead-man), lab gate kept as a mode (review, cloud session modest-keller)
- TD-21 — Scheduler scale: executor.dependents and topo re-sort keys on every operation (O(n^2)); 4000 objects = 13.7 s — index dependents once per plan (review, cloud session modest-keller)
- TD-26 — Core VRF tolerant delete: a table VPP keeps locked (nat64 never releases its FIB locks) is left, recorded and warned, not a failed transaction (review, cloud session modest-keller)
- TD-27 — ifsanitize: clear inherited SPAN source state and LLDP entries on interface create (V19 family) (review, cloud session modest-keller)
- F-notifications — Notifications: email (SMTP), Telegram and webhook for alarms, commits and link/VPN state (running, Codex manager delegated worker)
- F-setup-wizard — First-boot setup wizard: language/time, admin password, WAN (DHCP/static/PPPoE), LAN + DHCP, safe defaults, one commit (running, Codex manager delegated worker)
- F-dashboard-prom-alarms-host — Dashboard/Prometheus/alarms on the lab VPP: real StatsSource + collector/listener wiring, rig acceptance (running, Codex manager delegated worker)
- F-multiwan-host — Multi-WAN on the lab: two WAN netns, failover time, balance split, per-member NAT (running, Codex manager delegated worker)

## Parked

- LAB-vpp-per-slot — parked_on: PENDING-vpp-host-hardening
- P12-fib-proof — parked_on: PENDING-vpp-host-hardening (via LAB-vpp-per-slot)
