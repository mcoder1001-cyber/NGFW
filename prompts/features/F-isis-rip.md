# Task: F-isis-rip — IS-IS and RIPv2 / RIPng via FRR   (prepend 00-CONTEXT.md)

## Goal
IS-IS (v4+v6) and RIPv2 / RIPng end to end in FAST MODE: schema → FRR `isisd` / `ripd` / `ripngd` sections → routes into the **VPP FIB**
through linux-cp/linux-nl → API → UI → docs. Reference: TNSR "IS-IS", "RIP"; FRR 10 isisd/ripd/ripngd; VPP `linux_cp` + `linux_nl`
(loaded, D-060). WBS D3.6 (IS-IS, T2), D3.7 (RIPv2/RIPng, T2).

## Inputs to read first
- `packages/schema/src/domains/routing.ts` — existing: `routing.isis{net, level, vrf, interfaces{<if>: {passive, metric, circuitType,
  networkType, bfd}}, redistribute}` and `routing.rip{vrf, networks[] (IPv4), interfaces{<if>: {passive}}, redistribute, defaultMetric}`
  (D-045 layout). Missing → contract branch `contract/F-isis-rip` (additive): `routing.isis.interfaces.<if>.{ipv4, ipv6}` families
  (default both on), IS-IS area/domain password refs (`password/<name>`, D-051), `routing.ripng{networks[] (IPv6), interfaces, redistribute}`,
  `routing.rip.version` (2 only; v1 rejected) + proto messages; `docs/status/tasks/F-isis-rip-contract.md`.
- `apps/agent/internal/renderers/frr/README.md`, `section.go`, `frrtest/harness.go` (RF-1): one package per section, `RegisterSection` from
  `init()` (orders 400–899), escaping helpers, `rc.Secret` + redaction, `RegisterStateReader`/`RegisterPoller`.
- `prompts/P12-frr-linuxcp.md` + merged P12 code: LCP pairs, `rc.MapInterface`, netns FRR peers on the veth rig, `/state/routes?proto=`.
- `docs/vpp-code-track.md` V1 — IS-IS runs over raw L2 (ISO/CLNS, no IP): **linux-cp must punt/inject non-IP frames** on the pair. If VPP
  does not pass them to the tap (check `show lcp`, trace), record it under V1 and ship IS-IS as renderer + state with the host test skipped
  with the reason — do not write C.

## Scope — build exactly this
Files you own: `apps/agent/internal/renderers/frr/{isis,rip}/**`, `docs/agent/renderers/frr-{isis,rip}.md`, `apps/agent/internal/agent/project_isis_rip*.go`,
`apps/api/src/features/isis-rip/**`, `apps/web/src/domains/routing/isis-rip/**`, `apps/web/src/locales/*/isis-rip.json`, `docs/user/routing/isis-rip.md`,
`test/topology/isis-rip/**`. Shared files: one-line appends only (app.module import, router/nav entry, agent section import).
1. **Schema** (contract branch): IS-IS NET unique system id; interface exists; `circuitType` compatible with `level` (level-1 IS cannot run a
   level-2 circuit); RIP networks IPv4 only, RIPng IPv6 only; interfaces exist; redistribute route maps exist in `routing.policy.routeMaps`.
2. **Agent**: sections `isis` (`router isis vrx [vrf]`, `net`, `is-type`, per-interface `ip router isis vrx` / `ipv6 router isis vrx`,
   metric, circuit type, p2p, passive, auth via `rc.Secret`), `rip` (`router rip`, `version 2`, network, passive-interface, default-metric,
   redistribute) and `ripng` (`router ripng`). State readers: `show isis neighbor json`, `show isis database json`, `show ip rip status`
   (text → parsed; JSON where FRR 10 has it), `show ipv6 ripng status`; adjacency poller → events. Golden + hostile-input unit tests;
   `frrtest` integration with `Daemons: mgmtd, zebra, staticd, isisd, ripd, ripngd` in your slot pathspace (never the system unit).
3. **API**: config via pointer routes; `GET /api/v1/state/routing/isis/{adjacencies,database}`, `GET /api/v1/state/routing/rip/{peers,routes}`;
   routes via P12's `/state/routes?proto=isis|rip`.
4. **UI**: Routing → IS-IS page and RIP page (global form, interfaces table, neighbour/peer grid with live state); en + fa; screenshot against
   the real endpoint in `docs/status/tasks/F-isis-rip.md`.
5. **Docs**: `docs/user/routing/isis-rip.md` (L2-only IS-IS example, RIPv2 + RIPng example, CLI equivalent).

## Acceptance (paste the evidence)
- [ ] RIPv2 and RIPng: netns FRR peers on the veth rig (`path: af_packet`) announce 20 prefixes each → present in the **VPP FIB**
      (`vppctl show ip fib` / `show ip6 fib`), not only in `vtysh`; withdraw → gone within 200 s (RIP timers, or lowered timers pasted)
- [ ] IS-IS: adjacency Up and learned prefixes in the VPP FIB — or, if linux-cp does not carry CLNS frames, the V1 entry + trace showing the drop (pasted)
- [ ] Agent-restart simulation → sections re-rendered, adjacencies and VPP routes back within 30 s (log excerpt)
- [ ] Rollback removes the router blocks (rendered file + `show running-config`) and the routes from VPP (Retrieve)
- [ ] level-1 IS with a level-2 circuit → 400 problem+json with `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
OSPF (F-ospf); BGP, route maps / prefix lists (P12); BFD and the redistribution matrix UX (F-bfd-redistribution); LCP pairs (P12);
IS-IS segment routing / TE / multi-topology beyond v4+v6 unicast; RIPv1; RIP authentication chains beyond one key; any VPP change or restart.

## Open questions to surface, not to decide silently
IS-IS over linux-cp (CLNS/L2 frames) — confirm on the host before promising it; whether RIPng needs its own `routing.ripng` key (proposed).
