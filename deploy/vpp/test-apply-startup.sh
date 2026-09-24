#!/usr/bin/env bash
# shellcheck disable=SC2016,SC2034,SC2119,SC2120  # apply() takes optional extra flags; ok() evaluates its single-quoted condition later (uses $out, $rc, $W, …)
# deploy/vpp/test-apply-startup.sh — exercises apply-startup.sh against a FAKE host: fake systemctl (MainPID,
# ActiveEnterTimestampMonotonic, NRestarts reset on restart — as systemd does), systemd-run, vrx-vppcheck
# (incl. the D-080 boot identity), ip, ss, ping (a gateway that may drop ICMP), a TCP prober, driverctl,
# ifup / networkctl / netplan, logger, lslocks, a fake sysfs tree, a fake /etc (network configuration only),
# temp locks and a temp startup.conf. Nothing on the real host is changed (no VPP, no /etc, no /sys, no real
# locks); the only real files read are on main of /root/ngfw: the handover flag, docs/decisions/PENDING-handover.md
# and LOG.md (the approval check), read-only.
#
#   deploy/vpp/test-apply-startup.sh [path/to/vrx-startupgen]
#
# Without an argument the generator is built from apps/agent into a temp dir. Every process a scenario
# starts is killed by PID (never by pattern).
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
SCRIPT="$HERE/apply-startup.sh"
FIX="$REPO/apps/agent/internal/renderers/vppstartup/testdata"
TOP="$(mktemp -d)"
PIDS=()
kill_recorded() { local p; for p in $(cat "$TOP"/case.*/state/pids 2>/dev/null || true); do kill -KILL "$p" 2>/dev/null || true; done; }
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
ok() { if eval "$1"; then PASS=$((PASS + 1)); echo "  ok   $2"; else FAIL=$((FAIL + 1)); echo "  FAIL $2"; fi; }
elapsed() { echo $((SECONDS - T0)); }

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
  show) if [[ " $* " == *" -p MainPID "* ]]; then echo "MainPID=$(cat "$S/mainpid")"; echo "ActiveEnterTimestampMonotonic=$(cat "$S/active_enter")"; else cat "$S/nrestarts"; fi ;;
  restart|start) if [[ -x $FAKE/hooks/$1 ]]; then "$FAKE/hooks/$1"; fi
                 echo active > "$S/vpp"; echo 0 > "$S/nrestarts"; [[ -e $S/no-restart ]] || newpid ;;
  stop) if [[ $2 == vpp ]]; then echo inactive > "$S/vpp"; echo 0 > "$S/mainpid"; rm -f "$S/hang" "$S/hang-after" "$S/crash-after"; fi ;;
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
  "-j -6 route show default") echo '[]' ;;
  "-j route get "*) echo "[{\"dst\":\"$4\",\"dev\":\"ens192\",\"prefsrc\":\"10.0.0.5\",\"flags\":[],\"uid\":0,\"cache\":[]}]" ;;
  "-j addr show dev ens192") addr_json ;;
  "-j -4 route show table main dev ens192")
    r=(); [[ -e $S/route-default ]] && r+=('{"dst":"default","gateway":"10.0.0.1","flags":["onlink"]}')
    grep -q '^10.0.0.5 ' "$S/addrs" && r+=('{"dst":"10.0.0.0/24","protocol":"kernel","scope":"link","prefsrc":"10.0.0.5","flags":[]}')
    (IFS=,; echo "[${r[*]}]") ;;
  "-j -6 route show table main dev ens192") echo '[{"dst":"fe80::/64","protocol":"kernel","metric":256,"flags":[],"pref":"medium"}]' ;;
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
    VRX_LOGGER="$T/bin/logger" VRX_VPPCHECK="$T/bin/vrx-vppcheck" VRX_STARTUPGEN="$GEN" VRX_HANDOVER_EXTRA="$T/host-pending.md" \
    VRX_MGMT_PEERS="" VRX_MGMT_IFS="" VRX_MGMT_PROBE="auto"
  unset SSH_CONNECTION
  DOCF="$FIX/cases/six-nic-sample.json"
  HOSTARGS=(-- --no-host --mgmt-pci 0000:0b:00.0 --plugin-dir "$T/plugins" --online-cpus 0-31 --numa-nodes 2 --hugepages-mb 2048)
  ORIG="$(sha256sum "$T/etc/vpp/startup.conf" | awk '{print $1}')"
  NEW="$(rendered "$DOCF")"
  T0=$SECONDS
}
rendered() { "$GEN" --current "$VRX_STARTUP_CONF" "${HOSTARGS[@]:1}" "$1" 2>/dev/null | sha256sum | awk '{print $1}'; }
TIMING=(--window 2 --interval 1 --api-wait 2 --lock-timeout 2 --cmd-timeout 1 --svc-timeout 3 --deadman-lock-timeout 1 --rollback-retries 2 --retry-backoff 1)
APPROVE=(--i-have-product-owner-approval PENDING-handover)   # real: docs/decisions/PENDING-handover.md + LOG D-060 on main
apply() { "$SCRIPT" --doc "$DOCF" --apply --foreground "${TIMING[@]}" "${APPROVE[@]}" --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "$@" "${HOSTARGS[@]}"; }
unchanged() { [[ $(sha256sum "$T/etc/vpp/startup.conf" | awk '{print $1}') == "$ORIG" ]]; }
work() { find "$T/apply" -mindepth 1 -maxdepth 1 -type d | head -1; }
hook() { printf '#!/usr/bin/env bash\n%s\n' "${2//\$FAKE/$T}" > "$T/hooks/$1"; chmod +x "$T/hooks/$1"; }  # <restart|start> <body>
steal_mgmt_nic() {  # VPP takes the management NIC → vfio-pci; the kernel netdev comes back without addresses/route
  hook restart 'ln -sfn "$FAKE/sys/bus/pci/drivers/vfio-pci" "$FAKE/sys/bus/pci/devices/0000:0b:00.0/driver"
: > "$FAKE/state/addrs"; rm -f "$FAKE/state/route-default"'
}
waitfor() { local i; for ((i = 0; i < ${2:-100}; i++)); do [[ -e $1 ]] && return 0; sleep 0.2; done; return 1; }  # <file> [tries]
locks_free() { flock -n -x "$T/vpp.lock" true && flock -n -x "$T/lab.lock" true; }

echo "== 1. dry run (default) changes nothing; prints diffs, drivers, management restore plan, reachability check, preflight, gate, both sha256"
setup
rc=0; out="$(TMPDIR="$T/tmp" "$SCRIPT" --doc "$DOCF" --cmd-timeout 1 "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 0 ]] && grep -q "^+  dev 0000:04:00.0 {" <<<"$out"' "exit $rc; unified diff shows the new dev lines"
ok 'grep -q "^+ dpdk > dev 0000:04:00.0 > name wan" <<<"$out"' "semantic diff shows the logical names"
ok 'grep -q "0000:0b:00.0 vmxnet3" <<<"$out"' "drivers of the PCI devices involved are listed"
ok 'grep -q "ens192 pci=0000:0b:00.0 driver=vmxnet3 manager=ifupdown addrs=10.0.0.5/24 2001:db8::5/64" <<<"$out"' "management interface, ifupdown detected, addresses recorded"
ok 'grep -q "ip addr replace 10.0.0.5/24 broadcast 10.0.0.255 dev ens192" <<<"$out" && grep -q "ip -4 route replace default via 10.0.0.1 dev ens192 onlink" <<<"$out"' "exact restore plan printed (address + default route)"
ok '! grep -q "fe80\|proto kernel" <<<"$(sed -n "/restore plan/,/reachability/p" <<<"$out")"' "link-local address and kernel routes are not in the plan"
ok 'grep -q "will use: gateway-ping (passes now)" <<<"$out"' "reachability check chosen and shown: gateway-ping"
ok 'grep -q "present: local0" <<<"$out" && grep -q "boot identity: fake-boot/1000/5000" <<<"$out" && grep -q "REFUSED: docs/lab/host-vrx-a.md says handover: pending" <<<"$out"' "VPP preflight + boot identity; gate state shown"
ok 'grep -qx "$ORIG" <<<"$out" && grep -qx "$NEW" <<<"$out"' "sha256 of the live file and of the rendering printed"
ok 'unchanged && ! grep -qE "systemctl (restart|stop|start)|addr replace" "$T/calls"' "live file untouched, VPP not restarted, no ip change"
ok '[[ -z $(ls -A "$T/tmp") ]]' "dry run removed its temp dir (L3)"

echo "== 2. dry run: SSH peer shown and used; no viable reachability check → exit 3 (review H2, L2)"
setup
touch "$T/state/no-icmp" "$T/state/ssh-est"
rc=0; out="$(SSH_CONNECTION="10.0.0.9 51234 10.0.0.5 22" "$SCRIPT" --doc "$DOCF" --cmd-timeout 1 "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 0 ]] && grep -q "manager peer(s): 10.0.0.9" <<<"$out" && grep -q "will use: ssh-peer (passes now)" <<<"$out"' "exit $rc; peer 10.0.0.9 shown; gateway drops ICMP → ssh-peer chosen"
rm "$T/state/ssh-est"
rc=0; out="$("$SCRIPT" --doc "$DOCF" --cmd-timeout 1 "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 3 ]] && grep -q "NONE VIABLE — --apply will refuse: no viable management reachability check" <<<"$out"' "no peer, no ICMP: exit $rc, refused early"
rc=0; out="$("$SCRIPT" --doc "$DOCF" --cmd-timeout 1 --mgmt-probe tcp:10.0.0.1:22 "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 0 ]] && grep -q "will use: tcp:10.0.0.1:22 (passes now)" <<<"$out"' "--mgmt-probe tcp:10.0.0.1:22 viable (exit $rc)"
rc=0; out="$("$SCRIPT" --doc "$DOCF" --cmd-timeout 1 --mgmt-probe gateway-ping "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 3 ]] && grep -q "does not pass now: gateway 10.0.0.1 on ens192 does not answer ICMP" <<<"$out"' "explicit gateway-ping on a no-ICMP gateway: exit $rc"

echo "== 3. usage errors → exit 2, nothing changed"
setup
rc=0; "$SCRIPT" --doc "$DOCF" --apply --foreground "${APPROVE[@]}" --expect-sha256 "$ORIG" "${HOSTARGS[@]}" >/dev/null 2>&1 || rc=$?
ok '[[ $rc == 2 ]] && unchanged' "--apply without --expect-new-sha256 refused (exit $rc)"
rc=0; "$SCRIPT" --doc "$DOCF" --apply --i-have-product-owner-approval "D-060" --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" >/dev/null 2>&1 || rc=$?
ok '[[ $rc == 2 ]] && unchanged' "approval id that is not PENDING-<slug> refused (exit $rc)"
rc=0; SSH_CONNECTION="10.0.0.9 1 10.0.0.5 22" "$SCRIPT" --doc "$DOCF" --apply --foreground "${APPROVE[@]}" --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 2 ]] && unchanged && grep -q "foreground over SSH" "$T/out"' "--foreground over SSH without --console refused (exit $rc)"

echo "== 4. handover gate: pending → refused; approval verified on main (PENDING file + LOG D-row); unknown PENDING → refused"
setup
rc=0; out="$("$SCRIPT" --doc "$DOCF" --apply --foreground "${TIMING[@]}" --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 3 ]] && unchanged && [[ -z $(work) ]] && grep -q "REFUSED: docs/lab/host-vrx-a.md says handover: pending" <<<"$out"' "no approval: exit $rc, no work dir"
rc=0; out="$("$SCRIPT" --doc "$DOCF" --apply --foreground "${TIMING[@]}" --i-have-product-owner-approval PENDING-no-such-decision --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 3 ]] && unchanged && [[ -z $(work) ]] && grep -q "docs/decisions/PENDING-no-such-decision.md does not exist on main" <<<"$out"' "unknown PENDING id: exit $rc"
ok '! grep -q "systemctl" "$T/calls"' "systemd never touched"
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 0 ]] && grep -q "PRODUCT-OWNER APPROVAL PENDING-handover (docs/decisions/PENDING-handover.md@[0-9a-f]\{12\} + LOG .*D-060" "$W/gate" "$W/log"' "approval resolved on main (file blob + D-060), recorded in gate + log (exit $rc)"
ok 'grep -q "logger -t vrx-startup-apply -- gate: handover pending.*PENDING-handover" "$T/calls"' "approval sent to syslog"
ok '[[ $( (. "$SCRIPT"; printf "x\n\`handover: done\`\n" | handover_flag) ) == done && $( (. "$SCRIPT"; printf "handover: pending\n" | handover_flag) ) == pending && $( (. "$SCRIPT"; printf "nothing\n" | handover_flag) ) == pending ]]' "handover flag parser (tools/lab rule): done / pending / absent → pending"
ok '[[ $( (. "$SCRIPT"; ere_escape "ens192.10") ) == "ens192\\.10" ]]' "interface names are regex-escaped (L3)"

echo "== 5. --stage run verifies the sealed plan and re-evaluates the gate (review M2)"
setup
apply >/dev/null 2>&1
W="$(work)"
for f in committed installed backup.conf log; do rm -f "$W/$f"; done
cp "$T/etc/vpp/startup.conf" "$T/after"; cp "$FIX/host-startup.conf" "$T/etc/vpp/startup.conf"
echo "gate: handover done (forged)" > "$W/gate"
rc=0; "$W/bin/apply-startup.sh" --stage run --work "$W" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && grep -q "REFUSED: plan seal mismatch" "$T/out"' "forged gate file: exit $rc, refused, nothing installed"
sed -i 's/^WINDOW=.*/WINDOW=1/' "$W/settings"
rc=0; "$W/bin/apply-startup.sh" --stage run --work "$W" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && grep -q "REFUSED: plan seal mismatch" "$T/out"' "altered settings: exit $rc, refused"

echo "== 6. what was reviewed is pinned: live file or rendering changed → refused inside the lock"
setup
rc=0; "$SCRIPT" --doc "$DOCF" --apply --foreground "${TIMING[@]}" "${APPROVE[@]}" --expect-sha256 "$(printf '%064d' 0)" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && ! grep -q "systemctl restart" "$T/calls" && grep -q "changed since the review" "$T/out" && locks_free' "live file changed: exit $rc, no restart, locks released"
rc=0; "$SCRIPT" --doc "$DOCF" --apply --foreground "${TIMING[@]}" "${APPROVE[@]}" --expect-sha256 "$ORIG" --expect-new-sha256 "$(printf '%064d' 1)" "${HOSTARGS[@]}" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && ! grep -q "systemctl restart" "$T/calls" && grep -q "rendering differs from the reviewed one" "$T/out"' "rendering changed: exit $rc, no restart"

echo "== 7. VPP hung BEFORE the apply → preflight refuses within the timeout, nothing installed"
setup
touch "$T/state/hang"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && [[ ! -e $(work)/installed ]] && grep -q "REFUSED: VPP preflight failed" "$T/out" && locks_free' "exit $rc, refused, locks released"
ok '(( $(elapsed) < 15 ))' "bounded: $(elapsed)s"

echo "== 8. healthy apply commits — NRestarts 4 before, reset to 0 by the restart (review H1)"
setup
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 0 && -e $W/committed && ! -e $W/rolled-back ]] && [[ $(cat "$T/state/nrestarts") == 0 ]]' "exit $rc, committed although NRestarts went 4 → 0"
ok '[[ $(cat "$W/ident.before") == fake-boot/1000/5000 && $(cat "$W/ident.after") == fake-boot/1001/5100 ]] && grep -q "MainPID=1001" "$W/unit.after"' "new boot identity, PID = vpp.service MainPID"
ok 'cmp -s "$T/etc/vpp/startup.conf" "$W/new.conf" && grep -q "name wan" "$T/etc/vpp/startup.conf"' "new file installed"
ok 'cmp -s "$W/backup.conf" "$FIX/host-startup.conf" && ls "$T/etc/vpp/" | grep -q "startup.conf.bak-"' "backup kept (work dir + next to the file)"
ok 'grep -q "vppcheck ifaces dmz lan lan2 p2p sync wan" "$T/calls" && grep -q "vppcheck plugins" "$T/calls"' "logical interfaces + plugins verified through vrx-vppcheck"
ok 'grep -q "^0000:0b:00.0 vmxnet3$" "$W/drivers" && grep -q "^0000:04:00.0 vmxnet3$" "$W/drivers"' "drivers recorded before the restart"
ok '[[ $(cat "$W/mgmt.ifs") == ens192 && $(cat "$W/mgmt/ens192.netmgr") == ifupdown && $(cat "$W/probe") == gateway-ping ]]' "management snapshot: ens192, ifupdown; check gateway-ping"
ok '[[ $(grep -n "locks held by holder" "$T/out" | cut -d: -f1) -lt $(grep -n "backup:" "$T/out" | cut -d: -f1) ]]' "locks taken (holder) before backup and diff"
ok 'grep -q "systemd-run --unit=vrx-startup-apply-deadman-.* --on-active=.*--setenv=VRX_STARTUP_CONF=$T/etc/vpp/startup.conf.* --stage rollback" "$T/calls" && grep -q "systemd-run --unit=vrx-startup-apply-lock-.* --stage hold" "$T/calls"' "dead-man timer and lock holder started with the settings as --setenv"
ok 'grep -q "systemctl stop vrx-startup-apply-deadman-.*\.timer" "$T/calls" && locks_free' "dead-man timer cancelled, locks released after commit"
ok 'read -r a r b < <(sed -nE "s/.*rollback in ([0-9]+)s .*run budget ([0-9]+)s, rollback budget ([0-9]+)s.*/\1 \2 \3/p" "$T/out"); (( a > r + b ))' "dead-man deadline > run budget + rollback budget (review M3)"

echo "== 9. VPP did not actually restart (same boot identity) → rollback"
setup
touch "$T/state/no-restart"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && grep -q "ROLLBACK: VPP did not restart (boot identity still fake-boot/1000/5000)" "$T/out"' "exit $rc, detected"

echo "== 10. VPP crashes during the window (systemd brings it back) → rollback"
setup
hook restart 'echo 2 > "$FAKE/state/crash-after"'   # crashes at the first bootid probe of the window
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 && -e $(work)/rolled-back ]] && unchanged && grep -qE "ROLLBACK: (vpp.service restarted since the apply|VPP crashed and came back)" "$T/out"' "exit $rc, crash detected by MainPID/ActiveEnter/boot identity"

echo "== 11. gateway drops ICMP (vrx-a): auto check = the manager's SSH session → commits (review H2)"
setup
touch "$T/state/no-icmp" "$T/state/ssh-est"; export VRX_MGMT_PEERS="10.0.0.9"
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 0 && -e $W/committed ]] && [[ $(cat "$W/probe") == ssh-peer ]] && grep -q "ss -Htn state established dst 10.0.0.9" "$T/calls"' "exit $rc, committed with ssh-peer"

echo "== 12. gateway drops ICMP, --mgmt-probe tcp:… → commits; path lost → rollback verified by the same probe"
setup
touch "$T/state/no-icmp"
rc=0; apply --mgmt-probe tcp:10.0.0.1:22 > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 0 && -e $(work)/committed ]] && grep -q "tcpconnect 10.0.0.1 22" "$T/calls"' "exit $rc, committed with a TCP probe"
setup
touch "$T/state/no-icmp"
hook restart 'touch "$FAKE/state/tcp-down"'
hook start 'rm -f "$FAKE/state/tcp-down"'
rc=0; apply --mgmt-probe tcp:10.0.0.1:22 > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 && -e $(work)/rolled-back ]] && unchanged && grep -q "ROLLBACK: management path: TCP connect to 10.0.0.1 port 22 failed" "$T/out"' "exit $rc, lost TCP path → rollback, healthy afterwards"

echo "== 13. no viable check at apply time → refused before anything changes"
setup
touch "$T/state/no-icmp"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && [[ ! -e $(work)/installed ]] && grep -q "no viable management reachability check" "$T/out" && locks_free' "exit $rc, refused, locks released"

echo "== 14. a logical interface missing in VPP → rollback"
setup
grep -v '^lan2$' "$T/state/ifaces" > "$T/state/i2" && mv "$T/state/i2" "$T/state/ifaces"
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 1 && -e $W/rolled-back && ! -e $W/committed ]] && unchanged && locks_free' "exit $rc, rolled back, original file restored, locks released"
ok 'grep -q "ROLLBACK: logical interface(s) not in VPP (API check): missing: lan2" "$T/out"' "reason logged"
ok 'grep -q "systemctl stop vpp" "$T/calls" && grep -q "systemctl reset-failed vpp" "$T/calls" && grep -q "systemctl start vpp" "$T/calls"' "VPP stopped, reset-failed (N11), started on the old file"

echo "== 15. ifupdown host (vrx-a), no netplan: VPP steals the management NIC → rebind + EXACT address/route restore"
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

echo "== 16. ifupdown, exact re-apply not enough → ifup --force ens192"
setup
steal_mgmt_nic; touch "$T/state/ip-readonly"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && grep -q "ifup --force ens192" "$T/calls" && [[ -e $(work)/rolled-back ]]' "exit $rc, ifup --force used, rollback healthy"

echo "== 17. systemd-networkd host → networkctl reconfigure"
setup
rm -f "$T/etc/network/interfaces.d/ens192.cfg"; touch "$T/state/networkd"
steal_mgmt_nic; touch "$T/state/ip-readonly"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && [[ $(cat "$(work)/mgmt/ens192.netmgr") == networkd ]] && grep -q "networkctl reconfigure ens192" "$T/calls" && [[ -e $(work)/rolled-back ]]' "exit $rc, networkd detected, reconfigure used, rollback healthy"

echo "== 18. netplan host → netplan apply"
setup
rm -f "$T/etc/network/interfaces.d/ens192.cfg"; mkdir -p "$T/etc/netplan"; echo "network: {version: 2}" > "$T/etc/netplan/50-cloud-init.yaml"
export VRX_NETPLAN="$T/bin/netplan"
steal_mgmt_nic; touch "$T/state/ip-readonly"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && [[ $(cat "$(work)/mgmt/ens192.netmgr") == netplan ]] && grep -q "netplan apply" "$T/calls" && [[ -e $(work)/rolled-back ]]' "exit $rc, netplan detected, netplan apply used, rollback healthy"

echo "== 19. a plugin the new file enables is not loaded → rollback"
setup
grep -v npt66 "$T/state/plugins" > "$T/state/p2" && mv "$T/state/p2" "$T/state/plugins"
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && grep -q "plugin npt66_plugin.so enabled but not loaded" "$T/out"' "exit $rc, reason: npt66 not loaded"

echo "== 20. D-084 omission (re-review N6): the new document drops npt66 → it may vanish; the apply commits"
setup
jq 'del(.dataplane.plugins.switches["npt66_plugin.so"])' "$DOCF" > "$T/no-npt66.json"
DOCF="$T/no-npt66.json"; NEW="$(rendered "$DOCF")"
hook restart 'grep -v npt66 "$FAKE/state/plugins" > "$FAKE/state/p2" && mv "$FAKE/state/p2" "$FAKE/state/plugins"'
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 0 && -e $(work)/committed ]] && ! grep -q npt66 "$T/etc/vpp/startup.conf"' "exit $rc, committed without npt66"

echo "== 21. VPP HANGS after the restart (socket accepted, no answer) → bounded wait, rollback"
setup
hook restart 'touch "$FAKE/state/hang"'
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 1 && -e $W/rolled-back ]] && unchanged && grep -q "ROLLBACK: VPP API did not come up within 2s (hung or crashed)" "$T/out"' "exit $rc, rolled back"
ok '(( $(elapsed) < 30 ))' "bounded: $(elapsed)s (cmd-timeout 1s, api-wait 2s)"

echo "== 22. VPP hangs in the middle of the watch window → rollback"
setup
hook restart 'echo 3 > "$FAKE/state/hang-after"'
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 && -e $(work)/rolled-back ]] && unchanged && grep -qE "ROLLBACK: (VPP API does not answer|VPP did not list its plugins|logical interface|VPP boot identity unreadable)" "$T/out"' "exit $rc, hang detected, rolled back"
ok '(( $(elapsed) < 30 ))' "bounded: $(elapsed)s"

echo "== 23. systemctl restart itself hangs → svc timeout, rollback"
setup
hook restart 'echo $$ >> "$FAKE/state/pids"; exec sleep 1000'
rc=0; apply > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 && -e $(work)/rolled-back ]] && unchanged && grep -q "restart vpp failed or hung for 3s" "$T/out"' "exit $rc, rolled back"
ok '(( $(elapsed) < 30 ))' "bounded: $(elapsed)s"

echo "== 24. SSH session dies mid-apply (detached through systemd-run, clean environment) → the run still commits"
setup
hook restart 'sleep 2'
setsid bash -c '"$0" --doc "$1" --apply "${@:2}"; sleep 60' "$SCRIPT" "$DOCF" "${TIMING[@]}" "${APPROVE[@]}" --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" > "$T/caller.out" 2>&1 &
SSH=$!; PIDS+=("$SSH")
sleep 0.5; W="$(work)"
waitfor "$W/installed" 50 || true
kill -KILL -- "-$SSH" 2>/dev/null || true   # the whole "SSH session" process group dies while VPP restarts
sleep 0.2
ok '[[ -e $W/installed && ! -e $W/committed ]] && ! kill -0 "$SSH" 2>/dev/null' "session killed after install, before commit"
waitfor "$W/committed" 100 || true
ok '[[ -e $W/committed ]] && cmp -s "$T/etc/vpp/startup.conf" "$W/new.conf"' "detached run committed anyway"
ok 'grep -q "systemd-run --unit=vrx-startup-apply-[0-9-]* --collect --quiet --property=KillMode=process --setenv=VRX_STARTUP_CONF=.*--setenv=VRX_STARTUPGEN=$GEN .*--setenv=PATH=.*/bin/apply-startup.sh --stage run --work $W" "$T/calls"' "unit started with every setting as --setenv"
ok '! grep -q "WARNING: environment" "$W/log" && grep -q "journalctl -fu vrx-startup-apply-" "$T/caller.out"' "the unit saw exactly the recorded settings under env -i"

echo "== 25. SSH session dies mid-apply, systemd-run unavailable → setsid fallback survives too"
setup
touch "$T/state/systemd-run-fail"
hook restart 'sleep 2'
setsid bash -c '"$0" --doc "$1" --apply "${@:2}"; sleep 60' "$SCRIPT" "$DOCF" "${TIMING[@]}" "${APPROVE[@]}" --expect-sha256 "$ORIG" --expect-new-sha256 "$NEW" "${HOSTARGS[@]}" > "$T/caller.out" 2>&1 &
SSH=$!; PIDS+=("$SSH")
sleep 0.5; W="$(work)"
waitfor "$W/installed" 50 || true
kill -KILL -- "-$SSH" 2>/dev/null || true
if [[ -f $W/run-pid ]]; then read -r RUNPID _ < "$W/run-pid"; echo "$RUNPID" >> "$T/state/pids"; fi
waitfor "$W/committed" 100 || true
ok '[[ -e $W/committed ]] && grep -q "started with setsid" "$T/caller.out" && cmp -s "$T/etc/vpp/startup.conf" "$W/new.conf"' "setsid run committed after the session died"

echo "== 26. the run HANGS mid-apply; an integration run queues on the lab lock → dead-man kills the run, the locks never go free until the rollback is done (review M1)"
setup
hook restart 'echo $$ >> "$FAKE/state/pids"; exec sleep 1000'
TIMING_SAVE=("${TIMING[@]}"); TIMING=(--window 2 --interval 1 --api-wait 2 --lock-timeout 2 --cmd-timeout 1 --svc-timeout 600 --deadman-lock-timeout 5 --rollback-retries 2 --retry-backoff 1)
apply > "$T/run.out" 2>&1 & RUNNER=$!; PIDS+=("$RUNNER")
TIMING=("${TIMING_SAVE[@]}")
sleep 0.5; W="$(work)"; waitfor "$W/installed" 50 || true; sleep 0.3
read -r RUNPID _ < "$W/run-pid"; PIDS+=("$RUNPID"); HOLD="$(cat "$W/locks-held")"
ok 'kill -0 "$RUNPID" && kill -0 "$HOLD" && ! flock -n -x "$T/vpp.lock" true' "run stuck in 'systemctl restart'; holder $HOLD owns the locks"
flock -s "$T/lab.lock" -c "echo WAITER GOT THE LAB LOCK >> $T/calls" & WAITER=$!; PIDS+=("$WAITER")
sleep 0.3
T0=$SECONDS
rc=0; "$W/bin/apply-startup.sh" --stage rollback --work "$W" > "$T/out" 2>&1 || rc=$?
wait "$WAITER" 2>/dev/null || true
ok '[[ $rc == 1 && -e $W/rolled-back ]] && unchanged && ! kill -0 "$RUNPID" 2>/dev/null' "exit $rc, run killed, file restored"
ok 'grep -q "dead-man: lock holder $HOLD still owns the locks" "$T/out" && grep -q "dead-man: killing the run (pid $RUNPID)" "$T/out" && ! grep -q FORCED "$T/out"' "holder kept the locks across the kill; not FORCED"
ok '[[ $(grep -n "^systemctl start vpp" "$T/calls" | tail -1 | cut -d: -f1) -lt $(grep -n "WAITER GOT THE LAB LOCK" "$T/calls" | cut -d: -f1) ]]' "queued shared waiter got the lab lock only after the rollback restarted VPP"
ok 'locks_free && ! kill -0 "$HOLD" 2>/dev/null' "locks released at the end, holder gone"
ok '(( $(elapsed) < 20 ))' "bounded: $(elapsed)s"

echo "== 27. dead-man: no-op after commit; rollback when the run died before committing"
setup
apply >/dev/null 2>&1
W="$(work)"
out="$("$W/bin/apply-startup.sh" --stage rollback --work "$W" 2>&1)"
ok 'grep -q "nothing to do" <<<"$out" && ! unchanged' "committed run: dead-man does nothing"
rm "$W/committed"
rc=0; "$W/bin/apply-startup.sh" --stage rollback --work "$W" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && grep -q "dead-man (attempt 1/2)" "$T/out" && [[ -e $W/rolled-back ]] && grep -q "took the locks exclusively before touching the run" "$T/out" && locks_free' "uncommitted run, holder gone: dead-man took the locks itself, restored the backup (exit $rc)"

echo "== 28. dead-man, holder gone AND a foreign process holds the lab lock forever → bounded wait, FORCED rollback, holder logged"
setup
apply >/dev/null 2>&1
W="$(work)"; rm "$W/committed"
flock -x "$T/lab.lock" sleep 300 & HOLDER=$!; PIDS+=("$HOLDER")
sleep 0.3
T0=$SECONDS
rc=0; "$W/bin/apply-startup.sh" --stage rollback --work "$W" > "$T/out" 2>&1 || rc=$?
kill "$HOLDER" 2>/dev/null || true
ok '[[ $rc == 1 && -e $W/rolled-back ]] && unchanged && grep -q "FORCED — locks held by someone else for 1s (.*lslocks: 4242 flock WRITE $T/lab.lock" "$T/out"' "exit $rc, rolled back without the lock, holder named"
ok '(( $(elapsed) < 15 ))' "bounded: $(elapsed)s"

echo "== 29. lab lock held by someone else at apply time → refused, nothing changed"
setup
flock -x "$T/lab.lock" sleep 6 & HOLDER=$!; PIDS+=("$HOLDER")
sleep 0.3
rc=0; apply > "$T/out" 2>&1 || rc=$?
kill "$HOLDER" 2>/dev/null || true
ok '[[ $rc == 3 ]] && unchanged && grep -q "lab.lock held by: 4242 flock WRITE" "$T/out" && ! grep -q "systemctl restart" "$T/calls"' "exit $rc, lock busy (holder named), file unchanged"

echo "== 30. timer unavailable → setsid dead-man; stopped on commit; locks free"
setup
touch "$T/state/timer-fail"
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"; DM="$(cat "$W/deadman-pid")"; echo "$DM" >> "$T/state/pids"
ok '[[ $rc == 0 && -e $W/committed ]] && grep -q "dead-man started with setsid" "$T/out"' "exit $rc, committed, fallback dead-man used"
ok 'locks_free' "both locks free right after the commit"
ok 'sleep 0.3; ! kill -0 "$DM" 2>/dev/null' "fallback dead-man stopped by the commit"

echo "== 31. rollback cannot restore the management path → incomplete, locks STAY held, dead-man retries with backoff"
setup
rm -f "$T/etc/network/interfaces.d/ens192.cfg"
steal_mgmt_nic; touch "$T/state/ip-readonly"
rc=0; apply > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 1 && -e $W/rollback-incomplete && ! -e $W/rolled-back ]] && ! grep -q "systemctl stop vrx-startup-apply-deadman-.*\.timer" "$T/calls" && ! locks_free' "exit $rc, incomplete, timer NOT cancelled, locks still held"
rc=0; "$W/bin/apply-startup.sh" --stage rollback --work "$W" > "$T/out" 2>&1 || rc=$?
ok '[[ ! -e $W/rolled-back ]] && grep -q "dead-man (attempt 2/2)" "$T/out" && grep -q "retry in 1s" "$T/out" && grep -q "INCOMPLETE after 2 attempts — console access needed; releasing the locks" "$T/out" && locks_free' "still broken: 2 bounded attempts with backoff, then gives up and releases"
setup
rm -f "$T/etc/network/interfaces.d/ens192.cfg"
steal_mgmt_nic; touch "$T/state/ip-readonly"
apply >/dev/null 2>&1 || true
W="$(work)"
rm -f "$T/state/ip-readonly"
rc=0; "$W/bin/apply-startup.sh" --stage rollback --work "$W" > "$T/out" 2>&1 || rc=$?
ok '[[ -e $W/rolled-back ]] && unchanged && grep -q "management path are healthy" "$T/out" && locks_free' "path fixable: dead-man retry restored it and released the locks"

echo "== 32. the generator refuses the management NIC as a device (host facts) → nothing changed"
setup
echo '{"dataplane":{"managementPci":["0000:04:00.0"],"devices":{"0000:0b:00.0":{"name":"lan"}}}}' > "$T/bad.json"
rc=0; "$SCRIPT" --doc "$T/bad.json" "${HOSTARGS[@]}" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 2 ]] && unchanged && grep -q "does not match the host" "$T/out"' "dry run fails with the generator's error (exit $rc)"

echo
echo "apply-startup tests: $PASS passed, $FAIL failed"
((FAIL == 0))
