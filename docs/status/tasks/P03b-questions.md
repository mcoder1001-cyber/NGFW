# P03b — questions / decisions for the manager (worker kept going; nothing is parked)

1. **Where the factories' leaf messages live (D-055): `packages/proto/vrx/model/<family>/v1`, proto packages
   `vrx.model.{acl,nat,iface}.v1`, Go only.** They are the scheduler Value types (VPP terms: table ids, address ranges,
   protocol numbers), not the document shape, so they are *not* added to `DesiredState` (the document-level
   `AclConfig`/`NatConfig` messages already exist there and are what the API sends). `dataplane.proto` does not import
   them (test `TestModelStaysAgentInternal`), ts-proto does not generate them (`buf.gen.yaml` restricted to `vrx/v1`,
   new `buf.gen.model.yaml`). Alternative considered: add them to `vrx.v1` — rejected, it would put agent-internal VPP
   objects into the API↔agent contract and the TS package. Say so if you prefer another package name (renaming is
   free until a factory imports them).
2. **Implicit presence in `vrx.model.*` (deliberate exception to D-039).** D-039 exists for the JSON running-vs-actual
   diff; these values are diffed with `proto.Equal` and the typed specs always carry every field (zero = canonical
   "not set"). Mirroring the specs 1:1 keeps the factories' adaptation mechanical. The drift guard only walks
   `DesiredState`, so it does not flag them.
3. **Shared NAT messages.** Where DF-3's specs are field-for-field identical across plugins they share one message
   (`Endpoint`, `Timeouts`, `InterfaceFeature`, `OutputFeature`, `Forwarding`, `IdentityMapping`); everything else is
   plugin-prefixed (`Nat44EdStaticMapping`, `Nat64StaticBib`, …). A descriptor's `KeyOf` still type-asserts its own
   Value, sharing a type across descriptors is fine for the scheduler.
4. **DF-3 is not merged.** `vrx.model.nat.v1` mirrors `task/DF-3@08d0af4`. If DF-3's review changes a spec before merge,
   the model must follow (additive only after that). The mirror check used here (scratch test: spec JSON → strict
   protojson → back, key- and value-equal; evidence in `P03b.md`) can be repeated in minutes; I suggest DF-3's adaptation
   commit ports it into its own package test once it imports `natv1`.
5. **Other DF-1 local models** (`bond_model.proto`, `memif`, `tapv2`, `af_packet`, `l2`, `l3xc`) are already typed protos
   (no structpb), so D-055 does not require them; only the interface-attribute model was promoted (envelope scope).
   Promote the rest in the same way when P08 wires the builder? (~30 min, additive.)
6. **Accepted drift: the shared `Redistribute` message** has a key for each protocol, while `routing.<p>.redistribute`
   rejects the protocol's own key → four proto→schema supersets, listed in `acceptedDrift` with the reason. The
   alternative (four per-protocol messages) is a pre-tag reshape for no functional gain; the API validates with Zod
   before `fromJSON`, so the extra key can never be set. Reverse if you want zero exceptions.
7. **`packages/proto` now devDepends on `@ngfw/schema`** (`workspace:*`, lockfile updated) for the parsed-document
   guard (`RootConfig.parse()` + `redactSecrets` in `parsed-documents.test.ts`). turbo `test` already depends on
   `^build`, so the schema package is built first. No runtime dependency.
8. **`packages/proto/test/fixtures/all-domains.json` was not a valid document** (P03-era fixture: an IPv6 literal with
   non-hex groups, `kind: "spec"` instead of `"inline"`, host-ACL `permit`/VPP interface name, a non-base64 WireGuard
   key, protocol `"ip"`). Fixed with 8 one-line value edits so it passes `RootConfig.parse()`; its 64-bit leaves stay in
   protobuf-JSON string form on purpose (converted in the parsed-document test).
9. **Go drift guard reads the generated JSON Schema from `packages/schema/dist`** (not committed, `.gitignore`). CI runs
   `pnpm gen` before `make -C apps/agent test`, so it always has a fresh file; outside CI a missing file skips with the
   command to run, under `CI=1` it fails. A stale local `dist` can give a stale result — `pnpm gen` fixes it.
