# Task: F-ospf — OSPFv2 / OSPFv3 via FRR   (prepend 00-CONTEXT.md)

> Refreshed 2026-09-24 on `task/prep-rest` against main, `task/P08`, `task/W-seed` and the P12 envelope. Your TASK ENVELOPE
> (`docs/status/tasks/F-ospf.envelope.md`) wins for branch, files, numbers, anchors and process.

## Goal
OSPFv2 (IPv4) and OSPFv3 (IPv6) end to end in FAST MODE: schema → FRR `ospfd`/`ospf6d` section → routes into the **VPP FIB**
through linux-cp/linux-nl → API → UI → docs. Reference: TNSR "OSPF / OSPFv3"; FRR 10 ospfd/ospf6d; VPP `linux_cp` + `linux_nl`
(loaded on vrx-a since D-060). WBS D3.4 (OSPFv2, T1), D3.5 (OSPFv3, T2) in `plan/wbs.csv`.

## Inputs to read first
- `packages/schema/src/domains/routing.ts` — `routing.ospf{routerId, vrf, areas{<id>: {type normal|stub|nssa, noSummary}},
  interfaces{<if>: {area, cost, passive, networkType, helloIntervalSec, deadIntervalSec, priority, bfd}}, redistribute{…},
  defaultInformationOriginate}` exists (D-045 layout, D-070: area ids are strings, `ospfAreaNumber()` normalises `0`/`0.0.0.0`).
  Semantic rules that already exist in `semantic/routing.ts` (test them, never re-add): `routing.ospf-area-exists` (an interface's area
  must be defined under `areas` — area 0 is not implied), `routing.interface-exists`, `routing.route-map-exists`.
  **OSPFv3 has no model yet** → additive contract (`routing.ospf6{routerId, vrf, areas{…}, interfaces{<if>: {area, cost, passive,
  networkType, hello/dead, priority}}, redistribute}` + auth for v2 (`interfaces.<if>.auth{type: md5|none, keyId, keyRef:
  "password/<name>"}` — D-051 secret refs, never inline) + matching proto; `docs/status/tasks/F-ospf-contract.md`). Commit it **first on
  your task branch** as separate `contract(schema): …` / `contract(proto): …` commits — never a `contract/` branch (workers create no
  branches). Numbers from `docs/status/wave-BC-numbers.md`: RoutingConfig 13 `ospf6`, OspfInterface 9 `auth`, EventKind 20.
- `apps/agent/internal/renderers/frr/README.md` + `section.go` (RF-1): your section is its own package, `frr.RegisterSection` from `init()`,
  order in 400–899 (`ospf` 440, `ospf6` 450), every user string through `frr.IfName/RouteIfName/VRFName/Description`, secrets only via
  `rc.Secret()` and redacted (D-072 second half); `frr.RegisterStateReader` / `RegisterPoller` for state and neighbour events; `frrtest` harness.
  Blank-import your package from your own `apps/agent/internal/subsystems/ospf.go` so its `init()` runs.
- `prompts/P12-frr-linuxcp.md` and P12's merged code + `docs/status/tasks/P12.md` (dep) — the FRR singleton descriptor that renders every
  registered section, LCP pairs, the linux-cp mapper (`internal/lcpmap`, injected with `frr.WithInterfaceMapper`), the linux-nl test plan
  (slot kernel VRF; linux_nl listens in the root netns), the netns FRR peer pattern on the veth rig, P12's routing-state RPC, and the
  `proto` filter on F-vrf-static-ecmp's `/api/v1/state/routes` — reuse, do not re-implement. P12 owns BGP and the route-map/prefix-list sections.
- Framework gaps to expect (docs/status/wave-BC-numbers.md, seams S2/S3): RF-1 renders `interface X` blocks only for descriptions — use
  P12's per-interface hook if it added one, else render your own `interface X … exit` block and prove with `frrtest` that frr-reload's
  DryRun is empty after Apply; the "routing protocols … RF-1" warning in `projection.go` loses `ospf` only through P12's table (else the
  manager edits it at merge).
- `docs/vpp-code-track.md` V1 (linux-cp/linux-nl edge cases: IPv6 RA on pairs, multi-VRF mapping, full-table churn) — document hits, no C.
- FRR docs: https://docs.frrouting.org/en/latest/ospfd.html, ospf6d.html

## Scope — build exactly this
Files you own: see the envelope (`renderers/frr/ospf/**`, `subsystems/ospf*.go`, `desired/ospf*.go`, `agent/rpc_ospf*.go`, `ext/ospf*.ts`,
`semantic/ospf*.ts`, `features/ospf/**`, `domains/routing/ospf/**`, `ospf.json`, docs, `test/topology/ospf/**`). Shared files: one line under
your `// wave-BC: F-ospf` anchor only (app.module, router/nav, i18n, semantic index, proto).
1. **Schema** (contract commits): OSPF interface must exist and carry an address of the family; area referenced by an interface must
   exist in `areas` (existing rule — mirror it for `ospf6`); stub/NSSA not allowed for area 0; router id required when the VRF has no
   IPv4 address; `redistribute` route maps exist in `routing.policy.routeMaps` (existing rule — mirror for `ospf6`); `bfd: true` requires
   F-bfd-redistribution's FRR bfdd (warn only).
2. **Agent**: `ospf` and `ospf6` Sections rendering `router ospf [vrf]` / `router ospf6 [vrf]`, per-interface `ip ospf …`/`ipv6 ospf6 …`
   lines on the Linux-side names from `rc.MapInterface`, area types, passive, redistribute (own lines inside the router block),
   default-information originate, md5 auth via `rc.Secret`. State readers: `show ip ospf neighbor json`, `show ip ospf interface json`,
   `show ipv6 ospf6 neighbor json`; neighbour-state poller → events (EventKind 20). Unit tests: golden renders + hostile names; `frrtest` integration
   with `Daemons: mgmtd, zebra, staticd, ospfd, ospf6d` in your slot pathspace — never the system FRR unit, never `/etc/frr`.
   MD5 end to end waits for PENDING-secret-channel; build and test with a slot-local fixture resolver (keys ≤ 16 chars).
3. **API**: config via pointer routes; `GET /api/v1/state/routing/ospf/{neighbors,interfaces,database}` (LSDB paged), routes via
   `/state/routes?proto=ospf` (F-vrf-static-ecmp's FIB browser + P12's filter). State comes through P12's routing-state RPC when it serves
   registered readers by key, else your own `OspfState` RPC.
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
multi-instance OSPF; redistributing `ospf6` into other protocols; VRRP; any VPP restart (D-012).

## Open questions to surface, not to decide silently
OSPFv3 gets its own `routing.ospf6` key (decided default in wave-BC-numbers.md — surface it if you find a reason for a family switch
instead); OSPF auth key storage kind (`password/…`); whether linux-cp delivers 224.0.0.5/6 and ff02::5/6 to the tap (confirm on the host).
