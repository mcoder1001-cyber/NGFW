# Wave B/C (+S5) number allocation — continues `docs/status/wave-A-hotspots.md` §2

Prep for the envelopes after wave A (branch `task/prep-rest`, 2026-09-24). Base: P08's `packages/proto/vrx/v1/dataplane.proto` maxima plus
every number wave-A §2 allocated **or proposed** (ServicesConfig 8 `auto_sdl`, 9 `nsim`; ActionRequest 4–8; EventKind 10–15; Interface 13–22;
StaticRoute 7–8; NextHop 4; AclConfig 7–8; WireguardInterface 12; SyslogTarget 6–9; DesiredState 14 only via PENDING).

Rules (same as §2): a number appears **once** in this file; never reuse a §2 number; never take "next free"; the manager confirms a section
at spawn and copies it into the envelope. One section per task (`##`, or `###` under a group heading) — add your own, never edit another task's. RPC names and new
messages start with the feature's noun (§0 rule 5). New messages go in the task's `// ----- <task-id> -----` section at the end of the proto.
Shared-file anchors for these tasks read `// wave-BC: <task-id>` (`#`, `{/* */}` where the language needs it); W-seed seeds wave A only, so
the manager seeds these in a wave-B/C anchor pass before spawning (placement rule: "Anchor placement" at the end of this file).

## Wave C services/observability + F-licensing (F-host-stack, F-lb, F-qos-flat, F-snmp, F-ipfix-sflow, F-dashboard-prom-alarms, F-capture-trace, F-licensing)

Maxima checked on P08's proto (task/P08@e1587c9): ServicesConfig 7 · ManagementConfig 4 · SnmpService 11 (Community 3, V3User 6,
TrapReceiver 6) · IpfixService 3 (Exporter 8, Flowprobe 6, Sflow 8) · QosService 4 (QosPolicer 12, QosInterface 6) · CaptureAction 6
(CaptureDirection 3) · StatsBatch 5 · StreamStatsRequest 3 · ActionRequest 3 (+ wave-A 4–8) · EventKind 9 (+ wave-A 10–15).
None of these eight tasks needs an `ActionRequest` member or an `EventKind`: their actions (lb flush, policer reset, capture list/delete, PG)
are unary RPCs behind static API routes; the capture itself uses the existing member 3.

### F-host-stack
| message | current max | allocation |
|---|---|---|
| `ServicesConfig` | 7 (8, 9 taken by wave-A §2) | **10 `host_stack`** → `HostStackService` |
| new messages (`// ----- F-host-stack -----`) | — | `HostStackService{enabled, namespaces map, session_rules, tcp_source_addresses, http_static}`, `HostStackNamespace`, `HostStackSessionRule`, `HostStackTcpSource`, `HostStackHttpStatic` (field numbers from 1) |
| RPCs | — | `HostStackState` |

### F-lb
| message | current max | allocation |
|---|---|---|
| `ServicesConfig` | 7 | **11 `lb`** → `LbService` |
| new messages (`// ----- F-lb -----`) | — | `LbService{settings, vips map, nat_interfaces}`, `LbSettings`, `LbVip`, `LbServer`, `LbNatInterface`, `LbVipState` (from 1) |
| RPCs | — | `LbState`, `LbFlushVip` |

### F-qos-flat
| message | current max | allocation |
|---|---|---|
| `QosService` / `QosPolicer` / `QosInterface` | 4 / 12 / 6 | **5 / 13 / 7 reserved, only with a proven gap** (none expected: `services.qos` is complete) |
| RPCs | — | `QosPolicerState` (conform/exceed/violate counters), `QosPolicerReset` |

### F-snmp
| message | current max | allocation |
|---|---|---|
| `SnmpService` | 11 | **12 `sys_services`**, **13 `views`** (`map<string, SnmpView>`), **14 `monitors`** (`SnmpMonitors`), **15 `subagent`** (`SnmpSubagent{enabled}` — the private-MIB/AgentX switch, only if the UI needs it) — D-086 stand-ins |
| `SnmpService.Community` | 3 | **4 `view`** |
| `SnmpService.V3User` | 6 | **7 `view`** |
| new messages (`// ----- F-snmp -----`) | — | `SnmpView{include, exclude}`, `SnmpMonitors{disks, load}`, `SnmpMonitorDisk`, `SnmpMonitorLoad`, `SnmpSubagent` (from 1) |
| RPCs | — | `SnmpState` only if renderer Retrieve cannot carry daemon status / last trap (default none) |
Fields 12–15 and the two `view` fields are one-line insertions inside P02c's `SnmpService` block (single toucher; never reorder).

### F-ipfix-sflow
| message | current max | allocation |
|---|---|---|
| `IpfixService` / `.Exporter` / `.Flowprobe` / `.Sflow` | 3 / 8 / 6 / 8 | **4 / 9 / 7 / 9 reserved, only with a gap** (e.g. exporter selection per producer) |
| RPCs | — | `IpfixState` (exporters, flowprobe + sflow interfaces, sampling counters) |

### F-dashboard-prom-alarms
| message | current max | allocation |
|---|---|---|
| `ManagementConfig` | 4 | **5 `prometheus`** (`ManagementPrometheus{enabled, listen, allow}`), **6 `alarms`** (`ManagementAlarms{rules, targets}` — consumed by the API; mirrored for the drift guard) |
| `StatsBatch` / `StreamStatsRequest` | 5 / 3 | **6 `buffers`, 7 `node_errors` / 4 `include_buffers`, 5 `include_node_errors`** — only if `/state/dashboard` reads them through StreamStats |
| RPCs | — | `DashboardSnapshot` — only as the alternative to the StatsBatch fields (take one path, log it with options) |
| new messages (`// ----- F-dashboard-prom-alarms -----`) | — | `ManagementPrometheus`, `ManagementAlarms`, `AlarmRule`, `AlarmTarget`, `BufferStats`, `NodeErrorCounter` (from 1) |

### F-capture-trace
| message | current max | allocation |
|---|---|---|
| `CaptureAction` | 6 | **7 `drop`** (bool — VPP's drop capture), **8 `error_filter`** (string `node/error`, optional) |
| `CaptureDirection` / `ActionRequest` | 3 / 3 | nothing (capture = existing member 3) |
| RPCs (only if the agent keeps the files) | — | `CaptureList`, `CaptureRead` (server stream), `CaptureDelete`; PG: `PgInterfaceCreate`, `PgInterfaceDelete`, `PgCapture` |

### F-licensing
No proto or config-document numbers: nothing licence-related crosses the API↔agent boundary (D-040), and the licence is uploaded
through `PUT /api/v1/system/license`, not the config document.

### Shared seams these eight need beyond wave-A §1 (the manager seeds `// wave-BC: <id>` anchors before the first spawns)
- services tab registry `apps/web/src/domains/services/{ServicesPage.tsx,tabs.ts}` (W-seed shell): F-host-stack, F-lb, F-qos-flat, F-snmp, F-ipfix-sflow (+ F-kea-dhcp-relay, F-unbound-chrony-syslog)
- `Domains["services"]` in `subsystems.go`: the same five; `Domains["management"]`: F-dashboard-prom-alarms (F-unbound-chrony-syslog adds the key)
- agent metrics collector hook in `internal/agent/metrics.go` (A5 core, manager-seeded seam): F-dashboard-prom-alarms — its families are served by the existing P05 `/metrics`
- `server.go` Action switch case: F-capture-trace (`ActionRequest_Capture`, existing member 3); `tools` route in `router.tsx` + `tools` NavItem: F-capture-trace only
- `apps/api/src/commit/validation.service.ts` (`ValidationTier` + one stage call): F-licensing only · `apps/web/src/shell/AppShell.tsx` banner slot (beside `ConfirmBanner`/`SyncBanner`): F-licensing
- `apps/agent/internal/renderers/ALLOWLIST.md` (= SY5): F-lb (the constant `cli_inband` lb GC command, D-090) · `infra/bus.ts` TOPICS: F-dashboard-prom-alarms (`alarm.events`)
- DB (= SY3): F-dashboard-prom-alarms (alarm tables), F-licensing (only if the licence is stored in PostgreSQL) — protocol below

### DB migrations (API, Drizzle) — protocol, not numbers
F-dashboard-prom-alarms (alarm tables) and F-licensing (licence table, if stored in PostgreSQL) are the first features that add tables.
Tables go in `apps/api/src/db/schema.ts` directly under the task's anchor; the migration is generated with
`pnpm -C apps/api db:generate --name <slug>` against main's latest snapshot. **Migration indexes are not pre-allocated** (drizzle-kit chains
snapshots): at merge, if main already has a newer migration, the manager drops the branch's `migrations/<idx>_<slug>.sql`, its snapshot and
its `_journal.json` entry and regenerates on the merged `schema.ts` (generated-file rule 3). Main has 0000–0001; TD-2 adds 0002–0003.

## Wave B/C routing, MPLS, multicast, SRv6, LISP (F-ospf, F-isis-rip, F-bfd-redistribution, F-mpls-srmpls, F-mpls-ldp, F-igmp-mfib, F-srv6, F-lisp)

Sources: main@11a175b; `task/W-seed`@74ec04e + its working tree (anchor layout); `task/P08` proto maxima; RF-1 on main; the VPP v26.06 source in
`/root/vpp` (read-only) for facts marked "source". Maxima on P08's proto: `RoutingConfig` 9 (2, 3 reserved; wave-A §2 adds two more) ·
`OspfInterface` 8 · `OspfConfig` 6 · `IsisInterface` 5 · `IsisConfig` 5 · `RipInterface` 1 · `RipConfig` 5 · `BfdSession` 7 · `BfdConfig` 1 ·
`TunnelsConfig` 3 · `Redistribute` 6 (D-078 shared exception — not extended here) · `EventKind` 9 (wave-A §2 runs to 15).

### Pack rules (shared by the eight sections below)

**Anchor sites** — seeded by the manager in the wave-B/C anchor pass (W-seed rules: no behaviour change, generated files byte-identical,
proto anchors framed by one blank line), each group directly **above** the block's first `wave-A:` anchor, in the order listed (see "Anchor
placement" at the end of this file — never directly below the block's last anchor while that task is unmerged):
| id | file → block | tasks |
|---|---|---|
| A1 | `subsystems.go` → domain consts + new domain entries | F-lisp (`Tunnels` — shared with F-tunnels: whoever lands first adds the key) |
| A1 | `subsystems.go` → `Domains[Routing]` | F-bfd-redistribution, F-mpls-srmpls, F-igmp-mfib, F-srv6 (F-mpls-ldp's `mpls-route.ldp` is in no domain) |
| A1 | `subsystems.go` → end of `Register()` | all eight |
| A2 | `projection.go` → `project()` / `assemble()` feature blocks | F-bfd-redistribution, F-mpls-srmpls, F-igmp-mfib, F-srv6, F-lisp (F-ospf, F-isis-rip only via seam S3) |
| C1 | `domains/routing.ts` → `RoutingSchema` feature keys | F-ospf, F-isis-rip, F-mpls-srmpls, F-igmp-mfib, F-srv6 |
| C1 | `domains/routing.ts` → new anchor blocks in `OspfInterfaceSchema` (F-ospf, F-bfd-redistribution), `IsisSchema` / `RipSchema` / `RipInterfaceSchema` (F-isis-rip), `IsisInterfaceSchema` (F-isis-rip, F-bfd-redistribution), `BfdSessionSchema` / `BfdSchema` (F-bfd-redistribution) | as named |
| C1 | `domains/tunnels.ts` → `TunnelsSchema` | F-lisp (below F-tunnels' anchor) |
| C2 / C3 | `semantic/index.ts` (import + spread) / `schema/src/index.ts` | all eight |
| C5 | `dataplane.proto` → `service Dataplane` | all eight (F-ospf, F-isis-rip use theirs only if P12's routing-state RPC cannot serve their FRR readers) |
| C5 | `dataplane.proto` → `EventKind` | F-ospf, F-isis-rip, F-bfd-redistribution, F-mpls-ldp, F-igmp-mfib |
| C5 | `dataplane.proto` → `RoutingConfig` | F-ospf, F-isis-rip, F-mpls-srmpls, F-igmp-mfib, F-srv6 |
| C5 | `dataplane.proto` → `OspfInterface` (F-ospf, F-bfd-redistribution), `IsisConfig` / `RipConfig` / `RipInterface` (F-isis-rip), `IsisInterface` (F-isis-rip, F-bfd-redistribution), `BfdSession` / `BfdConfig` (F-bfd-redistribution), `TunnelsConfig` (F-lisp) | as named |
| C5 | `dataplane.proto` → end-of-file `// ----- <task-id> -----` section stubs | all eight |
| P1 / P4 / P5 | `app.module.ts` / `agent.client.ts` / `fake-agent.ts` | all eight (P4/P5 only with a new RPC) |
| P6 | `infra/bus.ts` TOPICS + `telemetry/relay.service.ts` EventKind cases | F-ospf, F-isis-rip, F-bfd-redistribution, F-mpls-ldp, F-igmp-mfib |
| W1 | `router.tsx` | F-ospf, F-isis-rip, F-bfd-redistribution, F-mpls-srmpls, F-igmp-mfib |
| W2 | `nav.ts` + `nav.test.ts` → non-domain NavItems (routing group) | F-ospf, F-isis-rip, F-bfd-redistribution, F-mpls-srmpls, F-igmp-mfib |
| W2 | `nav.ts` + `nav.test.ts` → `BUILT_DOMAINS` | F-srv6, F-lisp (`'vpn'`, only if P11/F-wireguard have not added it — a duplicate is a union the manager drops) |
| — | `apps/web/src/domains/vpn/tabs.ts` (W-seed shell) → `vpnTabs` | F-srv6, F-lisp |
| W3 | `i18n.ts` | all eight |
| C6 / A7 | `docs/contracts/proto.md` "Feature RPCs" / `docs/vpp-code-track.md` | append `### <task-id>: <Rpc>` / `### V-new (<task-id>)` — no anchor |

Dep-chained sites F-mpls-srmpls seeds in its **own** files for F-mpls-ldp (an obligation in its envelope): `packages/schema/src/domains/ext/mpls-srmpls.ts`
(`MplsSchema` → `// wave-BC: F-mpls-ldp`), its proto section (`MplsConfig` → comment `// 10 reserved: ldp (F-mpls-ldp)` + anchor),
`apps/web/src/domains/routing/mpls-srmpls/tabs.ts` (LDP tab anchor).

**Seams to decide before spawn** (none exists on main or in W-seed):
- **S1 dynamic desired source** (F-mpls-ldp label sync, F-igmp-mfib PIM→mFIB sync): "plan and apply a scoped KV set under the agent's
  transaction lock, started/stopped with the agent" needs `internal/agent/{agent,service}.go` (A5, read-only for features). Options:
  (a) the manager seeds a generic seam in the anchor pass, e.g. `subsystems.Env.ApplyScoped func(ctx, scope []string, kvs []scheduler.KV) error`
  + `Wiring.AddDynamicSource(name, run func(ctx))` started by `agent.Run` after the first resync, with a test proving scope isolation;
  (b) the first of the two to spawn owns it as an A5 exception (both may run at once → add/add risk); (c) each loop writes VPP outside the
  scheduler (breaks "one writer to VPP"). **Recommended (a)**, ~1–2 h. Envelope fallback: loop behind an interface in the task's own
  package, fake apply in tests, question filed, agent.go/service.go untouched.
- **S2 FRR per-interface lines** (`ip ospf …`, `ip router isis …`, `ip pim`, interface BFD): RF-1 renders `interface X` blocks only for
  descriptions and has no hook for protocol lines, while FRR prints them inside one `interface X` block. Recommended: add
  `frr.RegisterInterfaceLines` (or equivalent) to P12's scope (P12 owns `renderers/frr/*.go` gap-only and runs first). Fallback: a
  section renders its own `interface X … exit` block and proves with `frrtest` that frr-reload's DryRun sees it as converged; else question.
- **S3 routing warning** in `projection.go` (one `if` over bgp/ospf/isis/rip/bfd/policy → "rendered by RF-1 (FRR), not by this agent
  build"): P12 replaces it for its leaves. Recommended: P12 makes it table-driven (one line per handled leaf, `wave-BC` anchors).
  Fallback: do not edit the condition; question; the manager edits it at merge.

**FRR sections** (RF-1 registry: unique names, orders 400–899; equal orders are legal, sorted by name) — suggested: `rip` 420, `ripng` 430,
`ospf` 440, `ospf6` 450, `isis` 470, `pim` 480, `ldp` 600, `bfd` 700 (P12 picks `bgp`/policy). F-bfd-redistribution's `redistribute`
package is a state reader, not a section.

**FRR daemon ownership in parallel:** `frrtest` instances are per slot (pathspace `w<SLOT>`, own netns, own socket dir, per-slot flock),
so P12 and the FRR tasks here (F-ospf, F-isis-rip, F-bfd-redistribution, F-mpls-ldp, F-igmp-mfib) can run at once, each only in its own
slot; `frr.service` has no owner and stays disabled; nothing under `/etc/frr`. This reads shared-host-rules §3 as one owner per daemon
*instance* (as D-089 did for charon) — **the manager confirms and logs it**, or serializes the FRR tasks.

**Names:** controllers `OspfController`, `IsisRipController`, `BfdRedistributionController`, `MplsSrmplsController`, `MplsLdpController`,
`IgmpMfibController`, `Srv6Controller`, `LispController`; locale namespace = slug; WS topics `ospf.events`, `isis-rip.events`,
`bfd-redistribution.events`, `mpls-ldp.events`, `igmp-mfib.events`; validator ids `routing.<slug>-…` (`tunnels.lisp-…` for F-lisp; never
re-add the existing `routing.ospf-area-exists`, `routing.interface-exists`, `routing.route-map-exists`, `routing.bfd-session-unique`).
Test secrets use `VRX_TEST_PSK_<id>_<n>`, shortened where the protocol caps the length (D-086 precedent): OSPF MD5 ≤ 16 chars →
`VRXTPSKospf<n>`, BFD keyed SHA1 ≤ 20 bytes → `VRXTPSKbfd<n>`.

**EventKind for this pack: 20–25, spare 26–29.** 16–19 are left unallocated for other wave-B/C sections (VRRP, HA sync, alarms …).
**RoutingConfig for this pack: 13–17, spare 18–19.** 12 is left for P12 (its "routing-level LCP leaf" option); unused, it stays reserved.

### F-ospf
| message | allocation |
|---|---|
| `RoutingConfig` | **13 `ospf6`** → `Ospf6Config` (own key, not a family switch in `routing.ospf` — the prompt's open question, decided as the default) |
| `OspfInterface` | **9 `auth`** → `OspfInterfaceAuth{type md5\|none, key_id, key_ref "password/<name>"}` |
| `EventKind` | **20 `EVENT_KIND_OSPF_NEIGHBOR_CHANGED`** (v2 and v3) |
| new messages (`// ----- F-ospf -----`) | `Ospf6Config`, `Ospf6Area`, `Ospf6Interface`, `Ospf6Redistribute{connected, static, bgp, isis, ripng}`, `OspfInterfaceAuth` (+ `OspfState*` only with the RPC) |
| RPCs | `OspfState` only if P12's routing-state RPC cannot return registered FRR state readers by key |
Schema: `ext/ospf.ts` (`Ospf6Schema`, `OspfInterfaceAuthSchema`); key lines `RoutingSchema.ospf6`, `OspfInterfaceSchema.auth`. FRR sections `ospf`, `ospf6`.

### F-isis-rip
| message | allocation |
|---|---|
| `RoutingConfig` | **14 `ripng`** → `RipngConfig` |
| `IsisConfig` | **6 `area_password_ref`**, **7 `domain_password_ref`** |
| `IsisInterface` | **6 `ipv4`**, **7 `ipv6`** (explicit presence; default both on) |
| `RipConfig` | **6 `version`** (2 only) |
| `RipInterface` | **2 `auth`** (only if the one-key RIP auth is built; else stays reserved) |
| `EventKind` | **21 `EVENT_KIND_ISIS_ADJACENCY_CHANGED`** |
| new messages (`// ----- F-isis-rip -----`) | `RipngConfig`, `RipngInterface`, `RipngRedistribute{connected, static, bgp, isis, ospf6}` (+ `RipInterfaceAuth`, `IsisRipState*`) |
| RPCs | `IsisRipState` only under the F-ospf condition |
Schema: `ext/isis-rip.ts`; key lines `RoutingSchema.ripng`, `IsisSchema.{areaPasswordRef, domainPasswordRef}`, `IsisInterfaceSchema.{ipv4, ipv6}`,
`RipSchema.version`, (`RipInterfaceSchema.auth`). FRR sections `isis`, `rip`, `ripng`; new globals-only descriptor `lcp.osi-proto` (package `descriptors/lcp_osi`).

### F-bfd-redistribution
| message | allocation |
|---|---|
| `BfdSession` | **8 `auth`** → `BfdSessionAuth{type, key_id, key_ref "key/<name>"}`, **9 `multihop`** |
| `BfdConfig` | **2 `profiles`** → `map<string, BfdProfile>` (FRR bfdd profiles for protocol-attached BFD) |
| `OspfInterface` | **10 `bfd_profile`** (only if per-interface profiles are built; else reserved) |
| `IsisInterface` | **8 `bfd_profile`** (idem) |
| `EventKind` | **22 `EVENT_KIND_BFD_SESSION_CHANGED`** (VPP and FRR sessions) |
| new messages (`// ----- F-bfd-redistribution -----`) | `BfdSessionAuth`, `BfdProfile`, `BfdState*`, `Redistribution*` |
| RPCs | `BfdState`, `RedistributionMatrix` |
Not here: `StaticRoute.via_frr` and its selector (F-vrf-static-ecmp, D-109a). Schema: `ext/bfd-redistribution.ts`. FRR section `bfd`.

### F-mpls-srmpls
| message | allocation |
|---|---|
| `RoutingConfig` | **15 `mpls`** → `MplsConfig` |
| `MplsConfig` (new) | 1–9 this task (interfaces, tables, label_routes, ip_bindings, tunnels, sr …) · 10 is F-mpls-ldp's (comment + anchor) |
| new messages (`// ----- F-mpls-srmpls -----`) | `MplsConfig`, `MplsLabelRoute`, `MplsPath`, `MplsIpBinding`, `MplsTunnel`, `MplsSrPolicy`, `MplsSrSegmentList`, `MplsSrSteering`, `MplsState*` |
| RPCs | `MplsState` (MPLS FIB paged, tunnels) |
Schema: `ext/mpls-srmpls.ts` (`MplsSchema` a plain object + the `// wave-BC: F-mpls-ldp` anchor); key line `RoutingSchema.mpls`. No EventKind.

### F-mpls-ldp
| message | allocation |
|---|---|
| `MplsConfig` | **10 `ldp`** → `MplsLdp{router_id, transport_address, interfaces, neighbors map, label_range}` |
| `EventKind` | **23 `EVENT_KIND_LDP_NEIGHBOR_CHANGED`** (sync errors use the existing ERROR / DEGRADED kinds) |
| new messages (`// ----- F-mpls-ldp -----`) | `MplsLdp`, `MplsLdpNeighbor`, `MplsLdpLabelRange`, `MplsLdpState*` |
| RPCs | `MplsLdpState` (only if P12's FRR state path is BGP-specific — the prompt's condition) |
Schema: `ext/mpls-ldp.ts`; the key line goes under F-mpls-srmpls's anchor in `ext/mpls-srmpls.ts`. FRR section `ldp`; descriptor instance `mpls-route.ldp`; seam S1.

### F-igmp-mfib
| message | allocation |
|---|---|
| `RoutingConfig` | **16 `multicast`** → `MulticastConfig` |
| `EventKind` | **24 `EVENT_KIND_IGMP_GROUP_CHANGED`**, **25 `EVENT_KIND_PIM_NEIGHBOR_CHANGED`** (optional) |
| new messages (`// ----- F-igmp-mfib -----`) | `MulticastConfig`, `MulticastIgmp*`, `MulticastMroute*`, `MulticastPim*`, `MulticastState*` |
| RPCs | `MulticastState` |
Schema: `ext/igmp-mfib.ts`; key line `RoutingSchema.multicast`. FRR section `pim`; new descriptors `mfib.route`, `bier.*` (optional); seam S1.

### F-srv6
| message | allocation |
|---|---|
| `RoutingConfig` | **17 `srv6`** → `Srv6Config` (default home `routing.srv6`, not `tunnels.srv6`) |
| new messages (`// ----- F-srv6 -----`) | `Srv6Config`, `Srv6LocalSid`, `Srv6Policy`, `Srv6SidList`, `Srv6Steering`, `Srv6State*` |
| RPCs | `Srv6State` |
No proxy messages (srv6-ad/am/as have no binary API in 26.06). Schema: `ext/srv6.ts`; key line `RoutingSchema.srv6`. No EventKind.

### F-lisp
| message | allocation |
|---|---|
| `TunnelsConfig` | **10 `lisp`** → `LispConfig` (default home `tunnels.lisp`; the lower numbers are F-tunnels'; 8–9 stay unallocated) |
| new messages (`// ----- F-lisp -----`) | `LispConfig`, `LispLocatorSet`, `LispLocator`, `LispLocalEid`, `LispRemoteMapping`, `LispAdjacency`, `LispEidTable`, `LispState*` |
| RPCs | `LispState` |
Schema: `ext/lisp.ts`; key line `TunnelsSchema.lisp`. No EventKind.

## S5 system (F-restconf-yang, F-aaa, F-backup-restore, P10, P14, F-ab-upgrade, F-images, F-hardening-lite)

Maxima checked on main@11a175b and P08's proto: `ManagementConfig` 4 (5–6 = F-dashboard-prom-alarms above) · `ManagementAaa` 3 ·
`ActionRequest` 3 (wave-A 4–8; wave-B envelopes already claim 9–10 F-det44-map-dslite-cnat, 11 F-ikev2-native, 12 F-ra-vpn) ·
`ActionOutput` 3. Only F-aaa and F-backup-restore change the contract; the other six allocate names and paths, not numbers.
**`ActionRequest` for this pack: 20–21, spare 22–24.** 13–19 are left for other wave-B/C sections. **`ActionOutput`: 4**, conditional.

### Pack rules: shared files these eight add (ids SY1–SY9, not in wave-A §1)
Anchors read `// wave-BC: <task-id>` (`#` in shell/debian files), seeded by the manager before the first of these spawns. Until an
anchor exists, insert at the end of the block and list the hunk under "Shared hunks".
| id | path | touchers | protocol |
|---|---|---|---|
| SY1 | `apps/api/src/auth/route-guard.test.ts` (`PUBLIC` / `READONLY_MAY` / `ADMIN_ONLY` sets) | F-aaa (PUBLIC: MFA verify, OIDC start/callback; ADMIN_ONLY: `actions/aaa/test`), F-backup-restore (ADMIN_ONLY), F-restconf-yang (only if `/.well-known/host-meta` goes public) | anchor per task in each set |
| SY2 | `apps/api/src/app.ts` `configureApp` content-type parsers (8 MiB body limit) | F-restconf-yang (`application/yang-data+json`), F-backup-restore (streamed upload) | register from the feature's own module first (`HttpAdapterHost` in `onModuleInit`, before `ready`); only if impossible, one line under the anchor |
| SY3 | `apps/api/src/db/schema.ts` + `apps/api/migrations/**` | F-aaa (`f_aaa_mfa`), F-backup-restore (`f_backup_restore`) | the Drizzle protocol under "DB migrations" in the wave-C section above: tables under the anchor, `pnpm -C apps/api db:generate --name <slug>`, regenerated on top of main at the D-112 rebase / merge, never hand-merged |
| SY4 | `packages/schema/src/domains/management.ts` | `ManagementSchema`: F-backup-restore, F-dashboard-prom-alarms · `AaaSchema`: F-aaa only (in-place widening of `AuthMethod` and `order.max(3)` + key lines) · `SyslogTargetSchema`: F-unbound-chrony-syslog | C1: sub-schemas in `domains/ext/<slug>.ts`, one key line under the anchor |
| SY5 | `apps/agent/internal/renderers/ALLOWLIST.md` (read by the source-scanning allowlist test) | F-backup-restore (`vrx-upgrade`, `vrx-support-collect`); also P11, F-host-acl-nftables, F-unbound-chrony-syslog | one row per binary under the anchor, fixed argv documented |
| SY6 | `deploy/debian/vrx/**` install lists (exist after P10 merges) | F-ab-upgrade, F-hardening-lite, F-backup-restore | P10 seeds `# wave-BC: <id>` for these three in its install files (P10 envelope obligation) |
| SY7 | root `.gitignore` (`/.scratch/`) | P10, P14, F-ab-upgrade, F-images, F-hardening-lite | the manager adds the line once; until then never `git add -A` |
| SY8 | `pnpm-lock.yaml` (= wave-A D4) | F-restconf-yang (new workspace package `packages/yang`), F-aaa (e.g. `ldapts`, `openid-client`, a QR helper), F-backup-restore (e.g. `ssh2`) | questions file first; the worker runs `pnpm install` once and commits the lockfile; the manager re-resolves on main at merge, never hand-merges. New packages need the npm registry |
| SY9 | `tools/ci.sh` (= wave-A D3, manager only) | F-restconf-yang (`packages/yang/modules` → `GEN_PATHS`), P10/P14/F-ab-upgrade/F-images/F-hardening-lite/F-backup-restore (a `deploy/<dir>` shellcheck + `tests/run.sh` step like TD-6's deploy/vpp step; P14's `iso` target) | workers ask; the manager edits |
Plus wave-A ids: P1/W1/W3 per task. W2: these screens are **non-domain system items** in `buildNav`'s `groups.get('system')!.push(…)` block,
one anchor per task, and nobody adds `management` to `BUILT_DOMAINS`. A4: F-backup-restore's case in the Action switch.

### F-aaa
| message | allocation |
|---|---|
| `ManagementAaa` (max 3) | **4 `ldap`** (`AaaLdap`) · **5 `oidc`** (`AaaOidc`) · **6 `saml`** (`AaaSaml`) · **7 `role_map`** (repeated `AaaRoleMapping`) · **8 `mfa`** (`AaaMfa`) · **9 `fallback_local`** (optional bool) |
| new messages (`// ----- F-aaa -----`, own numbers from 1) | `AaaLdap{servers}`, `AaaLdapServer{url, bind_dn, bind_password_ref, base_dn, user_filter, group_attr, start_tls}`, `AaaOidc{issuer, client_id, client_secret_ref, scopes, role_claim}`, `AaaSaml{idp_metadata_url, idp_metadata, sp_entity_id, role_attr}`, `AaaRoleMapping{group, role}`, `AaaMfa{required, issuer}` |
| `ManagementAaa.order` | new string values `ldap`, `oidc`, `saml` (no number) |
| RPC / EventKind / ActionRequest | none: API-only (D-040) |
| DB | migration `f_aaa_mfa` (encrypted TOTP seeds, recovery-code hashes, external identity link), SY3 |
Secret-flagged leaves get no proto field (D-040). The references `bind_password_ref` and `client_secret_ref` are plain strings and are
mirrored, like `RadiusServer.secret_ref`. Names: `AaaController`, rule ids `management.aaa-…`, locale namespace `aaa`, nav item `aaa`.

### F-backup-restore
| message | allocation |
|---|---|
| `ManagementConfig` | **7 `backup`** (`ManagementBackup`) · **8 `templates`** (`map<string, ConfigTemplate>`), only if templates live in the document; if they go to a table, 8 stays unused |
| `ActionRequest.action` | **20 `upgrade`** (`UpgradeAction{op, bundle}` + enum `UpgradeOp`) · **21 `support_bundle`** (`SupportBundleAction{since_sec, audit_rows}`) |
| `ActionOutput.output` | **4 `file_chunk`** (bytes), only if the host-side bundle part streams through the Action. The alternative (the agent writes under `/data/support/` and returns the path in `done.stats`) needs no number |
| new messages (`// ----- F-backup-restore -----`) | `ManagementBackup`, `BackupTarget`, `ConfigTemplate`, `UpgradeAction`, `UpgradeOp`, `SupportBundleAction` |
| RPC / EventKind | none (progress = `ActionOutput.line`) |
| DB | migration `f_backup_restore` (schedules, run log, templates if a table), SY3 |
Names: `BackupRestoreController`, rule ids `management.backup-restore-…`, locale namespace `backup-restore`, nav items `backup-restore` and
`upgrade`, agent package `internal/actions/backup-restore`, ALLOWLIST rows `vrx-upgrade` and `vrx-support-collect`, paths `/data/backups`,
`/data/updates`, `/data/support` (created by P10; the api user may write them).

### F-restconf-yang
No schema, proto, RPC or EventKind change: YANG hints live in a `packages/yang` side table keyed by JSON pointer. The generated
`packages/yang/modules/*.yang` join C7 and `GEN_PATHS` (SY9). Proposal (open question in the prompt): module `vrx-<root-key>`, namespace
`urn:vrx:yang:<root-key>`, plus `vrx-operations` for `commit|rollback|confirm`. Names: `RestconfYangController`
(`@ApiExcludeController()` by default), locale namespace `restconf-yang`, nav item `restconf-yang`.

### P10
No numbers. Source package `deploy/debian/vrx/` → `vrx-agent`, `vrx-api`, `vrx-web`, `vrx-meta` (P11 builds `vrx-strongswan`
separately). Units `vrx-agent.service`, `vrx-api.service`, `vrx-firstboot.service`, nginx site `vrx`. User/group `vrx`. Directories
`/etc/vrx`, `/var/lib/vrx/{agent,api}`, `/run/vrx`, `/data/{backups,updates,support}`. nftables: static table **`inet vrx_base`** (never
`inet vrx`: F-host-acl-nftables, D-057). APT suite `resolute`, component `main`; the repo is published by the manager to
`/srv/vrx-artifacts/apt/`; the key lives in `~/.config/ngfw/apt-signing/`. VPP artefacts go to `/srv/vrx-artifacts/vpp/<version>/` (D-089).
sysctl files: `60-vrx-netlink.conf` (linux_nl `rmem_max`, PENDING-vpp-host-hardening B). Seeds the SY6 anchors.

### P14
No numbers. `deploy/image/iso/`, `deploy/image/common/` (read-only for F-images), `deploy/image/build-iso.sh`; output
`vrx-<version>.iso` + `.sha256` + `.asc` under `.scratch/`. **Partition labels** (F-ab-upgrade and F-images use the same ones):
`VRX-EFI`, `vrx-rootA`, `vrx-rootB`, `vrx-log`, `vrx-pg`, `vrx-data`. Appliance marker `/etc/vrx/appliance`, written by the installer and
checked by `vrx-upgrade` before it touches the running system.

### F-ab-upgrade
No numbers. CLI `vrx-upgrade` (`status [--json]` | `stage <bundle>` | `activate` | `confirm` | `rollback`; argv, exit codes and JSON are
F-backup-restore's contract). Unit `vrx-upgrade-health.service`. Bundle `vrx-update-<version>.tar`. Default key: dedicated Ed25519 in
`~/.config/ngfw/upgrade-signing/`. GRUB entries `vrx-rootA` and `vrx-rootB` (proposal).

### F-images
No numbers. Outputs `vrx-<version>.{qcow2,vmdk,ova,vhdx}` + `SHA256SUMS` + `manifest.json` under `.scratch/`. Builder
`deploy/image/vm/build.sh`, cloud profiles `deploy/image/cloud/{aws,azure,gcp}/`. P14's partition labels.

### F-hardening-lite
No numbers. Drop-ins `deploy/hardening/systemd/<unit>.d/10-vrx-hardening.conf`; sysctl `70-vrx-hardening.conf` (after P10's
`60-vrx-netlink.conf`, so hardening cannot silently lower `rmem_max`); sshd `sshd_config.d/50-vrx.conf`; tool `deploy/hardening/check.sh`.

**Next free after this pack:** `ManagementConfig` 9 · `ManagementAaa` 10 · `ActionOutput` 5 · `ActionRequest`: 20–24 are this pack's,
25+ after it; for 13–19 see the last "Next free" line in this file (later sections take from there).

## Wave B/C NAT transition, tunnels, VPN, HA (F-det44-map-dslite-cnat, F-tunnels, F-ikev2-native, F-pki, F-ra-vpn, F-vrrp-config-sync, F-ha-state-sync)

Maxima checked on P08's proto (`task/P08`, `dataplane.proto` 3373 lines): `NatConfig` 24 (25–26 = F-nat44-ed-sessions "only if needed") ·
`Det44Config` 7 · `DsliteConfig` 4 · `MapConfig` 3 · `MapDomain` 11 · `CnatConfig` 2 · `CnatTranslation` 6 · `TunnelsConfig` 3 ·
`GreTunnel` 13 · `VxlanTunnel` 16 · `IpipTunnel` 13 · `IpsecSettings` 2 · `IpsecTunnel` 26 · `PkiCa` 4 · `PkiCertificate` 6 ·
`RemoteAccessProfile` 16 · `RemoteAccessUser` 2 · `HaConfig` 3 (1 reserved) · `VrrpInstance` 14 · `HaCluster` 9 · `HaCluster.StateSync` 3 ·
`ActionRequest` 3 (+ wave-A 4–8) · `EventKind` 9 (+ wave-A 10–15).
**`ActionRequest` for this pack: 9–13** (9–12 were first written into the envelopes and are cited by the S5 section above; 13 is taken from
its "13–19 open" range; 14–19 stay open). **`EventKind` for this pack: 16–17** (from the 16–19 range the routing section left for VRRP/HA;
18–19 stay open). Kill/close/disconnect/resync-style operations are `ActionRequest` members (the F-nat44-ed-sessions pattern: the API
calls F-vrf-static-ecmp's generic Action stream); read-only state is a unary RPC.

### Pack rules
**Anchors** read `// wave-BC: <task-id>` like the rest of this file, seeded by the manager in the wave-B/C anchor pass. Sites (besides the
C5 end-of-file `// ----- <task-id> -----` stubs, W3 `i18n.ts` and P1/P4/P5 for all seven):
| id | file → block | tasks |
|---|---|---|
| A1 | `subsystems.go` → `Domains["nat"]` | F-det44-map-dslite-cnat (below F-nat44-ed-sessions / F-nat44-ei-64-66-nptv6) |
| A1 | `subsystems.go` → new domain entries | F-tunnels (`Tunnels`, shared with F-lisp: whoever lands first adds the key), F-vrrp-config-sync (`Ha`; F-ha-state-sync appends) |
| A1 | `subsystems.go` → `Domains["vpn"]` | F-ikev2-native, F-pki (P11 / F-wireguard add the key) |
| A1 | `subsystems.go` → end of `Register()` | all but F-ra-vpn (it rides P11's renderer descriptor) |
| A2 | `projection.go` → `project()` / `assemble()` | all but F-ra-vpn (only with agent-programmed pool routes) and F-pki (only if its materialiser needs a projection) |
| A4 | `server.go` → `Action` type switch | F-det44-map-dslite-cnat (×2), F-ikev2-native, F-ra-vpn, F-ha-state-sync |
| C1 | `domains/nat.ts` → `NatSchema` | F-det44-map-dslite-cnat (`pnat`) |
| C1 | `domains/tunnels.ts` → `TunnelsSchema` | F-tunnels (above F-lisp's anchor) |
| C1 | `domains/vpn.ts` → `PkiCaSchema` / `PkiCertificateSchema` | F-pki; `IpsecSettingsSchema` F-ikev2-native (conditional); `RemoteAccess*Schema` F-ra-vpn (conditional) |
| C1 | `domains/ha.ts` → `HaSchema` / `HaClusterSchema` / `VrrpInstanceSchema` | F-vrrp-config-sync; `StateSync` F-ha-state-sync |
| C5 | `dataplane.proto` → `NatConfig` · `TunnelsConfig` + `IpipTunnel` · `PkiCa` + `PkiCertificate` · `HaConfig` + `VrrpInstance` + `HaCluster` · `HaCluster.StateSync` · `ActionRequest` · `EventKind` · `service Dataplane` | as allocated below |
| P6 | `infra/bus.ts` TOPICS (+ relay case) | F-pki `pki.expiry`, F-vrrp-config-sync `vrrp.events`, F-ra-vpn `ra-vpn.events` (conditional) |
| W1 / W2 | `router.tsx` / `nav.ts` + `nav.test.ts` `BUILT_DOMAINS` | F-tunnels (`/vpn/tunnels`, `'tunnels'`), F-vrrp-config-sync (`/system/ha`, `'ha'`) |
| — | `apps/web/src/domains/vpn/tabs.ts` (W-seed shell) → `vpnTabs` | F-pki, F-ra-vpn, F-ikev2-native (conditional) |

Dep-chained sites (the earlier task is merged; one named hunk, listed under "Shared hunks"): F-nat44-ed-sessions' `desired/nat.go` dispatch
and natTabs registry → F-det44-map-dslite-cnat (below F-nat44-ei-64-66-nptv6, which may run in parallel — the manager seeds both anchors) ·
P11's strongSwan renderer → F-pki (descriptor dependency + `load-creds`), then F-ra-vpn (RA build call, new file set, EAP plugins) · P11's
`desired/ipsec*.go` dispatch and IPsec tab → F-ikev2-native · F-vrrp-config-sync's HA-page panel registry (it creates it) → F-ha-state-sync.

**Slot port offsets** (inside the worker's own slot; add new ones here): UDP `20000+100·<SLOT>+`: 0–2 and 10–11 DF-5 tests, 20–21 P11 charon
IKE/NAT-T (F-ra-vpn reuses them), 30–31 F-ha-state-sync NAT HA listener/failover. TCP `3000+100·<SLOT>+`: 0 API (shared-host-rules), 50
F-vrrp-config-sync cluster node B API, 61 F-pki local CRL/OCSP responder.

**Names:** controllers `Det44MapDsliteCnatController`, `TunnelsController`, `Ikev2NativeController`, `PkiController`, `RaVpnController`,
`VrrpConfigSyncController`, `HaStateSyncController`; locale namespace = slug; validator ids `nat.det44-map-dslite-cnat-…`, `tunnels.…`
(domain owner), `vpn.ikev2-native-…`, `vpn.pki-…`, `vpn.ra-vpn-…`, `ha.vrrp-config-sync-…`, `ha.ha-state-sync-…`. Test secrets
`VRX_TEST_PSK_FIKEV2_<n>`, `VRX_TEST_PSK_FRAVPN_<n>`; VRRPv2 PASS keys ≤ 8 chars → `FVRtpsk<n>` (D-086 precedent).

### F-det44-map-dslite-cnat
| message | allocation |
|---|---|
| `NatConfig` | **27 `pnat`** → `PnatConfig` (25–26 stay F-nat44-ed-sessions') |
| `ActionRequest` | **9 `det44_session_close`** (`Det44SessionCloseAction{direction in\|out, …}`), **10 `cnat_session_purge`** (`CnatSessionPurgeAction`, globals owner only) |
| `Det44Config` / `DsliteConfig` / `MapConfig` / `MapDomain` / `CnatConfig` / `CnatTranslation` | **8–9 / 5 / 4 / 12 / 3 / 7 reserved, only with a proven gap** |
| new messages (`// ----- F-det44-map-dslite-cnat -----`) | `PnatConfig`, `PnatBinding`, `PnatMatch`, `PnatRewrite`, `PnatAttachment`, `Det44Session*`, `Det44Lookup*`, `CnatSession*`, the two actions (from 1) |
| RPCs | `Det44Sessions`, `Det44Lookup`, `CnatSessions` — never new fields on F-nat44-ed-sessions' `NatSessions*` / `NatSessionKillAction` (F-nat44-ei-64-66-nptv6 appends there in parallel) |
Schema: `ext/det44-map-dslite-cnat.ts` (`nat.pnat`, IPv4 only); key line `NatSchema.pnat`. No EventKind.

### F-tunnels
| message | allocation |
|---|---|
| `TunnelsConfig` | **4 `vxlan_gpe`**, **5 `gtpu`**, **6 `l2tpv3`**, **7 `pppoe`** (8–9 unallocated, 10 = F-lisp) |
| `IpipTunnel` | **14 `sixrd`** → `IpipSixrd` |
| `GreTunnel` / `VxlanTunnel` | **14 / 17 reserved, only with a proven gap** |
| new messages (`// ----- F-tunnels -----`) | `VxlanGpeTunnel`, `GtpuTunnel`, `L2tpv3Tunnel`, `PppoeSession`, `IpipSixrd`, `TunnelState*` (from 1) |
| RPCs | `TunnelState` |
Schema: new kinds as sub-schemas in `ext/tunnels*.ts` (or in place in `tunnels.ts`, the domain owner's call), key lines under `// wave-BC: F-tunnels`
in `TunnelsSchema`; `TUNNEL_KINDS` gets the new VPP name prefixes. No ActionRequest, no EventKind.

### F-ikev2-native
| message | allocation |
|---|---|
| `ActionRequest` | **11 `ikev2_sa`** → `Ikev2SaAction{tunnel, op INITIATE\|REKEY_CHILD\|DELETE_IKE_SA\|DELETE_CHILD_SA}` |
| `IpsecSettings` | **3–4 reserved** (liveness, sleep interval), only if the globals-owner question is answered "configure them" |
| `IpsecTunnel` | **30–31 reserved, only with a proven gap** (27–29 left for P11) |
| `EventKind` | none — native SA changes reuse P11's **12** with attribute `engine=vpp-ikev2` |
| new messages (`// ----- F-ikev2-native -----`) | `Ikev2SaAction`, `Ikev2SaOp`, `Ikev2Sa*` (from 1) |
| RPCs | `Ikev2Sas` |

### F-pki
| message | allocation |
|---|---|
| `PkiCa` | **5 `key_spec`** → `PkiKeySpec`, **6 `issued`** → `PkiIssued` |
| `PkiCertificate` | **7 `csr`** → `PkiCsr{subject, san[], key_spec}`, **8 `issued`** → `PkiIssued{serial, not_before, not_after, issuer, fingerprint}` |
| new messages (`// ----- F-pki -----`) | `PkiKeySpec`, `PkiCsr`, `PkiIssued`, `PkiFileState*` (from 1) |
| RPCs | `PkiFileState` (agent-side materialised-file fingerprints) |
No ActionRequest (PKI actions are API-side), no EventKind (expiry = API bus topic `pki.expiry`). Schema: `ext/pki.ts`.

### F-ra-vpn
| message | allocation |
|---|---|
| `ActionRequest` | **12 `remote_access_disconnect`** → `RemoteAccessDisconnectAction{profile, session}` |
| `RemoteAccessProfile` / `RemoteAccessUser` | **17–18 / 3 reserved, only with a proven gap** (RADIUS accounting, per-user static IP) |
| `EventKind` | **16 `EVENT_KIND_REMOTE_ACCESS_SESSION`**, only if connect/disconnect events are built (else stays reserved) |
| new messages (`// ----- F-ra-vpn -----`) | `RemoteAccessSession*`, `RemoteAccessDisconnectAction` (from 1) |
| RPCs | `RemoteAccessSessions` (paged) |

### F-vrrp-config-sync
| message | allocation |
|---|---|
| `HaConfig` | **4 `keepalived`** → `HaKeepalived{scripts map, sync_groups map}` (D-086 stand-ins) |
| `VrrpInstance` | **15 `keepalived`** → `VrrpKeepalived{track_scripts[]}` |
| `HaCluster` | **10 `sync_exclude`** (repeated JSON pointers) |
| `EventKind` | **17 `EVENT_KIND_VRRP_STATE_CHANGED`** (VPP events + keepalived notify) |
| new messages (`// ----- F-vrrp-config-sync -----`) | `HaKeepalived`, `HaKeepalivedScript`, `VrrpKeepalived`, `VrrpState*` (from 1) |
| RPCs | `VrrpState` |
Config sync is API-to-API (no proto). Revisions from a peer carry `config_revision.kind = 'cluster-sync'` (text column — no migration).

### F-ha-state-sync
| message | allocation |
|---|---|
| `HaCluster.StateSync` | **4 `nat_listener`** → `HaNatListener{address, port, path_mtu}`, **5 `nat_failover`** → `HaNatFailover{address, port, session_refresh_sec}` |
| `ActionRequest` | **13 `ha_sync`** → `HaSyncAction{op RESYNC\|FLUSH}` |
| new messages (`// ----- F-ha-state-sync -----`) | `HaNatListener`, `HaNatFailover`, `HaSyncAction`, `HaSyncOp`, `HaSyncState*` (from 1) |
| RPCs | `HaSyncState` |
No EventKind (the resync-completed event is consumed inside the action).

**Next free after this pack:** `ActionRequest` 14–19 · `EventKind` 18–19 · `NatConfig` 28 · `TunnelsConfig` 8–9 · `HaConfig` 5 · `HaCluster` 11.

## Batch-2 follow-ons (wave-A §2 numbers, binding since D-109 e — restated so each contract task reads one file)
| task | allocation |
|---|---|
| P11 | **EventKind 12** (one kind; up/down/rekey go in an attribute), **ActionRequest 8** (initiate/terminate, optional), `IpsecTunnel` **27–29** only with a proven gap |
| P12 | LCP pair leaf: **Interface 22** (on the interface) **or RoutingConfig 12** (routing-level) — one of the two, the other stays reserved · **StaticRoute 8 `tag`** · **EventKind 14** (routing change), **15** (BGP neighbour) |
| F-wireguard | **WireguardInterface 12 `route_allowed_ips`** · **EventKind 13** |
| F-unbound-chrony-syslog | **SyslogTarget 6–9** (facilities, format, queue_size, tls) · **ActionRequest 7 `dns_lookup`** |
| F-kea-dhcp-relay | **DhcpRelay 9–10** only if option-82 / remote-id is built (P08 max 8); rpc `DhcpLeases` |
| F-loopback-bvi-gso-lldp-span | **Interface 20 `gso`, 21 `mirror`** · **ServicesConfig 9 `nsim`** |
| F-acl / F-host-acl-nftables | **AclConfig 7** / **8**, each only with a config gap |

## Critic pass (prep-rest, all packs)
**Collision audit.** Every allocation in wave-A §2, in this file and in the 49 unmerged envelopes, with maxima re-checked on `task/P08`'s
proto: no number is allocated twice. Ledger of the contested messages:
- `ActionRequest.action`: 1–3 P05 · 4 F-neighbors-ra · 5 F-nat44-ed-sessions · 6 F-vrf-static-ecmp spare (unused) · 7 F-unbound-chrony-syslog · 8 P11 ·
  9–10 F-det44-map-dslite-cnat · 11 F-ikev2-native · 12 F-ra-vpn · 13 F-ha-state-sync · 14–19 open · 20–21 F-backup-restore · 22–24 S5 spare
- `EventKind`: 10 F-neighbors-ra · 11 F-object-model · 12 P11 · 13 F-wireguard · 14–15 P12 · 16 F-ra-vpn (conditional) · 17 F-vrrp-config-sync ·
  18–19 open · 20 F-ospf · 21 F-isis-rip · 22 F-bfd-redistribution · 23 F-mpls-ldp · 24–25 F-igmp-mfib · 26–29 routing-pack spare
- `RoutingConfig`: 10 neighbors · 11 pbr · 12 P12 (LCP option) · 13 ospf6 · 14 ripng · 15 mpls · 16 multicast · 17 srv6 · 18–19 routing-pack spare
- `Interface`: 13 bond · 14 l2 · 15–17 F-neighbors-ra · 18–19 F-rpf-adl-pbr · 20–21 F-loopback · 22 P12 (option) · 23–29 open
- `ServicesConfig` 8 auto_sdl · 9 nsim · 10 host_stack · 11 lb — `ManagementConfig` 5–6 F-dashboard · 7–8 F-backup-restore — `NatConfig` 25–26 ED ·
  27 F-det44 — `TunnelsConfig` 4–7 F-tunnels · 10 F-lisp — `OspfInterface` 9 F-ospf · 10 F-bfd — `IsisInterface` 6–7 F-isis-rip · 8 F-bfd —
  `IpsecTunnel` 27–29 P11 · 30–31 F-ikev2-native — `HaCluster` 10 F-vrrp · `HaCluster.StateSync` 4–5 F-ha-state-sync
Fixed in place: the P11 envelope asked for "EventKind ×1–2" (a second kind would have taken F-wireguard's 13); the P11, P12, F-wireguard,
F-kea-dhcp-relay and F-unbound-chrony-syslog envelopes pointed at "the manager's wave-B allocation table" and now cite the numbers above.

**Anchor placement** (this replaces the "below the existing `wave-A:` anchors" wording in the pack rules). Git merges two inserts cleanly only
when at least one unchanged line separates them (§0 rule 2). A `wave-BC` anchor inserted directly below a block's last `wave-A:` anchor lands on
the spot where that task, possibly still running (P12, F-wireguard, F-kea, F-unbound, the follow-ons), inserts its own lines, so its D-112 rebase
conflicts. Seed each `wave-BC` group directly **above** the block's first `wave-A:` anchor, or after an existing separator line (the blank line
that frames the proto anchors). Keep `nav.ts` and `nav.test.ts` in the same relative order, and never seed inside a file a running task owns.
Anchors in dep-chained feature files are seeded by the task that creates the file: F-nat44-ed-sessions (EI group + CGNAT group in `desired/nat.go`
and natTabs, an ED envelope obligation), F-mpls-srmpls (for F-mpls-ldp), F-vrrp-config-sync (HA panel registry for F-ha-state-sync), P12 (the S3 table, if M4).

**Shared new `Domains` keys.** A duplicate map key is a compile error, not a text conflict: `Tunnels` (F-tunnels × F-lisp, parallel) and
`Management` (F-unbound-chrony-syslog × F-dashboard-prom-alarms, unless the dep is added) — the second lander moves its names into the existing
entry at rebase. `Ha` (F-vrrp-config-sync → F-ha-state-sync) and `vpn` (P11/F-wireguard → F-ikev2-native, F-pki) are sequential by dep.

**Next free (all packs):** ActionRequest 14 (after 19: 25) · EventKind 18 (after 19: 30) · RoutingConfig 20 · Interface 23 · ServicesConfig 12 ·
ManagementConfig 9 · ManagementAaa 10 · ActionOutput 5 · NatConfig 28 · TunnelsConfig 8 · HaConfig 5 · HaCluster 11 · DhcpRelay 11.
