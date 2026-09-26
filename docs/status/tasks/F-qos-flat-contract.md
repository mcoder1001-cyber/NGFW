# F-qos-flat — contract changes

Branch `task/F-qos-flat` (no own branch: workers never create branches, 00-CONTEXT). Numbers from
docs/status/wave-BC-numbers.md § F-qos-flat.

## contract(proto): QosPolicerState, QosPolicerReset

- `service Dataplane`, under the `// wave-BC: F-qos-flat` anchor:
  - `rpc QosPolicerState(QosPolicerStateRequest) returns (QosPolicerStateResponse)` — this owner's policers and
    shapers (egress policers `shaper:<name>`, V3) as VPP reports them: configuration, token buckets
    (`policer_dump_v2`) and conform / exceed / violate counters (stats segment `/net/policer/*`). Read-only, one walk
    at a time per agent (D-132).
  - `rpc QosPolicerReset(QosPolicerResetRequest) returns (QosPolicerResetResponse)` — `policer_reset` (refills the
    token buckets; VPP keeps the counters).
- New messages in the `// ----- F-qos-flat -----` section (field numbers from 1): `QosPolicerStateRequest`,
  `QosPolicerCounter`, `QosPolicerStatus`, `QosPolicerStateResponse`, `QosPolicerResetRequest`,
  `QosPolicerResetResponse`. Prefix `QosPolicer*` (wave-A-hotspots §0 rule 5). The row message is `QosPolicerStatus`,
  not `QosPolicerState`, so the RPC and the message never share a name.
- **Not used:** `QosService` 5 / `QosPolicer` 13 / `QosInterface` 7 (reserved only for a proven gap — there is none:
  `services.qos` already carries every leaf this feature projects). No `ActionRequest` member, no `EventKind`.
- docs/contracts/proto.md §11: `### F-qos-flat: QosPolicerState, QosPolicerReset` (appended at the end of §11 —
  no F-qos-flat anchor was seeded there; F-qos-flat-questions.md Q1).
- Regenerated: `apps/agent/gen/**`, `packages/proto/gen/ts/**` (`packages/proto/gen.sh`); API client and CLI table
  with the API routes (`pnpm gen`, `make -C apps/cli gen docs`).
- API hotspots that must compile with the new service: `apps/api/src/agent/agent.client.ts` (P4: `qosPolicerState`,
  `qosPolicerReset`), `apps/api/src/testing/fake-agent.ts` (P5: one `...qosFlatFake(this)` line + its import; the
  handlers live in `apps/api/src/features/qos-flat/fake.ts`).

## contract(schema): services.qos-flat-store-source

- New `packages/schema/src/semantic/qos-flat.ts` exporting `qosFlatValidators` (one spread + one import in
  `semantic/index.ts`, unanchored — Q1): `services.qos-flat-store-source` — VPP 26.06 implements `qos store` for the
  `ip` source only, so `services.qos.interfaces.<if>.store.source` ≠ `ip` is a 400 with pointer
  `/services/qos/interfaces/<if>/store/source`.
- No schema shape change (no new field, no ext file, no C1/C3 hunk).
- "mark requires map" and "shaper excludes policer.output" are already enforced by the schema tier
  (`QosInterfaceSchema`) and are pinned by `semantic/qos-flat.test.ts` instead of being repeated as semantic rules
  (tier (b) never sees a document tier (a) rejected) — F-qos-flat-questions.md Q2.
- Fixture: `packages/proto/test/fixtures/qos-flat-full.json` (valid document for the proto drift tests). No
  `packages/schema/examples/qos-flat-*.json`: `examples.test.ts` rejects file names outside its sibling groups
  (`nat|objects|acl|vpn|tunnels|services|ha-…`) — Q3.
