#!/usr/bin/env bash
# test/topology/wireguard/stack.sh — F-wireguard full-stack evidence run on one slot (host VPP, real processes):
#   the vrx-agent TEST build (-tags vrxtestsecrets: the slot-local secret fixture, PENDING-secret-channel), vrx-api on a
#   throwaway database, the production web build under `vite preview`, and a kernel WireGuard peer in a netns reached
#   through a tap. Steps: key pair through the API → commit → handshake (peer event → WS → UI) → state/vppctl evidence
#   → optional screenshots → simulated loss + agent restart → rollback → cleanup. Every process is stopped by PID.
#
#   eval "$(tools/lab env <slot>)"; test/topology/wireguard/stack.sh [<screenshot node script> <out dir>]
#
# Owner "<prefix>wg" (F-bonding shares slot 6), database vrx_<prefix>wg, API port 3000+100·slot+70, web 5000+100·slot+70,
# agent socket /run/vrx-test/<prefix>/wg/agent.sock, wg instance/table base+70, tap id slot·100+70, UDP 20000+100·slot+10/+11,
# 10.<slot>.70-72.0/24, loopback loop<slot>70. The screenshot script (kept outside the repo, P07a/P07b) is called as
# `node <script> <webUrl> <outDir> <adminPasswordFile>`. Output: $RUN/evidence.log (key material never written there).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
: "${VRX_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
: "${VRX_SLOT:?eval \"\$(tools/lab env <slot>)\" first}"
P=$VRX_TEST_PREFIX N=$VRX_SLOT OWNER=${VRX_TEST_PREFIX}wg
BASE=$((N * 1000)) INST=$((N * 1000 + 70)) TAPID=$((N * 100 + 70))
API_PORT=$((3000 + 100 * N + 70)) WEB_PORT=$((5000 + 100 * N + 70))
VPP_PORT=$((20000 + 100 * N + 10)) KERN_PORT=$((20000 + 100 * N + 11))
NS=ns-$OWNER TAP=tap$TAPID HOSTIF=$OWNER-t0 KIF=$OWNER-k0
RUN=/run/vrx-test/$P/wg
SHOTS=${1:-} SHOTS_OUT=${2:-}
[[ -d /run/vrx-test/$P ]] || install -d -m 0755 /run/vrx-test/$P
rm -rf "$RUN"; install -d -m 0700 "$RUN"
LOG=$RUN/evidence.log
say() { printf '%s %s\n' "$(date +%T)" "$*" | tee -a "$LOG"; }
redact() { python3 -c '
import re,sys
for l in sys.stdin:
    l=re.sub(r"(private[- ]key\s*:?)\s*.*", r"\1 <redacted>", l.rstrip("\n"), flags=re.I)
    l=re.sub(r"(mac[- ]key\s*:?)\s*.*", r"\1 <redacted>", l, flags=re.I)
    l=re.sub(r"(pre-shared key\s*:?)\s*.*", r"\1 <redacted>", l, flags=re.I)
    l=re.sub(r"\b[0-9a-f]{64}\b", "<hex-redacted>", l)
    print(l)'; }
PIDS=()
cleanup() {
  set +e
  say "cleanup"
  if [[ -n "${TOKEN:-}" ]]; then
    api PATCH /api/v1/config/vpn '{"wireguard":{"interfaces":null}}' >/dev/null
    api PATCH /api/v1/config/interfaces "{\"loop$TAPID\":null}" >/dev/null
    api POST "/api/v1/config/commit?comment=wg-stack-cleanup" | jq -c '{status,revision:.revision.id}' | tee -a "$LOG"
  fi
  for ((i=${#PIDS[@]}-1; i>=0; i--)); do kill "${PIDS[$i]}" 2>/dev/null; wait "${PIDS[$i]}" 2>/dev/null; done
  ip netns del "$NS" 2>/dev/null
  vppctl delete tap "$TAP" >/dev/null 2>&1
  "$ROOT/deploy/dev/pg-test.sh" drop "$OWNER" >/dev/null 2>&1
  rm -f "$RUN/secrets.json" "$RUN/kernel.key" "$RUN/admin.pw"
  say "VPP $(systemctl show vpp -p NRestarts) after; leftovers: $(vppctl show interface | grep -cE "^(wg$INST|$TAP) ") interface(s)"
}
trap cleanup EXIT

say "VPP $(systemctl show vpp -p NRestarts) before"
( cd "$ROOT/apps/agent" && go build -tags vrxtestsecrets -o bin/vrx-agent-wgtest ./cmd/vrx-agent && go build -o bin/vrx-vpp-preflight ./cmd/vrx-vpp-preflight )
( cd "$ROOT" && pnpm --filter @ngfw/api build >/dev/null && pnpm --filter @ngfw/web build >/dev/null )

# test vectors (00-CONTEXT: VRX_TEST_PSK_<id> labels) — the same values go to the agent fixture and to the API's store
vec() { python3 -c "import hashlib,base64,sys; print(base64.b64encode(hashlib.sha256(('VRX_TEST_PSK_F-wireguard_'+sys.argv[1]).encode()).digest()).decode())" "$1"; }
ITF_KEY=$(vec "${OWNER}_stack_itf") KERN_KEY=$(vec "${OWNER}_stack_kernel") PSK=$(vec "${OWNER}_stack_psk")
( umask 077
  printf '{"key/%s-site":"%s","psk/%s-k":"%s"}\n' "$OWNER" "$ITF_KEY" "$OWNER" "$PSK" > "$RUN/secrets.json"
  printf '%s\n' "$KERN_KEY" > "$RUN/kernel.key" )
KERN_PUB=$(wg pubkey < "$RUN/kernel.key") VPP_PUB=$(printf '%s' "$ITF_KEY" | wg pubkey)

# processes
"$ROOT/deploy/dev/pg-test.sh" create "$OWNER" >/dev/null
DSN=$(sed -n 's/^VRX_PG_DSN=//p' "/run/vrx-test/$OWNER/pg.env")
env -i PATH="$PATH" HOME="$HOME" VRX_OWNER="$OWNER" VRX_GLOBALS_OWNER=0 VRX_AGENT_SOCKET="$RUN/agent.sock" \
  VRX_AGENT_STATE_DIR="$RUN/agent-state" VRX_METRICS_ADDR=off VRX_SOCKET_GROUP=root VRX_LOG_LEVEL=info \
  VRX_VPP_TABLE_BASE="$BASE" VRX_TEST_WG_SECRETS="$RUN/secrets.json" \
  "$ROOT/apps/agent/bin/vrx-agent-wgtest" >> "$RUN/agent.log" 2>&1 & AGENT=$!; PIDS+=("$AGENT")
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
apij() { local m=$1 p=$2 b=${3:-}; curl -s -X "$m" -H "authorization: Bearer $TOKEN" ${b:+-H 'content-type: application/json' -d "$b"} "http://127.0.0.1:$API_PORT$p"; }
say "agent pid $AGENT, API :$API_PORT up; health: $(apij GET /api/v1/health | jq -c '{status}')"

# the kernel peer's side (a test fixture, not agent configuration): a tap into $NS with 10.<slot>.70.1/24 on the VPP side,
# and the V19 pre-flight before any packet crosses it (envelope V19/V24). The agent owns the listen address: a loopback
# loop<slot>70 with 10.<slot>.72.1/32, reached from $NS through the tap.
ip netns add "$NS"
vppctl create tap id "$TAPID" host-if-name "$HOSTIF" host-ns "$NS" host-ip4-addr "10.$N.70.2/24" >/dev/null
vppctl set interface ip address "$TAP" "10.$N.70.1/24"
vppctl set interface state "$TAP" up
ip -n "$NS" route add "10.$N.72.1/32" via "10.$N.70.1"
"$ROOT/apps/agent/bin/vrx-vpp-preflight" | tail -1 | tee -a "$LOG"
[[ ${PIPESTATUS[0]} -eq 0 ]] || { say "V19 pre-flight failed: no traffic"; exit 1; }
ip -n "$NS" link add "$KIF" type wireguard
ip netns exec "$NS" wg set "$KIF" private-key "$RUN/kernel.key" listen-port "$KERN_PORT" peer "$VPP_PUB" preshared-key <(printf '%s\n' "$PSK") \
  allowed-ips "10.$N.71.1/32" endpoint "10.$N.72.1:$VPP_PORT" persistent-keepalive 5
ip -n "$NS" addr add "10.$N.71.2/24" dev "$KIF"; ip -n "$NS" link set "$KIF" up

# the API side: secrets (the same test vectors the agent fixture holds), a server key pair (the action), the config
apij POST /api/v1/secrets "{\"kind\":\"key\",\"name\":\"$OWNER-site\",\"value\":\"$ITF_KEY\"}" | jq -c . | tee -a "$LOG"
apij POST /api/v1/secrets "{\"kind\":\"psk\",\"name\":\"$OWNER-k\",\"value\":\"$PSK\"}" | jq -c . | tee -a "$LOG"
say "keypair action: $(apij POST /api/v1/actions/vpn/wireguard/keypair "{\"name\":\"$OWNER-spare\"}" | jq -c .)"
# baseline revision: the listen address on an agent loopback (vpn.local-address-configured), without WireGuard
api PATCH /api/v1/config/interfaces "{\"loop$TAPID\":{\"enabled\":true,\"ipv4\":[\"10.$N.72.1/32\"]}}" >/dev/null
say "baseline commit: $(api POST "/api/v1/config/commit?comment=wg-stack-base" | jq -c '{status,revision:.revision.id}')"
api PATCH /api/v1/config/vpn "{\"wireguard\":{\"interfaces\":{\"site\":{\"description\":\"F-wireguard stack run\",\"instance\":$INST,
  \"listenAddress\":\"10.$N.72.1\",\"listenPort\":$VPP_PORT,\"privateKeyRef\":\"key/$OWNER-site\",\"address\":[\"10.$N.71.1/24\"],
  \"routeAllowedIps\":true,\"peers\":{\"kernel\":{\"description\":\"wg in $NS\",\"publicKey\":\"$KERN_PUB\",\"presharedKeyRef\":\"psk/$OWNER-k\",
  \"endpoint\":{\"address\":\"10.$N.70.2\",\"port\":$KERN_PORT},\"allowedIps\":[\"10.$N.71.2/32\"],\"persistentKeepaliveSec\":5},
  \"laptop\":{\"description\":\"road warrior (offline)\",\"publicKey\":\"HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw=\",\"allowedIps\":[\"10.$N.71.10/32\"]}}}}}}" >/dev/null
say "commit: $(api POST "/api/v1/config/commit?comment=wg-stack" | jq -c '{status,revision:.revision.id,errors}')"
for _ in $(seq 40); do st=$(apij GET /api/v1/state/vpn/wireguard); [[ $(jq -r '.interfaces[0].peers[]?|select(.name=="kernel")|.status' <<<"$st") == established ]] && break; sleep 0.5; done
say "state: $(jq -c . <<<"$st")"
say "ping through the tunnel: $(ip netns exec "$NS" ping -c 3 -W 2 "10.$N.71.1" | tail -2 | tr '\n' ' ')"
{ echo "--- vppctl show wireguard interface (redacted)"; vppctl show wireguard interface | grep -A3 "wg$INST" | redact
  echo "--- vppctl show wireguard peer (redacted)"; vppctl show wireguard peer | redact | grep -A5 "wg$INST"
  echo "--- vppctl show interface wg$INST"; vppctl show interface "wg$INST"
  echo "--- vppctl show interface address wg$INST"; vppctl show interface address "wg$INST"
  echo "--- vppctl show ip fib 10.$N.71.2/32"; vppctl show ip fib "10.$N.71.2/32" | head -12
  echo "--- wg show (kernel peer; public values only)"; ip netns exec "$NS" wg show "$KIF" | grep -v -i 'private' ; } >> "$LOG"
say "drift: $(apij GET /api/v1/state/drift | jq -c '{changes:[.changes[]|select(.pointer|startswith("/vpn"))]}')"
say "duplicate public key: $(api PATCH /api/v1/config/vpn "{\"wireguard\":{\"interfaces\":{\"dup\":{\"instance\":$((INST+1)),\"listenAddress\":\"10.$N.72.1\",\"listenPort\":$KERN_PORT,\"privateKeyRef\":\"key/$OWNER-spare\",\"peers\":{\"again\":{\"publicKey\":\"$KERN_PUB\",\"allowedIps\":[\"10.$N.72.0/24\"]}}}}}}" >/dev/null; api POST /api/v1/config/commit | jq -c '{status,type,errors}')"
api POST /api/v1/config/discard >/dev/null

if [[ -n "$SHOTS" ]]; then
  env VRX_HTTP_PORT="$API_PORT" VRX_WEB_PORT="$WEB_PORT" "$ROOT/apps/web/node_modules/.bin/vite" preview "$ROOT/apps/web" >> "$RUN/vite.log" 2>&1 & PIDS+=("$!")
  for _ in $(seq 60); do curl -sf -o /dev/null "http://127.0.0.1:$WEB_PORT/" && break; sleep 0.5; done
  say "screenshots: $(node "$SHOTS" "http://127.0.0.1:$WEB_PORT" "$SHOTS_OUT" "$RUN/admin.pw" 2>&1 | tr '\n' ' ')"
fi

# restart safety: stop the agent, delete the WireGuard objects behind its back (CLI = the binapi calls), start it again
kill "$AGENT"; wait "$AGENT" 2>/dev/null || true
for idx in $(vppctl show wireguard peer | sed -n "s/^\[\([0-9]*\)\].* wg$INST .*/\1/p"); do vppctl wireguard peer remove "$idx"; done
vppctl wireguard delete "wg$INST"
say "simulated loss: wg$INST present? $(vppctl show interface | grep -c "^wg$INST ")"
MARK=$(wc -c < "$RUN/agent.log")
T0=$(date +%s.%N)
env -i PATH="$PATH" HOME="$HOME" VRX_OWNER="$OWNER" VRX_GLOBALS_OWNER=0 VRX_AGENT_SOCKET="$RUN/agent.sock" \
  VRX_AGENT_STATE_DIR="$RUN/agent-state" VRX_METRICS_ADDR=off VRX_SOCKET_GROUP=root VRX_LOG_LEVEL=info \
  VRX_VPP_TABLE_BASE="$BASE" VRX_TEST_WG_SECRETS="$RUN/secrets.json" \
  "$ROOT/apps/agent/bin/vrx-agent-wgtest" >> "$RUN/agent.log" 2>&1 & AGENT=$!; PIDS+=("$AGENT")
for _ in $(seq 60); do st=$(apij GET /api/v1/state/vpn/wireguard); [[ $(jq '.interfaces[0].peers|length? // 0' <<<"$st" 2>/dev/null) == 2 ]] && break; sleep 0.5; done
say "restart: interface and $(jq '.interfaces[0].peers|length? // 0' <<<"$st") peers back after $(python3 -c "import time;print(round(time.time()-$T0,2))") s (agent pid $AGENT)"
tail -c +"$((MARK + 1))" "$RUN/agent.log" | grep -E 'reconcile (start|done)|subsystems wired|dynamic|TEST BUILD' | cut -c1-400 >> "$LOG"
for _ in $(seq 40); do [[ $(apij GET /api/v1/state/vpn/wireguard | jq -r '.interfaces[0].peers[]?|select(.name=="kernel")|.status') == established ]] && break; sleep 0.5; done
say "after restart: kernel peer $(apij GET /api/v1/state/vpn/wireguard | jq -c '.interfaces[0].peers[]?|select(.name=="kernel")|{status,lastHandshake,endpoint}')"

# rollback to the revision before the WireGuard commit
REV=$(apij GET "/api/v1/config/revisions?limit=10" | jq -r '[.items[]|select(.comment=="wg-stack-base")][0].id')
say "rollback to $REV (wg-stack-base): $(api POST "/api/v1/config/rollback/$REV" | jq -c '{status,revision:{id:.revision.id,kind:.revision.kind}}')"
say "after rollback: state $(apij GET /api/v1/state/vpn/wireguard | jq -c '.interfaces') · vppctl wg interfaces of $OWNER: $(vppctl show interface | grep -c "^wg$INST ")"
say "done"
