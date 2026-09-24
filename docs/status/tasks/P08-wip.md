# P08 — WIP log (vertical slice: interfaces end to end)

Slot 1 (`w1`, API 3100, web 5100, metrics 9111, `/run/vrx-test/w1/agent.sock`, DB `vrx_w1`, tables 1000–1999,
rig 10.1.{1,2}.0/24). Runs directly on the host (no wt.sh). NRestarts at start: 5.

## Plan (work order from the manager)
1. [x] Agent wiring: `internal/subsystems` (registry + persisted stores), `internal/desired` (interfaces builder:
       alias objects, creators, attributes, sub-interfaces, descriptions), IfRef = alias, DHCP Reconnected hook
2. [x] Retrieve-backed `/state/interfaces` (additive contract: `RetrieveResponse.interface_status`) + counters endpoint
3. [x] Topology test `test/topology/interfaces` (API commit → V19 check → ping → MTU 1400 DF → rollback; counters ±5%)
4. [x] Restart-safety test (stop agent, delete host-interfaces + addresses via binapi, start → ping within 30 s)
5. [x] UI list + drawer + sub-interfaces (en/fa)
6. [ ] Docs + screenshots, P08.md, vertical-slice.md, CI

## Log
- start: read context/ops/prompt/envelope/rules/LOG/DF-1/P05/P06/P07a/P07b/vpp-code-track.
- 05:32/05:39 (worker 1): agent wiring (a86d3f2), /state/interfaces + counters (bb71ddc), contracts 51b7c42/c02aa32.
- 07:16 manager salvaged test/topology/interfaces helpers (stack_test.go, vpp_test.go — no Test func yet) as de60fa8.
- 07:2x worker 2 (CONTINUE): merged main (DF-5); DF-5 WithBootStore/keyer wiring: Wiring.IPsecOptions/IKEv2Options
  (file BootStore + vpn-<owner>.key 0600, D-096/Q12) + unit test. Next: topology test, restart-safety, UI, docs.
- 07:27 VPP crash during my run 2 (NRestarts 5→6) — D-101/V24: af_packet delete with veth up; test now quiesces veths (peers(false)).
- 07:46 topology + restart-safety + cleanup PASS on slot 1, NRestarts 6→6 (log kept for P08.md).
- UI screen done (7b1a11d). Next: docs + screenshots, P08.md, vertical-slice.md, ci.sh. Pending: TD-3 ifsanitize.Release wiring after TD-3 merges.
- 07:50–08:00 screenshots (TestInterfacesScreenshots, prod build) → docs/user/interfaces; basics.md, vertical-slice.md, P08.md.
- 08:00 ci.sh #1 failed on agent lint (coretest G115/revive + my IKEv2Options comment) → fixed 390b409; merged main (F-startup-apply); ci.sh #3 running.
- 08:05 ci.sh #3 PASSED (after lint fix + main merge). 08:12 topology re-run green after merge (trace matching made run-unique; agent built in-test when no binary given).
