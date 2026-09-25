# TD-11a: agent reachability check (desired state → data plane)

Branch `task/TD-11a`, base be53867. Review REVIEW-2026-09-24 items 3(a), 3(b), 3.1 (doc), 6.1a / 6.6b; D-125.

## What

1. **Registry-to-board-row table** — `apps/agent/internal/subsystems/reachability_test.go` (`TestReachabilityTable`).
   Every package under `internal/descriptors/*` and `internal/renderers/*` has an entry: `wired` (row), `pending`
   (the board row that wires it) or `library` (reason). The test fails when
   - a package has no entry, or an entry names a package that does not exist;
   - a `wired` descriptor is not used inside `subsystems.register()` (renderers: `<pkg>.New` is not called by
     non-test code outside `internal/renderers`);
   - a `pending` package **is** now wired (flip to wired) — the allowlist is shrink-only: `maxPending` (57) must
     equal the pending count, so wiring lowers it and growing it is a visible edit;
   - a pending row id is not a row of `plan/tasks.yaml`.
   Today: wired = core, interface, af_packet, dhcp; library = df2, df6, df7, dfkit, natcommon, vpn, memif, tapv2,
   rfkit, vppstartup; 57 pending. Options-only uses in `Wiring` methods (classify, ipsec, ikev2, vpn) do not count
   as wired; `frr` is imported by `agent/projection.go` for a predicate only, so it stays pending (P12).
2. **memif/tapv2** → library-only, **D-141** in `docs/decisions/LOG.md` (tapv2 remains the integration-test rig
   creator).
3. **Doc fix** (review 3.1): `core/core.go` package comment and `core/README.md` "Handoff to P08" now say the alias
   is registered with `AliasInterfaceRef` by P08; the open remainder is 3.1b (untagged NIC claims, TD-11c).
4. **API-level reachability helper** (6.1a/6.6b) — `test/integration/reachability/` (own stdlib-only Go module).
   `Client.Check(ctx, pointer, value)`: PUT the candidate node → `POST /api/v1/config/commit` must answer
   `status: applied`, `notApplied: []`, no `agent.unsupported-field`/`agent.unimplemented-domain` warning related
   to the pointer → `GET /api/v1/state/drift` must list the pointer's domain in `subsystems`, not ignore the
   pointer, and report no changes (Retrieve() == desired). `VerifyCommit`/`VerifyDrift` are exported for tests
   that commit by other means. Unit tests use an httptest fake API; `TestReachabilityLoopback` is the live check
   (needs `VRX_INTEGRATION=1`, `VRX_API_URL`, `VRX_API_TOKEN`; optional `VRX_REACH_LOOPBACK`) and **skips here** —
   no API/agent/VPP in this container, so the live path is not proven.
5. `tools/ci.sh`: no change needed — `do_test_modules` already vets and unit-tests every Go module under `test/`.

## Verification

```
$ go test -count=1 -run TestReachabilityTable ./internal/subsystems/
ok  	ngfw/agent/internal/subsystems	0.280s
# mutation: core marked pending →
reachability_test.go:158: descriptors/core: now used in subsystems register() — flip it to wired and lower maxPending (shrink-only allowlist)
reachability_test.go:167: pending allowlist has 58 entries, maxPending is 57: ...
$ go -C test/integration/reachability test -count=1 -v ./...
--- PASS: TestCheck (0.01s)          # 7 cases: reachable, partially-applied, notApplied, unsupported-field, drift, domain not retrieved, leaf ignored
--- PASS: TestCheckRejectsRoot
--- PASS: TestHTTPErrorSurfaces
--- SKIP: TestReachabilityLoopback   # VRX_INTEGRATION != 1
ok  	ngfw/test/integration/reachability
$ go vet ./... (apps/agent) — clean; gofmt — clean
$ go test ./... (apps/agent) — all ok except internal/renderers/strongswan TestWatchResync: flaky, pre-existing
  (passes/fails alternately with and without this change; the package imports nothing touched here)
$ tools/ci.sh check → check PASSED
$ tools/ci.sh quick → fails at 'pnpm gen' (@ngfw/ui-kit#gen: Exec format error, container toolchain; no TS touched)
$ make -C apps/agent lint → golangci-lint here is built with go1.25 < go 1.26 target (container tool mismatch)
```

## Out of scope

The live API check on a lab slot; UI-level reachability; wiring any pending package (their rows); ci.sh edits.

## Open questions

- `pnat` is assigned to F-nat44-ei-64-66-nptv6 and `frr` to P12 by best fit — the manager may re-home them.
- D-141 is the next free id on be53867; renumber at merge if another branch took it.
- Wave-A rows that wire a package must flip their entry and lower `maxPending` (prompt/envelope obligation).
