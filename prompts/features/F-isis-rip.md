# Task: F-isis-rip — IS-IS and RIPv2 / RIPng via FRR   (prepend 00-CONTEXT.md)

> Refreshed 2026-09-24 on `task/prep-rest` against main, `task/P08`, `task/W-seed`, the P12 envelope and the VPP 26.06 linux-cp source.
> Your TASK ENVELOPE (`docs/status/tasks/F-isis-rip.envelope.md`) wins for branch, files, numbers, anchors and process.

## Goal
IS-IS (v4+v6) and RIPv2 / RIPng end to end in FAST MODE: schema → FRR `isisd` / `ripd` / `ripngd` sections → routes into the **VPP FIB**
through linux-cp/linux-nl → API → UI → docs. Reference: TNSR "IS-IS", "RIP"; FRR 10 isisd/ripd/ripngd; VPP `linux_cp` + `linux_nl`
(loaded, D-060). WBS D3.6 (IS-IS, T2), D3.7 (RIPv2/RIPng, T2).

## Inputs to read first
- `packages/schema/src/domains/routing.ts` — existing: `routing.isis{net, level, vrf, interfaces{<if>: {passive, metric, circuitType,
  networkType, bfd}}, redistribute}` and `routing.rip{vrf, networks[] (IPv4), interfaces{<if>: {passive}}, redistribute, defaultMetric}`
  (D-045 layout). Existing rules (test, never re-add): `routing.interface-exists` (IS-IS/RIP interfaces), `routing.route-map-exists`,
  RIP network uniqueness. Missing → additive contract, committed **first on your task branch** as separate `contract(schema): …` /
  `contract(proto): …` commits (never a `contract/` branch): `routing.isis.interfaces.<if>.{ipv4, ipv6}` families (default both on),
  IS-IS area/domain password refs (`password/<name>`, D-051), `routing.ripng{networks[] (IPv6), interfaces, redistribute}`,
  `routing.rip.version` (2 only; v1 rejected) + proto messages; `docs/status/tasks/F-isis-rip-contract.md`. Numbers from
  `docs/status/wave-BC-numbers.md`: RoutingConfig 14 `ripng`, IsisConfig 6–7, IsisInterface 6–7, RipConfig 6, (RipInterface 2), EventKind 21.
- `apps/agent/internal/renderers/frr/README.md`, `section.go`, `frrtest/harness.go` (RF-1): one package per section, `RegisterSection` from
  `init()` (orders 400–899: `isis` 470, `rip` 420, `ripng` 430), escaping helpers, `rc.Secret` + redaction, `RegisterStateReader`/`RegisterPoller`.
  Blank-import your packages from your own `apps/agent/internal/subsystems/isis_rip.go`.
- `prompts/P12-frr-linuxcp.md` + merged P12 code and `docs/status/tasks/P12.md`: the FRR singleton descriptor, LCP pairs, the linux-cp
  mapper (`internal/lcpmap`), the linux-nl test plan (slot kernel VRF), netns FRR peers on the veth rig, P12's routing-state RPC,
  `/state/routes?proto=`. Framework gaps (wave-BC-numbers.md S2/S3): per-interface lines and the projection warning — as in the envelope.
- IS-IS runs over raw L2 (ISO/CLNS, no IP). **VPP 26.06 has the switch for it** (source: `src/plugins/linux-cp/lcp.c`, `lcp.api`):
  `lcp_osi_proto_enable(<osi proto>)` registers the OSI protocol on `osi_plugin.so` (loaded by default) towards `linux-cp-punt-xc`, so
  IS-IS frames reach the LCP tap; `lcp_osi_proto_get` lists what is enabled; binapi `lcp.LcpOsiProtoEnable`/`LcpOsiProtoGet` exist.
  It is **VPP-wide and has no disable** → a small globals-only descriptor `lcp.osi-proto` in your own package `descriptors/lcp_osi`
  (Retrieve from the getter, Delete = documented no-op, registered only on the globals owner, D-071; `### V-new` for the missing disable).
  The host FIB proof needs it enabled: only behind an opt-in under the exclusive globals lock in a manager window (it cannot be undone
  before a VPP restart — ask first). Without the window ship IS-IS as renderer + state with the FIB step skipped with the reason — no C.
- `docs/vpp-code-track.md` V1.

## Scope — build exactly this
Files you own: see the envelope (`renderers/frr/{isis,rip}/**`, `descriptors/lcp_osi/**`, `subsystems/isis_rip*.go`, `desired/isis_rip*.go`,
`agent/rpc_isis_rip*.go`, `ext/isis-rip*.ts`, `semantic/isis-rip*.ts`, `features/isis-rip/**`, `domains/routing/isis-rip/**`,
`isis-rip.json`, docs, `test/topology/isis-rip/**`). Shared files: one line under your `// wave-BC: F-isis-rip` anchor only.
1. **Schema** (contract commits): IS-IS NET unique system id; interface exists (existing rule); `circuitType` compatible with `level`
   (level-1 IS cannot run a level-2 circuit); RIP networks IPv4 only, RIPng IPv6 only; interfaces exist; redistribute route maps exist in
   `routing.policy.routeMaps` (existing rule — mirror for `ripng`).
2. **Agent**: sections `isis` (`router isis vrx [vrf]`, `net`, `is-type`, per-interface `ip router isis vrx` / `ipv6 router isis vrx`,
   metric, circuit type, p2p, passive, auth via `rc.Secret`), `rip` (`router rip`, `version 2`, network, passive-interface, default-metric,
   redistribute) and `ripng` (`router ripng`); the `lcp.osi-proto` global (above). State readers: `show isis neighbor json`,
   `show isis database json` (paged), `show ip rip status` (text → parsed; JSON where FRR 10 has it), `show ipv6 ripng status`; adjacency
   poller → events (EventKind 21). Golden + hostile-input unit tests; `frrtest` integration with `Daemons: mgmtd, zebra, staticd, isisd,
   ripd, ripngd` in your slot pathspace (never the system unit). Password end to end waits for PENDING-secret-channel (fixture resolver meanwhile).
3. **API**: config via pointer routes; `GET /api/v1/state/routing/isis/{adjacencies,database}`, `GET /api/v1/state/routing/rip/{peers,routes}`;
   routes via `/state/routes?proto=isis|rip` (F-vrf-static-ecmp's FIB browser + P12's filter). State through P12's routing-state RPC when
   it serves registered readers by key, else your own `IsisRipState` RPC.
4. **UI**: Routing → IS-IS page and RIP page (global form, interfaces table, neighbour/peer grid with live state; RIPng tab); en + fa;
   screenshot against the real endpoint in `docs/status/tasks/F-isis-rip.md`.
5. **Docs**: `docs/user/routing/isis-rip.md` (L2-only IS-IS example, RIPv2 + RIPng example, CLI equivalent, the OSI-punt requirement).

## Acceptance (paste the evidence)
- [ ] RIPv2 and RIPng: netns FRR peers on the veth rig (`path: af_packet`) announce 20 prefixes each → present in the **VPP FIB**
      (`vppctl show ip fib` / `show ip6 fib`), not only in `vtysh`; withdraw → gone within 200 s (RIP timers, or lowered timers pasted)
- [ ] IS-IS: adjacency Up and learned prefixes in the VPP FIB (with the OSI punt enabled in a manager window) — or, without the window /
      if linux-cp still does not carry CLNS frames, `lcp_osi_proto_get` output + the V-new entry + trace showing the drop (pasted)
- [ ] Agent-restart simulation → sections re-rendered, adjacencies and VPP routes back within 30 s (log excerpt)
- [ ] Rollback removes the router blocks (rendered file + `show running-config`) and the routes from VPP (Retrieve)
- [ ] level-1 IS with a level-2 circuit → 400 problem+json with `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
OSPF (F-ospf); BGP, route maps / prefix lists (P12); BFD and the redistribution matrix UX (F-bfd-redistribution); LCP pairs (P12);
IS-IS segment routing / TE / multi-topology beyond v4+v6 unicast; RIPv1; RIP authentication chains beyond one key; redistributing `ripng`
into other protocols; any VPP change or restart.

## Open questions to surface, not to decide silently
Who enables the OSI punt on a real box (proposed: the globals owner whenever `routing.isis` is present) and whether the irreversible
enable is acceptable on the shared host; `routing.ripng` as its own key (decided default in wave-BC-numbers.md).
