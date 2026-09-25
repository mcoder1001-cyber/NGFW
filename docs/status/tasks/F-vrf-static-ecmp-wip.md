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
- 20:57 usage-limit stop; 22:40 salvage `bc83330` (config.e2e `actions answer 501` example: ping → reboot).
- 22:42 continued: branch ci.sh run 1 hit the D-127 guard flake; main's ci.sh copy: every step green, only its new deploy/vpp
  step failed (this branch's older harness, not touched here); 22:58 branch `tools/ci.sh --base main` → CI GATE PASSED (5m49s).
- 23:09 cleanup evidence (no w2 objects in VPP, vrx_w2 absent, rig down, NRestarts 1 unchanged); status doc final.
- left: nothing — waiting for the manager's merge (main merge + P08 dedupe is theirs, D-114/D-120).
- fix round 1 (review 4435fcf): 23:5x main merged (266d1dc, 4431c24; df67a8e as effective base); agent b7d45081, api/web
  5f619ae1, contract regen 9cdee9c8, questions Q14/Q15 8d20a853; e2e 53/53; 00:17 `tools/ci.sh --base main` → CI GATE
  PASSED; status "Fix round 1" section d2989170. Left: nothing in owned files (Q14 = manager/CLI tech debt).
