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
`GET /api/v1/state/interfaces/{name}/counters` is new.

`GET /api/v1/state/interfaces` — **correction (fix round 1, D-105).** The first version of this file called the change
"additive". It was not: P08 had moved `items[].config` from the Retrieve view to the running configuration (and put
the Retrieve view into a new `actual`), which silently changed what `vrx show interfaces` prints. Fix round 1 restores
the pre-P08 meaning and makes the change additive:

| field | before P08 | P08 (round 0) | now |
|---|---|---|---|
| `items[].config` | Retrieve view of the interface (non-null) | running configuration (nullable) | **Retrieve view again**; `null` only on rows P08 added (a live interface the agent does not manage, a configured one VPP does not have yet, or one only in the candidate) |
| `items[].running` | – | – | **new**: the running configuration of this (sub-)interface, `null` when not configured |
| `items[].actual` | – | Retrieve view | **removed** (never released; it duplicated `config`) |
| `items[].counters` | InterfaceCounters or `null` | same | same |
| `items[].name/kind/parent/state/hasPendingChange` | only `name` | new | new |
| rows | interfaces in Retrieve (sub-interfaces nested in `config.subinterfaces`) | + live-only, configured-only and one row per sub-interface (`<parent>.<id>`) | same as round 0 |

Every row the old endpoint returned is still returned with the same `name`, `config` and `counters`; the new rows and
fields are additions. The CLI reads `name/config/counters` (`apps/cli/internal/cli/cmd_op.go`); its operations table is
regenerated (`State_counters`, new summary). **Fix round 2 (re-review R1, D-118):** the new rows reached the CLI's text
output — `vrx show interfaces <candidate-only name>` exited 0 with an empty body instead of 5 "not in the data plane".
The CLI now skips items whose `config` is `null` in the table, the name lookup and completion, which restores its
pre-P08 output exactly (`TestShowInterfacesListsOnlyRetrievedRows`, P08 response shape); `--json` prints the API's
answer unchanged, as before. The `config` description names candidate-only rows (client regenerated). Consumers updated in the same round: the web screen
(`InterfacesPage.tsx`, `model.ts`: fall back to `running`), `apps/api/test/e2e/interfaces.e2e.test.ts` and
`test/topology/interfaces` (read the Retrieve view from `config`).
