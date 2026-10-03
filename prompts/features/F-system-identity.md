# Task: F-system-identity — hostname, timezone, banners, DNS client + System screen   (prepend 00-CONTEXT.md)

## Goal
Apply the `system` schema domain on the box and give it a screen (WBS D0.14, product owner decision DEC-system-identity / D-152).
Today a user can commit `system.*` and nothing on the router changes; the System nav entry shows "soon".
Reference: TNSR "System settings" (hostname, time zone, banners, name servers).

## Inputs to read first
- `packages/schema/src/domains/system.ts` — `hostname`, `timezone`, `banner.login`/`banner.motd`, `dns.servers[]`, `dns.searchDomains[]`, `dns.vrf`.
  Extend additively only (contract rule below).
- `apps/agent/internal/renderers/chrony/` — the pattern for a host-file renderer (staged render, checker, `scheduler.Validator`,
  `StageDaemon`, pending/state files). Copy its shape, do not share its package.
- `docs/agent/scheduler-validators.md` (TD-13), `docs/lab/shared-host-rules.md` (never change the real host's hostname/timezone in tests),
  `docs/decisions/LOG.md` D-040, D-049 (control characters), D-125 (DoD: reachable from desired state through the API and the UI).
- `apps/web/src/domains/services/ServicesPage.tsx` + `apps/web/src/config/**` (WEB-2 kit) for the screen.

## Contract changes
None expected. If a field is missing, commit it first on your task branch with subject `contract(schema): …` (additive only).

## Scope — build exactly this
1. **Agent renderer `sysident`**: renders `/etc/hostname` + `sethostname(2)`, `/etc/localtime` symlink to `/usr/share/zoneinfo/<tz>`
   (validate the zone file exists), `/etc/issue` + `/etc/issue.net` (login banner) and `/etc/motd`, and a systemd-resolved drop-in
   `/etc/systemd/resolved.conf.d/ngfw.conf` (DNS=, Domains=). All paths come from one `paths.go` so tests run under a temp root.
   Implements `scheduler.Validator` + `StageDaemon`; banners are rejected if they carry C0/C1/bidi control characters (D-049).
   `Retrieve` reads the files back. Idempotent: an unchanged document writes nothing.
2. **API**: config through the generic pointer routes; `GET /api/v1/state/system` (current hostname, timezone, uptime, resolver status).
   The web login page shows `banner.login` (read without auth, length-capped).
3. **UI** `apps/web/src/domains/system/identity/`: SchemaForm page for the domain, live state column, `system` added to `BUILT_DOMAINS`
   in `apps/web/src/nav/nav.ts` under a `// F-system-identity` anchor; en + fa strings.
4. **Tests**: renderer unit tests under a temp root (golden files), validator test (bad timezone, control characters), topology test
   `test/topology/system-identity/` that never touches the real host identity.
5. **Docs**: `docs/user/system/identity.md` with an example and the CLI equivalent.

## Acceptance (paste the evidence)
- [ ] Commit hostname/timezone/banners/DNS on a slot rig → files rendered under the rig root match the goldens (pasted)
- [ ] Unknown timezone or a banner with an escape sequence → 400 problem+json with a `pointer`
- [ ] Agent restart → no rewrite when nothing changed (log excerpt)
- [ ] Screenshot of the System screen against the real endpoint; the nav entry no longer shows "soon"
- [ ] `tools/ci.sh --base main` green

## Out of scope (do not build)
NTP (F-unbound-chrony-syslog), the Unbound DNS server (F-unbound-chrony-syslog), OS hardening of login (F-hardening-lite), AAA (F-aaa).

## Open questions to surface, not to decide silently
- systemd-resolved vs a plain `/etc/resolv.conf` on the 26.04 image (check P10's package list first).
- Should a hostname change also update the SNMP sysName / syslog hostname automatically, or stay independent?
