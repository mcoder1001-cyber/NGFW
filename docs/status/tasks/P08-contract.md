# P08 — contract changes (additive, for manager review)

## `contract(proto)`: `rpc InterfaceState` + `InterfaceStateRequest/Response`, `InterfaceState`

**Why.** P08 §1/§2: `/api/v1/state/interfaces` must show sw_if_index, type, admin/link state, MTU, addresses and VRF
*from the data plane*, not from the config. `docs/contracts/proto.md` §5 forbids read-only status (link state,
sw_if_index) in `RetrieveResponse` ("those come from StreamStats/StreamEvents/state RPCs") and no state RPC existed.

**What.** One new unary RPC on `vrx.v1.Dataplane` and three new messages (field numbers fresh, nothing renamed or
renumbered, `buf lint` clean, `buf breaking` FILE-level additive). Go + TS stubs regenerated with `packages/proto/gen.sh`.
`docs/contracts/proto.md` §8a documents it. Nothing under `DesiredState` changed — the schema⊆proto drift guard is
untouched.

**Options considered.** (a) status fields on `RetrieveResponse` — contradicts §5 and mixes config with status;
(b) a state RPC — chosen, §5 already names "state RPCs" as the place; (c) derive link/admin state in the API from
`StreamEvents` + interface names — no snapshot exists (events are transitions only), and the type/sw_if_index would be
guessed. Compatibility: older API builds never call it; an older agent answers `UNIMPLEMENTED` (the API then serves
the list from Retrieve alone with `state: null`).

## `contract(api-client)`: regenerated OpenAPI client
`GET /api/v1/state/interfaces` gains typed `items[]` (live state + config + `hasPendingChange`),
`GET /api/v1/state/interfaces/{name}/counters` is new. Additive.
