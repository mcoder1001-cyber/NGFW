# F-lisp — LISP / LISP-GPE (status)

Branch `task/F-lisp` (base be53867). Minimal LISP / LISP-GPE end to end in FAST MODE, fake-client evidence; **the opt-in host
run (VRX_DF6_LISP_HOST / VRX_INTEGRATION) was NOT run** — this is a cloud container without VPP; per D-064/V14 it needs a manager
VPP window (requested in F-lisp-questions.md). Nothing was run on the shared VPP.

## What
- Contract (first commits): `tunnels.lisp` (`packages/schema/src/domains/ext/lisp.ts`), `TunnelsConfig.lisp = 10` + LISP messages +
  `rpc LispState`, drift fixture `packages/proto/test/fixtures/lisp-full.json`, proto.md section — see F-lisp-contract.md.
- Schema rules `semantic/lisp.ts`: `tunnels.lisp-eid-canonical`, `-eid-unique`, `-references`, `-eid-table`, `-rloc-family`,
  `-enabled`, `-gpe-entries`.
- Agent: `desired/lisp.go` (builder + assembler), `subsystems/lisp.go` (lisp.Register with df6 **PairClaims("df6")** — not IfaceClaims,
  TD-11b refuses start otherwise — and `WithGlobalsOwner`; decorator: plugin-absent Retrieve = empty, globals declare
  RecordsNoOwnership), `agent/rpc_lisp.go` (LispState), `coretest/lisp.go` (installable LISP model, ported from DF-6's fake).
- API: `features/lisp` (`GET /api/v1/state/lisp`, real fake in fake.ts); config via generic pointer routes; api-client + CLI op table regenerated.
- UI: VPN page tab `lisp` ("LISP (advanced)"), sub-tabs Locators / EIDs / Mappings / Resolvers, SchemaForm per sub-tab + status
  column; en + fa; `vpn` into BUILT_DOMAINS.
- Docs: `docs/user/vpn/lisp.md`.

## Verification (pasted)
Agent unit (fake VPP, `go test ./internal/agent -run Lisp -v`):
```
    rpc_lisp_test.go:118: plan (lisp objects, apply order): lisp.enable/global → lisp-gpe.enable/global → lisp.locator-set/w11-rloc → lisp.locator/w11-rloc/loop1101 → lisp.eid-table-map/l3/1100 → lisp.local-eid/1100/10.11.100.0/24 → lisp.map-resolver/10.11.1.254 → lisp.map-server/10.11.1.253 → lisp.remote-mapping/1100/10.11.200.0/24 → lisp.adjacency/1100/10.11.200.0/24/10.11.100.0/24 → lisp.pitr/global → lisp-gpe.fwd-entry/1101/10.11.201.0/24/10.11.101.0/24
    rpc_lisp_test.go:141: restart resync: APPLY_STATUS_APPLIED in 14.540545ms, summary created:1  unchanged:19
    rpc_lisp_test.go:176: rollback deletes: lisp-gpe.fwd-entry/1101/10.11.201.0/24/10.11.101.0/24 → lisp.pitr/global → lisp.adjacency/1100/10.11.200.0/24/10.11.100.0/24 → lisp.remote-mapping/1100/10.11.200.0/24 → lisp.map-server/10.11.1.253 → lisp.map-resolver/10.11.1.254 → lisp.local-eid/1100/10.11.100.0/24 → lisp.eid-table-map/l3/1100 → lisp.locator/w11-rloc/loop1101 → lisp.locator-set/w11-rloc
    rpc_lisp_test.go:190: after rollback: local sets 0, mappings 0, adjacencies 0, eid-table maps 0, gpe entries 0; leaked remote locator sets 1 (V14); LISP still on true (globals kept on absence, D-071)
--- PASS: TestLispFullExampleApplyRetrieveResyncRollback (0.14s)
--- PASS: TestLispGpeEntryReappliedAfterVPPRestart (0.06s)
--- PASS: TestLispRequiresGlobalsOnSlotAgents (0.04s)
--- PASS: TestLispProjectionWarnsForTunnelKindsNotWired (0.00s)
--- PASS: TestLispStateRPC (0.05s)
ok  	ngfw/agent/internal/agent	0.301s
```
- Full example: 12 LISP objects planned; Retrieve == expected (everything but the write-only GPE entry); second apply 0 changes.
- Restart simulation (new process, same VPP boot, same state dir): resync APPLIED in ~15 ms (< 30 s), no LISP write sent,
  GPE entry not re-added (D-076; "created:1" is the write-only entry re-asserted from its claim, nothing sent).
- VPP restart (ForgetGpe): resync re-applies the GPE entry once (TestLispGpeEntryReappliedAfterVPPRestart).
- Rollback: adjacency → remote mapping → local EID → EID-table map → locator → locator set; VPP model empty except the
  V14 leaked remote locator set; LISP switch kept on (KeepOnAbsence, D-071). Note: the scheduler deletes the EID-table map
  before the locator set (dependency order; no dependency between them) — D-074's list puts maps last; harmless.
- Slot agent (not globals owner) with LISP off: Apply fails with "managed by the globals owner only", no enable sent.
- API unit (`apps/api src/features/lisp/lisp.test.ts`, fake agent over gRPC, in-memory repo): commit of the full example applied;
  state endpoint shape; **duplicate EID → 400 problem+json, tier semantic, pointer `/tunnels/lisp/localEids/1/eid`**, agent not asked;
  501 without the RPC. `test/e2e/lisp.e2e.test.ts` written for the PostgreSQL path — **not run here** (no PG daemon in the container).
- Schema: 14 lisp tests; package 1228 tests pass. Proto drift (vitest 70 + Go TestSchemaProtoDrift) pass.
- Web: model + LispTab tests; full web suite 103 pass. Screenshots (en + fa/RTL, 4 sub-tabs each) taken with headless Chromium +
  playwright-core against the vite dev server with **scripted API responses** (the fixture + a LispState matching the fake agent),
  not a real API/agent — no PG/Valkey here. Files kept outside the repo (worker scratchpad `shots/lisp-{en,fa}-*.png`).
- Gates: gofmt/go vet/go test ./... in apps/agent green; turbo lint/typecheck/test/build green except ui-kit SchemaForm tests
  timing out under load (all 48 pass when rerun alone). `tools/ci.sh --base main` **could not run in this container**: its
  `pnpm gen` step fails with "Exec format error" spawning pnpm's managed tool binary (environment; `npx turbo run gen` works).
  golangci-lint here is built with go1.25 and refuses the go1.26 module (not run). gitleaks: 1 finding in a base commit
  (c2a8ab7, vpn.test.ts), none in F-lisp commits.

## Shared hunks
- `packages/schema/src/domains/tunnels.ts`: key line under anchor + `import { LispSchema }` at the top (outside anchor).
- `packages/schema/src/index.ts`, `semantic/index.ts` (import + spread) — under anchors.
- `dataplane.proto`: rpc under service anchor; field 10 under TunnelsConfig anchor; F-lisp section. Generated stubs.
- `docs/contracts/proto.md`: appended `### F-lisp: LispState`.
- `subsystems.go`: `Tunnels` const, `Domains[Tunnels]` (added the key — F-tunnels appends), `registerLisp` call, lisp import.
- `projection.go`: one call each in project()/assemble().
- `apps/agent/internal/agent/service_test.go`: 2 lines — expected subsystems now `interfaces,vrfs,routing,tunnels` (unavoidable
  once a domain is added; not an owned file — see questions).
- `app.module.ts` (import/controllers/providers), `agent.client.ts` (type import + method), `fake-agent.ts` (lispState line + import).
- Web: `vpn/tabs.ts` (+ `lazy` import), `nav.ts`/`nav.test.ts` ('vpn'), `i18n.ts` (4 lines).
- Generated: packages/api-client schema.d.ts, apps/cli operations_gen.go.

## Out of scope (not built)
Map-server HMAC keys, NSH EIDs, ONE API, LISP-GPE over IPsec, VXLAN-GPE, SRv6, bridge domains, any V13/V14 fix,
switching LISP off via config (globals kept on absence).

## Open questions
See F-lisp-questions.md.
