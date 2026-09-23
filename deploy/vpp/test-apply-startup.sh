#!/usr/bin/env bash
# shellcheck disable=SC2016,SC2034  # ok() evaluates its single-quoted condition later (uses $out, $rc, $W)
# deploy/vpp/test-apply-startup.sh — exercises apply-startup.sh against a FAKE host: fake
# systemctl / systemd-run / vppctl / ip / ping / driverctl / interface check, a fake sysfs tree and
# a temp startup.conf. Nothing on the real host is read or changed (no VPP, no /etc, no /sys).
#
#   deploy/vpp/test-apply-startup.sh [path/to/vrx-startupgen]
#
# Without an argument the generator is built from apps/agent into a temp dir.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
SCRIPT="$HERE/apply-startup.sh"
FIX="$REPO/apps/agent/internal/renderers/vppstartup/testdata"
TOP="$(mktemp -d)"
trap 'rm -rf "$TOP"' EXIT

GEN="${1:-}"
if [[ -z $GEN ]]; then
  GEN="$TOP/vrx-startupgen"
  (cd "$REPO/apps/agent" && go build -o "$GEN" ./cmd/vrx-startupgen)
fi

PASS=0 FAIL=0
ok() { if eval "$1"; then PASS=$((PASS + 1)); echo "  ok   $2"; else FAIL=$((FAIL + 1)); echo "  FAIL $2"; fi; }

# ---------------------------------------------------------------- fake host
setup() {
  T="$(mktemp -d "$TOP/case.XXXX")"
  export FAKE="$T"
  mkdir -p "$T/bin" "$T/state" "$T/hooks" "$T/etc" "$T/plugins" "$T/sys/class/net/ens192" "$T/apply"
  for d in vmxnet3 vfio-pci; do mkdir -p "$T/sys/bus/pci/drivers/$d"; : > "$T/sys/bus/pci/drivers/$d/bind"; : > "$T/sys/bus/pci/drivers/$d/unbind"; done
  for p in 0000:0b:00.0 0000:04:00.0 0000:0c:00.0; do
    mkdir -p "$T/sys/bus/pci/devices/$p"; : > "$T/sys/bus/pci/devices/$p/driver_override"
    ln -s "$T/sys/bus/pci/drivers/vmxnet3" "$T/sys/bus/pci/devices/$p/driver"
  done
  ln -s "$T/sys/bus/pci/devices/0000:0b:00.0" "$T/sys/class/net/ens192/device"
  xargs -I{} touch "$T/plugins/{}" < "$FIX/plugins-vrx-a.txt"
  cp "$FIX/host-startup.conf" "$T/etc/startup.conf"
  python3 -c 'import socket,sys; socket.socket(socket.AF_UNIX).bind(sys.argv[1])' "$T/api.sock"
  echo active > "$T/state/vpp"; echo 2 > "$T/state/nrestarts"; echo 0 > "$T/state/ping"; echo 0 > "$T/state/iface"
  printf '  %d. %s   26.06-release   x\n' 1 dpdk_plugin.so 2 linux_cp_plugin.so 3 linux_nl_plugin.so 4 npt66_plugin.so 5 acl_plugin.so > "$T/state/plugins"

  cat > "$T/bin/systemctl" <<'EOF'
#!/usr/bin/env bash
echo "systemctl $*" >> "$FAKE/calls"
case "$1" in
  is-active) [[ $(cat "$FAKE/state/vpp") == active ]] ;;
  show) cat "$FAKE/state/nrestarts" ;;
  restart|start) echo active > "$FAKE/state/vpp"; if [[ -x $FAKE/hooks/$1 ]]; then "$FAKE/hooks/$1"; fi ;;
  stop) if [[ $2 == vpp ]]; then echo inactive > "$FAKE/state/vpp"; fi ;;
esac
EOF
  cat > "$T/bin/systemd-run" <<'EOF'
#!/usr/bin/env bash
echo "systemd-run $*" >> "$FAKE/calls"
EOF
  cat > "$T/bin/vppctl" <<'EOF'
#!/usr/bin/env bash
[[ $(cat "$FAKE/state/vpp") == active ]] || { echo "clib_socket_init: connect: Connection refused"; exit 0; }
case "$*" in
  "show version") echo "vpp v26.06-release built by root on vrx-a" ;;
  "show plugins") echo " Plugin path is: /usr/lib/x86_64-linux-gnu/vpp_plugins"; cat "$FAKE/state/plugins" ;;
esac
EOF
  cat > "$T/bin/ip" <<'EOF'
#!/usr/bin/env bash
echo "ip $*" >> "$FAKE/calls"
case "$*" in
  "-o -4 route show default") echo "default via 10.0.0.1 dev ens192 proto static" ;;
  "-o link show dev ens192") echo "2: ens192: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 state UP" ;;
  "-o -4 addr show dev ens192") [[ -e $FAKE/state/noaddr ]] || echo "2: ens192    inet 10.0.0.5/24 brd 10.0.0.255 scope global ens192" ;;
esac
EOF
  cat > "$T/bin/ping" <<'EOF'
#!/usr/bin/env bash
exit "$(cat "$FAKE/state/ping")"
EOF
  cat > "$T/bin/iface-check" <<'EOF'
#!/usr/bin/env bash
echo "iface-check $*" >> "$FAKE/calls"
exit "$(cat "$FAKE/state/iface")"
EOF
  cat > "$T/bin/driverctl" <<'EOF'
#!/usr/bin/env bash
echo "driverctl $*" >> "$FAKE/calls"
EOF
  cat > "$T/bin/netplan" <<'EOF'
#!/usr/bin/env bash
echo "netplan $*" >> "$FAKE/calls"
EOF
  chmod +x "$T"/bin/*
  : > "$T/calls"
  export VRX_STARTUP_CONF="$T/etc/startup.conf" VRX_SYSFS="$T/sys" VRX_APPLY_STATE="$T/apply" \
    VRX_LAB_LOCK="$T/lab.lock" VRX_VPP_LOCK="$T/vpp.lock" VRX_VPP_API_SOCKET="$T/api.sock" \
    VRX_SYSTEMCTL="$T/bin/systemctl" VRX_SYSTEMD_RUN="$T/bin/systemd-run" VRX_VPPCTL="$T/bin/vppctl" \
    VRX_IP="$T/bin/ip" VRX_PING="$T/bin/ping" VRX_DRIVERCTL="$T/bin/driverctl" VRX_NETPLAN="$T/bin/netplan" \
    VRX_IFACE_CHECK="$T/bin/iface-check" VRX_STARTUPGEN="$GEN"
  # the six-NIC sample document; host facts pinned by flags (vrx-a)
  DOCF="$FIX/cases/six-nic-sample.json"
  HOSTARGS=(-- --no-host --mgmt-pci 0000:0b:00.0 --plugin-dir "$T/plugins" --online-cpus 0-31 --numa-nodes 2 --hugepages-mb 2048)
  ORIG="$(sha256sum "$T/etc/startup.conf" | awk '{print $1}')"
}
apply() { "$SCRIPT" --doc "$DOCF" --apply --foreground --window 2 --interval 1 --api-wait 2 --lock-timeout 2 "$@" "${HOSTARGS[@]}"; }
unchanged() { [[ $(sha256sum "$T/etc/startup.conf" | awk '{print $1}') == "$ORIG" ]]; }
work() { find "$T/apply" -mindepth 1 -maxdepth 1 -type d | head -1; }

echo "== 1. dry run (default) changes nothing and prints the sha256"
setup
out="$("$SCRIPT" --doc "$DOCF" "${HOSTARGS[@]}")"
ok 'grep -q "^+  dev 0000:04:00.0 {" <<<"$out"' "unified diff shows the new dev lines"
ok 'grep -q "^+ dpdk > dev 0000:04:00.0 > name wan" <<<"$out"' "semantic diff shows the logical names"
ok 'grep -q "0000:0b:00.0 vmxnet3" <<<"$out"' "drivers of the PCI devices involved are listed"
ok 'grep -qx "$ORIG" <<<"$out"' "sha256 of the live file printed"
ok 'unchanged && [[ ! -s $T/calls || -z $(grep -E "systemctl (restart|stop|start)" "$T/calls") ]]' "live file untouched, VPP not restarted"

echo "== 2. --apply without --expect-sha256 is refused"
setup
rc=0; apply >/dev/null 2>&1 || rc=$?
ok '[[ $rc == 2 ]] && unchanged' "refused (exit $rc), file unchanged"

echo "== 3. file changed since the review → refused inside the lock, nothing restarted"
setup
rc=0; apply --expect-sha256 0000 > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 3 ]] && unchanged && ! grep -q "systemctl restart" "$T/calls"' "exit $rc, file unchanged, no restart"
ok 'grep -q "changed since the review" "$T/out"' "reason logged"

echo "== 4. healthy apply commits"
setup
rc=0; apply --expect-sha256 "$ORIG" > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 0 && -e $W/committed && ! -e $W/rolled-back ]]' "exit $rc, committed"
ok 'cmp -s "$T/etc/startup.conf" "$W/new.conf" && grep -q "name wan" "$T/etc/startup.conf"' "new file installed"
ok 'cmp -s "$W/backup.conf" "$FIX/host-startup.conf" && ls "$T/etc/" | grep -q "startup.conf.bak-"' "backup kept (work dir + next to the file)"
ok 'grep -q "systemctl restart vpp" "$T/calls"' "VPP restarted"
ok 'grep -q "systemd-run --unit=vrx-startup-apply-deadman-.* --on-active=.* --stage rollback" "$T/calls"' "dead-man timer armed"
ok 'grep -q "systemctl stop vrx-startup-apply-deadman-.*\.timer" "$T/calls"' "dead-man timer cancelled after commit"
ok 'grep -q "iface-check dmz lan lan2 p2p sync wan" "$T/calls"' "logical interfaces verified through the API checker"
ok 'grep -q "^0000:0b:00.0 vmxnet3$" "$W/drivers" && grep -q "^0000:04:00.0 vmxnet3$" "$W/drivers"' "drivers recorded before the restart"
ok 'grep -q "locks held" "$T/out" && [[ $(grep -n "locks held" "$T/out" | cut -d: -f1) -lt $(grep -n "backup:" "$T/out" | cut -d: -f1) ]]' "locks taken before backup and diff"

echo "== 5. a logical interface missing in VPP → rollback"
setup
echo 1 > "$T/state/iface"
rc=0; apply --expect-sha256 "$ORIG" > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 1 && -e $W/rolled-back && ! -e $W/committed ]]' "exit $rc, rolled back"
ok 'unchanged' "original file restored"
ok 'grep -q "ROLLBACK: logical interface(s) missing" "$T/out"' "reason logged"
ok 'grep -q "systemctl stop vpp" "$T/calls" && grep -q "systemctl start vpp" "$T/calls"' "VPP stopped and started on the old file"

echo "== 6. VPP steals the management NIC (vfio-pci) → rollback rebinds it to vmxnet3"
setup
cat > "$T/hooks/restart" <<'EOF'
#!/usr/bin/env bash
ln -sfn "$FAKE/sys/bus/pci/drivers/vfio-pci" "$FAKE/sys/bus/pci/devices/0000:0b:00.0/driver"
EOF
chmod +x "$T/hooks/restart"
rc=0; apply --expect-sha256 "$ORIG" > "$T/out" 2>&1 || rc=$?
W="$(work)"
ok '[[ $rc == 1 ]] && unchanged' "exit $rc, file restored"
ok 'grep -q "management NIC 0000:0b:00.0 changed driver" "$T/out"' "detected by the management check"
ok 'grep -qx "0000:0b:00.0" "$T/sys/bus/pci/drivers/vfio-pci/unbind" && grep -qx "0000:0b:00.0" "$T/sys/bus/pci/drivers/vmxnet3/bind"' "unbound from vfio-pci, bound back to vmxnet3"
ok 'grep -q "driverctl unset-override 0000:0b:00.0" "$T/calls"' "driverctl override cleared"
ok 'grep -q "ip link set dev ens192 up" "$T/calls"' "management interface brought up"

echo "== 7. a plugin the new file enables is not loaded → rollback"
setup
grep -v npt66 "$T/state/plugins" > "$T/state/p2" && mv "$T/state/p2" "$T/state/plugins"
rc=0; apply --expect-sha256 "$ORIG" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && grep -q "plugin npt66_plugin.so enabled but not loaded" "$T/out"' "exit $rc, reason: npt66 not loaded"

echo "== 8. gateway stops answering → rollback; address lost → netplan apply"
setup
echo 1 > "$T/state/ping"; touch "$T/state/noaddr"
rc=0; apply --expect-sha256 "$ORIG" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && grep -q "has no IPv4 address" "$T/out"' "exit $rc, management path failure detected"
ok 'grep -q "netplan apply" "$T/calls"' "netplan apply during rollback"

echo "== 9. VPP crash-restarts during the window → rollback"
setup
cat > "$T/hooks/restart" <<'EOF'
#!/usr/bin/env bash
echo 3 > "$FAKE/state/nrestarts"
EOF
chmod +x "$T/hooks/restart"
rc=0; apply --expect-sha256 "$ORIG" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && grep -q "restarted by itself" "$T/out"' "exit $rc, NRestarts change detected"

echo "== 10. dead-man timer: no-op after commit, rollback when the run died mid-way"
setup
apply --expect-sha256 "$ORIG" >/dev/null 2>&1
W="$(work)"
out="$("$SCRIPT" --stage rollback --work "$W" 2>&1)"
ok 'grep -q "nothing to do" <<<"$out" && ! unchanged' "committed run: timer does nothing"
rm "$W/committed"
rc=0; "$SCRIPT" --stage rollback --work "$W" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 1 ]] && unchanged && grep -q "dead-man timer fired" "$T/out"' "uncommitted run: timer restores the backup (exit $rc)"

echo "== 11. lab lock held by someone else → refused, nothing changed"
setup
flock -x "$T/lab.lock" sleep 4 & holder=$!
sleep 0.3
rc=0; apply --expect-sha256 "$ORIG" --lock-timeout 1 > "$T/out" 2>&1 || rc=$?
kill "$holder" 2>/dev/null || true; wait "$holder" 2>/dev/null || true
ok '[[ $rc == 3 ]] && unchanged && grep -q "busy" "$T/out"' "exit $rc, lock busy, file unchanged"

echo "== 12. without --foreground the run is detached through systemd-run"
setup
rc=0; out="$("$SCRIPT" --doc "$DOCF" --apply --expect-sha256 "$ORIG" "${HOSTARGS[@]}" 2>&1)" || rc=$?
ok '[[ $rc == 0 ]] && grep -q "systemd-run --unit=vrx-startup-apply-[0-9-]* --collect --quiet --property=KillMode=process .*apply-startup.sh --stage run" "$T/calls"' "systemd-run --unit=vrx-startup-apply-… --stage run"
ok 'unchanged && grep -q "journalctl -fu vrx-startup-apply-" <<<"$out"' "caller returns at once; file untouched by the caller"

echo "== 13. the generator refuses the management NIC as a device (host facts) → nothing changed"
setup
echo '{"dataplane":{"managementPci":["0000:04:00.0"],"devices":{"0000:0b:00.0":{"name":"lan"}}}}' > "$T/bad.json"
rc=0; "$SCRIPT" --doc "$T/bad.json" "${HOSTARGS[@]}" > "$T/out" 2>&1 || rc=$?
ok '[[ $rc == 2 ]] && unchanged && grep -q "does not match the host" "$T/out"' "dry run fails with the generator's error (exit $rc)"

echo
echo "apply-startup tests: $PASS passed, $FAIL failed"
((FAIL == 0))
