#!/usr/bin/env bash
# test/topology/sdk-terraform-ansible/live.sh — a real VRX API + real vrx-agent on ONE slot, for the SDK / Terraform
# provider live runs (F-sdk-terraform-ansible). Everything carries the slot prefix (docs/lab/shared-host-rules.md).
#
#   live.sh up            pg-test database vrx_<prefix>, vrx-agent (VRX_OWNER=<prefix>, slot socket/state/metrics),
#                         apps/api dist/main.js on the slot port, admin login → API key (admin role by default: the live runs also write management.users)
#   live.sh down          stop both processes BY PID, drop the database, delete the slot's sdk Valkey keys, remove state
#   live.sh run <cmd...>  up; <cmd> under `tools/lab lock shared` with VRX_SDK_URL / VRX_SDK_API_KEY_FILE exported; down
#   live.sh status        what is running
#
# Needs: `eval "$(tools/lab env <slot>)"` (VRX_TEST_PREFIX, VRX_HTTP_PORT, VRX_AGENT_SOCKET, VRX_VALKEY_DB, VRX_METRICS_PORT),
# a built API (`pnpm --filter @ngfw/api build`) and agent (`make -C apps/agent build`).
# Secrets (bootstrap admin password, JWT key, API key) are generated per run and live only in /run/vrx-test/<prefix>/
# (tmpfs, 0600) — never in the repository, never printed.
set -euo pipefail

ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
: "${VRX_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
: "${VRX_HTTP_PORT:?}" "${VRX_AGENT_SOCKET:?}" "${VRX_VALKEY_DB:?}" "${VRX_METRICS_PORT:?}"
P=$VRX_TEST_PREFIX
RUN="/run/vrx-test/$P"
STATE="$RUN/sdk-agent-state"
LOGS="${VRX_SDK_LOG_DIR:-/root/ngfw-wt/logs}"
KEYROLE="${VRX_SDK_KEY_ROLE:-admin}"
VALKEY_PREFIX="vrx:$P:sdk:"
URL="http://127.0.0.1:$VRX_HTTP_PORT"

say() { echo "live: $*"; }
rand() { head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c "${1:-32}"; }
alive() { [[ -f $1 ]] && kill -0 "$(cat "$1")" 2>/dev/null; }

stop_pid() {  # stop_pid <pidfile> <name> — only the PID this script started
  local f=$1 name=$2
  if alive "$f"; then
    local pid; pid=$(cat "$f"); kill -TERM "$pid"
    for _ in $(seq 50); do kill -0 "$pid" 2>/dev/null || break; sleep 0.2; done
    kill -0 "$pid" 2>/dev/null && { kill -KILL "$pid"; say "$name pid $pid killed (SIGKILL)"; } || say "$name stopped (pid $pid)"
  fi
  rm -f "$f"
}

up() {
  install -d -m 0750 "$RUN"; mkdir -p "$LOGS"
  [[ -x $ROOT/apps/agent/bin/vrx-agent ]] || { echo "build the agent: make -C apps/agent build" >&2; exit 1; }
  [[ -f $ROOT/apps/api/dist/main.js ]] || { echo "build the API: pnpm --filter @ngfw/api build" >&2; exit 1; }
  alive "$RUN/sdk-api.pid" && { say "already up ($URL)"; return 0; }

  "$ROOT/deploy/dev/pg-test.sh" create "$P"
  # shellcheck disable=SC1091
  set -a; . "$RUN/pg.env"; set +a

  rm -f "$VRX_AGENT_SOCKET"; mkdir -p "$STATE"
  VRX_OWNER=$P VRX_AGENT_SOCKET=$VRX_AGENT_SOCKET VRX_AGENT_STATE_DIR=$STATE VRX_METRICS_PORT=$VRX_METRICS_PORT \
    nohup "$ROOT/apps/agent/bin/vrx-agent" >"$LOGS/F-sdk-terraform-ansible-agent.log" 2>&1 &
  echo $! >"$RUN/sdk-agent.pid"
  for _ in $(seq 100); do [[ -S $VRX_AGENT_SOCKET ]] && break; sleep 0.2; done
  [[ -S $VRX_AGENT_SOCKET ]] || { echo "agent did not open $VRX_AGENT_SOCKET" >&2; down; exit 1; }
  say "agent up (pid $(cat "$RUN/sdk-agent.pid"), owner $P, socket $VRX_AGENT_SOCKET)"

  (umask 077; rand 24 >"$RUN/sdk-admin.pw")
  (
    export VRX_HTTP_PORT VRX_VALKEY_DB VRX_AGENT_SOCKET
    export VRX_DATABASE_URL="$VRX_PG_DSN" VRX_VALKEY_PREFIX="$VALKEY_PREFIX" VRX_AGENT_OWNER="$P" \
      VRX_SECRET_KEY_FILE="$RUN/sdk-secret.key" VRX_JWT_SECRET="$(rand 48)" \
      VRX_BOOTSTRAP_ADMIN_PASSWORD="$(cat "$RUN/sdk-admin.pw")" VRX_COOKIE_SECURE=0 VRX_AGENT_TIMEOUT_MS=30000
    cd "$ROOT/apps/api" && exec nohup node dist/main.js >"$LOGS/F-sdk-terraform-ansible-api.log" 2>&1
  ) &
  echo $! >"$RUN/sdk-api.pid"
  for _ in $(seq 100); do curl -fsS "$URL/api/v1/health" >/dev/null 2>&1 && break; sleep 0.3; done
  curl -fsS "$URL/api/v1/health" >/dev/null || { echo "API did not come up (log: $LOGS/F-sdk-terraform-ansible-api.log)" >&2; down; exit 1; }
  say "api up (pid $(cat "$RUN/sdk-api.pid"), $URL)"

  # admin login → an API key for the automation clients (shown once; stored 0600 in /run, never printed)
  python3 - "$URL" "$RUN/sdk-admin.pw" "$RUN/sdk-apikey" "$KEYROLE" <<'PY'
import json, os, sys, urllib.request
url, pwfile, keyfile, role = sys.argv[1:5]
def call(method, path, body=None, auth=None):
    req = urllib.request.Request(url + path, method=method, data=None if body is None else json.dumps(body).encode(),
                                 headers={"content-type": "application/json", **({"authorization": auth} if auth else {})})
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.loads(r.read() or b"null")
tok = call("POST", "/api/v1/auth/login", {"username": "admin", "password": open(pwfile).read().strip()})["accessToken"]
k = call("POST", "/api/v1/auth/api-keys", {"name": "sdk-live", "role": role, "expiresInDays": 1, "current": open(pwfile).read().strip()}, "Bearer " + tok)
fd = os.open(keyfile, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
os.write(fd, k["key"].encode()); os.close(fd)
print(f"live: API key '{k['name']}' role={k['role']} id={k['id']} → {keyfile} (0600, not printed)")
PY
}

down() {
  stop_pid "$RUN/sdk-api.pid" api
  stop_pid "$RUN/sdk-agent.pid" agent
  rm -f "$VRX_AGENT_SOCKET" "$RUN/sdk-apikey" "$RUN/sdk-admin.pw" "$RUN/sdk-secret.key"
  rm -rf "$STATE"
  local n=0 k
  while read -r k; do [[ -n $k ]] && valkey-cli -n "$VRX_VALKEY_DB" DEL "$k" >/dev/null && n=$((n + 1)); done \
    < <(valkey-cli -n "$VRX_VALKEY_DB" --scan --pattern "${VALKEY_PREFIX}*")
  say "deleted $n Valkey keys ${VALKEY_PREFIX}* in db $VRX_VALKEY_DB"
  "$ROOT/deploy/dev/pg-test.sh" drop "$P" || true
}

status() {
  alive "$RUN/sdk-agent.pid" && say "agent pid $(cat "$RUN/sdk-agent.pid")" || say "agent not running"
  alive "$RUN/sdk-api.pid" && say "api pid $(cat "$RUN/sdk-api.pid") $URL" || say "api not running"
}

case "${1:-}" in
  up) up ;;
  down) down ;;
  status) status ;;
  run)
    shift; (($#)) || { echo "live.sh run <cmd...>" >&2; exit 2; }
    trap down EXIT
    up
    VRX_SDK_URL=$URL VRX_SDK_API_KEY_FILE="$RUN/sdk-apikey" VRX_INTEGRATION=1 \
      "$ROOT/tools/lab" lock shared "$@"
    ;;
  *) sed -n '2,15p' "$0" | sed -E 's/^# ?//'; exit 2 ;;
esac
