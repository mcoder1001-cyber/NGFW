# Task: F-ospf — OSPFv2 / OSPFv3 via FRR   (prepend 00-CONTEXT.md)

## Goal
OSPFv2 (IPv4) and OSPFv3 (IPv6) end to end in FAST MODE: schema → FRR `ospfd`/`ospf6d` section → routes into the **VPP FIB**
through linux-cp/linux-nl → API → UI → docs. Reference: TNSR "OSPF / OSPFv3"; FRR 10 ospfd/ospf6d; VPP `linux_cp` + `linux_nl`
(loaded on vrx-a since D-060). WBS D3.4 (OSPFv2, T1), D3.5 (OSPFv3, T2) in `plan/wbs.csv`.

## Inputs to read first
- `packages/schema/src/domains/routing.ts` — `routing.ospf{routerId, vrf, areas{<id>: {type normal|stub|nssa, noSummary}},
  interfaces{<if>: {area, cost, passive, networkType, helloIntervalSec, deadIntervalSec, priority, bfd}}, redistribute{…},
  defaultInformationOriginate}` exists (D-045 layout, D-070: area ids are strings, `ospfAreaNumber()` normalises `0`/`0.0.0.0`).
  **OSPFv3 has no model yet** → contract branch `contract/F-ospf` (additive): `routing.ospf6{routerId, vrf, areas{…}, interfaces{<if>:
  {area, cost, passive, networkType, hello/dead, priority}}, redistribute}` + auth for v2 (`interfaces.<if>.auth{type: md5|none,
  keyId, keyRef: "password/<name>"}` — D-051 secret refs, never inline) + matching proto; `docs/status/tasks/F-ospf-contract.md`.
- `apps/agent/internal/renderers/frr/README.md` + `section.go` (RF-1): your section is its own package, `frr.RegisterSection` from `init()`,
  order in 400–899, every user string through `frr.IfName/RouteIfName/VRFName/Description`, secrets only via `rc.Secret()` and
  redacted (D-072 second half); `frr.RegisterStateReader` / `RegisterPoller` for state and neighbour events; `frrtest` harness.
- `prompts/P12-frr-linuxcp.md` and P12's merged code (dep) — LCP pairs, `rc.MapInterface` (VPP name → Linux name), the netns FRR peer
  pattern on the veth rig and `/api/v1/state/routes?proto=` — reuse, do not re-implement. P12 owns BGP and the route-map/prefix-list sections.
- `docs/vpp-code-track.md` V1 (linux-cp/linux-nl edge cases: IPv6 RA on pairs, multi-VRF mapping, full-table churn) — document hits, no C.
- FRR docs: https://docs.frrouting.org/en/latest/ospfd.html, ospf6d.html

## Scope — build exactly this
Files you own: `apps/agent/internal/renderers/frr/ospf/**`, `docs/agent/renderers/frr-ospf.md`, `apps/agent/internal/agent/project_ospf*.go`,
`apps/api/src/features/ospf/**`, `apps/web/src/domains/routing/ospf/**`, `apps/web/src/locales/*/ospf.json`, `docs/user/routing/ospf.md`,
`test/topology/ospf/**`. Shared files: one-line appends only (app.module import, router/nav entry, agent section import).
1. **Schema** (on the contract branch): OSPF interface must exist and carry an address of the family; area referenced by an interface must
   exist in `areas` (or area `0` implied — decide and document); stub/NSSA not allowed for area 0; router id required when the VRF has no
   IPv4 address; `redistribute` route maps exist in `routing.policy.routeMaps`; `bfd: true` requires F-bfd-redistribution's FRR bfdd (warn only).
2. **Agent**: `ospf` and `ospf6` Sections rendering `router ospf [vrf]` / `router ospf6 [vrf]`, per-interface `ip ospf …`/`ipv6 ospf6 …`
   lines on the Linux-side names from `rc.MapInterface`, area types, passive, redistribute (own lines inside the router block),
   default-information originate, md5 auth via `rc.Secret`. State readers: `show ip ospf neighbor json`, `show ip ospf interface json`,
   `show ipv6 ospf6 neighbor json`; neighbour-state poller → events. Unit tests: golden renders + hostile names; `frrtest` integration
   with `Daemons: mgmtd, zebra, staticd, ospfd, ospf6d` in your slot pathspace — never the system FRR unit, never `/etc/frr`.
3. **API**: config via pointer routes; `GET /api/v1/state/routing/ospf/{neighbors,interfaces,database}` (LSDB paged), routes via
   P12's `/state/routes?proto=ospf`.
4. **UI**: Routing → OSPF page: global form, areas table, interfaces table (SchemaForm), neighbours grid with state chips and live status;
   OSPFv3 tab; en + fa; screenshot against the real endpoint in `docs/status/tasks/F-ospf.md`.
5. **Docs**: `docs/user/routing/ospf.md` (single-area + stub example, v3 example, CLI equivalent).

## Acceptance (paste the evidence)
- [ ] Two FRR ospfd peers in netns on the veth rig (`path: af_packet`) announce 50 prefixes each: the routes are in the **VPP FIB**
      (`vppctl show ip fib` / `ip_route_dump`), not only in `vtysh` — pasted; peer withdraws → gone from VPP within 10 s; same for OSPFv3 (`show ip6 fib`)
- [ ] `show ip ospf neighbor` Full on both; UI neighbours grid shows the same
- [ ] Agent-restart simulation → FRR config re-rendered, adjacencies and VPP routes back within 30 s without API calls (log excerpt)
- [ ] Rollback removes `router ospf` (rendered file + `vtysh show running-config`) and the OSPF routes from VPP (Retrieve)
- [ ] Interface referencing an undefined area / area 0 as stub → 400 problem+json with `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
BGP, route-map / prefix-list sections (P12); IS-IS, RIP (F-isis-rip); BFD sessions and the redistribution matrix UX (F-bfd-redistribution);
static routes (F-vrf-static-ecmp); LCP pair management (P12); OSPF virtual links, sham links, TE/segment-routing extensions, graceful restart helper tuning,
multi-instance OSPF; VRRP; any VPP restart (D-012).

## Open questions to surface, not to decide silently
Whether OSPFv3 gets its own `routing.ospf6` key (proposed) or a family switch inside `routing.ospf`; OSPF auth key storage kind (`password/…`).
