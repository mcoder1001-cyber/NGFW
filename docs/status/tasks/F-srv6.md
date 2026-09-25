# F-srv6 — SRv6 policies, local SIDs, steering (status)

Branch `task/F-srv6` (worktree `/root/ngfw-wt/F-srv6`, slot 4), base `main@1d3ccf31`. Envelope:
`docs/status/tasks/F-srv6.envelope.md` · contract notes `F-srv6-contract.md` · questions `F-srv6-questions.md` ·
WIP log `F-srv6-wip.md`.

**Host runs are pending TD-25.** The manager closed host runs on the shared VPP until TD-25 merges (af_packet creates
fail until then). Everything except the host runs is built and verified: the fake VPP model (`coretest`), the API
with its fake agent, and the production web build against the real vrx-api. The host steps are written and listed
under "Pending host steps (after TD-25)".

## What was built

| layer | files | what |
|---|---|---|
| schema (contract) | `packages/schema/src/domains/ext/srv6.ts`, `semantic/srv6.ts` (+ `srv6.test.ts`) | `routing.srv6{encapSource, encapHopLimit, localSids, policies, steering}` (see F-srv6-contract.md); 11 rules `routing.srv6-*` |
| proto (contract) | `packages/proto/vrx/v1/dataplane.proto`, fixture `packages/proto/test/fixtures/srv6-full.json` | `RoutingConfig.srv6 = 17`, `Srv6Config/LocalSid/Policy/SidList/Steering`, rpc `Srv6State` + `Srv6State*` messages |
| agent | `internal/desired/srv6.go` | builder: routing.srv6 → `sr.localsid` / `sr.policy` / `sr.steering` / the two globals; every encap policy gets its own `encap_src` (policy override, else the global, D-074); agent-side validation with pointers and the same rule ids; write-only globals noted `agent.unsupported-field` (drift skips them). Assembler: Retrieve → routing.srv6 in canonical form, steering sorted (L3 by VRF then prefix, then L2) |
| agent | `internal/subsystems/srv6.go` (+ 5 names in `Domains[Routing]`, 1 call in `register()`) | `sr.Register` with `df6.WithClaims(Wiring.PairClaims("df6"))` (TD-11b) and `df6.WithGlobalsOwner(Env.GlobalsOwner)` (D-071); records the global encap source the globals owner applied (the assembler's `Srv6Env`); `Srv6State` read path (claimed objects + counters) |
| agent | `internal/agent/rpc_srv6.go` | `Srv6State` RPC: owner check, one SR walk in flight (3 s wait, then UNAVAILABLE — D-132), claimed objects only |
| agent (DF-6 gaps) | `descriptors/sr/register.go`, `descriptors/sr/stats.go` | `sr.Global`: the df6 global setter / require variant with the TD-11b `RecordsNoOwnership` declaration (df6 singletons declare nothing; the guard refused to start); `sr.LocalSidCounters` (`sr_localsids_with_packet_stats_dump`) |
| agent (test model) | `descriptors/core/coretest/srv6.go` (+1 line in `fakevpp.go` `New()`) | VPP 26.06 sr model: SIDs + counters, policies, steering, globals; counts the calls that crash 26.06 (unchecked `fib_table_find`, dangling steering) and SR entries left in deleted tables (V15) |
| API | `apps/api/src/features/srv6/**` | `GET /api/v1/state/srv6` (`Srv6_state`): the agent's Srv6State joined with the running VRF names and a `configured` flag; the fake agent's Srv6State |
| UI | `apps/web/src/domains/vpn/srv6/**`, `locales/{en,fa}/srv6.json` | SRv6 tab on the VPN page: Local SIDs (counters), Policies (+ VPP-wide settings card, SID-list editor), Steering; SchemaForm; canonicalises addresses; saves steering in Retrieve order; state every 30 s + Refresh (D-132); proxy note |
| docs | `docs/user/vpn/srv6.md` (+ screenshots), `docs/agent/descriptors/sr.md`, `docs/contracts/proto.md` "F-srv6: Srv6State", `docs/vpp-code-track.md` "V-new (F-srv6)" | L3VPN-over-SRv6 example, CLI/REST equivalent, validation, limits |
| host tests | `apps/agent/internal/agent/srv6_integration_test.go`, `test/topology/srv6/stack.sh` | written; run after TD-25 |

Not built (have-not): **End.AD / End.AM / End.AS** proxies — VPP 26.06 `src/plugins/srv6-{ad,am,as}` have no `.api`
file and their behaviours are not in `sr_types.api` (CLI only; V-new F-srv6). **SRv6-mobile** (`sr_mobile`, D-074/D-085:
no delete message, clashes with `sr.policy`) — a vpp-code-track candidate, listed in V-new F-srv6. uSID
(`sr_localsid_add_del_v2`), path tracing (`sr_pt`), BGP/IS-IS SRv6 signalling — out of scope.

## Acceptance

| item | state | evidence |
|---|---|---|
| after commit `Retrieve()` == desired; `vppctl show sr …` lists them | fake VPP ✔ · host **pending TD-25** | `TestSrv6ApplyRetrieveRollback` (below); host: `TestSrv6OnHost` logs Retrieve + `show sr localsids/policies/steering-policies` + `show ip6/ip fib table` |
| agent restart → objects back ≤ 30 s; claimed objects not re-added | fake VPP ✔ · host **pending** | `TestSrv6RestartSimulation`: converged restart sends no SR write; after a simulated loss the resync re-creates the 10 SR objects |
| rollback removes steering → policies → SIDs, no leaked routes | fake VPP ✔ · host **pending** | rollback call order below; model's `Leaks()` (SR entries in deleted tables) and `Crashes()` empty; host: `show ip6 fib` / `show ip fib` scan for the slot's SR range |
| encap policy without encapSource → 400 problem+json with a pointer | ✔ | API e2e (PostgreSQL + fake agent) and the screenshot stack run below |
| `tools/ci.sh --base main` green | ✔ | "CI (final)" below |

## Evidence (pasted output)

### Agent — fake VPP (coretest SR model)

```
$ go test -count=1 -v -run 'TestSrv6' ./internal/agent/ ./internal/desired/ ./internal/subsystems/
    srv6_test.go:140: apply: {"created":16}; Retrieve routing.srv6 == desired
    srv6_test.go:160: rollback VPP calls: steer-del steer-del steer-del policy-del policy-del sid-del sid-del sid-del sid-del sid-del table-del table-del
--- SKIP: TestSrv6OnHost (0.00s)
--- SKIP: TestSrv6GlobalsOnHost (0.00s)
--- PASS: TestSrv6ApplyRetrieveRollback (0.07s)
--- PASS: TestSrv6RestartSimulation (0.04s)
--- PASS: TestSrv6NeverTakesOverForeignSid (0.01s)
--- PASS: TestSrv6Validation (0.03s)
--- PASS: TestSrv6GlobalsNonOwner (0.00s)
--- PASS: TestSrv6GlobalsOwner (0.01s)
--- PASS: TestSrv6State (0.04s)
ok  	ngfw/agent/internal/agent	0.256s
--- PASS: TestSrv6Builder (0.01s)
--- PASS: TestSrv6BuilderSkipsOtherDomains (0.00s)
--- PASS: TestSrv6BuilderErrors (0.03s)
--- PASS: TestSrv6Assemble (0.00s)
--- PASS: TestSrv6RoundTrip (0.00s)
ok  	ngfw/agent/internal/desired	0.086s
--- PASS: TestSrv6Wiring (0.00s)
ok  	ngfw/agent/internal/subsystems	0.054s
--- PASS: TestGlobalsDeclareNoOwnership (0.00s)
--- PASS: TestLocalSidCounters (0.00s)
ok  	ngfw/agent/internal/descriptors/sr	0.022s
```

What they prove: `TestSrv6ApplyRetrieveRollback`: Retrieve == desired, an idempotent re-apply sends only dumps, and
the rollback deletes steering → policies → SIDs before the VRF tables, with no crash call and no leaked SR entry.
`TestSrv6RestartSimulation`: a converged restart re-adds nothing (claims persisted in `claims-df6-w7.json`); after
`SR().DeleteAll()` the resync re-creates all 10 SR objects and Retrieve == desired. `TestSrv6NeverTakesOverForeignSid`:
an unclaimed SID fails the transaction (`not ours`), and the rollback leaves only the foreign SID. `TestSrv6Validation`:
7 cases with pointer and rule id, VPP untouched. `TestSrv6GlobalsNonOwner` / `…Owner`: the D-071 split; the inherited
source is not reported, DryRun notes both globals, and removing them resets VPP's defaults. `TestSrv6State`: claimed
objects, counters, sorting; an owner mismatch gives INVALID_ARGUMENT, a busy walk or a disconnected VPP gives UNAVAILABLE.
The whole agent module: `go test ./...` 91 packages ok; `golangci-lint` 0 issues.

The tests fail on the base: main's `apps/agent` plus these four test files (scratch copy):

```
internal/descriptors/sr/gaps_test.go:45:20: undefined: sr.Global
internal/descriptors/sr/gaps_test.go:68:17: undefined: sr.LocalSidCounters
internal/desired/srv6_test.go:80:2: undefined: Srv6
internal/desired/srv6_test.go:217:2: undefined: AssembleSrv6
internal/desired/srv6_test.go:240:21: ds.GetRouting().GetSrv6 undefined (type *vrxv1.RoutingConfig has no field or method GetSrv6)
```

and each of these mutations makes a test fail (then reverted):

| mutation | fails |
|---|---|
| assembler reports an inherited source (`!= applied` → `!= applied+"x"`) | `TestSrv6GlobalsOwner` (routing.srv6 diff) |
| `df6.WithClaims(w.IfaceClaims())` added | every agent test: "subsystems: refusing to start: … df6: claim store binds claims to interface indexes … df6 sr.localsid: *subsystems.IfaceClaims" |
| `sr.Global` loses `RecordsNoOwnership` | the guard refuses to start the agent |
| steering sorted L2 first | `TestSrv6ApplyRetrieveRollback` |
| builder drops the global-source fallback | `TestSrv6GlobalsOwner` |

### Schema

```
$ pnpm exec vitest run        (packages/schema)
 Test Files  38 passed (38)
      Tests  1237 passed (1237)
$ go test -count=1 -v -run 'TestSchemaProtoDrift$|TestDesiredState' ./internal/contracttest/
--- PASS: TestDesiredStateMirrorsRootKeys (0.01s)
--- PASS: TestSchemaProtoDrift (0.06s)
```
(`semantic/srv6.test.ts`: 23 tests covering defaults, limits and every rule id with its pointer and message.)

### API (unit + e2e on slot 4: PostgreSQL vrx_w4 + the fake agent)

```
 ✓ srv6.controller.test.ts > F-srv6 API helpers > maps table ids to the running VRF names (0 = default, unknown = the id)
 ✓ srv6.controller.test.ts > F-srv6 API helpers > collects the configured keys canonically
 ✓ srv6.controller.test.ts > F-srv6 API helpers > joins the agent state with the running configuration
 ✓ srv6.controller.test.ts > F-srv6 fake agent (Srv6State) > reports what the fake applied, sorted like the agent, with the resolved encap source
 ✓ srv6.controller.test.ts > F-srv6 fake agent (Srv6State) > refuses another owner
$ eval "$(tools/lab env 4)"; pnpm exec vitest run -c vitest.e2e.config.ts test/e2e/srv6.e2e.test.ts --reporter=verbose
 ✓ … > encap policy without any encapSource → 400 problem+json pointing at the policy (D-074) 199ms
 ✓ … > schema errors are 400 at edit time (17 SIDs, a proxy behaviour, a non-IPv6 SID) 86ms
 ✓ … > readonly may not edit routing.srv6 (403) 10ms
 ✓ … > commit → the fake agent has it; GET /state/srv6 joins the running VRF names 240ms
 ✓ … > rollback to the revision before the SRv6 commit removes it (agent and state) 143ms
drop   database vrx_w4 · drop role vrx_w4 · ok nothing named vrx_w4 / vrx_w4 remains
```

### UI (unit) and screenshots

```
 ✓ Srv6Page.test.tsx > SRv6 model > canonicalises addresses before saving (routing.srv6-canonical)
 ✓ Srv6Page.test.tsx > SRv6 model > saves steering in Retrieve order: L3 by VRF then prefix (code-unit order), then L2 by interface
 ✓ Srv6Page.test.tsx > SRv6 model > checks segment lists (≤ 16 IPv6 SIDs, weight 1–65535) and reorders SIDs
 ✓ Srv6Page.test.tsx > SRv6 model > D-132: the state is not refetched on focus or remount within 30 s
 ✓ Srv6Page.test.tsx > SRv6 tab > lists local SIDs with counters, policies with segment lists, steering; the proxy note
 ✓ Srv6Page.test.tsx > SRv6 tab > SID-list editor: add, reorder and save a policy (canonical, through PATCH /config/routing)
 ✓ Srv6Page.test.tsx > SRv6 tab > adds a steering entry and saves the whole list in Retrieve order
 ✓ Srv6Page.test.tsx > SRv6 tab > renders in Persian (RTL) with the translated sub-tabs
      Tests  15 passed (15)   (with nav.test.ts)
```

Screenshots (interim, while host runs are closed): the production web build (`vite preview`, port 5400) against the
real vrx-api (`apps/api/dist`, port 3400, database vrx_w4srshot). The agent behind the API was its `FakeAgent`, served
on a unix socket with sample counters. The screenshot and runner scripts are kept outside the repo (P07a/P07b
practice). Every process was stopped by PID and the database was dropped:

```
11:30:49 commit: {"status":"applied","revision":1,"errors":null}
11:30:50 encap policy without source: 400 {"type":"https://vrx.dev/problems/validation","title":"Validation failed","status":400,"tier":"semantic",…,"errors":[{"pointer":"/routing/srv6/policies/fd00:4:bb::1/encapSource","message":"an encapsulating policy needs an outer source address: set encapSource here or routing.srv6.encapSource (VPP’s global default cannot be read back, D-074)"},{"pointer":"/routing/srv6/policies/fd00:4:bb::9/encapSource",…}]}
srv6-sids-en.png  html dir/lang=ltr/en  live="SID Behavior VRF Target Status Processed Dropped fd00:4:ff::1 end (PSP) default — Installed 5,120 pkts / 655,360 B 3 pkts / 384 B …" pageErrors=0
srv6-policies-en.png  html dir/lang=ltr/en  live="Binding SID Type Mode VRF Encapsulation source Segment lists Status fd00:4:bb::1 Default (weighted) Encapsulate default fd00:4::1 (global) …" pageErrors=0
srv6-sid-list-editor-en.png  html dir/lang=ltr/en  dialog="Policy fd00:4:bb::1 Type default …" pageErrors=0
srv6-steering-en.png  html dir/lang=ltr/en  live="Match VRF Traffic Binding SID Status 10.4.160.0/24 cust-a IPv4 fd00:4:bb::1 Installed fd00:4:160::/48 cust-a IPv6 fd00:4:bb::2 Installed loop461 — L2 …" pageErrors=0
srv6-sids-fa-rtl.png  html dir/lang=rtl/fa  live="SID رفتار VRF مقصد وضعیت پردازش‌شده دورریخته fd00:4:ff::1 end (PSP) default — نصب‌شده 5,120 بسته / 655,360 بایت …" pageErrors=0
srv6-sid-list-editor-fa-rtl.png  html dir/lang=rtl/fa  dialog="سیاست fd00:4:bb::1 نوع default …" pageErrors=0
11:31:14 stopped 1272920 1273427 1275563; database vrx_w4srshot dropped
```
Committed: `docs/user/vpn/img/srv6-{sids,policies,sid-list-editor,steering}-en.png`, `srv6-sids-fa-rtl.png` and
`srv6-sid-list-editor-fa-rtl.png`. The real-agent screenshots come from `test/topology/srv6/stack.sh <script> <dir>`
after TD-25.

## Pending host steps (after TD-25; the manager opens host runs)

1. `eval "$(tools/lab env 4)"; systemctl show vpp -p NRestarts` (baseline 2) — paste.
2. `cd apps/agent && VRX_INTEGRATION=1 go test -count=1 -v -run TestSrv6OnHost ./internal/agent/`. It takes the
   shared lab lock and uses owner `w4sr`, VRF table 4060, loop460/461 and SIDs in fd00:4::/48; it sends no packet and
   creates no af_packet interface. It pastes:
   - Retrieve == desired, and `vppctl show sr localsids` / `show sr policies` / `show sr steering-policies` /
     `show ip6 fib table 4060` / `show ip fib table 4060`;
   - Srv6State, and the pointer of the encap-without-source failure;
   - the loss simulation via binapi (steering → policies → SIDs, each dumped first, D-074) and the convergence time;
   - the converged restart with nothing re-added;
   - the rollback order, and "no SR route left" across `show ip6 fib` / `show ip fib`.
3. `test/topology/srv6/stack.sh <shots script> <out dir>`: the same through the API (commit, `GET /state/srv6`, drift,
   400, restart log excerpt, rollback to the baseline revision), with real-agent screenshots.
4. Optional, manager window only: `VRX_FSRV6_GLOBALS=1 VRX_INTEGRATION=1 go test -run TestSrv6GlobalsOnHost …`. It
   takes the exclusive `/run/lock/vrx-globals.lock`, records `show sr encaps source addr` / `hop-limit` first and
   restores them.
5. `systemctl show vpp -p NRestarts` after. Cleanup check: `show sr localsids` has no fd00:4: entry, table 4060 is
   gone, vrx_w4sr is dropped, `apps/agent/bin` is removed.

## Shared hunks (all under the F-srv6 anchors unless noted)

| file | hunk |
|---|---|
| `apps/agent/internal/subsystems/subsystems.go` | 5 names in `Domains[Routing]` (constants from `srv6.go`, so no import line) · 3 lines in `register()` (`w.registerSrv6(r)`) |
| `apps/agent/internal/agent/projection.go` | `desired.Srv6(p, ds, in, vrfID)` in `project()` · `desired.AssembleSrv6(ds, kvs, in, nameOf, subsystems.Srv6Env())` in `assemble()` |
| `apps/agent/internal/descriptors/core/coretest/fakevpp.go` | **no anchor:** one line `v.installSRv6()` in `New()` — becomes `RegisterExtension` in `coretest/srv6.go` once TD-23 is on main (Q8) |
| `packages/schema/src/domains/routing.ts` | `srv6: srv6Field,` · **outside the anchor:** one import line at the end of the import block (as F-rpf-adl-pbr) |
| `packages/schema/src/semantic/index.ts` | import + spread |
| `packages/schema/src/index.ts` | `export * from './domains/ext/srv6.js'` |
| `packages/proto/vrx/v1/dataplane.proto` | `Srv6Config srv6 = 17;` in `RoutingConfig` and `rpc Srv6State` in `service Dataplane` (framed by blank lines) · messages in `// ----- F-srv6 -----` |
| `docs/contracts/proto.md` | `### F-srv6: Srv6State` appended (§11 has no wave-BC anchors) |
| `docs/vpp-code-track.md` | `### V-new (F-srv6)` appended (the manager numbers it) |
| `apps/api/src/app.module.ts` | import · `...srv6Feature.controllers` · `...srv6Feature.providers` |
| `apps/api/src/agent/agent.client.ts` | `type Srv6StateResponse` import · `srv6State()` |
| `apps/api/src/testing/fake-agent.ts` | `srv6State: srv6FakeState(this)` · **outside:** one import line at the end of the imports (no import anchor; as F-wireguard) |
| `apps/web/src/i18n.ts` | 2 imports, `'srv6'`, `srv6: enSrv6`, `srv6: faSrv6` |
| `apps/web/src/domains/vpn/tabs.ts` | the `srv6` entry · **outside:** `import { lazy } from 'react'` at the top (the same line F-wireguard adds: trivial conflict) |
| `apps/web/src/nav/nav.ts` + `nav.test.ts` | `'vpn'` in `BUILT_DOMAINS` / the expected list (F-wireguard adds the same: a duplicate the manager drops) |
| generated (never hand-edited) | `apps/agent/gen/**`, `packages/proto/gen/ts/**`, `packages/api-client/src/generated/schema.d.ts`, `apps/cli/internal/api/operations_gen.go` (`Srv6_state`); `docs/user/cli/reference.md` unchanged; `sdk/python/vrx/_generated` not regenerated (Q9) |

## Decisions (with options) — for the LOG

1. **Per-policy `encapSource` override modelled.** Options: (a) global only (b) override plus global default (c)
   per-policy only. Chose (b): D-074 needs a source on every encap policy, and on a non-owner the write-only global can
   never be applied (D-071), so (a) would forbid encap policies on every slot agent. Q2.
2. **Steering is an array with a `type` discriminator (`l3`/`l2`).** Options: (a) flat object with optional fields (b)
   discriminated union (c) records keyed by vrf/prefix and by interface. Chose (b): it keeps the prompt's array shape,
   and presence stays clean (`l3` always has `vrf`, `l2` never has one). Retrieve returns the list sorted, and the UI
   saves it in that order.
3. **Canonical spelling is a validation rule** (`routing.srv6-canonical`). Options: (a) accept any spelling, which
   drifts on every Retrieve (b) refuse non-canonical text, with the canonical form in the message. Chose (b); the UI
   canonicalises.
4. **Inherited encap source in Retrieve.** Options: (a) always report the policy's effective source, which is false
   drift for policies that inherit it (b) leave it unset when it equals the global source this agent applied (recorded
   by the setter, `Srv6Env`). Chose (b). It is the value this process wrote (VPP has no getter), not an echo of
   desired objects.
5. **Globals on a non-owner fail the commit** (df6's require variant). Options: (a) fail with "configure it on the
   globals owner" (b) skip silently. Chose (a), D-071 as written.
6. **TD-11b declaration for the df6 globals in `descriptors/sr`** (`sr.Global` wrapper). Options: (a) df6 (read-only
   for this task) (b) a registry wrapper in subsystems (c) the sr package. Chose (c): any registration of the sr family
   passes the guard. F-lisp's enable singletons need the same, or df6 declares it once (Q6).
7. **Write-only globals noted as `agent.unsupported-field`** at `/routing/srv6/encapSource` and `/encapHopLimit` in
   DryRun, so `/state/drift` does not compare them (the F-unbound pattern). The agent still applies them.
8. **Interim screenshots against the real API with the API's FakeAgent** while host runs are closed; the real-agent
   screenshots are pending TD-25.
9. Limits: weight 1–65535, 1–64 segment lists per policy, 1–16 SIDs per list, hop limit 1–255. No `description`
   leaves: they are not VPP state, and D-073b would need service-side storage.

## Open questions

See `F-srv6-questions.md`:
- Q2: confirm the per-policy override.
- Q3: does the product need service chaining at all? It needs a VPP API patch.
- Q6: a df6-level TD-11b declaration for singletons.
- Q8: the coretest hook until TD-23 merges.
- Q9: sdk regeneration.
- Q10: interim screenshots.
