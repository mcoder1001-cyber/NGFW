Codegen and lab tooling. `tools/lab` (P04) drives the VMware lab over SSH; `tools/binapi-gen.sh` (P04) regenerates GoVPP bindings from the pinned VPP 26.06 API JSON.

`tools/app` runs the whole product on this host: `tools/app up` builds everything (pnpm + vrx-agent), then starts vrx-agent
(host VPP, owner `vrx`), vrx-api (127.0.0.1:3000, database `vrx_app`) and the web UI on `0.0.0.0:8080` (proxies `/api`), so the UI
is reachable from other machines at `http://<host-ip>:8080/`. `down`, `status`, `logs [agent|api|web]`, `up --no-build`, `up --no-agent`.
Ports/host: `VRX_APP_HOST`, `VRX_APP_WEB_PORT`, `VRX_APP_API_HOST`, `VRX_APP_API_PORT`. First-admin password: `/var/lib/vrx-app/secrets.env` (0600).
