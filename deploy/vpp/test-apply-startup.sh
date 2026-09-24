#!/usr/bin/env bash
# shellcheck disable=SC1090,SC2016,SC2034,SC2119,SC2120  # apply() takes optional extra flags; ok() evaluates its single-quoted condition later (uses $out, $rc, $W, …); scenarios source the script under test
# deploy/vpp/test-apply-startup.sh — exercises apply-startup.sh against a FAKE host: fake systemctl (MainPID,
# ActiveEnterTimestampMonotonic, NRestarts reset on restart — as systemd does), systemd-run, vrx-vppcheck
# (incl. the D-080 boot identity), ip, ss, ping (a gateway that may drop ICMP), a TCP prober, driverctl,
# ifup / networkctl / netplan, logger, lslocks, a fake sysfs tree, a fake /etc (network configuration only),
# temp locks, a temp startup.conf and a fixture "canonical repo" (VRX_TEST_ROOT/canon: handover flag, PENDING
# files, LOG.md on branch main). Nothing on the real host is read or changed (no VPP, no /etc, no /sys, no real
# locks, not /root/ngfw).
#
#   deploy/vpp/test-apply-startup.sh [path/to/vrx-startupgen]
#
# Without an argument the generator is built from apps/agent into a temp dir. Every process a scenario
# starts is killed by PID (never by pattern).
#   VRX_TEST_ONLY="8 32"        run only these scenarios (numbers as printed)
#   VRX_TEST_SHARD=i/n          run scenario N only when (N-1) % n == i-1 (tools/ci.sh runs n shards in parallel)
#   VRX_TEST_APPLY_SCRIPT=path  exercise another copy of apply-startup.sh (e.g. main's, to show a scenario failing before a fix)
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
SCRIPT="${VRX_TEST_APPLY_SCRIPT:-$HERE/apply-startup.sh}"
scen() {  # <N> → is scenario N selected?
  local shard="${VRX_TEST_SHARD:-1/1}"
  [[ -z ${VRX_TEST_ONLY:-} || " $VRX_TEST_ONLY " == *" $1 "* ]] || return 1
  [[ $shard =~ ^([0-9]+)/([0-9]+)$ ]] || { echo "VRX_TEST_SHARD must be i/n" >&2; exit 2; }
  (( ($1 - 1) % BASH_REMATCH[2] == BASH_REMATCH[1] - 1 ))
}
FIX="$REPO/apps/agent/internal/renderers/vppstartup/testdata"
TOP="$(mktemp -d)"
PIDS=()
export VRX_TEST_ROOT="$TOP"
# fixture canonical repo: host flag pending; PENDING-fake-change answered by D-900 (subject) naming the rendering's sha256
# (row added by approve_rendering once the sum is known); PENDING-closed answered by D-901 for another change; PENDING-
# mentioned that a D-row only mentions in passing
CANON="$TOP/canon"
mkdir -p "$CANON/docs/lab" "$CANON/docs/decisions"
printf '# vrx-a\n\n`handover: pending`\n' > "$CANON/docs/lab/host-vrx-a.md"
printf '# PENDING: fake-change\n\n- raised: 2026-09-24 by test\n- decision: **option 1** — product owner (D-900)\n' > "$CANON/docs/decisions/PENDING-fake-change.md"
printf '# PENDING: closed\n\n- raised: 2026-09-20 by test\n- decision: **option 1** — product owner; executed 2026-09-21\n' > "$CANON/docs/decisions/PENDING-closed.md"
printf '# PENDING: mentioned\n\n- raised: 2026-09-24 by test\n- decision: pending\n' > "$CANON/docs/decisions/PENDING-mentioned.md"
{
  echo '| date | id | decision | options considered | why | reversal cost | tasks |'
  echo '|---|---|---|---|---|---|---|'
  echo "| 2026-09-21 | D-901 | PENDING-closed answered by the product owner: option 1, rendering $(printf '%064d' 7) (executed 2026-09-21) | — | — | low | x |"
  echo '| 2026-09-24 | D-902 | Unrelated decision; see also PENDING-mentioned for background | — | — | low | x |'
} > "$CANON/docs/decisions/LOG.md"
git -C "$CANON" init -q -b main && git -C "$CANON" add -A && git -C "$CANON" -c user.name=test -c user.email=test@invalid commit -qm fixture
approve_rendering() {  # <sha256> — D-900 (subject PENDING-fake-change) approves exactly this rendering
  grep -qF "$1" "$CANON/docs/decisions/LOG.md" && return 0
  echo "| 2026-09-24 | D-900 | **PENDING-fake-change** answered by the product owner: apply rendering $1 once | — | — | low | F-startup-apply |" >> "$CANON/docs/decisions/LOG.md"
  git -C "$CANON" -c user.name=test -c user.email=test@invalid commit -qam "approve $1"
}
kill_recorded() { local p; for p in $(cat "$TOP"/case.*/state/pids 2>/dev/null || true); do kill -KILL "$p" 2>/dev/null || true; done; }
kill_recorded_case() { local p; for p in $(cat "$T/state/pids" 2>/dev/null || true); do kill -KILL "$p" 2>/dev/null || true; done; }   # this case's units + hooks
cleanup() {
  local p
  for p in "${PIDS[@]}"; do kill -KILL "$p" 2>/dev/null || true; done
  kill_recorded
  if [[ -n ${KEEP:-} ]]; then echo "kept $TOP"; else rm -rf "$TOP"; fi
}
trap cleanup EXIT

GEN="${1:-}"
if [[ -z $GEN ]]; then
  GEN="$TOP/vrx-startupgen"
  (cd "$REPO/apps/agent" && go build -o "$GEN" ./cmd/vrx-startupgen)
fi

PASS=0 FAIL=0
ok() {
  if eval "$1"; then PASS=$((PASS + 1)); echo "  ok   $2"; return; fi
  FAIL=$((FAIL + 1)); echo "  FAIL $2"
  # a failure explains itself: the verdict lines the script printed in this case + the host load (CI shares the host)
  echo "       load $(cut -d' ' -f1-3 /proc/loadavg) on $(nproc) CPUs; last verdicts in $T:"
  grep -hE 'ROLLBACK|REFUSED|CONSOLE NEEDED|SUPERSEDED|COMMITTED|FORCED|failed or hung|NONE VIABLE|took the locks|still owns' \
    "$T"/out "$T"/run.out "$T"/run1.out 2>/dev/null | tail -n 6 | cut -c1-220 | sed 's/^/       | /' || true
}
elapsed() { echo $((SECONDS - T0)); }
# "bounded" checks prove a hang costs the configured timeouts, not the fake's sleep (300–1000 s). Everything else in
# a scenario is fork-heavy shell whose wall time grows with the host's load (CI runs next to up to 11 workers; measured:
# 3.6× the idle time at load 32 on 32 CPUs), so the limit is the idle-host limit × a load factor 1 + 3·min(load/CPUs, 1.6):
# 1 idle, 4 at load = CPUs, 5.8 at load ≥ 1.6×CPUs (51 on 32). Idle-host limits are ~1.5–2× the idle time; even ×5.8
# every limit stays below the unbounded case it guards against (named in the check).
load_limit() {  # <idle-host limit in s> → the limit for the current load
  awk -v b="$1" -v l="$(cut -d' ' -f1 /proc/loadavg)" -v n="$(nproc)" 'BEGIN { r = l / n; if (r > 1.6) r = 1.6; printf "%d\n", b * (1 + 3 * r) + 0.5 }'
}
bounded() {  # <idle-host limit> <what an unbounded wait would cost> — one check line with the numbers
  local e lim; e=$(elapsed); lim=$(load_limit "$1")
  ok "(( $e <= $lim ))" "bounded: ${e}s (limit ${lim}s = ${1}s idle-host limit × load factor at load $(cut -d' ' -f1 /proc/loadavg); unbounded: $2)"
}
foreign_lock() {  # <lock file> — a foreign process holds flock -x on it until killed ($HOLDER); returns once it is held
  # shellcheck disable=SC2016  # $0 expands in the child
  bash -c 'exec 9>"$0"; flock -x 9; exec sleep 300' "$1" & HOLDER=$!; PIDS+=("$HOLDER")
  local i; for ((i = 0; i < 200; i++)); do flock -n -x "$1" true 2>/dev/null || return 0; sleep 0.05; done
  echo "foreign_lock: $1 not held after 10s" >&2; return 1
}

# ---------------------------------------------------------------- fake host
setup() {
  kill_recorded
  T="$(mktemp -d "$TOP/case.XXXX")"
  export FAKE="$T"
  mkdir -p "$T/bin" "$T/state" "$T/hooks" "$T/etc/vpp" "$T/etc/network/interfaces.d" "$T/plugins" "$T/sys/class/net/ens192" "$T/apply" "$T/tmp"
  for d in vmxnet3 vfio-pci; do mkdir -p "$T/sys/bus/pci/drivers/$d"; : > "$T/sys/bus/pci/drivers/$d/bind"; : > "$T/sys/bus/pci/drivers/$d/unbind"; done
  for p in 0000:0b:00.0 0000:04:00.0 0000:0c:00.0 0000:13:00.0 0000:14:00.0 0000:1b:00.0 0000:1c:00.0; do
    mkdir -p "$T/sys/bus/pci/devices/$p"; : > "$T/sys/bus/pci/devices/$p/driver_override"
    ln -s "$T/sys/bus/pci/drivers/vmxnet3" "$T/sys/bus/pci/devices/$p/driver"
  done
  ln -s "$T/sys/bus/pci/devices/0000:0b:00.0" "$T/sys/class/net/ens192/device"
  xargs -I{} touch "$T/plugins/{}" < "$FIX/plugins-vrx-a.txt"
  cp "$FIX/host-startup.conf" "$T/etc/vpp/startup.conf"
  # vrx-a: ifupdown owns ens192 (docs/lab/host-vrx-a.md)
  printf 'auto ens192\niface ens192 inet static\n  address 10.0.0.5/24\n  gateway 10.0.0.1\n' > "$T/etc/network/interfaces.d/ens192.cfg"
  python3 -c 'import socket,sys; socket.socket(socket.AF_UNIX).bind(sys.argv[1])' "$T/api.sock"
  echo active > "$T/state/vpp"; echo 4 > "$T/state/nrestarts"; echo 0 > "$T/state/ping"; : > "$T/state/pids"
  echo 1000 > "$T/state/mainpid"; echo 5000 > "$T/state/start"; echo 77000 > "$T/state/active_enter"
  printf '%s\n' dpdk_plugin.so linux_cp_plugin.so linux_nl_plugin.so npt66_plugin.so acl_plugin.so > "$T/state/plugins"
  printf '%s\n' local0 wan lan dmz p2p lan2 sync > "$T/state/ifaces"
  printf '%s\n' "10.0.0.5 24 10.0.0.255" "2001:db8::5 64 -" > "$T/state/addrs"
  touch "$T/state/route-default"
  printf 'handover: pending\n' > "$T/host-pending.md"

  # systemctl: a (re)start gives vpp a new MainPID/start time/ActiveEnter and RESETS NRestarts (systemd does)
  cat > "$T/bin/systemctl" <<'EOF'
#!/usr/bin/env bash
echo "systemctl $*" >> "$FAKE/calls"
S="$FAKE/state"
newpid() { local n; n=$(($(cat "$S/lastpid" 2>/dev/null || echo 1000) + 1)); echo "$n" > "$S/lastpid"; echo "$n" > "$S/mainpid"; echo $(($(cat "$S/start") + 100)) > "$S/start"; echo $(($(cat "$S/active_enter") + 1000)) > "$S/active_enter"; }
case "$1" in
  is-active) if [[ $3 == systemd-networkd ]]; then [[ -e $S/networkd ]]; exit; fi; [[ $(cat "$S/vpp") == active ]] ;;
  show) if [[ -e $S/show-fail ]]; then [[ $(cat "$S/show-fail") == always ]] || rm -f "$S/show-fail"; echo "Failed to get properties: Connection timed out" >&2; exit 1; fi   # D-Bus timeout
        if [[ " $* " == *" -p MainPID "* ]]; then echo "MainPID=$(cat "$S/mainpid")"; echo "ActiveEnterTimestampMonotonic=$(cat "$S/active_enter")"; echo "NRestarts=$(cat "$S/nrestarts")"; else cat "$S/nrestarts"; fi ;;
  restart|start) if [[ -x $FAKE/hooks/$1 ]]; then "$FAKE/hooks/$1"; fi
                 echo active > "$S/vpp"; echo 0 > "$S/nrestarts"; [[ -e $S/no-restart ]] || newpid
                 if [[ $1 == restart && -e $S/crash-at-restart ]]; then   # dies at once; Restart=always is done before anyone looks
                   rm "$S/crash-at-restart"; echo $(($(cat "$S/mainpid") + 50)) > "$S/mainpid"; echo $(($(cat "$S/start") + 7)) > "$S/start"
                   echo $(($(cat "$S/active_enter") + 9)) > "$S/active_enter"; echo 1 > "$S/nrestarts"; fi ;;
  stop) if [[ $2 == vpp ]]; then if [[ -x $FAKE/hooks/stop ]]; then "$FAKE/hooks/stop"; fi
          echo inactive > "$S/vpp"; echo 0 > "$S/mainpid"; rm -f "$S/hang" "$S/hang-after" "$S/crash-after"; fi ;;
  kill) if [[ ${*: -1} == vpp ]]; then echo inactive > "$S/vpp"; fi ;;
esac
exit 0
EOF
  # systemd-run: a timer (--on-active) is only recorded; a unit is started like systemd does it —
  # its own session, a clean environment plus exactly the --setenv values
  cat > "$T/bin/systemd-run" <<'EOF'
#!/usr/bin/env bash
echo "systemd-run $*" >> "$FAKE/calls"
if [[ " $* " == *" --on-active="* ]]; then [[ ! -e $FAKE/state/timer-fail ]]; exit; fi
[[ ! -e $FAKE/state/systemd-run-fail ]] || exit 1
envs=()
while [[ $1 == --* ]]; do case "$1" in --setenv=*) envs+=("${1#--setenv=}") ;; esac; shift; done
setsid env -i "${envs[@]}" "$@" </dev/null >/dev/null 2>&1 &
echo $! >> "$FAKE/state/pids"
EOF
  cat > "$T/bin/vrx-vppcheck" <<'EOF'
#!/usr/bin/env bash
while [[ $1 == --* ]]; do shift 2; done
echo "vppcheck $*" >> "$FAKE/calls"
S="$FAKE/state"
if [[ -e $S/hang-after ]]; then
  n=$(cat "$S/hang-after"); if ((n <= 0)); then touch "$S/hang"; else echo $((n - 1)) > "$S/hang-after"; fi
fi
if [[ -e $S/crash-after ]]; then   # VPP crashes; systemd (Restart=always) brings it back with a new PID
  n=$(cat "$S/crash-after"); if ((n <= 0)); then rm "$S/crash-after"; echo $(($(cat "$S/mainpid") + 50)) > "$S/mainpid"
    echo $(($(cat "$S/start") + 7)) > "$S/start"; echo $(($(cat "$S/active_enter") + 9)) > "$S/active_enter"; echo 1 > "$S/nrestarts"
  else echo $((n - 1)) > "$S/crash-after"; fi
fi
[[ ! -e $S/hang ]] || exec sleep 1000          # VPP accepts the socket and never answers
[[ $(cat "$S/vpp") == active ]] || { echo "vrx-vppcheck: cannot reach VPP" >&2; exit 2; }
case "$1" in
  version) echo "vpp 26.06-release" ;;
  bootid) echo "fake-boot/$(cat "$S/mainpid")/$(cat "$S/start")" ;;
  plugins) cat "$S/plugins" ;;
  ifaces) shift; miss=(); for n in "$@"; do grep -qx "$n" "$S/ifaces" || miss+=("$n"); done
          if ((${#miss[@]})); then echo "missing: ${miss[*]}"; exit 1; fi; echo "present: $*" ;;
esac
EOF
  # ip: a JSON-speaking model of ens192 (addresses in state/addrs, default route = state/route-default).
  # `link set up` also plays the kernel: a NIC written to a driver's bind file is bound to it.
  cat > "$T/bin/ip" <<'EOF'
#!/usr/bin/env bash
echo "ip $*" >> "$FAKE/calls"
S="$FAKE/state"
addr_json() {
  local sep="" l p b
  printf '[{"ifindex":3,"ifname":"ens192","flags":["BROADCAST","MULTICAST","UP","LOWER_UP"],"addr_info":['
  while read -r l p b; do
    [[ -n $l ]] || continue
    if [[ $l == *:* ]]; then printf '%s{"family":"inet6","local":"%s","prefixlen":%s,"scope":"global"}' "$sep" "$l" "$p"
    else printf '%s{"family":"inet","local":"%s","prefixlen":%s,"broadcast":"%s","scope":"global","label":"ens192"}' "$sep" "$l" "$p" "$b"; fi
    sep=,
  done < "$S/addrs"
  printf '%s{"family":"inet6","local":"fe80::250:56ff:fe9f:1e53","prefixlen":64,"scope":"link","protocol":"kernel_ll"}]}]\n' "$sep"
}
case "$*" in
  "-j -4 route show default") if [[ -e $S/route-default ]]; then echo '[{"dst":"default","gateway":"10.0.0.1","dev":"ens192","flags":["onlink"]}]'; else echo '[]'; fi ;;
  "-j -6 route show default") if [[ -e $S/v6-default ]]; then echo '[{"dst":"default","gateway":"fe80::1","dev":"ens192","protocol":"ra","metric":1024,"flags":[],"pref":"medium"}]'; else echo '[]'; fi ;;
  "-j neigh show "*) [[ ! -e $S/neigh-hang ]] || exec sleep 30 ;;&
  "-j neigh show fe80::1 dev ens192") if [[ -e $S/nudged-fe80 ]]; then st=REACHABLE; else st=STALE; fi; echo "[{\"dst\":\"fe80::1\",\"state\":[\"$st\"]}]" ;;
  "-j route get 127."*) echo "[{\"dst\":\"$4\",\"dev\":\"lo\",\"flags\":[]}]" ;;
  "-j neigh show 10.0.0.1 dev ens192") if [[ -e $S/route-default && ! -e $S/arp-dead ]] && grep -q '^10.0.0.5 ' "$S/addrs"; then st=REACHABLE; else st=FAILED; fi
                                        echo "[{\"dst\":\"10.0.0.1\",\"state\":[\"$st\"]}]" ;;
  "-j route get "*) echo "[{\"dst\":\"$4\",\"dev\":\"ens192\",\"prefsrc\":\"10.0.0.5\",\"flags\":[],\"uid\":0,\"cache\":[]}]" ;;
  "-j addr show dev ens192") addr_json ;;
  "-j -4 route show table main dev ens192")
    r=(); [[ -e $S/route-default ]] && r+=('{"dst":"default","gateway":"10.0.0.1","flags":["onlink"]}')
    grep -q '^10.0.0.5 ' "$S/addrs" && r+=('{"dst":"10.0.0.0/24","protocol":"kernel","scope":"link","prefsrc":"10.0.0.5","flags":[]}')
    (IFS=,; echo "[${r[*]}]") ;;
  "-j -6 route show table main dev ens192") if [[ -e $S/v6-default ]]; then echo '[{"dst":"default","gateway":"fe80::1","protocol":"ra","metric":1024,"flags":[],"pref":"medium"},{"dst":"fe80::/64","protocol":"kernel","metric":256,"flags":[],"pref":"medium"}]'
                                             else echo '[{"dst":"fe80::/64","protocol":"kernel","metric":256,"flags":[],"pref":"medium"}]'; fi ;;
  "-o link show dev ens192") echo "3: ens192: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 state UP" ;;
  "link set dev ens192 up") d="$FAKE/sys/bus/pci"; for drv in vmxnet3 vfio-pci; do
                              if grep -qx 0000:0b:00.0 "$d/drivers/$drv/bind"; then ln -sfn "$d/drivers/$drv" "$d/devices/0000:0b:00.0/driver"; fi; done ;;
  "addr replace "*) [[ -e $S/ip-readonly ]] || { a="${3%/*}" p="${3#*/}" b=-; [[ $4 == broadcast ]] && b=$5
                    grep -q "^$a " "$S/addrs" || echo "$a $p $b" >> "$S/addrs"; } ;;
  "-4 route replace default via 10.0.0.1 dev ens192 onlink") [[ -e $S/ip-readonly ]] || touch "$S/route-default" ;;
esac
exit 0
EOF
  # the gateway answers ICMP unless state/no-icmp (vrx-a's gateway does not); TCP / the manager's session
  # work while the management path (address + default route) is there
  cat > "$T/bin/ping" <<'EOF'
#!/usr/bin/env bash
echo "ping $*" >> "$FAKE/calls"
[[ -e $FAKE/state/route-default && ! -e $FAKE/state/no-icmp ]] || exit 1
exit "$(cat "$FAKE/state/ping")"
EOF
  cat > "$T/bin/tcpconnect" <<'EOF'
#!/usr/bin/env bash
echo "tcpconnect $*" >> "$FAKE/calls"
case "$1" in fe80::1%ens192) touch "$FAKE/state/nudged-fe80"; exit 1 ;; fe80::*) exit 1 ;; esac   # no scope: connect() fails, nothing is sent
[[ -e $FAKE/state/route-default && ! -e $FAKE/state/tcp-down ]] && grep -q '^10.0.0.5 ' "$FAKE/state/addrs"
EOF
  cat > "$T/bin/ss" <<'EOF'
#!/usr/bin/env bash
echo "ss $*" >> "$FAKE/calls"
if [[ -e $FAKE/state/ssh-est && -e $FAKE/state/route-default ]] && grep -q '^10.0.0.5 ' "$FAKE/state/addrs"; then
  echo "0 0 10.0.0.5:22 ${*: -1}:51234"
fi
EOF
  cat > "$T/bin/lslocks" <<'EOF'
#!/usr/bin/env bash
echo "4242 flock WRITE $FAKE/lab.lock"
EOF
  # a network manager's re-apply restores the address + route (as ifup / networkd / netplan would)
  for m in ifup networkctl netplan; do
    cat > "$T/bin/$m" <<'EOF'
#!/usr/bin/env bash
echo "$(basename "$0") $*" >> "$FAKE/calls"
printf '%s\n' "10.0.0.5 24 10.0.0.255" "2001:db8::5 64 -" > "$FAKE/state/addrs"; touch "$FAKE/state/route-default"; rm -f "$FAKE/state/ip-readonly"
EOF
  done
  for m in driverctl logger; do printf '#!/usr/bin/env bash\necho "%s $*" >> "$FAKE/calls"\n' "$m" > "$T/bin/$m"; done
  sed -i "s#\\\$FAKE#$T#g" "$T"/bin/*   # fakes carry their root literally: a unit runs under env -i
  chmod +x "$T"/bin/*
  : > "$T/calls"
  export VRX_STARTUP_CONF="$T/etc/vpp/startup.conf" VRX_SYSFS="$T/sys" VRX_ETC="$T/etc" VRX_APPLY_STATE="$T/apply" \
    VRX_LAB_LOCK="$T/lab.lock" VRX_VPP_LOCK="$T/vpp.lock" VRX_VPP_API_SOCKET="$T/api.sock" \
    VRX_SYSTEMCTL="$T/bin/systemctl" VRX_SYSTEMD_RUN="$T/bin/systemd-run" VRX_IP="$T/bin/ip" VRX_PING="$T/bin/ping" \
    VRX_SS="$T/bin/ss" VRX_TCPCONNECT="$T/bin/tcpconnect" VRX_LSLOCKS="$T/bin/lslocks" \
    VRX_DRIVERCTL="$T/bin/driverctl" VRX_NETPLAN="$T/bin/netplan.absent" VRX_IFUP="$T/bin/ifup" VRX_NETWORKCTL="$T/bin/networkctl" \
    VRX_LOGGER="$T/bin/logger" VRX_VPPCHECK="$T/bin/vrx-vppcheck" VRX_STARTUPGEN="$GEN" VRX_HANDOVER_EXTRA="" \
    VRX_MGMT_PEERS="" VRX_MGMT_IFS="" VRX_MGMT_PROBE="auto"
  unset SSH_CONNECTION
  DOCF="$FIX/cases/six-nic-sample.json"
  HOSTARGS=(-- --no-host --mgmt-pci 0000:0b:00.0 --plugin-dir "$T/plugins" --online-cpus 0-31 --numa-nodes 2 --hugepages-mb 2048)
  ORIG="$(sha256sum "$T/etc/vpp/startup.conf" | awk '{print $1}')"
  NEW="$(rendered "$DOCF")"
  approve_rendering "$NEW"
  T0=$SECONDS
}
rendered() { "$GEN" --current "$VRX_STARTUP_CONF" "${HOSTARGS[@]:1}" "$1" 2>/dev/null | sha256sum | awk '{print $1}'; }
# CT/ST: every fake command must answer within CT (vrx-vppcheck: CT+2) and every fake systemctl job within ST even on a
# loaded CI host (TD-6: 1 s / 3 s failed under load 35–45 — `hook restart 'sleep 2'` had 1 s to spare; with CT=2 one
# vrx-vppcheck still missed its 4 s in a burst to load 37); the hang scenarios wait for them
CT=3 ST=6
TIMING=(--window 2 --interval 1 --settle 1 --api-wait 2 --lock-timeout 2 --cmd-timeout "$CT" --svc-timeout "$ST" --deadman-lock-timeout 1)
APPROVE=(--i-have-product-owner-approval PENDING-fake-change)   # fixture: D-900 has it as subject and names the rendering
apply() { "$SCRIPT" --doc "$DOCF" --apply --foreground "${TIMING[@]}" "${APPROVE[@]}" --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "$@" "${HOSTARGS[@]}"; }
unchanged() { [[ $(sha256sum "$T/etc/vpp/startup.conf" | awk '{print $1}') == "$ORIG" ]]; }
work() { find "$T/apply" -mindepth 1 -maxdepth 1 -type d | head -1; }
work_wait() { local i w=""; for ((i = 0; i < 300; i++)); do w="$(work)"; [[ -n $w ]] && break; sleep 0.1; done; echo "$w"; }   # a background planner creates it
hook() { printf '#!/usr/bin/env bash\n%s\n' "${2//\$FAKE/$T}" > "$T/hooks/$1"; chmod +x "$T/hooks/$1"; }  # <restart|start> <body>
steal_mgmt_nic() {  # VPP takes the management NIC → vfio-pci; the kernel netdev comes back without addresses/route
  hook restart 'ln -sfn "$FAKE/sys/bus/pci/drivers/vfio-pci" "$FAKE/sys/bus/pci/devices/0000:0b:00.0/driver"
: > "$FAKE/state/addrs"; rm -f "$FAKE/state/route-default"'
}
waitfor() { local i; for ((i = 0; i < ${2:-100}; i++)); do [[ -e $1 ]] && return 0; sleep 0.2; done; return 1; }  # <file> [tries]
locks_free() { flock -n -x "$T/vpp.lock" true && flock -n -x "$T/lab.lock" true; }
finished_dir() { [[ -e $1/committed || -e $1/rolled-back || -e $1/console-needed || -e $1/superseded ]]; }   # <work dir> has a result marker

state_line() {  # <work dir> <rc> → one evidence line (what the host looks like after a scenario)
  local w="$1" m="" f
  for f in committed rolled-back console-needed superseded; do [[ -e $w/$f ]] && m+="$f "; done
  echo "    state: rc=$2 finished=[${m% }] vpp=$(cat "$T/state/vpp") locks-free=$(locks_free && echo y || echo n) timer-cancelled=$(grep -q "systemctl stop vrx-startup-apply-deadman-$(basename "$w").timer" "$T/calls" && echo y || echo n) live==new=$(cmp -s "$T/etc/vpp/startup.conf" "$w/new.conf" 2>/dev/null && echo y || echo n) live==orig=$(unchanged 2>/dev/null && echo y || echo n)"
}
restarts() { grep -cE "^systemctl (restart|start) vpp" "$T/calls" || true; }   # VPP (re)starts done by the script

if scen 1; then
echo "== 1. dry run (default) changes nothing; prints diffs, drivers, management restore plan, reachability check, preflight, gate, both sha256"
setup
rc=0; out="$(TMPDIR="$T/tmp" "$SCRIPT" --doc "$DOCF" --cmd-timeout "$CT" "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 3 ]] && grep -q "^+  dev 0000:04:00.0 {" <<<"$out"' "exit $rc (the gate would refuse --apply: TD-6 V9); unified diff shows the new dev lines"
ok 'grep -q "^+ dpdk > dev 0000:04:00.0 > name wan" <<<"$out"' "semantic diff shows the logical names"
ok 'grep -q "0000:0b:00.0 vmxnet3" <<<"$out"' "drivers of the PCI devices involved are listed"
ok 'grep -q "ens192 pci=0000:0b:00.0 driver=vmxnet3 manager=ifupdown addrs=10.0.0.5/24 2001:db8::5/64" <<<"$out"' "management interface, ifupdown detected, addresses recorded"
ok 'grep -q "ip addr replace 10.0.0.5/24 broadcast 10.0.0.255 dev ens192" <<<"$out" && grep -q "ip -4 route replace default via 10.0.0.1 dev ens192 onlink" <<<"$out"' "exact restore plan printed (address + default route)"
ok '! grep -q "fe80\|proto kernel" <<<"$(sed -n "/restore plan/,/reachability/p" <<<"$out")"' "link-local address and kernel routes are not in the plan"
ok 'grep -q "will use: neigh (passes now)" <<<"$out"' "reachability check chosen and shown: neigh (next hop REACHABLE)"
ok 'grep -q "present: local0" <<<"$out" && grep -q "boot identity: fake-boot/1000/5000" <<<"$out" && grep -q "REFUSED: docs/lab/host-vrx-a.md says handover: pending" <<<"$out"' "VPP preflight + boot identity; gate state shown"
ok 'grep -qx "$ORIG" <<<"$out" && grep -qx "$NEW" <<<"$out"' "sha256 of the live file and of the rendering printed"
ok 'unchanged && ! grep -qE "systemctl (restart|stop|start)|addr replace" "$T/calls"' "live file untouched, VPP not restarted, no ip change"
ok '[[ -z $(ls -A "$T/tmp") ]]' "dry run removed its temp dir"

fi
if scen 2; then
echo "== 2. dry run: reachability — gateway drops ICMP (vrx-a) → neigh; SSH peer shown as a signal only; nothing viable → exit 3; loopback TCP target rejected"
setup
touch "$T/state/no-icmp" "$T/state/ssh-est"
rc=0; out="$(SSH_CONNECTION="10.0.0.9 51234 10.0.0.5 22" "$SCRIPT" --doc "$DOCF" --cmd-timeout "$CT" "${APPROVE[@]}" "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 0 ]] && grep -q "manager peer(s): 10.0.0.9" <<<"$out" && grep -q "will use: neigh (passes now); manager session with 10.0.0.9: established" <<<"$out"' "exit $rc; peer shown; neigh chosen although ICMP is dropped"
touch "$T/state/arp-dead"
rc=0; out="$("$SCRIPT" --doc "$DOCF" --cmd-timeout "$CT" "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 3 ]] && grep -q "NONE VIABLE — --apply will refuse: no viable management reachability check on this host: next hop 10.0.0.1 on ens192 is not REACHABLE" <<<"$out"' "next hop does not answer ARP: exit $rc, refused early"
rc=0; out="$("$SCRIPT" --doc "$DOCF" --cmd-timeout "$CT" --mgmt-probe tcp:10.0.0.1:22 "${APPROVE[@]}" "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 0 ]] && grep -q "will use: tcp:10.0.0.1:22 (passes now)" <<<"$out"' "--mgmt-probe tcp:10.0.0.1:22 viable (exit $rc)"
rc=0; out="$("$SCRIPT" --doc "$DOCF" --cmd-timeout "$CT" --mgmt-probe tcp:127.0.0.1:22 "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 3 ]] && grep -q "TCP target 127.0.0.1 is not routed through a management interface (via lo)" <<<"$out"' "loopback TCP target rejected (exit $rc)"
rc=0; out="$("$SCRIPT" --doc "$DOCF" --cmd-timeout "$CT" --mgmt-probe gateway-ping "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 3 ]] && grep -q "does not pass now: gateway 10.0.0.1 on ens192 does not answer ICMP" <<<"$out"' "explicit gateway-ping on a no-ICMP gateway: exit $rc"

fi
if scen 3; then
echo "== 3. usage errors → exit 2, nothing changed"
setup
rc=0; "$SCRIPT" --doc "$DOCF" --apply --foreground "${APPROVE[@]}" --expect-sha256 "$ORIG" "${HOSTARGS[@]}" >/dev/null 2>&1 || rc=$?
ok '[[ $rc == 2 ]] && unchanged' "--apply without --expect-new-sha256 refused (exit $rc)"
rc=0; "$SCRIPT" --doc "$DOCF" --apply --i-have-product-owner-approval "D-060" --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" >/dev/null 2>&1 || rc=$?
ok '[[ $rc == 2 ]] && unchanged' "approval id that is not PENDING-<slug> refused (exit $rc)"
rc=0; "$SCRIPT" --doc "$DOCF" --window 0 "${HOSTARGS[@]}" >/dev/null 2>&1 || rc=$?
ok '[[ $rc == 2 ]]' "--window 0 refused (exit $rc)"
rc=0; SSH_CONNECTION="10.0.0.9 1 10.0.0.5 22" "$SCRIPT" --doc "$DOCF" --apply --foreground "${APPROVE[@]}" --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 2 ]] && unchanged && grep -q "foreground over SSH" "$T/out"' "--foreground over SSH without --console refused (exit $rc)"
rc=0; VRX_STARTUP_CONF=/etc/vpp/startup.conf "$SCRIPT" --doc "$DOCF" "${HOSTARGS[@]}" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 2 ]] && grep -q "VRX_TEST_ROOT is only honoured when" "$T/out"' "a test root with the real startup.conf is refused (exit $rc)"

fi
if scen 4; then
echo "== 4. gate: pending → refused; approval only when a D-row has the PENDING as subject AND names this rendering; spent approval refused (N4)"
setup
rc=0; out="$("$SCRIPT" --doc "$DOCF" --apply --foreground "${TIMING[@]}" --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 3 ]] && unchanged && [[ -z $(work) ]] && grep -q "REFUSED: docs/lab/host-vrx-a.md says handover: pending" <<<"$out"' "no approval: exit $rc, no work dir"
for bad in "PENDING-no-such:does not exist on main" "PENDING-closed:no D-row on main answering PENDING-closed names this rendering" "PENDING-mentioned:no D-row on main has PENDING-mentioned as the subject"; do
  id="${bad%%:*}" msg="${bad#*:}"
  rc=0; out="$("$SCRIPT" --doc "$DOCF" --apply --foreground "${TIMING[@]}" --i-have-product-owner-approval "$id" --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" 2>&1)" || rc=$?
  ok '[[ $rc == 3 ]] && unchanged && [[ -z $(work) ]] && grep -q "$msg" <<<"$out"' "$id refused (exit $rc)"
done
ok '! grep -q "systemctl" "$T/calls"' "systemd never touched"
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 0 ]] && grep -q "PRODUCT-OWNER APPROVAL PENDING-fake-change (docs/decisions/PENDING-fake-change.md@[0-9a-f]\{12\} + LOG D-900 for rendering $NEW)" "$W/gate" "$W/log"' "D-900 has the PENDING as subject and names this rendering: accepted, recorded in gate + log (exit $rc)"
ok 'grep -q "logger -t vrx-startup-apply -- gate: handover pending.*PENDING-fake-change" "$T/calls"' "approval sent to syslog"
cp "$FIX/host-startup.conf" "$T/etc/vpp/startup.conf"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && [[ $(find "$T/apply" -mindepth 1 -maxdepth 1 -type d | wc -l) == 1 ]] && grep -q "PENDING-fake-change was already executed for this rendering (.* committed) — a spent approval cannot be replayed" "$T/out"' "same approval + same rendering again after the commit: spent, exit $rc, no new work dir"
jq '.dataplane.devices |= with_entries(if .value.name == "sync" then .value.name = "spare" else . end)' "$DOCF" > "$T/other.json"
rc=0; "$SCRIPT" --doc "$T/other.json" --apply --foreground "${TIMING[@]}" "${APPROVE[@]}" --expect-sha256 "$ORIG" --expect-new-sha256 "$(rendered "$T/other.json")" "${HOSTARGS[@]}" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && grep -q "no D-row on main answering PENDING-fake-change names this rendering" "$T/out"' "same PENDING, a different change: not covered, exit $rc"
rc=0; out="$("$SCRIPT" --doc "$T/other.json" --cmd-timeout "$CT" "${APPROVE[@]}" "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok 'grep -q "does not cover this change" <<<"$out"' "the dry run shows the gate against its own rendering"
setup
sed -i 's/handover: pending/handover: done/' "$CANON/docs/lab/host-vrx-a.md"
rc=0; "$SCRIPT" --doc "$DOCF" --apply --foreground "${TIMING[@]}" --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 0 ]] && grep -q "gate: handover done" "$T/out"' "handover: done → accepted without approval (exit $rc)"
rc=0; VRX_HANDOVER_EXTRA="$T/host-pending.md" "$SCRIPT" --doc "$DOCF" --apply --foreground "${TIMING[@]}" --expect-sha256 "$(sha256sum "$T/etc/vpp/startup.conf" | awk '{print $1}')" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" > "$T/out" 2>&1 || rc=$?
sed -i 's/handover: done/handover: pending/' "$CANON/docs/lab/host-vrx-a.md"
ok '[[ $rc == 3 ]] && grep -q "host-pending.md=pending" "$T/out"' "one more pending source keeps it pending (exit $rc)"
ok '[[ $( (. "$SCRIPT"; printf "x\n\`handover: done\`\n" | handover_flag) ) == done && $( (. "$SCRIPT"; printf "nothing\n" | handover_flag) ) == pending ]] && [[ $( (. "$SCRIPT"; ere_escape "ens192.10") ) == "ens192\\.10" ]]' "flag parser (tools/lab rule); interface names regex-escaped"

fi
if scen 5; then
echo "== 5. --stage run verifies the sealed plan (forged gate / altered settings → refused)"
setup
apply >/dev/null 2>&1
W="$(work)"
for f in committed installed backup.conf log; do rm -f "$W/$f"; done
cp "$FIX/host-startup.conf" "$T/etc/vpp/startup.conf"
echo "gate: handover done (forged)" > "$W/gate"
rc=0; "$W/bin/apply-startup.sh" --stage run --work "$W" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && grep -q "REFUSED: plan seal mismatch" "$T/out"' "forged gate file: exit $rc"
sed -i 's/^WINDOW=.*/WINDOW=1/' "$W/settings"
rc=0; "$W/bin/apply-startup.sh" --stage run --work "$W" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && grep -q "REFUSED: plan seal mismatch" "$T/out"' "altered settings: exit $rc"

fi
if scen 6; then
echo "== 6. what was reviewed is pinned: live file or rendering changed → refused inside the lock"
setup
rc=0; "$SCRIPT" --doc "$DOCF" --apply --foreground "${TIMING[@]}" "${APPROVE[@]}" --expect-sha256 "$(printf '%064d' 0)" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && ! grep -q "systemctl restart" "$T/calls" && grep -q "changed since the review" "$T/out" && locks_free' "live file changed: exit $rc, no restart, locks released"
approve_rendering "$(printf '%064d' 1)"   # the approval matches the claimed sum; the run must still catch the real rendering
rc=0; "$SCRIPT" --doc "$DOCF" --apply --foreground "${TIMING[@]}" "${APPROVE[@]}" --expect-sha256 "$ORIG" --expect-new-sha256 "$(printf '%064d' 1)" "${HOSTARGS[@]}" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && ! grep -q "systemctl restart" "$T/calls" && grep -q "rendering differs from the reviewed one" "$T/out"' "rendering changed: exit $rc, no restart"

fi
if scen 7; then
echo "== 7. VPP hung BEFORE the apply → preflight refuses within the timeout, nothing installed"
setup
touch "$T/state/hang"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && [[ ! -e $(work)/installed ]] && grep -q "REFUSED: VPP preflight failed" "$T/out" && locks_free' "exit $rc, refused, locks released"
bounded 15 "the preflight waits for the hung VPP's sleep 1000"

fi
if scen 8; then
echo "== 8. healthy apply commits — NRestarts 4 before, reset to 0 by the restart (H1); settle + ≥2 identity reads (N3)"
setup
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 0 && -e $W/committed && ! -e $W/rolled-back ]] && [[ $(cat "$T/state/nrestarts") == 0 ]]' "exit $rc, committed although NRestarts went 4 → 0"
ok '[[ $(cat "$W/ident.before") == fake-boot/1000/5000 && $(cat "$W/ident.after") == fake-boot/1001/5100 ]] && grep -q "MainPID=1001 NRestarts=0" "$W/unit.restart"' "new boot identity = the unit's MainPID, NRestarts 0 right after the restart"
ok '(( $(wc -l < "$W/ident.reads") >= 2 ))' "identity stable across $(wc -l < "$W/ident.reads" 2>/dev/null || echo 0) reads in the window"
ok 'cmp -s "$T/etc/vpp/startup.conf" "$W/new.conf" && cmp -s "$W/backup.conf" "$FIX/host-startup.conf" && ls "$T/etc/vpp/" | grep -q "startup.conf.bak-"' "new file installed, backup kept"
ok 'grep -q "vppcheck ifaces dmz lan lan2 p2p sync wan" "$T/calls" && grep -q "vppcheck plugins" "$T/calls"' "logical interfaces + plugins verified through vrx-vppcheck"
ok '[[ $(cat "$W/mgmt.ifs") == ens192 && $(cat "$W/mgmt/ens192.netmgr") == ifupdown && $(cat "$W/probe") == neigh ]] && grep -q "ip -j neigh show 10.0.0.1 dev ens192" "$T/calls"' "management snapshot: ens192, ifupdown; check neigh (ARP of the next hop)"
ok '[[ $(grep -n "locks held by holder" "$T/out" | cut -d: -f1) -lt $(grep -n "backup:" "$T/out" | cut -d: -f1) ]]' "locks taken (holder unit) before backup and diff"
ok 'grep -q "systemd-run --unit=vrx-startup-apply-deadman-.* --on-active=.*--setenv=VRX_STARTUP_CONF=$T/etc/vpp/startup.conf.* --stage rollback" "$T/calls" && grep -q "systemd-run --unit=vrx-startup-apply-lock-.* --stage hold" "$T/calls"' "dead-man timer and lock holder units started with the settings as --setenv"
ok 'grep -q "systemctl stop vrx-startup-apply-deadman-.*\.timer" "$T/calls" && locks_free && [[ $(restarts) == 1 ]]' "timer cancelled, locks released, exactly one VPP restart"
ok 'read -r a r b < <(sed -nE "s/.*rollback in ([0-9]+)s .*run budget ([0-9]+)s, rollback budget ([0-9]+)s.*/\1 \2 \3/p" "$T/out"); (( a > r + b ))' "dead-man deadline > run budget + rollback budget"

fi
if scen 9; then
echo "== 9. VPP did not actually restart (same boot identity) → rollback"
setup
touch "$T/state/no-restart"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && grep -q "ROLLBACK: VPP did not restart (boot identity still fake-boot/1000/5000)" "$T/out"' "exit $rc, detected"

fi
if scen 10; then
echo "== 10. VPP crashes and is brought back by systemd — inside the settle time (N3) and inside the window → rollback"
setup
hook restart 'echo 0 > "$FAKE/state/crash-after"'   # crashes at the first API probe after the restart
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 && -e $(work)/rolled-back ]] && unchanged && grep -q "ROLLBACK: vpp.service restarted during the settle time" "$T/out"' "crash before the first identity read: exit $rc, rolled back"
setup
hook restart 'echo 2 > "$FAKE/state/crash-after"'   # crashes at the first identity read of the window
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 && -e $(work)/rolled-back ]] && unchanged && grep -qE "ROLLBACK: (vpp.service restarted since the apply|VPP crashed and came back)" "$T/out"' "crash in the window: exit $rc, rolled back"

fi
if scen 11; then
echo "== 11. reviewer's scenario: gateway drops ICMP, the manager's SSH session closes during the window → commits (N1)"
setup
touch "$T/state/no-icmp" "$T/state/ssh-est"; export VRX_MGMT_PEERS="10.0.0.9"
hook restart 'rm -f "$FAKE/state/ssh-est"'
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 0 && -e $W/committed ]] && [[ $(cat "$W/probe") == neigh ]] && grep -q "manager session: not established (informational only)" "$T/out" && [[ $(restarts) == 1 ]]' "exit $rc, committed with neigh; closed session logged only; one VPP restart"

fi
if scen 12; then
echo "== 12. --mgmt-probe tcp:… → commits; path lost → rollback judged by the same probe"
setup
touch "$T/state/no-icmp"
rc=0; apply --mgmt-probe tcp:10.0.0.1:22 > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 0 && -e $(work)/committed ]] && grep -q "tcpconnect 10.0.0.1 22" "$T/calls"' "exit $rc, committed with a TCP probe"
setup
hook restart 'touch "$FAKE/state/tcp-down"'
hook start 'rm -f "$FAKE/state/tcp-down"'
rc=0; apply --mgmt-probe tcp:10.0.0.1:22 > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 && -e $(work)/rolled-back ]] && unchanged && grep -q "ROLLBACK: management path: TCP connect to 10.0.0.1 port 22 failed" "$T/out"' "exit $rc, lost TCP path → rollback, healthy afterwards by the same probe"

fi
if scen 13; then
echo "== 13. the path stays dead after the rollback → ONE rollback, console-needed, locks released, no restart loop (N1)"
setup
hook restart 'touch "$FAKE/state/arp-dead"'
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 1 && -e $W/console-needed && ! -e $W/rolled-back ]] && unchanged && grep -q "CONSOLE NEEDED" "$T/out"' "exit $rc, file restored, console-needed marker"
ok 'locks_free && grep -q "systemctl stop vrx-startup-apply-deadman-.*\.timer" "$T/calls" && [[ $(restarts) == 2 ]]' "locks released, timer cancelled, VPP (re)started twice in total (apply + one rollback)"
out="$("$W/bin/apply-startup.sh" --stage rollback --work "$W" 2>&1)"
ok 'grep -q "nothing to do (the apply already finished)" <<<"$out" && [[ $(restarts) == 2 ]]' "a late dead-man does nothing — no further restart"

fi
if scen 14; then
echo "== 14. no viable check at apply time → refused before anything changes"
setup
touch "$T/state/arp-dead"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && [[ ! -e $(work)/installed ]] && grep -q "no viable management reachability check" "$T/out" && locks_free' "exit $rc, refused, locks released"

fi
if scen 15; then
echo "== 15. a logical interface missing in VPP → rollback"
setup
grep -v '^lan2$' "$T/state/ifaces" > "$T/state/i2" && mv "$T/state/i2" "$T/state/ifaces"
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 1 && -e $W/rolled-back && ! -e $W/committed ]] && unchanged && locks_free' "exit $rc, rolled back, original file restored, locks released"
ok 'grep -q "ROLLBACK: logical interface(s) not in VPP (API check): missing: lan2" "$T/out"' "reason logged"
ok 'grep -q "systemctl stop vpp" "$T/calls" && grep -q "systemctl reset-failed vpp" "$T/calls" && grep -q "systemctl start vpp" "$T/calls"' "VPP stopped, reset-failed, started on the old file"
n="$(restarts)"; rc=0; "$W/bin/apply-startup.sh" --stage run --work "$W" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && [[ $(restarts) == "$n" ]] && grep -q "REFUSED: this work dir was already used" "$T/out"' "replaying the rolled-back work dir with --stage run: refused (exit $rc), no restart"

fi
if scen 16; then
echo "== 16. ifupdown host (vrx-a), no netplan: VPP steals the management NIC → rebind + EXACT address/route restore"
setup
steal_mgmt_nic
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 1 ]] && unchanged && grep -q "ROLLBACK: management interface ens192 lost: .*addr replace 10.0.0.5/24 broadcast 10.0.0.255" "$T/out"' "exit $rc, loss detected (addresses/routes compared exactly)"
ok 'grep -qx "0000:0b:00.0" "$T/sys/bus/pci/drivers/vfio-pci/unbind" && grep -qx "0000:0b:00.0" "$T/sys/bus/pci/drivers/vmxnet3/bind" && grep -q "driverctl unset-override 0000:0b:00.0" "$T/calls"' "unbound from vfio-pci, bound back to vmxnet3, override cleared"
ok 'grep -q "ip addr replace 10.0.0.5/24 broadcast 10.0.0.255 dev ens192" "$T/calls" && grep -q "ip addr replace 2001:db8::5/64 dev ens192" "$T/calls" && grep -q "ip -4 route replace default via 10.0.0.1 dev ens192 onlink" "$T/calls"' "recorded addresses (v4+v6) and default route re-applied verbatim"
ok '[[ $(grep -n "ip addr replace 10.0.0.5/24" "$T/calls" | head -1 | cut -d: -f1) -lt $(grep -n "ip -4 route replace default" "$T/calls" | head -1 | cut -d: -f1) ]]' "addresses before routes"
ok 'grep -q "^10.0.0.5 24 10.0.0.255$" "$T/state/addrs" && [[ -e $T/state/route-default ]] && [[ $(basename "$(readlink "$T/sys/bus/pci/devices/0000:0b:00.0/driver")") == vmxnet3 ]]' "fake kernel: address, default route and vmxnet3 driver back"
ok '! grep -qE "^(netplan|ifup|networkctl) " "$T/calls" && [[ -e $W/rolled-back ]] && grep -q "management path are healthy" "$T/out"' "no network manager needed; rollback verified healthy"

fi
if scen 17; then
echo "== 17. ifupdown, exact re-apply not enough → ifup --force ens192"
setup
steal_mgmt_nic; touch "$T/state/ip-readonly"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && grep -q "ifup --force ens192" "$T/calls" && [[ -e $(work)/rolled-back ]]' "exit $rc, ifup --force used, rollback healthy"

fi
if scen 18; then
echo "== 18. systemd-networkd host → networkctl reconfigure"
setup
rm -f "$T/etc/network/interfaces.d/ens192.cfg"; touch "$T/state/networkd"
steal_mgmt_nic; touch "$T/state/ip-readonly"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && [[ $(cat "$(work)/mgmt/ens192.netmgr") == networkd ]] && grep -q "networkctl reconfigure ens192" "$T/calls" && [[ -e $(work)/rolled-back ]]' "exit $rc, networkd detected, reconfigure used, rollback healthy"

fi
if scen 19; then
echo "== 19. netplan host → netplan apply"
setup
rm -f "$T/etc/network/interfaces.d/ens192.cfg"; mkdir -p "$T/etc/netplan"; echo "network: {version: 2}" > "$T/etc/netplan/50-cloud-init.yaml"
export VRX_NETPLAN="$T/bin/netplan"
steal_mgmt_nic; touch "$T/state/ip-readonly"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && [[ $(cat "$(work)/mgmt/ens192.netmgr") == netplan ]] && grep -q "netplan apply" "$T/calls" && [[ -e $(work)/rolled-back ]]' "exit $rc, netplan detected, netplan apply used, rollback healthy"

fi
if scen 20; then
echo "== 20. plugins: enabled but not loaded → rollback; D-084 omission (dropped from the document) → commits"
setup
grep -v npt66 "$T/state/plugins" > "$T/state/p2" && mv "$T/state/p2" "$T/state/plugins"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && grep -q "plugin npt66_plugin.so enabled but not loaded" "$T/out"' "exit $rc, reason: npt66 not loaded"
setup
jq 'del(.dataplane.plugins.switches["npt66_plugin.so"])' "$DOCF" > "$T/no-npt66.json"
DOCF="$T/no-npt66.json"; NEW="$(rendered "$DOCF")"; approve_rendering "$NEW"
hook restart 'grep -v npt66 "$FAKE/state/plugins" > "$FAKE/state/p2" && mv "$FAKE/state/p2" "$FAKE/state/plugins"'
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 0 && -e $(work)/committed ]] && ! grep -q npt66 "$T/etc/vpp/startup.conf"' "exit $rc, committed without npt66"

fi
if scen 21; then
echo "== 21. VPP HANGS after the restart (socket accepted, no answer) → bounded wait, rollback"
setup
hook restart 'touch "$FAKE/state/hang"'
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 && -e $(work)/rolled-back ]] && unchanged && grep -q "ROLLBACK: VPP API did not come up within 2s (hung or crashed)" "$T/out"' "exit $rc, rolled back"
bounded 30 "wait_api on a VPP that never answers: sleep 1000 per probe"

fi
if scen 22; then
echo "== 22. VPP hangs in the middle of the watch window → rollback"
setup
hook restart 'echo 3 > "$FAKE/state/hang-after"'
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 && -e $(work)/rolled-back ]] && unchanged && grep -qE "ROLLBACK: (VPP API does not answer|VPP did not list its plugins|logical interface|VPP boot identity unreadable)" "$T/out"' "exit $rc, hang detected, rolled back"
bounded 30 "a health read on a hung VPP: sleep 1000"

fi
if scen 23; then
echo "== 23. systemctl restart itself hangs → svc timeout, rollback"
setup
hook restart 'echo $$ >> "$FAKE/state/pids"; exec sleep 1000'
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 && -e $(work)/rolled-back ]] && unchanged && grep -q "restart vpp failed or hung for ${ST}s" "$T/out"' "exit $rc, rolled back"
bounded 30 "systemctl restart hangs in sleep 1000"

fi
if scen 24; then
echo "== 24. SSH session dies mid-apply (the run is a systemd-run unit with a clean environment) → the run still commits"
setup
hook restart 'sleep 2'
setsid bash -c '"$0" --doc "$1" --apply "${@:2}"; sleep 60' "$SCRIPT" "$DOCF" "${TIMING[@]}" "${APPROVE[@]}" --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" > "$T/caller.out" 2>&1 &
SSH=$!; PIDS+=("$SSH")
W="$(work_wait)"
waitfor "$W/installed" 150 || true
kill -KILL -- "-$SSH" 2>/dev/null || true   # the whole "SSH session" process group dies while VPP restarts
sleep 0.2
ok '[[ -e $W/installed && ! -e $W/committed ]] && ! kill -0 "$SSH" 2>/dev/null' "session killed after install, before commit"
waitfor "$W/committed" 300 || true
ok '[[ -e $W/committed ]] && cmp -s "$T/etc/vpp/startup.conf" "$W/new.conf"' "detached run committed anyway"
ok 'grep -q "systemd-run --unit=vrx-startup-apply-[0-9-]* --collect --quiet --property=KillMode=process --setenv=VRX_STARTUP_CONF=.*--setenv=VRX_STARTUPGEN=$GEN .*--setenv=PATH=.*/bin/apply-startup.sh --stage run --work $W" "$T/calls"' "unit started with every setting as --setenv"
ok '! grep -q "WARNING: environment" "$W/log" && grep -q "journalctl -fu vrx-startup-apply-" "$T/caller.out"' "the unit saw exactly the recorded settings under env -i"

fi
if scen 25; then
echo "== 25. systemd-run unavailable → --apply refused, no fallback (N2)"
setup
touch "$T/state/systemd-run-fail"
rc=0; "$SCRIPT" --doc "$DOCF" --apply "${TIMING[@]}" "${APPROVE[@]}" --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && grep -q "systemd-run cannot start the apply unit (there is no fallback" "$T/out" && ! grep -q "systemctl restart" "$T/calls"' "run unit: exit $rc, nothing changed"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && grep -q "systemd-run cannot start the lock holder unit" "$T/out" && locks_free' "lock holder unit (--foreground): exit $rc, nothing changed"
setup
touch "$T/state/timer-fail"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && [[ ! -e $(work)/installed ]] && grep -q "cannot arm the dead-man timer" "$T/out" && locks_free' "dead-man timer: exit $rc, refused before install, locks released"

fi
if scen 26; then
echo "== 26. the run HANGS; an integration run queues on the lab lock → dead-man kills the run; the holder keeps the locks until its single rollback is done (M1)"
setup
hook restart 'echo $$ >> "$FAKE/state/pids"; exec sleep 1000'
TIMING_SAVE=("${TIMING[@]}"); TIMING=(--window 2 --interval 1 --settle 1 --api-wait 2 --lock-timeout 2 --cmd-timeout "$CT" --svc-timeout 600 --deadman-lock-timeout 5)
apply > "$T/run.out" 2>&1 & RUNNER=$!; PIDS+=("$RUNNER")
TIMING=("${TIMING_SAVE[@]}")
W="$(work_wait)"; waitfor "$W/installed" 150 || true; sleep 0.3
read -r RUNPID < "$W/run-pid"; PIDS+=("$RUNPID"); HOLD="$(cat "$W/locks-held")"
ok 'kill -0 "$RUNPID" && kill -0 "$HOLD" && ! flock -n -x "$T/vpp.lock" true' "run stuck in 'systemctl restart'; holder unit $HOLD owns the locks"
flock -s "$T/lab.lock" -c "echo WAITER GOT THE LAB LOCK >> $T/calls" & WAITER=$!; PIDS+=("$WAITER")
sleep 0.3
T0=$SECONDS
rc=0; "$W/bin/apply-startup.sh" --stage rollback --work "$W" > "$T/out" 2>&1 || rc=$?
wait "$WAITER" 2>/dev/null || true
ok '[[ $rc == 1 && -e $W/rolled-back ]] && unchanged && ! kill -0 "$RUNPID" 2>/dev/null' "exit $rc, run killed, file restored"
ok 'grep -q "dead-man: lock holder $HOLD still owns the locks" "$T/out" && grep -q "dead-man: killing the run (pid $RUNPID)" "$T/out" && ! grep -q FORCED "$T/out"' "holder kept the locks across the kill; not FORCED"
ok '[[ $(grep -n "^systemctl start vpp" "$T/calls" | tail -1 | cut -d: -f1) -lt $(grep -n "WAITER GOT THE LAB LOCK" "$T/calls" | cut -d: -f1) ]]' "queued shared waiter got the lab lock only after the rollback restarted VPP"
ok 'locks_free && ! kill -0 "$HOLD" 2>/dev/null && [[ $(grep -c "^systemctl start vpp" "$T/calls") == 1 ]]' "locks released, holder gone, exactly one rollback start"
bounded 20 "the run's systemctl restart hangs (sleep 1000, --svc-timeout 600)"

fi
if scen 27; then
echo "== 27. the lock holder dies during the apply → rollback (the locks are not ours any more)"
setup
hook restart 'for f in "$FAKE"/apply/*/locks-held; do kill -KILL "$(cat "$f")"; done'
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 && -e $(work)/rolled-back ]] && unchanged && grep -q "ROLLBACK: the lock holder is gone" "$T/out"' "exit $rc, rolled back"

fi
if scen 28; then
echo "== 28. dead-man: no-op after commit; one rollback when the run died before finishing (holder gone → takes the locks itself)"
setup
apply >/dev/null 2>&1
W="$(work)"
out="$("$W/bin/apply-startup.sh" --stage rollback --work "$W" 2>&1)"
ok 'grep -q "nothing to do" <<<"$out" && ! unchanged' "committed run: dead-man does nothing"
rm "$W/committed"
rc=0; "$W/bin/apply-startup.sh" --stage rollback --work "$W" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && [[ -e $W/rolled-back ]] && grep -q "dead-man: lock holder gone — took the locks exclusively before touching VPP" "$T/out" && locks_free && [[ $(grep -c "ROLLBACK:" "$T/out") == 1 ]]' "exactly one rollback, locks taken exclusively first, released after (exit $rc)"

fi
if scen 29; then
echo "== 29. dead-man, holder gone AND a foreign process holds the lab lock → bounded wait, FORCED rollback, holder logged"
setup
apply >/dev/null 2>&1
W="$(work)"; rm "$W/committed"
foreign_lock "$T/lab.lock"
T0=$SECONDS
rc=0; "$W/bin/apply-startup.sh" --stage rollback --work "$W" > "$T/out" 2>&1 || rc=$?
kill "$HOLDER" 2>/dev/null || true
ok '[[ $rc == 1 && -e $W/rolled-back ]] && unchanged && grep -q "FORCED — locks held by someone else for 1s (.*lslocks: 4242 flock WRITE $T/lab.lock" "$T/out"' "exit $rc, rolled back without the lock, holder named"
bounded 15 "the foreign holder keeps the lab lock for 300 s"

fi
if scen 30; then
echo "== 30. lab lock held by someone else at apply time → refused, nothing changed"
setup
foreign_lock "$T/lab.lock"   # held until killed: a fixed 6 s hold raced the planner's ~4 s + --lock-timeout 2 (TD-6)
rc=0; apply > "$T/out" 2>&1 || rc=$?
kill "$HOLDER" 2>/dev/null || true
ok '[[ $rc == 3 ]] && unchanged && grep -q "lab.lock held by: 4242 flock WRITE" "$T/out" && ! grep -q "systemctl restart" "$T/calls"' "exit $rc, lock busy (holder named), file unchanged"

fi
if scen 31; then
echo "== 31. the generator refuses the management NIC as a device (host facts) → nothing changed"
setup
echo '{"dataplane":{"managementPci":["0000:04:00.0"],"devices":{"0000:0b:00.0":{"name":"lan"}}}}' > "$T/bad.json"
rc=0; "$SCRIPT" --doc "$T/bad.json" "${HOSTARGS[@]}" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 2 ]] && unchanged && grep -q "does not match the host" "$T/out"' "dry run fails with the generator's error (exit $rc)"

fi
# ---------------------------------------------------------------- TD-6 (D-103): V1–V9 of the F-startup-apply verify review
if scen 32; then
echo "== 32. V1: a dead run's armed dead-man never reverts a newer apply — the planner refuses while an apply is unfinished; a superseded dead-man changes nothing"
setup
printf '%s\n' spare >> "$T/state/ifaces"
jq '.dataplane.devices |= with_entries(if .value.name == "sync" then .value.name = "spare" else . end)' "$DOCF" > "$T/other.json"
NEW2="$(rendered "$T/other.json")"; approve_rendering "$NEW2"
apply2() { "$SCRIPT" --doc "$T/other.json" --apply --foreground "${TIMING[@]}" "${APPROVE[@]}" --expect-sha256 "$1" --expect-new-sha256 "$NEW2" "${HOSTARGS[@]}"; }
hook restart 'echo $$ >> "$FAKE/state/pids"; exec sleep 1000'
TIMING_SAVE=("${TIMING[@]}"); TIMING=(--window 2 --interval 1 --settle 1 --api-wait 2 --lock-timeout 2 --cmd-timeout "$CT" --svc-timeout 600 --deadman-lock-timeout 1)
apply > "$T/run1.out" 2>&1 & RUNNER=$!; PIDS+=("$RUNNER")
TIMING=("${TIMING_SAVE[@]}")
W1="$(work_wait)"; waitfor "$W1/installed" 150 || true; sleep 0.3
read -r RUNPID < "$W1/run-pid"; HOLD="$(cat "$W1/locks-held")"
kill -KILL "$RUNPID" "$RUNNER" 2>/dev/null || true; kill_recorded_case   # the run dies (OOM, SIGKILL) during the restart; its holder too
rm -f "$T/hooks/restart"; sleep 0.3                                      # (the operator "unsticks" the locks)
ok '[[ -e $W1/installed ]] && ! finished_dir "$W1" && cmp -s "$T/etc/vpp/startup.conf" "$W1/new.conf" && locks_free' "run 1 died after installing; its holder is gone; its dead-man is still armed"
rc=0; apply2 "$(sha256sum "$T/etc/vpp/startup.conf" | awk '{print $1}')" > "$T/out" 2>&1 || rc=$?
state_line "$W1" "$rc"
ok '[[ $rc == 3 ]] && grep -q "REFUSED: an earlier apply has not finished: $W1 (dead-man vrx-startup-apply-deadman-.*\.timer)" "$T/out" && cmp -s "$T/etc/vpp/startup.conf" "$W1/new.conf" && [[ $(find "$T/apply" -mindepth 1 -maxdepth 1 -type d | wc -l) == 1 ]]' "a new apply is refused while run 1 is unfinished (exit $rc; dir and timer named), nothing changed"
rc=0; "$W1/bin/apply-startup.sh" --stage rollback --work "$W1" > "$T/dm1.out" 2>&1 || rc=$?
ok '[[ $rc == 1 && -e $W1/rolled-back ]] && unchanged' "run 1's dead-man makes its one rollback (exit $rc): original file back"
rc=0; apply2 "$ORIG" > "$T/out" 2>&1 || rc=$?
W2="$(find "$T/apply" -mindepth 1 -maxdepth 1 -type d ! -path "$W1" | head -1)"
ok '[[ $rc == 0 && -e $W2/committed ]] && cmp -s "$T/etc/vpp/startup.conf" "$W2/new.conf" && [[ $(cat "$T/apply/current" 2>/dev/null) == "$W2" ]]' "then the new apply commits (exit $rc) and is recorded as the current one"
n="$(restarts)"; stops="$(grep -c '^systemctl stop vpp' "$T/calls" || true)"
rm -f "$W1/rolled-back"   # a stale run 1 that never finished (planner check bypassed, marker lost): its dead-man fires late
rc=0; "$W1/bin/apply-startup.sh" --stage rollback --work "$W1" > "$T/out" 2>&1 || rc=$?
state_line "$W1" "$rc"
ok '[[ -e $W1/superseded ]] && grep -q "SUPERSEDED: a newer apply ($W2) installed" "$T/out" && cmp -s "$T/etc/vpp/startup.conf" "$W2/new.conf"' "late dead-man of run 1: superseded, the newer committed file stays (exit $rc)"
ok '[[ $(restarts) == "$n" && $(grep -c "^systemctl stop vpp" "$T/calls") == "$stops" ]] && locks_free' "VPP neither stopped nor restarted by it, locks released"
fi
if scen 33; then
echo "== 33. V2: the rollback cannot write startup.conf (/etc/vpp replaced by a file during the window) → VPP is NOT stopped; console-needed; timer cancelled; locks released"
setup
grep -v '^lan2$' "$T/state/ifaces" > "$T/state/i2" && mv "$T/state/i2" "$T/state/ifaces"
hook restart 'mv "$FAKE/etc/vpp" "$FAKE/etc/vpp.gone"; echo not-a-directory > "$FAKE/etc/vpp"'
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
state_line "$W" "$rc"
ok '[[ $rc == 1 && -e $W/console-needed && ! -e $W/rolled-back ]] && grep -q "cannot restore" "$W/console-needed" && grep -q "CONSOLE NEEDED" "$T/out"' "exit $rc, console-needed names the failed restore"
ok '[[ $(cat "$T/state/vpp") == active ]] && ! grep -q "^systemctl stop vpp" "$T/calls" && locks_free && grep -q "systemctl stop vrx-startup-apply-deadman-.*\.timer" "$T/calls"' "VPP left running (never stopped), locks released, timer cancelled"
out="$("$W/bin/apply-startup.sh" --stage rollback --work "$W" 2>&1)" || true
ok 'grep -q "nothing to do (the apply already finished)" <<<"$out" && ! grep -q "^systemctl stop vpp" "$T/calls"' "a late dead-man does nothing"
fi
if scen 34; then
echo "== 34. V2: the old file was staged, but /etc/vpp breaks while VPP is stopped → VPP is started again anyway; console-needed; locks released"
setup
grep -v '^lan2$' "$T/state/ifaces" > "$T/state/i2" && mv "$T/state/i2" "$T/state/ifaces"
hook stop 'mv "$FAKE/etc/vpp" "$FAKE/etc/vpp.gone"; echo not-a-directory > "$FAKE/etc/vpp"'
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
state_line "$W" "$rc"
ok '[[ $rc == 1 && -e $W/console-needed ]] && grep -q "NOT restored" "$W/console-needed"' "exit $rc, console-needed: startup.conf not restored"
ok '[[ $(cat "$T/state/vpp") == active ]] && [[ $(grep -n "^systemctl start vpp" "$T/calls" | tail -1 | cut -d: -f1) -gt $(grep -n "^systemctl stop vpp" "$T/calls" | tail -1 | cut -d: -f1) ]] && locks_free' "VPP started again after the stop, locks released"
fi
if scen 35; then
echo "== 35. V3: \`systemctl show\` fails right after the restart — once → read again, commits; persistently → immediate rollback (not a dead run)"
setup
hook restart 'echo once > "$FAKE/state/show-fail"'
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
state_line "$W" "$rc"
ok '[[ $rc == 0 && -e $W/committed ]] && grep -q "MainPID=1001 NRestarts=0" "$W/unit.restart" && locks_free' "transient D-Bus failure: exit $rc, committed with a complete unit tuple"
setup
hook restart 'echo always > "$FAKE/state/show-fail"'
hook start 'rm -f "$FAKE/state/show-fail"'
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
state_line "$W" "$rc"
ok '[[ $rc == 1 && -e $W/rolled-back ]] && unchanged && locks_free && grep -q "ROLLBACK: cannot read vpp.service" "$T/out"' "persistent failure: exit $rc, rolled back at once, locks released"
fi
if scen 36; then
echo "== 36. V4: VPP crashes and systemd restarts it BEFORE the first \`systemctl show\` → NRestarts=1 in the baseline → rollback"
setup
touch "$T/state/crash-at-restart"
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
state_line "$W" "$rc"; echo "    unit.restart: $(cat "$W/unit.restart" 2>/dev/null)"
ok '[[ $rc == 1 && -e $W/rolled-back ]] && unchanged && grep -q "ROLLBACK: vpp.service was restarted automatically right after the restart" "$T/out"' "exit $rc, the auto-restarted instance is not accepted"
fi
if scen 37; then
echo "== 37. V5: the holder dies while a shared waiter queues on the lab lock → the run's rollback takes the locks exclusively (bounded) before it stops VPP"
setup
hook restart 'sleep 2'
TIMING_SAVE=("${TIMING[@]}"); TIMING=(--window 2 --interval 1 --settle 1 --api-wait 2 --lock-timeout 2 --cmd-timeout "$CT" --svc-timeout "$ST" --deadman-lock-timeout 20)
apply > "$T/run.out" 2>&1 & RUNNER=$!; PIDS+=("$RUNNER")
TIMING=("${TIMING_SAVE[@]}")
W="$(work_wait)"; waitfor "$W/installed" 150 || true
flock -s "$T/lab.lock" -c "echo WAITER-IN >> $T/calls; sleep 6; echo WAITER-OUT >> $T/calls" & WAITER=$!; PIDS+=("$WAITER")
sleep 0.3; kill -KILL "$(cat "$W/locks-held")" 2>/dev/null || true
rc=0; wait "$RUNNER" || rc=$?
wait "$WAITER" 2>/dev/null || true
state_line "$W" "$rc"; echo "    order: $(grep -nE '^(WAITER-IN|WAITER-OUT|systemctl stop vpp|systemctl start vpp)' "$T/calls" | tr '\n' ' ')"
ok '[[ $rc == 1 && -e $W/rolled-back ]] && unchanged && grep -q "ROLLBACK: the lock holder is gone" "$T/run.out"' "exit $rc, rolled back"
wi=$(grep -n "^WAITER-IN" "$T/calls" | cut -d: -f1); wo=$(grep -n "^WAITER-OUT" "$T/calls" | cut -d: -f1)
rs=$(grep -n "^systemctl stop vpp" "$T/calls" | tail -1 | cut -d: -f1); re=$(grep -n "^systemctl start vpp" "$T/calls" | tail -1 | cut -d: -f1)
ok '[[ -n $wi && -n $wo && -n $rs && -n $re ]] && (( wo < rs || wi > re ))' "the waiter's lab-lock section [$wi,$wo] and the rollback's VPP stop..start [$rs,$re] do not overlap (calls lines)"
ok 'grep -q "took the locks exclusively" "$T/run.out" && ! grep -q FORCED "$T/run.out" && locks_free' "the rollback took the locks exclusively (not FORCED) and released them"
fi
if scen 38; then
echo "== 38. V6: the VRX_TEST_ROOT guard resolves paths — '/.' and a symlink out of the test root are refused"
setup
rc=0; out="$( (. "$SCRIPT"; VRX_TEST_ROOT=/. VRX_STARTUP_CONF=/./etc/vpp/startup.conf VRX_SYSFS=/./sys VRX_SYSTEMCTL=/./usr/bin/systemctl VRX_SYSTEMD_RUN=/./usr/bin/systemd-run VRX_IP=/./usr/bin/ip VRX_APPLY_STATE=/./var/lib/vrx/startup-apply VRX_VPP_LOCK=/./run/lock/vrx-vpp.lock VRX_LAB_LOCK=/./run/lock/vrx-lab.lock; canon_root) 2>&1)" || rc=$?
echo "    canon_root with VRX_TEST_ROOT=/. → rc=$rc: $out"
ok '[[ $rc == 2 ]] && grep -q "VRX_TEST_ROOT" <<<"$out"' "test root '/.' with /./etc/vpp/startup.conf refused (exit $rc)"
ln -s /etc "$TOP/esc"
rc=0; out="$( (. "$SCRIPT"; VRX_STARTUP_CONF="$TOP/esc/vpp/startup.conf"; canon_root) 2>&1)" || rc=$?
echo "    canon_root with startup.conf behind a symlink to /etc → rc=$rc: $out"
ok '[[ $rc == 2 ]] && grep -q "VRX_TEST_ROOT is only honoured when" <<<"$out"' "startup.conf reached through a symlink to /etc refused (exit $rc)"
rm -f "$TOP/esc"
rc=0; out="$( (. "$SCRIPT"; canon_root) 2>&1)" || rc=$?
ok '[[ $rc == 0 && $out == "$(realpath -m "$TOP")/canon" ]]' "the harness's own test root is still accepted"
fi
if scen 39; then
echo "== 39. V7: the lock holder keeps the locks until the run's recomputed deadline (hold-until), not its own default-count HOLD_MAX"
setup
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 0 && -s $W/hold-until ]] && (. "$SCRIPT"; WORK="$W"; load_settings >/dev/null; (( $(cat "$W/hold-until") >= $(stat -c %Y "$W/hold-until") + DEADMAN_AFTER + 2 * DEADMAN_LOCK_TIMEOUT + RB_BUDGET )))' "the run records hold-until ≥ its dead-man deadline + lock wait + one rollback"
S="$T/apply/syn"; mkdir -p "$S/bin"; cp "$W/settings" "$W/gen-args" "$S/"; cp "$W/bin/apply-startup.sh" "$S/bin/"
setsid "$S/bin/apply-startup.sh" --stage hold --work "$S" > /dev/null 2>&1 & SH=$!; PIDS+=("$SH")
waitfor "$S/locks-held" 100 || true
echo $((EPOCHSECONDS + 2)) > "$S/hold-until"
for ((i = 0; i < 100; i++)); do kill -0 "$SH" 2>/dev/null || break; sleep 0.2; done
ok '! kill -0 "$SH" 2>/dev/null && locks_free && grep -q "lock holder: hold-until reached" "$S/log"' "a holder follows hold-until (released after ~2 s)"
for n in 1 3; do
  read -r hm de < <(. "$SCRIPT"; WORK="$T/syn$n"; mkdir -p "$WORK/mgmt"; : > "$WORK/mgmt.ifs"; budgets; h=$HOLD_MAX
    for ((j = 0; j < n; j++)); do echo "eth$j" >> "$WORK/mgmt.ifs"; printf 'a\nb\nc\nd\n' > "$WORK/mgmt/eth$j.restore"; echo 10.0.0.1 > "$WORK/mgmt/eth$j.gateways"; done
    printf '0000:00:0%s.0 x\n' 1 2 3 4 5 6 7 > "$WORK/drivers"; budgets; echo "$h $((DEADMAN_AFTER + 2 * DEADMAN_LOCK_TIMEOUT + RB_BUDGET))")
  echo "    defaults: $n management interface(s): holder's default HOLD_MAX ${hm}s, dead-man worst-case end ${de}s"
done
fi
if scen 40; then
echo "== 40. V8: neigh probe — a link-local IPv6 default gateway is nudged with its scope; a hanging \`ip neigh\` is bounded by time"
setup
touch "$T/state/v6-default"
rc=0; out="$("$SCRIPT" --doc "$DOCF" --cmd-timeout "$CT" "${APPROVE[@]}" "${HOSTARGS[@]}" 2>&1)" || rc=$?
echo "    $(grep -E "^  (will use:|NONE VIABLE)" <<<"$out")"
ok '[[ $rc == 0 ]] && grep -q "will use: neigh (passes now)" <<<"$out" && grep -q "tcpconnect fe80::1%ens192 9" "$T/calls"' "dual stack, gateway fe80::1: nudged as fe80::1%ens192, neigh viable (exit $rc)"
setup
touch "$T/state/neigh-hang"
rc=0; out="$("$SCRIPT" --doc "$DOCF" --cmd-timeout 3 "${APPROVE[@]}" "${HOSTARGS[@]}" 2>&1)" || rc=$?
echo "    elapsed $(elapsed)s: $(grep -E "NONE VIABLE" <<<"$out" | cut -c1-120)"
ok '[[ $rc == 3 ]] && grep -q "is not REACHABLE" <<<"$out"' "ip neigh hangs (cmd-timeout 3s): refused (exit $rc)"
bounded 10 "the pre-V8 poll bounded by count: 2×cmd-timeout reads × (3 s + 0.5 s) = 21 s + the same load-scaled overhead (28–29 s measured)"
fi
if scen 41; then
echo "== 41. V9: the dry run exits 3 when --apply would be refused by the gate, 0 when the approval covers this rendering"
setup
rc=0; out="$("$SCRIPT" --doc "$DOCF" --cmd-timeout "$CT" "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 3 ]] && grep -q "REFUSED: docs/lab/host-vrx-a.md says handover: pending" <<<"$out" && grep -qx "$NEW" <<<"$out"' "handover pending, no approval: exit $rc (sums still printed)"
rc=0; out="$("$SCRIPT" --doc "$DOCF" --cmd-timeout "$CT" "${APPROVE[@]}" "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 0 ]] && grep -q "PRODUCT-OWNER APPROVAL PENDING-fake-change" <<<"$out"' "approval names this rendering: exit $rc"
fi

echo
echo "apply-startup tests: $PASS passed, $FAIL failed"
((FAIL == 0))
