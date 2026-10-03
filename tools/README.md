Codegen and lab tooling. `tools/lab` (P04) drives the VMware lab over SSH; `tools/binapi-gen.sh` (P04) regenerates GoVPP bindings from the pinned VPP 26.06 API JSON.

`tools/app` runs the whole product on this host: `tools/app up` builds everything (pnpm + ngfw-agent), then starts ngfw-agent
(host VPP, owner `ngfw`), ngfw-api (127.0.0.1:3000, database `ngfw_app`) and the web UI on `0.0.0.0:8080` (proxies `/api`), so the UI
is reachable from other machines at `http://<host-ip>:8080/`. `down`, `status`, `logs [agent|api|web]`, `up --no-build`, `up --no-agent`.
Ports/host: `NGFW_APP_HOST`, `NGFW_APP_WEB_PORT`, `NGFW_APP_API_HOST`, `NGFW_APP_API_PORT`. `NGFW_APP_WEAK_PASSWORDS=1` (development only) lets the API accept passwords shorter than 12 characters; `status` warns while it is on. First-admin password: `/var/lib/ngfw-app/secrets.env` (0600).
`tools/app reset-admin [<user>]` asks for a new password and sets it directly in the database (ends the user's sessions, clears lockouts, creates the admin if missing). `tools/app reset-db [--yes] [--no-start] [--no-agent]` drops the whole `ngfw_app` database and the `ngfw:app:*` Valkey keys, generates a new first-admin password and starts the stack again (tables are recreated by the API's migrations at boot; the new password is printed).

`tools/slot-check.py` proves the shared-host slot scheme (slots 1–11 and 14–32 developers, 12 CI; D-156) has no port, id-range or database collision and that `tools/lab env` matches it; `tools/ci.sh check` and `quick` run it.

### Renamed scheduling boundary

NGFW tools use `/run/lock/ngfw-heavy-*` and `NGFW_*` controls. Finish all workers
using the older lock namespace before switching the scheduler; old and new
semaphores do not coordinate concurrent workers. This source change does not
migrate host locks or change running processes. The fast lane rejects inherited
legacy integration/nested-worker flags and handoff removes them before launching
a job, so an older checkout cannot accidentally enable laboratory execution or
bypass its semaphore from a renamed scheduler. These legacy flags are safety
refusals/sanitization only, not supported configuration aliases.
