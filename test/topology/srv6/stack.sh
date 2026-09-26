#!/usr/bin/env bash
# test/topology/srv6/stack.sh — F-srv6 full-stack evidence run on one slot (host VPP, real processes): the vrx-agent
#   (VRX_GLOBALS_OWNER=0, D-071), vrx-api on a throwaway database and, optionally, the production web build under
#   `vite preview` for the screenshots. Steps: baseline commit (VRF + two loopbacks) → routing.srv6 commit through the API
#   → GET /state/srv6 + `vppctl show sr …` + FIB evidence → drift → validation failure (encap policy without a source →
#   400 problem+json with a pointer) → optional screenshots → agent restart without loss (claimed objects not re-added:
#   agent log) → rollback to the baseline (steering → policies → SIDs removed, no SR route left) → cleanup.
#   Loss of SR objects behind the agent's back (binapi) is the Go host test's part:
#   apps/agent/internal/agent/srv6_integration_test.go (TestSrv6OnHost).
#
#   eval "$(tools/lab env <slot>)"; test/topology/srv6/stack.sh [<screenshot node script> <out dir>]
#
# Owner "<prefix>sr", database vrx_<prefix>sr, API port 3000+100·slot+60, web 5000+100·slot+60, agent socket
# /run/vrx-test/<prefix>/sr/agent.sock, VRF <owner>-cust table base+60, loopbacks loop<slot>60/61, SIDs/BSIDs in
# fd00:<slot hex>::/48, steered prefixes 10.<slot>.160.0/24 and fd00:<slot hex>:160::/48. No packet is sent (no rig, no
# af_packet). The screenshot script (kept outside the repo, P07a/P07b) is called as
# `node <script> <webUrl> <outDir> <adminPasswordFile>`. Output: $RUN/evidence.log. Every process is stopped by PID.
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
: "${VRX_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
: "${VRX_SLOT:?eval \"\$(tools/lab env <slot>)\" first}"
P=$VRX_TEST_PREFIX N=$VRX_SLOT OWNER=${VRX_TEST_PREFIX}sr
[[ "$N" =~ ^([1-9]|1[01])$ ]] || { echo "stack.sh: slots 1–11 only (12 is CI)" >&2; exit 1; }
BASE=$((N * 1000)) TABLE=$((N * 1000 + 60)) L1=loop$((N * 100 + 60)) L2=loop$((N * 100 + 61))
H=$(printf 'fd00:%x' "$N") VRF=$OWNER-cust
API_PORT=$((3000 + 100 * N + 60)) WEB_PORT=$((5000 + 100 * N + 60))
RUN=/run/vrx-test/$P/sr
SHOTS=${1:-} SHOTS_OUT=${2:-}
[[ -d "/run/vrx-test/$P" ]] || install -d -m 0755 "/run/vrx-test/$P"
rm -rf "$RUN"; install -d -m 0700 "$RUN"
LOG=$RUN/evidence.log
say() { printf '%s %s\n' "$(date +%T)" "$*" | tee -a "$LOG"; }
show() { echo "--- vppctl $*"; vppctl "$@"; }
slotlines() { grep -E "^(\S|  )|$H:|10\.$N\.160\.|$L1|$L2" || true; }
PIDS=()
cleanup() {
  set +e
  say "cleanup"
  if [[ -n "${TOKEN:-}" ]]; then
    api PATCH /api/v1/config/routing '{"srv6":null}' >/dev/null
    api PATCH /api/v1/config/interfaces "{\"$L1\":null,\"$L2\":null}" >/dev/null
    api PATCH /api/v1/config/vrfs "{\"$VRF\":null}" >/dev/null
    api POST "/api/v1/config/commit?comment=srv6-stack-cleanup" | jq -c '{status,revision:.revision.id}' | tee -a "$LOG"
  fi
  for ((i=${#PIDS[@]}-1; i>=0; i--)); do kill "${PIDS[$i]}" 2>/dev/null; wait "${PIDS[$i]}" 2>/dev/null; done
  "$ROOT/deploy/dev/pg-test.sh" drop "$OWNER" >/dev/null 2>&1
  rm -f "$RUN/admin.pw"
  { show show sr localsids | slotlines; show show sr policies | grep -c "$H:" ; show show sr steering-policies | slotlines; } >> "$LOG" 2>&1
  say "VPP $(systemctl show vpp -p NRestarts) after"
}
trap cleanup EXIT

say "VPP $(systemctl show vpp -p NRestarts) before"
( cd "$ROOT/apps/agent" && go build -o bin/vrx-agent ./cmd/vrx-agent )
( cd "$ROOT" && pnpm --filter @ngfw/api build >/dev/null && { [[ -z "$SHOTS" ]] || pnpm --filter @ngfw/web build >/dev/null; } )

"$ROOT/deploy/dev/pg-test.sh" create "$OWNER" >/dev/null
DSN=$(sed -n 's/^VRX_PG_DSN=//p' "/run/vrx-test/$OWNER/pg.env")
start_agent() {
  env -i PATH="$PATH" HOME="$HOME" VRX_OWNER="$OWNER" VRX_GLOBALS_OWNER=0 VRX_AGENT_SOCKET="$RUN/agent.sock" \
    VRX_AGENT_STATE_DIR="$RUN/agent-state" VRX_METRICS_ADDR=off VRX_SOCKET_GROUP=root VRX_LOG_LEVEL=info \
    VRX_VPP_TABLE_BASE="$BASE" "$ROOT/apps/agent/bin/vrx-agent" >> "$RUN/agent.log" 2>&1 & AGENT=$!; PIDS+=("$AGENT")
}
start_agent
ADMIN_PW=$(head -c 18 /dev/urandom | base64 | tr -dc 'A-Za-z0-9')
( umask 077; printf '%s' "$ADMIN_PW" > "$RUN/admin.pw" )
env -i PATH="$PATH" HOME="$HOME" NODE_ENV=production VRX_HTTP_PORT="$API_PORT" VRX_HTTP_HOST=127.0.0.1 VRX_PG_DSN="$DSN" \
  VRX_VALKEY_DB="$N" VRX_VALKEY_PREFIX="vrx:$OWNER:stack:" VRX_AGENT_SOCKET="$RUN/agent.sock" VRX_AGENT_OWNER="$OWNER" \
  VRX_AGENT_TIMEOUT_MS=60000 VRX_JWT_SECRET="$(head -c 48 /dev/urandom | base64 | tr -dc 'A-Za-z0-9')" \
  VRX_SECRET_KEY_FILE="$RUN/secret.key" VRX_BOOTSTRAP_ADMIN_PASSWORD="$ADMIN_PW" VRX_COOKIE_SECURE=0 VRX_LOG_LEVEL=warn \
  node "$ROOT/apps/api/dist/main.js" >> "$RUN/api.log" 2>&1 & PIDS+=("$!")
for _ in $(seq 120); do curl -sf "http://127.0.0.1:$API_PORT/api/v1/health" >/dev/null && break; sleep 0.5; done
TOKEN=$(curl -sf -H 'content-type: application/json' -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PW\"}" "http://127.0.0.1:$API_PORT/api/v1/auth/login" | jq -r .accessToken)
api() { local m=$1 p=$2 b=${3:-}; curl -s -X "$m" -H "authorization: Bearer $TOKEN" ${b:+-H 'content-type: application/merge-patch+json' -d "$b"} "http://127.0.0.1:$API_PORT$p"; }
apij() { local m=$1 p=$2; curl -s -X "$m" -H "authorization: Bearer $TOKEN" "http://127.0.0.1:$API_PORT$p"; }
say "agent pid $AGENT, API :$API_PORT up; health: $(apij GET /api/v1/health | jq -c '{status}')"

# baseline: the customer VRF and the two loopbacks (no SRv6)
api PATCH /api/v1/config/vrfs "{\"$VRF\":{\"id\":$TABLE}}" >/dev/null
api PATCH /api/v1/config/interfaces "{\"$L1\":{\"enabled\":true,\"ipv6\":[\"$H:1::1/64\"]},\"$L2\":{\"enabled\":true}}" >/dev/null
say "baseline commit: $(api POST "/api/v1/config/commit?comment=srv6-stack-base" | jq -c '{status,revision:.revision.id}')"

# routing.srv6: L3VPN-style (end.dt4/end.dt6 into the customer VRF, an encap policy steering its prefixes), plus end,
# end.x, end.dx2 and an L2 steering entry (VPWS pair on $L2)
SRV6=$(cat <<EOF
{"srv6":{
  "localSids":{
    "$H:ff::1":{"behavior":"end","psp":true},
    "$H:ff::2":{"behavior":"end.x","interface":"$L1","nextHop":"$H:1::2"},
    "$H:ff::a":{"behavior":"end.dt4","lookupVrf":"$VRF"},
    "$H:ff::b":{"behavior":"end.dt6","vrf":"$VRF","lookupVrf":"$VRF"},
    "$H:ff::c":{"behavior":"end.dx2","interface":"$L2"}},
  "policies":{
    "$H:bb::1":{"encapSource":"$H::1","sidLists":[{"sids":["$H:ee::1","$H:ee::a"],"weight":1},{"sids":["$H:ee::2","$H:ee::a"],"weight":3}]},
    "$H:bb::2":{"type":"spray","encap":false,"vrf":"$VRF","sidLists":[{"sids":["$H:ee::3"]}]}},
  "steering":[
    {"type":"l3","prefix":"10.$N.160.0/24","vrf":"$VRF","bsid":"$H:bb::1"},
    {"type":"l3","prefix":"$H:160::/48","vrf":"$VRF","bsid":"$H:bb::2"},
    {"type":"l2","interface":"$L2","bsid":"$H:bb::1"}]}}
EOF
)
api PATCH /api/v1/config/routing "$SRV6" >/dev/null
say "candidate diff: $(apij GET /api/v1/config/diff | jq -c '[.changes[]?|{op,pointer}]' | cut -c1-400)"
say "commit: $(api POST "/api/v1/config/commit?comment=srv6-stack" | jq -c '{status,revision:.revision.id,errors}')"
say "GET /api/v1/state/srv6: $(apij GET /api/v1/state/srv6 | jq -c .)"
{ show show sr localsids | slotlines
  show show sr policies
  show show sr steering-policies | slotlines
  show show ip6 fib table "$TABLE"
  show show ip fib table "$TABLE"
  show show ip6 fib "$H:bb::1/128"; } >> "$LOG" 2>&1
say "drift (/routing): $(apij GET /api/v1/state/drift | jq -c '{changes:[.changes[]|select(.pointer|startswith("/routing"))],ignored:[.ignored[]|select(.pointer|startswith("/routing"))]}')"

# validation failure: an encapsulating policy without any source (D-074) → 400 problem+json with a pointer
api PATCH /api/v1/config/routing "{\"srv6\":{\"policies\":{\"$H:bb::9\":{\"sidLists\":[{\"sids\":[\"$H:ee::9\"]}]}}}}" >/dev/null
api PATCH /api/v1/config/routing '{"srv6":{"encapSource":null}}' >/dev/null
say "encap policy without source: $(curl -s -o "$RUN/bad.json" -w '%{http_code}' -X POST -H "authorization: Bearer $TOKEN" "http://127.0.0.1:$API_PORT/api/v1/config/commit") $(jq -c . "$RUN/bad.json" | cut -c1-600)"
api POST /api/v1/config/discard >/dev/null

if [[ -n "$SHOTS" ]]; then
  env VRX_HTTP_PORT="$API_PORT" VRX_WEB_PORT="$WEB_PORT" "$ROOT/apps/web/node_modules/.bin/vite" preview "$ROOT/apps/web" >> "$RUN/vite.log" 2>&1 & PIDS+=("$!")
  for _ in $(seq 60); do curl -sf -o /dev/null "http://127.0.0.1:$WEB_PORT/" && break; sleep 0.5; done
  say "screenshots: $(node "$SHOTS" "http://127.0.0.1:$WEB_PORT" "$SHOTS_OUT" "$RUN/admin.pw" 2>&1 | tr '\n' ' ')"
fi

# restart without loss: the claims are persisted (claims-df6-<owner>.json), the resync re-adds nothing
kill "$AGENT"; wait "$AGENT" 2>/dev/null || true
MARK=$(wc -c < "$RUN/agent.log")
T0=$(date +%s.%N)
start_agent
for _ in $(seq 60); do [[ $(apij GET /api/v1/state/srv6 | jq '.localSids|length? // 0' 2>/dev/null) == 5 ]] && break; sleep 0.5; done
say "restart: $(apij GET /api/v1/state/srv6 | jq -c '{localSids:(.localSids|length),policies:(.policies|length),steering:(.steering|length)}') after $(python3 -c "import time;print(round(time.time()-$T0,2))") s (agent pid $AGENT)"
tail -c +"$((MARK + 1))" "$RUN/agent.log" | grep -E 'reconcile (start|done)|subsystems wired|sr\.' | cut -c1-400 >> "$LOG"
say "drift after restart (/routing): $(apij GET /api/v1/state/drift | jq -c '[.changes[]|select(.pointer|startswith("/routing"))]')"

# rollback to the baseline revision (steering → policies → SIDs removed by the scheduler's order)
REV=$(apij GET "/api/v1/config/revisions?limit=10" | jq -r '[.items[]|select(.comment=="srv6-stack-base")][0].id')
say "rollback to $REV (srv6-stack-base): $(api POST "/api/v1/config/rollback/$REV" | jq -c '{status,revision:{id:.revision.id,kind:.revision.kind},results:[.results[]?|select(.key|startswith("sr."))|.key]}' | cut -c1-900)"
say "after rollback: state $(apij GET /api/v1/state/srv6 | jq -c '{localSids,policies,steering}')"
{ echo "--- after rollback"; show show sr localsids | slotlines; show show sr steering-policies | slotlines
  show show ip6 fib table "$TABLE"; show show ip fib table "$TABLE"; } >> "$LOG" 2>&1
say "done"
