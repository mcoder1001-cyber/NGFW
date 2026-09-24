# Roadmap — 0 to 100

Assumes a team of 6–9. A 2–3 person team can reach P5 but not GA parity in a year.

| Phase | Weeks | Output | Acceptance test |
|---|---|---|---|
| **P0 Foundation** | 1–8 | monorepo, CI, `.deb`, installer, VPP boots with generated `startup.conf`, agent↔API gRPC, auth/RBAC, audit, UI shell | fresh install on bare metal → login → see NIC inventory; reboot → still healthy |
| **P1 Interfaces/L2** | 9–16 | DPDK binding, IP/MTU/MAC, VLAN/QinQ, LACP, bridge domains, live counters | ping across a VLAN sub-if at line rate; counters match `vppctl show int` |
| **P2 L3/static** | 17–22 | VRFs, static routes, ECMP, ARP/ND, FIB viewer, ping/traceroute | 3-router static topology forwards; VPP restart → config restored automatically |
| **P3 Dynamic routing** | 23–34 | FRR + linux-cp: BGP/OSPF/IS-IS/RIP/BFD, route-maps, prefix-lists | full BGP table (≈1M routes) converges < 90 s; BFD failover < 300 ms |
| **P4 NAT/ACL** | 35–44 | NAT44/1:1/port-forward/CGNAT/MAP-T, object model, ACLs to 100k rules | 1M NAT sessions sustained; 100k-rule ACL with no measurable pps loss |
| **P5 VPN** | 45–58 | IPsec S2S (strongSwan+VPP), PKI, WireGuard, GRE/VXLAN/IPIP | 20 Gbps AES-GCM tunnel; interop with FortiGate/Cisco/strongSwan peers |
| **P6 Services** | 59–66 | Kea DHCP, Unbound, chrony, LLDP, SNMP, IPFIX, syslog, Prometheus | DHCP leases visible in UI; IPFIX records land in a collector |
| **P7 Observability/ops** | 67–76 | dashboard, packet capture, session browser, top talkers, backup/restore, upgrade | upgrade with auto-rollback proven by injecting a failure |
| **P8 HA** | 77–86 | VRRP, config sync, state sync | pull the power on the master → < 1 s failover, sessions survive |
| **P9 Hardening/GA** | 87–100 | hardening, signing, licensing, offline updates, docs, pen test, soak | 72 h at line rate with zero leaks/crashes; pen test clean |

## MVP cut (if you need something sellable fast)

P0 + P1 + P2 + P4-NAT-basics + P5-IPsec-S2S ≈ **5–6 months**, 4 engineers. That is
already a credible "high-performance IPsec concentrator / edge router" product, and it
is a much better first market than "TNSR clone".

## Team

| Role | Count | Why |
|---|---|---|
| VPP/DPDK data-plane engineer (C, Go) | 2 | the hardest hiring problem — start recruiting now |
| Go control-plane engineer | 1–2 | agent, reconciler, renderers |
| Node.js/TypeScript backend | 2 | API, transactions, auth, telemetry |
| React/MUI frontend | 1–2 | ~60 screens |
| QA / network test automation | 1 | topology lab, TRex, Robot |
| DevOps/release | 1 | packaging, APT repo, signing, CI |

## Order-of-magnitude budget

6–9 engineers × 12–18 months, plus a lab (2–4 servers with 25/100G NICs, a TRex traffic
generator box, switches) ≈ mid-six-figures USD equivalent. The lab is not optional:
without a real traffic generator you cannot claim any performance number.

## What to build first, concretely (week 1)

1. `git init`, monorepo skeleton (see `06-repo-skeleton.md`), CI green on an empty build.
2. A Vagrant/QEMU box that boots Ubuntu 24.04, installs VPP from the FD.io APT repo, and
   brings up two `virtio` interfaces bound to VPP. Commit the automation, not the notes.
3. `vrx-agent` that connects via GoVPP, dumps interfaces, and serves one gRPC RPC.
4. `vrx-api` that calls it and returns `GET /api/v1/state/interfaces`.
5. React page that lists interfaces with live counters.

That vertical slice proves the entire architecture in week one. Everything after is volume.
