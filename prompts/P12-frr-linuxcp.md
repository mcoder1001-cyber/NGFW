# Task P12 — FRR + linux-cp framework, BGP basic   (prepend 00-CONTEXT.md)

## Goal
Dynamic routing: FRR runs on the Linux side, VPP's `linux-cp` plugin mirrors VPP
interfaces into Linux and `linux-nl` syncs the kernel FIB into VPP. Deliver the framework
plus BGP with neighbours, address-families, prefix-lists and route-maps. OSPF/BFD are
separate F-tasks that reuse this framework.

## Read first
VPP 26.06 `linux-cp` and `linux-nl` plugin docs; FRR 10 docs (`frr-reload.py`, JSON
show commands); `docs/01-architecture.md` AD-2; `apps/agent/internal/renderers/frr/README.md` (RF-1 — the framework you plug into;
P11 runs in parallel and is not a pattern source).

## Precondition (checked 2026-09-24 — satisfied)
`linux_cp_plugin.so` and `linux_nl_plugin.so` are **loaded** since D-060 (startup.conf `plugins { … { enable } }`, one manager restart);
this task is no longer parked. `handover` stays pending: nobody edits `/etc/vpp/startup.conf` or restarts VPP. Host facts that shape the
test: there is **no** `linux-cp { default netns … }` section, so `linux_nl` listens in the namespace that was the lcp default netns when
its netlink socket opened (`lcp_nl.c` `lcp_nl_open_socket`) — today the **root** namespace; `lcp.default-netns` is VPP-global (D-071,
globals owner only). FRR 10.7.1 is installed, `frr.service` disabled — never enable or start it.

## Already built — use, do not rebuild (prep-waveA check, 2026-09-24, main + task/P08)
- **RF-1** FRR framework `apps/agent/internal/renderers/frr/` + README ("Adding a protocol") + `docs/agent/renderers/frr.md`: frr.conf +
  vtysh.conf render, `vtysh -C -f` validate, `frr-reload.py --reload` apply with convergence check, `DryRun`, fixed-argv runner
  (ALLOWLIST rows exist), state readers + 1 Hz pollers, `RenderContext.Secret` (D-051/D-072 redaction), `MapInterface` +
  `WithInterfaceMapper` (default `NoMapper` until you inject the linux-cp mapping), D-072 `StaticOwnedByFRR` / `RegisterStaticSelector`,
  and the `frrtest` harness (slot pathspace, own netns, never `/etc/frr`, never the system unit). Protocol sections plug in from their own
  package (`internal/renderers/frr/bgp`, …) — they never edit the framework files.
- **DF-8** `apps/agent/internal/descriptors/lcp/` + `docs/agent/descriptors/lcp.md`: `lcp.itf-pair` (`lcp_itf_pair_add_del_v3`, VPP-side
  logical name, host tap name ≤ 15, netns), `lcp.default-netns` (globals only), replace helpers (act on **every** owner's pairs — never on
  the shared host). TD-3 (merged) added the interface sanitizer to its Create.
- **P02a** schema `packages/schema/src/domains/routing.ts`: `routing.bgp` (asn, routerId, vrf, peerGroups, neighbors keyed by address with
  `passwordRef`, afi, networks, redistribute, gracefulRestart, ebgpRequiresPolicy) and `routing.policy` (prefix lists / route maps as
  objects, D-045/D-070) + `semantic/routing.ts` rules; proto `BgpConfig`, `RoutingPolicy`.
- **P08** wiring patterns (`internal/desired/`, `subsystems.go`, `projection.go`); `Domains["routing"]` = core static routes today.
- Wave A: **F-vrf-static-ecmp** owns `GET /api/v1/state/routes` (paged FIB browser, `ListRoutes` RPC, `source` filter), adds
  `routing.static[].viaFrr` (the D-072 flag) and **registers** the D-072 selector for it (`frr.RegisterStaticSelector` panics on a second
  call) — you render `viaFrr` routes in FRR (staticd), you do not register the selector again.

## Build exactly this
1. **Lab**: FRR (test instances via `frrtest`) next to VPP with `linux-cp` + `linux-nl` loaded; the agent creates LCP pairs through DF-8's
   `lcp.itf-pair` descriptor for every routed VPP interface (host tap name from the logical name), syncs MTU/state both ways, handles the
   netns. The config has **no** LCP leaf yet — model it on the contract branch (additive, D-061 style) and say which option you chose.
2. **Contract** (additive only — the BGP/policy model exists, see above): the routing state message (RF-1 Q2), event kinds for routing
   changes / neighbour state (RF-1 Q3), `StaticRoute.tag` (RF-1 Q1), the LCP pair leaf. Numbers come from the manager, never "next free".
3. **FRR sections** (framework exists — RF-1): package `internal/renderers/frr/bgp` (+ `internal/renderers/frr/policy` for prefix lists /
   route maps, reused by F-ospf …): `router bgp` + neighbours + AFs + policy lines through `rc.Secret`/validators, state readers
   (`show bgp summary json`, `show bgp ipv4|ipv6 unicast json`), a 1 Hz neighbour poller; inject the linux-cp interface mapper
   (`frr.WithInterfaceMapper`); `viaFrr` static routes render through RF-1's framework `static` section (F-vrf-static-ecmp registered
   the selector). Fixed argv only (the RF-1 runner).
4. **FIB verification**: routes learned by FRR must appear in VPP (`ip_route_dump` via binapi). `GET /api/v1/state/routes?vrf=&proto=bgp`
   is F-vrf-static-ecmp's FIB browser: add the FRR-source annotation / `proto` filter through it (shared hunk), do not build a second one.
5. **API/UI**: BGP global + neighbours (state, uptime, prefixes rx/tx, flaps), prefix-list and
   route-map editors (SchemaForm with array widgets), redistribution toggles; en+fa.
6. **Topology test** (single host): VRX ↔ two FRR instances running in network namespaces on the veth rig (`frrtest` harness: `-N <prefix>`
   pathspace, own config dirs — never the system FRR unit), eBGP, each announcing 100 prefixes. Before the first host run write the
   linux-nl plan into `P12-questions.md`: the VRX-side FRR's kernel routes reach VPP only from the namespace `linux_nl` listens in (root
   today) — BGP routes must never land in the root namespace's main table (management path on ens192): use a slot kernel VRF/table in
   `<N>000–<N>999`, or ask the manager for a globals window that sets `lcp default netns`; after
   commit: `vppctl show ip fib` contains all 200; withdraw on peer → gone from VPP within 5 s;
   apply route-map denying half → 100 remain; agent-restart simulation (and, after handover, the manager's `kill -9 vpp`) → LCP pairs +
   BGP sessions recover and FIB is repopulated without API involvement; rollback of the whole BGP config → sessions torn
   down cleanly, FIB empty of BGP routes.

## Acceptance
- [ ] Routes present in **VPP** FIB, not only in `vtysh` (this is the whole point)
- [ ] `frr-reload.py` used for changes; FRR never restarted on a config edit
- [ ] Link down on a VPP interface propagates to the Linux pair within 1 s and BGP notices

## Out of scope
OSPF, IS-IS, RIP, BFD (F-tasks), full-table performance, VPNv4, BGP over IPsec. Use, do not rebuild: the RF-1 framework and `frrtest`,
DF-8's lcp descriptors, P02a's routing schema, F-vrf-static-ecmp's FIB browser / `ListRoutes`. `lcp_itf_pair_replace_begin/end` and
`lcp default netns` changes on the shared host (VPP-global).
