# Task: F-bfd-redistribution — BFD (VPP + FRR), redistribution matrix, route-policy UX   (prepend 00-CONTEXT.md)

## Goal
BFD in both engines and the operator view of route redistribution, end to end in FAST MODE. Reference: TNSR "BFD", "route maps /
prefix lists / redistribution"; VPP `bfd` (UDP single/multi-hop), FRR 10 `bfdd`. WBS D3.8 (BFD native VPP + FRR integration, T1),
D3.10 (redistribution matrix and route-policy UX, T1).

## Inputs to read first
- `packages/schema/src/domains/routing.ts` — existing: `routing.bfd.sessions[]` (`interface, localAddress, peerAddress, desiredMinTxUs,
  requiredMinRxUs, detectMultiplier, enabled`, itemKey interface+peer), `bfd: boolean` on BGP neighbours / OSPF / IS-IS interfaces,
  `redistribute{<source>: {metric, routeMap}}` per protocol (D-045), `routing.policy{prefixLists, routeMaps}` objects (D-070).
  Missing → contract branch `contract/F-bfd-redistribution` (additive): session auth (`auth{type: keyed-sha1|meticulous-keyed-sha1,
  keyId, keyRef: "key/<name>"}`, D-051), `multihop`, `routing.static[].viaFrr` (D-072 explicit flag — the route then goes to FRR/staticd
  and is **not** programmed by the agent), FRR BFD profile knobs for protocol-attached BFD; proto; `docs/status/tasks/F-bfd-redistribution-contract.md`.
- VPP BFD descriptors (DF-7, **still on branch** until merged — read with `git show task/DF-7:docs/agent/descriptors/bfd.md` and
  `…:apps/agent/internal/descriptors/bfd/`): `bfd.auth-key`, `bfd.udp-session` (`bfd_udp_add/mod/del`, `bfd_udp_session_set_flags`,
  `bfd_udp_auth_activate`), global `bfd.echo-source` (globals owner only, D-071), `bfd.WatchEvents` (`want_bfd_events`). Verify names in
  `apps/agent/binapi/bfd/` (also `bfd_udp_enable_multihop`). Reuse; fix inside `descriptors/bfd/` if needed (you own it after DF-7 merges).
- RF-1 FRR framework (`renderers/frr/README.md`, `section.go`, `model.go`): `RegisterSection`; D-072 `RegisterStaticSelector` (register it
  once, reading the new `viaFrr` field) and `StaticOwnedByFRR` — the P05/F-vrf-static-ecmp static-route descriptor must skip exactly those routes.
- `prompts/P12-frr-linuxcp.md`: P12 renders route-map / prefix-list / BGP; F-ospf and F-isis-rip render their own `redistribute` lines.

## Scope — build exactly this
Files you own: `apps/agent/internal/descriptors/bfd/**`, `apps/agent/internal/renderers/frr/{bfd,redistribute}/**`, `docs/agent/descriptors/bfd.md`,
`docs/agent/renderers/frr-{bfd,redistribute}.md`, `apps/agent/internal/agent/project_bfd_redistribution*.go`, `apps/api/src/features/bfd-redistribution/**`,
`apps/web/src/domains/routing/bfd-redistribution/**`, `apps/web/src/locales/*/bfd-redistribution.json`, `docs/user/routing/bfd-redistribution.md`,
`test/topology/bfd-redistribution/**`. Shared files: one-line appends only.
1. **Schema** (contract branch): BFD session interface exists and owns `localAddress`; one session per (interface, peer); a (interface, peer)
   used by a protocol with `bfd: true` must not also be a VPP session (one engine per peer — VPP BFD and FRR bfdd would both claim UDP 3784);
   redistribute/route-map references exist; redistribution loop warning (A→B and B→A without a route map).
2. **Agent**: project `routing.bfd.sessions` → DF-7 BFD objects; stream `bfd.WatchEvents` into agent events. FRR `bfd` section (`bfd` block
   with `peer … interface …` for protocol-attached BFD, profiles) and the `redistribute` package: D-072 static selector + a state reader that
   builds the **redistribution matrix** (source → target, route map, route counts from `show ip route json` per protocol).
3. **API**: pointer routes; `GET /api/v1/state/routing/bfd/sessions` (VPP + FRR sessions, state, timers, last flap), `GET /api/v1/state/routing/redistribution`
   (matrix), `GET /api/v1/state/routing/policy/{name}/hits` if FRR exposes counters (else omit and say so).
4. **UI**: BFD page (sessions grid with live up/down), redistribution matrix (protocols × protocols grid, click → edit that `redistribute` entry),
   route-policy editor UX over `routing.policy` (prefix-list rules + route-map entries with reorder, where-used links); en + fa; screenshot.
5. **Docs**: `docs/user/routing/bfd-redistribution.md` (VPP static-peer BFD, OSPF+BFD via FRR, redistribute static→OSPF with a route map).

## Acceptance (paste the evidence)
- [ ] VPP BFD session to a netns peer (FRR bfdd in its own pathspace, `path: af_packet` rig): `vppctl show bfd sessions` Up; peer down → event in the API within 2 s
- [ ] Static route with `viaFrr: true` redistributed into OSPF appears at the netns OSPF peer; the same route is **not** programmed by the agent (ip_route_dump source) — D-072
- [ ] Agent-restart simulation → sessions and FRR sections back within 30 s; rollback removes sessions (Retrieve) and FRR blocks
- [ ] Same (interface, peer) as VPP session and OSPF `bfd: true` → 400 problem+json with `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
BGP and the route-map/prefix-list FRR sections (P12); OSPF/IS-IS/RIP sections and their own redistribute lines (F-ospf, F-isis-rip);
static-route programming in VPP (F-vrf-static-ecmp); BFD echo mode tuning beyond the global owner; BFD for VRRP (F-vrrp-config-sync); PBR (F-rpf-adl-pbr).

## Open questions to surface, not to decide silently
Which engine owns BFD for protocol peers (proposed: FRR bfdd for protocol-attached, VPP for standalone/static peers — never both on one peer);
whether F-vrf-static-ecmp or this task registers the D-072 static selector (proposed: this task — it owns `viaFrr`).
