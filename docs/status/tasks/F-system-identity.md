# F-system-identity: hostname, time zone, login/MOTD banners, DNS client + System screen

Branch `claude/modest-keller-upaw4m` (cloud session modest-keller). Decision: DEC-system-identity / D-152 (option 1,
WBS D0.14). Contract: **no change**. `system.ts` and `SystemConfig` in the proto already had every field.

## What was built
| layer | what |
|---|---|
| agent renderer | `internal/renderers/sysident`: `/etc/hostname` (+ `sethostname(2)`, globals owner only), `/etc/localtime` → zoneinfo (zone file must exist), `/etc/issue` + `/etc/issue.net` (banner.login), `/etc/motd`, systemd-resolved drop-in `/etc/systemd/resolved.conf.d/vrx.conf` (`DNS=`, `Domains=`, embedded render input). All paths in `paths.go` (`ProductPaths`, `PathsUnder`) |
| agent descriptor | singleton `system.identity/vrx`: `scheduler.Validator` (zone, D-049 banner controls incl. C1 and bidi, host name, IPs, search domains, VRF = default; findings name the leaf), `Stage() = StageDaemon`, writes only changed files (an unchanged document writes nothing), Retrieve = embedded input re-rendered and compared (drift → Update), Delete removes the drop-in only, `RecordsNoOwnership` (TD-11b) |
| projection | `desired/system_identity.go`: `system` → the singleton (defaults filled); structural DryRun errors at `/system/...` with rule `system.identity`; assembler for Retrieve |
| wiring | `subsystems/system_identity.go` (product paths only for the globals owner; otherwise `<slot dir>/sysident/etc/…`), `System` domain in `Domains`, register + projection/assemble lines (unanchored: no anchor was seeded for this row), `reachability_test.go`: `sysident` wired (maxPending unchanged, 23) |
| web | `domains/system/identity/SystemIdentityPage.tsx`: SchemaForm of `domainSchemas.system` (i18n `system-identity:field`), live column candidate vs running with an uncommitted chip, server pointers mapped onto fields; route `/system`; `'system'` added to `BUILT_DOMAINS` (the System nav entry no longer shows "soon"); en + fa `system-identity.json` |
| docs | `docs/agent/renderers/sysident.md`, `docs/user/system/identity.md` (example + CLI equivalent) |
| evidence harness | `test/topology/system-identity` (own module, real agent + API on a slot, goldens, restart-no-rewrite, host identity unchanged). **Not run** (see below) |

## Tests run here (cloud sandbox, no VPP, no lab slot)
- `go test ./...` in `apps/agent`: all packages ok (contracttest needed `regen.sh` first: stale schema dist, pre-existing).
  New: `renderers/sysident` (goldens under a temp root, unchanged apply writes nothing and makes no sethostname call,
  lifecycle/drift/delete, validator table: unknown zone, `..` traversal, ESC, CR, C1, bidi, bad host, bad server,
  VRF), `desired` `TestSystemIdentityProjection`.
- Changed existing tests (unavoidable, because `system` is now implemented): `service_test.go` canonical Retrieve gains `system`,
  +1 created/unchanged object, the unimplemented-domain examples use `dataplane`. `td9_test.go` does the same. `newSvc` points
  `VRX_HOST_SERVICES_DIR` at a per-state-dir temp directory. `sysident_main_test.go` in `agent` and `subsystems` does the
  same for the whole package, so no test writes under `/run/vrx-test`.
- `golangci-lint` on the touched agent packages: 0 issues. `go vet` + unit-mode `go test` of `test/topology/system-identity`: ok (skips).
- Web: `tsc --noEmit` clean, eslint clean, logical-CSS check ok, full `vitest run` (see the final report) incl. the new
  page test (nav availability, candidate vs running, PATCH body, pointer → field) and the locale parity test; `nav.test.ts` gains `'system'`.

## Not tested / not built (open)
- **Topology/lab run not executed**: this sandbox has no slot, VPP, PostgreSQL or lab lock. The acceptance evidence
  (goldens on a slot rig, 400 with pointer through the real API, restart log excerpt, screenshot) is still owed:
  `eval "$(tools/lab env <N>)"; test/topology/system-identity/run.sh`. `tools/ci.sh --base main` could not run here
  (turbo cannot spawn). The per-package checks above were run instead.
- **`GET /api/v1/state/system`** (current hostname, time zone, uptime, resolver status) is **not built**. It needs a new
  agent RPC (a `contract(proto)` change) plus an API route and OpenAPI regen. The screen's live column shows the running
  configuration instead. Follow-up.
- **The web login page showing `banner.login`** (unauthenticated, length-capped read) is **not built**. It needs a public API
  route (a contract change). Follow-up.
- systemd-resolved is not restarted after a DNS change. The agent logs the restart request (D-079,
  PENDING-agent-privileges, same hand-off as unbound/chrony M4).

## Open questions (surfaced, not decided)
- Q1 systemd-resolved vs plain `/etc/resolv.conf` on the 26.04 image: I chose the resolved drop-in, as the prompt says. I
  did not check P10's package list here.
- Q2 whether a hostname change should also drive SNMP `sysName` / the syslog hostname: kept **independent** (no coupling).
- Q3 `/etc/issue` is written verbatim, and agetty expands backslash escapes (`\n`, `\l`) in it. Should the renderer escape them?
- Q4 `system.dns.vrf` other than `default` is refused with a pointer (resolved has no VRF support). Should it be a warning instead?
