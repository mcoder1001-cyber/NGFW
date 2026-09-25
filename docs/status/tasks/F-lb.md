# F-lb — VPP load balancer (GRE / NAT / L3DSR / Maglev), tier T3

Branch `task/F-lb` (worktree `/root/ngfw-wt/F-lb`, slot 2, base `main@1d3ccf31`). Questions: `F-lb-questions.md`;
contract: `F-lb-contract.md`; WIP log: `F-lb-wip.md`.

**Status:** code, unit tests, fake-VPP agent tests, API e2e (slot DB) and CI done. **Host runs NOT done** — closed until
TD-25 merges (manager addendum); the opt-in host tests are written and listed under *Pending host steps*.

## What was built

| layer | files | what |
|---|---|---|
| schema (contract) | `packages/schema/src/domains/ext/lb.ts` (+ key line in `domains/services.ts`, export in `index.ts`), `semantic/lb.ts` (+ spread) | `services.lb{settings, vips{<name>}, natInterfaces[]}`; refinements: encap↔VIP family, server family = encap family, port↔protocol, powers of two, dscp/nat-only fields, NAT prerequisites (tcp/udp+port, targetPort, `natInterfaces` of the family), unique (prefix, protocol, port), VPP per-prefix rules, unique servers, unique (server, targetPort) across NAT VIPs, 0.0.0.0/8 reserved; semantic `services.lb-nat-interface-exists` |
| proto (contract) | `dataplane.proto` | `ServicesConfig.lb = 11` → `LbService/LbSettings/LbVip/LbServer/LbNatInterface`; RPCs `LbState`, `LbFlushVip` (+ `LbStateRequest/Response`, `LbVipState`, `LbServerState`, `LbFlushVipRequest/Response`); `docs/contracts/proto.md` § F-lb |
| agent — descriptors (gap-only) | `descriptors/lb/{ownership,state,gc}.go`, `lb.go` | TD-11b declarations; intf-nat claim first; `DumpASes`; `FlushVIP` fixed (ip46 layout + in-use guard); `GarbageCollect` (constant `lb vip 0.0.0.0/32 del` via `cli_inband`) + `GCSafe` |
| agent — projection | `desired/lb.go`, `desired/lb_services_seam.go` (merge seam, delete after F-rpf-adl-pbr) | `services.lb` → `lb.conf` (globals owner only), `lb.vip`, `lb.as`, `lb.intf-nat`; `agent.write-only` note; slot: settings → `agent.unsupported-field`; 0.0.0.0/8 → `services.lb-vip-reserved` |
| agent — wiring | `subsystems/lb.go` (+ `Services` domain in `subsystems.go`) | one `lb.Register` (+ `RegisterGlobals` in the globals owner); globals owner: successful `lb.vip`/`lb.as` Delete → debounced timer → one GC 65 s after the last delete; slots never |
| agent — RPCs | `agent/rpc_lb.go` | `LbState` (stored desired VIPs × lb_vip_dump × lb_as_dump, boot record = applied; one walk at a time, UNAVAILABLE after 3 s, D-132), `LbFlushVip` (stored VIP + boot record + server in use) |
| API | `apps/api/src/features/lb/{lb.controller,index,fake}.ts` (+ P1/P4/P5 lines) | `GET /api/v1/state/lb/vips` (status active/not-applied/missing/no-servers, servers in use/removed, removed copies), `POST /api/v1/actions/lb/vips/{name}/flush` (404/409/501 problem+json, audited) |
| web | `apps/web/src/domains/services/lb/*`, `locales/{en,fa}/lb.json` (+ tab, i18n, nav lines) | Services → *Load balancer* tab: VIP table with status chip + removed copies, server sub-table, Flush, Add/Edit (SchemaForm dialog), Delete, settings + NAT interfaces form, write-only/GC notice, Refresh + 30 s poll |
| docs | `docs/user/services/lb.md`, `docs/agent/descriptors/lb.md` (F-lb additions), `docs/vpp-code-track.md` (V20 follow-up), ALLOWLIST row | GRE and L3DSR examples, what it does not do, V20 caveats, CLI equivalent |

## Obligations (envelope) with evidence

| obligation | how | evidence |
|---|---|---|
| D-104 use DF-7's descriptors | `lb.Register`/`RegisterGlobals` only; gap edits named by tests | `gaps_test.go` (7 tests, fail on base — below) |
| D-063/D-076/D-080 write-only, records in the persisted DF-7 BootStore, `DumpVIPs` never Retrieve | descriptors unchanged in that respect; `CheckPersistent` requires the persisted BootStore (+ claims for intf-nat); `LbState` is a separate RPC | `TestOwnershipDeclared`, `TestRegisterGuardsEveryDescriptor` (product wiring passes its own guard), `TestLbApplyStateFlushRemove` (Retrieve has no `services.lb`) |
| D-071 `lb.conf` only in the globals owner | `registerLb`; projection gates `settings` | `TestLbSlotAgentNeverCollects` (no lb.conf registered), `TestLbProjection` (owner=false: no `lb.conf/global`, `agent.unsupported-field`), `TestLbApplyStateFlushRemove` (0 `lb_conf` calls) |
| D-090 (2) GC once, globals owner, constant `cli_inband` + ALLOWLIST row; slot never | `subsystems/lb.go` + `lb.GarbageCollect`; ALLOWLIST row moved to *Active* | `TestLbGlobalsOwnerCollectsOnce` (two delete bursts → exactly one `lb vip 0.0.0.0/32 del`), `TestLbSlotAgentNeverCollects`, `TestGarbageCollect*` |
| D-082 VPP-global test: `VRX_LB_GLOBALS=1`, `flock -x /run/lock/vrx-globals.lock`, restore | `TestLbGarbageCollectOnHost` (changes no lb_conf value — asserts `show lb` source lines unchanged) | not run (host runs closed) |
| D-064 crash → opt-in + V-item | NAT SNAT-key hazard found by source reading: never reproduced; `GCSafe` guard; V20 follow-up | `docs/vpp-code-track.md` |
| DF-7 precedent: host test opt-in `VRX_LB_HOST=1` | `TestLbOnHost` | SKIP without the variable (below) |
| TD-11b / TD-11c / TD-8 / TD-23 / D-132 / WEB-1 (addendum) | declarations + claim first; no interface creator; no seams forked; fake handler via one line; 30 s poll + Refresh, one walk at a time; no `dropPhantomOptionals` | tests above; `LB_POLL_MS` asserted ≥ 30 000 in `LbPage.test.tsx` |

## Verification (pasted output)

### Go unit tests (fake VPP)
```
$ go test -count=1 -v -run 'TestLb|TestOwnershipDeclared|TestIntfNatClaimsFirst|TestDumpASes|TestFlushVIP|TestGarbageCollect|TestRegisterGuardsEveryDescriptor|TestVIP|TestASAndNat|TestConf' ./internal/descriptors/lb/ ./internal/desired/ ./internal/subsystems/ ./internal/agent/
--- PASS: TestOwnershipDeclared (0.00s)
--- PASS: TestIntfNatClaimsFirst (0.00s)
--- PASS: TestDumpASes (0.00s)
--- PASS: TestFlushVIPIPv4Layout (0.00s)
--- PASS: TestFlushVIPRefusesWithoutServerInUse (0.00s)
--- PASS: TestGarbageCollect (0.00s)
--- PASS: TestGarbageCollectRefusesSharedSNATMapping (0.00s)
--- PASS: TestConf (0.00s)
--- PASS: TestVIP (0.00s)
--- PASS: TestVIPEnumOrder (0.00s)
--- PASS: TestASAndNat (0.00s)
ok  	ngfw/agent/internal/descriptors/lb	0.036s
--- PASS: TestLbProjection (0.01s)
--- PASS: TestLbProjectionErrors (0.00s)
--- PASS: TestLbUnsupportedServicesMembers (0.01s)
ok  	ngfw/agent/internal/desired	0.053s
--- PASS: TestRegisterGuardsEveryDescriptor (0.00s)
--- PASS: TestLbSlotAgentNeverCollects (0.21s)
--- PASS: TestLbGlobalsOwnerCollectsOnce (0.41s)
--- PASS: TestLbDomain (0.00s)
ok  	ngfw/agent/internal/subsystems	0.675s
--- SKIP: TestLbOnHost (0.00s)
--- SKIP: TestLbGarbageCollectOnHost (0.00s)
--- PASS: TestLbApplyStateFlushRemove (0.07s)
--- PASS: TestLbRestartReappliesWithoutDuplicates (0.04s)
--- PASS: TestLbProjectionRefusesSentinel (0.00s)
ok  	ngfw/agent/internal/agent	1.245s
```

### Tests fail on the base first
The gap tests against DF-7's `lb.go`/`lb_test.go` from `main` (scratch copy of the module; a test-only shim declares
the new helpers as no-ops so the assertions, not the compiler, fail):
```
--- FAIL: TestOwnershipDeclared (0.00s)
    gaps_test.go:41: lb.conf: descriptor declares neither CheckPersistent (it records ownership) nor RecordsNoOwnership
--- FAIL: TestIntfNatClaimsFirst (0.00s)
    gaps_test.go:88: the feature was enabled before the claim (1 calls)
--- FAIL: TestDumpASes (0.00s)
    gaps_test.go:129: base: no DumpASes
--- FAIL: TestFlushVIPIPv4Layout (0.00s)
    gaps_test.go:156: flush request &{Pfx:10.0.30.1/128 Protocol:6 Port:80} (un bytes 61 30 30 3a 31 65 30 31 3a 3a)
--- FAIL: TestFlushVIPRefusesWithoutServerInUse (0.00s)
    gaps_test.go:185: other prefix: <nil>
--- FAIL: TestGarbageCollect (0.00s)
    gaps_test.go:210: base: no GarbageCollect
--- FAIL: TestGarbageCollectRefusesSharedSNATMapping (0.00s)
    gaps_test.go:246: base: no GarbageCollect
```
(`un bytes` is the IPv6 text of the 16 union bytes: `a00:1e01::` = 10.0.30.1 in the first four bytes — the lookup VPP
can never match.) Every other new test file references symbols that do not exist on the base (`desired.Lb`,
`registerLb`, `LbState`, `services.lb`, `/api/v1/state/lb/vips`, the `lb` tab) and fails there to compile or with
404/absent UI; the two edited assertions in `service_test.go` are registry-derived now (D-129 F5).

### Agent-restart simulation (fake VPP, unit level)
```
INFO reconcile start txn_id=lb1 mode=apply domains="[interfaces services]"
INFO reconcile done txn_id=lb1 mode=apply domains="[interfaces services]" status=APPLY_STATUS_APPLIED summary=created:11 ...
INFO reconcile start txn_id="" mode=resync domains="[interfaces services]"
INFO reconcile done txn_id="" mode=resync ... status=APPLY_STATUS_APPLIED summary="created:8  unchanged:3" ...
--- PASS: TestLbRestartReappliesWithoutDuplicates (0.07s)
```
`created:8` = the write-only lb objects re-applied (D-063); the model still holds 3 VIP entries, 4 ASes and one NAT
feature instance (no duplicates: `VALUE_EXIST` with our boot record = success, intf-nat applied once per identity); a VIP
lost while the agent was down is re-created on the next resync.

### Schema
```
$ npx vitest run src/domains/ext/lb.test.ts
 ✓ src/domains/ext/lb.test.ts (16 tests)
 Test Files  1 passed (1)   Tests  16 passed (16)
```
Acceptance "GRE4 VIP with an IPv6 AS → 400 problem+json with pointer": schema test
`/services/lb/vips/web/servers/0/address: encap gre4 needs IPv4 application servers, 2001:db8:2::10 is IPv6`, and the
API e2e below (status 400, `application/problem+json`, the same pointer). Drift guard: `drift guard: 894 scalar leaves and
198 messages compared, 4 accepted difference(s), 0 finding(s)`.

### API e2e (slot 2 PostgreSQL `vrx_w2`, fake agent)
```
$ eval "$(tools/lab env 2)"; npx vitest run -c vitest.e2e.config.ts test/e2e/lb.e2e.test.ts
create role vrx_w2
create database vrx_w2 (owner vrx_w2)
 ✓ test/e2e/lb.e2e.test.ts (2 tests) 3332ms
 Test Files  1 passed (1)   Tests  2 passed (2)
drop   database vrx_w2
drop   role vrx_w2
ok     nothing named vrx_w2 / vrx_w2 remains
```
Covers: 400 pointer, commit, `GET /state/lb/vips` (status, removed server, removed copies), flush 200 / 403 readonly /
404 unknown / 409 agent precondition, the audit row (`POST /api/v1/actions/lb/vips/:name/flush`,
resource `services/lb/vips/web`), 501 without the RPC, removal.

### Web (jsdom)
```
 ✓ src/nav/nav.test.ts (5 tests)
 ✓ src/domains/services/lb/LbPage.test.tsx (4 tests)
 Test Files  2 passed (2)   Tests  9 passed (9)
```

### CI
CI_RESULT_PLACEHOLDER

## Shared hunks (append-only)
Anchored (directly below `wave-BC: F-lb`): `dataplane.proto` (RPCs, `ServicesConfig` field), `subsystems.go`
(`Domains[Services]` entry), `apps/api/src/app.module.ts` (import, controllers, providers), `agent.client.ts` (types,
methods), `fake-agent.ts` (one handler line), `apps/web/src/i18n.ts` (imports, namespace, en, fa),
`apps/web/src/domains/services/tabs.ts` (tab entry).
**Unanchored** (no F-lb anchor; appended at the end of the block, Q2): `packages/schema/src/domains/services.ts` (key line
`lb: servicesLbField` + import), `packages/schema/src/index.ts` (export), `packages/schema/src/semantic/index.ts`
(import + spread), `docs/contracts/proto.md` (§ F-lb), `apps/agent/internal/agent/projection.go` (`desired.Lb` call in
`project()`), `apps/agent/internal/subsystems/subsystems.go` (`Services` constant, `w.registerLb(r)` + import),
`apps/web/src/nav/nav.ts` + `nav.test.ts` (`'services'`), `apps/api/src/testing/fake-agent.ts` (import of
`features/lb/fake.ts`), `apps/web/src/domains/services/tabs.ts` (`import { lazy }`).
No `assemble()` hunk: lb is write-only, nothing to assemble (Retrieve reports no `services.lb`).
Outside the owned list, sanctioned: `apps/agent/internal/agent/service_test.go` (two registry-derived domain assertions,
D-129 F5); `apps/agent/internal/descriptors/lb/lb_test.go` (the flush lines moved to `gaps_test.go`);
`apps/agent/internal/renderers/ALLOWLIST.md` (the row); `docs/vpp-code-track.md` (V20 follow-up, A7); generated files.

## Decisions taken (with options) — for the LOG
1. GC command (Q5): (a) `lb conf <desired values>` (b) **constant `lb vip 0.0.0.0/32 del` + 0.0.0.0/8 reserved** (c) no GC
   → (b); run once 65 s after the last delete (VPP's 10 s AS / 60 s VIP gates), debounced.
2. NAT SNAT-key hazard (Q6): (a) **skip the GC while an (AS, target port) pair repeats** + schema uniqueness (b) forbid
   NAT VIP changes (c) doc only → (a).
3. FlushVIP (Q7): fix the helper under the gap rule (ip46 layout + in-use guard) rather than only guarding in the RPC.
4. `services` domain + services seam copied from F-rpf-adl-pbr (Q3, the D-131 precedent) as a deletable merge seam.
5. Slot agents report `settings` as `agent.unsupported-field` (Q8).
6. Ship behind T3 with the visible notice (Q9, prompt default).

## Out of scope / not done
Health checks, L7, weights; NAT44-ED LB static mappings; CNAT VIPs; VRRP of VIPs; V20 in C. No `Retrieve` for lb
(write-only). No curated CLI command for the live state/flush (REST operations only).

## Pending host steps (after TD-25; slot 2, one host package at a time, `systemctl show vpp -p NRestarts` before/after)
1. `eval "$(tools/lab env 2)"; systemctl show vpp -p NRestarts; VRX_INTEGRATION=1 VRX_LB_HOST=1 go test -count=1 -v -run TestLbOnHost ./internal/agent/; systemctl show vpp -p NRestarts`
   — pastes `show lb vips verbose` after commit, LbState, flush, restart (no duplicate entries, `lb-nat4-in2out` count
   unchanged), loss → re-created, removal (`show lb vips verbose` with the removed entries). Loopback only, no af_packet.
   Leftovers it creates: removed VIPs 10.2.250.1/.2/.3 (and the re-created 10.2.250.1) with their ASes
   10.2.2.10–13, the ASes' recursive `/32`s in table 0 — until a GC or a VPP restart (V20).
2. Manager window only: `VRX_INTEGRATION=1 VRX_LB_HOST=1 VRX_LB_GLOBALS=1 go test -run TestLbGarbageCollectOnHost …`
   (exclusive `/run/lock/vrx-globals.lock`; waits 65 s; proves the constant command collects; lb_conf untouched).
3. UI screenshot against the real endpoint (headless Chrome, production build, real API + agent on slot 2).
4. Optional: GRE evidence (`tcpdump` in ns-w2-wan) after TD-3's preflight on the rig interfaces.

## Cleanup
No process left running (the e2e harness created and dropped `vrx_w2`); no VPP object created (no host run); no
`dist/` or `apps/agent/bin` committed.
