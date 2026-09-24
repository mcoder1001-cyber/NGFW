# Task: F-unbound-chrony-syslog — Unbound DNS + VPP DNS cache, chrony NTP, syslog export + log explorer   (prepend 00-CONTEXT.md)

## Goal
Three host services end to end (WBS D7.3, D7.4, D7.7 in `plan/wbs.csv`): Unbound resolver/forwarder (DNSSEC, local zones, views) with
VPP's caching DNS plugin as an option; chrony NTP client/server; remote syslog export via rsyslog plus a log explorer in the UI.
Reference: TNSR "DNS resolver (Unbound)", "NTP", "Logging"; VPP plugin `dns`.

## Inputs to read first
- `packages/schema/src/domains/services.ts` — `services.dns{resolvers{<name>}, vppCache}` and `services.ntp` (NTP lives **only** here,
  D-050); `packages/schema/src/domains/management.ts` — remote syslog is `management.syslog[]` (address, port, protocol udp|tcp|tls,
  severity, vrf). Extend only additively.
- `apps/agent/internal/renderers/{unbound,chrony}/` (RF-3, merged) + `docs/agent/renderers/{unbound,chrony}.md`: Unbound listen/port/
  interface changes return a **restart request with a convergence check** (reload does not reopen sockets); pending restart requests persist
  until acted on (D-079, chrony M2 pattern)
- `task/RF-4:apps/agent/internal/renderers/rsyslog/` (branch, read with `git show`; merged before you start — dep RF-4): renders
  `management.syslog`, `rsyslogd -N1` validation, restart + impstats convergence, fixed templates only
- `apps/agent/internal/descriptors/dns/` (DF-8) + `docs/agent/descriptors/dns.md`: `dns.name-server`, `dns.enable` — VPP-global and
  **write-only** (D-063/D-076), registered only by the globals owner (D-071); enable needs a server first (ordering note)
- `apps/agent/binapi/dns/` — `dns_enable_disable`, `dns_name_server_add_del`, `dns_resolve_name` (verified)

## Contract changes
Needed only for leaves the UI cannot live without (e.g. log-explorer retention, syslog TLS CA ref as `cert/<name>` D-051): branch
`contract/F-unbound-chrony-syslog`, additive, `…-contract.md`, questions file, continue.

## Scope — build exactly this
Files you own: `apps/agent/internal/renderers/{unbound,chrony,rsyslog}/**`, `apps/agent/internal/descriptors/dns/**`,
`docs/agent/renderers/{unbound,chrony,rsyslog}.md`, `docs/agent/descriptors/dns.md`, `apps/agent/internal/agent/project_unbound_chrony_syslog*.go`,
`apps/api/src/features/unbound-chrony-syslog/**`, `apps/web/src/domains/services/unbound-chrony-syslog/**`,
`apps/web/src/locales/*/unbound-chrony-syslog.json`, `docs/user/services/unbound-chrony-syslog.md`, `test/topology/unbound-chrony-syslog/**`.
Shared files: one-line appends only (agent registry, `app.module.ts`, router/nav).
1. **Schema** (only if missing): VPP cache and an Unbound resolver must not listen on the same address:port; syslog TLS requires a CA ref.
2. **Agent**: project the three sections onto the renderers; `vppCache` → `dns.*` descriptors only in the globals owner (non-owners
   *require*, never set — D-071; host test opt-in, holds `flock -x /run/lock/vrx-globals.lock`, D-082); surface the renderers' restart
   requests as pending actions in `Retrieve`/health, never restart the system units from a test slot.
3. **API**: config via pointer routes; `GET /api/v1/state/dns` (unbound `stats_noreset`, forwards, local zones), `GET /api/v1/state/ntp`
   (chronyc tracking/sources), `GET /api/v1/state/logs?since&severity&facility&q&page` (log explorer over the files rsyslog writes —
   paged, bounded, read-only; no shell, no grep exec), `POST /api/v1/actions/dns-lookup` via `dns_resolve_name` with a deadline.
4. **UI**: Services → DNS (resolvers, forward zones, local zones/records, VPP cache), NTP (servers + live sync status), Logging (remote
   targets + log explorer ServerDataGrid with severity filter); en + fa; screenshot against the real endpoint.
5. **Docs**: `docs/user/services/unbound-chrony-syslog.md` — caching resolver with DNSSEC, NTP client + LAN server, remote syslog over TCP.

## Acceptance (paste the evidence)
- [ ] Slot Unbound instance answers `dig @<slot addr> <local record>`; chrony test instance `chronyc -h <slot sock> sources` lists the servers
- [ ] rsyslog test instance forwards a `logger` line to a slot TCP collector; the log explorer page shows it (screenshot)
- [ ] Agent-restart simulation → configs re-rendered, restart requests still pending if unacted (log excerpt)
- [ ] Rollback restores the previous rendered files (Retrieve / `list_local_data`), not assumption
- [ ] Forwarder equal to a listen address → 400 problem+json with a `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
Kea/DHCP (F-kea-dhcp-relay), SNMP (F-snmp), alarms/event engine and dashboards (F-dashboard-prom-alarms), support bundle (F-backup-restore),
PTP, NTS server certificates (refused by the chrony renderer), DNS views beyond what RF-3 renders, a SIEM/log database (explorer reads files).

## Open questions to surface, not to decide silently
Log explorer source: rsyslog local files vs journald — pick what RF-4 renders and say so. VPP DNS cache has no getter (write-only), so the UI
cannot show its live state — confirm "configured" is acceptable.
