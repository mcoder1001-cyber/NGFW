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

---

# Part B — wiring (branch `task/F-nat46-b`, based on `task/F-det44-b` @ 74864bae)

Cloud container: **no VPP host, no PostgreSQL** — the fake VPP client only; `TestNat46OnHost` skips without
`VRX_INTEGRATION`; the API e2e (`apps/api/test/e2e/nat46.e2e.test.ts`) is written and typechecked but was **not run**
(needs the host database and `VRX_TEST_PREFIX`).

## What
1. **Contract** (`contract(schema)`, `F-nat46-contract.md`): `nat.nat46` (`domains/ext/nat46.ts`, refinements mirroring
   `nat46.Validate`: /96, host bits zero, server outside the client prefix, unique names/addresses, interfaces with
   mappings), semantic rules `nat.nat46-interfaces`, `nat.nat46-map-overlap` (service /32 inside a `nat.map` prefix;
   `nat46-<mapping>` MAP domain name — the name guard now holds both ways), `nat.nat46-map-interface` (map-e interface);
   proto `NatConfig.nat46 = 28` (25–26 stay F-nat44-ed-sessions', 27 pnat). Go/TS/YANG/api-client regenerated.
2. **Agent**: `desired/nat46.go` builder (dispatched from `desired.Nat`) + `assembleNat46` (from `AssembleNat`, after
   `assembleMap`). Shared MAP-T interface: nat.map's builder emits the one key; on Retrieve the owners come from the
   stored desired state (`desired.SetNat46Owners` in the service snapshot refresh), so a shared interface reads back
   under both and no false drift appears; a foreign map-t binding stays nat.map drift. projection.go needed no hunk
   (the `nat` dispatch already calls `desired.Nat` / `AssembleNat`); service.go got one line.
3. **API**: config through the generic `/config/nat` routes; guarded read-only `GET /state/nat/nat46` and
   `GET /state/nat/nat46/client?ipv4=`; unit test + e2e. CLI operation table regenerated.
4. **UI**: NAT46 tab after NPTv6 (schema form + client-address helper), en/fa, jsdom test; locale parity covers it.
5. **Docs**: `docs/user/firewall/nat46.md`; D-160 (stateful NAT46 out of scope) + `docs/vpp-code-track.md` note.

## Verification (cloud)
```
apps/agent: gofmt clean (touched files); go vet ./... clean; go test -race ./... all ok
  --- PASS: TestNat46Projection / TestNat46SharedInterface / TestNat46Refusals / TestNat46RoundTrip (desired)
  --- PASS: TestNat46DomainOnFake (agent: commit → Retrieve == canonical → empty re-apply → domain loss + restart +
            resync → rollback: 0 domains, 0 map-t, 0 map-e)
golangci-lint (desired, agent, subsystems): only the 2 pre-existing gosec G115 in snmp_integration_test.go
apps/cli: go test ./... ok
turbo lint typecheck test (schema, proto, api, web, api-client): 27/28 — the one failure is the pre-existing
  @ngfw/proto "has exactly the 13 root keys in ROOT_KEYS order" (fails on main too, not touched)
  ✓ src/semantic/nat46.test.ts (9)  ✓ src/features/nat46/nat46.test.ts (3)  ✓ Nat46Tab.test.tsx (3)  ✓ locales (53)
tools/ci.sh check: PASSED
```

## Remains (host)
- `VRX_INTEGRATION=1 go test -run TestNat46OnHost ./internal/descriptors/nat46` (does 26.06 accept /32↔/128 with
  ea_bits_len 0), `vppctl show map domain` after a commit, af_packet packet test IPv4 client → IPv6-only server,
  restart (≤ 30 s) and rollback evidence.
- API e2e on the host PostgreSQL (`VRX_TEST_PREFIX` set); screenshot of the NAT46 tab against the real endpoint.
