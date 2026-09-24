# VRX Master Schedule
## Full TNSR feature parity + complete VPP 26.06 feature surface

Reference baseline: **VPP 26.06** (released 24 June 2026 — 28 new features including the
Marvell scalable mGig NIC driver, CNAT SNAT/DNAT policy, HTTP/3 CONNECT and UDP proxying,
IKEv2 crypto enhancements, the Trace Path plugin and IPv6 DAD) plus the development branch
**26.10-rc0**. TNSR scope taken from Netgate's published feature documentation.

> **Per-feature detail lives in [`wbs/VRX-WBS.xlsx`](../wbs/VRX-WBS.xlsx)** — 102 work items
> with person-day estimates, tier, squad, dependencies, start/end quarter, and live formulas.
> This document is the narrative; the workbook is the authoritative plan.
> A Persian version of this document is at [`08-master-schedule-fa.md`](08-master-schedule-fa.md).

---

## 1. Executive summary

| Metric | Value |
|---|---|
| Net engineering effort | **5,185 person-days** (≈ 247 person-months ≈ 20.6 person-years) |
| With 25% overhead (management, integration, rework) | **6,480 person-days** |
| Recommended scenario | **36 months, 14 delivery FTE** (15–16 headcount) |
| First sellable release (MVP) | **Month 10** |
| First commercial GA (edge router) | **Month 16** |
| Full TNSR parity | **Month 27** (see §7 — capacity-constrained; month 24 needs +2 FTE or a scope cut) |
| Complete VPP feature surface | **Month 30** |
| Final GA with hardening + certification | **Month 36** |
| Lab CapEx | **≈ USD 165k**, staged across 5 purchases |

**Two findings that should shape the decision:**

1. **"All of TNSR + all of VPP" is a larger scope than TNSR itself.** TNSR does not expose
   VPP's whole plugin surface. Effort splits as: **Tier 1** (needed to sell) 3,345 PD = 65% ·
   **Tier 2** (needed for TNSR parity) 1,427 PD = 27% · **Tier 3** (complete VPP coverage:
   SRv6-mobile, LISP, BIER, quicly, host stack) 413 PD = 8%.

2. **Recommendation: run Tier 1 only — 24 months.** A sellable product with every
   critical capability in 22 months beats 36 months for a product whose last 8% of scope has
   no router customer asking for it. Revenue from T1 funds T2 and T3.

---

## 2. Estimating basis

Unit: person-day (PD) of a mid-to-senior engineer. Effective working year = **190 PD**
(after holidays, leave, meetings, support duty).

Every estimate includes all nine "definition of done" conditions — working data plane proven
by a packet-level test, restart/reboot survival, REST endpoint + OpenAPI + generated client,
candidate/commit/rollback with failure paths, UI screen with schema-driven form and live
status, en+fa strings, unit/integration/E2E tests, user doc + CLI equivalent, audit entry.
**Pure coding is roughly 45% of each number.**

---

## 3. Scope by domain

| Domain | Net PD | Share | Contents |
|---|---|---|---|
| D0 Foundation & platform | 685 | 13% | monorepo, CI, VPP build, OS image, `.deb`/APT, A/B upgrade, `startup.conf` generator, agent core + reconciler, API core + commit engine, auth/RBAC/audit, UI shell, CLI, telemetry pipeline, test infra |
| D1 Interfaces & L2 | 300 | 6% | all VPP drivers (dpdk, rdma, af_xdp, af_packet, vmxnet3, virtio, vhost-user, tap, netmap, memif, pipe, Marvell mGig), MTU/MAC/queues, 802.1q/QinQ, LACP bonding, bridge domains/L2XC/L3XC, LLDP, GSO, nsim, SPAN/ERSPAN, BVI |
| D2 L3, MPLS, multicast | 375 | 7% | VRF + source-VRF-select, static/ECMP, ARP/ND/RA/DAD, RPF/ADL/Auto-SDL, FIB browser, classifier + IP session redirect + ACL-based forwarding, MPLS + SR-MPLS, IGMPv3/PIM/mfib/BIER |
| D3 Dynamic routing | 540 | 10% | linux-cp hardening, FRR framework, BGP (incl. RFC 9234, VPNv4/v6, graceful restart), OSPFv2/v3, IS-IS, RIP, BFD, redistribution matrix |
| D4 NAT & CGNAT | 365 | 7% | NAT44-ED/EI, NAT64/66, NPTv6, DET44, DS-Lite, MAP-E/MAP-T, LW4o6, 464XLAT, CNAT policy, 1M-session browser |
| D5 Firewall & ACL | 270 | 5% | object model, MACIP/L3/L4 stateful ACLs, host ACL + nftables, 100k-rule editor, state sync |
| D6 VPN & tunnels | 650 | 13% | IPsec core + crypto engines + QAT, strongSwan `kernel-vpp`, native IKEv2, PKI/HSM, WireGuard, GRE/IPIP/VXLAN/GPE/GTP-U/L2TPv3/PPPoE, SRv6 + service chaining + SRv6-mobile, LISP, remote-access VPN |
| D7 Services, QoS, host stack | 575 | 11% | Kea DHCP, relay/client, Unbound + VPP DNS cache, chrony, SNMP, IPFIX/sFlow, syslog, QoS/HQoS, load balancer, host stack (session layer, VCL, TLS, QUIC, HTTP/3, SRTP, HSI), packet generator |
| D8 Observability & ops | 425 | 8% | dashboard, capture/trace/Trace-Path, Prometheus, alarms, backup/restore, support bundle, upgrade UI, RESTCONF/NETCONF + YANG, Terraform/Ansible/SDK |
| D9 High availability | 210 | 4% | VRRPv3, config sync, session/SA state sync, failover automation, cluster UI |
| D10 Multi-tenancy & AAA | 125 | 2% | VDOM/tenant model, RADIUS/TACACS+/LDAP/SAML/OIDC + MFA |
| D11 Cloud & virtualization | 130 | 3% | AWS/Azure/GCP images + marketplace, KVM/VMware/Hyper-V/Proxmox, SR-IOV |
| D12 Hardening, QA, docs, release | 535 | 10% | CIS hardening, secure boot, licensing, pen test, ~400 pages of docs, regression/interop/perf CI, soak & chaos, certification |
| **Net total** | **5,185** | 100% | |
| **Loaded (×1.25)** | **6,480** | | |

---

## 4. Team

| Squad | People | Responsibility |
|---|---|---|
| S1 Dataplane & Agent | 3 | VPP plugins, govpp, reconciler, renderers, stats |
| S2 Routing & L3 | 2 | linux-cp, FRR, BGP/OSPF/IS-IS, MPLS, multicast, FIB |
| S3 Security & VPN | 2 | IPsec, strongSwan, PKI, WireGuard, NAT, ACL |
| S4 Control Plane | 2 | NestJS, commit engine, RBAC, telemetry, RESTCONF, SDKs |
| S5 Frontend | 2 | React/MUI, SchemaForm, dashboards, i18n/RTL |
| S6 Platform & Release | 1 | VPP build, packaging, images, APT, upgrade, CI |
| QA / Lab | 2 | Robot, TRex, interop, nightly, perf CI |
| Architect / TPM | 1 | architecture, review, plan (50% delivery) |
| Technical writer | 0.5 from M12 | user docs, CLI/API reference |
| **Headcount** | **15–16** | **≈ 14 delivery FTE** |

**Capacity math:** 14 FTE × 190 PD = 2,660 PD/year. 6,480 ÷ 2,660 = 2.44 years of pure
delivery, plus one ramp-up quarter at 50% capacity and three months of non-parallelizable
release gates → **36 months**.

**Hiring:** M0 architect + **one senior VPP/DPDK engineer** (the binding constraint — start
recruiting immediately) · M1 2×Go, 2×Node, 1×React, 1×DevOps · M3 1×React, 1×QA ·
M6 1×Go, 1×senior network engineer, 1×QA/perf · M12 technical writer.

---

## 5. Release train

| Release | Month | Scope added | Commercial status |
|---|---|---|---|
| **R0.1 α** | 6 | foundation, interfaces/L2, live telemetry | internal alpha |
| **R0.5 β (MVP)** | 10 | L3/static/VRF, basic NAT44, IPsec S2S, DHCP/DNS/NTP | **first customer PoC — limited sale** |
| **R1.0 GA-1** | 16 | BGP/OSPF/BFD, full ACL + NAT, VRRP, SNMP, backup/upgrade | **GA "edge router + IPsec concentrator"** |
| **R1.5 GA-2** | 24–27 | IS-IS/OSPFv3/RIP, CGNAT/MAP, WireGuard, PKI, QoS, IPFIX/sFlow, RESTCONF, HA state sync, AAA | **GA "carrier" — full TNSR parity** |
| **R2.0 GA-3** | 30 | MPLS/SR-MPLS, multicast/BIER, SRv6, LISP, host stack/QUIC/HTTP3, load balancer, multi-tenancy, cloud | **complete VPP 26.06 surface** |
| **R2.5 GA-Final** | 36 | hardening, secure boot, licensing, offline updates, pen test, 72h soak, full docs, certification | **final enterprise/government product** |

---

## 6. Critical path

```
reconciler core (sprint 5) ──► linux-cp hardening (Q3) ──► BGP (Q4–Q5) ──► GA-1 (Q6)
        │                              │
        └──► commit engine (Q2) ───────┘
                                       └──► HA state sync (Q8) ──► GA-2
IPsec core (Q3) ──► strongSwan+VPP (Q3–Q4) ──► PKI (Q7) ──► GA-2
MPLS/SRv6/host stack (Q9–Q10) ──► GA-3 ──► hardening & certification (Q11–Q12)
```

Three items that cannot be parallelized, where a slip moves the whole programme:
1. **Reconciler core** — everything sits on it.
2. **linux-cp ↔ FRR** — the classic swamp. **Prototype it in Q3, not Q5.**
3. **Certification requirements** — get the list before the end of Q2 or you will rework
   architecture in Q11.

---

## 7. Capacity finding: GA-2 lands at month 27, not 24

Balancing the 102 work items against 532 net PD of quarterly capacity (see the
`Quarter Plan` sheet) pushes eight Tier-2 items past Q8: remote-access VPN, AAA,
Terraform/Ansible/SDK, native IKEv2, RPF/ADL, syslog/log explorer, support bundle and the
virtualization images. Full TNSR parity therefore completes at **end of Q9 = month 27**.

Three ways to close the gap — pick one deliberately rather than discovering it in month 23:

| Option | Effect |
|---|---|
| Accept month 27 for GA-2 | zero cost, honest plan |
| Add 2 FTE in Q7–Q8 | GA-2 holds at month 24, ≈ USD 120k extra |
| Cut those 8 items from GA-2 scope | GA-2 holds at month 24, features land in R2.0 |

Total loaded load is 6,481 PD against 7,648 PD of capacity across 12 quarters (11 full
quarters plus one at half capacity) — **84.7% utilisation, a 15.3% contingency reserve**,
concentrated in Q9, Q11 and Q12. That reserve is the plan's risk buffer; do not spend it on
new scope before month 30. Quarterly utilisation runs 75% → 87% → 95% → 96% → 99% → 89% →
98% → 102% → 80% → 99% → 65% → 28%; **Q8 is the only overcommitted quarter**.

---

## 8. Lab CapEx

| Month | Items | Estimate |
|---|---|---|
| M0 | 2 lab servers (single socket, AES-NI, AVX2) + Intel XXV710 25G + 25G switch | ≈ $40k |
| M4 | **TRex traffic generator box** + E810 100G NICs — without this no performance claim is defensible | ≈ $35k |
| M10 | HA pair + second switch + ConnectX-6 for the rdma driver path | ≈ $30k |
| M18 | 100G switch + optics + QAT card for crypto offload | ≈ $35k |
| M24 | Interop gear (used Cisco/Juniper router, FortiGate) for the compatibility matrix | ≈ $25k |
| **Total** | | **≈ $165k** |

---

## 9. Scenarios

| Scenario | Scope | Delivery FTE | Duration | Risk |
|---|---|---|---|---|
| **A Aggressive** | full (T1+T2+T3) | 22 | 24 months (model floor) | High — the critical path does not parallelize, coordination overhead +15%, and hiring 22 VPP-capable engineers is not realistic in most markets |
| **B Recommended** | full | 14 | **36 months** (model: 34 + 2 months planning slack) | Balanced |
| **C Lean** | full | 10 | 46 months | Falls behind VPP releases and market timing |
| **D Smart** | **Tier 1 only** | 14 | **24 months** | **Lowest risk, best return** |
| **E TNSR parity** | T1 + T2 | 14 | 32 months | Balanced |

---

## 10. Gate acceptance criteria

| Gate | Month | Measurable criteria |
|---|---|---|
| **G1** | 3 | unattended install on bare metal < 15 min; login; NIC inventory; survives reboot |
| **G2** | 6 | ≥ 10 Mpps/core IPv4 forwarding at 64B; `kill -9 vpp` → full config rebuilt < 30 s; UI counters == `vppctl` |
| **G3** | 9 | IPsec tunnel up against strongSwan, FortiGate and Cisco peers; 3-router static topology forwards |
| **G4** | 12 | a real customer PoC passed; 100k NAT sessions sustained; commit/rollback with no outage |
| **G5** | 16 | full BGP table (~1M routes) converges < 90 s; BFD failover < 300 ms; 100k-rule ACL with < 2% pps loss; IPsec ≥ 20 Gbps AES-GCM-128; clean 72h soak |
| **G6** | 24–27 | signed-off 100% TNSR feature matrix; VRRP failover < 1 s with sessions preserved; 1M NAT sessions; ≥ 100 Gbps on 16 workers |
| **G7** | 30 | every VPP 26.06 plugin either exposed in API/UI or recorded in a justified out-of-scope list |
| **G8** | 36 | zero crashes and zero leaks over 72h at line rate; clean independent pen test; signed offline update bundle; complete SBOM + NOTICE; ~400 pages of documentation |
