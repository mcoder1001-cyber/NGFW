# Task: F-igp-followups-e — IGP state routes + live grids, OSPFv3/RIPng/auth in the UI, VRRP role chips, routing form i18n   (prepend 00-CONTEXT.md)

> Split 5 of 6 of board row F-igp-followups (D-173). Needs -a, -b, -c and -d merged (fields, readers `ospf6Neighbors`, EventKind 20/21, VrrpState).

## Goal
Make the IGP and VRRP screens live: neighbour/adjacency grids from the agent, OSPFv3 and RIPng configuration tabs, auth fields as secret
references, role chips on the HA page, and fa field labels on the routing forms. Reference: TNSR "OSPF neighbors", "IS-IS adjacency", "VRRP
status" screens; WBS D2.3, D2.4, D9.1. Server-side paging where a list can exceed 1k rows (not expected here; state it in OpenAPI).

## Inputs to read first
- `apps/agent/internal/renderers/frr/{ospf,ospf6,isis}/state.go` (reader keys `ospfNeighbors`, `ospfInterfaces`, `ospf6Neighbors`, `isisNeighbors` and their JSON shape)
- P12 `RoutingState` (`dataplane.proto:274-277`, `RoutingStateRequest.readers`) and its API use in `apps/api/src/features/bgp/bgp.controller.ts`; pattern for
  state routes `apps/api/src/features/mpls-ldp/mpls-ldp.controller.ts`; anchors `// wave-BC: F-ospf` / `// wave-BC: F-isis-rip` in
  `apps/api/src/{app.module.ts,infra/bus.ts,telemetry/relay.service.ts}` (topics `ospf.events`, `isis-rip.events` per wave-BC-numbers "Names")
- `GET /api/v1/state/ha/vrrp` + `vrrp.events` from F-igp-followups-d
- Web: `apps/web/src/domains/routing/ospf/{OspfPage.tsx,ProtocolForm.tsx,queries.ts,locale.ts}`, `routing/isis-rip/IsisRipPage.tsx`,
  `system/ha/{HaPage.tsx,queries.ts}` (it already passes `i18nPrefix` to its SchemaForms — copy that for routing), `locales/{en,fa}/{routingIgp,ha}.json`
- `docs/tech-debt.md` S-rva-web-fixes hand-off (2) (routing SchemaForms without `i18nPrefix`) and (3) (`ha.cluster.secretRef` picker — that one is -f's)

## Contract changes
None (all from -a). A missing field → questions file, never a local type.

## Scope — build exactly this
1. **API** (new `apps/api/src/features/ospf/**` `OspfController`, `apps/api/src/features/isis-rip/**` `IsisRipController`):
   `GET /api/v1/state/routing/ospf/neighbors`, `/ospf/interfaces`, `/ospf6/neighbors`, `/isis/adjacencies` — each one `routingState({readers:[…]})` call,
   parse + map to a typed Zod output (no raw FRR JSON passthrough), `agentError` on 501/unavailable/FRR not running; app.module lines under the anchors;
   relay cases EventKind 20 → `ospf.events`, 21 → `isis-rip.events` + bus topics; OpenAPI; regenerate `packages/api-client`; fake agent returns readers
   for the applied document (existing fake RoutingState hook — extend under your anchor).
2. **UI**: OSPF page: OSPFv2 neighbours grid (neighbor id, address, interface, state, dead time) and OSPFv3 tab (schema-driven form for `routing.ospf6` +
   its neighbours grid); auth fields render as a secret-reference picker if `packages/ui-kit` has one, else the plain ref field with a help text (no
   plaintext input). IS-IS/RIP page: adjacency grid (system id, interface, level, state), RIPng tab (schema form), RIP version/auth fields, IS-IS family
   switches and password refs. HA page: live role chip per VR from `/state/ha/vrrp`, refreshed on `vrrp.events`. Live updates via the WS topics, polling
   fallback ≤ 10 s. `i18nPrefix` on every routing SchemaForm with en + fa keys for every field (closes tech-debt hand-off (2)).
3. **Tests**: API unit tests (reader mapping, 501 → `agentError`, malformed reader JSON → `agentError`, never a 500); web vitest for the grids (empty,
   error, rows), the chip states and fa label resolution.
4. **Docs**: `docs/user/routing/ospf.md` + `isis-rip.md` "Live state" sections (routes and CLI equivalents), `docs/user/system/vrrp-config-sync.md` role chips.

## Acceptance (paste the evidence)
- [ ] own API + web tests green (paste)
- [ ] `curl` of the four state routes against your slot API + agent with a running slot frrtest (or the fake agent if no FRR window: say which) (paste)
- [ ] screenshot owed to T4 on the main stack after merge (D-175; T4 is off while `plan/NO-TESTS` exists — list it as owed); do not install a browser
- [ ] `tools/ci-slot.sh --base main` green (tail pasted)

## Out of scope (do not build)
New agent readers or RPCs (-b/-c/-d own them); config sync, cluster view and the `ha.cluster.secretRef` picker (-f); BFD page fixes (F-bfd-redistribution,
tech-debt hand-off (4)); MPLS/multicast pages; any schema or proto change; RIP state (FRR has no RIP JSON — show "not available").

## Open questions to surface, not to decide silently
Whether neighbour grids need history (flap counts) — not built; whether the secret picker belongs in packages/ui-kit (shared) — ask, do not add one there.

## Files you own
`apps/api/src/features/ospf/**`, `apps/api/src/features/isis-rip/**` (new), the `// wave-BC: F-ospf` / `// wave-BC: F-isis-rip` lines in
`apps/api/src/{app.module.ts,infra/bus.ts,telemetry/relay.service.ts,testing/fake-agent.ts}`, `packages/api-client/**` (generated),
`apps/web/src/domains/routing/ospf/**`, `apps/web/src/domains/routing/isis-rip/**`, `apps/web/src/domains/system/ha/HaPage.tsx` + `queries.ts` (role chips only),
`apps/web/src/locales/{en,fa}/{routingIgp,ha}.json`, `docs/user/routing/{ospf,isis-rip}.md` + `docs/user/system/vrrp-config-sync.md` (live-state sections),
`docs/status/tasks/F-igp-followups-e*`.

## Rules
- Files you own: above. Everything else read-only; a needed edit elsewhere → `docs/status/tasks/F-igp-followups-e-questions.md`.
- Web parts: only after M-origin-sync landed (`git merge-base --is-ancestor origin/main main` succeeds in /root/NGFW); then `git merge main` into your branch
  and follow the product wording rules (packages/ui-kit/src/i18n/product-wording.ts, docs/status/tasks/network-defaults-web.md — they arrive with that merge).
- Shared VPP: not written by this row; slot prefix `w<N>`, never restart or kill VPP, `timeout 10` on every vppctl, packet trace banned (D-128). Daemons: frr only
  if your envelope names it (slot frrtest for the curl evidence), left stopped.
- D-210a: write tests for your change and get them passing in your package (paste output); run them through `tools/heavy.sh` (D-224), e.g.
  `tools/heavy.sh pnpm --filter @ngfw/web exec vitest run <files>`; no full suite, no lint, no other packages' tests.
- Contracts: additive only — `contract(schema|proto): …` commits first, numbers only from `docs/status/wave-BC-numbers.md`; renaming/reshaping = PENDING.
- Finish: `tools/ci-slot.sh --base main` green (compile-only gate, D-220/D-222; never `tools/ci.sh` directly), commit on your branch,
  `docs/status/tasks/F-igp-followups-e.md` with pasted real output.
