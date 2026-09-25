# F-qos-flat — flat QoS: policers, rate limits, QoS record/store, egress maps, marking (WBS D7.8; HQoS = V3, excluded)

Branch `task/F-qos-flat` (worktree `/root/ngfw-wt/F-qos-flat`), base `main@1d3ccf31`, slot 1 (`w1`, id range
1000–1999), daemon-owner: none. **Host runs on the shared VPP were closed for this task (manager: until TD-25 merges):**
everything below that needs VPP is proven on the coretest VPP model and the fake agent; the host test is written and
waits (`test/topology/qos-flat`, Q13).

## What was built

| layer | what |
|---|---|
| contract | `rpc QosPolicerState`, `rpc QosPolicerReset` + `QosPolicerStateRequest/Response`, `QosPolicerStatus`, `QosPolicerCounter`, `QosPolicerResetRequest/Response` (F-qos-flat proto section; `QosService` 5 / `QosPolicer` 13 / `QosInterface` 7 unused — no gap); semantic rule `services.qos-flat-store-source` (`packages/schema/src/semantic/qos-flat.ts`); proto fixture `qos-flat-full.json` — `F-qos-flat-contract.md` |
| agent | `Domains["services"]` = policer.policer, policer.interface, qos.egress-map, qos.record, qos.store, qos.mark, qos.meta. `desired/qos.go` projects `services.qos` (policers; shapers as 1r2c egress policers `shaper:<name>`, burst = burstBytes or ≈10 ms ≥ 3000 B, exceed → drop; maps with the document id or the lowest free id of the agent's range; record / store (ip) / mark; write-only policer attachments marked `agent.write-only`) and assembles Retrieve (`AssembleQoS`). `subsystems/qos.go`: `policer.Register` + `qos.Register` (never `df7/registry.Register`) + `qos.meta`, with the persisted claim store, the DF-7 BootStore, the id range (`df7.WithIDRange`) and DF-2's classify store (`df7.WithClassifyTables`); no globals (D-071). `agent/rpc_qos.go`: QosPolicerState (DF-7 policer walk + `/net/policer/*` stats-segment counters), QosPolicerReset (`policer_reset`), one walk at a time (D-132). |
| DF-7 gaps (owned, gap-only) | TD-11b ownership declarations for all 8 policer/qos descriptors; claim-first Creates; `policer.States` / `policer.ResetIndex`; **attachment re-point** after a policer re-creation (VPP binds by pool index, `policer_del` leaves it dangling); new `qos.meta` document record (names/ids of maps, descriptions — D-073b) |
| API | `GET /api/v1/state/services/qos/policers?name` (VPP state joined with the running config: attachments, configured/present), `POST /api/v1/actions/qos/policers/{name}/reset` (operator, audited, 404 unknown) — `features/qos-flat`; fake agent handlers; e2e; api-client + CLI operation table regenerated |
| UI | Services → QoS tab: Policers (counters column, status, reset, rate/burst helper), Rate limits (egress) with the V3 drop-not-queue caveat, Marking maps (compact grid: 64 DSCP / 8 PCP / 8 EXP / 256 ext cells), Interface attachments; candidate edits through the generic pointer routes; state poll 30 s + Refresh (D-132); en + fa |
| docs | `docs/user/services/qos-flat.md`, `docs/agent/descriptors/policer.md` + `qos.md` (F-qos-flat additions), `docs/vpp-code-track.md` V-new (F-qos-flat), `docs/contracts/proto.md` §11 |
| tests | agent: `desired/qos_test.go`, `agent/rpc_qos_test.go` (coretest model `coretest/qos_flat.go`), `policer/qosflat_test.go`, `qos/qosflat_test.go`; schema `semantic/qos-flat.test.ts`; API `features/qos-flat/model.test.ts` + `test/e2e/qos-flat.e2e.test.ts`; web `qos-flat/model.test.ts` + `QosPage.test.tsx`; host `test/topology/qos-flat` (pending) |

## Acceptance — evidence

Host runs on the shared VPP are **closed until TD-25 merges** (manager addendum, 2026-09-25): nothing was run on
`/run/vpp/api.sock`, no VPP object was created, `NRestarts` untouched by this task. The acceptance items are proven on the
coretest VPP model (it models VPP 26.06's behaviour: feature stacking on a repeated apply, the out-of-bounds un-apply,
binding by pool index, reference-counted record/store, ip-only store) and on the fake agent + real PostgreSQL; the host
proof is `test/topology/qos-flat/run.sh` (written, compiles, skips without `VRX_INTEGRATION`; see "Pending").

| acceptance item | status | evidence |
|---|---|---|
| `vppctl show policer`, `show qos egress map` / `show qos mark` reflect the committed config | **pending TD-25** (model-proven) | `TestQoSApplyRetrieveRollback`: after the commit the model holds `w7:gold`, `w7:pps`, `w7:shaper:up` (1r2c, cir 50000, cb 62500, exceed drop), maps 7000 (auto) + 7001 (fixed), mark loop7102/ip → 7001, records, store; Retrieve equals the document minus the write-only attachments. Host test prints the five `vppctl show` outputs |
| agent restart → policers, maps, marks back within 30 s; attachments applied exactly once (D-076 test on the fake) | **done on the model** | `TestQoSAgentRestartRecreates` (agent stopped, `w7:gold` + map 7000 deleted behind its back, new agent on the same state dir → resync recreates both, every attachment still exactly one feature instance, and the attachment re-pointed to the re-created policer's new pool index); `TestQoSApplyRetrieveRollback` (a repeat commit + two resyncs: feature count stays 1); `TestQoSVPPRestart` (new boot identity + VPP tables gone → applied once more) |
| rollback removes policers/maps/marks (Retrieve); write-only attachments removed only if applied in this VPP lifetime | **done on the model** | `TestQoSApplyRetrieveRollback` (rollback → model empty, Retrieve `services.qos = {}`, qos.meta file gone, un-applies sent, `UnseenUnapplies() == 0`); `TestQoSVPPRestart` (after a second VPP restart a rollback sends no un-apply at all: `UnseenUnapplies() == 0`) |
| `store.source: vlan` → 400 problem+json with `pointer` | **done** (real API + PostgreSQL) | e2e `store.source on loop1002 = vlan → 400 problem+json at …/store/source` (validate and commit, `tier: semantic`, never sent to the agent) |
| `tools/ci.sh --base main` green | **done** | below |

### Unit / model / e2e runs (2026-09-25, this worktree)

```
$ go test -count=1 -v ./internal/agent/ -run 'QoS|SumPolicer'
--- PASS: TestQoSApplyRetrieveRollback (0.11s)
--- PASS: TestQoSAgentRestartRecreates (0.03s)
--- PASS: TestQoSVPPRestart (0.05s)
--- PASS: TestQoSDryRun (0.01s)
--- PASS: TestQoSPolicerRPCs (0.07s)
--- PASS: TestSumPolicerCounters (0.00s)
ok  	ngfw/agent/internal/agent	0.311s
$ go test -count=1 -v ./internal/desired/ ./internal/descriptors/policer/ ./internal/descriptors/qos/ -run '…'
--- PASS: TestShaperBurstBytes · TestQoSProjection · TestQoSAssembleRoundTrip · TestQoSAssembleRetrieve · TestQoSProjectionErrors · TestServicesUnsupported
ok  	ngfw/agent/internal/desired	0.113s
--- PASS: TestAttachments · TestOwnershipDeclared · TestAttachmentClaimFirst · TestStatesAndResetIndex · TestAttachmentRepointsAfterPolicerLoss
ok  	ngfw/agent/internal/descriptors/policer	0.040s
--- PASS: TestOwnershipDeclared · TestClaimFirst · TestMetaRecord
ok  	ngfw/agent/internal/descriptors/qos	0.036s

$ (packages/schema) vitest run src/semantic/qos-flat.test.ts
 ✓ src/semantic/qos-flat.test.ts (7 tests) 210ms        Tests  7 passed (7)
$ (apps/web) vitest run src/domains/services/qos-flat/ src/nav/
 ✓ src/nav/nav.test.ts (5 tests) · ✓ qos-flat/model.test.ts (12 tests) · ✓ qos-flat/QosPage.test.tsx (5 tests)   Tests  22 passed (22)
$ (apps/api) eval "$(tools/lab env 1)"; vitest run -c vitest.e2e.config.ts test/e2e/qos-flat.e2e.test.ts --reporter=verbose
 ✓ store.source on loop1002 = vlan → 400 problem+json at …/store/source (services.qos-flat-store-source)
 ✓ store.source on loop1001 = mpls → 400 problem+json at …/store/source (services.qos-flat-store-source)
 ✓ mark without map and shaper + policer.output are 400 with pointers at edit time
 ✓ a valid flat QoS configuration commits, reaches the agent and shows in the policer state
 ✓ a configured policer VPP lacks is listed with present:false; a VPP-only one with configured:false
 ✓ reset: operator 200 (audited), unknown 404, bad name 400, readonly 403
 ✓ an agent without the RPCs answers 501 (no fake data)
 ✓ rollback to the revision before QoS empties services.qos on the agent
      Tests  8 passed (8)
e2e teardown: deleted 10 Valkey keys vrx:w1:e2e:* in db 1
ok     nothing named vrx_w1 / vrx_w1 remains
```

**Every new test fails on the base.** The new files reference symbols, routes and files that do not exist on `main`
(build / 404 failures). The behavioural gaps were also checked against main's descriptor code (a copy of apps/agent with
main's `policer/attach.go` and `qos/qos.go` and the new tests):

```
--- FAIL: TestAttachmentClaimFirst
    qosflat_test.go:121: a refused claim must fail before VPP is touched: claim eth0 for policer.interface/eth0/input: disk full, 1 applies
--- FAIL: TestAttachmentRepointsAfterPolicerLoss
    qosflat_test.go:235: re-point: ops [] stack 1 bound 0 (want 1)
--- FAIL: TestClaimFirst
    qosflat_test.go:101: qos.record: refused claim must fail before VPP: claim eth0 for qos.record/eth0/ip: disk full, 1 calls
    qosflat_test.go:101: qos.store: refused claim must fail before VPP: … 1 calls
    qosflat_test.go:101: qos.mark: refused claim must fail before VPP: … 1 calls
```
(`TestOwnershipDeclared` fails on main with persist.ErrUndeclared: the DF-7 descriptors had no TD-11b declaration, so
registering them would have made the product agent refuse to start.) The two schema-tier pins in
`semantic/qos-flat.test.ts` pass on main by design (they pin existing rules, Q2).

### CI

```
$ TMPDIR=/tmp/g-w1 tools/ci.sh --base main          (tip 28ea8d20; log /root/ngfw-wt/logs/F-qos-flat-ci-1.log)
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   1m40s
  forbidden patterns (+ gitleaks)                    0m04s
  packet-trace ban on the shared VPP (D-128)         0m01s
  lint · typecheck · unit tests · build (turbo)   3m19s
  apps/agent: make lint test build                   1m11s
  apps/cli: make lint test build                     0m18s
  test/ Go modules, unit mode (test/integration/smoke test/topology/interfaces test/topology/qos-flat)   0m11s
  deploy/vpp: shellcheck + apply-startup fake-host harness   4m52s
  mode quick · wall time 11m40s · logs /root/ngfw-wt/logs/ci/F-qos-flat-20260925-111215-1052055

CI GATE PASSED
```
The final commit (status docs + the vlan e2e case) is re-checked at the end: see "Final CI" below.

## Obligations from the decisions (each with where it is honoured)

| decision | how | evidence |
|---|---|---|
| D-104 use DF-7's descriptors, do not rebuild | `policer.Register` + `qos.Register` from `subsystems/qos.go`; only gap fixes in the DF-7 packages, each with a named test | `policer/qosflat_test.go`, `qos/qosflat_test.go` |
| D-063 / D-076 / D-080 write-only attachments, applied once per boot identity, never un-applied unless applied in this VPP lifetime; the fake models duplicate apply | `policer.interface` keeps its applied-once record (now with the bound pool index); coretest counts feature instances and out-of-bounds un-applies; DryRun `agent.write-only` notes | `TestQoSApplyRetrieveRollback`, `TestQoSVPPRestart`, `TestAttachmentRepointsAfterPolicerLoss`, `TestQoSDryRun` |
| V3 shaper = egress policer (1r2c, cir = rateKbps, burst = burstBytes or ≈10 ms, exceed → drop) named `shaper:<name>`, documented as drop-based | `desired.QosShaperSpec`, `ShaperBurstBytes` (floor 3000 B, Q5); UI "Rate limits (egress)" + caveat; user guide section "Rate limits are drop-based" | `TestQoSProjection`, `TestShaperBurstBytes` |
| D-071 egress-map ids by id range (`df7.WithIDRange`), no QoS globals | registration passes `Wiring.IDRange()` to the descriptors and the projection; empty range = no map (fail closed, Q9); nothing global is set | `TestQoSProjectionErrors` (out of range, exhausted, empty range), `TestQoSApplyRetrieveRollback` (ids 7000/7001 in 7000–7999) |
| semantic rules: store.source ip-only, mark requires map, shaper vs policer.output — in `semantic/qos-flat.ts`, ids `services.qos-flat-…` | `services.qos-flat-store-source`; the other two are schema-tier and pinned (Q2) | `semantic/qos-flat.test.ts`, e2e |
| D-064 crash → opt-in + V-item | no host run, no crash; two VPP findings from reading the source recorded as V-new (F-qos-flat) | `docs/vpp-code-track.md` |
| TD-11b (addendum) declarations + claim-first | `ownership.go` in both packages, `qos.meta` `CheckPersistent`; claim-first in policer.interface/classify, qos.record/store/mark | `TestOwnershipDeclared` ×2, `TestAttachmentClaimFirst`, `TestClaimFirst`; the product wiring passes `subsystems.RequirePersistent` in every agent test |
| TD-8 seams (addendum): Wiring.IDRange, no forks | the id range comes from `Wiring.IDRange`; no new seam | `subsystems/qos.go` |
| TD-23 (addendum) coretest hook in an own file | `coretest/qos_flat.go` + one `v.installQoSFlat()` line in `New()` (Q8) | — |
| D-132 no UI timer < 30 s on VPP walks, Refresh button, one walk at a time in the agent | `QOS_POLL_MS = 30_000` + Refresh; `qosWalk` (one slot, UNAVAILABLE after 3 s) | `TestQoSPolicerRPCs` (busy walk → UNAVAILABLE), web `model.test.ts` pins the poll period |
| D-137/D-139, D-128, D-126 | no dns.api, no trace (the host test refuses any non-`show` vppctl), no classify sweep | `test/topology/qos-flat/qos_test.go` `vppctl()` |
| WEB-1 (addendum) never `dropPhantomOptionals` | not used; attachment dialog is a custom form until WEB-1 (Q14) | — |

## Shared hunks (hotspots)

| id | file | hunk | anchor |
|---|---|---|---|
| A1 | `apps/agent/internal/subsystems/subsystems.go` | `"services": {policer.NamePolicer, … qos.NameMeta}` in `Domains` | `// wave-BC: F-qos-flat` |
| A1 | same | `if err := w.registerQoS(r); err != nil {…}` at the end of `register()`; imports `descriptors/policer`, `descriptors/qos` | **unanchored** (Q1) |
| A2 | `apps/agent/internal/agent/projection.go` | `desired.QoS` + `desired.ServicesUnsupported` in `project()`; `desired.AssembleQoS` in `assemble()` | **unanchored** (Q1) |
| A6 | `apps/agent/internal/descriptors/core/coretest/fakevpp.go` | `v.installQoSFlat()` in `New()` | not in the envelope — Q8 |
| (A5 test) | `apps/agent/internal/agent/service_test.go` | canonicalDoc `services.qos`, `implementedDomains()` ×2, `policer_dump_v2` ×3 | not in the envelope — Q8 |
| C2 | `packages/schema/src/semantic/index.ts` | import + `...qosFlatValidators` | **unanchored** (Q1) |
| C4 | `packages/proto/test/fixtures/qos-flat-full.json` | new file | — |
| C5 | `packages/proto/vrx/v1/dataplane.proto` | 2 RPCs | `// wave-BC: F-qos-flat` (service) |
| C5 | same | 6 messages | `// ----- F-qos-flat -----` section |
| C6 | `docs/contracts/proto.md` | `### F-qos-flat: QosPolicerState, QosPolicerReset` | **unanchored**, end of §11 (Q1) |
| C7 | generated | `apps/agent/gen/**`, `packages/proto/gen/ts/**`, `packages/api-client/src/generated/schema.d.ts`, `apps/cli/internal/api/operations_gen.go` (`docs/user/cli/reference.md` unchanged) | regen |
| P1 | `apps/api/src/app.module.ts` | import, `...qosFlatFeature.controllers`, `...qosFlatFeature.providers` | `// wave-BC: F-qos-flat` ×3 |
| P3 | `apps/api/src/features/qos-flat/qos-flat.controller.ts` | static `POST /api/v1/actions/qos/policers/:name/reset` (own controller, actions/** untouched) | — |
| P4 | `apps/api/src/agent/agent.client.ts` | 4 type imports; `qosPolicerState`, `qosPolicerReset` | `// wave-BC: F-qos-flat` ×2 |
| P5 | `apps/api/src/testing/fake-agent.ts` | `...qosFlatFake(this),` (+ its import at the top, unanchored like F-kea's) | `// wave-BC: F-qos-flat` |
| W2 | `apps/web/src/nav/nav.ts`, `nav.test.ts` | `'services'` in `BUILT_DOMAINS` / expected list | **unanchored** (Q1, Q4) |
| W3 | `apps/web/src/i18n.ts` | 2 imports, `NAMESPACES`, `en`, `fa` | `// wave-BC: F-qos-flat` ×4 |
| — | `apps/web/src/domains/services/tabs.ts` | `{ id: 'qos', labelKey: 'qos-flat:tab', … }` (+ `import { lazy }` at the top) | `// wave-BC: F-qos-flat` |
| A7 | `docs/vpp-code-track.md` | `### V-new (F-qos-flat)` (qos mark survives delete; policer_del leaves bindings dangling) | appended |

## Decisions taken (for the LOG; options in F-qos-flat-questions.md)

1. Mark-requires-map and shaper-vs-output stay schema-tier, pinned by tests; one new semantic rule (Q2).
2. Shapers kept as "Rate limits (egress)"; derived burst = max(≈10 ms, 3000 B) (Q5).
3. New agent-local `qos.meta` record for names/descriptions/explicit ids (Q11).
4. Policer attachments record the bound pool index and are re-pointed after a policer re-creation (VPP finding, Q7b).
5. No id range → QoS registers fail-closed (maps refused with a pointer) instead of failing the agent (Q9).
6. Map ids without `id`: lowest free id of the agent's range in name order (renumbering documented in the user guide).
7. Write-only attachments: DryRun rule `agent.write-only` (same id as F-rpf-adl-pbr), whole interface entry when it has
   nothing retrievable (Q6).

## Pending (host runs closed until TD-25) and out of scope

- `test/topology/qos-flat/run.sh` on slot 1: `vppctl show policer`, `show qos egress map`, `show qos mark`,
  `show qos record`, `show qos store`, `show interface features` pasted; agent restart ≤ 30 s; rollback; NRestarts
  before/after. Then the screenshots of Services → QoS against the real endpoint (headless Chrome approach of P07a/P08).
- Counters with traffic (optional, V19/V24 rules) — not attempted.
- Out of scope (prompt): HQoS, queues/schedulers (V3); classifier policing UI (F-rpf-adl-pbr); ACL-matched marking
  (F-acl); MPLS EXP on label routes (F-mpls-srmpls); `policer.bind` tuning (no workers); traffic generators.

## Cleanup

No process of this task is running (the API e2e harness and the sub-workers' vitest runs ended; no agent, API or vite
was left); `vrx_w1` dropped by the e2e harness ("nothing named vrx_w1 / vrx_w1 remains"); no Valkey keys left
(`vrx:w1:e2e:*` deleted); no VPP object was ever created (host runs closed); no rig; build outputs (`dist/`,
`apps/agent/bin`, `apps/cli/bin`) removed at the end.
