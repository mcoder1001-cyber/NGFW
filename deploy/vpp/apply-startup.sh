#!/usr/bin/env bash
# deploy/vpp/apply-startup.sh — the manager's procedure to apply a generated VPP startup.conf
# (F-startup-gen, WBS D0.6; replaces the inline procedure reviewed as F2). MANAGER ONLY, and only
# when docs/lab/host-vrx-a.md / an explicit decision allows a VPP restart (D-012, D-060).
#
#   apply-startup.sh --doc <document.json> [-- <vrx-startupgen flags>]
#       DRY RUN (default): render, show the unified and semantic diff against the live file, the
#       drivers of every PCI device involved and the sha256 of the live file. Changes nothing.
#   apply-startup.sh --doc <document.json> --apply --expect-sha256 <sum from the dry run> [--window 60]
#       APPLY: re-runs itself DETACHED from the SSH session (systemd-run --unit=vrx-startup-apply-…,
#       fallback setsid nohup) and returns. The detached run:
#         1. takes flock -x on /run/lock/vrx-vpp.lock then /run/lock/vrx-lab.lock (same order as
#            tools/lab) — BEFORE anything else, so backup, diff and install see one consistent file
#         2. refuses unless sha256(live file) == --expect-sha256 (what the manager reviewed)
#         3. renders again, backs up the live file, logs both diffs, records the kernel driver of
#            every PCI device in the old and new file, the loaded plugins and NRestarts
#         4. arms a dead-man timer (systemd-run --on-active=window+120) that rolls back if this run
#            dies before it commits
#         5. installs the file, restarts VPP, then watches for --window seconds: vpp active and not
#            crash-restarting, API answers, plugins per `show plugins` content (enabled loaded,
#            disabled absent, previously loaded still loaded), every logical interface present via
#            the VPP API (vpp-iface-check.py: sw_interface_dump by name), management interface up
#            with its address, its driver unchanged and its gateway answering
#         6. any failure → rollback: stop VPP, restore the backup, rebind every NIC whose driver
#            changed to its recorded driver, bring the management interface up (netplan apply if
#            it lost its address), start VPP, verify; success → mark committed, cancel the timer
#   apply-startup.sh --stage run|rollback --work <dir>     internal (the detached parts)
#
# Logs and state: /var/lib/vrx/startup-apply/<stamp>/ (log, doc.json, new.conf, backup.conf,
# drivers, plugins.before, committed | rolled-back). Exit: 0 ok / nothing to do, 1 failed and
# rolled back, 2 usage, 3 refused before any change.
#
# Every system path and command is overridable through VRX_* variables (see below) so the script
# is tested against a fake systemctl/vppctl/sysfs tree (deploy/vpp/test-apply-startup.sh); it is
# never run on the shared host by a worker.
set -euo pipefail

SELF="$(readlink -f "$0")"
HERE="$(dirname "$SELF")"
CONF="${VRX_STARTUP_CONF:-/etc/vpp/startup.conf}"
SYSFS="${VRX_SYSFS:-/sys}"
STATE_ROOT="${VRX_APPLY_STATE:-/var/lib/vrx/startup-apply}"
LAB_LOCK="${VRX_LAB_LOCK:-/run/lock/vrx-lab.lock}"
VPP_LOCK="${VRX_VPP_LOCK:-/run/lock/vrx-vpp.lock}"
API_SOCK="${VRX_VPP_API_SOCKET:-/run/vpp/api.sock}"
SYSTEMCTL="${VRX_SYSTEMCTL:-systemctl}"
SYSTEMD_RUN="${VRX_SYSTEMD_RUN:-systemd-run}"
VPPCTL="${VRX_VPPCTL:-vppctl}"
IPCMD="${VRX_IP:-ip}"
PING="${VRX_PING:-ping}"
DRIVERCTL="${VRX_DRIVERCTL:-driverctl}"
NETPLAN="${VRX_NETPLAN:-netplan}"
IFACE_CHECK="${VRX_IFACE_CHECK:-$HERE/vpp-iface-check.py}"
STARTUPGEN="${VRX_STARTUPGEN:-/root/ngfw/apps/agent/bin/vrx-startupgen}"
UNIT_PREFIX="vrx-startup-apply"

DOC="" APPLY=0 EXPECT="" WINDOW=60 INTERVAL=5 LOCK_TIMEOUT=1800 API_WAIT=30 FOREGROUND=0 STAGE="" WORK="" DELAY=0
GEN_ARGS=()

die() { echo "apply-startup: $*" >&2; exit 2; }
say() { echo "apply-startup: $(date '+%F %T') $*"; }

while (($#)); do
  case "$1" in
    --doc) DOC="${2:?--doc needs a file}"; shift ;;
    --apply) APPLY=1 ;;
    --expect-sha256) EXPECT="${2:?}"; shift ;;
    --window) WINDOW="${2:?}"; shift ;;
    --interval) INTERVAL="${2:?}"; shift ;;
    --lock-timeout) LOCK_TIMEOUT="${2:?}"; shift ;;
    --api-wait) API_WAIT="${2:?}"; shift ;;
    --foreground) FOREGROUND=1 ;;               # run the apply stage in this process (tests, debugging)
    --stage) STAGE="${2:?}"; shift ;;
    --work) WORK="${2:?}"; shift ;;
    --delay) DELAY="${2:?}"; shift ;;           # rollback stage: sleep first (setsid fallback of the timer)
    --) shift; GEN_ARGS=("$@"); break ;;
    -h|--help) sed -n '2,/^set -euo pipefail/{/^set -euo pipefail/!p}' "$SELF" | sed -E 's/^# ?//'; exit 0 ;;
    *) die "unknown argument '$1' (try --help)" ;;
  esac
  shift
done
for n in WINDOW INTERVAL LOCK_TIMEOUT API_WAIT DELAY; do [[ ${!n} =~ ^[0-9]+$ ]] || die "--${n,,} must be a number"; done

sha() { sha256sum "$1" | awk '{print $1}'; }

# ---------------------------------------------------------------- facts
pci_driver() {  # <pci> → driver name or "none"
  local link="$SYSFS/bus/pci/devices/$1/driver"
  if [[ -L $link ]]; then basename "$(readlink "$link")"; else echo none; fi
}
conf_pcis() {  # <file> → PCI addresses named in dev/blacklist lines
  grep -oE '^[[:space:]]*(dev|blacklist)[[:space:]]+[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-7]' "$1" | awk '{print tolower($2)}' | sort -u
}
conf_names() { sed -nE 's/^[[:space:]]*name[[:space:]]+([a-z][a-z0-9_-]*)[[:space:]]*$/\1/p' "$1" | sort -u; }
conf_plugins() {  # <file> <enable|disable>
  sed -nE "s/^[[:space:]]*plugin[[:space:]]+([A-Za-z0-9_-]+_plugin\\.so)[[:space:]]*\\{[[:space:]]*$2[[:space:]]*\\}.*/\\1/p" "$1" | sort -u
}
loaded_plugins() { "$VPPCTL" show plugins 2>/dev/null | grep -oE '[A-Za-z0-9_-]+_plugin\.so' | sort -u; }
nrestarts() { "$SYSTEMCTL" show vpp -p NRestarts --value 2>/dev/null || echo "?"; }
mgmt_route() {  # → "<dev> <gateway|->" of the IPv4 default route
  "$IPCMD" -o -4 route show default 2>/dev/null | awk '{d="";g="-"; for(i=1;i<NF;i++){if($i=="dev")d=$(i+1); if($i=="via")g=$(i+1)} if(d!=""){print d, g; exit}}'
}

render() {  # <out> — the generator validates against this host and keeps the live file's plugin switches
  "$STARTUPGEN" --current "$CONF" -o "$1" "${GEN_ARGS[@]}" "$DOC"
}

# ---------------------------------------------------------------- dry run
dry_run() {
  [[ -f $CONF ]] || die "$CONF not found"
  local tmp; tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' RETURN
  render "$tmp/new.conf" || die "rendering failed (nothing changed)"
  echo "== unified diff: $CONF → rendered"
  "$STARTUPGEN" --current "$CONF" --diff "$CONF" "${GEN_ARGS[@]}" "$DOC" || true
  echo "== semantic diff (comments, order, indentation ignored)"
  "$STARTUPGEN" --current "$CONF" --diff "$CONF" --semantic "${GEN_ARGS[@]}" "$DOC" || true
  echo "== PCI devices involved (driver now)"
  { conf_pcis "$CONF"; conf_pcis "$tmp/new.conf"; } | sort -u | while read -r p; do echo "  $p $(pci_driver "$p")"; done
  echo "== sha256 of $CONF (pass it to --apply --expect-sha256)"
  sha "$CONF"
  echo "dry run: nothing changed"
}

# ---------------------------------------------------------------- locking
take_locks() {
  exec 8>"$VPP_LOCK" 9>"$LAB_LOCK"
  flock -x -w "$LOCK_TIMEOUT" 8 || { say "REFUSED: $VPP_LOCK busy for ${LOCK_TIMEOUT}s"; return 3; }
  flock -x -w "$LOCK_TIMEOUT" 9 || { say "REFUSED: $LAB_LOCK busy for ${LOCK_TIMEOUT}s"; return 3; }
}

# ---------------------------------------------------------------- health
wait_api() {
  local i
  for ((i = 0; i <= API_WAIT; i++)); do
    [[ -S $API_SOCK ]] && "$VPPCTL" show version 2>/dev/null | grep -q '^vpp v' && return 0
    sleep 1
  done
  return 1
}
check_mgmt() {  # management interface up, addressed, same driver, gateway answering
  local dev gw pci
  read -r dev gw < "$WORK/mgmt" || return 1
  "$IPCMD" -o link show dev "$dev" 2>/dev/null | grep -qE '[<,]UP[,>]' || { echo "management interface $dev is not up"; return 1; }
  [[ -n $("$IPCMD" -o -4 addr show dev "$dev" 2>/dev/null) ]] || { echo "management interface $dev has no IPv4 address"; return 1; }
  if [[ -L $SYSFS/class/net/$dev/device ]]; then
    pci="$(basename "$(readlink "$SYSFS/class/net/$dev/device")")"
    [[ $(pci_driver "$pci") == "$(awk -v p="$pci" '$1==p{print $2}' "$WORK/drivers")" ]] || { echo "management NIC $pci changed driver"; return 1; }
  fi
  if [[ $gw != - ]]; then "$PING" -c 1 -W 2 "$gw" >/dev/null 2>&1 || { echo "gateway $gw does not answer"; return 1; }; fi
}
check_health() {  # → prints the first problem, returns 1
  "$SYSTEMCTL" is-active --quiet vpp || { echo "vpp.service is not active"; return 1; }
  [[ $(nrestarts) == "$(cat "$WORK/nrestarts")" ]] || { echo "vpp.service restarted by itself (NRestarts $(cat "$WORK/nrestarts") → $(nrestarts))"; return 1; }
  [[ -S $API_SOCK ]] || { echo "no API socket $API_SOCK"; return 1; }
  "$VPPCTL" show version 2>/dev/null | grep -q '^vpp v' || { echo "VPP CLI does not answer"; return 1; }
  local loaded p names
  loaded="$(loaded_plugins)"
  for p in $(conf_plugins "$WORK/new.conf" enable); do grep -qx "$p" <<<"$loaded" || { echo "plugin $p enabled but not loaded"; return 1; }; done
  for p in $(conf_plugins "$WORK/new.conf" disable); do ! grep -qx "$p" <<<"$loaded" || { echo "plugin $p disabled but loaded"; return 1; }; done
  while read -r p; do
    [[ -n $p ]] || continue
    grep -qx "$p" <(conf_plugins "$WORK/new.conf" disable) && continue
    grep -qx "$p" <<<"$loaded" || { echo "plugin $p was loaded before and is gone"; return 1; }
  done < "$WORK/plugins.before"
  mapfile -t names < <(conf_names "$WORK/new.conf")
  if ((${#names[@]})); then
    "$IFACE_CHECK" "${names[@]}" || { echo "logical interface(s) missing in VPP (API check)"; return 1; }
  fi
  check_mgmt
}

# ---------------------------------------------------------------- rollback
rebind_drivers() {
  local pci want cur
  while read -r pci want; do
    cur="$(pci_driver "$pci")"
    [[ $cur == "$want" ]] && continue
    say "rebind $pci: $cur → $want"
    command -v "$DRIVERCTL" >/dev/null 2>&1 && "$DRIVERCTL" unset-override "$pci" >/dev/null 2>&1 || true
    [[ $cur == none ]] || echo "$pci" > "$SYSFS/bus/pci/devices/$pci/driver/unbind" || true
    [[ -e $SYSFS/bus/pci/devices/$pci/driver_override ]] && { echo > "$SYSFS/bus/pci/devices/$pci/driver_override" || true; }
    [[ $want == none ]] || echo "$pci" > "$SYSFS/bus/pci/drivers/$want/bind" || say "WARNING: bind $pci to $want failed"
  done < "$WORK/drivers"
}
rollback() {  # restore file + drivers + management path, restart VPP, verify
  say "ROLLBACK: $1"
  "$SYSTEMCTL" stop vpp || true
  install -m 0644 "$WORK/backup.conf" "$CONF"
  rebind_drivers
  local dev gw
  if read -r dev gw < "$WORK/mgmt"; then
    "$IPCMD" link set dev "$dev" up || true
    if [[ -z $("$IPCMD" -o -4 addr show dev "$dev" 2>/dev/null) ]] && command -v "$NETPLAN" >/dev/null 2>&1; then
      say "management interface $dev has no address: netplan apply"; "$NETPLAN" apply || true
    fi
  fi
  "$SYSTEMCTL" start vpp || true
  if wait_api && "$SYSTEMCTL" is-active --quiet vpp && check_mgmt; then
    say "rolled back to $WORK/backup.conf; VPP and the management path are healthy"
  else
    say "ROLLBACK INCOMPLETE — VPP or the management path is still unhealthy; console access needed"
  fi
  touch "$WORK/rolled-back"
  "$SYSTEMCTL" stop "$(cat "$WORK/deadman-unit" 2>/dev/null || echo none).timer" >/dev/null 2>&1 || true
}

# ---------------------------------------------------------------- stages
detach() {  # re-run "--stage run" outside the SSH session
  local stamp; stamp="$(date +%Y%m%d-%H%M%S)-$$"
  WORK="$STATE_ROOT/$stamp"
  install -d -m 0750 "$WORK"
  install -m 0640 "$DOC" "$WORK/doc.json"
  printf '%s\n' "${GEN_ARGS[@]}" > "$WORK/gen-args"
  local cmd=("$SELF" --stage run --work "$WORK" --expect-sha256 "$EXPECT" --window "$WINDOW" --interval "$INTERVAL"
    --lock-timeout "$LOCK_TIMEOUT" --api-wait "$API_WAIT")
  if ((FOREGROUND)); then "${cmd[@]}"; return; fi
  if "$SYSTEMD_RUN" --unit="$UNIT_PREFIX-$stamp" --collect --quiet --property=KillMode=process "${cmd[@]}"; then
    say "started detached unit $UNIT_PREFIX-$stamp — follow: journalctl -fu $UNIT_PREFIX-$stamp  (log: $WORK/log)"
  else
    setsid nohup "${cmd[@]}" </dev/null >/dev/null 2>&1 &
    say "systemd-run unavailable: started with setsid nohup (pid $!) — follow: tail -f $WORK/log"
  fi
}

stage_run() {
  [[ -d $WORK ]] || die "--work $WORK missing"
  exec > >(tee -a "$WORK/log") 2>&1
  DOC="$WORK/doc.json"
  mapfile -t GEN_ARGS < "$WORK/gen-args"
  [[ -n ${GEN_ARGS[0]:-} ]] || GEN_ARGS=()
  take_locks || return 3
  say "locks held: $VPP_LOCK, $LAB_LOCK"
  local now; now="$(sha "$CONF")"
  [[ $now == "$EXPECT" ]] || { say "REFUSED: $CONF changed since the review (sha256 $now, expected $EXPECT) — dry-run again"; return 3; }
  render "$WORK/new.conf" || { say "REFUSED: rendering failed"; return 3; }
  cp -p "$CONF" "$WORK/backup.conf"
  cp -p "$CONF" "$CONF.bak-$(basename "$WORK")"
  say "backup: $WORK/backup.conf and $CONF.bak-$(basename "$WORK")"
  say "unified diff:"; "$STARTUPGEN" --current "$CONF" --diff "$CONF" "${GEN_ARGS[@]}" "$DOC" || true
  say "semantic diff:"; "$STARTUPGEN" --current "$CONF" --diff "$CONF" --semantic "${GEN_ARGS[@]}" "$DOC" || true
  if cmp -s "$CONF" "$WORK/new.conf"; then say "nothing to do: the rendering equals $CONF"; touch "$WORK/committed"; return 0; fi

  { conf_pcis "$CONF"; conf_pcis "$WORK/new.conf"; } | sort -u | while read -r p; do echo "$p $(pci_driver "$p")"; done > "$WORK/drivers"
  mgmt_route > "$WORK/mgmt" || true
  [[ -s $WORK/mgmt ]] || { say "REFUSED: no IPv4 default route — cannot watch the management path"; return 3; }
  read -r dev _ < "$WORK/mgmt"
  if [[ -L $SYSFS/class/net/$dev/device ]]; then  # the management NIC's driver is watched even if no file names it
    p="$(basename "$(readlink "$SYSFS/class/net/$dev/device")")"
    grep -q "^$p " "$WORK/drivers" || echo "$p $(pci_driver "$p")" >> "$WORK/drivers"
  fi
  loaded_plugins > "$WORK/plugins.before"
  nrestarts > "$WORK/nrestarts"
  say "recorded drivers: $(tr '\n' ';' < "$WORK/drivers") mgmt: $(cat "$WORK/mgmt") NRestarts: $(cat "$WORK/nrestarts")"

  local deadman
  deadman="$UNIT_PREFIX-deadman-$(basename "$WORK")"
  echo "$deadman" > "$WORK/deadman-unit"
  if ! "$SYSTEMD_RUN" --unit="$deadman" --on-active=$((WINDOW + API_WAIT + 120)) --collect --quiet "$SELF" --stage rollback --work "$WORK"; then
    setsid nohup "$SELF" --stage rollback --work "$WORK" --delay $((WINDOW + API_WAIT + 120)) </dev/null >/dev/null 2>&1 &
  fi
  say "dead-man timer armed: rollback in $((WINDOW + API_WAIT + 120))s unless committed"

  install -m 0644 "$WORK/new.conf" "$CONF"
  say "installed $CONF; restarting VPP"
  if ! "$SYSTEMCTL" restart vpp; then rollback "systemctl restart vpp failed"; return 1; fi
  if ! wait_api; then rollback "VPP API did not come up within ${API_WAIT}s"; return 1; fi
  local end=$((SECONDS + WINDOW)) why
  while :; do
    if ! why="$(check_health)"; then rollback "$why"; return 1; fi
    ((SECONDS < end)) || break
    sleep "$INTERVAL"
  done
  touch "$WORK/committed"
  "$SYSTEMCTL" stop "$deadman.timer" >/dev/null 2>&1 || true
  say "COMMITTED: healthy for ${WINDOW}s; dead-man timer cancelled. Record it in docs/decisions/LOG.md (backup $WORK/backup.conf)"
}

stage_rollback() {  # the dead-man timer
  [[ -d $WORK ]] || die "--work $WORK missing"
  exec > >(tee -a "$WORK/log") 2>&1
  sleep "$DELAY"
  if [[ -e $WORK/committed || -e $WORK/rolled-back ]]; then say "dead-man: nothing to do (the apply already committed or rolled back)"; return 0; fi
  take_locks || return 3
  [[ -e $WORK/committed || -e $WORK/rolled-back ]] && return 0
  [[ -f $WORK/backup.conf ]] || { say "dead-man: no backup yet — nothing was installed"; return 0; }
  rollback "dead-man timer fired before the apply committed"
  return 1
}

case "$STAGE" in
  run) stage_run ;;
  rollback) stage_rollback ;;
  "")
    [[ -n $DOC && -f $DOC ]] || die "--doc <document.json> is required"
    [[ -x $STARTUPGEN ]] || die "$STARTUPGEN not found (cd apps/agent && go build -o bin/vrx-startupgen ./cmd/vrx-startupgen)"
    if ((APPLY)); then
      [[ -n $EXPECT ]] || die "--apply needs --expect-sha256 <sum printed by the dry run>"
      [[ -f $CONF ]] || die "$CONF not found"
      detach
    else
      dry_run
    fi ;;
  *) die "unknown stage '$STAGE'" ;;
esac
