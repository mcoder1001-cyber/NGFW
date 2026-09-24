# F-acl — contract change: `AclState` (additive)

Branch `task/F-acl` (P08 pattern: the contract commit sits on the task branch, no own branch). Commit subject
`contract(proto): acl state`.

## What
- `packages/proto/vrx/v1/dataplane.proto`
  - `service Dataplane`: `rpc AclState(AclStateRequest) returns (AclStateResponse);` under the `// wave-A: F-acl` anchor
    (framed by blank lines, C5).
  - `// ----- F-acl -----` section: `AclStateRequest {owner = 1, list = 2, offset = 3, limit = 4, filter = 5,
    include_interfaces = 6}`, `AclStateFilter {sequences = 1, hits_only = 2}`, `AclStateResponse {owner = 1,
    retrieved_at = 2, counters_available = 3, counters_reason = 4, lists = 5, rules = 6, total = 7, interfaces = 8,
    macip_lists = 9}`, `AclListState {name = 1, acl_index = 2, vpp_rules = 3, mapping_known = 4, config_rules = 5,
    packets = 6, bytes = 7}`, `enum AclRuleStatus {UNSPECIFIED, APPLIED, DISABLED, SCHEDULE_INACTIVE, EMPTY}`,
    `AclRuleState {sequence = 1, status = 2, vpp_rules = 3, first_vpp_rule = 4, packets = 5, bytes = 6}`,
    `AclInterfaceState {interface = 1, sw_if_index = 2, input = 3, output = 4, macip = 5}`,
    `AclBoundAcl {acl_index = 1, name = 2, tag = 3, foreign = 4}`.
  - Names follow the prefix rule (§0.5): every message and the enum start with `Acl`. No field is added to an existing
    message, so no §2 number is used (`AclConfig` 7 stays unallocated: no config gap).
- Regenerated (C7, never hand-edited): `apps/agent/gen/vrx/v1/dataplane{,_grpc}.pb.go`, `packages/proto/gen/ts/vrx/v1/dataplane.ts`.
- `apps/api/src/testing/fake-agent.ts` (P5): the UNIMPLEMENTED stub handler under the anchor (the exhaustive
  `DataplaneServer` type would not compile without it). The real fake behaviour replaces that one line later
  (`features/acl/fake.ts`).
- `docs/contracts/proto.md` §11 (C6): `### F-acl: AclState`.

## Why
Hit counters, VPP indexes and the interface bindings as VPP holds them (other owners' ACLs included, D-066) are runtime
state; `Retrieve` must stay configuration-only (proto.md §5). The rule editor must stay usable at 100 000 rules, so the
RPC is **paged** (≤ 1000 rules per message, a `total`) and filterable by the sequences of the page the API shows; a whole
list never travels in one message. Counters are per **configuration** rule (sequence), mapped back through the agent's
expansion (one config rule → address objects × families × services VPP rules).

## Not changed
- No config field (`AclConfig` exists; `packages/schema/src/domains/acl.ts` and `semantic/acl.ts` are P02b's).
- No `EventKind`, no `ActionRequest` member: counters are pulled for the visible page, not pushed.

## Compatibility
Additive only: one new RPC, eight new messages and one enum. `buf lint` clean; nothing existing changed. An older
agent answers UNIMPLEMENTED, which the API maps to 501.
