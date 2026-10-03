# F-dashboard-prom-alarms — dashboard, Prometheus export, alarms (cloud session charming-johnson, 2026-09-27)

Prometheus metrics (agent exporter), a threshold alarm engine (API) with webhook notifications, an alarms UI and a
dashboard card, and an import-ready Grafana dashboard. User page: `docs/user/dashboard/dashboard-prom-alarms.md`.

## Decisions (the prompt's open questions)
- **Exporter path (one path, logged):** the **agent exporter** only (stdlib text, D-063). Buffers and node errors are
  emitted as Prometheus text; `/state/dashboard` is built from the existing StatsBatch + state routes. So the
  `StatsBatch.buffers/node_errors` and `DashboardSnapshot` proto additions in the number envelope were **not** taken.
  VPP's `prom_plugin.so` (startup.conf stanza) is documented as the alternative; enabling it is a manager step.
- **Email delivery:** modelled in the contract (`AlarmTarget` email) but not delivered — webhook only for now; email
  is left to a later notifications feature (the API records the target but does not send).
- **Alarm metrics the engine evaluates** come from the agent's live StreamStats + link events: `interface_rx_bps`,
  `interface_tx_bps`, `interface_rx_drops`, `interface_tx_drops`, `interface_link_down`, `worker_cpu_percent`.
  `buffer_used_percent` / `node_error_rate` are Prometheus-scraped (documented for Prometheus/Alertmanager).

## Built (all runs in this container)
- **Contract** (83aa23a8): schema `management.prometheus{enabled,listen,port,allow[]}`, `management.alarms{rules,
  targets}`; semantic `management.alarms` (targets exist; interface limit only on per-interface metrics and an
  existing interface); proto `ManagementConfig.prometheus=5, alarms=6` + ManagementPrometheus/ManagementAlarms/
  AlarmRule/AlarmTarget. Drift guard + buf breaking clean.
- **Agent** (6fa998ac): `internal/promexport` — Prometheus 0.0.4 text encoder (stdlib), StatsSource interface,
  Collect() (interface counters, worker CPU/vectors, buffers, node errors top-N), and an allow-listed external HTTP
  listener for `management.prometheus`. Fake-source + text-parser + allow-list + httptest tests.
- **API** (15b4ec2d): pure `evaluate()` (forSec hysteresis, per-interface) + `AlarmsService` (own StreamStats + link
  events off the bus; rules from `management.alarms`, reloaded on commit; raise/clear to the `alarm` table with a
  partial unique index so raise is idempotent; webhook delivery with timeout/retry and a token secret); routes
  `GET /state/alarms`, `POST /actions/alarms/{id}/ack`, `GET /state/dashboard`; migration `0005_dashboard_prom_alarms`;
  `alarm.events` bus topic.
- **Web** (c605436f): AlarmsCard (dashboard) + AlarmsPage (`/system/alarms`, ack); rules/targets edited via the
  generic Config → Management editor; en + fa. `deploy/grafana/ngfw-overview.json`.

## Evidence (this session)
- Agent: `go test ./internal/promexport/` (encoder/parser/allow-list/httptest) ok; go vet + golangci-lint clean;
  schema-proto drift guard passes; `buf breaking` clean.
- API: engine unit tests (hysteresis, per-interface, prune) + e2e (raise→webhook→list→ack→clear, idempotent
  re-raise, `/state/dashboard` summary, unknown-target rejected); unit 306/306.
- Web: 494/494, lint, `check-logical-css`.
- No VPP/Prometheus/Grafana in the container (host facts): the exporter is verified with a fake StatsSource and a
  text-format parser test; the Grafana JSON is import-ready (valid JSON, metric names match the exporter).

## Not done here → `F-dashboard-prom-alarms-host` (lab: VPP)
- The agent-side wiring of the real govpp `StatsSource` over the VPP stats segment and registering the metrics
  collector (manager hook) + the `management.prometheus` listener descriptor under `Domains["management"]`.
- Acceptance on the rig: `/metrics` shows `ngfw_interface_rx_bytes_total` increasing after traffic; link-down raises
  an alarm within 5 s and clears on link-up with the webhook receiving both; agent/API restart re-evaluation without
  duplicate notifications; screenshot of the dashboard against the real endpoint.
