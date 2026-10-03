#!/usr/bin/env bash
# test/topology/sdk-terraform-ansible/live.sh — a real NGFW API + real ngfw-agent on ONE slot, for the SDK / Terraform
# provider live runs (F-sdk-terraform-ansible). Everything carries the slot prefix (docs/lab/shared-host-rules.md).
#
#   live.sh up            pg-test database ngfw_<prefix>, ngfw-agent (NGFW_OWNER=<prefix>, slot socket/state/metrics),
#                         apps/api dist/main.js on the slot port, admin login → API key (admin role by default: the live runs also write management.users)
#   live.sh down          stop both processes BY PID, drop the database, delete the slot's sdk Valkey keys, remove state
#   live.sh run <cmd...>  up; <cmd> under `tools/lab lock shared` with NGFW_SDK_URL / NGFW_SDK_API_KEY_FILE exported; down
#   live.sh status        what is running
#
# Needs: `eval "$(tools/lab env <slot>)"` (NGFW_TEST_PREFIX, NGFW_HTTP_PORT, NGFW_AGENT_SOCKET, NGFW_VALKEY_DB, NGFW_METRICS_PORT),
# a built API (`pnpm --filter @ngfw/api build`) and agent (`make -C apps/agent build`).
# Secrets (bootstrap admin password, JWT key, API key) are generated per run and live only in /run/ngfw-test/<prefix>/
# (tmpfs, 0600) — never in the repository, never printed.
set -euo pipefail

ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
: "${NGFW_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
: "${NGFW_HTTP_PORT:?}" "${NGFW_AGENT_SOCKET:?}" "${NGFW_VALKEY_DB:?}" "${NGFW_METRICS_PORT:?}"
P=$NGFW_TEST_PREFIX
RUN="/run/ngfw-test/$P"
STATE="$RUN/sdk-agent-state"
LOGS="${NGFW_SDK_LOG_DIR:-/root/ngfw-wt/logs}"
KEYROLE="${NGFW_SDK_KEY_ROLE:-admin}"
VALKEY_PREFIX="ngfw:$P:sdk:"
URL="http://127.0.0.1:$NGFW_HTTP_PORT"

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
  [[ -x $ROOT/apps/agent/bin/ngfw-agent ]] || { echo "build the agent: make -C apps/agent build" >&2; exit 1; }
  [[ -f $ROOT/apps/api/dist/main.js ]] || { echo "build the API: pnpm --filter @ngfw/api build" >&2; exit 1; }
  alive "$RUN/sdk-api.pid" && { say "already up ($URL)"; return 0; }

  "$ROOT/deploy/dev/pg-test.sh" create "$P"
  # shellcheck disable=SC1091
  set -a; . "$RUN/pg.env"; set +a

  rm -f "$NGFW_AGENT_SOCKET"; mkdir -p "$STATE"
  NGFW_OWNER=$P NGFW_AGENT_SOCKET=$NGFW_AGENT_SOCKET NGFW_AGENT_STATE_DIR=$STATE NGFW_METRICS_PORT=$NGFW_METRICS_PORT \
    nohup "$ROOT/apps/agent/bin/ngfw-agent" >"$LOGS/F-sdk-terraform-ansible-agent.log" 2>&1 &
  echo $! >"$RUN/sdk-agent.pid"
  for _ in $(seq 100); do [[ -S $NGFW_AGENT_SOCKET ]] && break; sleep 0.2; done
  [[ -S $NGFW_AGENT_SOCKET ]] || { echo "agent did not open $NGFW_AGENT_SOCKET" >&2; down; exit 1; }
  say "agent up (pid $(cat "$RUN/sdk-agent.pid"), owner $P, socket $NGFW_AGENT_SOCKET)"

  (umask 077; rand 24 >"$RUN/sdk-admin.pw")
  (
    export NGFW_HTTP_PORT NGFW_VALKEY_DB NGFW_AGENT_SOCKET
    export NGFW_DATABASE_URL="$NGFW_PG_DSN" NGFW_VALKEY_PREFIX="$VALKEY_PREFIX" NGFW_AGENT_OWNER="$P" \
      NGFW_SECRET_KEY_FILE="$RUN/sdk-secret.key" NGFW_JWT_SECRET="$(rand 48)" \
      NGFW_BOOTSTRAP_ADMIN_PASSWORD="$(cat "$RUN/sdk-admin.pw")" NGFW_COOKIE_SECURE=0 NGFW_AGENT_TIMEOUT_MS=30000
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
  rm -f "$NGFW_AGENT_SOCKET" "$RUN/sdk-apikey" "$RUN/sdk-admin.pw" "$RUN/sdk-secret.key"
  rm -rf "$STATE"
  local n=0 k
  while read -r k; do [[ -n $k ]] && valkey-cli -n "$NGFW_VALKEY_DB" DEL "$k" >/dev/null && n=$((n + 1)); done \
    < <(valkey-cli -n "$NGFW_VALKEY_DB" --scan --pattern "${VALKEY_PREFIX}*")
  say "deleted $n Valkey keys ${VALKEY_PREFIX}* in db $NGFW_VALKEY_DB"
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
    NGFW_SDK_URL=$URL NGFW_SDK_API_KEY_FILE="$RUN/sdk-apikey" NGFW_INTEGRATION=1 \
      "$ROOT/tools/lab" lock shared "$@"
    ;;
  *) sed -n '2,15p' "$0" | sed -E 's/^# ?//'; exit 2 ;;
esac
