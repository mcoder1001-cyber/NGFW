# P03 — contract change summary (`contract(proto)`) for the manager's review

Branch `task/P03`, worktree `/root/ngfw-wt/P03`. What the `vrx.v1.Dataplane` contract looks like after the review round
(`P03-review.md`, APPROVE WITH CHANGES) and *why* each change is in — every design point below is a manager decision in
`docs/decisions/LOG.md`; this file only records how it was applied. Full semantics: `docs/contracts/proto.md`; evidence
with pasted output: `docs/status/tasks/P03.md` §"Review fixes".

## What changed in the contract and why

| change | decision | why now (not after `contracts-v1`) |
|---|---|---|
| Every scalar leaf under `DesiredState` is proto3 `optional` (182 declarations retyped; none implicit left, pinned by a descriptor-walk test) | **D-039** | Implicit presence made `toJSON`/`protojson.Marshal` drop `vrfs.default.id = 0` and `enabled: false`, so the documented running-vs-actual diff reported false drift and hid real drift on Zod-`true` defaults (review F3). Changing cardinality later is `FIELD_SAME_CARDINALITY`-breaking. |
| 64-bit integers are `string` in the TS stubs (`forceLong=string`); `proto.md` §9 states the real arithmetic (2^53 B = 9 PB ≈ 8.3 days at 100 Gbit/s) | **D-039** | The default `number` mapping throws inside the grpc-js stream once an absolute byte counter crosses 2^53 (review F1). `string` over `bigint`: it is the canonical protobuf JSON form, so Go and TS `toJSON` agree and no consumer needs a JSON↔bigint shim; it needs no extra dependency. TS-only change, no wire change, but it changes the TS type of every 64-bit field — hence before P06 codes against `number`. |
| `ManagementUser.password_hash` removed (`reserved 4; reserved "password_hash"`); rule "schema leaves flagged `secret: true` have no proto field" in the file header and `proto.md` §1; structural test forbids secret-named leaves | **D-040** | Authentication material never crosses the API↔agent boundary; P05 persists the whole desired state to `desired.pb`, so any such field would land on disk (00-CONTEXT rule 10; review F4). The API strips secret-flagged leaves generically before `fromJSON`; strict decode now rejects a document that still carries one. |
| Domain presence semantics: a domain **unset** in `ApplyRequest.desired_state` (and not named in `subsystems`) is skipped; a **present** domain, even `{}`, is authoritative (delete everything owned that is absent); `subsystems` narrows and can force an unset/empty map domain to be authoritative | **D-041** | With "unset = no objects", a partial document wiped interfaces/VRFs/routes with a valid `APPLIED` (review F7). Semantics only, no wire change; written into the proto comments and `proto.md` §2 as a decision table so P05 implements exactly one reading. |
| P02a HEAD (`df554dc`) shapes: `system.banner` → `SystemBanner{login?, motd?}`; `dataplane.corelist` → `repeated uint32`; `Interface.rx_mode`, `NextHop.address` → `optional`; flat additive P02a leaves taken along (`promiscuous`, `dot1ad`, `tx_queues`, `distance`, `description`, `ssh_keys`, `full_name`, `disabled`) | **D-042** | Each retype/presence change would trip `buf breaking` after the tag (review F2 proved it on a scratch copy); P02a is committed, so guessing is over. Shells (`SystemNtp`, `BgpConfig`, `ManagementAaa`, …) and P02b/P02c leaf additions stay for P03b — additive only. P02b/P02c re-checked for renames since the mirrored commits: none. |
| Enum renames `Operation`→`ApplyOperation`, `ResultCode`→`ObjectResultCode`, `Severity`→`IssueSeverity`; `Event.interface` → `optional`; numbering convention "append-only, next free number"; `rx_misses` comment | review F10 (manager: "wording fixes … `Event.interface` → optional; enum names; numbering sentence") | Free today, impossible after the tag; generic names in package `vrx.v1` would collide with later state/event files. `HealthResponse` fields 1–3 untouched (P05a's stub). |

## What did not change

RPC set and request/response shapes (`Apply`, `Retrieve`→`RetrieveResponse`, `DryRun`→`ValidationReport`, streams,
`Action`, `Health`), field numbers of every existing field, the JSON-projection rules (snake_case ↔ Zod names, records →
maps, enums → strings, unions flattened), D-034's earlier decisions (maps for `interfaces`/`vrfs`, no `reserved` ranges
for planned fields, lint exceptions). `buf breaking --against main` is green (main has only `Health`).

## Non-contract changes in the same round

- Go contract test moved out of the generated tree to `apps/agent/internal/contracttest/` (review F8); `gen.sh` wipes
  `apps/agent/gen` fully again.
- `@ngfw/proto` gets a real `build` (`tsconfig.build.json` → `dist/`, `exports` → `dist/vrx/v1/dataplane.js`), so a
  Node ESM import from `apps/api` resolves (review F9; verified).
- Test corpus: every `packages/schema/examples/*.json` plus `packages/proto/test/fixtures/all-domains.json`; Go strict
  (`DiscardUnknown=false`) with negative cases; TS structural (`toJSON∘fromJSON == doc`) because ts-proto `fromJSON` is
  lenient — documented (review F5). The `RootConfig.parse()` → JSON-Schema-keys ⊆ proto-fields drift guard is P03b's.

## For the tag gate (D-035)

Ready to merge as the `contracts-v1` proto candidate together with P02a/P02b/P02c; P03b (post-merge, additive) fills the
shells and the P02b/P02c leaf keys and adds the drift guard — sized in `P03-questions.md` #9.
