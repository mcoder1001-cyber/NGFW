# Task: F-lisp — LISP and LISP-GPE   (prepend 00-CONTEXT.md)

## Goal
Expose the **minimal LISP/LISP-GPE** set end to end in FAST MODE: enable, locator sets and locators, local EIDs, map-resolvers and
map-servers, static remote mappings, adjacencies, EID-table ↔ VRF/bridge mapping, PITR, GPE forwarding entries. T3 ("effectively
dead technology") — keep it thin: correct, restart-safe, documented limits. Reference: TNSR has no LISP UI; VPP plugins `lisp`,
`lisp_gpe` (WBS D6.8 in `plan/wbs.csv`).

## Inputs to read first
- Contract first: there is **no** LISP config path in `packages/schema`. Add on branch `contract/F-lisp` (schema + proto + drift guard,
  additive after contracts-v1), e.g. `tunnels.lisp{enabled, gpe, locatorSets{<name>: {locators[{interface, priority, weight}]}},
  localEids[{vni, eid, locatorSet}], mapResolvers[], mapServers[], remoteMappings[{vni, eid, rlocs[]}], adjacencies[], eidTables{<vni>:
  {vrf|bridgeDomain}}, pitr?}`; agree the path in a questions file and continue
- Merged DF-6: `apps/agent/internal/descriptors/lisp/` + `docs/agent/descriptors/lisp.md` (keys `lisp.locator-set/<name>`,
  `lisp.local-eid/<vni>/<eid>`, `lisp.remote-mapping/…`, `lisp.adjacency/…`, `lisp.eid-table-map/l3|l2/<vni>`, `lisp-gpe.fwd-entry/…`;
  globals `lisp.enable`, `lisp-gpe.enable`, `lisp.pitr`; `lisp.SafeToDisable`)
- `apps/agent/binapi/{lisp,lisp_gpe,lisp_types}/` — only source of names
- `docs/vpp-code-track.md` **V13** (gpe fwd-entry path dump replies with the wrong id → `lisp-gpe.fwd-entry` write-only with an existence
  probe) and **V14** (each enable/disable leaks a `<remote-N>` locator set and `lisp_gpe*` interfaces no API deletes → host test opt-in,
  never on the shared VPP); DF-6 doc numbers these V9/V10 locally — use the code-track ids
- `docs/decisions/LOG.md` D-063/D-076/D-080 (write-only + boot identity), D-064 (host test opt-in `VRX_DF6_LISP_HOST=1`, not run on the
  shared VPP), D-071 (LISP enable is a global: globals owner only; never disabled while any owner has LISP objects), D-074, D-082

## Scope — build exactly this
1. **Schema**: semantic rules — EIDs canonical (masked prefix or MAC), unique per VNI; locator interfaces exist; remote mapping RLOC
   family consistent; each VNI with local EIDs has an eid-table mapping; `gpe` requires `enabled`.
2. **Agent**: projection → DF-6 lisp descriptors; the enable globals are emitted only on the globals owner, other agents use the
   "require" variants (fail clearly if LISP is off). Retrieve covers every non-write-only object. Unit tests with the fake cover the full
   set; the host integration check (`VRX_INTEGRATION=1`) is **opt-in only** and runs in a manager VPP window (V14) — say so in status.
3. **API**: config via pointer routes; `GET /api/v1/state/lisp` (status, map-cache/adjacencies from the dumps). OpenAPI; regenerate
   `packages/api-client`.
4. **UI**: one LISP page with tabs (Locators / EIDs / Mappings / Resolvers), SchemaForm, status column; en + fa; screenshot against
   the real endpoint (fake-backed agent acceptable if the host test is not run — state which).
5. **Docs**: `docs/user/vpn/lisp.md` — one static-mapping example, CLI equivalent, the V13/V14 limits in plain words.
Files you own: `apps/agent/internal/descriptors/lisp/**`, `docs/agent/descriptors/lisp.md`, `apps/agent/internal/agent/project_lisp*.go`,
`apps/api/src/features/lisp/**`, `apps/web/src/domains/vpn/lisp/**`, `apps/web/src/locales/*/lisp.json`, `docs/user/vpn/lisp.md`, `test/topology/lisp/**`.
Shared files: one-line appends only; `descriptors/df6/**` read-only.

## Acceptance (paste the evidence)
- [ ] Unit: projection + descriptors produce the expected plan for the full example; second plan empty; write-only fwd entries not
      re-added on resync (D-076)
- [ ] If the opt-in host run was granted: `vppctl show lisp locator-set`, `show lisp eid-table` reflect the config (pasted), else the
      unit evidence + the reason
- [ ] Agent-restart simulation (fake or host) → config back within 30 s (log excerpt)
- [ ] Rollback removes adjacencies → mappings → EIDs → sets (Retrieve), leaks per V14 documented
- [ ] Duplicate EID in one VNI → 400 problem+json with a `pointer`; `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
Map-server authentication keys (HMAC secret — not modelled); NSH EIDs; ONE/`one` API; LISP-GPE over IPsec; VXLAN-GPE tunnels
(F-tunnels); SRv6 (F-srv6); bridge domains (F-bridge-l2); any fix for V13/V14.

## Open questions to surface, not to decide silently
Config home (`tunnels.lisp` vs `routing.lisp`) — manager decides on the contract branch. Whether the product should hide LISP behind a
feature flag given V14 — default: visible under "advanced", documented.
