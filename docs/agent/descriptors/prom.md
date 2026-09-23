# prom / http_static descriptors (DF-8, WBS D8.2/D8.3)

Package `apps/agent/internal/descriptors/prom` — VPP's static HTTP server, the listener the `prom` plugin serves
`/stats.prom` from. Message names only from `apps/agent/binapi/http_static`. `prom.Register(registry, client)`.

| Object type | Key | VPP messages | Retrieve | Update | Delete |
|---|---|---|---|---|---|
| `prom.http-static-server` (singleton) | `prom.http-static-server/global` | `http_static_enable_v5` (uri, www_root, fifo/cache sizes, max_age, keepalive, max_body_size, rx_buff_thresh, prealloc_fifos, private_segment_size) | **write-only** (`ErrRetrieveUnsupported`) | `dfkit.ErrNotSupported` (needs a VPP restart) | no-op (no disable API) |
| `prom-exporter` | — | **none: not API-configurable** | — | — | — |

## Why there is no prom-exporter descriptor
The `prom` plugin (loaded, `prom_plugin.so`) has **no `.api` file** in VPP 26.06 (`src/plugins/prom`: prom.c, prom_cli.c
only), so there is no binapi package and no binary-API message. It is enabled and tuned only by the CLI (`prom enable`,
`prom stat-patterns`, `prom min-scrape-interval`, `prom used-only`) or `startup.conf`. Shelling out to `vppctl` is
forbidden (00-CONTEXT rules 1 and 9), so the exporter is documented as not API-configurable and raised in
`DF-8-questions.md` (options: startup.conf via F-startup-gen, a VPP API patch on the code track, or the agent's own
stats-segment exporter P05/F-*).

## http_static semantics (VPP 26.06, `http_static.c`)
- `hss_enable_api` accepts one enable per VPP process (`APP_ALREADY_ATTACHED` afterwards), has no disable, no
  reconfigure and no getter, and switches the **session layer** on (`vnet_session_enable_disable`) for everyone.
- Create: success; on `APP_ALREADY_ATTACHED` success only if this process enabled the identical server (write-only
  re-apply), else `ErrServerBusy`. Update: `ErrNotSupported`. Delete: no-op (the listener runs until VPP restarts).
- Validation: uri `tcp://<ip>/<port>` or `tls://…` (canonical IP, port 1..65535), www_root absolute, no `..`,
  path characters only, 2..255 bytes. The deprecated `http_static_enable_v4` is not used.
- Host test: because enabling is irreversible until a VPP restart and changes the shared VPP (session layer), the full
  run is opt-in (`VRX_DF8_HTTP_STATIC=1`, listens on `127.0.0.1:$((VRX_METRICS_PORT+1))`, www root
  `/run/vrx-test/<prefix>/www`); by default the test checks message compatibility only. See `DF-8-questions.md`.
