# prom / http_static descriptors (DF-8, WBS D8.2/D8.3)

Package `apps/agent/internal/descriptors/prom` — VPP's static HTTP server, the listener the `prom` plugin serves
`/stats.prom` from. Message names only from `apps/agent/binapi/http_static`. `prom.RegisterGlobals(registry, client, owner)`.

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
- Create: success; a re-apply of the value this owner recorded for the running VPP process is skipped (D-076, below);
  any other `APP_ALREADY_ATTACHED` is `ErrServerBusy`. Update: `ErrNotSupported`. Delete: no-op (the listener runs until VPP restarts).
- Validation: uri `tcp://<ip>/<port>` or `tls://…` (canonical IP, port 1..65535), www_root absolute, no `..`,
  path characters only, 2..255 bytes. The deprecated `http_static_enable_v4` is not used.
- Host test: because enabling is irreversible until a VPP restart and changes the shared VPP (session layer), the full
  run is opt-in (`VRX_DF8_HTTP_STATIC=1`, listens on `127.0.0.1:$((VRX_METRICS_PORT+1))`, www root
  `/run/vrx-test/<prefix>/www`); by default the test checks message compatibility only. See `DF-8-questions.md`.

## Registration, ownership and restarts (D-069, D-071, D-074, D-076)
- There is no per-owner `Register` (everything here is VPP-global); `prom.RegisterGlobals(...)` registers the
  VPP-global singletons (`prom.http-static-server`) constructed as **globals owner** — P08 calls it only in the designated globals
  owner's agent (D-071). A descriptor constructed without the role (`dfkit.GlobalsOwner(false)`)
  only *requires* the value: Create succeeds when VPP already has it (checked through the getter where one exists,
  otherwise `dfkit.ErrNotGlobalsOwner`), Delete is a no-op, Retrieve is write-only.
- Interfaces are named by their **logical name** and resolved with DF-1's `iface.ResolveName` (D-069): this owner's
  tag id first, then an untagged interface's VPP name; another owner's interface fails with
  `iface.ErrForeignInterface`, local0 never resolves. Objects on an **untagged** interface (a DPDK NIC) are recorded
  in the owner's ClaimStore (`iface.Claims`, shared with DF-1; P05/P08 install a persisted one) on Create, released on
  Delete, and reported by Retrieve only while claimed (D-071 claim rule).
- Deletes re-resolve the logical name right before acting by sw_if_index (never a Meta index — indexes are reused
  after a VPP restart) and first check that the object still exists (D-074); "already gone" is success.
- Retrieve never reports a key twice (`dfkit.Dedupe`).
- **D-076:** `http_static_enable_v5` is not idempotent (`APP_ALREADY_ATTACHED`); the applied value is recorded in the owner's
  BootStore (`dfkit.Boot`, keyed by the VPP main-thread PID; P05/P08 install `dfkit.NewFileBootStore` in the state
  dir) and a resync on the same VPP process skips the re-add; after a VPP restart it is added once more.
- Restart simulation (fresh connection + fresh descriptors → empty plan; objects deleted via binapi → exactly their
  re-creation planned → empty plan again): `internal/descriptors/dfkit/restarttest`, output in `DF-8.md`.
