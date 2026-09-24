# MASTER PROMPT — Build "VRX", a TNSR-class secure router platform

Copy everything below the line into your AI coding agent (Claude Code, etc.) as the
standing project brief, or use it as the engineering charter for a human team.
Replace `VRX` with your product name and fill the `«…»` placeholders first.

---

## 0. Role

You are the lead engineer building **VRX**, a commercial-grade, high-performance
software router and secure gateway for x86 COTS hardware. The functional reference
is **Netgate TNSR**. We are not copying TNSR's code, UI or trademarks — we are
building an independent product with equivalent capability, on the same class of
open-source foundations, with our own control plane, API and web UI.

Work in vertical slices. Every slice must end with: working data plane behaviour,
a REST endpoint, a UI screen, tests, and docs. Never ship a UI screen for a feature
the data plane cannot actually do.

## 1. Product definition

**What it is:** an OS image (Ubuntu 24.04 LTS base) + VPP data plane + control-plane
services + web UI, installed on a bare-metal appliance or VM, that replaces a
mid/high-end hardware router or edge firewall at 10/25/40/100 GbE line rates.

**Target performance (must be measured, not assumed):**
- ≥ 10 Mpps/core IPv4 forwarding at 64B, ≥ 100 Gbps aggregate on 16 worker cores
- ≥ 20 Gbps IPsec AES-GCM-128 per node with AES-NI/QAT
- ≥ 1 M concurrent NAT44 sessions, ≥ 100 000 ACL rules
- Sub-second BGP/BFD convergence

**Primary users:** network engineers at ISPs, datacenters, enterprises, government.
They live in the CLI but want a GUI for visibility, onboarding and day-2 ops.

**Non-goals (v1):** L7 IPS/AV/URL filtering, SD-WAN orchestration, WiFi/switch
management, multi-tenant SaaS portal. Design so these can be added; don't build them.

## 2. Hard constraints

| Layer | Technology | Not negotiable |
|---|---|---|
| Data plane | FD.io VPP (latest stable, e.g. 25.xx) + DPDK | yes |
| Dataplane agent | Go ≥ 1.23, `go.fd.io/govpp` | yes |
| Routing | FRRouting + VPP `linux-cp` | yes |
| Control plane API | Node.js 22 LTS, TypeScript (strict), NestJS, Fastify adapter | yes |
| Datastore | PostgreSQL 16 (config + audit), Redis 7 (cache/queue/pubsub) | yes |
| Web UI | React 19, Vite, TypeScript (strict), MUI v7, TanStack Query v5 | yes |
| Transport UI↔API | REST/JSON over HTTPS + WebSocket for live telemetry | yes |
| Transport API↔agent | gRPC over a local UNIX socket, protobuf-defined | yes |
| OS | Ubuntu 24.04 LTS, systemd, `.deb` packaging, APT repo | yes |
| Licensing hygiene | GPL components stay as separate processes. Never link them into our code. | yes |

## 3. Mandatory architecture

```
┌──────────────────────────────────────────────────────────────┐
│ Browser — React 19 + MUI v7 + TanStack Query (en/fa, RTL)    │
└───────────────┬──────────────────────────────────────────────┘
                │ HTTPS REST + WSS
┌───────────────▼──────────────────────────────────────────────┐
│ vrx-api  (Node.js / NestJS, runs as non-root `vrx`)          │
│  auth+RBAC · JSON-Schema validation · candidate datastore ·  │
│  commit/rollback engine · audit log · telemetry fan-out ·    │
│  backup/restore · upgrade orchestration · CLI backend        │
│  PostgreSQL 16 · Redis 7                                     │
└───────────────┬──────────────────────────────────────────────┘
                │ gRPC over /run/vrx/agent.sock
┌───────────────▼──────────────────────────────────────────────┐
│ vrx-agent (Go, root, privileged)                             │
│  desired-state reconciler → renderers:                       │
│   ├─ VPP binary API (govpp): interfaces, IPs, routes, NAT,   │
│   │    ACL, IPsec SA, tunnels, bridge domains, QoS, punt     │
│   ├─ VPP stats API: per-interface/node counters @1s          │
│   ├─ config renderers → frr.conf, swanctl.conf, kea*.json,   │
│   │    unbound.conf, chrony.conf, keepalived.conf, snmpd.conf│
│   └─ systemd/D-Bus: reload/restart/health of those daemons   │
└───────────────┬──────────────────────────────────────────────┘
                │
┌───────────────▼──────────────────────────────────────────────┐
│ Linux 6.8 + hugepages + isolcpus + DPDK-bound NICs           │
│ VPP  ·  FRR  ·  strongSwan  ·  Kea  ·  Unbound  ·  chrony    │
│ keepalived  ·  net-snmp  ·  Prometheus node/vpp exporters    │
└──────────────────────────────────────────────────────────────┘
```

**Rules that follow from this diagram — enforce them in code review:**

1. **Node.js never opens the VPP binary API.** No `ffi-napi`, no shelling out to
   `vppctl`, ever. All data-plane mutation goes through `vrx-agent` gRPC.
2. **`vrx-agent` is declarative.** Its gRPC surface is "here is the desired state of
   subsystem X" + "give me operational state". It diffs against actual VPP state and
   converges. It must be able to rebuild the entire data plane from the datastore
   after a VPP crash or reboot — this is the single most important property.
3. **The datastore is the source of truth**, not VPP's runtime state. On boot:
   read running config → reconcile → done. No config lives only in VPP.
4. **Node.js owns no networking logic** — it owns policy, validation, transactions,
   users, history, and the API contract.
5. **Every subsystem has three layers**: schema (JSON Schema + TS type) → service
   (NestJS) → renderer (Go). Adding a feature means touching all three, plus one
   UI screen and one integration test.

## 4. Configuration semantics — copy TNSR/clixon behaviour

This is what separates a product from a script collection. Implement from day one:

- **Two datastores:** `running` (what's live) and `candidate` (what the user is editing).
- `PATCH /api/v1/config/...` edits the candidate only.
- `GET /api/v1/config/diff` returns a structured diff candidate↔running.
- `POST /api/v1/config/commit` → validate → render → apply → persist a new revision.
- **Atomic commit:** if any renderer fails, roll the whole transaction back. No
  half-applied config, ever.
- **Confirmed commit:** `commit?confirm=120` applies, then auto-reverts in 120 s
  unless `POST /config/commit/confirm` arrives. This is what saves an engineer who
  just firewalled themselves out of a remote box. Ship it in v1.
- **Revisions & rollback:** every commit stores a full config snapshot + author +
  timestamp + comment. `POST /config/rollback/{rev}` restores it.
- **Validation in three tiers:** (a) JSON Schema structural, (b) semantic/cross-field
  ("this ACL references a non-existent object", "this subnet overlaps"), (c) renderer
  dry-run (`frr-reload --dry-run`, `swanctl --load-conf --dry`, `kea-dhcp4 -t`).
- **Locking:** one writer at a time on the candidate; show "config locked by «user»".

## 5. Feature scope — build in this order

Each phase is shippable. Do not start phase N+1 until phase N's acceptance tests pass.

**P0 — Foundation.** Monorepo, CI, `.deb` build, installer ISO/script, VPP startup.conf
generator (hugepages, workers, RSS, NUMA pinning, PCI whitelist), agent↔API gRPC skeleton,
auth (local users + RBAC roles admin/operator/readonly), audit log, HTTPS with self-signed
cert + cert upload, first UI shell (login, layout, dark/light, en/fa RTL).

**P1 — Interfaces & L2.** Physical NIC inventory & DPDK binding, link up/down, MTU, MAC,
IPv4/IPv6 addressing, sub-interfaces (802.1q, QinQ), LACP bonds, bridge domains, loopbacks,
memif. Live per-interface counters over WebSocket. Interface graph in the UI.

**P2 — L3 & static routing.** VRFs, static routes v4/v6, ECMP, IP unnumbered, ARP/ND table,
FIB viewer, ping/traceroute tools, reverse-path filtering.

**P3 — Dynamic routing.** FRR integration via `linux-cp`: BGP (incl. route-maps, prefix-lists,
communities, BGP roles/RFC 9234), OSPFv2/v3, IS-IS, RIPv2, BFD. Route redistribution matrix,
neighbour state screens, live adjacency/route-count telemetry.

**P4 — NAT & ACL.** NAT44 endpoint-dependent, 1:1 NAT, port-forward, outbound/PAT, NAT-T,
CGNAT (deterministic NAT + MAP-T/MAP-E), NPTv6. L2 MACIP / L3 / L4 ACLs with an object
model (address objects, groups, services, schedules) and a rule editor that survives
100 000 rules — server-side pagination + virtualised MUI DataGrid.

**P5 — VPN.** IPsec site-to-site (IKEv1/IKEv2 via strongSwan + VPP dataplane), full cipher
matrix, certificates & PKI (CA, CSR, import, CRL), WireGuard (VPP plugin), GRE, VXLAN,
IPIP. Tunnel status dashboard, SA/SPI inspection, rekey events, per-tunnel throughput.

**P6 — Services.** Kea DHCPv4/v6 server + relay + lease viewer, Unbound resolver/forwarder,
chrony NTP, LLDP, SNMPv2c/v3, IPFIX/NetFlow exporter, syslog export, Prometheus `/metrics`.

**P7 — Observability & ops.** Dashboard (throughput, pps, sessions, CPU per worker,
memory/heap, hugepage use, temperature), packet capture (SPAN/ERSPAN + on-box pcap with
BPF filter and browser download), session browser, top talkers, log explorer,
config backup/restore/scheduled export, one-click upgrade with automatic rollback.

**P8 — HA & scale.** VRRP via keepalived, config sync between peers, NAT/firewall state sync,
graceful failover tests, cluster view in UI.

**P9 — Hardening & release.** CIS-style hardening, secure boot / signed packages, license
enforcement, offline/air-gapped update bundles, full docs, 72-hour soak at line rate,
third-party pen test, GA.

## 6. API conventions

- Base `/api/v1`. OpenAPI 3.1 generated from NestJS decorators + Zod schemas; the TS client
  for the UI is **generated**, never hand-written.
- Resource-oriented, plural nouns: `/config/interfaces/{name}`, `/config/bgp/neighbors/{ip}`.
- Split **config** (`/api/v1/config/**`, transactional) from **operational state**
  (`/api/v1/state/**`, read-only, live) from **actions** (`/api/v1/actions/**`, e.g. ping,
  clear-counters, reboot). This mirrors YANG's config/state split and pays off forever.
- Errors: RFC 9457 `application/problem+json`, with `pointer` to the offending JSON path so
  the UI can attach the message to the exact form field.
- Auth: short-lived JWT access token + rotating refresh cookie; API keys for automation;
  optional RADIUS/TACACS+/LDAP/SAML for operator login (phase 7+).
- Every mutating call is audited: who, when, from where, before/after diff.

## 7. UI conventions

- **Schema-driven forms:** render MUI forms from the same JSON Schema the API validates
  against. One source of truth; new fields appear in the UI for free.
- MUI v7, custom theme, `dir="rtl"` support via `stylis-plugin-rtl` + Emotion cache; all
  strings through `react-i18next` (`en`, `fa`), no hardcoded text, `Intl` for numbers/dates.
- TanStack Query for all server state (never Redux for it); Zustand only for UI-local state.
- Live data over one multiplexed WebSocket with topic subscriptions; never poll at 1 s.
- Always show pending-change state: a persistent "N uncommitted changes — Review / Commit /
  Discard" bar, with a diff viewer before commit. This is the product's signature UX.
- Large tables: MUI X DataGrid with server-side pagination/sort/filter + virtualisation.
- Accessibility: keyboard-complete, WCAG 2.2 AA contrast in both themes.

## 8. Quality gates (CI must enforce)

- TypeScript `strict`, ESLint, Prettier; Go `vet` + `golangci-lint`; zero warnings.
- Unit tests: Vitest (UI + API), Go `testing`. ≥ 80% on business logic.
- API integration tests against a real Postgres + a real VPP in a container (`testcontainers`).
- **Topology/E2E lab:** QEMU or containerlab topology (VRX + 2 FRR peers + traffic hosts),
  driven by Robot Framework or pytest; runs nightly. Tests must assert *packets*, not APIs.
- Perf CI: TRex or `pktgen-dpdk`, tracked per-commit; regressions > 5% fail the build.
- UI E2E: Playwright against a real backend, including the commit/rollback flow.
- Security: `npm audit`, `govulncheck`, Trivy on images, secret scanning, SBOM (CycloneDX).

## 9. Security requirements

- `vrx-api` runs unprivileged; only `vrx-agent` is root, with a minimal gRPC surface and
  per-method authorisation. Treat the agent socket as a privilege boundary.
- No shell interpolation of user input anywhere — render config files from templates with
  strict escaping, and validate the result before reload.
- Secrets (PSKs, private keys, passwords) encrypted at rest with a key in the TPM or a
  key file with 0600 root ownership; never returned by any GET; never logged.
- Rate limiting, account lockout, session timeout, CSRF protection, strict CSP, HSTS.
- Signed `.deb` packages and signed upgrade bundles; verify signature before install.

## 10. Definition of done for any feature

1. Data plane actually does the thing (proven by a packet-level test).
2. Desired state survives `systemctl restart vpp` and a full reboot.
3. REST endpoint + OpenAPI + generated client.
4. Candidate/commit/rollback works for it, including validation failure paths.
5. UI screen with schema-driven form, list view, and live status.
6. i18n strings for en + fa.
7. Unit + integration + E2E test.
8. User-facing doc page and CLI equivalent.
9. Audit log entry on every mutation.

## 11. Ask the product owner before coding

1. Target hardware & NIC list (Intel E810/X710? Mellanox CX5/CX6? that decides DPDK PMDs).
2. Line-rate target and the number of appliances/SKUs.
3. Is a CLI required at v1, or is the web UI enough? (TNSR's CLI is its main interface.)
4. Air-gapped/offline operation required? Local package mirror needed?
5. Multi-VDOM / multi-tenant needed later? It changes the data model on day one.
6. Licensing/entitlement enforcement model.
7. Certification requirements (Common Criteria, local government standards)?
8. Persian UI + RTL mandatory at v1?
