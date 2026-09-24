# F-vrf-static-ecmp — WIP log

- 17:27 start (slot 2, base W-seed@8b7558e); 18:05 merged task/W-seed@df67a8e (manager A1 safety update: TD-5 + D-113 rig fix).
- contract: `contract(schema): vrfs source-select, next-hop vrf, viaFrr` (0435d87), `contract(proto): ListRoutes` (4fe7ae3).
- agent: core next-hop table (RoutePath.next_hop_table), VRF Retrieve skips svs tables, svs descriptors + fake model,
  desired builder/assembler, wiring (selector sync.Once, Domains), projection hunks, ListRoutes lister, ping/traceroute.
- host: svs/core host test PASS; 100k FIB test PASS (govpp drop found → 64k-reply stream; V15 leak of the first cleanup
  removed, Q7); topology test (real agent + API + rig + screenshots) PASS 19:36.
- api: VrfStaticEcmpController (`/state/routes`, operationId State_routes kept), Action bridge, fake behaviour, e2e PASS.
- web: VRFs page, routing page (static routes + ECMP editor, FIB browser, ping), en/fa; merge-patch writes (Q-web: `%2F`).
- incidents: VPP crash 18:41 (D-126/D-128: `show trace`, not this task; Q7 has slot 2's timeline).
- left: CI (`tools/ci.sh --base main`), final status doc with evidence, cleanup.
