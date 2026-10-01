# Dashboard, Prometheus export and alarms

Operational visibility: live dashboard tiles, a Prometheus metrics endpoint (with a ready-to-import Grafana
dashboard), and a threshold **alarm engine** that raises/clears alarms and notifies webhooks.

## Prometheus metrics
The agent serves the exposition format 0.0.4 at its `/metrics` endpoint (loopback, always on). To let an external
Prometheus scrape it, enable **Config → Management → Prometheus** (`management.prometheus`):

```
management.prometheus:
  enabled: true
  listen: 0.0.0.0
  port: 9101
  allow: ["10.0.0.0/8"]   # optional; empty = any source that reaches the listener
```

Scrape config:

```yaml
scrape_configs:
  - job_name: vrx
    static_configs: [{ targets: ["<appliance>:9101"] }]
```

The real govpp source publishes `vrx_interface_rx_bytes_total`, `vrx_interface_tx_bytes_total`,
`vrx_interface_rx_packets_total`, `vrx_interface_tx_packets_total`, `vrx_interface_rx_errors_total`,
`vrx_interface_tx_errors_total` and `vrx_interface_drops_total` (`interface`). VPP's stats segment only
provides aggregate interface drops: directional drop samples and admin/link gauges are omitted rather than
reported as zero. Link events used by the API alarm engine continue through the existing agent event stream.
`vrx_worker_vectors_per_call` and `vrx_worker_clocks_per_vector` (`worker`) are cumulative node-counter
ratios for each stats thread (`vpp_main`, `vpp_worker_1`, …), not CPU utilization percentages.
`vrx_buffer_used`, `vrx_buffer_available`, `vrx_buffer_used_percent` (`pool`) and `vrx_node_errors_total`
(`node`, `reason`, top-N) come from the same dedicated stats connection. Failed reads discard the mapping;
the next scrape reconnects. The loopback endpoint also includes the agent's `vrx_agent_*` families.
The configurable external listener exposes dataplane families and enforces its CIDR allow-list, including
updates on the same address; disabling or rolling back its singleton closes the owned socket.

Import `deploy/grafana/vrx-overview.json` into Grafana (pick your Prometheus data source). VPP's `prom_plugin.so`
is an alternative exporter enabled through a `prom { … }` stanza in the startup configuration (a manager step); the
agent exporter above needs no VPP change.

## Alarms
Alarm rules and notification targets live in **Config → Management → Alarms** (`management.alarms`):

```
management.alarms:
  targets:
    ops:   { kind: webhook, url: "https://hooks.example.net/vrx", secretRef: "token/ops-hook" }
  rules:
    wan-down:  { metric: interface_link_down, op: ge, threshold: 1, severity: critical, interface: TenGigabitEthernet0/0/0, targets: [ops] }
    rx-drops:  { metric: interface_rx_drops, op: gt, threshold: 100, forSec: 30, severity: warning, targets: [ops] }
```

- **Metrics the engine evaluates** (from the agent's live metric stream): `interface_rx_bps`, `interface_tx_bps`,
  `interface_rx_drops`, `interface_tx_drops`, `interface_link_down`, `worker_cpu_percent`. `buffer_used_percent` and
  `node_error_rate` are exported for Prometheus scraping and are best alerted through Prometheus/Alertmanager.
- **op**: `gt` `ge` `lt` `le` `eq`. **forSec**: the condition must hold this long before the alarm is raised
  (hysteresis); it clears as soon as the condition stops holding.
- **severity**: `info` | `warning` | `critical`.
- **targets**: names of `management.alarms.targets`. A `webhook` target is POSTed the alarm JSON (below) on raise and
  clear; its bearer token is a `token/<name>` secret, never inline. (Email targets are modelled for a later
  notifications feature.)
- A rule with an unknown target, or an interface metric limited to an interface that does not exist, is refused at
  commit with a JSON-pointer error.

Webhook payload:

```json
{ "kind": "raised", "rule": "wan-down", "instance": "TenGigabitEthernet0/0/0", "metric": "interface_link_down",
  "severity": "critical", "value": 1, "threshold": 1, "message": "…", "at": "2026-09-27T10:00:00.000Z" }
```

### Viewing alarms
**System → Alarms** lists active and historical alarms; **Acknowledge** records who and when (it does not clear the
alarm — an alarm clears only when its condition ends). The dashboard's **Active alarms** card shows the current counts
by severity. `ALARM_RAISED` / `ALARM_CLEARED` also appear in the system event log; failed webhook deliveries raise
`ALARM_NOTIFY_FAILED`.
