#!/usr/bin/env bash
# Dev stack for the CLI e2e test and demo sessions, on ONE worker slot (docs/lab/shared-host-rules.md):
# vrx-api (apps/api/dist/main.js) on VRX_HTTP_PORT with the slot database and Valkey db, and — unless --no-agent —
# the real vrx-agent (apps/agent/bin/vrx-agent) as owner VRX_TEST_PREFIX on the slot socket.
#
#   eval "$(tools/lab env 3)"; deploy/dev/pg-test.sh create w3
#   apps/cli/test/devstack.sh start [--no-agent]   # prints nothing secret; admin password → $RUN/admin.pw (0600)
#   apps/cli/test/devstack.sh stop                  # stops exactly the PIDs it started
#
# Secrets (DB password, JWT key, bootstrap admin password) live only in /run/vrx-test/<prefix>/ (tmpfs, 0600) and
# in the processes' environment — never on the command line, never in the repository.
set -euo pipefail
: "${VRX_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
: "${VRX_HTTP_PORT:?}" "${VRX_VALKEY_DB:?}" "${VRX_AGENT_SOCKET:?}" "${VRX_METRICS_PORT:?}"
REPO=$(cd "$(dirname "$0")/../../.." && pwd)
RUN=/run/vrx-test/$VRX_TEST_PREFIX
LOGDIR=${VRX_DEVSTACK_LOGS:-$RUN}

start() {
  local agent=1
  [[ ${1:-} == --no-agent ]] && agent=0
  mkdir -p "$RUN" && chmod 700 "$RUN"
  [[ -f $RUN/pg.env ]] || { echo "no $RUN/pg.env — run deploy/dev/pg-test.sh create $VRX_TEST_PREFIX" >&2; exit 1; }
  umask 077
  [[ -s $RUN/admin.pw ]] || head -c 18 /dev/urandom | base64 | tr '+/' '-_' > "$RUN/admin.pw"
  [[ -s $RUN/jwt.key ]] || head -c 48 /dev/urandom | base64 | tr '+/' '-_' > "$RUN/jwt.key"
  if [[ $agent == 1 ]]; then
    [[ -x $REPO/apps/agent/bin/vrx-agent ]] || make -C "$REPO/apps/agent" build >/dev/null
    rm -f "$VRX_AGENT_SOCKET"
    VRX_OWNER=$VRX_TEST_PREFIX VRX_AGENT_SOCKET=$VRX_AGENT_SOCKET VRX_AGENT_STATE_DIR=$RUN/agent-state \
      VRX_METRICS_PORT=$VRX_METRICS_PORT nohup "$REPO/apps/agent/bin/vrx-agent" > "$LOGDIR/agent.log" 2>&1 &
    echo $! > "$RUN/agent.pid"
    for _ in $(seq 1 100); do [[ -S $VRX_AGENT_SOCKET ]] && break; sleep 0.1; done
    [[ -S $VRX_AGENT_SOCKET ]] || { echo "agent did not open $VRX_AGENT_SOCKET (log: $LOGDIR/agent.log)" >&2; exit 1; }
    echo "agent  pid $(cat "$RUN/agent.pid") owner $VRX_TEST_PREFIX socket $VRX_AGENT_SOCKET"
  fi
  [[ -f $REPO/apps/api/dist/main.js ]] || pnpm --dir "$REPO" --filter @ngfw/api build >/dev/null
  (
    set -a; . "$RUN/pg.env"; set +a
    VRX_JWT_SECRET=$(cat "$RUN/jwt.key") VRX_BOOTSTRAP_ADMIN_PASSWORD=$(cat "$RUN/admin.pw") \
    VRX_HTTP_PORT=$VRX_HTTP_PORT VRX_VALKEY_DB=$VRX_VALKEY_DB VRX_VALKEY_PREFIX="vrx:$VRX_TEST_PREFIX:cli:" \
    VRX_AGENT_SOCKET=$VRX_AGENT_SOCKET VRX_AGENT_OWNER=$VRX_TEST_PREFIX VRX_SECRET_KEY_FILE=$RUN/secret.key \
    VRX_COOKIE_SECURE=0 VRX_AGENT_TIMEOUT_MS=15000 \
      exec nohup node "$REPO/apps/api/dist/main.js" > "$LOGDIR/api.log" 2>&1
  ) &
  echo $! > "$RUN/api.pid"
  for _ in $(seq 1 150); do curl -fsS "http://127.0.0.1:$VRX_HTTP_PORT/api/v1/health" >/dev/null 2>&1 && break; sleep 0.2; done
  curl -fsS "http://127.0.0.1:$VRX_HTTP_PORT/api/v1/health" >/dev/null || { echo "API did not come up (log: $LOGDIR/api.log)" >&2; exit 1; }
  echo "api    pid $(cat "$RUN/api.pid") http://127.0.0.1:$VRX_HTTP_PORT (admin password in $RUN/admin.pw)"
}

stop() {
  for p in api agent; do
    if [[ -f $RUN/$p.pid ]]; then
      pid=$(cat "$RUN/$p.pid")
      if kill -0 "$pid" 2>/dev/null; then
        kill "$pid"
        for _ in $(seq 1 50); do kill -0 "$pid" 2>/dev/null || break; sleep 0.1; done
        echo "$p stopped (pid $pid)"
      fi
      rm -f "$RUN/$p.pid"
    fi
  done
  rm -rf "$RUN/agent-state" "$VRX_AGENT_SOCKET"
}

case ${1:-} in
  start) shift; start "$@" ;;
  stop) stop ;;
  *) echo "usage: $0 start [--no-agent] | stop" >&2; exit 2 ;;
esac
