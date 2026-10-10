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
# A flag alone cannot certify socket isolation: require the root-owned marker
# written by isolated-vpp.py and a mount namespace distinct from PID1 before
# creating runtime directories, touching rig devices or connecting an agent.
[[ "${NGFW_DISPOSABLE_VPP:-}" == 1 && -f /run/vpp/startup.conf && ! -L /run/vpp/startup.conf ]] || { echo "private VPP required" >&2; exit 64; }
[[ "$(stat -c %u /run/vpp/startup.conf)" == 0 ]] && grep -Eq '^api-segment \{ prefix fulltest[0-9]+ \}$' /run/vpp/startup.conf &&
  [[ "$(readlink /proc/self/ns/mnt)" != "$(readlink /proc/1/ns/mnt)" ]] || { echo "private VPP startup marker/mount namespace not verified" >&2; exit 64; }
[[ "$NGFW_TEST_PREFIX" == "w$NGFW_SLOT" && "$NGFW_SLOT" =~ ^([1-9]|[12][0-9]|3[0-2])$ ]] || { echo "invalid slot/prefix" >&2; exit 64; }
case $CMD in run|step1) ;; *) echo "usage: $0 run|step1 [evidence-dir]" >&2; exit 64 ;; esac
P=$NGFW_TEST_PREFIX N=$NGFW_SLOT OWNER=$NGFW_TEST_PREFIX BASE=$NGFW_VPP_TABLE_BASE
H=$(printf '%x' "$N")                       # slot in hex: the slot's IPv6 block is fd00:<hex>::/32
LAN_IF=host-${P}l0 WAN_IF=host-${P}w0 NS_LAN=ns-$P-lan NS_WAN=ns-$P-wan LAN_PEER=${P}l1 WAN_PEER=${P}w1
LAN_GW=10.$N.1.1 WAN_GW=10.$N.2.1 CLIENT=10.$N.1.2 SVC4=10.$N.46.10
WAN6_GW=fd00:$H:2::1 LINK6=fd00:$H:2::46 SRV64=fd00:$H:46::/64
SERVER6=fd00:$H:46::$(printf '%x:%x' $((10 * 256 + N)) $((46 * 256 + 10))) CPFX=fd00:$H:4646::/96
CLIENT6=fd00:$H:4646::$(printf '%x' $((10 * 256 + N))):$(printf '%x' $((1 * 256 + 2)))   # RFC 6052: CPFX + 10.N.1.2
PORT=$((NGFW_HTTP_PORT + 46))                 # sub-port of the slot block (shared-host-rules §1)
RUN=/run/ngfw-test/$P/nat46
BIN=${NGFW_NAT46_BIN_DIR:-/tmp/g-$P/bin}
mkdir -p "$EVID" "$RUN"
LOG=$RUN/evidence.log
: > "$LOG"
die() { say "FAIL: $*"; exit 1; }
say() { printf '%s %s\n' "$(date +%T)" "$*" | tee -a "$LOG"; }
nres() { say "NRestarts $1: $(systemctl show vpp -p NRestarts)"; }
V() { timeout 10 vppctl "$@" 2>&1; }
PIDS=()
CTL=("$BIN/ngfw-agentctl" -s "$RUN/agent.sock")
for executable in ngfw-agent ngfw-agentctl ngfw-vpp-preflight; do
  [[ -x "$BIN/$executable" ]] || die "missing executable $BIN/$executable (select NGFW_NAT46_BIN_DIR)"
done

agent_env() {
  exec env -i PATH="$PATH" HOME="$HOME" NGFW_OWNER="$OWNER" NGFW_GLOBALS_OWNER=0 NGFW_AGENT_SOCKET="$RUN/agent.sock" \
    NGFW_AGENT_STATE_DIR="$RUN/agent-state" NGFW_METRICS_ADDR=off NGFW_SOCKET_GROUP=root NGFW_LOG_LEVEL=info \
    NGFW_VPP_TABLE_BASE="$BASE" NGFW_TEST_PREFIX="$P" "$@"
}
start_agent() { ( agent_env "$BIN/ngfw-agent" ) >> "$RUN/agent.log" 2>&1 9>&- & AGENT=$!; PIDS+=("$AGENT"); say "agent started pid $AGENT"; }
wait_agent() { local i; for i in $(seq 1 60); do "${CTL[@]}" health >/dev/null 2>&1 && return 0; sleep 0.5; done; say "agent did not answer health"; return 1; }
stop_agent() { [[ -n "${AGENT:-}" ]] || return 0; kill "$AGENT" 2>/dev/null; wait "$AGENT" 2>/dev/null; say "agent pid $AGENT stopped"; AGENT=; }
agent_log_since() { tail -c +"$(( ${1:-0} + 1 ))" "$RUN/agent.log" | grep -E "$2" | cut -c1-"${3:-400}"; }
retrieve_nat() { "${CTL[@]}" retrieve -subsystems nat 2>/dev/null | jq -cS '.desiredState.nat // {}'; }
show_domains() { V show map domain | grep -E "^\[[0-9]+\] tag \{$OWNER:" || true; }

doc_full() { cat <<EOF
{"interfaces": {
   "$LAN_IF": {"enabled": true, "description": "nat46 IPv4 side (rig lan)", "ipv4": ["$LAN_GW/24"]},
   "$WAN_IF": {"enabled": true, "description": "nat46 IPv6 side (rig wan)", "ipv4": ["$WAN_GW/24"], "ipv6": ["$WAN6_GW/64"]}},
 "routing": {"static":[{"prefix":"$SRV64","nextHops":[{"address":"$LINK6","interface":"$WAN_IF"}]}]},
 "nat": {"nat46": {"clientPrefix": "$CPFX", "interfaces": ["$LAN_IF", "$WAN_IF"],
   "mappings": [{"name": "web", "ipv4": "$SVC4", "ipv6": "$SERVER6"}]}}}
EOF
}
doc_ifs_only() { cat <<EOF
{"interfaces": {
   "$LAN_IF": {"enabled": true, "description": "nat46 IPv4 side (rig lan)", "ipv4": ["$LAN_GW/24"]},
   "$WAN_IF": {"enabled": true, "description": "nat46 IPv6 side (rig wan)", "ipv4": ["$WAN_GW/24"], "ipv6": ["$WAN6_GW/64"]}},
 "routing": {"static": []}, "nat": {}}
EOF
}
doc_dup() { cat <<EOF
{"interfaces": {
   "$LAN_IF": {"enabled": true, "ipv4": ["$LAN_GW/24"]},
   "$WAN_IF": {"enabled": true, "ipv4": ["$WAN_GW/24"], "ipv6": ["$WAN6_GW/64"]}},
 "routing": {"static":[{"prefix":"$SRV64","nextHops":[{"address":"$LINK6","interface":"$WAN_IF"}]}]},
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
  if [[ -n "${AGENT:-}" ]] && "${CTL[@]}" health >/dev/null 2>&1; then
    ip -n "$NS_LAN" link set "$LAN_PEER" down 2>/dev/null
    ip -n "$NS_WAN" link set "$WAN_PEER" down 2>/dev/null
    printf '{"nat":{},"interfaces":{},"routing":{"static":[]}}\n' > "$RUN/cleanup.json"
    local result
    result=$("${CTL[@]}" apply "$RUN/cleanup.json" -txn nat46-cleanup 2>&1)
    say "owned object cleanup: $result"
  fi
  for ((i=${#PIDS[@]}-1; i>=0; i--)); do kill "${PIDS[$i]}" 2>/dev/null; wait "${PIDS[$i]}" 2>/dev/null; done
  ip -n "$NS_WAN" -6 route del "$CPFX" 2>/dev/null
  ip -n "$NS_WAN" -6 addr del "$SERVER6/128" dev lo 2>/dev/null
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
  grep -Eq '^--- PASS: TestNat46OnHost ' "$RUN/step1.txt" || die "selected host test did not execute and pass"
  ! grep -q '^--- SKIP:' "$RUN/step1.txt" || die "host test skipped"
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
  ip -n "$NS_WAN" -6 addr add "$LINK6/64" dev "$WAN_PEER" nodad
  ip -n "$NS_WAN" -6 addr add "$SERVER6/128" dev lo nodad
  ip -n "$NS_WAN" -6 route replace "$CPFX" via "$WAN6_GW"
  say "ns $NS_WAN: $(ip -n "$NS_WAN" -6 addr show dev "$WAN_PEER" | grep -E 'inet6 fd00' | xargs) · route: $(ip -n "$NS_WAN" -6 route show "$CPFX" | xargs)"
  say "ns $NS_LAN: $(ip -n "$NS_LAN" -4 addr show dev "$LAN_PEER" | grep -E 'inet ' | xargs) · route: $(ip -n "$NS_LAN" route show default | xargs)"
  # The rig creates unowned VPP ports; the declarative agent must create its
  # own tagged ports. Quiesce the Linux peers before handing over VPP sides.
  ip -n "$NS_LAN" link set "$LAN_PEER" down || return 1
  ip -n "$NS_WAN" link set "$WAN_PEER" down || return 1
  ip link set "${P}l0" down || return 1
  ip link set "${P}w0" down || return 1
  cat > "$RUN/handoff.go" <<'GO'
package main
import (
 "context"
 "fmt"
 "regexp"
 "syscall"
 "os"
 "strings"
 "time"
 "go.fd.io/govpp"
 "ngfw/agent/binapi/af_packet"
 interfaces "ngfw/agent/binapi/interface"
 "ngfw/agent/binapi/interface_types"
)
func main() {
 info,err:=os.Lstat("/run/vpp/startup.conf");if err!=nil{panic(err)}
 stat,ok:=info.Sys().(*syscall.Stat_t);if !ok||stat.Uid!=0||!info.Mode().IsRegular(){panic("private startup file identity refused")}
 marker,err:=os.ReadFile("/run/vpp/startup.conf");if err!=nil{panic(err)}
 ownns,e1:=os.Readlink("/proc/self/ns/mnt");rootns,e2:=os.Readlink("/proc/1/ns/mnt")
 if os.Getenv("NGFW_DISPOSABLE_VPP")!="1"||!regexp.MustCompile(`(?m)^api-segment \{ prefix fulltest[0-9]+ \}$`).Match(marker)||e1!=nil||e2!=nil||ownns==rootns{panic("private VPP marker/namespace refused before handoff")}
 c,err:=govpp.Connect("/run/vpp/api.sock");if err!=nil{panic(err)};defer c.Disconnect()
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
 client:=interfaces.NewServiceClient(c)
 stream,err:=client.SwInterfaceDump(ctx,&interfaces.SwInterfaceDump{});if err!=nil{panic(err)}
 targets:=map[string]interface_types.InterfaceIndex{}
 names:=[]string{os.Args[1]+"l0",os.Args[1]+"w0"}
 for {r,e:=stream.Recv();if e!=nil{if e.Error()!="EOF"{panic(e)};break};for _,n:=range names{if r.InterfaceName=="host-"+n{if strings.Trim(r.Tag,"\x00")!=""{panic("refuse tagged/nonfixture port: "+r.InterfaceName)};targets[n]=r.SwIfIndex}}}
 if len(targets)!=2{panic("both exact unowned rig ports required before handoff")}
 for _,n:=range names {idx:=targets[n];if _,e:=client.SwInterfaceAddDelAddress(ctx,&interfaces.SwInterfaceAddDelAddress{SwIfIndex:idx,DelAll:true});e!=nil{panic(e)};if _,e:=af_packet.NewServiceClient(c).AfPacketDelete(ctx,&af_packet.AfPacketDelete{HostIfName:n});e!=nil{panic(e)};fmt.Printf("handoff: removed unowned VPP host-%s index%d; Linux veth retained\n",n,idx)}
}
GO
  go -C "$ROOT/apps/agent" run "$RUN/handoff.go" "$P" 9>&- | tee -a "$LOG" || return 1
  nres "after step 2"
}

step3_agent() {
  say "=== step 3: agent apply nat.nat46 → Retrieve == canonical, idempotent re-apply, vppctl show map domain ==="
  nres "before step 3"
  rm -rf "$RUN/agent-state" "$RUN/agent.log"; start_agent; wait_agent || return 1
  doc_full > "$RUN/full.json"; doc_ifs_only > "$RUN/ifs.json"; doc_dup > "$RUN/dup.json"
  local result
  result=$("${CTL[@]}" apply "$RUN/full.json" -txn nat46-1) || die "apply transport failed"
  say "apply full.json: $(jq -c '{txnId,status,results:[.results[]?|{key,op,code}],errors}' <<< "$result")"
  jq -e '.status=="APPLY_STATUS_APPLIED" and ((.errors//[])|length)==0' <<< "$result" >/dev/null || die "configuration was not applied"
  local got; got=$(retrieve_nat)
  say "Retrieve nat: $got"
  say "canonical  : $CANON"
  [[ "$got" == "$CANON" ]] || die "Retrieve != canonical"
  say "Retrieve == canonical: OK"
  result=$("${CTL[@]}" apply "$RUN/full.json" -txn nat46-2) || die "re-apply transport failed"
  say "re-apply full.json: $(jq -c '{status,results}' <<< "$result")"
  jq -e '.status=="APPLY_STATUS_APPLIED" and ((.results//[])|length)==0' <<< "$result" >/dev/null || die "re-apply did not converge with empty plan"
  for side in l w; do ip link set "${P}${side}0" up || return 1; done
  ip -n "$NS_LAN" link set "$LAN_PEER" up || return 1
  ip -n "$NS_WAN" link set "$WAN_PEER" up || return 1
  # Bringing veths down withdraws their Linux routes/IPv6 state; restore the
  # fixture endpoints after ownership transfer, before sending any packets.
  ip -n "$NS_LAN" route replace default via "$LAN_GW" || return 1
  ip -n "$NS_WAN" route replace default via "$WAN_GW" || return 1
  ip -n "$NS_WAN" -6 addr replace "$LINK6/64" dev "$WAN_PEER" nodad
  ip -n "$NS_WAN" -6 addr replace "$SERVER6/128" dev lo nodad || return 1
  ip -n "$NS_WAN" -6 route replace "$CPFX" via "$WAN6_GW" || return 1
  "$BIN/ngfw-vpp-preflight" | tee -a "$LOG" || die "V19 preflight failed"
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
data=b""
while len(data)<len(b"ngfw-nat46-ok\n"):
    part=c.recv(64)
    if not part: break
    data+=part
assert data==b"ngfw-nat46-ok\n",repr(data)
print("client4: verified exact payload", data.decode().strip(), flush=True)
c.close()
EOF
  ip netns exec "$NS_WAN" python3 "$srv" "$SERVER6" "$PORT" > "$RUN/server6.txt" 2>&1 9>&- & PIDS+=("$!")
  ip netns exec "$NS_WAN" tcpdump -lni "$WAN_PEER" -c 6 "ip6 and tcp port $PORT" > "$RUN/tcpdump6.txt" 2>&1 9>&- & local td=$!; PIDS+=("$td")
  # Wait for capture readiness: a fixed sleep can miss all packets when the
  # shared host is busy loading tcpdump. An early exit must fail explicitly.
  local capture_ready=0
  for i in $(seq 1 120); do
    if grep -q 'listening on' "$RUN/tcpdump6.txt"; then capture_ready=1; break; fi
    kill -0 "$td" 2>/dev/null || die "IPv6 capture exited before readiness: $(cat "$RUN/tcpdump6.txt")"
    sleep 0.25
  done
  [[ "$capture_ready" == 1 ]] || die "IPv6 capture did not become ready within 30 seconds"
  local rx0 tx0
  rx0=$(V show interface "$LAN_IF" | awk '/rx packets/ {print $NF}' | head -1); tx0=$(V show interface "$WAN_IF" | awk '/tx packets/ {print $NF}' | head -1)
  say "ping $SVC4 from $NS_LAN (ICMP → ICMPv6 → ICMP):"
  # Warm neighbour resolution before strict three-request acceptance.
  ip netns exec "$NS_LAN" ping -c 1 -W 2 "$SVC4" >/dev/null 2>&1 || true
  local pingout
  pingout=$(ip netns exec "$NS_LAN" ping -c 3 -W 2 "$SVC4" 2>&1) || die "NAT46 ICMP round trip failed: $pingout"
  say "$pingout"
  grep -Eq '3 packets transmitted, 3 (packets )?received' <<< "$pingout" || die "ICMP did not receive all three replies"
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
  local elapsed; elapsed=$(awk -v a="$(date +%s.%N)" -v b="$t0" 'BEGIN{printf "%.2f", a-b}')
  awk -v elapsed="$elapsed" 'BEGIN{exit !(elapsed<=30)}' || die "restart exceeded 30 seconds ($elapsed)"
  [[ "$(show_domains | wc -l)" == 1 ]] || die "restart did not recreate exactly one owned MAP domain"
  say "Retrieve == canonical again after $(awk -v a="$(date +%s.%N)" -v b="$t0" 'BEGIN{printf "%.2f", a-b}') s (limit 30 s): $([[ "$got" == "$CANON" ]] && echo OK || echo "NOT YET: $got")"
  say "vppctl show map domain (ours, recreated):"; show_domains | tee -a "$LOG"
  say "agent log (reconcile/resync lines since the restart):"
  agent_log_since "$off" 'reconcile|resync|map\.domain|nat46' 300 | head -12 | tee -a "$LOG"
  local response
  response=$(ip netns exec "$NS_LAN" python3 "$RUN/client4.py" "$CLIENT" 46002 "$SVC4" "$PORT" 2>&1) || die "post-restart TCP failed: $response"
  say "tcp again after the restart: $response"
  nres "after step 5"
}

step6_dup() {
  say "=== step 6: duplicate IPv4 service address → dryrun refusal with pointer (agent level; no API stack on slot $N) ==="
  local result
  result=$("${CTL[@]}" dryrun "$RUN/dup.json") || die "duplicate dryrun transport failure"
  say "dryrun dup.json: $(jq -c '{ok,errors}' <<< "$result")"
  jq -e '(.ok//false)==false and any(.errors[]?; .pointer=="/nat/nat46/mappings/1/ipv4")' <<< "$result" >/dev/null || die "duplicate dryrun did not refuse with expected pointer"
  result=$("${CTL[@]}" apply "$RUN/dup.json" -txn nat46-dup) || die "duplicate apply transport failure"
  say "apply dup.json: $(jq -c '{status,errors}' <<< "$result")"
  jq -e '.status=="APPLY_STATUS_FAILED"' <<< "$result" >/dev/null || die "duplicate apply was not refused"
  [[ "$(retrieve_nat)" == "$CANON" ]] || die "invalid duplicate configuration changed state"
  say "Retrieve nat unchanged: OK"
}

step7_rollback() {
  say "=== step 7: rollback (nat: {}) → nothing left ==="
  nres "before step 7"
  local result; result=$("${CTL[@]}" apply "$RUN/ifs.json" -txn nat46-rb) || die "rollback transport failure"
  say "apply ifs.json: $(jq -c '{status,results}' <<< "$result")"
  jq -e '.status=="APPLY_STATUS_APPLIED"' <<< "$result" >/dev/null || die "rollback not applied"
  [[ "$(retrieve_nat)" == '{}' ]] || die "NAT remains in Retrieve after rollback"
  [[ "$(show_domains | wc -l)" == 0 ]] || die "MAP domains remain after rollback"
  say "map domains of $OWNER after rollback: 0"
  for iface in "$LAN_IF" "$WAN_IF"; do
    local features; features=$(V show interface features "$iface") || die "feature readback failed"
    ! grep -q 'map-t' <<< "$features" || die "MAP-T feature remains on $iface"
  done
  local fib; fib=$(V show ip6 fib "$SRV64") || die "route readback failed"
  ! grep -Fq "$SRV64" <<< "$fib" || die "owned server route remains after rollback"
  say "MAP-T features and owned server route removed"
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
