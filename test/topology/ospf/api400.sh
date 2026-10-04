#!/usr/bin/env bash
# test/topology/ospf/api400.sh — F-ospf host evidence: the two OSPF validation failures answer HTTP 400 problem+json with a
# pointer THROUGH THE API (RV-A R7 owed list: "undefined area / area 0 stub → 400 with pointer through the API").
#
#   eval "$(tools/lab env <slot>)"; test/topology/ospf/api400.sh
#
# A slot stack only: the slot database (deploy/dev/pg-test.sh), the slot agent (owner w<N>, table base of the slot, FRR
# pathspace w<N> so nothing of the system frr unit is touched) and the API on the slot's HTTP port, all under
# /run/ngfw-test/w<N>/ospf-api. Nothing is committed to VPP: `POST /config/validate` stops at the failing tier
# (schema → semantic → agent DryRun), and the candidate is discarded at the end.
#   1. undefined area   → tier 2 (packages/schema semantic rule routing.ospf-area-exists), pointer
#                         /routing/ospf/interfaces/<itf>/area — no agent round trip
#   2. area 0 stub      → tier 3 (the agent's ospf renderer refuses stub/nssa for the backbone), pointer
#                         /routing/ospf/areas/0/type — needs the agent's DryRun, which needs the VPP binary API
# Held: the shared lab lock for the run (the agent dials /run/vpp/api.sock). Every background child closes the lock fds
# (8>&- 9>&-). Kills only the PIDs it spawned. Leaves the database dropped, the run directory removed.
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${NGFW_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
[[ "$NGFW_TEST_PREFIX" =~ ^w([0-9]{1,2})$ ]] || { echo "api400.sh: NGFW_TEST_PREFIX must be w<N>" >&2; exit 1; }
P=$NGFW_TEST_PREFIX; N=${BASH_REMATCH[1]}; OWNER=$P; API_PORT=${NGFW_HTTP_PORT:?}; BASE=${NGFW_VPP_TABLE_BASE:?}
RUN=/run/ngfw-test/$P/ospf-api
EVID=${NGFW_OSPF_EVIDENCE:-$ROOT/docs/status/tasks/F-ospf-host-evidence}
LOG=$EVID/api-400.txt
ITF=host-${P}l0
PIDS=()

if [[ "${1:-}" != "--locked" ]]; then
  exec "$ROOT/tools/lab" lock shared "$0" --locked
fi

say() { printf '%s %s\n' "$(date +%T)" "$*" | tee -a "$LOG"; }
api() { local m=$1 p=$2 b=${3:-}; curl -s -X "$m" -H "authorization: Bearer $TOKEN" ${b:+-H 'content-type: application/merge-patch+json' -d "$b"} "http://127.0.0.1:$API_PORT$p"; }
apicode() { local m=$1 p=$2 b=${3:-}; curl -s -o "$RUN/last.json" -w '%{http_code} %{content_type}' -X "$m" -H "authorization: Bearer $TOKEN" ${b:+-H 'content-type: application/merge-patch+json' -d "$b"} "http://127.0.0.1:$API_PORT$p"; }
assert_problem() {
  local result
  result=$(apicode "$1" "$2")
  say "$1 $2 → HTTP $result"
  [[ "$result" == 400\ application/problem+json* ]] || { say "FAIL: expected 400 problem+json"; exit 1; }
  jq -e --arg pointer "$3" '.status == 400 and any(.errors[]?; .pointer == $pointer)' "$RUN/last.json" >/dev/null || { say "FAIL: expected error pointer $3"; exit 1; }
}
nrestarts() { systemctl show vpp -p NRestarts; }

cleanup() {
  set +e
  say "=== cleanup ==="
  [[ -n "${TOKEN:-}" ]] && api POST /api/v1/config/discard >/dev/null && say "candidate discarded"
  for pid in "${PIDS[@]}"; do kill "$pid" 2>/dev/null; done
  for pid in "${PIDS[@]}"; do for _ in $(seq 50); do kill -0 "$pid" 2>/dev/null || break; sleep 0.1; done; kill -9 "$pid" 2>/dev/null; wait "$pid" 2>/dev/null; done
  say "stopped pids: ${PIDS[*]:-none}"
  "$ROOT/deploy/dev/pg-test.sh" drop "$OWNER" >/dev/null 2>&1 && say "database ngfw_$OWNER dropped"
  rm -rf "$RUN"
  say "VPP $(nrestarts) after"
}
trap cleanup EXIT

install -d -m 0755 "$EVID"; : > "$LOG"
rm -rf "$RUN"; install -d -m 0755 "$RUN" "$RUN/agent-state"
say "=== F-ospf-host API 400 evidence: slot $N, owner $OWNER, API :$API_PORT, agent socket $RUN/agent.sock ==="
say "VPP $(nrestarts) before"

# licence: routing.ospf is licence-gated (feature "ospf", F-licensing; without it validate answers 403 license-required at
# /routing/ospf before tier 2). A slot-local Ed25519 key pair outside the checkout signs a 2-day test licence bound to the
# serial override; the API trusts it through NGFW_LICENSE_PUBKEY_FILE (development only) — same recipe as test/topology/vrrp/host.sh.
LIC=/tmp/g-$P/lic
[[ -r $LIC/ngfw-license-signing.pem ]] || ( cd /tmp && "$ROOT/tools/license/ngfw-license" keygen --out-dir "$LIC" >/dev/null )
"$ROOT/tools/license/ngfw-license" issue --key "$LIC/ngfw-license-signing.pem" --customer "F-ospf-host slot $N" \
  --id "LIC-$P-ospf" --days 2 --serial "$P-ospf-host" --features ospf --out "$RUN/license.ngfwlic" >/dev/null
"$ROOT/deploy/dev/pg-test.sh" create "$OWNER" >/dev/null
DSN=$(sed -n 's/^NGFW_PG_DSN=//p' "/run/ngfw-test/$OWNER/pg.env")
( exec env -i PATH="$PATH" HOME="$HOME" NGFW_OWNER="$OWNER" NGFW_GLOBALS_OWNER=0 NGFW_AGENT_SOCKET="$RUN/agent.sock" \
    NGFW_AGENT_STATE_DIR="$RUN/agent-state" NGFW_METRICS_ADDR=off NGFW_SOCKET_GROUP=root NGFW_LOG_LEVEL=info \
    NGFW_VPP_TABLE_BASE="$BASE" NGFW_TEST_PREFIX="$P" NGFW_FRR_PATHSPACE="$P" \
    "$ROOT/apps/agent/bin/ngfw-agent" ) >> "$RUN/agent.log" 2>&1 8>&- 9>&- &
AGENT=$!; PIDS+=("$AGENT")
ADMIN_PW=$(head -c 18 /dev/urandom | base64 | tr -dc 'A-Za-z0-9'); ( umask 077; printf '%s' "$ADMIN_PW" > "$RUN/admin.pw" )
( exec env -i PATH="$PATH" HOME="$HOME" NODE_ENV=production NGFW_HTTP_PORT="$API_PORT" NGFW_HTTP_HOST=127.0.0.1 NGFW_PG_DSN="$DSN" \
    NGFW_VALKEY_DB="${NGFW_VALKEY_DB:?}" NGFW_VALKEY_PREFIX="ngfw:$OWNER:ospf:" NGFW_AGENT_SOCKET="$RUN/agent.sock" NGFW_AGENT_OWNER="$OWNER" \
    NGFW_AGENT_TIMEOUT_MS=20000 NGFW_JWT_SECRET="$(head -c 48 /dev/urandom | base64 | tr -dc 'A-Za-z0-9')" \
    NGFW_SECRET_KEY_FILE="$RUN/secret.key" NGFW_BOOTSTRAP_ADMIN_PASSWORD="$ADMIN_PW" NGFW_COOKIE_SECURE=0 NGFW_LOG_LEVEL=warn \
    NGFW_LICENSE_FILE="$RUN/license.ngfwlic" NGFW_LICENSE_PUBKEY_FILE="$LIC/ngfw-license-public.pem" NGFW_LICENSE_SERIAL="$P-ospf-host" \
    node "$ROOT/apps/api/dist/main.js" ) >> "$RUN/api.log" 2>&1 8>&- 9>&- &
PIDS+=("$!")
for _ in $(seq 120); do curl -sf "http://127.0.0.1:$API_PORT/api/v1/health" >/dev/null && break; sleep 0.5; done
TOKEN=$(curl -sf -H 'content-type: application/json' -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PW\"}" "http://127.0.0.1:$API_PORT/api/v1/auth/login" | jq -r .accessToken)
[[ -n "$TOKEN" && "$TOKEN" != null ]] || { say "login failed: $(tail -3 "$RUN/api.log")"; exit 1; }
say "agent pid $AGENT (NGFW_FRR_PATHSPACE=$P NGFW_VPP_TABLE_BASE=$BASE), API pid ${PIDS[1]} health $(curl -s "http://127.0.0.1:$API_PORT/api/v1/health" | jq -c '{status}')"
say "agent log: $(grep -E 'VPP binary API|connect round' "$RUN/agent.log" | tail -1 | cut -c1-200)"

EXPECTED_POINTER="/routing/ospf/interfaces/$ITF/area"
say "=== 1. undefined area: interface $ITF in area 51, areas = {0} → POST /config/validate ==="
api PATCH /api/v1/config/interfaces "{\"$ITF\":{\"enabled\":true,\"ipv4\":[\"10.$N.1.1/24\"]}}" >/dev/null
say "PATCH /config/routing (ospf: routerId 10.$N.1.1, areas {0}, interfaces {$ITF: area 51}) → HTTP $(apicode PATCH /api/v1/config/routing "{\"ospf\":{\"routerId\":\"10.$N.1.1\",\"areas\":{\"0\":{}},\"interfaces\":{\"$ITF\":{\"area\":\"51\"}}}}")"
assert_problem POST /api/v1/config/validate "$EXPECTED_POINTER"
say "body: $(jq -c '{type,title,status,errors:(.errors//[]|map({pointer,message}))}' "$RUN/last.json")"
assert_problem POST '/api/v1/config/commit?comment=ospf-host-undefined-area' "$EXPECTED_POINTER"
say "body: $(jq -c '{type,title,status,errors:(.errors//[]|map({pointer,message}))}' "$RUN/last.json")"
say "running config after the refused commit: routing.ospf = $(api GET /api/v1/config | jq -c '.routing.ospf // null')"

[[ "$(api GET /api/v1/config | jq -c ' .routing.ospf // null')" == null ]] || { say "FAIL: refused commit changed running OSPF"; exit 1; }
EXPECTED_POINTER=/routing/ospf/areas/0/type
say "=== 2. backbone stub: areas = {0: stub}, interface $ITF in area 0 → POST /config/validate ==="
say "PATCH /config/routing (ospf.areas.0.type = stub, interfaces {$ITF: area 0}) → HTTP $(apicode PATCH /api/v1/config/routing "{\"ospf\":{\"areas\":{\"0\":{\"type\":\"stub\"}},\"interfaces\":{\"$ITF\":{\"area\":\"0\"}}}}")"
assert_problem POST /api/v1/config/validate "$EXPECTED_POINTER"
say "body: $(jq -c '{type,title,status,detail,errors:(.errors//[]|map({pointer,message}))}' "$RUN/last.json" | cut -c1-600)"
say "agent log tail: $(tail -1 "$RUN/agent.log" | cut -c1-200)"
