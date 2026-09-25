# P12 WIP (slot 8, started 2026-09-24 23:16 +03:30, time box 24 h)

## Plan
1. [x] contract: schema (`interfaces.<n>.lcp`, `static[].tag`) + proto (Interface 22, StaticRoute 8, EventKind 14/15,
       `RoutingState` rpc) + gen + P12-contract.md
2. [x] agent: frr/policy + frr/bgp sections, S2 interface-lines hook, lcpmap, lcp section (tap addresses), frr.config
       singleton descriptor, desired/{bgp,lcp}, subsystems/{frr,lcp}, projection S3 table, rpc_routing (RoutingState),
       event publisher (EventKind 14/15), frrtest multi-instance
3. [x] unit tests + golden files
4. [x] frrtest integration (BGP between slot instances, no VPP)
5. [x] API: features/bgp (state endpoints), /state/routes FRR annotation + proto filter, client/fake/bus
6. [x] UI: routing/bgp screen (global, neighbours live, prefix lists, route maps, redistribution), en/fa
7. [x] docs: user/routing/bgp.md, agent/renderers/frr-bgp.md, frr.md, descriptors/lcp.md
8. [x] topology test test/topology/bgp (T1; T2 opt-in)
9. [~] CI (gitleaks false positive Q16, svs/TD-11b Q15), [x] P12.md

## Log
- 23:16 read context, envelope, framework, DF-8, TD-8, F-vrf-static-ecmp
- 23:40 P12-questions.md written (linux-nl plan before any host run)
- 23:50 contract commits (schema, proto)
- 00:10 bgp/policy sections + live frrtest test green; agent wiring; unit tests
- 00:15–03:40 usage-limit stop
- 03:48 merge main (TD-8 seams); coordination notes
- 04:04 API feature + e2e green; 04:18 web screen
- 04:27 topology run 1: sessions Established over linux-cp in 3.7 s, then VPP SIGSEGV in dns_plugin (not P12) —
  NRestarts 1→2, host runs stopped (Q13)
- 04:38 CI: gitleaks false positive (Q16), fixed forward; TD-11b merged → svs undeclared (Q15, F-vrf's)
- 04:44 topology run 4 PASS (link down 400 ms), run 5 PASS (cleanup fixed), 04:48 run 6 PASS (tap addresses kept)
- 04:57 screenshots en/fa (API + fake agent), stack stopped by PID, DB dropped, Valkey keys removed
