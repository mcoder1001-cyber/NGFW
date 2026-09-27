# Progress

Updated 2026-09-27 from plan/tasks.yaml (estimated hours are the plan's, not actuals).

**Overall: 68.0% by hours (917.0/1348.5 h), 65.8% by tasks (102/155)**

| state | tasks |
|---|---|
| merged | 102 |
| review | 13 |
| running | 3 |
| ready | 15 |
| parked | 2 |
| failed | 0 |
| todo | 20 |

| stage | merged h / total h | % | tasks merged/total | running | ready | parked |
|---|---|---|---|---|---|---|
| S0 | 6 / 6 | 100.0% | 1/1 | 0 | 0 | 0 |
| S1 | 73 / 73 | 100.0% | 9/9 | 0 | 0 | 0 |
| S2 | 264 / 274 | 96.4% | 28/29 | 0 | 0 | 1 |
| S3 | 16 / 19 | 84.2% | 1/2 | 0 | 0 | 1 |
| S4 | 500.0 / 781.0 | 64.0% | 57/95 | 2 | 15 | 0 |
| S5 | 58 / 147.5 | 39.3% | 6/15 | 1 | 0 | 0 |
| S6 | 0 / 48 | 0.0% | 0/4 | 0 | 0 | 0 |

## Running / review

- F-system-identity — Wave B: system identity — hostname, timezone, login/MOTD banners, DNS client + System screen (review, cloud session modest-keller)
- F-dataplane-ui — Dataplane screen: VPP plugins, NIC queues/descriptors, workers/corelist, hugepages — startup.conf preview + gated apply (review, cloud session modest-keller)
- F-management-ui — Management screen: tabbed shell (local users, AAA, API TLS, remote syslog) + apply of management.tls (review, cloud session modest-keller)
- F-tunnels — Wave B (day 10-12): GRE, IPIP, VXLAN(-GPE), GTP-U, L2TPv3, PPPoE (review, cloud session modest-keller)
- F-ospf — Wave B (day 10-12): OSPFv2/v3 via FRR (review, cloud session modest-keller)
- F-isis-rip — Wave B (day 10-12): IS-IS, RIPv2/RIPng via FRR (review, cloud session modest-keller)
- F-igmp-mfib — Wave C (day 13-15): IGMPv3, static mfib, BIER (PIM via agent-side sync, V5 fallback) (running, cloud session charming-johnson)
- F-capture-trace — Wave C (day 13-15): pcap capture, BPF trace filter, trace node, Trace Path, PG (running, cloud session modest-keller)
- P10 — Debian packaging + systemd + install (26.04, our VPP debs) (running, unassigned)
- WEB-4a — Pre-built routing screens (OSPF, IS-IS/RIP, BFD+redistribution) merged UNROUTED; features add routes + status (review, cloud session modest-keller)
- WEB-4b — Pre-built HA/VRRP + cluster screens merged UNROUTED (review, cloud session modest-keller)
- TD-17 — apply-startup product mode: installed paths, appliance approval gate (sha256 + dead-man), lab gate kept as a mode (review, cloud session modest-keller)
- TD-21 — Scheduler scale: executor.dependents and topo re-sort keys on every operation (O(n^2)); 4000 objects = 13.7 s — index dependents once per plan (review, cloud session modest-keller)
- TD-26 — Core VRF tolerant delete: a table VPP keeps locked (nat64 never releases its FIB locks) is left, recorded and warned, not a failed transaction (review, cloud session modest-keller)
- TD-27 — ifsanitize: clear inherited SPAN source state and LLDP entries on interface create (V19 family) (review, cloud session modest-keller)
- F-aaa-login — AAA increment 2: external login integration + MFA enforcement + LDAP/OIDC/TACACS+/SAML (review, cloud session eloquent-wozniak)

## Parked

- LAB-vpp-per-slot — parked_on: PENDING-vpp-host-hardening
- P12-fib-proof — parked_on: PENDING-vpp-host-hardening (via LAB-vpp-per-slot)
