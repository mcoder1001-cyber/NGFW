# Task P03 — gRPC contract agent↔api (contract)   (prepend 00-CONTEXT.md)

## Goal
Define `packages/proto/vrx/dataplane.proto` — the frozen interface between `vrx-api`
(Node) and `vrx-agent` (Go). Mirror the schema from P02 for the modelled domains only.

## Read first
`docs/04-api-datamodel.md` (gRPC sketch), the P02 schema in `packages/schema/src`.

## Build exactly this
```proto
service Dataplane {
  rpc Apply(ApplyRequest) returns (ApplyResponse);          // desired state + txn id + confirm timeout
  rpc Retrieve(RetrieveRequest) returns (DesiredState);     // actual state dumped from VPP/daemons
  rpc DryRun(ApplyRequest) returns (ValidationReport);
  rpc StreamStats(StatsRequest) returns (stream StatsBatch);
  rpc StreamEvents(EventRequest) returns (stream Event);
  rpc Action(ActionRequest) returns (stream ActionOutput);  // ping, traceroute, capture
  rpc Health(HealthRequest) returns (HealthResponse);
}
```
- `DesiredState` contains `Interfaces`, `Vrfs`, `StaticRoutes`, `Dataplane` messages that
  correspond 1:1 to the P02 Zod models (same field names in snake_case). Other domains:
  reserve field numbers with comments, do not define yet.
- `ApplyRequest{ txn_id, desired_state, subsystems[] (empty = all), confirm_timeout_sec }`.
- `ApplyResponse{ txn_id, status (APPLIED|FAILED|ROLLED_BACK), per-object results with
  code, message, pointer }`.
- `ValidationReport{ errors[]{pointer,message,severity} }`.
- `StatsBatch{ ts, interface_counters[]{name, rx_packets, rx_bytes, tx_packets, tx_bytes,
  drops, errors}, worker_cpu[] }` — designed for 1 Hz, ≤ 1000 interfaces.
- `Event{ ts, kind (LINK_UP|LINK_DOWN|RECONCILE_START|RECONCILE_DONE|ERROR), interface?, message }`.
- `ActionRequest{ oneof: Ping{target, vrf, count, size}, Traceroute{...}, Capture{interface, bpf, max_packets, seconds} }`;
  `ActionOutput{ oneof: line, pcap_chunk, done{summary} }`.
- Field-level comments for every field. `buf lint` with the DEFAULT rule set, `buf breaking` against `main`.
- Generate Go into `apps/agent/gen/vrx/v1` and TS into `packages/proto/gen/ts`.
- `docs/contracts/proto.md`: RPC semantics — idempotency of Apply, what Retrieve must include,
  ordering guarantees of streams, the confirm-timeout self-revert behaviour.

## Acceptance
- [ ] `buf lint` and `buf breaking --against .git#branch=main` pass
- [ ] `pnpm gen` regenerates both stubs deterministically
- [ ] A Go and a TS compile-check that constructs a `DesiredState` from the P02 example documents

## Out of scope
Implementing the service. NAT/ACL/VPN messages (reserved numbers only).
