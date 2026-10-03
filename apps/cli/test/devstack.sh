#!/usr/bin/env bash
# Dev stack for the CLI e2e test and demo sessions, on ONE worker slot (docs/lab/shared-host-rules.md):
# ngfw-api (apps/api/dist/main.js) on NGFW_HTTP_PORT with the slot database and Valkey db, and — unless --no-agent —
# the real ngfw-agent (apps/agent/bin/ngfw-agent) as owner NGFW_TEST_PREFIX on the slot socket.
#
#   eval "$(tools/lab env 3)"; deploy/dev/pg-test.sh create w3
#   apps/cli/test/devstack.sh start [--no-agent]   # prints nothing secret; admin password → $RUN/admin.pw (0600)
#   apps/cli/test/devstack.sh stop [--keep]         # stops exactly the PIDs it started; removes the slot secrets and logs
#                                                   # it created (admin.pw, jwt.key, secret.key, api.log, agent.log) unless --keep
#
# Secrets (DB password, JWT key, bootstrap admin password) live only in /run/ngfw-test/<prefix>/ (tmpfs, 0600) and
# in the processes' environment — never on the command line, never in the repository.
set -euo pipefail
: "${NGFW_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
: "${NGFW_HTTP_PORT:?}" "${NGFW_VALKEY_DB:?}" "${NGFW_AGENT_SOCKET:?}" "${NGFW_METRICS_PORT:?}"
REPO=$(cd "$(dirname "$0")/../../.." && pwd)
RUN=/run/ngfw-test/$NGFW_TEST_PREFIX
LOGDIR=${NGFW_DEVSTACK_LOGS:-$RUN}

start() {
  local agent=1
  [[ ${1:-} == --no-agent ]] && agent=0
  mkdir -p "$RUN" && chmod 700 "$RUN"
  [[ -f $RUN/pg.env ]] || { echo "no $RUN/pg.env — run deploy/dev/pg-test.sh create $NGFW_TEST_PREFIX" >&2; exit 1; }
  umask 077
  [[ -s $RUN/admin.pw ]] || head -c 18 /dev/urandom | base64 | tr '+/' '-_' > "$RUN/admin.pw"
  [[ -s $RUN/jwt.key ]] || head -c 48 /dev/urandom | base64 | tr '+/' '-_' > "$RUN/jwt.key"
  if [[ $agent == 1 ]]; then
    [[ -x $REPO/apps/agent/bin/ngfw-agent ]] || make -C "$REPO/apps/agent" build >/dev/null
    rm -f "$NGFW_AGENT_SOCKET"
    NGFW_OWNER=$NGFW_TEST_PREFIX NGFW_AGENT_SOCKET=$NGFW_AGENT_SOCKET NGFW_AGENT_STATE_DIR=$RUN/agent-state \
      NGFW_METRICS_PORT=$NGFW_METRICS_PORT nohup "$REPO/apps/agent/bin/ngfw-agent" > "$LOGDIR/agent.log" 2>&1 &
    echo $! > "$RUN/agent.pid"
    for _ in $(seq 1 100); do [[ -S $NGFW_AGENT_SOCKET ]] && break; sleep 0.1; done
    [[ -S $NGFW_AGENT_SOCKET ]] || { echo "agent did not open $NGFW_AGENT_SOCKET (log: $LOGDIR/agent.log)" >&2; exit 1; }
    echo "agent  pid $(cat "$RUN/agent.pid") owner $NGFW_TEST_PREFIX socket $NGFW_AGENT_SOCKET"
  fi
  [[ -f $REPO/apps/api/dist/main.js ]] || pnpm --dir "$REPO" --filter @ngfw/api build >/dev/null
  (
    set -a; . "$RUN/pg.env"; set +a
    NGFW_JWT_SECRET=$(cat "$RUN/jwt.key") NGFW_BOOTSTRAP_ADMIN_PASSWORD=$(cat "$RUN/admin.pw") \
    NGFW_HTTP_PORT=$NGFW_HTTP_PORT NGFW_VALKEY_DB=$NGFW_VALKEY_DB NGFW_VALKEY_PREFIX="ngfw:$NGFW_TEST_PREFIX:cli:" \
    NGFW_AGENT_SOCKET=$NGFW_AGENT_SOCKET NGFW_AGENT_OWNER=$NGFW_TEST_PREFIX NGFW_SECRET_KEY_FILE=$RUN/secret.key \
    NGFW_COOKIE_SECURE=0 NGFW_AGENT_TIMEOUT_MS=15000 \
      exec nohup node "$REPO/apps/api/dist/main.js" > "$LOGDIR/api.log" 2>&1
  ) &
  echo $! > "$RUN/api.pid"
  for _ in $(seq 1 150); do curl -fsS "http://127.0.0.1:$NGFW_HTTP_PORT/api/v1/health" >/dev/null 2>&1 && break; sleep 0.2; done
  curl -fsS "http://127.0.0.1:$NGFW_HTTP_PORT/api/v1/health" >/dev/null || { echo "API did not come up (log: $LOGDIR/api.log)" >&2; exit 1; }
  echo "api    pid $(cat "$RUN/api.pid") http://127.0.0.1:$NGFW_HTTP_PORT (admin password in $RUN/admin.pw)"
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
  rm -rf "$RUN/agent-state" "$NGFW_AGENT_SOCKET"
  if [[ ${1:-} != --keep ]]; then
    rm -f "$RUN/admin.pw" "$RUN/jwt.key" "$RUN/secret.key" "$LOGDIR/api.log" "$LOGDIR/agent.log"
    echo "removed slot secrets and logs from $RUN (use stop --keep to keep them)"
  fi
}

case ${1:-} in
  start) shift; start "$@" ;;
  stop) shift; stop "$@" ;;
  *) echo "usage: $0 start [--no-agent] | stop [--keep]" >&2; exit 2 ;;
esac
