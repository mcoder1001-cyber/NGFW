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

- 00:50 scheduler (plan/apply/verify/rollback, scope, recreate + dependent re-creation, write-only D-063, Normalizer,
  observe-only DeleteOnAbsence D-065), core descriptors + host integration test (green on VPP), fake VPP model,
  agent service/persistence/confirm/resync/events/stats/gRPC/metrics with unit tests — all green; main merged (0d1e33d).
- D-065 applied: loopback no longer provides `interface/<name>`; core refs via Env.IfRef (creator key for loopbacks
  until DF-1's alias descriptor is wired, then AliasInterfaceRef).

- 00:55 host integration (in-process + real binary kill -9), manual evidence with vrx-agentctl, VRF API-lock bug found
  by the loss simulation and fixed (Reapplier), main merged again (P02a proto sync: blackhole, routing.policy), CI GATE PASSED.
- CLOSED: final report docs/status/tasks/P05.md.

## Next (was)
- agent-level host integration test (apply doc → Retrieve == desired, vppctl evidence, two owners, confirm revert)
- process restart simulation (binary, kill -9 by PID, delete prefixed objects via binapi, restart → recreated)
- dev client cmd/vrx-agentctl for evidence; core README table; tools/ci.sh --base main; P05.md
