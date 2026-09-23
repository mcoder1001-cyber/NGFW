# P03b — contract change summary (`contract(proto)`) for the manager's review — gate for `contracts-v1`

Branch `task/P03b`, worktree `/root/ngfw-wt/P03b`, base `main@dfc2f90`. Evidence with pasted output:
`docs/status/tasks/P03b.md`. Semantics: `docs/contracts/proto.md` §1 "Sync state and the drift guard", §10.

## What changed in the contract

| change | kind | wire / API effect | why |
|---|---|---|---|
| `vrx.v1` — 68 field comments added (routing BGP/OSPF/IS-IS/RIP/BFD, management AAA/syslog, `ActionRequest` oneof members, `Health*` message comments); section headers point at the merged `packages/schema/src/domains/*.ts` instead of branch hashes; numbering header names the drift guard | comments only | none (generated Go/TS differ only in doc comments) | scope 2: P03's rule "every field commented" was broken by the group (a) sync; stale `task/P02b@ec0ccda` references |
| `vrx.v1.Redistribute` comment: the shared message's own-protocol key can never be set | comment | none | documents the one accepted drift (below) |
| **new** `vrx/model/acl/v1/acl.proto` — `vrx.model.acl.v1` (8 messages) | additive (new package) | Go only, not referenced by `vrx.v1` | scope 3, D-055: DF-4's structpb stand-in |
| **new** `vrx/model/nat/v1/*.proto` — `vrx.model.nat.v1` (9 files, 41 messages: 6 shared + 35 per plugin) | additive | Go only | scope 3, D-055: DF-3's structpb carrier (`task/DF-3@08d0af4`) |
| **new** `vrx/model/iface/v1/interface.proto` — `vrx.model.iface.v1` (8 messages + `RxModeKind`) | additive | Go only | scope 3, D-055: DF-1 interface attributes promoted from the descriptor-local proto |
| `buf.gen.yaml` restricted to `inputs: vrx/v1`; new `buf.gen.model.yaml` (Go only, `vrx/model`); `gen.sh` runs both | generation | `@ngfw/proto` TS output unchanged | the API never sees the object model |

No existing field was renamed, renumbered, retyped or removed by P03b. `buf breaking` vs `main`: exit 0.

## Renames / breaks since the P03 merge (`4ec9518`) — all from the D-061 group syncs, none from P03b

`buf breaking --against "../../.git#ref=4ec9518,subdir=packages/proto"` reports 8 lines (exit 100), four changes:

| break (buf) | commit | decision | justification |
|---|---|---|---|
| message `SystemNtp` deleted; `SystemConfig` field 4 `ntp` deleted (`reserved 4; reserved "ntp"`) | `57f216e` (P02a sync) | D-050 | NTP modelled once, in `services.ntp` (chrony renderer); pre-tag removal |
| `RoutingConfig` fields 2 `prefix_lists`, 3 `route_maps` deleted (+ their map-entry messages), `reserved 2, 3` + names; moved to `RoutingPolicy policy = 9` | `57f216e` (P02a sync) | D-045 | P12 layout `routing.policy.{prefixLists, routeMaps}`; `PrefixList`/`RouteMap` types kept |
| `HaConfig` field 1 `vrrp` (repeated) deleted, `reserved 1`; `map<string, VrrpInstance> vrrp = 2` | `6bccef5` (P02c sync) | D-053 | keyed collections are records; new number so old bytes are never misread as a map |
| `CnatConfig.Snat.PolicyInterface` field 2 `side` deleted (`reserved 2; reserved "side"`), `table` added | `1d4188d` (manager-applied P02b sync) | D-043 M3 | cnat `side` has no VPP counterpart; table selection instead |

Every removed number is reserved (names too, except `vrrp`, which lives on as field 2). None of these is reachable
by a consumer today (P05/P06 were built against the synced main). After `contracts-v1` the same kind of change is
always-PENDING (decision-policy #1).

## Guards added (so the tag stays true)

- `apps/agent/internal/contracttest/drift_test.go`: `TestSchemaProtoDrift` (generated JSON Schema of `RootConfig` ↔
  `DesiredState` descriptor, both directions, names + containers + scalar type/width + D-039 presence + D-040 secrets;
  868 scalar leaves, 191 messages, 0 findings, 4 accepted), `TestSchemaProtoDriftDetectsBreakage`,
  `TestSchemaProtoDriftDetectsImplicitPresence`, `TestModelStaysAgentInternal`.
- `packages/proto/test/parsed-documents.test.ts`: `RootConfig.parse()` + `redactSecrets` of `{}` and every valid
  example/fixture through `DesiredState.fromJSON/toJSON` (28 documents: `{}`, 26 examples, 1 fixture).

## Non-contract changes in the same branch

- `packages/proto/package.json`: devDependency `@ngfw/schema: workspace:*` (lockfile +5/−2 lines).
- `packages/proto/test/fixtures/all-domains.json`: 8 value fixes so the fixture is a valid document (questions #8).
- `packages/proto/test/desired-state.test.ts`: header comment only.

## For the tag gate (D-035)

With this branch merged, every schema leaf has its proto field and vice versa (4 documented supersets), CI fails on
any future drift, and the factories have typed messages to adapt to. Open points for the manager:
`docs/status/tasks/P03b-questions.md` (#1 package placement, #2 implicit presence in `vrx.model`, #6 the
`Redistribute` exception) — none blocks the tag.
