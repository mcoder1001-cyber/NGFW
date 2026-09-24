# auto_sdl descriptor (F-rpf-adl-pbr, WBS D2.4)

Package `apps/agent/internal/descriptors/auto_sdl` (Go package `autosdl`). Messages from `apps/agent/binapi/auto_sdl` only.

| Object | Descriptor / key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| Auto-SDL (VPP-global) | `auto-sdl.config` / `auto-sdl.config/global` | Create: `auto_sdl_config{enable=0}` then `auto_sdl_config{enable=1, threshold, remove_timeout}`, once per VPP instance (applied-once record in the owner's BootStore, D-076/D-080); Update → `ErrRecreate`; Delete: `auto_sdl_config{enable=0}` only when this owner enabled it on the running VPP instance | **none — write-only** (`dfkit.ErrRetrieveUnsupported`: no getter) | none | Registered **only by the globals owner** (D-071). Value: `dfkit.Encode(autosdl.Config{enable, threshold, remove_timeout})`; `enable=false` is never desired (omit the object). |

Projection: `services.autoSdl{enabled: true, threshold, removeTimeoutSec}` → the singleton (defaults 5 / 300, VPP's
`auto_sdl.api` defaults). `enabled: false` or absent → no object. A non-owner agent reports `services.autoSdl` as
`agent.unsupported-field` in DryRun and applies nothing.

## VPP 26.06 behaviour the descriptor works around (`plugins/auto_sdl/auto_sdl.c`)
- `auto_sdl_config` answers `FEATURE_DISABLED` (-30) unless the session layer's SDL backend is enabled
  (`session_sdl_is_enabled`; startup.conf `session { enable rt-backend sdl }`). The descriptor returns
  `autosdl.ErrSessionSDLDisabled`. On vrx-a the session layer is off (`show session`: "session layer is not enabled";
  startup.conf is handover-gated), so the host test skips.
- An enable while enabled is a silent no-op that keeps the **old** threshold and timeout (`session_sdl_register_callbacks`
  fails first; the API handler ignores the error). Create therefore disables first.
- Disable removes every automatic entry — the reason the applied-once record exists: a resync (D-063 re-apply) or an agent
  restart on the same VPP instance sends nothing.

## Tests
- Unit (`auto_sdl_test.go`): model of FEATURE_DISABLED, the no-op second enable, the flush on disable; applied-once across
  a re-apply and a descriptor restart on the same store; one more apply after a VPP restart (identity change).
- Host (`integration_test.go`): opt-in `VRX_AUTOSDL_GLOBALS=1` (a getter-less VPP-global: the previous value cannot be
  restored; manager window only), `flock -x /run/lock/vrx-globals.lock`, skip-unless-supported on FEATURE_DISABLED.

CLI: `show auto-sdl` (VPP), `auto-sdl <enable|disable> [threshold <n>] [remove-timeout <t>]`.
