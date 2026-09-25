# F-object-model — contract change: `FqdnObjectState` (additive)

Branch `task/F-object-model` (P08 pattern: the contract commit sits on the task branch, no own branch). Commit subject
`contract(proto): FqdnObjectState`.

## What
- `packages/proto/vrx/v1/dataplane.proto`
  - `service Dataplane`: `rpc FqdnObjectState(FqdnObjectStateRequest) returns (FqdnObjectStateResponse);` under the
    `// wave-A: F-object-model` anchor (framed by blank lines, C5).
  - `// ----- F-object-model -----` section: `FqdnObjectStateRequest {names = 1, owner = 2}`,
    `FqdnObjectStateResponse {objects = 1, owner = 2, retrieved_at = 3}`,
    `FqdnObjectState {name = 1, fqdn = 2, addresses = 3, last_resolved = 4, next_refresh = 5, error = 6, failures = 7}`.
  - Names follow the prefix rule (§0.5): everything starts with `FqdnObject`. No field is added to an existing message,
    so no §2 number is used. **EventKind 11 is not taken** (see questions Q1): FQDN changes are state only.
- Regenerated (C7, never hand-edited): `apps/agent/gen/vrx/v1/dataplane{,_grpc}.pb.go`, `packages/proto/gen/ts/vrx/v1/dataplane.ts`.
- `apps/api/src/testing/fake-agent.ts` (P5): the UNIMPLEMENTED stub handler under the anchor (the exhaustive `DataplaneServer`
  type would not compile without it). The task's real fake behaviour replaces that one line later (`features/object-model/fake.ts`).
- `docs/contracts/proto.md` §11 (C6): `### F-object-model: FqdnObjectState`.

## Why
The API can get FQDN resolution results from the agent only through a read-only RPC (P08's live-state pattern);
`Retrieve` must stay configuration-only (proto.md §5), so the resolver's addresses, timestamps and errors never appear
in the `objects` domain it returns.

## Not changed
- No config field (schema/proto `ObjectsConfig` 8–9 unused). The refresh interval is an agent setting
  (`VRX_OBJECTS_FQDN_REFRESH_SEC`, clamped to 30–3600 s), because `packages/schema/src/domains/objects.ts` is P02b's
  file without a C1 anchor (envelope: must not touch) — see questions Q3.
- `vrx.model.*`: the agent-local `objects.*` descriptors use the `vrx.v1` messages themselves (a single-entry
  `ObjectsConfig` per object, see `docs/agent/objects.md`), so no agent-internal model package is needed.

## Compatibility
Additive only: a new RPC and three new messages. `buf lint` clean; `buf breaking` against main has nothing to report
(no existing element changed). An older agent answers UNIMPLEMENTED, which the API maps to 501.
