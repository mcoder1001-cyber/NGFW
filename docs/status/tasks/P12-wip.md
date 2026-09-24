# P12 WIP (slot 8, started 2026-09-24 23:16 +03:30, time box 24 h)

## Plan
1. [ ] contract: schema (`interfaces.<n>.lcp`, `static[].tag`) + proto (Interface 22, StaticRoute 8, EventKind 14/15,
       `RoutingState` rpc) + gen + P12-contract.md
2. [ ] agent: frr/policy + frr/bgp sections, S2 interface-lines hook, lcpmap, lcp section (tap addresses), frr.config
       singleton descriptor, desired/{bgp,lcp}, subsystems/{frr,lcp}, projection S3 table, rpc_routing (RoutingState),
       event publisher (EventKind 14/15), frrtest multi-instance
3. [ ] unit tests + golden files
4. [ ] frrtest integration (BGP between slot instances, no VPP)
5. [ ] API: features/bgp (state endpoints), /state/routes FRR annotation + proto filter, client/fake/bus
6. [ ] UI: routing/bgp screen (global, neighbours live, prefix lists, route maps, redistribution), en/fa
7. [ ] docs: user/routing/bgp.md, agent/renderers/frr-bgp.md, frr.md, descriptors/lcp.md
8. [ ] topology test test/topology/bgp (T1; T2 opt-in)
9. [ ] CI green, P12.md

## Log
- 23:16 read context, envelope, framework, DF-8, TD-8, F-vrf-static-ecmp
- 23:40 P12-questions.md written (linux-nl plan before any host run)
