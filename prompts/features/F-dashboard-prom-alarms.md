# Task: F-dashboard-prom-alarms — dashboard, Prometheus export, alarms/events, tunnel dashboards   (prepend 00-CONTEXT.md)

## Goal
Operational visibility (WBS D8.1, D8.3, D8.4, D6.10 in `plan/wbs.csv`): a real dashboard (per-worker CPU, interface rates, drops/errors,
buffers, system health), a Prometheus endpoint with VRX metrics + Grafana dashboards, and an event/alarm engine (thresholds, raise/clear,
email/webhook; SNMP trap target via F-snmp later), plus tunnel dashboards for whatever tunnel state exists at start time.
Reference: TNSR "Dashboard", "Prometheus exporter"; VPP plugin `prom` and the stats segment.

## Inputs to read first
- `packages/proto/vrx/v1/dataplane.proto` — `StreamStats` → `StatsBatch{interface_counters, worker_cpu}`, `StreamEvents` → `Event`
  (`EVENT_KIND_LINK_UP/DOWN`, `RECONCILE_*`, `ERROR`, `CONFIRM_REVERTED`, `VPP_(DIS)CONNECTED`); P05 `apps/agent/internal/agent/{telemetry,
  metrics,events}.go` (existing agent metrics — reuse, do not fork)
- `task/P06:apps/api/src/telemetry/{relay.service,stream.route}.ts` and `state/state.controller.ts` (`/state/system`, `/state/events`) —
  the WS fan-out and state routes this builds on; `apps/web/src/pages/DashboardPage.tsx` (P07a placeholder) — replace its content only
- **V18 / D-077**: VPP's `prom_plugin.so` is on disk and loaded but has **no binary API** — it is enabled only through a `prom { … }` stanza in
  startup.conf (F-startup-gen, D-081; applying startup.conf is a manager step, never this task). Fallback that needs no VPP change: the
  agent's own exporter reading the stats segment (`apps/agent/internal/promexport/`)
- `docs/lab/shared-host-rules.md` — metrics port per slot `9100 + 10·N + 1` (D-025)
- tunnel state: only what is merged when you start (P11 IPsec SAs, F-wireguard peers, F-tunnels) — read their state routes, add none

## Contract changes
Alarm rules/targets need a home: add `management.alarms{rules{<name>: {metric, op, threshold, forSec, severity}}, targets{<name>: {kind:
email|webhook, url|address, secretRef?}}}` and `management.prometheus{enabled, listen, allow[]}` on branch `contract/F-dashboard-prom-alarms`
(schema + proto + drift guard, additive; webhook tokens are `token/<name>` refs, D-051), questions file, continue.

## Scope — build exactly this
Files you own: `apps/agent/internal/promexport/**`, `deploy/grafana/**`, `apps/agent/internal/agent/project_dashboard_prom_alarms*.go`,
`apps/api/src/features/dashboard-prom-alarms/**`, `apps/web/src/domains/dashboard/dashboard-prom-alarms/**`,
`apps/web/src/locales/*/dashboard-prom-alarms.json`, `docs/user/dashboard/dashboard-prom-alarms.md`, `test/topology/dashboard-prom-alarms/**`.
Shared files: one-line appends only (agent registry, `app.module.ts`, router/nav; `DashboardPage.tsx` may become a one-line re-export).
1. **Agent**: `promexport` — HTTP `/metrics` (Prometheus text format, no external deps beyond the Go client already vendored or stdlib)
   with interface counters, worker CPU/vectors, buffer usage, error counters by node (top-N), reconcile/apply counters and
   `retrieve_unsupported` (D-063); bound to the configured listen address, allow-list enforced; port from config (slot port in tests).
2. **API**: alarm engine as a Nest service consuming the stats/events relay: threshold rules with hysteresis (`forSec`), active/cleared
   alarm table in PostgreSQL, `GET /api/v1/state/alarms?active&page`, `POST /api/v1/actions/alarms/{id}/ack`, delivery to email/webhook
   (timeouts, retries, secrets never logged); `GET /api/v1/state/dashboard` summary (one call for the tiles).
3. **UI**: dashboard tiles + charts (interface bps/pps, worker CPU, drops, alarms list, system health); tunnel dashboard card listing
   IPsec/WireGuard/tunnel state **only for features already merged** (otherwise "not available"); alarm rules editor via SchemaForm; en + fa.
4. **Grafana**: `deploy/grafana/vrx-overview.json` (import-ready) using the metric names above; document the prom-plugin alternative.
5. **Docs**: `docs/user/dashboard/dashboard-prom-alarms.md` — scrape config, alarm rule example (link down, drops/s), webhook payload.

## Acceptance (paste the evidence)
- [ ] `curl http://127.0.0.1:<slot port>/metrics` shows `vrx_interface_rx_bytes_total{interface="<prefixed>"}` increasing after rig traffic
- [ ] Link-down on a slot interface raises an alarm within 5 s and clears on link-up; webhook receiver (slot port) got both (pasted)
- [ ] Agent/API restart simulation → exporter back, active alarms re-evaluated, no duplicate notifications (log excerpt)
- [ ] Rollback of an alarm rule removes it (DB/API state)
- [ ] Rule with an unknown metric → 400 problem+json with a `pointer`; screenshot of the dashboard against the real endpoint
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
Editing startup.conf / enabling the prom stanza on the host (F-startup-gen + manager), SNMP traps (F-snmp), syslog/log explorer
(F-unbound-chrony-syslog), packet capture/trace (F-capture-trace), IPsec SA/SPI inspection and rekey history internals (P11 owns the data),
cluster view (F-vrrp-config-sync), long-term metrics history DB (Prometheus is the history), buffer metadata tracker (needs VPP code).

## Open questions to surface, not to decide silently
Ship VPP's prom plugin (startup.conf stanza, V18) or only the agent exporter? Default: agent exporter; document prom. Email delivery needs an
SMTP relay config — model it in the contract or defer to webhook-only?
