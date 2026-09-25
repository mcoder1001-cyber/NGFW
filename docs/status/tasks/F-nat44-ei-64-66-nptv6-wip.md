# F-nat44-ei-64-66-nptv6 — WIP log

- 22:40 manager salvage commit 5a29084 (envelope only); session resumed 23:00.
- 23:05 `contract(proto): nat session variants` (651d620) + comment clarification (c8828ae).
- 23:15 agent: builders/assemblers (desired/{nat44ei,nat64,nat66,nptv6}.go), descriptors/npt66 (write-only), coretest
  models, subsystems registration; ED-owned tests adapted (nat_test.go, rpc_nat44_ed_test.go, service_test.go).
- 23:25 agent state: actions/nat44-ei-64-66-nptv6, rpc_nat44_ei.go, variant dispatch hunks; tests green, lint 0.
- 23:29 host: `TestNpt66OnHost` — first npt66 messages on this VPP: 3 adds → 1 binding, update in place, delete;
  NRestarts 1 → 1 (log /root/ngfw-wt/logs/F-nat44-ei-64-66-nptv6-npt66-host.log).
- 23:37 API feature + e2e (6/6) + api-client / CLI table regenerated.
- next: web tabs (EI, NAT64, NAT66, NPTv6), docs, topology test on the af_packet rig (EI / NAT64 / NPTv6 packets,
  NAT66 mapping, restart, rollback), screenshots, CI.
- 23:40–00:32 topology runs 1–5: EI port forward pool rule (builder), IPv6 rig scope, IPv6 capture filter, NAT64
  tenant VRF on the inside only; run 5: NAT64 end to end, API ports wrong → VPP st_details defect (agent correction).
- 00:57 second usage-limit stop; 03:40 resumed (CONTINUE notes: TD-8 seams, TD-11b, D-132, WEB-1).
- 03:45 merged ED fix round (base), adapted pager API; D-132 walks serialised + 30-s refresh; WEB-1.
- 03:58–04:02 runs 7–9: leftover slot-VRF route (from run 5) removed; nat64 FIB lock leak → VRF kept; run 9 PASS.
- 04:19 screenshots (en + fa/RTL). 04:25 docs, example, CI (main's ci.sh: packet-trace ban trips on P08's old
  test/topology/interfaces copy inherited from the base; branch quick gate run separately).
