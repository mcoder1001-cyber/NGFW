# P05 WIP log (agent core)

Slot 7 · prefix `w7` · tables 7000–7999 · loopbacks loop700–loop799 · addresses 10.7.0.0/16 · socket /run/vrx-test/w7/agent.sock · metrics 9171

## Status
- 2026-09-24 start (CONTINUE): previous worker left only the envelope commit; design done, implementation starting.

## Design (short)
- Core descriptors `internal/descriptors/core`: `interface.loopback`, `interface-ip` (addresses), `interface-ip.table` (VRF binding),
  `vrf` (key `vrf/<id>`, VPP table name `<owner>:<vrf name>`), `ip.route` (key `ip.route/<table>/<prefix>`, owner table in state dir).
- Alias keys `interface/<name>` (loopback) so DF-2..6 dependencies on `interface/<name>` resolve.
- Scheduler `internal/scheduler/reconciler.go`: plan/apply/verify/rollback journal.
- Agent `internal/agent`: projection DesiredState ⇄ KVs, persistence (desired.pb + confirmed.pb + agent-state.json), confirm timer,
  resync on start/reconnect, events bus, gRPC server, metrics (hand-written exposition).

## Next
