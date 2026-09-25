# Task: F-dashboard-prom-alarms — dashboard, Prometheus export, alarms/events, tunnel dashboards   (prepend 00-CONTEXT.md)

## Goal
Operational visibility (WBS D8.1, D8.3, D8.4, D6.10 in `plan/wbs.csv`): a real dashboard (per-worker CPU, interface rates, drops/errors,
buffers, system health), a Prometheus endpoint with VRX metrics + Grafana dashboards, and an event/alarm engine (thresholds, raise/clear,
email/webhook; SNMP trap target via F-snmp later), plus tunnel dashboards for whatever tunnel state exists at start time.
Reference: TNSR "Dashboard", "Prometheus exporter"; VPP plugin `prom` and the stats segment.

## Inputs to read first
- `packages/proto/vrx/v1/dataplane.proto` — `StreamStats` → `StatsBatch{interface_counters, worker_cpu}`, `StreamEvents` → `Event`
  (`EVENT_KIND_LINK_UP/DOWN`, `RECONCILE_*`, `ERROR`, `CONFIRM_REVERTED`, `VPP_(DIS)CONNECTED`, `DEGRADED`); P05 `apps/agent/internal/agent/{telemetry,
  metrics,events}.go` (existing agent metrics — reuse, do not fork). **The agent already serves `/metrics`** on `VRX_METRICS_ADDR` /
  `127.0.0.1:$VRX_METRICS_PORT` (`agent/metrics.go`: hand-written text format 0.0.4, `vrx_agent_*` families incl.
  `vrx_agent_retrieve_unsupported_*` (D-063); **no `prometheus/client_golang` in go.mod** — stay stdlib). `internal/agent/*.go` is agent core
  (read-only, A5): your families plug into that endpoint through a collector hook the manager seeds (ask in the questions file if it is not
  on main — never edit agent.go/metrics.go yourself); background work starts from your own `subsystems/dashboard_prom_alarms*.go`
- `apps/api/src/telemetry/{relay.service,stream.route}.ts`, `apps/api/src/infra/bus.ts` (`TOPICS`), `apps/api/src/state/state.controller.ts`
  (`/state/system`, `/state/events`), `apps/api/src/audit/system-events.service.ts` (`system_event` table) — P06, merged: the WS fan-out and
  state routes this builds on. The relay opens `StreamStats` only while a WS client wants counters → the alarm engine takes its **own**
  upstream subscription through `AgentClient` (no relay edit). `apps/web/src/pages/DashboardPage.tsx` (P07a placeholder, only you touch it) —
  replace its content
- host facts (checked 2026-09-24): no Prometheus, promtool, Grafana, SMTP relay/`sendmail` installed (no package installs) — validate the
  exposition format with a Go/TS parser test, Grafana JSON stays import-ready only; `prometheus-node-exporter` is the **host's own** service
  on :9100 (active) — never touch it; vrx-a has **no VPP worker threads** (main core only), so worker CPU shows `vpp_main` only
- **V18 / D-077**: VPP's `prom_plugin.so` is on disk and loaded but has **no binary API** — it is enabled only through a `prom { … }` stanza in
  startup.conf (F-startup-gen, D-081; applying startup.conf is a manager step, never this task). Fallback that needs no VPP change: the
  agent's own exporter reading the stats segment (`apps/agent/internal/promexport/`)
- `docs/lab/shared-host-rules.md` — metrics port per slot `9100 + 10·N + 1` (D-025)
- tunnel state: only what is merged when you start (P11 IPsec SAs, F-wireguard peers, F-tunnels) — read their state routes, add none

## Contract changes
Alarm rules/targets need a home: add `management.alarms{rules{<name>: {metric, op, threshold, forSec, severity}}, targets{<name>: {kind:
email|webhook, url|address, secretRef?}}}` and `management.prometheus{enabled, listen, allow[]}` as separate `contract(schema): …` /
`contract(proto): …` commits on **your task branch** (no own branches; ManagementConfig numbers from your envelope /
`docs/status/wave-BC-numbers.md`; schema + proto + drift guard, additive; webhook tokens are `token/<name>` refs, D-051 — the alarm engine
runs in the API, which reads its own secret store, so no API→agent secret channel is needed), questions file, continue.

## Scope — build exactly this
Files you own and shared hotspots: your TASK ENVELOPE is authoritative (the board's old `agent/project_dashboard_prom_alarms*.go` became
`internal/desired/prometheus*.go` + `internal/subsystems/dashboard_prom_alarms*.go`, wave-A hotspots A2). Main pieces:
`apps/agent/internal/promexport/**`, `deploy/grafana/**`, `apps/api/src/features/dashboard-prom-alarms/**`,
`apps/web/src/domains/dashboard/dashboard-prom-alarms/**`, `apps/web/src/pages/DashboardPage.tsx` (may become a one-line re-export).
Shared files: registration lines under your anchor only (agent registry, `app.module.ts`, router, i18n, bus topic, DB schema/migrations).
1. **Agent**: `promexport` — Prometheus text format, **stdlib only** (like `agent/metrics.go`), with interface counters, worker CPU/vectors,
   buffer usage, error counters by node (top-N) from the stats segment (own govpp stats connection; the P05 reader is unexported core). The
   existing `vrx_agent_*` reconcile/apply/`retrieve_unsupported` families (D-063) are **reused** through the manager's collector hook, not
   re-implemented. `management.prometheus` → one singleton scheduler descriptor (D-109 d) under `Domains["management"]` (shared with
   F-unbound-chrony-syslog: whoever lands first adds the key) that runs the external listener on the configured address with the allow-list
   enforced (slot port in tests); the loopback `VRX_METRICS_PORT` endpoint stays as P05 built it.
2. **API**: alarm engine as a Nest service with its own upstream `StreamStats`/`StreamEvents` subscriptions (the relay's stats stream runs
   only while a WS client listens): threshold rules with hysteresis (`forSec`), active/cleared alarm table in PostgreSQL (DB migration
   protocol in your envelope), `GET /api/v1/state/alarms?active&page`, `POST /api/v1/actions/alarms/{id}/ack` (static route in your own
   controller), delivery to webhook (+ email only if the SMTP question is answered; timeouts, retries, secrets never logged);
   `GET /api/v1/state/dashboard` summary (one call for the tiles).
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
