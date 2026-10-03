#!/usr/bin/env bash
# test/topology/tunnels/run.sh — isolated S-tunnels-contract baseline (real feature output
# against the slot stack). Slot stack on the host VPP: this branch's agent + API, PostgreSQL ngfw_<p>, prefix <p>.
#
#   eval "$(tools/lab env <slot>)"; test/topology/tunnels/run.sh run [evidence-dir]
#
# Shared lab lock only (no VPP-global is touched). Everything carries the slot prefix; processes are killed by PID; the
# document is rolled back and the database dropped at the end. Never restarts VPP.
#
# Steps: 0 NRestarts + plugins · 1 stack up · 2 GRE tunnel without instance → 400 tunnels.instance-required (pointer) ·
#        3 commit gre + vxlan-gpe tunnels → show gre|vxlan-gpe tunnel, GET /state/tunnels, drift · 4 rollback → nothing left.
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
: "${NGFW_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
: "${NGFW_SLOT:?eval \"\$(tools/lab env <slot>)\" first}"
CMD=${1:-run}; EVID=${2:-$HERE}
if [[ "$CMD" == --dry-run ]]; then
  echo "isolated API GRE/GPE baseline; all: GRE/IPIP/VXLAN, duplicate refusal, loss/restart, historical rollback"
  exit 0
fi
[[ ${NGFW_DISPOSABLE_VPP:-} == 1 ]] || { echo "requires disposable VPP" >&2; exit 2; }
P=$NGFW_TEST_PREFIX N=$NGFW_SLOT OWNER=$NGFW_TEST_PREFIX BASE=$NGFW_VPP_TABLE_BASE API_PORT=$NGFW_HTTP_PORT
LOOP=loop${N}070 SRC=10.$N.7.1
RUN=/run/ngfw-test/$P/tunnels-t1
mkdir -p "$EVID" "$RUN"
LOG=$RUN/evidence.log
: > "$LOG"
say() { printf '%s %s\n' "$(date +%T)" "$*" | tee -a "$LOG"; }
nres() { say "NRestarts $1: $(systemctl show vpp -p NRestarts)"; }
V() { timeout 10 vppctl "$@"; }
PIDS=()
api() { local m=$1 p=$2 b=${3:-}; curl --fail-with-body --max-time 15 -s -X "$m" -H "authorization: Bearer $TOKEN" ${b:+-H 'content-type: application/merge-patch+json' -d "$b"} "http://127.0.0.1:$API_PORT$p"; }
apij() { local m=$1 p=$2; curl --fail-with-body --max-time 15 -s -X "$m" -H "authorization: Bearer $TOKEN" "http://127.0.0.1:$API_PORT$p"; }
commit() {
  local expected=${2:-applied} result; result=$(api POST "/api/v1/config/commit?comment=$1")
  if [[ "$expected" == noop ]]; then
    if ! jq -e '(.status=="applied" or .status=="unchanged") and (.results|type=="array" and length==0)' <<<"$result" >/dev/null; then
      say "unchanged commit failed: $result" >&2
      return 1
    fi
  elif [[ $(jq -r '.status' <<<"$result") != "$expected" ]]; then
    say "commit refused: $result" >&2
    return 1
  fi
  jq -c '{status,revision:.revision.id}' <<<"$result"
}

start_agent() {
  ( exec env -i PATH="$PATH" HOME="$HOME" NGFW_OWNER="$OWNER" NGFW_GLOBALS_OWNER=0 NGFW_AGENT_SOCKET="$RUN/agent.sock" \
      NGFW_AGENT_STATE_DIR="$RUN/agent-state" NGFW_METRICS_ADDR=off NGFW_SOCKET_GROUP=root NGFW_LOG_LEVEL=info \
      NGFW_VPP_TABLE_BASE="$BASE" NGFW_TEST_PREFIX="$P" "${NGFW_TUNNELS_AGENT_BIN:-$ROOT/apps/agent/bin/ngfw-agent}" ) >> "$RUN/agent.log" 2>&1 9>&- & AGENT=$!; PIDS+=("$AGENT")
}


stop_owned() {
  local pid=$1 attempt
  kill "$pid" 2>/dev/null || true
  for attempt in $(seq 100); do
    if ! kill -0 "$pid" 2>/dev/null || [[ $(awk '{print $3}' "/proc/$pid/stat" 2>/dev/null) == Z ]]; then
      wait "$pid" 2>/dev/null || true
      return 0
    fi
    sleep 0.1
  done
  say "owned process $pid failed graceful shutdown; forcing bounded cleanup"
  kill -KILL "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
  return 1
}

cleanup() {
  local original=$? cleanup_failed=0 cleaned
  set +e
  say "=== cleanup ==="
  if [[ -n "${TOKEN:-}" ]]; then
    api POST /api/v1/config/discard >/dev/null || cleanup_failed=1
    api PATCH /api/v1/config/tunnels "{\"gre\":{\"$P-gre\":null},\"ipip\":{\"$P-ipip\":null},\"vxlan\":{\"$P-vxlan\":null,\"$P-duplicate\":null},\"vxlanGpe\":{\"$P-gpe\":null}}" >/dev/null || cleanup_failed=1
    api PATCH /api/v1/config/interfaces "{\"$LOOP\":null}" >/dev/null || cleanup_failed=1
    if cleaned=$(commit t1-cleanup); then say "cleanup commit: $cleaned"; else cleanup_failed=1; fi
  fi
  for ((i=${#PIDS[@]}-1; i>=0; i--)); do stop_owned "${PIDS[$i]}" || cleanup_failed=1; done
  sleep 0.5
  "$ROOT/deploy/dev/pg-test.sh" drop "$OWNER" >/dev/null 2>&1 || cleanup_failed=1
  rm -f "$RUN/admin.pw" "$RUN/secret.key"
  valkey-cli -n "$N" EVAL 'local c="0" repeat local r=redis.call("SCAN",c,"MATCH",ARGV[1].."*","COUNT",100) c=r[1] for _,k in ipairs(r[2]) do redis.call("DEL",k) end until c=="0" return 0' 0 "ngfw:$OWNER:t1:" >/dev/null || cleanup_failed=1
  say "leftovers in VPP: $(V show interface | grep -cE "^($LOOP|gre${BASE}|vxlan_gpe_tunnel[0-9]+) ") interface(s) named like ours; show gre tunnel: $(V show gre tunnel | grep -c "$SRC") · show vxlan-gpe tunnel: $(V show vxlan-gpe tunnel | grep -c "$SRC")"
  nres "after cleanup"
  say "window end $(date -Is)"
  say "cleanup status $cleanup_failed; original status $original"
  cp "$LOG" "$EVID/evidence.txt"; cp "$RUN/agent.log" "$EVID/agent-log.txt" 2>/dev/null; cp "$RUN/api.log" "$EVID/api-log.txt" 2>/dev/null
  if ((original!=0)); then exit "$original"; fi
  exit "$cleanup_failed"
}

step0() {
  say "=== 0 host facts ==="
  nres "before"
  say "plugins: $(V show plugins | grep -oE 'gre_plugin\.so|vxlan_gpe_plugin\.so|gtpu_plugin\.so|l2tp_plugin\.so|pppoe_plugin\.so' | tr '\n' ' ')"
  say "branch $(git -C "$ROOT" rev-parse --short HEAD) slot $N prefix $P table base $BASE"
}

step1() {
  say "=== 1 slot stack ==="
  "$ROOT/deploy/dev/pg-test.sh" create "$OWNER" >/dev/null
  DSN=$(sed -n 's/^NGFW_PG_DSN=//p' "/run/ngfw-test/$OWNER/pg.env")
  start_agent
  ADMIN_PW=$(head -c 18 /dev/urandom | base64 | tr -dc 'A-Za-z0-9'); ( umask 077; printf '%s' "$ADMIN_PW" > "$RUN/admin.pw" )
  env -i PATH="$PATH" HOME="$HOME" NODE_ENV=production NGFW_HTTP_PORT="$API_PORT" NGFW_HTTP_HOST=127.0.0.1 NGFW_PG_DSN="$DSN" \
    NGFW_VALKEY_DB="$N" NGFW_VALKEY_PREFIX="ngfw:$OWNER:t1:" NGFW_AGENT_SOCKET="$RUN/agent.sock" NGFW_AGENT_OWNER="$OWNER" \
    NGFW_AGENT_TIMEOUT_MS=60000 NGFW_JWT_SECRET="$(head -c 48 /dev/urandom | base64 | tr -dc 'A-Za-z0-9')" \
    NGFW_SECRET_KEY_FILE="$RUN/secret.key" NGFW_BOOTSTRAP_ADMIN_PASSWORD="$ADMIN_PW" NGFW_COOKIE_SECURE=0 NGFW_LOG_LEVEL=warn \
    node "$ROOT/apps/api/dist/main.js" >> "$RUN/api.log" 2>&1 9>&- & PIDS+=("$!")
  for _ in $(seq 120); do curl -sf "http://127.0.0.1:$API_PORT/api/v1/health" >/dev/null && break; sleep 0.5; done
  TOKEN=$(curl -sf -H 'content-type: application/json' -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PW\"}" "http://127.0.0.1:$API_PORT/api/v1/auth/login" | jq -r .accessToken)
  say "agent pid $AGENT (owner $OWNER, table base $BASE), API :$API_PORT health $(apij GET /api/v1/health | jq -c '{status}')"
  say "GET /state/tunnels (empty stack): $(apij GET /api/v1/state/tunnels | jq -c .)"
}

step2() {
  say "=== 2 tunnels.instance-required (D-205): GRE without instance → 400 with a pointer, agent not asked ==="
  api PATCH /api/v1/config/interfaces "{\"$LOOP\":{\"enabled\":true,\"ipv4\":[\"$SRC/24\"]}}" >/dev/null
  api PATCH /api/v1/config/tunnels "{\"gre\":{\"$P-nogre\":{\"src\":\"$SRC\",\"dst\":\"10.$N.7.2\"}}}" >/dev/null
  local code; code=$(curl --max-time 15 -s -o "$RUN/last.json" -w '%{http_code}' -X POST -H "authorization: Bearer $TOKEN" "http://127.0.0.1:$API_PORT/api/v1/config/commit?comment=t1-noinstance")
  say "commit → HTTP $code $(jq -c '{title,tier,errors:(.errors//[]|map({pointer,message}))}' "$RUN/last.json")"
  [[ "$code" == 400 ]] || { say "missing instance was not refused"; return 1; }
  api PATCH /api/v1/config/tunnels "{\"gre\":{\"$P-nogre\":null}}" >/dev/null
}

step3() {
  say "=== 3 gre + vxlan-gpe on the real engine ==="
  api PATCH /api/v1/config/tunnels "{\"gre\":{\"$P-gre\":{\"instance\":$((BASE+1)),\"description\":\"S-tunnels-contract\",\"src\":\"$SRC\",\"dst\":\"10.$N.7.2\",\"ipv4\":[\"10.$N.254.1/30\"]}},
    \"vxlanGpe\":{\"$P-gpe\":{\"src\":\"$SRC\",\"dst\":\"10.$N.7.3\",\"vni\":$((BASE+3)),\"mtu\":1500,\"protocol\":\"ip4\",\"ipv4\":[\"10.$N.253.1/30\"]}}}" >/dev/null
  local committed; committed=$(commit t1-tunnels) || return 1
  say "commit → $committed"
  say "vppctl show gre tunnel:"; V show gre tunnel | tee -a "$LOG"
  say "vppctl show vxlan-gpe tunnel:"; V show vxlan-gpe tunnel | tee -a "$LOG"
  say "vppctl show interface (ours):"; V show interface | grep -E "^(gre$((BASE+1))|vxlan_gpe_tunnel[0-9]+) " | tee -a "$LOG"
  say "vppctl show interface addr (ours):"; V show interface addr | grep -A1 -E "^(gre$((BASE+1))|vxlan_gpe_tunnel[0-9]+) " | tee -a "$LOG"
  local tunnel_state; tunnel_state=$(apij GET /api/v1/state/tunnels)
  jq -e --arg gre "$P-gre" --arg gpe "$P-gpe" '.tunnels | length == 2 and any(.[]; .name == $gre and .kind == "gre" and .adminUp) and any(.[]; .name == $gpe and .kind == "vxlanGpe" and .adminUp)' <<<"$tunnel_state" >/dev/null
  say "GET /state/tunnels: $tunnel_state"
  say "GET /config/tunnels (running): $(apij GET /api/v1/config/tunnels | jq -c '{gre,vxlanGpe}')"
  local drift; drift=$(apij GET /api/v1/state/drift)
  [[ $(jq '.changes|length' <<<"$drift") == 0 ]] || { say "unexpected drift: $drift"; return 1; }
  say "GET /state/drift: $drift"
  api PATCH /api/v1/config/tunnels "{\"gre\":{\"$P-gre\":{\"description\":\"S-tunnels-contract\"}}}" >/dev/null
  local unchanged; unchanged=$(commit t1-nochange noop) || return 1
  say "re-commit (no change) → $unchanged"
}

step4() {
  say "=== 4 rollback ==="
  api PATCH /api/v1/config/tunnels "{\"gre\":{\"$P-gre\":null},\"vxlanGpe\":{\"$P-gpe\":null}}" >/dev/null
  local reverted; reverted=$(commit t1-rollback) || return 1
  BASE_REV=$(jq -r .revision <<<"$reverted")
  say "commit → $reverted"
  say "show gre tunnel lines with $SRC: $(V show gre tunnel | grep -c "$SRC") · show vxlan-gpe tunnel lines with $SRC: $(V show vxlan-gpe tunnel | grep -c "$SRC")"
  local empty_state; empty_state=$(apij GET /api/v1/state/tunnels)
  jq -e '.tunnels | length == 0' <<<"$empty_state" >/dev/null
  say "GET /state/tunnels: $empty_state"
  assert_native_absent gre
  assert_native_absent vxlan-gpe
}


step_all() {
  say "=== 5 all-kind GRE/IPIP/VXLAN acceptance ==="
  [[ -x "$ROOT/.scratch/tunnels-delete-owned" ]] || { say "build delete-owned.go helper first"; return 1; }
  local config state drift code result began elapsed old_agent
  config=$(jq -nc --arg p "$P" --arg src "$SRC" --arg dst "10.$N.7.2" --arg ipipdst "10.$N.7.3" --arg vxdst "10.$N.7.4" --arg ga "10.$N.251.1/30" --arg ia "10.$N.252.1/30" --arg va "10.$N.253.1/30" --argjson b "$BASE" '{gre:{($p+"-gre"):{instance:($b+1),src:$src,dst:$dst,ipv4:[$ga]}},ipip:{($p+"-ipip"):{instance:($b+2),src:$src,dst:$ipipdst,ipv4:[$ia]}},vxlan:{($p+"-vxlan"):{instance:($b+3),src:$src,dst:$vxdst,vni:($b+3),decap:"ip4",mtu:1500,ipv4:[$va]}}}')
  api PATCH /api/v1/config/tunnels "$config" >/dev/null
  result=$(commit all-kinds); say "commit → $result"
  check_all
  result=$(commit all-kinds-unchanged noop) || return 1
  say "unchanged commit → $result"
  check_all
  say "=== 6 duplicate VXLAN tuple rejected ==="
  api PATCH /api/v1/config/tunnels "$(jq -nc --arg p "$P" --argjson c "$config" --argjson b "$BASE" '{vxlan:{($p+"-duplicate"):($c.vxlan[$p+"-vxlan"] + {instance:($b+4)})}}')" >/dev/null
  code=$(curl --max-time 15 -s -o "$RUN/duplicate.json" -w '%{http_code}' -X POST -H "authorization: Bearer $TOKEN" "http://127.0.0.1:$API_PORT/api/v1/config/commit?comment=duplicate-vxlan")
  [[ "$code" == 400 ]] || { say "duplicate accepted HTTP $code"; return 1; }
  jq -e --arg p "/tunnels/vxlan/$P-duplicate" 'any(.errors[]; .pointer|startswith($p))' "$RUN/duplicate.json" >/dev/null
  say "duplicate HTTP $code $(jq -c . "$RUN/duplicate.json")"
  api POST /api/v1/config/discard >/dev/null
  say "=== 7 real agent restart after native tunnel loss ==="
  old_agent=$AGENT
  stop_owned "$old_agent"
  local kept=() pid
  for pid in "${PIDS[@]}"; do [[ "$pid" == "$old_agent" ]] || kept+=("$pid"); done
  PIDS=("${kept[@]}")
  "$ROOT/.scratch/tunnels-delete-owned" "$SRC" "$BASE" | tee -a "$LOG"
  for kind in gre ipip vxlan; do
    assert_native_absent "$kind"
  done
  began=$SECONDS
  start_agent
  while (( SECONDS-began < 30 )); do
    state=$(apij GET /api/v1/state/tunnels || true)
    drift=$(apij GET /api/v1/state/drift || true)
    if jq -e '.tunnels|length==3' <<<"$state" >/dev/null 2>&1 && jq -e '.changes|type=="array" and length==0' <<<"$drift" >/dev/null 2>&1; then break; fi
    sleep 0.2
  done
  elapsed=$((SECONDS-began)); ((elapsed<30)) || { say "restart exceeded30s: $state $drift"; return 1; }
  say "restart recovery ${elapsed}s"
  check_all
  say "=== 8 historical revision rollback ==="
  result=$(api POST "/api/v1/config/rollback/$BASE_REV?comment=all-kinds-rollback")
  jq -e '.status=="applied"' <<<"$result" >/dev/null
  say "rollback → $(jq -c '{status,revision:.revision.id}' <<<"$result")"
  state=$(apij GET /api/v1/state/tunnels); jq -e '.tunnels|length==0' <<<"$state" >/dev/null
  say "empty state $state"
  for kind in gre ipip vxlan; do
    assert_native_absent "$kind"
  done
  drift=$(apij GET /api/v1/state/drift); jq -e '.changes|type=="array" and length==0' <<<"$drift" >/dev/null
  say "rollback empty drift $drift"
  jq -e '.entries|type=="array" and length==0' "$RUN/agent-state/tunnels-meta-$OWNER.json" >/dev/null
  say "persisted tunnels.meta entries=0"
}

assert_native_absent() {
  local native
  native=$(V show "$1" tunnel) || return 1
  if grep -Fq "$SRC" <<<"$native"; then say "native residue for $1: $native"; return 1; fi
}

check_all() {
  local state drift kind
  state=$(apij GET /api/v1/state/tunnels)
  jq -e --arg p "$P" '.tunnels|length==3 and any(.[];.name==($p+"-gre") and .kind=="gre" and .adminUp) and any(.[];.name==($p+"-ipip") and .kind=="ipip" and .adminUp) and any(.[];.name==($p+"-vxlan") and .kind=="vxlan" and .adminUp)' <<<"$state" >/dev/null
  say "all-kind state $state"
  drift=$(apij GET /api/v1/state/drift); jq -e '.changes|type=="array" and length==0' <<<"$drift" >/dev/null
  say "all-kind drift $drift"
  for kind in gre ipip vxlan; do V show "$kind" tunnel | tee -a "$LOG"; done
  V show interface addr | tee -a "$LOG"
}

case $CMD in
  run|all)
    exec 9>>/run/lock/ngfw-lab.lock; flock -s -w 600 9 || { echo "lab lock busy"; exit 1; }
    say "window start $(date -Is): flock -s ngfw-lab.lock (no VPP-global touched)"
    trap cleanup EXIT
    step0; step1; step2; step3; step4
    if [[ "$CMD" == all ]]; then step_all; fi
    ;;
  *) echo "usage: $0 run|all [evidence-dir]"; exit 2;;
esac
