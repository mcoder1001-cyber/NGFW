#!/usr/bin/env bash
# docs/status/tasks/F-nat46-host-evidence/host.sh — F-nat46-host evidence driver (D-175: committed beside the evidence).
#
#   cd /root/ngfw-wt/F-nat46-host; eval "$(tools/lab env 17)"; docs/status/tasks/F-nat46-host-evidence/host.sh run [evidence-dir]
#
# Slot 17 has no Valkey database (envelope) → Go/agent level only, no API stack: the real ngfw-agent (owner = slot prefix,
# NGFW_GLOBALS_OWNER=0) on the slot socket, driven with ngfw-agentctl (protobuf-JSON documents exactly as the API sends them),
# on the af_packet rig of tools/lab (`path: af_packet`). Everything created carries the slot prefix; every process is killed by
# PID; the shared lab lock (fd 9, flock -s) is held for the whole run and closed in every background child (`9>&-`). No VPP-global
# setting is touched (map params stay as they are; D-167 window not needed). Never restarts VPP, never `delete host-interface`
# by hand (tools/lab rig down does it with the veth down, D-101).
#
# Steps (owed list of docs/status/tasks/F-nat46.md "Not done / remains"):
#   0 NRestarts + map plugin · 1 TestNat46OnHost (descriptor level; pauses for `vppctl show map domain`) ·
#   2 rig up + IPv6 on the wan namespace (the IPv6-only server) · 3 agent: apply nat.nat46 → Retrieve == canonical, re-apply
#   empty, `vppctl show map domain` · 4 packet test IPv4 client → IPv6-only server (tcpdump in ns-<p>-wan, counters) ·
#   5 agent restart simulation (domain deleted behind its back, agent restarted → recreated, timed) · 6 duplicate IPv4 service
#   address → dryrun refusal with pointer · 7 rollback → nothing left · cleanup + NRestarts.
set -uo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../../.." && pwd)"
: "${NGFW_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
: "${NGFW_SLOT:?eval \"\$(tools/lab env <slot>)\" first}"
CMD=${1:-run}; EVID=${2:-$HERE}
[[ "$NGFW_TEST_PREFIX" == "w$NGFW_SLOT" && "$NGFW_SLOT" =~ ^([1-9]|[12][0-9]|3[0-2])$ ]] || { echo "invalid slot/prefix" >&2; exit 64; }
case $CMD in run|step1) ;; *) echo "usage: $0 run|step1 [evidence-dir]" >&2; exit 64 ;; esac
P=$NGFW_TEST_PREFIX N=$NGFW_SLOT OWNER=$NGFW_TEST_PREFIX BASE=$NGFW_VPP_TABLE_BASE
H=$(printf '%x' "$N")                       # slot in hex: the slot's IPv6 block is fd00:<hex>::/32
LAN_IF=host-${P}l0 WAN_IF=host-${P}w0 NS_LAN=ns-$P-lan NS_WAN=ns-$P-wan LAN_PEER=${P}l1 WAN_PEER=${P}w1
LAN_GW=10.$N.1.1 WAN_GW=10.$N.2.1 CLIENT=10.$N.1.2 SVC4=10.$N.46.10
WAN6_GW=fd00:$H:2::1 SERVER6=fd00:$H:2::46 CPFX=fd00:$H:4646::/96
CLIENT6=fd00:$H:4646::$(printf '%x' $((10 * 256 + N))):$(printf '%x' $((1 * 256 + 2)))   # RFC 6052: CPFX + 10.N.1.2
PORT=$((NGFW_HTTP_PORT + 46))                 # sub-port of the slot block (shared-host-rules §1)
RUN=/run/ngfw-test/$P/nat46
BIN=/tmp/g-$P/bin
mkdir -p "$EVID" "$RUN"
LOG=$RUN/evidence.log
: > "$LOG"
die() { say "FAIL: $*"; exit 1; }
say() { printf '%s %s\n' "$(date +%T)" "$*" | tee -a "$LOG"; }
nres() { say "NRestarts $1: $(systemctl show vpp -p NRestarts)"; }
V() { timeout 60 vppctl "$@" 2>&1; }
PIDS=()
CTL="$BIN/ngfw-agentctl -s $RUN/agent.sock"

agent_env() {
  exec env -i PATH="$PATH" HOME="$HOME" NGFW_OWNER="$OWNER" NGFW_GLOBALS_OWNER=0 NGFW_AGENT_SOCKET="$RUN/agent.sock" \
    NGFW_AGENT_STATE_DIR="$RUN/agent-state" NGFW_METRICS_ADDR=off NGFW_SOCKET_GROUP=root NGFW_LOG_LEVEL=info \
    NGFW_VPP_TABLE_BASE="$BASE" NGFW_TEST_PREFIX="$P" "$@"
}
start_agent() { ( agent_env "$BIN/ngfw-agent" ) >> "$RUN/agent.log" 2>&1 9>&- & AGENT=$!; PIDS+=("$AGENT"); say "agent started pid $AGENT"; }
wait_agent() { local i; for i in $(seq 1 60); do $CTL health >/dev/null 2>&1 && return 0; sleep 0.5; done; say "agent did not answer health"; return 1; }
stop_agent() { [[ -n "${AGENT:-}" ]] || return 0; kill "$AGENT" 2>/dev/null; wait "$AGENT" 2>/dev/null; say "agent pid $AGENT stopped"; AGENT=; }
agent_log_since() { tail -c +"$(( ${1:-0} + 1 ))" "$RUN/agent.log" | grep -E "$2" | cut -c1-"${3:-400}"; }
retrieve_nat() { $CTL retrieve -subsystems nat 2>/dev/null | jq -cS '.desiredState.nat // {}'; }
show_domains() { V show map domain | grep -E "^\[[0-9]+\] tag \{$OWNER:" || true; }

doc_full() { cat <<EOF
{"interfaces": {
   "$LAN_IF": {"enabled": true, "description": "nat46 IPv4 side (rig lan)", "ipv4": ["$LAN_GW/24"]},
   "$WAN_IF": {"enabled": true, "description": "nat46 IPv6 side (rig wan)", "ipv4": ["$WAN_GW/24"], "ipv6": ["$WAN6_GW/64"]}},
 "nat": {"nat46": {"clientPrefix": "$CPFX", "interfaces": ["$LAN_IF", "$WAN_IF"],
   "mappings": [{"name": "web", "ipv4": "$SVC4", "ipv6": "$SERVER6"}]}}}
EOF
}
doc_ifs_only() { cat <<EOF
{"interfaces": {
   "$LAN_IF": {"enabled": true, "description": "nat46 IPv4 side (rig lan)", "ipv4": ["$LAN_GW/24"]},
   "$WAN_IF": {"enabled": true, "description": "nat46 IPv6 side (rig wan)", "ipv4": ["$WAN_GW/24"], "ipv6": ["$WAN6_GW/64"]}},
 "nat": {}}
EOF
}
doc_dup() { cat <<EOF
{"interfaces": {
   "$LAN_IF": {"enabled": true, "ipv4": ["$LAN_GW/24"]},
   "$WAN_IF": {"enabled": true, "ipv4": ["$WAN_GW/24"], "ipv6": ["$WAN6_GW/64"]}},
 "nat": {"nat46": {"clientPrefix": "$CPFX", "interfaces": ["$LAN_IF", "$WAN_IF"],
   "mappings": [{"name": "web", "ipv4": "$SVC4", "ipv6": "$SERVER6"}, {"name": "dup", "ipv4": "$SVC4", "ipv6": "fd00:$H:2::47"}]}}}
EOF
}
CANON=$(jq -cS . <<EOF
{"nat46": {"clientPrefix": "$CPFX", "interfaces": ["$LAN_IF", "$WAN_IF"], "mappings": [{"name": "web", "ipv4": "$SVC4", "ipv6": "$SERVER6"}]}}
EOF
)

cleanup() {
  set +e
  say "=== cleanup ==="
  for ((i=${#PIDS[@]}-1; i>=0; i--)); do kill "${PIDS[$i]}" 2>/dev/null; wait "${PIDS[$i]}" 2>/dev/null; done
  ip -n "$NS_WAN" -6 route del "$CPFX" 2>/dev/null
  ip -n "$NS_WAN" -6 addr del "$SERVER6/64" dev "$WAN_PEER" 2>/dev/null
  "$ROOT/tools/lab" rig down "$P" >> "$LOG" 2>&1 9>&-
  say "leftover map domains of $OWNER: $(show_domains | wc -l) · rig: $("$ROOT/tools/lab" rig show "$P" 2>&1 9>&- | grep -E '^state' || true)"
  nres "after cleanup"
  say "run end $(date -Is)"
  cp "$LOG" "$EVID/evidence.txt"; cp "$RUN/agent.log" "$EVID/agent-log.txt" 2>/dev/null
}

# ---------------------------------------------------------------- steps
step0() {
  say "=== step 0: host facts (run start $(date -Is), slot $N prefix $P, path: af_packet) ==="
  nres "before"
  say "load: $(cut -d' ' -f1-3 /proc/loadavg)"
  say "vpp: $(V show version | head -1)"
  say "map plugin: $(V show plugins | grep -E 'map_plugin' | head -1)"
  say "map domains of $OWNER before: $(show_domains | wc -l)"
}

step1_test() {
  say "=== step 1: TestNat46OnHost (descriptor level, NGFW_INTEGRATION=1, shared lab lock) ==="
  nres "before step 1"
  local ev=$RUN/ev; rm -rf "$ev"; mkdir -p "$ev"
  ( cd "$ROOT/apps/agent" && NGFW_INTEGRATION=1 NGFW_EVIDENCE_DIR="$ev" TMPDIR=/tmp/g-$P \
      go test -count=1 -v -run 'TestNat46OnHost$' ./internal/descriptors/nat46 ) > "$RUN/step1.txt" 2>&1 9>&- &
  local tpid=$!
  local i; for i in $(seq 1 600); do [[ -f $ev/nat46.ready ]] && break; kill -0 $tpid 2>/dev/null || break; sleep 0.5; done
  if [[ -f $ev/nat46.ready ]]; then
    say "vppctl show map domain (ours, while the test holds the domain):"
    show_domains | tee -a "$LOG"
    say "vppctl show map domain index <ours> counters:"
    local idx; idx=$(show_domains | sed -nE 's/^\[([0-9]+)\].*/\1/p' | head -1)
    [[ -n $idx ]] && V show map domain index "$idx" counters | tee -a "$LOG"
    say "interfaces with map-t (show interface features, ours):"
    V show interface features "loop${N}46" | grep -E 'map-t' | tee -a "$LOG"
    touch "$ev/nat46.go"
  fi
  local rc=0; wait "$tpid" || rc=$?
  [[ "$rc" == 0 ]] || die "TestNat46OnHost exited $rc"
  say "go test rc=$rc:"; grep -E '^(=== RUN|--- |PASS|FAIL|ok|\s+nat46_integration_test)' "$RUN/step1.txt" | cut -c1-300 | tee -a "$LOG"
  cp "$RUN/step1.txt" "$EVID/step1-test-nat46-on-host.txt"
  say "map domains of $OWNER after the test: $(show_domains | wc -l)"
  nres "after step 1"
}

step2_rig() {
  say "=== step 2: rig up $P (af_packet) + IPv6-only server side ==="
  nres "before step 2"
  "$ROOT/tools/lab" rig up "$P" 2>&1 9>&- | tee -a "$LOG"
  local kv
  for kv in accept_ra=0 autoconf=0 accept_dad=0 disable_ipv6=0; do
    ip netns exec "$NS_WAN" sysctl -qw "net.ipv6.conf.$WAN_PEER.$kv"
  done
  ip -n "$NS_WAN" -6 addr add "$SERVER6/64" dev "$WAN_PEER" nodad
  ip -n "$NS_WAN" -6 route replace "$CPFX" via "$WAN6_GW"
  say "ns $NS_WAN: $(ip -n "$NS_WAN" -6 addr show dev "$WAN_PEER" | grep -E 'inet6 fd00' | xargs) · route: $(ip -n "$NS_WAN" -6 route show "$CPFX" | xargs)"
  say "ns $NS_LAN: $(ip -n "$NS_LAN" -4 addr show dev "$LAN_PEER" | grep -E 'inet ' | xargs) · route: $(ip -n "$NS_LAN" route show default | xargs)"
  nres "after step 2"
}

step3_agent() {
  say "=== step 3: agent apply nat.nat46 → Retrieve == canonical, idempotent re-apply, vppctl show map domain ==="
  nres "before step 3"
  rm -rf "$RUN/agent-state" "$RUN/agent.log"; start_agent; wait_agent || return 1
  doc_full > "$RUN/full.json"; doc_ifs_only > "$RUN/ifs.json"; doc_dup > "$RUN/dup.json"
  say "apply full.json: $($CTL apply "$RUN/full.json" -txn nat46-1 | jq -c '{txnId,status,results:[.results[]?|{key,op,code}],errors:(.errors//[]|map({pointer,message}))}')"
  local got; got=$(retrieve_nat)
  say "Retrieve nat: $got"
  say "canonical  : $CANON"
  [[ "$got" == "$CANON" ]] || die "Retrieve != canonical"
  say "Retrieve == canonical: OK"
  say "re-apply full.json (results must be empty): $($CTL apply "$RUN/full.json" -txn nat46-2 | jq -c '{status,results:[.results[]?|{key,op}]}')"
  say "vppctl show map domain (ours):"; show_domains | tee -a "$LOG"
  say "vppctl show interface features $LAN_IF / $WAN_IF (map-t):"
  V show interface features "$LAN_IF" | grep -E 'map-t' | sed "s/^/  $LAN_IF /" | tee -a "$LOG"
  V show interface features "$WAN_IF" | grep -E 'map-t' | sed "s/^/  $WAN_IF /" | tee -a "$LOG"
  say "vppctl show interface addr (ours):"; V show interface addr "$WAN_IF" | tee -a "$LOG"
  nres "after step 3"
}

step4_packets() {
  say "=== step 4: packet test IPv4 client $CLIENT → $SVC4:$PORT ⇒ IPv6-only server [$SERVER6]:$PORT (path: af_packet) ==="
  nres "before step 4"
  local srv=$RUN/server6.py cli=$RUN/client4.py
  cat > "$srv" <<'EOF'
import socket, sys
s = socket.socket(socket.AF_INET6)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind((sys.argv[1], int(sys.argv[2])))
s.listen(16)
while True:
    c, peer = s.accept()
    print("server6: connection from", peer[0], peer[1], flush=True)
    try:
        c.sendall(b"ngfw-nat46-ok\n")
    except OSError:
        pass
    c.close()
EOF
  cat > "$cli" <<'EOF'
import socket, sys
c = socket.socket(socket.AF_INET)
c.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
c.bind((sys.argv[1], int(sys.argv[2])))
c.settimeout(5)
c.connect((sys.argv[3], int(sys.argv[4])))
print("client4: read", c.recv(64).decode().strip(), flush=True)
c.close()
EOF
  ip netns exec "$NS_WAN" python3 "$srv" "$SERVER6" "$PORT" > "$RUN/server6.txt" 2>&1 9>&- & PIDS+=("$!")
  ip netns exec "$NS_WAN" tcpdump -lni "$WAN_PEER" -c 6 "ip6 and tcp port $PORT" > "$RUN/tcpdump6.txt" 2>&1 9>&- & local td=$!; PIDS+=("$td")
  sleep 1
  local rx0 tx0
  rx0=$(V show interface "$LAN_IF" | awk '/rx packets/ {print $NF}' | head -1); tx0=$(V show interface "$WAN_IF" | awk '/tx packets/ {print $NF}' | head -1)
  say "ping $SVC4 from $NS_LAN (ICMP → ICMPv6 → ICMP):"
  ip netns exec "$NS_LAN" ping -c 3 -W 2 "$SVC4" 2>&1 | tail -3 | tee -a "$LOG"
  local response
  response=$(ip netns exec "$NS_LAN" python3 "$cli" "$CLIENT" 46001 "$SVC4" "$PORT" 2>&1) || die "IPv4-to-IPv6 TCP failed: $response"
  say "tcp: $response"
  sleep 1
  say "server side: $(cat "$RUN/server6.txt")"
  say "tcpdump in $NS_WAN on $WAN_PEER (IPv6 only — the client appears as $CLIENT6):"
  for i in $(seq 1 20); do kill -0 "$td" 2>/dev/null || break; sleep 0.25; done; kill "$td" 2>/dev/null; wait "$td" 2>/dev/null
  grep -E 'IP6|packets captured' "$RUN/tcpdump6.txt" | cut -c1-200 | tee -a "$LOG"
  grep -q "$CLIENT6" "$RUN/tcpdump6.txt" || die "translated source $CLIENT6 NOT seen"
  say "translated source $CLIENT6 seen on IPv6 side: OK"
  say "counters: $LAN_IF rx $rx0 → $(V show interface "$LAN_IF" | awk '/rx packets/ {print $NF}' | head -1) · $WAN_IF tx $tx0 → $(V show interface "$WAN_IF" | awk '/tx packets/ {print $NF}' | head -1)"
  say "vppctl show map domain index <ours> counters:"
  local idx; idx=$(show_domains | sed -nE 's/^\[([0-9]+)\].*/\1/p' | head -1)
  [[ -n $idx ]] && V show map domain index "$idx" counters | tee -a "$LOG"
  cp "$RUN/tcpdump6.txt" "$EVID/step4-tcpdump-wan6.txt"
  nres "after step 4"
}

step5_restart() {
  say "=== step 5: agent restart simulation (domain deleted behind the agent's back) ==="
  nres "before step 5"
  local idx; idx=$(show_domains | sed -nE 's/^\[([0-9]+)\].*/\1/p' | head -1)
  [[ "$idx" =~ ^[0-9]+$ ]] || die "no uniquely identified owned MAP domain for loss simulation"
  [[ "$(show_domains | wc -l)" == 1 ]] || die "expected exactly one owned MAP domain"
  local off; off=$(stat -c %s "$RUN/agent.log")
  stop_agent
  say "map del domain index $idx (simulated loss): $(V map del domain index "$idx" | xargs)"
  say "map domains of $OWNER after the loss: $(show_domains | wc -l)"
  local t0; t0=$(date +%s.%N)
  start_agent; wait_agent || return 1
  local i got; for i in $(seq 1 120); do got=$(retrieve_nat); [[ "$got" == "$CANON" ]] && break; sleep 0.25; done
  [[ "$got" == "$CANON" ]] || die "restart did not converge within 30 seconds"
  say "Retrieve == canonical again after $(awk -v a="$(date +%s.%N)" -v b="$t0" 'BEGIN{printf "%.2f", a-b}') s (limit 30 s): $([[ "$got" == "$CANON" ]] && echo OK || echo "NOT YET: $got")"
  say "vppctl show map domain (ours, recreated):"; show_domains | tee -a "$LOG"
  say "agent log (reconcile/resync lines since the restart):"
  agent_log_since "$off" 'reconcile|resync|map\.domain|nat46' 300 | head -12 | tee -a "$LOG"
  say "tcp again after the restart: $(ip netns exec "$NS_LAN" python3 "$RUN/client4.py" "$CLIENT" 46002 "$SVC4" "$PORT" 2>&1)"
  nres "after step 5"
}

step6_dup() {
  say "=== step 6: duplicate IPv4 service address → dryrun refusal with pointer (agent level; no API stack on slot $N) ==="
  say "dryrun dup.json: $($CTL dryrun "$RUN/dup.json" 2>&1 | jq -c '{ok,errors:[.errors[]?|{rule,pointer,message}]}' 2>/dev/null || $CTL dryrun "$RUN/dup.json" 2>&1 | head -3)"
  say "apply dup.json: $($CTL apply "$RUN/dup.json" -txn nat46-dup 2>&1 | jq -c '{status,errors:[.errors[]?|{rule,pointer,message}]}' 2>/dev/null || true)"
  [[ "$(retrieve_nat)" == "$CANON" ]] || die "invalid duplicate configuration changed state"
  say "Retrieve nat unchanged: OK"
}

step7_rollback() {
  say "=== step 7: rollback (nat: {}) → nothing left ==="
  nres "before step 7"
  say "apply ifs.json: $($CTL apply "$RUN/ifs.json" -txn nat46-rb | jq -c '{status,results:[.results[]?|{key,op,code}]}')"
  say "Retrieve nat after rollback: $(retrieve_nat)"
  [[ "$(show_domains | wc -l)" == 0 ]] || die "MAP domains remain after rollback"
  say "map domains of $OWNER after rollback: 0"
  say "map-t features left on $LAN_IF/$WAN_IF: $(V show interface features "$LAN_IF" "$WAN_IF" | grep -c 'map-t')"
  stop_agent
  nres "after step 7"
}

case $CMD in
  run)
    exec 9>/run/lock/ngfw-lab.lock; flock -s 9; say "shared lab lock held (fd 9)"
    trap cleanup EXIT
    step0 || exit 1
    step1_test || exit 1
    step2_rig || exit 1
    step3_agent || exit 1
    step4_packets || exit 1
    step5_restart || exit 1
    step6_dup || exit 1
    step7_rollback || exit 1
    ;;
  step1) exec 9>/run/lock/ngfw-lab.lock; flock -s 9; trap cleanup EXIT; step0; step1_test ;;
  *) echo "usage: $0 run|step1 [evidence-dir]" >&2; exit 64 ;;
esac
