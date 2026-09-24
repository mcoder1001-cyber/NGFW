# P08 — WIP log (vertical slice: interfaces end to end)

Slot 1 (`w1`, API 3100, web 5100, metrics 9111, `/run/vrx-test/w1/agent.sock`, DB `vrx_w1`, tables 1000–1999,
rig 10.1.{1,2}.0/24). Runs directly on the host (no wt.sh). NRestarts at start: 5.

## Plan (work order from the manager)
1. [ ] Agent wiring: `internal/subsystems` (registry + persisted stores), `internal/desired` (interfaces builder:
       alias objects, creators, attributes, sub-interfaces, descriptions), IfRef = alias, DHCP Reconnected hook
2. [ ] Retrieve-backed `/state/interfaces` (additive contract: `RetrieveResponse.interface_status`) + counters endpoint
3. [ ] Topology test `test/topology/interfaces` (API commit → V19 check → ping → MTU 1400 DF → rollback; counters ±5%)
4. [ ] Restart-safety test (stop agent, delete host-interfaces + addresses via binapi, start → ping within 30 s)
5. [ ] UI list + drawer + sub-interfaces (en/fa)
6. [ ] Docs + screenshots, P08.md, vertical-slice.md, CI

## Log
- start: read context/ops/prompt/envelope/rules/LOG/DF-1/P05/P06/P07a/P07b/vpp-code-track.
