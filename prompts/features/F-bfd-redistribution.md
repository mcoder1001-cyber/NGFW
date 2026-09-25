# Task: F-bfd-redistribution — BFD (VPP + FRR), redistribution matrix, route-policy UX   (prepend 00-CONTEXT.md)

> Refreshed 2026-09-24 on `task/prep-rest` against main (DF-7 merged), `task/P08`, `task/W-seed`, the P12 / F-vrf-static-ecmp
> envelopes and the VPP 26.06 bfd source. Your TASK ENVELOPE (`docs/status/tasks/F-bfd-redistribution.envelope.md`) wins for branch,
> files, numbers, anchors and process.

## Goal
BFD in both engines and the operator view of route redistribution, end to end in FAST MODE. Reference: TNSR "BFD", "route maps /
prefix lists / redistribution"; VPP `bfd` (UDP single/multi-hop), FRR 10 `bfdd`. WBS D3.8 (BFD native VPP + FRR integration, T1),
D3.10 (redistribution matrix and route-policy UX, T1).

## Inputs to read first
- `packages/schema/src/domains/routing.ts` — existing: `routing.bfd.sessions[]` (`interface, localAddress, peerAddress, desiredMinTxUs,
  requiredMinRxUs, detectMultiplier, enabled`, itemKey interface+peer), `bfd: boolean` on BGP neighbours / OSPF / IS-IS interfaces,
  `redistribute{<source>: {metric, routeMap}}` per protocol (D-045), `routing.policy{prefixLists, routeMaps}` objects (D-070). Existing
  rules (test, never re-add): `routing.bfd-session-unique`, `routing.interface-exists`, `routing.route-map-exists`.
  Missing → additive contract, committed **first on your task branch** as separate `contract(schema): …` / `contract(proto): …` commits
  (never a `contract/` branch): session auth (`auth{type: keyed-sha1|meticulous-keyed-sha1, keyId, keyRef: "key/<name>"}`, D-051),
  `multihop`, FRR BFD profiles for protocol-attached BFD (`routing.bfd.profiles{<name>: …}`); proto; `docs/status/tasks/F-bfd-redistribution-contract.md`.
  Numbers from `docs/status/wave-BC-numbers.md`: BfdSession 8–9, BfdConfig 2, (OspfInterface 10, IsisInterface 8), EventKind 22.
  **`routing.static[].viaFrr` is not yours**: F-vrf-static-ecmp added it and registered `frr.RegisterStaticSelector` for it (D-072,
  D-109a) — consume it, never add a field or call the selector again (a second call panics).
- VPP BFD descriptors (DF-7, **merged**): `docs/agent/descriptors/bfd.md` and `apps/agent/internal/descriptors/bfd/` on main —
  `bfd.auth-key`, `bfd.udp-session` (`bfd_udp_add/mod/del`, `bfd_udp_session_set_flags`, `bfd_udp_auth_activate`), global
  `bfd.echo-source` (`bfd.RegisterGlobals`, globals owner only, D-071), `bfd.WatchEvents` (`want_bfd_events`), `bfd.Sessions`, the
  `bfd.Secrets` resolver interface. Verify names in `apps/agent/binapi/bfd/` (also `bfd_udp_enable_multihop`). Reuse; fix inside
  `descriptors/bfd/` only for a proven defect (you own it gap-only).
- **VPP claims the BFD ports** (source: `src/vnet/bfd/bfd_udp.c` 596–651 / 787–824): when the first VPP session of a kind exists, VPP
  registers UDP 3784/3785 (single-hop) or 4784 (multi-hop) for its own BFD node and releases them only at zero sessions. While any VPP
  BFD session of that AF/hop type exists on a VPP instance, BFD packets addressed to VPP never reach FRR bfdd behind linux-cp (linux-cp
  only punts *unknown* UDP). So "one engine per peer" is not enough — see Scope 1 and the open question; record it as `### V-new`.
- RF-1 FRR framework (`renderers/frr/README.md`, `section.go`, `model.go`): `RegisterSection` (your `bfd` section, order 700) and
  `StaticOwnedByFRR` (already called by P08's projection for `viaFrr` routes).
- `prompts/P12-frr-linuxcp.md` + `docs/status/tasks/P12.md`: P12 renders route-map / prefix-list / BGP and ships the basic prefix-list and
  route-map editors (`apps/web/src/domains/routing/bgp/`); F-ospf and F-isis-rip render their own `redistribute` and `… bfd` lines.

## Scope — build exactly this
Files you own: see the envelope (`descriptors/bfd/**` gap-only, `renderers/frr/{bfd,redistribute}/**`, `desired/bfd*.go`, `subsystems/bfd*.go`,
`agent/rpc_bfd*.go`, `ext/bfd-redistribution*.ts`, `semantic/bfd-redistribution*.ts`, `features/bfd-redistribution/**`,
`domains/routing/bfd-redistribution/**`, `bfd-redistribution.json`, docs, `test/topology/bfd-redistribution/**`). Shared files: one line
under your `// wave-BC: F-bfd-redistribution` anchor only.
1. **Schema** (contract commits): BFD session interface exists and owns `localAddress`; one session per (interface, peer) (existing rule);
   VPP sessions and protocol-attached FRR BFD of the **same address family and hop type** cannot coexist on one box (VPP holds the port,
   see above) — error with `pointer` on the protocol's `bfd: true` (decide with the open question; at minimum the per-peer check);
   redistribute/route-map references exist (existing rule); redistribution loop warning (A→B and B→A without a route map).
2. **Agent**: project `routing.bfd.sessions` → DF-7 BFD objects (keys through DF-7's `bfd.Secrets`, echo source only on the globals
   owner); stream `bfd.WatchEvents` into agent events (`Wiring.Publish`, EventKind 22). FRR `bfd` section (`bfd` block with `peer …
   interface …` for protocol-attached BFD, profiles) and the `redistribute` package: a state reader that builds the **redistribution
   matrix** (source → target, route map, route counts from scoped per-protocol summaries — never a full-RIB dump).
3. **API**: pointer routes; `GET /api/v1/state/routing/bfd/sessions` (VPP + FRR sessions, state, timers, last flap), `GET /api/v1/state/routing/redistribution`
   (matrix), `GET /api/v1/state/routing/policy/{name}/hits` if FRR exposes counters (else omit and say so). RPCs `BfdState`, `RedistributionMatrix`.
4. **UI**: BFD page (sessions grid with live up/down), redistribution matrix (protocols × protocols grid, click → edit that `redistribute` entry),
   route-policy UX over `routing.policy`: reorder + where-used links **on top of P12's editors** (import its components; never a second
   editor — if they are not reusable, questions file); en + fa; screenshot.
5. **Docs**: `docs/user/routing/bfd-redistribution.md` (VPP static-peer BFD, OSPF+BFD via FRR, redistribute static→OSPF with a route map,
   the port rule above).

## Acceptance (paste the evidence)
- [ ] VPP BFD session to a netns peer (FRR bfdd in its own pathspace, `path: af_packet` rig): `vppctl show bfd sessions` Up; peer down → event in the API within 2 s
- [ ] Static route with `viaFrr: true` redistributed into OSPF appears at the netns OSPF peer; the same route is **not** programmed by the agent (ip_route_dump source) — D-072
- [ ] FRR bfdd for an OSPF peer over linux-cp: Up in a manager window with no VPP BFD session on the host — or netns↔netns bfdd + the reason (the port claim)
- [ ] Agent-restart simulation → sessions and FRR sections back within 30 s; rollback removes sessions (Retrieve) and FRR blocks
- [ ] Same (interface, peer) as VPP session and OSPF `bfd: true` → 400 problem+json with `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
BGP and the route-map/prefix-list FRR sections and their basic editors (P12); the `viaFrr` flag and its selector (F-vrf-static-ecmp);
OSPF/IS-IS/RIP sections and their own redistribute lines (F-ospf, F-isis-rip); static-route programming in VPP (F-vrf-static-ecmp); BFD
echo mode tuning beyond the global owner; BFD for VRRP (F-vrrp-config-sync); PBR (F-rpf-adl-pbr); redistributing `ospf6`/`ripng` into
other protocols (show them read-only in the matrix).

## Open questions to surface, not to decide silently
Which engine owns BFD on a box (proposed: FRR bfdd for protocol-attached, VPP for standalone/static peers — and, because VPP holds the
UDP port, never both for the same AF/hop type on one box; options: (a) validation error (b) warn and let FRR BFD stay down (c) route all
BFD through VPP and feed protocol state to FRR — not available without FRR/VPP glue); whether `ospf6`/`ripng` become redistribution
sources for the other protocols.
