# P08 — WIP log (vertical slice: interfaces end to end)

Slot 1 (`w1`, API 3100, web 5100, metrics 9111, `/run/vrx-test/w1/agent.sock`, DB `vrx_w1`, tables 1000–1999,
rig 10.1.{1,2}.0/24). Runs directly on the host (no wt.sh). NRestarts at start: 5.

## Plan (work order from the manager)
1. [x] Agent wiring: `internal/subsystems` (registry + persisted stores), `internal/desired` (interfaces builder:
       alias objects, creators, attributes, sub-interfaces, descriptions), IfRef = alias, DHCP Reconnected hook
2. [x] Retrieve-backed `/state/interfaces` (additive contract: `RetrieveResponse.interface_status`) + counters endpoint
3. [ ] Topology test `test/topology/interfaces` (API commit → V19 check → ping → MTU 1400 DF → rollback; counters ±5%)
4. [ ] Restart-safety test (stop agent, delete host-interfaces + addresses via binapi, start → ping within 30 s)
5. [ ] UI list + drawer + sub-interfaces (en/fa)
6. [ ] Docs + screenshots, P08.md, vertical-slice.md, CI

## Log
- start: read context/ops/prompt/envelope/rules/LOG/DF-1/P05/P06/P07a/P07b/vpp-code-track.
- 05:32/05:39 (worker 1): agent wiring (a86d3f2), /state/interfaces + counters (bb71ddc), contracts 51b7c42/c02aa32.
- 07:16 manager salvaged test/topology/interfaces helpers (stack_test.go, vpp_test.go — no Test func yet) as de60fa8.
- 07:2x worker 2 (CONTINUE): merged main (DF-5); DF-5 WithBootStore/keyer wiring: Wiring.IPsecOptions/IKEv2Options
  (file BootStore + vpn-<owner>.key 0600, D-096/Q12) + unit test. Next: topology test, restart-safety, UI, docs.
