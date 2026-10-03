#!/usr/bin/env bash
# test/topology/isis-rip/api400.sh — F-isis-rip host evidence: a level-1 IS with a level-2 circuit answers HTTP 400
# problem+json with a pointer THROUGH THE API (RV-A R7 owed list; S-rva-agent-gates review #11: the semantic rule
# routing.isis.l1-l2-circuit, pointer /routing/isis/interfaces/<if>/circuitType). Adapted from test/topology/ospf/api400.sh.
#
#   eval "$(tools/lab env <slot>)"; test/topology/isis-rip/api400.sh
#
# Slot stack only (slot DB via deploy/dev/pg-test.sh, slot agent owner w<N> with FRR pathspace w<N>, API on the slot port,
# all under /run/ngfw-test/w<N>/isis-api); validate/commit stop at the failing tier, nothing reaches VPP, the candidate is
# discarded. Builds apps/agent/bin/ngfw-agent and apps/api/dist under tools/heavy.sh first (removed by the row at the end).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${NGFW_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
[[ "$NGFW_TEST_PREFIX" =~ ^w([0-9]{1,2})$ ]] || { echo "api400.sh: NGFW_TEST_PREFIX must be w<N>" >&2; exit 1; }
P=$NGFW_TEST_PREFIX; N=${BASH_REMATCH[1]}; OWNER=$P; API_PORT=${NGFW_HTTP_PORT:?}; BASE=${NGFW_VPP_TABLE_BASE:?}
RUN=/run/ngfw-test/$P/isis-api
EVID=$ROOT/docs/status/tasks/F-isis-rip-host-2026-10-03-evidence
LOG=$EVID/api-400.txt
ITF=host-${P}l0
PIDS=()
[[ ${NGFW_DISPOSABLE_VPP:-} == 1 ]] || { echo "requires disposable VPP" >&2; exit 2; }

if [[ "${1:-}" != "--locked" ]]; then
  ( cd "$ROOT/apps/agent" && "$ROOT/tools/heavy.sh" go build -o bin/ngfw-agent ./cmd/ngfw-agent )
  ( cd "$ROOT/apps/api" && "$ROOT/tools/heavy.sh" pnpm build >/dev/null )
  exec "$ROOT/tools/lab" lock shared "$0" --locked
fi

say() { printf '%s %s\n' "$(date +%T)" "$*" | tee -a "$LOG"; }
api() { local m=$1 p=$2 b=${3:-}; curl -s -X "$m" -H "authorization: Bearer $TOKEN" ${b:+-H 'content-type: application/merge-patch+json' -d "$b"} "http://127.0.0.1:$API_PORT$p"; }
apicode() { local m=$1 p=$2 b=${3:-}; curl -s -o "$RUN/last.json" -w '%{http_code} %{content_type}' -X "$m" -H "authorization: Bearer $TOKEN" ${b:+-H 'content-type: application/merge-patch+json' -d "$b"} "http://127.0.0.1:$API_PORT$p"; }
nrestarts() { systemctl show vpp -p NRestarts; }

cleanup() {
  set +e
  say "=== cleanup ==="
  [[ -n "${TOKEN:-}" ]] && api POST /api/v1/config/discard >/dev/null && say "candidate discarded"
  for pid in "${PIDS[@]}"; do kill "$pid" 2>/dev/null; done
  for pid in "${PIDS[@]}"; do for _ in $(seq 50); do kill -0 "$pid" 2>/dev/null || break; sleep 0.1; done; kill -9 "$pid" 2>/dev/null; wait "$pid" 2>/dev/null; done
  say "stopped pids: ${PIDS[*]:-none}"
  "$ROOT/deploy/dev/pg-test.sh" drop "$OWNER" >/dev/null 2>&1 && say "database ngfw_$OWNER dropped"
  valkey-cli -n "$N" EVAL 'local c="0" repeat local r=redis.call("SCAN",c,"MATCH",ARGV[1].."*","COUNT",100) c=r[1] for _,k in ipairs(r[2]) do redis.call("DEL",k) end until c=="0" return 0' 0 "ngfw:$OWNER:isis:" >/dev/null
  rm -rf "$RUN"
  say "VPP $(nrestarts) after"
}
trap cleanup EXIT

install -d -m 0755 "$EVID"; : > "$LOG"
rm -rf "$RUN"; install -d -m 0755 "$RUN" "$RUN/agent-state"
say "=== F-isis-rip-host API 400 evidence: slot $N, owner $OWNER, API :$API_PORT, agent socket $RUN/agent.sock ==="
say "VPP $(nrestarts) before"

# licence: routing.isis is licence-gated (feature "isis", F-licensing; without it validate answers 403 license-required at
# /routing/isis before tier 2). A slot-local Ed25519 key pair outside the checkout signs a 2-day test licence bound to the
# serial override; the API trusts it through NGFW_LICENSE_PUBKEY_FILE (development only) — same recipe as test/topology/vrrp/host.sh.
LIC=$RUN/licence-keys
[[ -r $LIC/ngfw-license-signing.pem ]] || ( cd /tmp && "$ROOT/tools/license/ngfw-license" keygen --out-dir "$LIC" >/dev/null )
"$ROOT/tools/license/ngfw-license" issue --key "$LIC/ngfw-license-signing.pem" --customer "F-isis-rip-host slot $N" \
  --id "LIC-$P-isis" --days 2 --serial "$P-isis-host" --features isis --out "$RUN/license.ngfwlic" >/dev/null
"$ROOT/deploy/dev/pg-test.sh" create "$OWNER" >/dev/null
DSN=$(sed -n 's/^NGFW_PG_DSN=//p' "/run/ngfw-test/$OWNER/pg.env")
( exec env -i PATH="$PATH" HOME="$HOME" NGFW_OWNER="$OWNER" NGFW_GLOBALS_OWNER=0 NGFW_AGENT_SOCKET="$RUN/agent.sock" \
    NGFW_AGENT_STATE_DIR="$RUN/agent-state" NGFW_METRICS_ADDR=off NGFW_SOCKET_GROUP=root NGFW_LOG_LEVEL=info \
    NGFW_VPP_TABLE_BASE="$BASE" NGFW_TEST_PREFIX="$P" NGFW_FRR_PATHSPACE="$P" \
    "${NGFW_ISISRIP_API_AGENT_BIN:-$ROOT/apps/agent/bin/ngfw-agent}" ) >> "$RUN/agent.log" 2>&1 8>&- 9>&- &
AGENT=$!; PIDS+=("$AGENT")
ADMIN_PW=$(head -c 18 /dev/urandom | base64 | tr -dc 'A-Za-z0-9'); ( umask 077; printf '%s' "$ADMIN_PW" > "$RUN/admin.pw" )
( exec env -i PATH="$PATH" HOME="$HOME" NODE_ENV=production NGFW_HTTP_PORT="$API_PORT" NGFW_HTTP_HOST=127.0.0.1 NGFW_PG_DSN="$DSN" \
    NGFW_VALKEY_DB="$N" NGFW_VALKEY_PREFIX="ngfw:$OWNER:isis:" NGFW_AGENT_SOCKET="$RUN/agent.sock" NGFW_AGENT_OWNER="$OWNER" \
    NGFW_AGENT_TIMEOUT_MS=20000 NGFW_JWT_SECRET="$(head -c 48 /dev/urandom | base64 | tr -dc 'A-Za-z0-9')" \
    NGFW_SECRET_KEY_FILE="$RUN/secret.key" NGFW_BOOTSTRAP_ADMIN_PASSWORD="$ADMIN_PW" NGFW_COOKIE_SECURE=0 NGFW_LOG_LEVEL=warn \
    NGFW_LICENSE_FILE="$RUN/license.ngfwlic" NGFW_LICENSE_PUBKEY_FILE="$LIC/ngfw-license-public.pem" NGFW_LICENSE_SERIAL="$P-isis-host" \
    node "$ROOT/apps/api/dist/main.js" ) >> "$RUN/api.log" 2>&1 8>&- 9>&- &
PIDS+=("$!")
for _ in $(seq 120); do curl -sf "http://127.0.0.1:$API_PORT/api/v1/health" >/dev/null && break; sleep 0.5; done
TOKEN=$(curl -sf -H 'content-type: application/json' -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PW\"}" "http://127.0.0.1:$API_PORT/api/v1/auth/login" | jq -r .accessToken)
[[ -n "$TOKEN" && "$TOKEN" != null ]] || { say "login failed: $(tail -3 "$RUN/api.log")"; exit 1; }
say "agent pid $AGENT (NGFW_FRR_PATHSPACE=$P NGFW_VPP_TABLE_BASE=$BASE), API pid ${PIDS[1]} health $(curl -s "http://127.0.0.1:$API_PORT/api/v1/health" | jq -c '{status}')"
say "agent log: $(grep -E 'VPP binary API|connect round' "$RUN/agent.log" | tail -1 | cut -c1-200)"

say "=== L1 IS with an L2 circuit: isis.level = level-1, interfaces.$ITF.circuitType = level-2 ==="
api PATCH /api/v1/config/interfaces "{\"$ITF\":{\"enabled\":true,\"ipv4\":[\"10.$N.1.1/24\"],\"lcp\":{\"hostIfName\":\"$P-l0\",\"hostIfType\":\"tap\"}}}" >/dev/null
say "PATCH /config/routing (isis: net 49.000$N.0000.0000.0001.00, level level-1, interfaces {$ITF: circuitType level-2}) → HTTP $(apicode PATCH /api/v1/config/routing "{\"isis\":{\"net\":\"49.000$N.0000.0000.0001.00\",\"level\":\"level-1\",\"interfaces\":{\"$ITF\":{\"circuitType\":\"level-2\"}}}}")"
say "body: $(jq -c . "$RUN/last.json" | cut -c1-400)"
code=$(apicode POST /api/v1/config/validate)
say "POST /config/validate → HTTP $code"
[[ "$code" == 400* ]] || exit 1
jq -e --arg pointer "/routing/isis/interfaces/$ITF/circuitType" '.status==400 and any(.errors[]; .pointer==$pointer)' "$RUN/last.json" >/dev/null
say "body: $(jq -c '{type,title,status,detail,errors:(.errors//[]|map({pointer,message}))}' "$RUN/last.json" | cut -c1-700)"
code=$(apicode POST '/api/v1/config/commit?comment=isis-host-l1-l2')
say "POST /config/commit → HTTP $code"
[[ "$code" == 400* ]] || exit 1
jq -e --arg pointer "/routing/isis/interfaces/$ITF/circuitType" '.status==400 and any(.errors[]; .pointer==$pointer)' "$RUN/last.json" >/dev/null
say "body: $(jq -c '{type,title,status,detail,errors:(.errors//[]|map({pointer,message}))}' "$RUN/last.json" | cut -c1-700)"
say "running config after the refused commit: routing.isis = $(api GET /api/v1/config | jq -c '.routing.isis // null')"
say "=== control: the same IS at level-1-2 → validate ==="
say "PATCH level level-1-2 → HTTP $(apicode PATCH /api/v1/config/routing '{"isis":{"level":"level-1-2"}}')"
code=$(apicode POST /api/v1/config/validate)
say "POST /config/validate → HTTP $code"
[[ "$code" == 200* ]]
say "body: $(jq -c '{status,valid,errors:(.errors//[]|map({pointer,message})),warnings:(.warnings//[]|map({pointer,code}))}' "$RUN/last.json" | cut -c1-700)"
