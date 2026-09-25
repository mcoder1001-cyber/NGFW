# FU-lint-hardening

## What
1. golangci-lint findings fixed: kit_test.go:43 G304 (`#nosec G304` — path is under t.TempDir()); service.go `revertRetryMin` → `revertRetryFloor` (revive time-naming; only use at l.136); coretest/lisp.go:20 QF1008 (`f.VPP.On` → `f.On`).
2. F-host-stack follow-up: IPv4-mapped unspecified (`::ffff:0.0.0.0`) refused for http_static uri.
   - Go: hoststack ValidURI uses `a.Unmap().IsUnspecified()`; tests `tcp://::ffff:0.0.0.0/80`, `tcp://::ffff:0:0/80`.
   - Schema: host-stack.ts refine also rejects canonical `::ffff:0:0`; tests `tcp://::ffff:0.0.0.0/80`, `tcp://::FFFF:0:0/80`.
3. TD-18 (tech-debt "WEB-3 review"): `@ngfw/web lint` now runs `eslint src test/e2e` (flow.e2e.mjs lints clean under the root config).
   The forbidden-pattern secret grep (tools/ci.sh do_forbidden §4) and gitleaks already scan the whole repo (`.`), so test/e2e was already covered there — no ci.sh change needed. (The control-plane shell/VPP grep §1 intentionally stays on src paths.)

## Verification
- gofmt -l: empty; go vet ./...: ok
- golangci-lint run ./...: `0 issues.`
- go test -race ./...: all ok except one TestWatchResync (strongswan, 45 s timeout, `resync true poll false`) under full-suite load; re-run with -count=1 passed (5.5 s) — timing flake, unrelated.
- pnpm --filter @ngfw/schema test: `Tests 1250 passed (1250)`
- turbo test --filter=@ngfw/web: `Tests 113 passed (113)`; web lint OK
- tools/ci.sh check: `check PASSED`

## Out of scope / open
- strongswan TestWatchResync flakes under -race full-suite load (pre-existing).
- tech-debt.md entry not edited (manager bookkeeping).
