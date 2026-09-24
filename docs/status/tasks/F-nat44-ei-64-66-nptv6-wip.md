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
