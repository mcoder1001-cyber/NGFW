# F-nat46 — NAT46 (IPv4 clients → IPv6-only servers), stateless

Branch `task/F-nat46`, base `origin/main@5966f618`. Cloud container session: **no VPP host** — fake VPP client only;
the real-VPP test `TestNat46OnHost` **skips without `VRX_INTEGRATION`** and was not run.

## What
- `prompts/features/F-nat46.md` generated from `FEATURE-TEMPLATE.md` (board row, WBS D4.5, §3 D4, host-vrx-a plugin list).
- Design spike (`docs/agent/descriptors/nat46.md`): (a) stateless 1:1 SIIT is a MAP-T domain per mapping
  (`A/32` ↔ `S/128`, `ea_bits_len 0`, `ip6_src` = RFC 6052 /96) + MAP-T on the interfaces; (b) stateful NAT46 has no VPP
  implementation → not built, question Q1.
- `apps/agent/internal/descriptors/nat46/` — projection library (no descriptors): `Validate` (field paths), `Project` →
  `mapnat.DomainSpec` / `mapnat.InterfaceSpec`, `Assemble` back, `ClientAddress`. Uses only binapi `map` via `mapnat`.
- `subsystems/reachability_test.go`: `nat46` added as `library` + pin (no descriptors; nothing to flip to wired).

## Verification (cloud, pasted)
```
--- SKIP: TestNat46OnHost (0.00s)      # VRX_INTEGRATION unset
--- PASS: TestProject / TestValidate / TestAssembleIgnoresForeignShapes / TestClientAddress
--- PASS: TestApplyThroughMapnat       # projection through mapnat descriptors on the fake: create, idempotent re-apply, Retrieve+Assemble == desired, rollback empty
ok  	ngfw/agent/internal/descriptors/nat46
```
apps/agent: `go vet ./...` clean; `go test -race ./...` all ok; gofmt clean on touched files (3 pre-existing unformatted files on
main: multiwan/health_test.go, renderers/pppoe/supervisor.go, vpp/ifsanitize/sanitize.go — not touched);
golangci-lint `./internal/descriptors/nat46/...` 0 issues (subsystems: 2 pre-existing gosec in snmp_integration_test.go).
`tools/ci.sh check --base origin/main` PASSED; plain `tools/ci.sh check` fails on gitleaks findings in main's history
(apps/api e2e/totp tests, commits 4e595289 5b33b153 5a2d88d8 3bd18dd2 — none of this branch). No TS touched → turbo not run.

## Not done / remains
- Wiring (schema `nat.nat46`, `desired/` builder, `Domains["nat"]`, API/UI/user docs): outside `files_owned`
  (`descriptors/nat46/**` + `docs/agent/descriptors/nat46.md`) — Q2.
- Host: `VRX_INTEGRATION=1 go test -run TestNat46OnHost ./internal/descriptors/nat46` (does 26.06 accept /32↔/128 with
  ea_bits_len 0), then an af_packet packet test IPv4 client → IPv6-only server, restart + rollback evidence.

## Out of scope
Stateful NAT46, mapnat edits, nat.map/DS-Lite/464XLAT, DNS46, binapi, startup.conf.
