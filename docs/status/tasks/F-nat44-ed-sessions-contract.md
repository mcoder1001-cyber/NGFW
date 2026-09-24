# F-nat44-ed-sessions — contract changes

Committed on `task/F-nat44-ed-sessions` itself (envelope: no `contract/<id>` branch, P08 pattern). Additive only; no
existing field renamed, renumbered or reshaped. Numbers from `docs/status/wave-A-hotspots.md` §2.

## `contract(proto): nat sessions`

`packages/proto/vrx/v1/dataplane.proto`:

| where | what | number |
|---|---|---|
| `service Dataplane`, under `// wave-A: F-nat44-ed-sessions` (blank-line framed) | `rpc NatSessions(NatSessionsRequest) returns (NatSessionsResponse)`, `rpc NatSummary(NatSummaryRequest) returns (NatSummaryResponse)` | — |
| `ActionRequest.action` oneof, under the same anchor | `NatSessionKillAction nat_session_kill` | **5** (§2 allocation) |
| `// ----- F-nat44-ed-sessions -----` section | `NatSessionFilter`, `NatSessionsRequest`, `NatSession`, `NatSessionsResponse`, `NatSummaryRequest`, `NatPoolUsage`, `NatSummaryResponse`, `NatSessionKillAction` (own numbers from 1) | — |

Not used: `NatConfig` 25–26 (no config gap: `NatConfig` already mirrors every NAT44 leaf the builder projects).

Semantics: `docs/contracts/proto.md` §11 "F-nat44-ed-sessions: NatSessions". Bounded messages: `limit` ≤ 1000
(`INVALID_ARGUMENT` above), so no response carries the whole table.

Regenerated (never hand-edited): `apps/agent/gen/vrx/v1/*`, `packages/proto/gen/ts/vrx/v1/dataplane.ts` (`packages/proto/gen.sh`).
Shared hunks: `apps/api/src/testing/fake-agent.ts` (P5: the two `UNIMPLEMENTED` stubs under the anchor, replaced by the
feature's `fake.ts` wiring in the feature commit), `docs/contracts/proto.md` (C6 section).

Coordination: F-nat44-ei-64-66-nptv6 appends its EI variant to `NatSessions*` / `NatSessionKillAction` with new field
numbers; F-det44-map-dslite-cnat uses its own RPCs (wave-BC-numbers.md), never these messages.

## `contract(schema): nat adjacent pools`

See the section below once committed.
