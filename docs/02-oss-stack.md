# Open-source stack, roles and licensing

## Data plane and its control

| Component | Role | License | Notes |
|---|---|---|---|
| **FD.io VPP** | packet forwarding, NAT, ACL, IPsec, tunnels, QoS | Apache-2.0 | the core; track a stable release branch, don't chase master |
| **DPDK** | userspace NIC drivers | BSD-3 | PMD support decides your hardware matrix |
| **GoVPP** (`go.fd.io/govpp`) | Go client for binary + stats API, codegen | Apache-2.0 | regenerate bindings per VPP version |
| **VPP `linux-cp` / `linux-nl`** | mirror VPP interfaces into Linux, sync FIB | Apache-2.0 | the FRR bridge; see also out-of-tree `lcpng` |
| **Ligato vpp-agent** | reference control-plane models | Apache-2.0 | study, don't depend |

## Network services (separate processes — GPL-safe)

| Component | Role | License |
|---|---|---|
| **FRRouting** | BGP, OSPFv2/v3, IS-IS, RIP, BFD, PIM | GPL-2.0 |
| **strongSwan** (+ `kernel-vpp`, `socket-vpp`) | IKEv1/IKEv2, certificates | GPL-2.0 |
| **WireGuard** | modern VPN — native VPP plugin | Apache-2.0 (VPP plugin) |
| **ISC Kea** | DHCPv4/v6 server, relay, lease DB | MPL-2.0 |
| **Unbound** | validating DNS resolver/forwarder | BSD-3 |
| **chrony** | NTP client/server | GPL-2.0 |
| **keepalived** | VRRP for HA | GPL-2.0 |
| **net-snmp** | SNMP v2c/v3 agent | BSD-like |
| **nftables** | host-protection firewall for management plane | GPL-2.0 |
| **Prometheus + node_exporter** | metrics/history | Apache-2.0 |

## Control plane and UI

| Component | Role | License |
|---|---|---|
| Node.js 22, NestJS, Fastify | API server | MIT |
| PostgreSQL 16 | config revisions, users, audit | PostgreSQL licence |
| Redis 7 | cache, pub/sub, BullMQ jobs | RSALv2/SSPL — **check**; use **Valkey** (BSD-3) instead to stay clean |
| Prisma or Drizzle | ORM/migrations | Apache-2.0 |
| Zod + `zod-to-json-schema` | one schema for validation, OpenAPI and UI forms | MIT |
| React 19, Vite, TypeScript | UI | MIT |
| **MUI v7** + MUI X DataGrid | components | MIT (DataGrid Pro/Premium is **commercial** — budget for it or use the MIT DataGrid) |
| TanStack Query v5, react-hook-form, i18next, Recharts/ECharts, xterm.js | UI libs | MIT / Apache-2.0 |
| Playwright, Vitest, Robot Framework, TRex | testing | MIT / Apache-2.0 / BSD |

## Licensing rules — put these in CONTRIBUTING.md

1. **Never link GPL code into your binaries.** FRR, strongSwan, chrony, keepalived are
   invoked as separate processes and configured through files/CLI/JSON. That is
   "mere aggregation" and keeps your control plane proprietary if you want it to be.
2. VPP and DPDK are Apache-2.0/BSD — you may write proprietary VPP plugins and link them.
3. If you **modify** FRR or strongSwan, those modifications are GPL and must be offered
   to your customers. Keep patches in a separate public repo; prefer upstreaming.
4. Ship an accurate NOTICE/third-party-licences file and an SBOM (CycloneDX) per release.
5. Mellanox/NVIDIA PMDs need `rdma-core`; Intel QAT needs the QAT driver — check
   redistribution terms for any firmware blobs you bundle.
6. **Do not** use the names TNSR, Netgate, pfSense, FortiGate, or their icons/CSS anywhere.

## Hardware notes

- NICs: Intel E810 (100G), XXV710/X710 (10/25G), Mellanox ConnectX-5/6 are the safe
  choices; check the DPDK supported-NIC list before buying anything.
- CPU: AES-NI and AVX2/AVX-512 matter a lot for crypto and vector processing. Prefer
  single-socket to avoid NUMA pain in v1.
- 1 GB hugepages, `isolcpus` + `nohz_full` + `rcu_nocbs` for worker cores, IOMMU on,
  C-states/turbo pinned for deterministic latency. Generate all of this from the UI wizard.
