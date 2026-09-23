# P03 — questions / decisions for the manager (worker keeps going; nothing is parked)

1. **Lint exception added: `RPC_RESPONSE_STANDARD_NAME`** (extends D-008). buf STANDARD rejects the docs/04 sketch's
   `stream StatsBatch`, `stream Event`, `stream ActionOutput` and `returns (ValidationReport)`. Options: (a) wrapper
   messages `StreamStatsResponse{batch}` etc. — one extra indirection in every consumer loop; (b) rename the element
   types to `<Rpc>Response` — loses the docs/04 names P05/P06 prompts use; (c) except the rule. Took (c), same
   rationale as D-008 ("matches docs/04 naming"). Request types are standard (`DryRunRequest`, `StreamStatsRequest`,
   `StreamEventsRequest`), which also satisfies `RPC_REQUEST_RESPONSE_UNIQUE`. Say so if you prefer (a); it is a
   pre-`contracts-v1` rename, ~30 min.
2. **`DesiredState.interfaces` / `.vrfs` are `map<string, …>` fields, not `<Key>Config` wrapper messages.** The envelope
   said "one message per domain key"; the two record-shaped domains cannot be wrapped without breaking the property
   "protobuf JSON of a parsed document *is* a DesiredState" (a wrapper needs `{interfaces: {interfaces: …}}`). The
   other 11 domains are `<Key>Config` messages. Alternative: wrappers + a two-line transform in the API and both tests.
3. **`Retrieve` returns `RetrieveResponse{desired_state, subsystems, owner, retrieved_at}`**, not a bare `DesiredState`
   (sketch), and **`DryRun` takes `DryRunRequest`**, not `ApplyRequest` (sketch): `RPC_REQUEST_RESPONSE_UNIQUE` forbids
   sharing `ApplyRequest`, a dry run has no confirm fields, and Retrieve needs to say which subsystems it actually
   dumped (implemented vs not). Both are pre-v1 shape choices; reversible in minutes.
4. **Unmodelled domains mirror the committed P02b/P02c WIP, not docs/04's literal sketch.** P02b's committed NAT model
   (`task/P02b@ec0ccda`) has `staticMappings/identityMappings/loadBalancedMappings/pools/nat64/…/cnat` — docs/04's
   `static/portForwards/cgnat` do not exist there; a literal docs/04 mirror would have been dead on arrival. Mirrored
   `nat`, `objects`, `acl` (P02b) and `vpn` (P02c@b815d15) field-by-field; `tunnels`, `services`, `ha` (P02c, nothing
   committed yet) and the P02a domains carry the documented target shapes with empty sub-messages. **Follow-up needed
   after P02a/b/c merge:** a `contract(proto)` sync (mechanical diff, est. 1–2 h). Suggest a board row "P03b: proto sync
   after contracts-v1 candidates merge" owned by one worker (P02x workers do not own `packages/proto`). If P02b/P02c
   rename fields before merging, that sync is *breaking* against main — acceptable before the `contracts-v1` tag?
5. **No `reserved` ranges for planned fields.** Proto `reserved` means "never use again"; buf's
   `RESERVED_FIELD_NO_DELETE` (FILE category, which `buf.yaml` uses) would flag P02b/P02c *filling them in* as a breaking
   change. Used comments + fixed top-level numbering ("Zod fields from 1 in declaration order; agent-only annotations
   from 100") instead.
6. **P02a shape guesses to confirm when P02a merges:** `management.users[]` = `{username, role, scope, passwordHash}` flat
   (vdom.md #3 could also be `roles[]{role, scope}`); `dataplane.corelist` as a string in VPP syntax (`"2-5,8"`);
   `interfaces.<n>.mtu` optional (presence = explicitly set); `routing.static[]` without `description`/`distance`;
   `system.ntp`/`system.dns` and `routing.{bgp,ospf,isis,rip,bfd,prefixLists,routeMaps}` as empty messages.
7. **Drift guard proposal (not implemented, out of scope):** a test in `packages/schema` (or CI) that walks
   `z.toJSONSchema(RootConfig)` and asserts every property has a field with the same JSON name in the `DesiredState`
   descriptor (schema ⊆ proto). It would pass today and fail the moment a P02x field lands without the proto follow-up —
   which couples their merges to a `contract(proto)` commit. Manager's call; ~1 h in P09.
8. **`owner` on requests is a guard, not a selector:** an agent serves one owner (VRX_OWNER); a mismatching non-empty
   `owner` fails with INVALID_ARGUMENT. Multi-owner agents were rejected (one desired.pb, one confirm timer, one resync).
   Envelope listed `owner` in ApplyRequest without semantics — this is the interpretation written into
   `docs/contracts/proto.md` §6.
