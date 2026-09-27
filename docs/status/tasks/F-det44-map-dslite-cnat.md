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

## Not built (remaining — follow-up row suggested, D-099)

- **Contract**: `contract(schema): nat.pnat` (NatConfig 27), `contract(proto): Det44Sessions, Det44Lookup,
  CnatSessions` + ActionRequest 9 `det44_session_close` / 10 `cnat_session_purge`; the TS semantic rules
  (`nat.det44-map-dslite-cnat-…`) mirroring the agent's two new checks.
- **PNAT** projection (`desired/pnat.go`) and wiring — needs the contract; `pnat` stays `pending` in reachability.
- **State/actions**: `rpc_det44*.go` / `rpc_cnat*.go` (DET44 paged per-user sessions + close, det44 forward/reverse
  lookup, CNAT paged sessions + purge), `actions/det44-map-dslite-cnat/**`, server.go Action cases.
- **API** `features/det44-map-dslite-cnat/**` + e2e; **UI** tabs CGNAT/MAP/CNAT/PNAT + port-block calculator, en/fa
  locales; screenshots.
- **Host evidence** (every acceptance packet line, `show map domain`, `show cnat translation`, `show dslite …`,
  NRestarts before/after): needs a VPP host / manager window (`VRX_FDET44_DET44_HOST=1`, `VRX_FDET44_GLOBALS=1`).
  `test/topology/det44-map-dslite-cnat/` not written.

## Acceptance

- [ ] DET44 packet path (af_packet rig) — not run: no VPP host. Fake: mapping/interfaces programmed and retrieved.
- [ ] MAP-T/E BR packet path + `show map domain` — not run (no host). Fake: LW4o6 domain + rule + interface round trip.
- [ ] CNAT VIP → two backends + `show cnat translation`; DS-Lite shows — not run (no host). Fake: translation with two paths round trip.
- [x] Agent-restart simulation → all objects back (fake: one resync, `created:8 unchanged:12`); write-only objects
      re-applied without duplicates (fake). Host part not run.
- [x] Rollback removes the owner's objects (Retrieve empty, models empty); det44 stays enabled (V9, 0 disable calls).
      NRestarts: n/a (no host).
- [~] cnat SNAT policy without an SNAT address → refused with pointer `/nat/cnat/snat/addresses` by the agent
      (DryRun/commit); the API 400 path is not tested here. `tools/ci.sh` — see below. UI screenshot — not built.

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
