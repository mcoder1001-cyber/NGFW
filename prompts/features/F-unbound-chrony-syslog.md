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
- `apps/agent/internal/renderers/rsyslog/` (RF-4, merged) + `docs/agent/renderers/rsyslog.md`: renders `management.syslog`, `rsyslogd -N1`
  validation, restart + impstats convergence, fixed templates only — it emits **only `omfwd`** (forwarding); it writes no local log file
  (`omfile` is forbidden by its template rules). It reads D-055 stand-ins on `management.syslog[i]`: `facilities`, `format`, `queueSize`,
  `tls{caRef,certRef,keyRef,authMode,permittedPeers}` (RF-4-questions Q2, D-086).
- `apps/agent/internal/descriptors/dns/` (DF-8) + `docs/agent/descriptors/dns.md`: `dns.name-server`, `dns.enable` — VPP-global and
  **write-only** (D-063/D-076), registered only by the globals owner via `dns.RegisterGlobals` (D-071); enable needs a server first
  (ordering note); `dns.ResolveName(ctx, …)` needs a ctx deadline
- `apps/agent/binapi/dns/` — `dns_enable_disable`, `dns_name_server_add_del`, `dns_resolve_name` (verified)
- **P08** (vertical slice) patterns you extend: builders in `apps/agent/internal/desired/`, registration + `Domains` in
  `apps/agent/internal/subsystems/subsystems.go` (`Env.GlobalsOwner` = D-071), the hook in `apps/agent/internal/agent/projection.go`,
  read-only state RPCs like `InterfaceState`. `Domains["services"]` is shared (F-rpf-adl-pbr, F-kea-dhcp-relay, later F-snmp …) and you are
  the first to add `Domains["management"]` (syslog only; the other management leaves stay `agent.unsupported-field`). Restart-safety trap:
  the agent persists only implemented domains (`agent/state.go` `mergeDomains`).
- **Agent integration fact (checked 2026-09-24):** no renderer is called anywhere in the agent (`agent/service.go` runs only the descriptor
  scheduler). Default (same as F-host-acl-nftables, log it as a decision with options): wrap each renderer in one singleton scheduler
  descriptor (`unbound.config/vrx`, `chrony.config/vrx`, `rsyslog.config/vrx`: Create/Update = Render → Validate → Apply, Retrieve = the
  renderer's Retrieve), registered under `Domains["services"]` / `["management"]` — no change to the agent core; use a shared renderer
  stage instead only if the manager has put one on main when you start. Secret-bearing leaves (syslog TLS key, chrony keys) need the
  API→agent secret channel, which does not exist yet: refuse them with a clear DryRun error until it lands and say so.
- Host facts: `chrony.service` and `rsyslog.service` are the **host's own** time and logging (active, enabled) — never stop, restart,
  reconfigure or disable them; `unbound.service` is installed, disabled. Test instances only (RF-3/RF-4 `TestPaths`). No DNS query tool is
  installed (`dig`, `drill`, `host` are all missing).

## Contract changes
Move RF-4's D-055 stand-ins into the contract (D-086: "additive contract branch when F-unbound-chrony-syslog starts"):
`management.syslog[i].{facilities, format, queueSize, tls{caRef (cert/<name>), certRef, keyRef (key/<name>), authMode, permittedPeers}}`,
plus leaves the UI cannot live without (e.g. log-explorer retention): branch `contract/F-unbound-chrony-syslog`, additive (schema + proto +
drift guard), `…-contract.md`, questions file, continue. Proto field/rpc/oneof numbers come from the manager, never "next free".

## Scope — build exactly this
Files you own: `apps/agent/internal/renderers/{unbound,chrony,rsyslog}/**`, `apps/agent/internal/descriptors/dns/**`,
`docs/agent/renderers/{unbound,chrony,rsyslog}.md`, `docs/agent/descriptors/dns.md`, `apps/agent/internal/desired/{dns,ntp,syslog}*.go`,
`apps/agent/internal/subsystems/{unbound,chrony,rsyslog,dns}*.go`, `apps/agent/internal/agent/rpc_{dns,ntp,logs}*.go`,
`apps/agent/internal/actions/unbound-chrony-syslog/**`, `apps/api/src/features/unbound-chrony-syslog/**`,
`apps/api/test/e2e/unbound-chrony-syslog*`, `apps/web/src/domains/services/unbound-chrony-syslog/**`,
`apps/web/src/locales/*/unbound-chrony-syslog.json`, `docs/user/services/unbound-chrony-syslog.md`, `test/topology/unbound-chrony-syslog/**`.
Shared files: registration hunks only, listed in your PR (`subsystems.go` `Domains["services"]`/`["management"]` + Register lines,
`projection.go` hook, `server.go` Action case for the lookup, proto rpc/oneof/field numbers from the manager, `agent.client.ts`,
`fake-agent.ts`, `app.module.ts`, `renderers/ALLOWLIST.md` rows, router/nav, `i18n.ts`, the services page's tab registry).
1. **Schema** (only if missing): VPP cache and an Unbound resolver must not listen on the same address:port; syslog TLS requires a CA ref.
2. **Agent**: project the three sections onto the renderers; `vppCache` → `dns.*` descriptors only in the globals owner (non-owners
   *require*, never set — D-071; host test opt-in, holds `flock -x /run/lock/vrx-globals.lock`, D-082); surface the renderers' restart
   requests as pending actions in `Retrieve`/health, never restart the system units from a test slot.
3. **API**: config via pointer routes; `GET /api/v1/state/dns` (unbound `stats_noreset`, forwards, local zones), `GET /api/v1/state/ntp`
   (chronyc tracking/sources), `GET /api/v1/state/logs?since&severity&facility&q&page` (log explorer — paged, bounded, read-only; no
   shell, no grep exec; RF-4 writes no local file, so the source is journald through a fixed-argv `journalctl -o json` with bounded
   `--since/--lines`, allow-listed in `renderers/ALLOWLIST.md` — see open questions), `POST /api/v1/actions/dns-lookup` via
   `dns_resolve_name` with a deadline (works only where the VPP dns plugin is enabled, i.e. the globals owner; on a slot test it through
   the fake and state that).
4. **UI**: Services → DNS (resolvers, forward zones, local zones/records, VPP cache), NTP (servers + live sync status), Logging (remote
   targets + log explorer ServerDataGrid with severity filter); en + fa; screenshot against the real endpoint.
5. **Docs**: `docs/user/services/unbound-chrony-syslog.md` — caching resolver with DNSSEC, NTP client + LAN server, remote syslog over TCP.

## Acceptance (paste the evidence)
- [ ] Slot Unbound instance answers a query for a local record at its slot loopback address/port (no `dig` on the host: use a Go
      `net.Resolver` pinned to that address, the RF-3 integration-test pattern, and paste its output); chrony test instance
      `chronyc -h <slot sock> sources` lists the servers
- [ ] rsyslog test instance forwards a `logger -u <slot log.sock>` line (never plain `logger`: it writes to the host's `/dev/log`) to a slot
      TCP collector; the log explorer page shows a line from its source (screenshot)
- [ ] Agent-restart simulation → configs re-rendered, restart requests still pending if unacted (log excerpt)
- [ ] Rollback restores the previous rendered files (Retrieve / `list_local_data`), not assumption
- [ ] Forwarder equal to a listen address → 400 problem+json with a `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
Kea/DHCP (F-kea-dhcp-relay), SNMP (F-snmp), alarms/event engine and dashboards (F-dashboard-prom-alarms), support bundle (F-backup-restore),
PTP, NTS server certificates (refused by the chrony renderer), DNS views beyond what RF-3 renders, a SIEM/log database (explorer reads files).

## Open questions to surface, not to decide silently
Log explorer source: RF-4 renders only `omfwd`, so there are no rsyslog-written files to read. Default: journald via fixed-argv
`journalctl -o json` (bounded, allow-listed); the alternative — adding an `omfile` target to RF-4 — breaks its "fixed templates, never
omfile" rule and needs a manager decision. Say which you built. VPP DNS cache has no getter (write-only), so the UI cannot show its live
state — confirm "configured" is acceptable.
