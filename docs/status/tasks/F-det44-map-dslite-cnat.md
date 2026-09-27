# F-det44-map-dslite-cnat — DET44, MAP-E/T/LW4o6, DS-Lite, CNAT (agent layer)

Branch `task/F-det44-map-dslite-cnat`, base `origin/main@5966f618`. Cloud container: **no VPP host** — all evidence is
from the fake VPP (coretest models); real-VPP integration tests (`*_integration_test.go`) skip without
`VRX_INTEGRATION` and none were run. Questions: `F-det44-map-dslite-cnat-questions.md`.

## What (built)

| layer | built |
|---|---|
| descriptor | new `descriptors/dslite`: `dslite.aftr` / `dslite.b4` (D-071 globals via `natcommon.Global`: owner sets/resets to `::`, slot requires), `dslite.pool` (claimed ranges, dump merged back into ranges, slot-range scoped); unit tests on a fake modelling VALUE_EXIST / NO_SUCH_ENTRY; `docs/agent/descriptors/dslite.md` |
| projection | `desired/det44.go` (enable+VRFs, non-default timeouts, interfaces, maps; prefix/ratio/overlap checks; `Det44PortsPerHost`), `desired/map.go` (domains incl. LW4o6 rules, params ≠ VPP defaults, interfaces map-e/map-t; tcpMss / preResolve → `agent.unsupported-field`), `desired/dslite.go`, `desired/cnat.go` (translations → paths, SNAT addresses/policy/interfaces/exclude prefixes, cnat feature per policy interface; policy without SNAT address → error with pointer; CNAT on a NAT44-ED interface → error) + assemblers back into `NatConfig` |
| wiring | `subsystems/det44_map_dslite_cnat.go` (persisted `KeyedClaims("nat")`, `WithGlobalsOwner`), `Domains["nat"]` group; reachability `det44`, `mapnat`, `cnat` pending→wired, `dslite` added wired, `maxPending` 19→16 |
| fakes | `coretest/det44.go` (det44 + dslite; a det44 disable is counted as a V9 violation), `coretest/mapnat.go` (map + cnat; V10 crash calls recorded; `feature_is_enabled` answers for ip4-map, ip4-map-t, cnat-input-ip4) |
| tests | builder unit tests + round trip (`desired/det44_map_dslite_cnat_test.go`), service on the fake (`agent/rpc_det44_test.go`): commit → Retrieve == canonical → empty re-apply → loss + **agent restart** (fresh Service, same state dir) + resync → all back, MAP domain not duplicated → rollback: models empty, det44 still enabled, 0 disables, AFTR and SNAT entry untouched, 0 V10 crash calls; slot agent refuses an AFTR it does not own and never writes it |
| docs | `docs/user/firewall/det44-map-dslite-cnat.md`, `docs/vpp-code-track.md` V-new (F-det44-map-dslite-cnat) (a) |

## Review round 1 (merge of origin/main incl. F-nat46)

- `nat.map` refuses empty domain names and names starting with `nat46.DomainPrefix` (pointer `/nat/map/domains/<i>/name`);
  `assembleMap` skips `nat46-` domains and their rules (they belong to the nat46 assembler).
- An lw4o6 domain without rules is refused (pointer `/nat/map/domains/<i>/rules`): it would round-trip as map-e.
- cnat's `snat-policy`, `snat-interface`, `snat-exclude-prefix` (write-only, D-063) and the derived
  `interface-feature` are never read back into `nat.cnat`. This is documented in `docs/agent/descriptors/dslite.md`.

## Part B (branch `task/F-det44-b`, base `origin/main@a302de71`) — the follow-ups

Commits (not pushed): `bf4ab04a` contract · `f3b7fc2a` PNAT + wiring + Q4 · `9654d282` state/actions (agent) ·
`eaf87ad5` API + fake agent + e2e · `ad5474fb` UI · (this file / user doc in the last commit).

| layer | built |
|---|---|
| contract | `nat.pnat` (NatConfig **27**, verified free: max was 24) + `PnatConfig/Binding/Match/Rewrite/Attachment`; RPCs `Det44Sessions`, `Det44Lookup`, `CnatSessions`; `ActionRequest` **9** `det44_session_close`, **10** `cnat_session_purge` (both were free). Schema `ext/det44-map-dslite-cnat.ts` (optional, refinements); TS semantic rules `nat.det44-map-dslite-cnat-{cnat-snat-address,cnat-nat44-interface,map-domain-name,lw4o6-rules,pnat-interfaces}`; Q3: MAP `securityCheck.enabled` / `trafficClass.copy` default `true`. Regenerated Go/TS/YANG/api-client (timestamp.ts restored). See `-contract.md`. |
| PNAT | `desired/pnat.go` (projection + assembler; names/order from the applied document via `RelabelPnat`, unmatched → `pnat-<n>`), registered in `subsystems/det44_map_dslite_cnat.go`; reachability `pnat` → wired, `maxPending` 16 → 15; coretest pnat model (V11 crash calls recorded); service test `agent/rpc_det44_pnat_test.go` (commit → Retrieve == canonical → empty re-apply → loss + restart + resync → rollback empty, 0 crash calls). `service_test.go` read-only allowlists gain `pnat_bindings_get` / `pnat_interfaces_get`. |
| Q4 | info notice `nat.det44-map-dslite-cnat-cnat-interface-feature` naming the derived interfaces (ISSUE_SEVERITY_INFO through the optional `desired.InfoSink`; `projected.Infof` added in `agent/projection.go`). |
| state/actions | `agent/rpc_det44.go` (sessions per user, paged, port block via det44_forward; lookup forward/reverse; close in/out, exit 1 = none; slot scope → PERMISSION_DENIED), `agent/rpc_cnat.go` (paged, cap → truncated; purge globals-owner only), server.go cases under the anchor; coretest det44 gains VPP's forward/reverse formulas + sessions. Tests over the gRPC server (`rpc_det44_state_test.go`). |
| API | `features/det44-map-dslite-cnat/**`: `GET state/nat/det44/sessions`, `POST actions/nat/det44/lookup`, `POST actions/nat/det44/sessions/close` (audited), `GET state/nat/cnat/sessions`, `POST actions/nat/cnat/sessions/purge` (`@MinRole('admin')`, in ADMIN_ONLY); NOT_FOUND → 404, PERMISSION_DENIED → 403. Fake agent handlers (`fake.ts`), unit test over gRPC + commit engine (CNAT policy without addresses → 400 `/nat/cnat/snat/addresses`, agent not asked). **e2e** `test/e2e/det44-map-dslite-cnat.e2e.test.ts` written, **not run: no PostgreSQL in this container** (typechecked only). CLI operation table regenerated. |
| UI | NAT page tabs CGNAT / MAP / CNAT / PNAT (natTabs anchor), port-block calculator, per-user sessions, lookup, CNAT sessions + admin purge; en + fa (`locales.test.ts` parity green). Form field titles come from the schema (English); screenshots not taken (no browser here). |

Shared hunks (part B): `packages/schema/src/{domains/nat.ts (key line + import + Q3 defaults),index.ts,semantic/index.ts}`,
`dataplane.proto` anchors, `apps/api/src/{app.module.ts,agent/agent.client.ts,testing/fake-agent.ts,auth/route-guard.test.ts}`,
`apps/web/src/{i18n.ts,domains/firewall/nat44-ed-sessions/tabs.ts}`, `NatV6Tabs.test.tsx` (its exact tab list → its own
slice, two lines), `desired/nat.go` (two lines), `agent/{server.go,projection.go,service_test.go}`, reachability test.

### Gates part B (2026-09-27, cloud container, pasted)

```
apps/agent: gofmt -l (touched pkgs) → (empty); go vet ./... → vet-ok
go test -race ./... → 122 ok, 0 FAIL
  --- PASS: TestPnatDomainOnFake / TestDet44StateOnFake / TestCnatStateOnFake / TestCgnatDomainOnFake / TestCgnatSlotRequiresGlobals
  --- PASS: TestPnatProjection / TestPnatRoundTrip / TestCnatFeatureInfo / TestReachabilityTable
golangci-lint run ./... → 2 issues, both gosec G115 in internal/subsystems/snmp_integration_test.go (pre-existing, not this task)
apps/cli: go test ./... → all ok
turbo lint typecheck test (schema, proto, api, web, api-client), --continue:
  @ngfw/schema  Test Files 72 passed, Tests 1565 passed
  @ngfw/api     Test Files 56 passed, Tests 340 passed
  @ngfw/web     Test Files 89 passed, Tests 521 passed (lint clean after the fix commit)
  @ngfw/proto   1 failed | 103 passed: "DesiredState mirrors RootConfig > has exactly the 13 root keys" —
                PRE-EXISTING on origin/main (reproduced with this branch's changes stashed), not this task's
tools/ci.sh check → check PASSED (gitleaks: no leaks found)
```

## Not built (remaining — for the host / later)

- **Host evidence** (every acceptance packet line, `show map domain`, `show cnat translation`, `show dslite …`,
  NRestarts before/after): needs a VPP host / manager window (`VRX_FDET44_DET44_HOST=1`, `VRX_FDET44_GLOBALS=1`).
  `test/topology/det44-map-dslite-cnat/` not written. Real-VPP integration tests skip without `VRX_INTEGRATION`.
- **e2e on PostgreSQL**: `pnpm --filter @ngfw/api test:e2e test/e2e/det44-map-dslite-cnat.e2e.test.ts` on the host.
- **UI screenshot** (acceptance line) and localized field titles of the schema forms (fa shows the schema's English titles).
- `actions/det44-map-dslite-cnat/**` directory not used: the actions live in `agent/rpc_{det44,cnat}.go` like F-nat44-ed-sessions.

## Acceptance

- [ ] DET44 packet path (af_packet rig) — not run: no VPP host. Fake: mapping/interfaces programmed and retrieved.
- [ ] MAP-T/E BR packet path + `show map domain` — not run (no host). Fake: LW4o6 domain + rule + interface round trip.
- [ ] CNAT VIP → two backends + `show cnat translation`; DS-Lite shows — not run (no host). Fake: translation with two paths round trip.
- [x] Agent-restart simulation → all objects back (fake: one resync, `created:8 unchanged:12`); write-only objects
      re-applied without duplicates (fake). Host part not run.
- [x] Rollback removes the owner's objects (Retrieve empty, models empty); det44 stays enabled (V9, 0 disable calls).
      NRestarts: n/a (no host).
- [~] cnat SNAT policy without an SNAT address → 400 with pointer `/nat/cnat/snat/addresses` from the TS semantic rule
      (API unit test over the commit engine; e2e written, not run — no PostgreSQL) and refused by the agent too. `tools/ci.sh` — see below. UI screenshot — not built.

## Shared hunks

- `apps/agent/internal/desired/nat.go`: the four `natUnsupported` lines under `// wave-BC: F-det44-map-dslite-cnat`
  replaced by `det44Build/dsliteBuild/mapBuild/cnatBuild`; four `assemble*` calls under the same anchor in `AssembleNat`.
- `apps/agent/internal/desired/nat_test.go` (ED's test, dep-chained consequence of the dispatch hunk): one expectation
  line — `det44 {enabled:true}` now projects `det44.enable/global` and no longer warns.
- `apps/agent/internal/subsystems/subsystems.go`: one group line in `Domains["nat"]`, one Register block, both under the anchor.
- `apps/agent/internal/subsystems/reachability_test.go`: cnat/det44/mapnat → wired, dslite added, maxPending 16.
- `docs/vpp-code-track.md`: `### V-new (F-det44-map-dslite-cnat)` appended.

## How verified (cloud container, fake VPP)

See "Gates" (pasted in the final commit of this file).

### Gates (2026-09-27, cloud container)

- `gofmt -l` on touched packages: clean. `go vet ./...`: clean. `go test -race ./...` (apps/agent): all packages ok
  (after adding cnat's write-only names to the shared `withoutWriteOnly` helper — `TestProjectSchemaExamples`
  round-trips `nat-cgnat.json` now that cnat is projected).
- `golangci-lint run ./...`: 2 issues, both pre-existing in `internal/subsystems/snmp_integration_test.go` (gosec G115,
  not this task's file); 0 in this task's files.
- TS turbo gates: no TS package touched.
- `tools/ci.sh check`: stops at gitleaks on 5 `generic-api-key` hits in commits already on `origin/main`
  (4e595289, 5b33b153, 5a2d88d8, 3bd18dd2 — e2e/test files of other tasks); none in this branch's 5 commits.
  The later stages did not run.
- Real-VPP integration tests: skipped (no `VRX_INTEGRATION`, no VPP host).
