# F-mpls-srmpls — static MPLS + SR-MPLS (wave C, WBS D2.8)

Branch `task/F-mpls-srmpls`, worktree `/root/ngfw-wt/F-mpls-srmpls`, slot 5 (w5), base `main`@1d3ccf31.
LDP is F-mpls-ldp's (D-085/D-109); its anchors are seeded (see "For F-mpls-ldp").

## What was built

| Layer | Files | What |
|---|---|---|
| Contract (schema) | `packages/schema/src/domains/ext/mpls-srmpls.ts`, `semantic/mpls-srmpls.ts` (+ test), key line in `domains/routing.ts`, lines in `semantic/index.ts`, `index.ts` | `routing.mpls` (plain `strictObject`, optional): `interfaces`, `tables`, `labelRoutes`, `ipBindings`, `tunnels`, `sr{policies, steering}`; 12 rules `routing.mpls-srmpls-…` — see `F-mpls-srmpls-contract.md` |
| Contract (proto) | `packages/proto/vrx/v1/dataplane.proto` | `RoutingConfig.mpls = 15` → `MplsConfig` (1–6; 10 left to F-mpls-ldp), `rpc MplsState` + `MplsState*` messages; regenerated Go/TS stubs, API client, CLI operation table; fixture `packages/proto/test/fixtures/mpls-srmpls-full.json`; `docs/contracts/proto.md` § F-mpls-srmpls: MplsState |
| Descriptors (gap-only, D-104) | `apps/agent/internal/descriptors/mpls/{mpls,tunnel,ownership}.go`, `sr_mpls/ownership.go`, tests `f_mpls_srmpls_test.go` | D-071 table-0 role; table-0 routes of a non-globals-owner; TD-11b declarations + claim/record-first; TD-11c tunnel alias key; missing dependencies — every gap with a named test that fails on DF-7/DF-6 (below) |
| Agent | `desired/mpls_srmpls.go`, `subsystems/mpls_srmpls.go`, `agent/rpc_mpls_srmpls.go` (+ tests), `coretest/mpls_srmpls.go` | projection routing.mpls → `mpls-*` / `sr-mpls.*` (table 0 only when needed; segment lists sorted, D-074), Retrieve assembly (canonical; bindings/SR write-only, D-063), registration with persisted Wiring stores + slot id range, `MplsState` (MPLS FIB paged in the agent, one walk at a time, D-132; tunnels) |
| API | `apps/api/src/features/mpls-srmpls/{index,mpls-srmpls.controller,fake}.ts` (+ unit test), `test/e2e/mpls-srmpls.e2e.test.ts` | `GET /api/v1/state/routing/mpls/fib?table&label&page&pageSize` (404 unreadable table, 400 bad query/window), `GET /api/v1/state/routing/mpls/tunnels`; config through the generic pointer routes; fake agent's `MplsState` |
| UI | `apps/web/src/domains/routing/mpls-srmpls/**`, `locales/{en,fa}/mpls-srmpls.json` | Routing → MPLS (`/routing/mpls`): tabs Interfaces & tables · Label routes (+ bindings) · Tunnels (live status) · SR-MPLS (policies, steering) · MPLS FIB (paged, on demand); schema-driven editors (the one schema), one merge patch of `/routing` per save; `tabs.ts` a plain array with the F-mpls-ldp anchor |
| Docs | `docs/user/routing/mpls-srmpls.md`, `docs/agent/descriptors/{mpls,sr_mpls}.md` (appended sections) | model, examples (CLI `merge`/`set` + REST), live state, write-only note, table-0/globals rule |
| Host check | `apps/agent/internal/agent/rpc_mpls_srmpls_integration_test.go` | `TestMplsOnHost` — written, **not run** (host runs closed until TD-25; the table-0 half needs a manager window) |

## Evidence

(Every output below is pasted from this worktree; logs in `/root/ngfw-wt/logs/F-mpls-srmpls-*.log`.)

### Descriptor gaps: each test fails on DF-7/DF-6 as merged
The six DF-7 gap tests run against the base sources of `descriptors/mpls` (copied into a scratch package, removed again);
the seventh (`TestTableZeroRequiredByNonOwner`) needs the new `NewTableFor` API and does not compile on the base.
```
--- FAIL: TestRouteTableZeroOfTheGlobalsOwner (0.00s)
    f_test.go:40: mpls-route: re-apply plan for 1 desired object(s):
          create mpls-route/0/50016/eos
    f_test.go:40: mpls-route: re-applying the same desired state must plan nothing, got 1 op(s)
--- FAIL: TestRouteTableZeroRecordFirst (0.00s)
    f_test.go:67: the label was added although its record could not be written
--- FAIL: TestInterfaceClaimFirst (0.00s)
    f_test.go:94: MPLS enabled on eth0 without a claim (counter 1)
--- FAIL: TestOwnershipDeclarations (0.00s)
    f_test.go:120: mpls-table: descriptor declares neither CheckPersistent (it records ownership) nor RecordsNoOwnership
    f_test.go:120: mpls-interface: descriptor declares neither CheckPersistent (it records ownership) nor RecordsNoOwnership
    f_test.go:120: mpls-route: descriptor declares neither CheckPersistent (it records ownership) nor RecordsNoOwnership
    f_test.go:120: mpls-ip-bind: descriptor declares neither CheckPersistent (it records ownership) nor RecordsNoOwnership
    f_test.go:120: mpls-tunnel: descriptor declares neither CheckPersistent (it records ownership) nor RecordsNoOwnership
    f_test.go:127: with the in-memory defaults exactly mpls-interface and mpls-route must fail the persistence check: []
--- FAIL: TestTunnelProvidesInterfaceAlias (0.00s)
    f_test.go:151: mpls-tunnel does not provide its interface alias
--- FAIL: TestDependenciesOfBindingsAndLookupPaths (0.00s)
    f_test.go:166: default-VRF binding deps [{mpls-table/0 false} {vrf/0 false}]
FAIL	ngfw/agent/internal/descriptors/mplsbasecheck	0.034s
```
sr_mpls (`ownership.go` moved away, then back):
```
--- FAIL: TestOwnershipDeclarations (0.00s)
    f_mpls_srmpls_test.go:45: in-memory claims: undeclared [descriptor declares neither CheckPersistent (it records ownership) nor RecordsNoOwnership]; every descriptor must refuse them, got 2: [...]
FAIL	ngfw/agent/internal/descriptors/sr_mpls	0.021s
```
With the fixes: `ok ngfw/agent/internal/descriptors/mpls` and `ok ngfw/agent/internal/descriptors/sr_mpls`.

EVIDENCE_TESTS

### Screenshots (en + fa/RTL)
`docs/status/tasks/F-mpls-srmpls-screens/`: `mpls-interfaces`, `mpls-label-routes`, `mpls-label-route-editor`,
`mpls-tunnels`, `mpls-sr`, `mpls-fib` — each `-en.png` and `-fa-rtl.png` (`document.documentElement` = `rtl/fa`).
Taken with headless Chrome-for-Testing + playwright-core (P07a/P07b/P08 approach, scripts outside the repo) against the
real vrx-api (`apps/api/dist`, slot port 3500, database `vrx_w5`) and the web UI (vite, port 5500) of this worktree, with
**the API's FakeAgent standing in for vrx-agent + VPP** (host runs closed until TD-25): the FIB and tunnel views show the
fake's model of the committed document, not VPP. A pending edit (tunnel `t3`) shows the pending-change bar and the
"missing" live state of an uncommitted tunnel. Stack stopped by PID, `vrx_w5` dropped, 31 Valkey keys `vrx:w5:shots:*`
deleted, lab lock released.

### Host check — pending
Not run: host runs on the shared VPP are closed until TD-25 merges, and the table-0 half needs a manager window.
Acceptance items that therefore rest on the fake VPP (coretest) for now: `vppctl show mpls fib table <t>` /
`show sr mpls policies` output, the restart simulation on the host, "no stray entry" after rollback. The steps are in
`F-mpls-srmpls-questions.md` ("Host steps pending").

## Acceptance

- [ ] `vppctl show mpls fib <table>` / `show sr mpls policies`, and nothing after rollback — **pending host run**
  (`TestMplsOnHost`; SR part in a manager window). On the fake VPP: `TestMplsApplyRetrieveRollback` (routes before their
  table, steering before its policy, table 0 untouched by a non-owner, nothing left).
- [x] Agent-restart simulation → label routes + SR policy back within 30 s, write-only policy re-applied once per boot
  identity (`TestMplsRestartSimulation`, fake VPP; host run pending).
- [x] Label 5 / duplicate (table, label, eos) → 400 problem+json with `pointer` (API e2e on PostgreSQL + fake agent,
  schema tests).
- [x] `tools/ci.sh --base main` green (below).

CI_RESULT

## Shared hunks (hotspots; all under the task's own anchor unless noted)
- `packages/schema/src/domains/routing.ts`: `mpls: routingMpls,` under `// wave-BC: F-mpls-srmpls` (C1) **and one import
  line** `import { routingMpls } from './ext/mpls-srmpls.js';` between the `ui.js` and `vrfs.js` imports (imports have no
  anchor; F-vrf-static-ecmp inserts after the `vrfs.js` line, one unchanged line apart).
- `packages/schema/src/semantic/index.ts`: import + `...mplsSrmplsValidators,` under the two anchors (C2).
- `packages/schema/src/index.ts`: `export * from './domains/ext/mpls-srmpls.js';` (C3).
- `packages/proto/vrx/v1/dataplane.proto`: `rpc MplsState` under the service anchor; `MplsConfig mpls = 15;` under the
  RoutingConfig anchor (both blank-line framed); messages in `// ----- F-mpls-srmpls -----` (C5).
- `docs/contracts/proto.md`: `### F-mpls-srmpls: MplsState` appended at the end (no wave-BC anchor there) (C6).
- Generated (C7, regenerate on conflict): `apps/agent/gen/**`, `packages/proto/gen/ts/**`,
  `packages/api-client/src/generated/schema.d.ts`, `apps/cli/internal/api/operations_gen.go`.
- `apps/agent/internal/subsystems/subsystems.go`: 7 descriptor-name lines in `Domains[Routing]` and the
  `registerMplsSrmpls` call (3 lines, it returns an error) under the anchors (A1).
- `apps/agent/internal/agent/projection.go`: `desired.MplsSrmpls(p, ds, in, vrfID)` and the 3-line
  `desired.AssembleMplsSrmpls` call under the two anchors (A2).
- `apps/agent/internal/descriptors/core/coretest/fakevpp.go`: **one line in `New()`** (`v.installMplsSrmpls()`), no anchor
  exists — becomes a `RegisterExtension` line in my file with TD-23 (questions Q5).
- `apps/api/src/app.module.ts`: import + 2 spreads (P1); `apps/api/src/agent/agent.client.ts`: 2 type imports +
  `mplsState()` (P4); `apps/api/src/testing/fake-agent.ts`: `...mplsSrmplsFake(this),` under the anchor (P5) **and one
  import line** after `import { dirname } from 'node:path';` (the same spot F-vrf-static-ecmp uses: a trivial union).
- `apps/web/src/router.tsx` (W1), `nav/nav.ts` + `nav/nav.test.ts` (W2: a `routing`-group NavItem `mpls`,
  `labelKey: 'mpls-srmpls:nav'`), `i18n.ts` (W3: 2 imports + 3 lines).
- TD-11c (not on main): remove `mpls.NameTunnel` from `knownAliasCreatorGaps` at the second merge (questions Q4).

## For F-mpls-ldp (seeded anchors)
- `packages/schema/src/domains/ext/mpls-srmpls.ts` → `// wave-BC: F-mpls-ldp` inside `MplsSchema` (plain object).
- `dataplane.proto` → `MplsConfig`: `// 10 reserved: ldp (F-mpls-ldp)` + `// wave-BC: F-mpls-ldp`.
- `apps/web/src/domains/routing/mpls-srmpls/tabs.ts` → `// wave-BC: F-mpls-ldp` at the end of `mplsTabs`.
- `apps/agent/internal/desired/mpls_srmpls.go` → `// wave-BC: F-mpls-ldp` in `MplsNeedsTableZero` (LDP labels live in
  table 0).
- State routes `/api/v1/state/routing/mpls/{fib,tunnels}`; table-0 answer: questions Q3.

## Decisions (for the LOG)
1. **Table 0** (questions Q3): declared when the MPLS configuration needs it; the D-071 role decides create (globals
   owner) vs require (every other agent) — `mpls.NewTableFor`/`RegisterFor`. Options: declare whenever `routing.mpls` is
   non-empty / declare when needed + role in the descriptor (chosen) / role read from the environment in the projection.
2. **Pop-and-lookup paths** (`paths[].vrf`) added to the prompt's path model — needed for the "basic L3VPN" termination
   of D2.8 and supported by DF-7's path codec (`TableID`). Options: leave out / add (chosen).
3. **`payload`** (EOS payload, optional, documented default) added to label routes — VPP needs one for every EOS route.
   Options: derive only from next hops (breaks pop-and-lookup to IPv6, pseudowires) / optional field with a default
   (chosen; the assembler omits it when equal to the default).
4. **MPLS tunnel alias**: `KeyProvider` rather than `iface.RegisterKind` (questions Q4).
5. **`MplsState`**: one RPC with `view` "fib"|"tunnels", `MplsState*` messages; FIB paged in the agent with a
   100 000-entry window and one walk at a time (D-132). Options: two RPCs / one with a view (chosen, the allocation names
   one RPC).
6. **UI writes** one RFC 7386 merge patch of `/routing` computed from the candidate (deleted record entries → `null`,
   arrays whole) — no new API route.

## Out of scope / left undone
- Host run (TD-25) and the table-0 / SR-MPLS host run (manager window); real-stack screenshots.
- LDP, L3VPN/BGP labels, RSVP-TE, SR-TE color steering (endpoint-color is registered, in no domain), SRv6, MPLS
  multicast, EXP marking, OSPF/IS-IS SR extensions — as the prompt lists.
- Addressing an MPLS tunnel / IP static routes through it (needs A3, questions Q8); a CLI `show mpls` (questions Q9).

## Open questions
`docs/status/tasks/F-mpls-srmpls-questions.md` — Q1 contract review, Q2 schema examples regex, Q3 table 0 (incl. an
unnamed table 0 on a real box), Q4 TD-11c allowlist line, Q5 TD-23 hook, Q6 id range, Q7 write-only drift, Q8 tunnels
as interfaces, Q9 CLI, Q10 walk limiter, Q11 lcp MPLS sync (F-mpls-ldp), Q12 payload canonical form, Q13 scratchpad.
