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

The prompt's optional rule, decided **reject** (its default; the alternative "merge in the builder" would make the
applied state differ from the document, so Retrieve could never equal desired). New file
`packages/schema/src/semantic/nat44-ed-sessions.ts` (`nat44EdSessionsValidators`), rule id
`nat.nat44-ed-sessions-adjacent-pools`: two NAT44 range pools of one twice-NAT class and tenant VRF that touch
(end + 1 = start, either order; a missing `vrf` equals `"default"`) → finding at `/nat/pools/<later>/range`. Overlaps stay
`nat.pools-valid`'s, reversed ranges too (no double finding). Shared hunk C2: one import and one spread line under the
anchors of `packages/schema/src/semantic/index.ts`. Tests: `nat44-ed-sessions.test.ts` (the rule, and the existing P02b
rules on the projected ED paths: inside/outside disjoint, overlapping pools, external port without protocol).

## Fix round 1: `contract(proto): nat sessions — comments` (comment-only)

Review L1/M1/H1: `NatSession.external_nat_*` says VPP reports "0.0.0.0"/0 without twice-NAT; `NatSessionKillAction.
external_*` is the remote end as the inside host addresses it (`external_nat_*` of a twice-NAT session);
`NatSessionsResponse.truncated` / `NatSummaryResponse.truncated` name the per-call caps; `NatSummary` and its
`retrieved_at` mention the 30-s cache. No field, number or type changed; `buf lint` clean; generated code regenerated.
