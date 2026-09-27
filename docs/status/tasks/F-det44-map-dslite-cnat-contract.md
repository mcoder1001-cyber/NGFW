# F-det44-map-dslite-cnat — contract changes (additive only)

Numbers from `docs/status/wave-BC-numbers.md` § F-det44-map-dslite-cnat, verified free on `origin/main` (a302de71):
`NatConfig` max was 24 (25–26 stay F-nat44-ed-sessions'), `ActionRequest` used 1–5, 7 (+ others' anchors) — 9 and 10 free.

## Schema (`contract(schema)`)
- `nat.pnat` (optional; absent by default) — `packages/schema/src/domains/ext/det44-map-dslite-cnat.ts`, key line
  `NatSchema.pnat` under the C1 anchor: `bindings[]{name, match{proto?, src?, sport?, dst?, dport?}, rewrite{src?, sport?,
  dst?, dport?}}`, `attachments[]{binding, interface, point: input|output}`. IPv4 only. Refinements (pointers inside
  `nat.pnat`): non-empty match and rewrite, ports only with tcp/udp, unique names and match tuples, attachment names
  an existing binding, no duplicate attachment, one match mask per (interface, point).
- Manager answer Q3: `nat.map.parameters.securityCheck.enabled` and `trafficClass.copy` now default to `true` (VPP's
  defaults). This changes a default value, not the shape; a document that omits them now asks for VPP's defaults.
  Stored documents keep their explicit `false` (they were parsed with the old defaults on save); only imports, restores
  and hand-written candidates get `true`. No migration: operators reset `securityCheck.enabled` /
  `trafficClass.copy` by hand on existing documents.
- Semantic rules (`semantic/det44-map-dslite-cnat.ts`, mirroring the agent): `nat.det44-map-dslite-cnat-cnat-snat-address`,
  `…-cnat-nat44-interface`, `…-map-domain-name` (`nat46-` reserved), `…-lw4o6-rules`, `…-pnat-interfaces`.

## Proto (`contract(proto)`, `packages/proto/vrx/v1/dataplane.proto`)
- `NatConfig.pnat = 27` → `PnatConfig{bindings, attachments}`, `PnatBinding`, `PnatMatch`, `PnatRewrite`, `PnatAttachment`.
- RPCs `Det44Sessions`, `Det44Lookup`, `CnatSessions` (+ request/response/row messages).
- `ActionRequest.det44_session_close = 9` (`Det44SessionCloseAction{direction in|out, address, port, external_address,
  external_port}`), `ActionRequest.cnat_session_purge = 10` (`CnatSessionPurgeAction{}`, globals owner only).
- Regenerated: Go (`apps/agent/gen`), TS (`packages/proto/gen/ts`; `timestamp.ts` restored — comment-only diff),
  `packages/yang/generated/vrx-nat.yang`, `packages/api-client/src/generated/schema.d.ts`.
- Fake agent: UNIMPLEMENTED stubs for the three RPCs under the anchor (replaced by the feature fake later).
