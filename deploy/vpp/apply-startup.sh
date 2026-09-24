#!/usr/bin/env bash
# deploy/vpp/apply-startup.sh — the manager's tool to apply a generated VPP startup.conf
# (F-startup-apply; generator: F-startup-gen, WBS D0.6). MANAGER ONLY. Refuses --apply unless
# docs/lab/host-<vm>.md says `handover: done` (D-012) or the product owner approved this change
# (--i-have-product-owner-approval <PENDING-id|D-nnn>, recorded in the log, the gate file and syslog).
#
#   apply-startup.sh --doc <document.json> [options] [-- <vrx-startupgen flags>]
#       DRY RUN (default): render, show the unified and semantic diff against the live file, the
#       drivers of every PCI device involved, the protected management interfaces with their
#       network manager and exact restore plan, a read-only VPP preflight, the handover gate, the
#       sha256 of the live file and of the rendering. Changes nothing. Exit 3 when --apply would refuse.
#   apply-startup.sh --doc <document.json> --apply --expect-sha256 <live sum> --expect-new-sha256 <rendered sum>
#       APPLY: copies the document, the generator, vrx-vppcheck and this script into a work dir
#       (pinned: a later rebuild in a shared tree does not matter), records every setting there and
#       re-runs itself DETACHED from the SSH session (systemd-run --unit=vrx-startup-apply-<stamp>,
#       every VRX_* setting passed as --setenv; fallback setsid). The detached run:
#         1. takes flock -x on /run/lock/vrx-vpp.lock then /run/lock/vrx-lab.lock (tools/lab order),
#            bounded by --lock-timeout — BEFORE anything else
#         2. refuses unless sha256(live file) == --expect-sha256, sha256(rendering) ==
#            --expect-new-sha256 (exactly what was reviewed) and VPP answers the read-only preflight
#            (vrx-vppcheck ifaces local0)
#         3. records per management interface `ip -j addr` / `ip -j route` and the exact restore
#            commands, the kernel driver of every PCI device in the old and new file and of every
#            management NIC, the loaded plugins and NRestarts; backs up the live file; logs both diffs
#         4. arms a dead-man timer (systemd-run --on-active; fallback setsid, lock fds closed)
#         5. installs the file, restarts VPP, then watches for --window seconds: vpp active and not
#            crash-restarting, API answers, plugins per `show plugins` content, every logical
#            interface present (sw_interface_dump by name), every management interface up with all
#            its recorded addresses, its NIC's driver unchanged and its default gateway(s) answering
#         6. any failure → rollback: stop VPP (SIGKILL if the stop hangs), restore the backup, rebind
#            every NIC whose driver changed, re-apply the recorded addresses and routes of each
#            management interface verbatim and — only if it is still broken — the network manager's
#            own re-apply (ifupdown `ifup --force`, systemd-networkd `networkctl reconfigure`,
#            netplan `netplan apply`), reset-failed + start VPP, verify. Healthy → rolled-back and the
#            timer is cancelled; not healthy → rollback-incomplete and the dead-man stays armed (retry).
#       Every call that talks to VPP, the kernel or systemd has a timeout (--cmd-timeout,
#       --svc-timeout) and the run has a time budget. When the dead-man fires it first KILLS the run
#       (its process group, or its unit), takes the locks with a bounded wait (--deadman-lock-timeout)
#       and rolls back even if the locks stay busy — it never gives up without rolling back.
#   apply-startup.sh --stage run|rollback --work <dir>     internal (the detached parts)
#
# Options: --vm NAME (vrx-a) · --window S (60) · --interval S (5) · --api-wait S (60) ·
#   --cmd-timeout S (10) · --svc-timeout S (120) · --lock-timeout S (600) · --deadman-lock-timeout S (60) ·
#   --mgmt-if IF (extra interface to protect, repeatable) · --mgmt-peer IP (address the manager connects
#   from; $SSH_CONNECTION is added automatically) · --foreground (run the detached stage in this
#   process: console/tests only) · --i-have-product-owner-approval <PENDING-slug|D-nnn>
#
# Logs and state: /var/lib/vrx/startup-apply/<stamp>/ (log, settings, gate, doc.json, bin/, new.conf,
# backup.conf, drivers, mgmt.ifs, mgmt/<if>.{addr,route4,route6}.json|.restore|.addrs|.gateways|.netmgr,
# plugins.before, installed, committed | rolled-back | rollback-incomplete). Syslog tag vrx-startup-apply.
# Exit: 0 ok / nothing to do · 1 failed and rolled back · 2 usage · 3 refused before any change.
#
# Every system path and command is overridable through VRX_* variables (below) so the script is
# tested against a fake host (deploy/vpp/test-apply-startup.sh). Workers never run --apply for real.
set -euo pipefail

SELF="$(readlink -f "${BASH_SOURCE[0]}")"
HERE="$(dirname "$SELF")"

# ---------------------------------------------------------------- settings (recorded + passed on explicitly, N6)
: "${VRX_STARTUP_CONF:=/etc/vpp/startup.conf}"
: "${VRX_SYSFS:=/sys}"
: "${VRX_ETC:=/etc}"                                   # network configuration lookup only (read)
: "${VRX_APPLY_STATE:=/var/lib/vrx/startup-apply}"
: "${VRX_LAB_LOCK:=/run/lock/vrx-lab.lock}"
: "${VRX_VPP_LOCK:=/run/lock/vrx-vpp.lock}"
: "${VRX_VPP_API_SOCKET:=/run/vpp/api.sock}"
: "${VRX_SYSTEMCTL:=systemctl}"
: "${VRX_SYSTEMD_RUN:=systemd-run}"
: "${VRX_IP:=ip}"
: "${VRX_PING:=ping}"
: "${VRX_DRIVERCTL:=driverctl}"
: "${VRX_NETPLAN:=netplan}"
: "${VRX_IFUP:=ifup}"
: "${VRX_NETWORKCTL:=networkctl}"
: "${VRX_LOGGER:=logger}"
: "${VRX_VPPCHECK:=/root/ngfw/apps/agent/bin/vrx-vppcheck}"
: "${VRX_STARTUPGEN:=/root/ngfw/apps/agent/bin/vrx-startupgen}"
: "${VRX_HANDOVER_EXTRA:=}"                             # extra handover docs: can only make the gate stricter
: "${VRX_MGMT_PEERS:=}"
: "${VRX_MGMT_IFS:=}"
SETTINGS_VARS=(VRX_STARTUP_CONF VRX_SYSFS VRX_ETC VRX_APPLY_STATE VRX_LAB_LOCK VRX_VPP_LOCK VRX_VPP_API_SOCKET
  VRX_SYSTEMCTL VRX_SYSTEMD_RUN VRX_IP VRX_PING VRX_DRIVERCTL VRX_NETPLAN VRX_IFUP VRX_NETWORKCTL VRX_LOGGER
  VRX_VPPCHECK VRX_STARTUPGEN VRX_MGMT_PEERS VRX_MGMT_IFS
  WINDOW INTERVAL API_WAIT CMD_TIMEOUT SVC_TIMEOUT LOCK_TIMEOUT DEADMAN_LOCK_TIMEOUT EXPECT EXPECT_NEW VM)
CANON_ROOT=/root/ngfw   # the manager's main checkout: its handover flag always counts; no override
UNIT_PREFIX="vrx-startup-apply"

DOC="" APPLY=0 EXPECT="" EXPECT_NEW="" WINDOW=60 INTERVAL=5 LOCK_TIMEOUT=600 DEADMAN_LOCK_TIMEOUT=60 API_WAIT=60
CMD_TIMEOUT=10 SVC_TIMEOUT=120 FOREGROUND=0 STAGE="" WORK="" DELAY=0 VM="vrx-a" APPROVAL="" GATE="" DEADMAN_FALLBACK=""
GEN_ARGS=()
STARTUPGEN_BIN="$VRX_STARTUPGEN" VPPCHECK_BIN="$VRX_VPPCHECK"

die() { echo "apply-startup: $*" >&2; exit 2; }
say() { echo "apply-startup: $(date '+%F %T') $*"; }

parse_args() {
  while (($#)); do
    case "$1" in
      --doc) DOC="${2:?--doc needs a file}"; shift ;;
      --apply) APPLY=1 ;;
      --expect-sha256) EXPECT="${2:?}"; shift ;;
      --expect-new-sha256) EXPECT_NEW="${2:?}"; shift ;;
      --window) WINDOW="${2:?}"; shift ;;
      --interval) INTERVAL="${2:?}"; shift ;;
      --lock-timeout) LOCK_TIMEOUT="${2:?}"; shift ;;
      --deadman-lock-timeout) DEADMAN_LOCK_TIMEOUT="${2:?}"; shift ;;
      --api-wait) API_WAIT="${2:?}"; shift ;;
      --cmd-timeout) CMD_TIMEOUT="${2:?}"; shift ;;
      --svc-timeout) SVC_TIMEOUT="${2:?}"; shift ;;
      --vm) VM="${2:?}"; shift ;;
      --mgmt-if) VRX_MGMT_IFS="${VRX_MGMT_IFS:+$VRX_MGMT_IFS }${2:?}"; shift ;;
      --mgmt-peer) VRX_MGMT_PEERS="${VRX_MGMT_PEERS:+$VRX_MGMT_PEERS }${2:?}"; shift ;;
      --i-have-product-owner-approval) APPROVAL="${2:?}"; shift ;;
      --foreground) FOREGROUND=1 ;;
      --stage) STAGE="${2:?}"; shift ;;
      --work) WORK="${2:?}"; shift ;;
      --delay) DELAY="${2:?}"; shift ;;   # rollback stage: sleep first (setsid fallback of the timer)
      --) shift; GEN_ARGS=("$@"); break ;;
      -h|--help) sed -n '2,/^set -euo pipefail/{/^set -euo pipefail/!p}' "$SELF" | sed -E 's/^# ?//'; exit 0 ;;
      *) die "unknown argument '$1' (try --help)" ;;
    esac
    shift
  done
  local n w
  for n in WINDOW INTERVAL LOCK_TIMEOUT DEADMAN_LOCK_TIMEOUT API_WAIT CMD_TIMEOUT SVC_TIMEOUT DELAY; do
    [[ ${!n} =~ ^[0-9]+$ ]] || die "--${n,,} must be a number"
  done
  ((CMD_TIMEOUT > 0 && SVC_TIMEOUT > 0)) || die "timeouts must be > 0"
  [[ $VM =~ ^[a-z0-9][a-z0-9-]*$ ]] || die "--vm must be a lab VM name"
  [[ -z $APPROVAL || $APPROVAL =~ ^(PENDING-[A-Za-z0-9._-]+|D-[0-9]{3,4})$ ]] ||
    die "--i-have-product-owner-approval needs a PENDING-<slug> or D-nnn id, got '$APPROVAL'"
  for w in $VRX_MGMT_IFS; do [[ $w =~ ^[A-Za-z0-9_.@-]{1,15}$ ]] || die "--mgmt-if '$w' is not an interface name"; done
  for w in $VRX_MGMT_PEERS; do [[ $w =~ ^[0-9A-Fa-f:.]+$ ]] || die "--mgmt-peer '$w' is not an IP address"; done
}

sha() { sha256sum "$1" | awk '{print $1}'; }

# Every external call that can block goes through one of these (re-review N1). timeout(1) kills the
# command's whole process group; fds 8/9 (the flocks) are closed so a stuck child never holds a lock.
tmo() { timeout -k 2 "$CMD_TIMEOUT" "$@" 8>&- 9>&-; }
svc() { timeout -k 5 "$SVC_TIMEOUT" "$VRX_SYSTEMCTL" "$@" 8>&- 9>&-; }
vppcheck() { timeout -k 2 $((CMD_TIMEOUT + 2)) "$VPPCHECK_BIN" --socket "$VRX_VPP_API_SOCKET" --timeout "${CMD_TIMEOUT}s" "$@" 8>&- 9>&-; }
sysfs_write() { printf '%s\n' "$1" | timeout -k 2 "$CMD_TIMEOUT" tee "$2" >/dev/null 8>&- 9>&-; }  # <value> <file>
syslog() { tmo "$VRX_LOGGER" -t "$UNIT_PREFIX" -- "$*" >/dev/null 2>&1 || true; }

# ---------------------------------------------------------------- facts
pci_driver() {  # <pci> → driver name or "none"
  local link="$VRX_SYSFS/bus/pci/devices/$1/driver"
  if [[ -L $link ]]; then basename "$(readlink "$link")"; else echo none; fi
}
if_pci() {  # <if> → its PCI address, or nothing
  local l="$VRX_SYSFS/class/net/$1/device"
  if [[ -L $l ]]; then basename "$(readlink "$l")"; fi
}
conf_pcis() {  # <file> → PCI addresses named in dev/blacklist lines
  { grep -oE '^[[:space:]]*(dev|blacklist)[[:space:]]+[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-7]' "$1" || true; } | awk '{print tolower($2)}' | sort -u
}
conf_names() { sed -nE 's/^[[:space:]]*name[[:space:]]+([a-z][a-z0-9_-]*)[[:space:]]*$/\1/p' "$1" | sort -u; }
conf_plugins() {  # <file> <enable|disable>
  sed -nE "s/^[[:space:]]*plugin[[:space:]]+([A-Za-z0-9_-]+_plugin\\.so)[[:space:]]*\\{[[:space:]]*$2[[:space:]]*\\}.*/\\1/p" "$1" | sort -u
}
nrestarts() { tmo "$VRX_SYSTEMCTL" show vpp -p NRestarts --value 2>/dev/null || echo "?"; }

mgmt_ifs() {  # → every interface to protect: default routes (v4+v6), the manager's path(s), --mgmt-if
  local peer d
  {
    tmo "$VRX_IP" -j -4 route show default 2>/dev/null | jq -r '.[]?.dev // empty' || true
    tmo "$VRX_IP" -j -6 route show default 2>/dev/null | jq -r '.[]?.dev // empty' || true
    for peer in $VRX_MGMT_PEERS; do tmo "$VRX_IP" -j route get "$peer" 2>/dev/null | jq -r '.[0]?.dev // empty' || true; done
    for d in $VRX_MGMT_IFS; do echo "$d"; done
  } | while read -r d; do
    [[ $d =~ ^[A-Za-z0-9_.@-]{1,15}$ ]] || continue
    # a tun/tap without a device (a linux-cp tap) belongs to VPP and goes away with it: not a management path
    if [[ -z $(if_pci "$d") && -e $VRX_SYSFS/class/net/$d/tun_flags ]]; then echo "note: $d is a tap without a device (VPP-owned), not watched" >&2; continue; fi
    echo "$d"
  done | sort -u
}

netmgr_of() {  # <if> → ifupdown | netplan | networkd | none (who re-applies the interface's configuration)
  local dev="$1" f
  for f in "$VRX_ETC/network/interfaces" "$VRX_ETC"/network/interfaces.d/*; do
    [[ -f $f ]] || continue
    if grep -qE "^[[:space:]]*(auto|allow-hotplug|iface)([[:space:]]+[^[:space:]]+)*[[:space:]]+$dev([[:space:]]|$)" "$f"; then echo ifupdown; return; fi
  done
  if compgen -G "$VRX_ETC/netplan/*.yaml" >/dev/null && command -v "$VRX_NETPLAN" >/dev/null 2>&1; then echo netplan; return; fi
  if tmo "$VRX_SYSTEMCTL" is-active --quiet systemd-networkd 2>/dev/null; then echo networkd; return; fi
  echo none
}

# The restore plan: `ip` argument lines that re-create exactly the recorded addresses and the
# non-kernel routes of table main on the interface (kernel routes come back with the addresses).
# shellcheck disable=SC2016  # jq program: $dev is a jq variable
RESTORE_JQ='
  ((.addr[0].addr_info // [])[] | select(.scope != "link")
    | ["addr","replace",("\(.local)/\(.prefixlen)")] + (if .broadcast then ["broadcast",.broadcast] else [] end) + ["dev",$dev]
    | join(" ")),
  (((.r4 | map(. + {fam:"-4"})) + (.r6 | map(. + {fam:"-6"})))[]
    | select((.protocol // "boot") != "kernel") | select(.nexthops == null) | select((.dst // "") | startswith("fe80") | not)
    | [.fam,"route","replace",.dst] + (if .gateway then ["via",.gateway] else [] end) + ["dev",$dev]
      + (if .protocol then ["proto",.protocol] else [] end) + (if .metric then ["metric","\(.metric)"] else [] end)
      + (if .scope then ["scope",.scope] else [] end) + (if .prefsrc then ["src",.prefsrc] else [] end)
      + (if ((.flags // []) | index("onlink")) then ["onlink"] else [] end)
    | join(" "))'
snapshot_mgmt() {  # <if> → $WORK/mgmt/<if>.* ; fails when the snapshot cannot be taken
  local dev="$1" m="$WORK/mgmt/$1"
  mkdir -p "$WORK/mgmt"
  tmo "$VRX_IP" -j addr show dev "$dev" > "$m.addr.json" || return 1
  tmo "$VRX_IP" -j -4 route show table main dev "$dev" > "$m.route4.json" || return 1
  tmo "$VRX_IP" -j -6 route show table main dev "$dev" > "$m.route6.json" || return 1
  jq -n -r --arg dev "$dev" --slurpfile addr "$m.addr.json" --slurpfile r4 "$m.route4.json" --slurpfile r6 "$m.route6.json" \
    '{addr:$addr[0], r4:($r4[0] // []), r6:($r6[0] // [])} | '"$RESTORE_JQ" > "$m.restore" || return 1
  if grep -vqE '^[-A-Za-z0-9:./_ ]+$' "$m.restore"; then echo "unexpected token in the restore plan of $dev" >&2; return 1; fi
  jq -r '(.[0].addr_info // [])[] | select(.scope != "link") | "\(.local)/\(.prefixlen)"' "$m.addr.json" | sort > "$m.addrs"
  jq -r '.[]? | select(.dst == "default" and .gateway != null) | .gateway' "$m.route4.json" "$m.route6.json" > "$m.gateways"
  netmgr_of "$dev" > "$m.netmgr"
  [[ -s $m.addrs ]] || { echo "management interface $dev has no global address to protect" >&2; return 1; }
}

render() {  # <out> — the generator validates against this host and keeps the live file's plugin switches
  "$STARTUPGEN_BIN" --current "$VRX_STARTUP_CONF" -o "$1" "${GEN_ARGS[@]}" "$DOC"
}

# ---------------------------------------------------------------- handover gate (D-012; re-review N8)
handover_flag() {  # stdin: doc text → pending|done (no/ambiguous flag → pending) — same rule as tools/lab
  local s; s="$(grep -oE '^`?handover: *(pending|done)' | head -1 | grep -oE 'pending|done' || true)"; echo "${s:-pending}"
}
handover_sources() {  # → "<source> <pending|done>" lines: the canonical copy always, this tree's copy, extras
  local rel="docs/lab/host-$VM.md" repo f
  if [[ -r $CANON_ROOT/$rel ]]; then echo "$CANON_ROOT/$rel $(handover_flag < "$CANON_ROOT/$rel")"
  else echo "$CANON_ROOT/$rel(absent) pending"; fi
  repo="$(cd "$HERE/../.." 2>/dev/null && pwd || true)"
  if [[ -n $repo && $repo != "$CANON_ROOT" && -r $repo/$rel ]]; then echo "$repo/$rel $(handover_flag < "$repo/$rel")"; fi
  for f in $VRX_HANDOVER_EXTRA; do
    if [[ -r $f ]]; then echo "$f $(handover_flag < "$f")"; else echo "$f(absent) pending"; fi
  done
}
gate() {  # → prints the gate record; returns 3 when --apply is not allowed
  local st="done" src s summary=""
  while read -r src s; do summary+="${summary:+ · }$src=$s"; [[ $s == "done" ]] || st=pending; done < <(handover_sources)
  if [[ $st == "done" ]]; then echo "gate: handover done ($summary)"; return 0; fi
  if [[ -n $APPROVAL ]]; then
    echo "gate: handover pending ($summary) — APPLIED UNDER PRODUCT-OWNER APPROVAL $APPROVAL by ${SUDO_USER:-${USER:-uid $(id -u)}}"
    return 0
  fi
  echo "REFUSED: docs/lab/host-$VM.md says handover: pending ($summary) — a VPP restart needs handover: done or --i-have-product-owner-approval <PENDING-id> (D-012)"
  return 3
}

# ---------------------------------------------------------------- dry run
dry_run() {
  [[ -f $VRX_STARTUP_CONF ]] || die "$VRX_STARTUP_CONF not found"
  local tmp rc=0 d p; tmp="$(mktemp -d)"
  render "$tmp/new.conf" || { rm -rf "$tmp"; die "rendering failed (nothing changed)"; }
  echo "== unified diff: $VRX_STARTUP_CONF → rendered"
  "$STARTUPGEN_BIN" --current "$VRX_STARTUP_CONF" --diff "$VRX_STARTUP_CONF" "${GEN_ARGS[@]}" "$DOC" 2>/dev/null || true
  echo "== semantic diff (comments, order, indentation ignored)"
  "$STARTUPGEN_BIN" --current "$VRX_STARTUP_CONF" --diff "$VRX_STARTUP_CONF" --semantic "${GEN_ARGS[@]}" "$DOC" 2>/dev/null || true
  echo "== PCI devices involved (driver now)"
  { conf_pcis "$VRX_STARTUP_CONF"; conf_pcis "$tmp/new.conf"; } | sort -u | while read -r p; do echo "  $p $(pci_driver "$p")"; done
  echo "== management interfaces protected (network manager; exact restore plan)"
  WORK="$tmp"
  while read -r d; do
    if snapshot_mgmt "$d"; then
      p="$(if_pci "$d")"
      echo "  $d pci=${p:--} driver=$(pci_driver "${p:-none}") manager=$(cat "$tmp/mgmt/$d.netmgr") addrs=$(tr '\n' ' ' < "$tmp/mgmt/$d.addrs")"
      sed 's/^/    ip /' "$tmp/mgmt/$d.restore"
    else echo "  $d: SNAPSHOT FAILED — --apply will refuse"; rc=3; fi
  done < <(mgmt_ifs)
  [[ -n $(mgmt_ifs 2>/dev/null) ]] || { echo "  none found — --apply will refuse (use --mgmt-if)"; rc=3; }
  echo "== VPP preflight (read-only: vrx-vppcheck ifaces local0)"
  if ! vppcheck ifaces local0 2>&1 | sed 's/^/  /'; then echo "  VPP preflight FAILED — --apply will refuse"; rc=3; fi
  echo "== handover gate (--apply only)"
  gate | sed 's/^/  /' || true
  echo "== sha256 of $VRX_STARTUP_CONF (--expect-sha256)"
  sha "$VRX_STARTUP_CONF"
  echo "== sha256 of the rendering (--expect-new-sha256)"
  sha "$tmp/new.conf"
  echo "dry run: nothing changed"
  rm -rf "$tmp"
  return "$rc"
}

# ---------------------------------------------------------------- locking
take_locks() {  # <timeout>
  exec 8>"$VRX_VPP_LOCK" 9>"$VRX_LAB_LOCK"
  flock -x -w "$1" 8 || { say "locks: $VRX_VPP_LOCK busy for ${1}s"; return 3; }
  flock -x -w "$1" 9 || { say "locks: $VRX_LAB_LOCK busy for ${1}s"; return 3; }
}

# ---------------------------------------------------------------- health
wait_api() {  # bounded by API_WAIT (every probe by CMD_TIMEOUT)
  local end=$((SECONDS + API_WAIT))
  while :; do
    if [[ -S $VRX_VPP_API_SOCKET ]] && vppcheck version 2>/dev/null | grep -q '^vpp '; then return 0; fi
    ((SECONDS < end)) || return 1
    sleep 1
  done
}
mgmt_addrs_ok() {  # <if> — link UP and every recorded address present
  local dev="$1" now missing
  tmo "$VRX_IP" -o link show dev "$dev" 2>/dev/null | grep -qE '[<,]UP[,>]' || { echo "management interface $dev is not up"; return 1; }
  now="$(tmo "$VRX_IP" -j addr show dev "$dev" 2>/dev/null | jq -r '(.[0].addr_info // [])[] | "\(.local)/\(.prefixlen)"' 2>/dev/null | sort)" || true
  missing="$(comm -23 "$WORK/mgmt/$dev.addrs" <(printf '%s\n' "$now"))"
  [[ -z $missing ]] || { echo "management interface $dev lost $(tr '\n' ' ' <<<"$missing")"; return 1; }
}
check_mgmt() {  # every protected interface: up, addressed as recorded, same driver, gateway answering
  local dev pci gw
  while read -r dev; do
    mgmt_addrs_ok "$dev" || return 1
    pci="$(if_pci "$dev")"
    if [[ -n $pci ]]; then
      [[ $(pci_driver "$pci") == "$(awk -v p="$pci" '$1==p{print $2}' "$WORK/drivers")" ]] || { echo "management NIC $pci changed driver"; return 1; }
    fi
    while read -r gw; do
      [[ -n $gw ]] || continue
      tmo "$VRX_PING" -c 1 -W 2 -I "$dev" "$gw" >/dev/null 2>&1 || { echo "gateway $gw on $dev does not answer"; return 1; }
    done < "$WORK/mgmt/$dev.gateways"
  done < "$WORK/mgmt.ifs"
}
check_health() {  # → prints the first problem, returns 1
  tmo "$VRX_SYSTEMCTL" is-active --quiet vpp || { echo "vpp.service is not active"; return 1; }
  [[ $(nrestarts) == "$(cat "$WORK/nrestarts")" ]] || { echo "vpp.service restarted by itself (NRestarts $(cat "$WORK/nrestarts") → $(nrestarts))"; return 1; }
  [[ -S $VRX_VPP_API_SOCKET ]] || { echo "no API socket $VRX_VPP_API_SOCKET"; return 1; }
  vppcheck version >/dev/null 2>&1 || { echo "VPP API does not answer within ${CMD_TIMEOUT}s (hung?)"; return 1; }
  local loaded p names out
  loaded="$(vppcheck plugins 2>/dev/null)" || { echo "VPP did not list its plugins within ${CMD_TIMEOUT}s (hung?)"; return 1; }
  for p in $(conf_plugins "$WORK/new.conf" enable); do grep -qx "$p" <<<"$loaded" || { echo "plugin $p enabled but not loaded"; return 1; }; done
  for p in $(conf_plugins "$WORK/new.conf" disable); do ! grep -qx "$p" <<<"$loaded" || { echo "plugin $p disabled but loaded"; return 1; }; done
  while read -r p; do
    [[ -n $p ]] || continue
    grep -qx "$p" <<<"$(conf_plugins "$WORK/new.conf" disable)" && continue
    # re-review N6 / D-084: a plugin the old file enabled and the new file no longer mentions may be
    # default-disabled in VPP (linux_cp, linux_nl, npt66): its absence is the intended result
    if grep -qx "$p" <<<"$(conf_plugins "$WORK/backup.conf" enable)" && ! grep -qE "^[[:space:]]*plugin[[:space:]]+${p//./\\.}[[:space:]]*\\{" "$WORK/new.conf"; then continue; fi
    grep -qx "$p" <<<"$loaded" || { echo "plugin $p was loaded before and is gone"; return 1; }
  done < "$WORK/plugins.before"
  mapfile -t names < <(conf_names "$WORK/new.conf")
  if ((${#names[@]})); then
    out="$(vppcheck ifaces "${names[@]}" 2>&1)" || { echo "logical interface(s) not in VPP (API check): $out"; return 1; }
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
    if command -v "$VRX_DRIVERCTL" >/dev/null 2>&1; then tmo "$VRX_DRIVERCTL" unset-override "$pci" >/dev/null 2>&1 || true; fi
    [[ $cur == none ]] || sysfs_write "$pci" "$VRX_SYSFS/bus/pci/devices/$pci/driver/unbind" || true
    if [[ -e $VRX_SYSFS/bus/pci/devices/$pci/driver_override ]]; then sysfs_write "" "$VRX_SYSFS/bus/pci/devices/$pci/driver_override" || true; fi
    [[ $want == none ]] || sysfs_write "$pci" "$VRX_SYSFS/bus/pci/drivers/$want/bind" || say "WARNING: bind $pci to $want failed"
  done < "$WORK/drivers"
}
apply_restore_plan() {  # <if>
  local -a a
  while read -r -a a; do
    ((${#a[@]})) || continue
    say "  ip ${a[*]}"
    tmo "$VRX_IP" "${a[@]}" || say "  (failed: ip ${a[*]})"
  done < "$WORK/mgmt/$1.restore"
}
restore_mgmt() {  # re-review N2: independent of the network manager first, the manager's own re-apply second
  local dev why i mgr
  while read -r dev; do
    for ((i = 0; i < 10; i++)); do
      if [[ -e $VRX_SYSFS/class/net/$dev ]]; then break; fi
      sleep 1
    done
    tmo "$VRX_IP" link set dev "$dev" up || true
    if why="$(mgmt_addrs_ok "$dev")"; then say "management interface $dev: addresses intact"; continue; fi
    say "management interface $dev: $why — re-applying the recorded addresses and routes"
    apply_restore_plan "$dev"
    if mgmt_addrs_ok "$dev" >/dev/null; then continue; fi
    mgr="$(cat "$WORK/mgmt/$dev.netmgr")"
    say "management interface $dev still broken — network manager re-apply ($mgr)"
    case "$mgr" in
      ifupdown) tmo "$VRX_IFUP" --force "$dev" || true ;;
      networkd) tmo "$VRX_NETWORKCTL" reconfigure "$dev" || true ;;
      netplan) tmo "$VRX_NETPLAN" apply || true ;;
      *) say "  no network manager known for $dev" ;;
    esac
    tmo "$VRX_IP" link set dev "$dev" up || true
    apply_restore_plan "$dev"
  done < "$WORK/mgmt.ifs"
}
cancel_deadman() {
  local u p
  u="$(cat "$WORK/deadman-unit" 2>/dev/null || true)"
  if [[ -n $u ]]; then tmo "$VRX_SYSTEMCTL" stop "$u.timer" >/dev/null 2>&1 || true; fi
  p="$(cat "$WORK/deadman-pid" 2>/dev/null || true)"
  if [[ -n $p && $p != "$$" ]] && tr '\0' ' ' < "/proc/$p/cmdline" 2>/dev/null | grep -qF -- "--stage rollback --work $WORK"; then kill "$p" 2>/dev/null || true; fi
}
rollback() {  # restore file + drivers + management path, restart VPP, verify
  say "ROLLBACK: $1"
  syslog "ROLLBACK $WORK: $1"
  if ! svc stop vpp; then
    say "systemctl stop vpp failed or hung for ${SVC_TIMEOUT}s — SIGKILL"
    svc kill --signal=KILL vpp || true
  fi
  install -m 0644 "$WORK/backup.conf" "$VRX_STARTUP_CONF"
  rebind_drivers
  restore_mgmt
  svc reset-failed vpp >/dev/null 2>&1 || true   # re-review N11: never stuck in start-limit-hit
  svc start vpp || say "systemctl start vpp failed"
  local why=""
  if ! wait_api; then why="VPP API did not come up"
  elif ! tmo "$VRX_SYSTEMCTL" is-active --quiet vpp; then why="vpp.service not active"
  else why="$(check_mgmt)" || true; fi
  if [[ -z $why ]]; then
    touch "$WORK/rolled-back"
    say "rolled back to $WORK/backup.conf; VPP and the management path are healthy"
    syslog "rolled back $WORK"
    cancel_deadman
  else
    touch "$WORK/rollback-incomplete"
    say "ROLLBACK INCOMPLETE ($why) — the dead-man stays armed and retries; console access may be needed"
    syslog "ROLLBACK INCOMPLETE $WORK: $why"
  fi
}

# ---------------------------------------------------------------- stages
write_settings() {
  local v
  for v in "${SETTINGS_VARS[@]}"; do
    [[ ${!v} != *$'\n'* ]] || die "setting $v contains a newline"
    printf '%s=%s\n' "$v" "${!v}"
  done > "$WORK/settings"
}
load_settings() {  # the work dir is authoritative; a differing environment is reported (N6 regression guard)
  [[ -f $WORK/settings ]] || die "$WORK/settings missing"
  local line k v
  while IFS= read -r line; do
    k="${line%%=*}" v="${line#*=}"
    [[ " ${SETTINGS_VARS[*]} " == *" $k "* ]] || die "unknown setting '$k' in $WORK/settings"
    if [[ $k == VRX_* && ${!k} != "$v" ]]; then say "WARNING: environment $k='${!k}' differs from the recorded '$v' (using the recorded value)"; fi
    printf -v "$k" '%s' "$v"
  done < "$WORK/settings"
  STARTUPGEN_BIN="$WORK/bin/vrx-startupgen" VPPCHECK_BIN="$WORK/bin/vrx-vppcheck" DOC="$WORK/doc.json"
  mapfile -t GEN_ARGS < "$WORK/gen-args"
  [[ -n ${GEN_ARGS[0]:-} ]] || GEN_ARGS=()
}
setenv_args() {  # → --setenv=NAME=value per setting: the unit never depends on systemd's own environment
  local v
  for v in "${SETTINGS_VARS[@]}"; do if [[ $v == VRX_* ]]; then printf -- '--setenv=%s=%s\n' "$v" "${!v}"; fi; done
  printf -- '--setenv=PATH=%s\n' "$PATH"
}

detach() {  # pin everything into the work dir, then run "--stage run" outside the SSH session
  local stamp; stamp="$(date +%Y%m%d-%H%M%S)-$$"
  WORK="$VRX_APPLY_STATE/$stamp"
  install -d -m 0750 "$WORK" "$WORK/bin"
  install -m 0640 "$DOC" "$WORK/doc.json"
  install -m 0755 "$STARTUPGEN_BIN" "$WORK/bin/vrx-startupgen"
  install -m 0755 "$VPPCHECK_BIN" "$WORK/bin/vrx-vppcheck"
  install -m 0755 "$SELF" "$WORK/bin/apply-startup.sh"
  printf '%s\n' "${GEN_ARGS[@]}" > "$WORK/gen-args"
  if [[ -n ${SSH_CONNECTION:-} ]]; then VRX_MGMT_PEERS="${VRX_MGMT_PEERS:+$VRX_MGMT_PEERS }${SSH_CONNECTION%% *}"; fi
  write_settings
  echo "$GATE" > "$WORK/gate"
  local cmd=("$WORK/bin/apply-startup.sh" --stage run --work "$WORK")
  if ((FOREGROUND)); then "${cmd[@]}"; return; fi
  local -a envs; mapfile -t envs < <(setenv_args)
  if "$VRX_SYSTEMD_RUN" --unit="$UNIT_PREFIX-$stamp" --collect --quiet --property=KillMode=process "${envs[@]}" "${cmd[@]}"; then
    echo "$UNIT_PREFIX-$stamp" > "$WORK/run-unit"
    say "started detached unit $UNIT_PREFIX-$stamp — follow: journalctl -fu $UNIT_PREFIX-$stamp  (log: $WORK/log)"
  else
    setsid "${cmd[@]}" </dev/null >/dev/null 2>&1 &
    say "systemd-run unavailable: started with setsid (pid $!) — follow: tail -f $WORK/log"
  fi
}

log_to_work() { exec > >(tee -a "$WORK/log") 2>&1; }

stage_run() {
  [[ -d $WORK ]] || die "--work $WORK missing"
  log_to_work
  load_settings
  echo "$$ $(ps -o pgid= -p $$ | tr -d ' ')" > "$WORK/run-pid"
  [[ -s $WORK/gate ]] || { say "REFUSED: no handover gate record"; return 3; }
  say "$(cat "$WORK/gate")"
  syslog "apply $WORK started; $(cat "$WORK/gate")"
  take_locks "$LOCK_TIMEOUT" || { say "REFUSED: locks busy"; return 3; }
  say "locks held: $VRX_VPP_LOCK, $VRX_LAB_LOCK"
  local now; now="$(sha "$VRX_STARTUP_CONF")"
  [[ $now == "$EXPECT" ]] || { say "REFUSED: $VRX_STARTUP_CONF changed since the review (sha256 $now, expected $EXPECT) — dry-run again"; return 3; }
  render "$WORK/new.conf" || { say "REFUSED: rendering failed"; return 3; }
  now="$(sha "$WORK/new.conf")"
  [[ $now == "$EXPECT_NEW" ]] || { say "REFUSED: the rendering differs from the reviewed one (sha256 $now, expected $EXPECT_NEW) — document, generator or host facts changed; dry-run again"; return 3; }
  local pre; pre="$(vppcheck ifaces local0 2>&1)" || { say "REFUSED: VPP preflight failed (vrx-vppcheck ifaces local0): $pre"; return 3; }
  say "preflight: $pre"
  if cmp -s "$VRX_STARTUP_CONF" "$WORK/new.conf"; then say "nothing to do: the rendering equals $VRX_STARTUP_CONF"; touch "$WORK/committed"; return 0; fi

  mgmt_ifs > "$WORK/mgmt.ifs" || true
  [[ -s $WORK/mgmt.ifs ]] || { say "REFUSED: no management interface found (no default route, no --mgmt-if/--mgmt-peer)"; return 3; }
  local dev p
  while read -r dev; do snapshot_mgmt "$dev" || { say "REFUSED: cannot snapshot management interface $dev"; return 3; }; done < "$WORK/mgmt.ifs"
  { conf_pcis "$VRX_STARTUP_CONF"; conf_pcis "$WORK/new.conf"; while read -r dev; do if_pci "$dev"; done < "$WORK/mgmt.ifs"; } \
    | sed '/^$/d' | sort -u | while read -r p; do echo "$p $(pci_driver "$p")"; done > "$WORK/drivers"
  vppcheck plugins > "$WORK/plugins.before" || { say "REFUSED: VPP did not list its plugins"; return 3; }
  nrestarts > "$WORK/nrestarts"
  cp -p "$VRX_STARTUP_CONF" "$WORK/backup.conf"
  cp -p "$VRX_STARTUP_CONF" "$VRX_STARTUP_CONF.bak-$(basename "$WORK")"
  say "backup: $WORK/backup.conf and $VRX_STARTUP_CONF.bak-$(basename "$WORK")"
  say "unified diff:"; "$STARTUPGEN_BIN" --current "$VRX_STARTUP_CONF" --diff "$VRX_STARTUP_CONF" "${GEN_ARGS[@]}" "$DOC" 2>/dev/null || true
  say "semantic diff:"; "$STARTUPGEN_BIN" --current "$VRX_STARTUP_CONF" --diff "$VRX_STARTUP_CONF" --semantic "${GEN_ARGS[@]}" "$DOC" 2>/dev/null || true
  say "recorded drivers: $(tr '\n' ';' < "$WORK/drivers") NRestarts: $(cat "$WORK/nrestarts")"
  while read -r dev; do
    say "management $dev ($(cat "$WORK/mgmt/$dev.netmgr")): $(tr '\n' ' ' < "$WORK/mgmt/$dev.addrs")gw $(tr '\n' ' ' < "$WORK/mgmt/$dev.gateways")"
  done < "$WORK/mgmt.ifs"

  # time budget of this run; the dead-man fires after it and kills a run that overstays
  local iter=$((12 * CMD_TIMEOUT + INTERVAL))
  local run_end=$((SECONDS + SVC_TIMEOUT + API_WAIT + WINDOW + iter))
  local after=$((SVC_TIMEOUT + API_WAIT + WINDOW + 2 * iter + 30)) deadman
  deadman="$UNIT_PREFIX-deadman-$(basename "$WORK")"
  echo "$deadman" > "$WORK/deadman-unit"
  local -a envs; mapfile -t envs < <(setenv_args)
  if ! tmo "$VRX_SYSTEMD_RUN" --unit="$deadman" --on-active="$after" --timer-property=AccuracySec=1s --collect --quiet "${envs[@]}" \
      "$WORK/bin/apply-startup.sh" --stage rollback --work "$WORK"; then
    : > "$WORK/deadman-unit"
    setsid "$WORK/bin/apply-startup.sh" --stage rollback --work "$WORK" --delay "$after" </dev/null >/dev/null 2>&1 8>&- 9>&- &   # N9: no lock fds
    echo "$!" > "$WORK/deadman-pid"
    say "systemd-run unavailable for the timer: dead-man started with setsid (pid $!)"
  fi
  say "dead-man armed: rollback in ${after}s unless committed"

  touch "$WORK/installed"
  install -m 0644 "$WORK/new.conf" "$VRX_STARTUP_CONF"
  say "installed $VRX_STARTUP_CONF; restarting VPP"
  if ! svc restart vpp; then rollback "systemctl restart vpp failed or hung for ${SVC_TIMEOUT}s"; return 1; fi
  if ! wait_api; then rollback "VPP API did not come up within ${API_WAIT}s (hung or crashed)"; return 1; fi
  local end=$((SECONDS + WINDOW)) why
  while :; do
    if ! why="$(check_health)"; then rollback "$why"; return 1; fi
    ((SECONDS < run_end)) || { rollback "the run exceeded its time budget"; return 1; }
    ((SECONDS < end)) || break
    sleep "$INTERVAL"
  done
  [[ ! -e $WORK/deadman-fired ]] || { say "dead-man already fired — not committing"; return 1; }
  touch "$WORK/committed"
  cancel_deadman
  say "COMMITTED: healthy for ${WINDOW}s; dead-man cancelled. Record it in docs/decisions/LOG.md (backup $WORK/backup.conf)"
  syslog "COMMITTED $WORK"
}

kill_run() {  # the run is stuck or dead: make sure it is gone (only the process we started, checked by its cmdline)
  local pid pgid u i
  u="$(cat "$WORK/run-unit" 2>/dev/null || true)"
  if [[ -n $u && -z $DEADMAN_FALLBACK ]]; then  # a fallback dead-man lives in the run unit's cgroup: never kill the unit then
    say "dead-man: killing unit $u"
    tmo "$VRX_SYSTEMCTL" kill --kill-whom=all --signal=KILL "$u.service" >/dev/null 2>&1 || true
  fi
  read -r pid pgid < "$WORK/run-pid" 2>/dev/null || return 0
  if [[ -n $pid ]] && tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null | grep -qF -- "--stage run --work $WORK"; then
    if [[ $pgid == "$pid" ]]; then say "dead-man: killing the run's process group $pgid"; kill -KILL -- "-$pgid" 2>/dev/null || true
    else say "dead-man: killing the run (pid $pid)"; kill -KILL "$pid" 2>/dev/null || true; fi
    for ((i = 0; i < 50; i++)); do
      if [[ ! -e /proc/$pid ]]; then break; fi
      sleep 0.1
    done
  fi
}

done_already() { [[ -e $WORK/committed || -e $WORK/rolled-back ]]; }
stage_rollback() {  # the dead-man
  [[ -d $WORK ]] || die "--work $WORK missing"
  log_to_work
  load_settings
  ((DELAY == 0)) || DEADMAN_FALLBACK=1
  sleep "$DELAY"
  if done_already; then say "dead-man: nothing to do (the apply already committed or rolled back)"; return 0; fi
  touch "$WORK/deadman-fired"
  say "dead-man fired: the run did not commit in time"
  kill_run
  if done_already; then say "dead-man: the run finished while being stopped — nothing to do"; return 0; fi
  if take_locks "$DEADMAN_LOCK_TIMEOUT"; then say "dead-man: locks held"
  else
    say "dead-man: FORCED — locks still busy after ${DEADMAN_LOCK_TIMEOUT}s; rolling back without them (a lost management path outranks the lock)"
    syslog "dead-man FORCED rollback without locks $WORK"
  fi
  if [[ ! -e $WORK/installed ]]; then say "dead-man: nothing was installed — nothing to roll back"; touch "$WORK/rolled-back"; return 0; fi
  rollback "dead-man timer fired before the apply committed"
  return 1
}

main() {
  parse_args "$@"
  case "$STAGE" in
    run) stage_run ;;
    rollback) stage_rollback ;;
    "")
      [[ -n $DOC && -f $DOC ]] || die "--doc <document.json> is required"
      [[ -x $STARTUPGEN_BIN ]] || die "$STARTUPGEN_BIN not found (cd apps/agent && go build -o bin/vrx-startupgen ./cmd/vrx-startupgen)"
      [[ -x $VPPCHECK_BIN ]] || die "$VPPCHECK_BIN not found (cd apps/agent && go build -o bin/vrx-vppcheck ./cmd/vrx-vppcheck)"
      command -v jq >/dev/null 2>&1 || die "jq is required (management snapshot)"
      if ((APPLY)); then
        [[ $EXPECT =~ ^[0-9a-f]{64}$ ]] || die "--apply needs --expect-sha256 <live sum printed by the dry run>"
        [[ $EXPECT_NEW =~ ^[0-9a-f]{64}$ ]] || die "--apply needs --expect-new-sha256 <rendered sum printed by the dry run>"
        [[ -f $VRX_STARTUP_CONF ]] || die "$VRX_STARTUP_CONF not found"
        if ! GATE="$(gate)"; then echo "apply-startup: $GATE" >&2; exit 3; fi
        say "$GATE"
        syslog "$GATE"
        detach
      else
        dry_run
      fi ;;
    *) die "unknown stage '$STAGE'" ;;
  esac
}

if [[ ${BASH_SOURCE[0]} == "$0" ]]; then main "$@"; fi
