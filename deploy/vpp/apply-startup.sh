#!/usr/bin/env bash
# deploy/vpp/apply-startup.sh — the manager's tool to apply a generated VPP startup.conf
# (F-startup-apply; generator: F-startup-gen, WBS D0.6). MANAGER ONLY. Refuses --apply unless
# docs/lab/host-<vm>.md says `handover: done` (D-012) or the product owner approved the change:
# --i-have-product-owner-approval PENDING-<slug> — docs/decisions/PENDING-<slug>.md must exist on main of /root/ngfw
# and a D-row on main must have that PENDING id as the SUBJECT of its decision text (a mention elsewhere does not count)
# AND name the sha256 of this rendering (--expect-new-sha256): the approval covers exactly one change. An approval
# already executed for that rendering (a committed work dir) is refused. Resolved, recorded in the gate and logged.
#
#   apply-startup.sh --doc <document.json> [options] [-- <vrx-startupgen flags>]
#       DRY RUN (default): render; unified + semantic diff; drivers of every PCI device involved; the
#       protected management interfaces (default routes, the SSH peer's path, --mgmt-if) with network
#       manager and exact restore plan; the management REACHABILITY CHECK that will be used (exit 3 when none
#       is viable on this host); a read-only VPP preflight; the gate; the sha256 of the live file and of the
#       rendering. Changes nothing. Exit 3 when --apply would refuse.
#   apply-startup.sh --doc <document.json> --apply --expect-sha256 <live sum> --expect-new-sha256 <rendered sum>
#       APPLY (needs systemd-run — there is no fallback): the planner evaluates the gate, copies the document,
#       the generator, vrx-vppcheck and this script into a work dir, records every setting there, seals the plan
#       (<work>/plan.sha256) and starts the run as its own unit (systemd-run --unit=vrx-startup-apply-<stamp>,
#       settings as --setenv); the caller returns at once. The run:
#         1. verifies the seal, re-evaluates the gate (must equal the sealed record) and starts a LOCK HOLDER
#            unit that takes flock -x on /run/lock/vrx-vpp.lock then /run/lock/vrx-lab.lock (tools/lab order,
#            bounded) and keeps them until the apply has committed or rolled back
#         2. refuses unless sha256(live) == --expect-sha256 and sha256(rendering) == --expect-new-sha256; VPP
#            preflight; snapshot of the management interfaces; the reachability check passes now (baseline)
#         3. records NIC drivers, loaded plugins, the VPP boot identity (D-080); backup; diffs; arms the dead-man
#            timer (systemd-run --on-active; if it cannot be armed: refuse)
#         4. installs, restarts VPP, reads NRestarts + MainPID/ActiveEnterTimestamp right after the restart,
#            waits for the API, waits --settle seconds, then requires: a NEW boot identity whose PID is the unit's
#            MainPID and no restart since (NRestarts, MainPID, ActiveEnter unchanged); then for --window seconds
#            (at least two identity reads): the same, plus plugins per `show plugins`, logical interfaces per
#            sw_interface_dump, every management interface with addresses and routes exactly as snapshotted, NIC
#            driver unchanged, the reachability check, and the lock holder alive
#         5. any failure → ONE rollback: stop VPP (SIGKILL if stop hangs), restore the backup, rebind changed
#            NICs, re-apply the recorded addresses/routes verbatim, then only if still broken the network manager
#            (ifupdown `ifup --force` / networkd `networkctl reconfigure` / `netplan apply`), reset-failed + start
#            VPP, verify with the SAME checks as the snapshot. Healthy → rolled-back; not → console-needed. Either
#            way the timer is cancelled and the locks are released — VPP is never restarted in a loop.
#       Reachability (--mgmt-probe): auto = `neigh` — the default route's next hop answers ARP/ND (REACHABLE
#       after a fresh nudge) on top of the exact address/route match; independent of the operator's session.
#       tcp:HOST:PORT = a TCP connect to a target routed through a management interface (not loopback).
#       gateway-ping = ICMP to the default gateway(s). The manager's SSH session is logged as an extra signal
#       only: closing it never fails an apply or a rollback.
#       The dead-man (fires after the run's worst-case budget plus a worst-case rollback) kills the run's
#       process tree, and — only if no rollback has finished — makes at most one rollback, then writes the
#       result marker and releases the locks. If the holder is gone it takes the locks itself, exclusively and
#       bounded, before touching the run; only a foreign holder makes it roll back without them (FORCED, logged).
#       A rollback stage first takes a per-apply lock (flock on the apply's work dir, non-blocking): a second rollback
#       of the same apply is refused while one runs. It then disarms the timer. An unfinished apply is finished by hand
#       through the dead-man's own unit: systemctl start vrx-startup-apply-deadman-<stamp>.service (systemd runs a
#       unit only once).
#   apply-startup.sh --stage run|rollback|hold --work <dir>     internal
#
# Options: --vm NAME (vrx-a) · --window S (60, ≥ --interval) · --interval S (5, > 0) · --settle S (10) ·
#   --api-wait S (60) · --cmd-timeout S (10) · --svc-timeout S (120) · --lock-timeout S (600) ·
#   --deadman-lock-timeout S (60) · --mgmt-if IF (repeatable) · --mgmt-peer IP (the client of $SSH_CONNECTION is
#   added automatically) · --mgmt-probe auto|neigh|gateway-ping|tcp:HOST:PORT · --foreground (run the run stage
#   in this process; refused over SSH unless --console) · --i-have-product-owner-approval PENDING-<slug>
#
# Logs and state: /var/lib/vrx/startup-apply/<stamp>/ (log, settings, gate, plan.sha256, doc.json, bin/, new.conf,
# backup.conf, drivers, mgmt.ifs, mgmt/<if>.*, probe, ident.*, unit.*, plugins.before, installed, rollback-started,
# rollback-pid, committed | rolled-back | console-needed | superseded). Syslog tag vrx-startup-apply.
# Exit: 0 ok / nothing to do · 1 failed (rolled back or console needed) · 2 usage · 3 refused before any change.
#
# Every system path and command is overridable through VRX_* variables (below) so the script is tested against a
# fake host (deploy/vpp/test-apply-startup.sh). VRX_TEST_ROOT replaces the canonical /root/ngfw with
# $VRX_TEST_ROOT/canon ONLY when startup.conf, sysfs, systemctl and systemd-run all live under VRX_TEST_ROOT.
# Workers never run --apply for real.
set -euo pipefail

SELF="$(readlink -f "${BASH_SOURCE[0]}")"
HERE="$(dirname "$SELF")"

# ---------------------------------------------------------------- settings (recorded + passed on explicitly)
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
: "${VRX_SS:=ss}"
: "${VRX_PING:=ping}"
: "${VRX_TCPCONNECT:=}"                                # empty: bash /dev/tcp
: "${VRX_DRIVERCTL:=driverctl}"
: "${VRX_NETPLAN:=netplan}"
: "${VRX_IFUP:=ifup}"
: "${VRX_NETWORKCTL:=networkctl}"
: "${VRX_LOGGER:=logger}"
: "${VRX_LSLOCKS:=lslocks}"
: "${VRX_VPPCHECK:=/root/ngfw/apps/agent/bin/vrx-vppcheck}"
: "${VRX_STARTUPGEN:=/root/ngfw/apps/agent/bin/vrx-startupgen}"
: "${VRX_HANDOVER_EXTRA:=}"                             # extra handover docs: can only make the gate stricter
: "${VRX_TEST_ROOT:=}"
: "${VRX_MGMT_PEERS:=}"
: "${VRX_MGMT_IFS:=}"
: "${VRX_MGMT_PROBE:=auto}"
SETTINGS_VARS=(VRX_STARTUP_CONF VRX_SYSFS VRX_ETC VRX_APPLY_STATE VRX_LAB_LOCK VRX_VPP_LOCK VRX_VPP_API_SOCKET
  VRX_SYSTEMCTL VRX_SYSTEMD_RUN VRX_IP VRX_SS VRX_PING VRX_TCPCONNECT VRX_DRIVERCTL VRX_NETPLAN VRX_IFUP VRX_NETWORKCTL
  VRX_LOGGER VRX_LSLOCKS VRX_VPPCHECK VRX_STARTUPGEN VRX_HANDOVER_EXTRA VRX_TEST_ROOT VRX_MGMT_PEERS VRX_MGMT_IFS
  VRX_MGMT_PROBE WINDOW INTERVAL SETTLE API_WAIT CMD_TIMEOUT SVC_TIMEOUT LOCK_TIMEOUT DEADMAN_LOCK_TIMEOUT
  EXPECT EXPECT_NEW VM APPROVAL GATE_REPO)
UNIT_PREFIX="vrx-startup-apply"

DOC="" APPLY=0 EXPECT="" EXPECT_NEW="" WINDOW=60 INTERVAL=5 SETTLE=10 LOCK_TIMEOUT=600 DEADMAN_LOCK_TIMEOUT=60 API_WAIT=60
CMD_TIMEOUT=10 SVC_TIMEOUT=120 FOREGROUND=0 CONSOLE=0 STAGE="" WORK="" VM="vrx-a" APPROVAL="" GATE="" GATE_REPO=""
GEN_ARGS=()
STARTUPGEN_BIN="$VRX_STARTUPGEN" VPPCHECK_BIN="$VRX_VPPCHECK"
OWN_LOCKS=0      # 1 while this process itself has fds 8/9 open (the holder is gone: dead-man or the run's rollback)
LOCKS_SECURED=0  # secure_locks ran (taken, the holder's, or FORCED)
RUN_LOCKED=0 RUN_INSTALLED=0 RB_MINE=0   # the run's EXIT trap: what this process has done so far

die() { echo "apply-startup: $*" >&2; exit 2; }
say() { echo "apply-startup: $(date '+%F %T') $*"; }

canon_root() {  # the manager's main checkout; a test root only when every mutating path is inside it
  if [[ -z $VRX_TEST_ROOT ]]; then echo /root/ngfw; return; fi
  # TD-6 V6: compared after resolving `.`, `..` and symlinks — a lexical prefix test let /./etc/vpp/startup.conf through
  local root v p r b
  root="$(realpath -m -- "$VRX_TEST_ROOT")"
  [[ -n $root && $root != / ]] || die "VRX_TEST_ROOT resolves to '/' — refused"
  for v in VRX_STARTUP_CONF VRX_SYSFS VRX_SYSTEMCTL VRX_SYSTEMD_RUN VRX_IP VRX_APPLY_STATE VRX_VPP_LOCK VRX_LAB_LOCK; do
    p="${!v}"; r="$(realpath -m -- "$p")"
    [[ $p == /* && $r == "$root"/* ]] ||
      die "VRX_TEST_ROOT is only honoured when startup.conf, sysfs, systemctl, systemd-run, ip, the state dir and the locks are inside it ($v=$p resolves to $r, outside $root)"
  done
  [[ $(realpath -m -- "$VRX_STARTUP_CONF") != /etc/vpp/startup.conf && $(realpath -m -- "$VRX_SYSFS") != /sys ]] ||
    die "VRX_TEST_ROOT with the real startup.conf or sysfs — refused"
  for b in systemctl systemd-run; do
    p="$(command -v "$b" 2>/dev/null)" || continue
    r="$(realpath -m -- "$p")"
    [[ $(realpath -m -- "$VRX_SYSTEMCTL") != "$r" && $(realpath -m -- "$VRX_SYSTEMD_RUN") != "$r" ]] ||
      die "VRX_TEST_ROOT with the real $b ($r) — refused"
  done
  echo "$root/canon"
}

parse_args() {
  while (($#)); do
    case "$1" in
      --doc) DOC="${2:?--doc needs a file}"; shift ;;
      --apply) APPLY=1 ;;
      --expect-sha256) EXPECT="${2:?}"; shift ;;
      --expect-new-sha256) EXPECT_NEW="${2:?}"; shift ;;
      --window) WINDOW="${2:?}"; shift ;;
      --interval) INTERVAL="${2:?}"; shift ;;
      --settle) SETTLE="${2:?}"; shift ;;
      --lock-timeout) LOCK_TIMEOUT="${2:?}"; shift ;;
      --deadman-lock-timeout) DEADMAN_LOCK_TIMEOUT="${2:?}"; shift ;;
      --api-wait) API_WAIT="${2:?}"; shift ;;
      --cmd-timeout) CMD_TIMEOUT="${2:?}"; shift ;;
      --svc-timeout) SVC_TIMEOUT="${2:?}"; shift ;;
      --vm) VM="${2:?}"; shift ;;
      --mgmt-if) VRX_MGMT_IFS="${VRX_MGMT_IFS:+$VRX_MGMT_IFS }${2:?}"; shift ;;
      --mgmt-peer) VRX_MGMT_PEERS="${VRX_MGMT_PEERS:+$VRX_MGMT_PEERS }${2:?}"; shift ;;
      --mgmt-probe) VRX_MGMT_PROBE="${2:?}"; shift ;;
      --i-have-product-owner-approval) APPROVAL="${2:?}"; shift ;;
      --foreground) FOREGROUND=1 ;;
      --console) CONSOLE=1 ;;
      --stage) STAGE="${2:?}"; shift ;;
      --work) WORK="${2:?}"; shift ;;
      --) shift; GEN_ARGS=("$@"); break ;;
      -h|--help) sed -n '2,/^set -euo pipefail/{/^set -euo pipefail/!p}' "$SELF" | sed -E 's/^# ?//'; exit 0 ;;
      *) die "unknown argument '$1' (try --help)" ;;
    esac
    shift
  done
  local n w
  for n in WINDOW INTERVAL SETTLE LOCK_TIMEOUT DEADMAN_LOCK_TIMEOUT API_WAIT CMD_TIMEOUT SVC_TIMEOUT; do
    [[ ${!n} =~ ^[0-9]+$ ]] || die "--${n,,} must be a number"
  done
  ((CMD_TIMEOUT > 0 && SVC_TIMEOUT > 0)) || die "timeouts must be > 0"
  ((INTERVAL > 0 && WINDOW >= INTERVAL)) || die "--window must be ≥ --interval > 0 (at least two health reads)"
  [[ $VM =~ ^[a-z0-9][a-z0-9-]*$ ]] || die "--vm must be a lab VM name"
  [[ -z $APPROVAL || $APPROVAL =~ ^PENDING-[A-Za-z0-9_-]+$ ]] ||
    die "--i-have-product-owner-approval needs a PENDING-<slug> id (docs/decisions/PENDING-<slug>.md), got '$APPROVAL'"
  [[ $VRX_MGMT_PROBE =~ ^(auto|neigh|gateway-ping|tcp:(\[[0-9A-Fa-f:.]+\]|[0-9A-Za-z.-]+):[0-9]{1,5})$ ]] ||
    die "--mgmt-probe must be auto, neigh, gateway-ping or tcp:HOST:PORT"
  for w in $VRX_MGMT_IFS; do [[ $w =~ ^[A-Za-z0-9_.@-]{1,15}$ ]] || die "--mgmt-if '$w' is not an interface name"; done
  for w in $VRX_MGMT_PEERS; do [[ $w =~ ^[0-9A-Fa-f:.]+$ ]] || die "--mgmt-peer '$w' is not an IP address"; done
}

sha() { sha256sum "$1" | awk '{print $1}'; }
ere_escape() { sed -E 's/[][\.^$*+?(){}|/]/\\&/g' <<<"$1"; }

# Every external call that can block goes through one of these (re-review N1). timeout(1) kills the
# command's whole process group; fds 7/8/9 are closed so a child never holds a lock (7 = the per-apply rollback
# lock, TD-7 F1: an orphaned child that kept it would make a later rollback of the same apply refuse).
tmo() { timeout -k 2 "$CMD_TIMEOUT" "$@" 7>&- 8>&- 9>&-; }
svc() { timeout -k 5 "$SVC_TIMEOUT" "$VRX_SYSTEMCTL" "$@" 7>&- 8>&- 9>&-; }
vppcheck() { timeout -k 2 $((CMD_TIMEOUT + 2)) "$VPPCHECK_BIN" --socket "$VRX_VPP_API_SOCKET" --timeout "${CMD_TIMEOUT}s" "$@" 7>&- 8>&- 9>&-; }
sysfs_write() { printf '%s\n' "$1" | timeout -k 2 "$CMD_TIMEOUT" tee "$2" >/dev/null 7>&- 8>&- 9>&-; }  # <value> <file>
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
unit_ident() {  # vpp.service main process, when it became active, and its automatic restarts
  tmo "$VRX_SYSTEMCTL" show vpp -p MainPID -p ActiveEnterTimestampMonotonic -p NRestarts 2>/dev/null | sort | tr '\n' ' '
}
read_unit_restart() {  # → $WORK/unit.restart right after the restart job, or fail (prints why): the baseline of every later check
  # TD-6 V3: a failing `systemctl show` (D-Bus timeout) is retried, then it is a failed apply, never a dead run.
  # TD-6 V4: the tuple must be complete, name a main process and say NRestarts=0 — systemd resets the counter on an
  # explicit restart (vrx-a journal: counter 4 → manual restart → next automatic restart "counter is at 1"), so 1+ means
  # the new instance already crashed and Restart=always brought up another one before this read.
  local i u=""
  for ((i = 0; i < 3; i++)); do
    u="$(unit_ident || true)"
    [[ $u =~ ^ActiveEnterTimestampMonotonic=[0-9]+\ MainPID=[0-9]+\ NRestarts=[0-9]+\ $ ]] && break
    u=""; sleep 0.5
  done
  printf '%s' "$u" > "$WORK/unit.restart" 2>/dev/null || true
  [[ -n $u ]] || { echo "cannot read vpp.service's state right after the restart (systemctl show failed 3 times)"; return 1; }
  [[ $u == *" NRestarts=0 "* ]] || { echo "vpp.service was restarted automatically right after the restart ($u): the new instance died at once"; return 1; }
  [[ $u != *" MainPID=0 "* ]] || { echo "vpp.service has no main process right after the restart ($u)"; return 1; }
}

mgmt_ifs() {  # → every interface to protect: default routes (v4+v6), the manager's path(s), --mgmt-if
  local peer d
  {
    tmo "$VRX_IP" -j -4 route show default 2>/dev/null | jq -r '.[]?.dev // empty' || true
    tmo "$VRX_IP" -j -6 route show default 2>/dev/null | jq -r '.[]?.dev // empty' || true
    for peer in $VRX_MGMT_PEERS; do tmo "$VRX_IP" -j route get "$peer" 2>/dev/null | jq -r '.[0]?.dev // empty' || true; done
    for d in $VRX_MGMT_IFS; do echo "$d"; done
  } | while read -r d; do
    [[ $d =~ ^[A-Za-z0-9_.@-]{1,15}$ && $d != lo ]] || continue
    # a tun/tap without a device (a linux-cp tap) belongs to VPP and goes away with it: not a management path
    if [[ -z $(if_pci "$d") && -e $VRX_SYSFS/class/net/$d/tun_flags ]]; then echo "note: $d is a tap without a device (VPP-owned), not watched" >&2; continue; fi
    echo "$d"
  done | sort -u
}

netmgr_of() {  # <if> → ifupdown | netplan | networkd | none (who re-applies the interface's configuration)
  local f re; re="$(ere_escape "$1")"
  for f in "$VRX_ETC/network/interfaces" "$VRX_ETC"/network/interfaces.d/*; do
    [[ -f $f ]] || continue
    if grep -qE "^[[:space:]]*(auto|allow-hotplug|iface)([[:space:]]+[^[:space:]]+)*[[:space:]]+$re([[:space:]]|$)" "$f"; then echo ifupdown; return; fi
  done
  if compgen -G "$VRX_ETC/netplan/*.yaml" >/dev/null && command -v "$VRX_NETPLAN" >/dev/null 2>&1; then echo netplan; return; fi
  if tmo "$VRX_SYSTEMCTL" is-active --quiet systemd-networkd 2>/dev/null; then echo networkd; return; fi
  echo none
}

# The restore plan: `ip` argument lines that re-create exactly the recorded addresses and the non-kernel
# routes of table main on the interface (kernel routes come back with the addresses). It is also the
# "exactly as snapshotted" comparison after the restart.
# shellcheck disable=SC2016  # jq program: $dev is a jq variable
RESTORE_JQ='
  ((.addr[0].addr_info // [])[] | select(.scope != "link")
    | ["addr","replace",("\(.local)/\(.prefixlen)")] + (if .broadcast then ["broadcast",.broadcast] else [] end) + ["dev",$dev]
      + (if .noprefixroute then ["noprefixroute"] else [] end)
    | join(" ")),
  (((.r4 | map(. + {fam:"-4"})) + (.r6 | map(. + {fam:"-6"})))[]
    | select((.protocol // "boot") != "kernel") | select(.nexthops == null) | select((.dst // "") | startswith("fe80") | not)
    | [.fam,"route","replace",.dst] + (if .gateway then ["via",.gateway] else [] end) + ["dev",$dev]
      + (if .protocol then ["proto",.protocol] else [] end) + (if .metric then ["metric","\(.metric)"] else [] end)
      + (if .scope then ["scope",.scope] else [] end) + (if .prefsrc then ["src",.prefsrc] else [] end)
      + (if ((.flags // []) | index("onlink")) then ["onlink"] else [] end)
    | join(" "))'
snapshot_mgmt() {  # <if> [dir] → <dir>/<if>.* ; fails when the snapshot cannot be taken
  local dev="$1" dir="${2:-$WORK/mgmt}" m
  m="$dir/$1"; mkdir -p "$dir"
  tmo "$VRX_IP" -j addr show dev "$dev" > "$m.addr.json" || return 1
  tmo "$VRX_IP" -j -4 route show table main dev "$dev" > "$m.route4.json" || return 1
  tmo "$VRX_IP" -j -6 route show table main dev "$dev" > "$m.route6.json" || return 1
  jq -n -r --arg dev "$dev" --slurpfile addr "$m.addr.json" --slurpfile r4 "$m.route4.json" --slurpfile r6 "$m.route6.json" \
    '{addr:$addr[0], r4:($r4[0] // []), r6:($r6[0] // [])} | '"$RESTORE_JQ" > "$m.restore" || return 1   # order kept: addresses before routes
  if grep -vqE '^[-A-Za-z0-9:./_ ]+$' "$m.restore"; then echo "unexpected token in the restore plan of $dev" >&2; return 1; fi
  jq -r '(.[0].addr_info // [])[] | select(.scope != "link") | "\(.local)/\(.prefixlen)"' "$m.addr.json" | sort > "$m.addrs"
  jq -r '.[]? | select(.dst == "default" and .gateway != null) | .gateway' "$m.route4.json" "$m.route6.json" > "$m.gateways"
  netmgr_of "$dev" > "$m.netmgr"
  [[ -s $m.addrs ]] || { echo "management interface $dev has no global address to protect" >&2; return 1; }
}

# ---------------------------------------------------------------- management reachability (review H2, re-review N1)
tcp_connect() {  # <host> <port> [seconds]
  local t="${3:-$CMD_TIMEOUT}"
  if [[ -n $VRX_TCPCONNECT ]]; then timeout -k 1 "$t" "$VRX_TCPCONNECT" "$1" "$2" 7>&- 8>&- 9>&-; return; fi
  # shellcheck disable=SC2016  # $0/$1 expand in the child bash
  timeout -k 1 "$t" bash -c 'exec 3<>"/dev/tcp/$0/$1"' "$1" "$2" 7>&- 8>&- 9>&-
}
nudge() {  # <dev> <gw> — any packet to the next hop makes the kernel resolve it (ARP/ND); the SYN itself may be dropped
  # TD-6 V8: a link-local gateway (fe80::/10, the usual RA default) needs its scope, or connect() fails and nothing is
  # sent; the nudge is bounded by 3 s (resolution takes ≤ 3 probes of 1 s) — vrx-a's gateway drops the SYN, so waiting
  # for the connect is time lost in every health round
  local host="$2" t=$((CMD_TIMEOUT < 3 ? CMD_TIMEOUT : 3))
  if [[ ${2,,} =~ ^fe[89ab][0-9a-f]: ]]; then host="$2%$1"; fi
  tcp_connect "$host" 9 "$t" >/dev/null 2>&1 || true
}
snap_gateways() {  # → "<dev> <gw>" for every default gateway of a protected interface (from the snapshot)
  local dev gw
  while read -r dev; do while read -r gw; do [[ -n $gw ]] && echo "$dev $gw"; done < "$WORK/mgmt/$dev.gateways"; done < "$WORK/mgmt.ifs"
}
neigh_state() { tmo "$VRX_IP" -j neigh show "$2" dev "$1" 2>/dev/null | jq -r '.[0].state[0]? // "NONE"' 2>/dev/null || echo NONE; }  # <dev> <gw>
probe_ok() {  # <probe> → 0 when the management path answers (prints why not)
  local p="$1" dev gw hp host port n=0 st end rdev
  case "$p" in
    neigh)  # the next hop answers ARP/ND: nudge (any packet), then REACHABLE within the command timeout
      while read -r dev gw; do
        n=$((n + 1))
        nudge "$dev" "$gw"
        st=NONE end=$((SECONDS + CMD_TIMEOUT))   # TD-6 V8: bounded by time, not by a count of possibly hanging `ip` calls
        while :; do
          st="$(neigh_state "$dev" "$gw")"
          [[ $st != REACHABLE && $SECONDS -lt $end ]] || break
          sleep 0.5
        done
        [[ $st == REACHABLE ]] || { echo "next hop $gw on $dev is not REACHABLE (neighbour state $st)"; return 1; }
      done < <(snap_gateways)
      ((n)) || { echo "no default gateway on a management interface (use --mgmt-probe tcp:HOST:PORT)"; return 1; } ;;
    gateway-ping)
      while read -r dev gw; do
        n=$((n + 1))
        tmo "$VRX_PING" -c 1 -W 2 -I "$dev" "$gw" >/dev/null 2>&1 || { echo "gateway $gw on $dev does not answer ICMP"; return 1; }
      done < <(snap_gateways)
      ((n)) || { echo "no default gateway to ping"; return 1; } ;;
    tcp:*)
      hp="${p#tcp:}" port="${hp##*:}" host="${hp%:*}"; host="${host#[}"; host="${host%]}"
      rdev="$(tmo "$VRX_IP" -j route get "$host" 2>/dev/null | jq -r '.[0]?.dev // empty' 2>/dev/null || true)"
      grep -qx -- "${rdev:-none}" "$WORK/mgmt.ifs" || { echo "TCP target $host is not routed through a management interface (via ${rdev:-nothing})"; return 1; }
      tcp_connect "$host" "$port" 2>/dev/null || { echo "TCP connect to $host port $port failed"; return 1; } ;;
    *) echo "no management reachability check"; return 1 ;;
  esac
}
resolve_probe() {  # VRX_MGMT_PROBE → a probe that passes NOW (the baseline), or fail (prints why)
  local why
  if [[ $VRX_MGMT_PROBE != auto ]]; then
    if why="$(probe_ok "$VRX_MGMT_PROBE")"; then echo "$VRX_MGMT_PROBE"; return 0; fi
    echo "--mgmt-probe $VRX_MGMT_PROBE does not pass now: $why" >&2; return 1
  fi
  if why="$(probe_ok neigh)"; then echo neigh; return 0; fi
  echo "no viable management reachability check on this host: $why — pass --mgmt-probe tcp:HOST:PORT (a TCP service reached through the management interface)" >&2
  return 1
}
session_signal() {  # extra signal only (never a verdict): is the manager's SSH session still there?
  local peer
  for peer in $VRX_MGMT_PEERS; do
    if [[ -n $(tmo "$VRX_SS" -Htn state established dst "$peer" 2>/dev/null) ]]; then echo "manager session with $peer: established"; return; fi
  done
  echo "manager session: ${VRX_MGMT_PEERS:+not established (informational only)}${VRX_MGMT_PEERS:-no peer known}"
}

render() {  # <out> — the generator validates against this host and keeps the live file's plugin switches
  "$STARTUPGEN_BIN" --current "$VRX_STARTUP_CONF" -o "$1" "${GEN_ARGS[@]}" "$DOC"
}

# ---------------------------------------------------------------- handover gate (D-012; review M2, re-review N4)
handover_flag() {  # stdin: doc text → pending|done (no/ambiguous flag → pending) — same rule as tools/lab
  local s; s="$(grep -oE '^`?handover: *(pending|done)' | head -1 | grep -oE 'pending|done' || true)"; echo "${s:-pending}"
}
handover_sources() {  # → "<source> <pending|done>" lines: the canonical copy always, the planner's tree, extras
  local rel="docs/lab/host-$VM.md" f canon; canon="$(canon_root)"
  if [[ -r $canon/$rel ]]; then echo "$canon/$rel $(handover_flag < "$canon/$rel")"
  else echo "$canon/$rel(absent) pending"; fi
  if [[ -z $VRX_TEST_ROOT && -n $GATE_REPO && $GATE_REPO != "$canon" && -r $GATE_REPO/$rel ]]; then echo "$GATE_REPO/$rel $(handover_flag < "$GATE_REPO/$rel")"; fi
  for f in $VRX_HANDOVER_EXTRA; do
    if [[ -r $f ]]; then echo "$f $(handover_flag < "$f")"; else echo "$f(absent) pending"; fi
  done
}
canon_git() { local c; c="$(canon_root)" && [[ -n $c ]] || return 1; git -c safe.directory="$c" -C "$c" "$@"; }
approval_ref() {  # APPROVAL → "PENDING file @blob + D-nnn for rendering <sha>" from main of the canonical repo, or fail (prints why)
  # re-review N4: the approval binds to THIS change — a D-row on main whose decision column has the PENDING id as its
  # subject (not a mention) and names the sha256 of the rendering (--expect-new-sha256); an approval already executed by
  # this tool for that rendering (a committed work dir whose gate record names both) is spent.
  local file="docs/decisions/$APPROVAL.md" blob ds re w
  [[ $EXPECT_NEW =~ ^[0-9a-f]{64}$ ]] || { echo "no rendering sha256 to bind the approval to"; return 1; }
  blob="$(canon_git rev-parse --verify -q "main:$file" 2>/dev/null)" || { echo "$file does not exist on main"; return 1; }
  re="$(ere_escape "$APPROVAL")"
  # the D-row's decision column (3rd) must start with the PENDING id: it is the row's subject, not a mention
  ds="$(canon_git show main:docs/decisions/LOG.md 2>/dev/null | awk -F'|' -v re="^[[:space:]]*\\\\**$re([^A-Za-z0-9_-]|$)" \
    '$3 ~ /^[[:space:]]*D-[0-9]+[[:space:]]*$/ && $4 ~ re {id = $3; gsub(/[[:space:]]/, "", id); print id "\t" $0}' || true)"
  [[ -n $ds ]] || { echo "no D-row on main has $APPROVAL as the subject of its decision"; return 1; }
  ds="$(grep -F -- "$EXPECT_NEW" <<<"$ds" | cut -f1 | sort -u | tr '\n' ' ' || true)"
  [[ -n $ds ]] || { echo "no D-row on main answering $APPROVAL names this rendering (sha256 $EXPECT_NEW) — the approval does not cover this change"; return 1; }
  for w in "$VRX_APPLY_STATE"/*/; do
    [[ -e ${w}committed && -r ${w}gate ]] || continue
    if grep -qF -- "APPROVAL $APPROVAL (" "${w}gate" && grep -qF -- "rendering $EXPECT_NEW" "${w}gate"; then
      echo "$APPROVAL was already executed for this rendering (${w%/} committed) — a spent approval cannot be replayed"; return 1
    fi
  done
  echo "$file@${blob:0:12} + LOG ${ds% } for rendering $EXPECT_NEW"
}
gate() {  # → the gate record (deterministic: the run recomputes and compares it); returns 3 when --apply is not allowed
  local st="done" src s summary="" ref
  while read -r src s; do summary+="${summary:+ · }$src=$s"; [[ $s == "done" ]] || st=pending; done < <(handover_sources)
  [[ -n $summary ]] || { st=pending; summary="no handover source could be read"; }   # never "done" by default
  if [[ $st == "done" ]]; then echo "gate: handover done ($summary)"; return 0; fi
  if [[ -n $APPROVAL ]]; then
    if ref="$(approval_ref)"; then echo "gate: handover pending ($summary) — PRODUCT-OWNER APPROVAL $APPROVAL ($ref)"; return 0; fi
    echo "REFUSED: --i-have-product-owner-approval $APPROVAL: $ref"; return 3
  fi
  echo "REFUSED: docs/lab/host-$VM.md says handover: pending ($summary) — a VPP restart needs handover: done or --i-have-product-owner-approval PENDING-<slug> (D-012)"
  return 3
}
plan_seal() {  # sha256 over everything the planner decided (settings, document, generator args, binaries, gate record)
  cat "$WORK/settings" "$WORK/doc.json" "$WORK/gen-args" "$WORK/bin/vrx-startupgen" "$WORK/bin/vrx-vppcheck" \
    "$WORK/bin/apply-startup.sh" "$WORK/gate" | sha256sum | awk '{print $1}'
}

# ---------------------------------------------------------------- dry run
dry_run() {
  [[ -f $VRX_STARTUP_CONF ]] || die "$VRX_STARTUP_CONF not found"
  local tmp rc=0 d p probe; tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand now: the temp dir of this run
  trap "rm -rf '$tmp'" EXIT
  render "$tmp/new.conf" || die "rendering failed (nothing changed)"
  echo "== unified diff: $VRX_STARTUP_CONF → rendered"
  "$STARTUPGEN_BIN" --current "$VRX_STARTUP_CONF" --diff "$VRX_STARTUP_CONF" "${GEN_ARGS[@]}" "$DOC" 2>/dev/null || true
  echo "== semantic diff (comments, order, indentation ignored)"
  "$STARTUPGEN_BIN" --current "$VRX_STARTUP_CONF" --diff "$VRX_STARTUP_CONF" --semantic "${GEN_ARGS[@]}" "$DOC" 2>/dev/null || true
  echo "== PCI devices involved (driver now)"
  { conf_pcis "$VRX_STARTUP_CONF"; conf_pcis "$tmp/new.conf"; } | sort -u | while read -r p; do echo "  $p $(pci_driver "$p")"; done
  echo "== management interfaces protected (manager peer(s): ${VRX_MGMT_PEERS:-none}; network manager; exact restore plan)"
  WORK="$tmp"
  mgmt_ifs > "$tmp/mgmt.ifs" 2>/dev/null || true
  while read -r d; do
    if snapshot_mgmt "$d"; then
      p="$(if_pci "$d")"
      echo "  $d pci=${p:--} driver=$(pci_driver "${p:-none}") manager=$(cat "$tmp/mgmt/$d.netmgr") addrs=$(tr '\n' ' ' < "$tmp/mgmt/$d.addrs")"
      sed 's/^/    ip /' "$tmp/mgmt/$d.restore"
    else echo "  $d: SNAPSHOT FAILED — --apply will refuse"; rc=3; fi
  done < "$tmp/mgmt.ifs"
  [[ -s $tmp/mgmt.ifs ]] || { echo "  none found — --apply will refuse (use --mgmt-if)"; rc=3; }
  echo "== management reachability check (--mgmt-probe $VRX_MGMT_PROBE)"
  if probe="$(resolve_probe 2>"$tmp/probe.err")"; then echo "  will use: $probe (passes now); $(session_signal)"
  else echo "  NONE VIABLE — --apply will refuse: $(cat "$tmp/probe.err")"; rc=3; fi
  echo "== VPP preflight (read-only: vrx-vppcheck ifaces local0, bootid)"
  if ! vppcheck ifaces local0 2>&1 | sed 's/^/  /'; then echo "  VPP preflight FAILED — --apply will refuse"; rc=3; fi
  vppcheck bootid 2>&1 | sed 's/^/  boot identity: /' || true
  echo "== handover gate (--apply only; an approval must name the rendering's sha256 below)"
  [[ -n $EXPECT_NEW ]] || EXPECT_NEW="$(sha "$tmp/new.conf")"
  if ! gate | sed 's/^/  /'; then rc=3; fi   # TD-6 V9: exit 3 = --apply would refuse, the gate included
  command -v "$VRX_SYSTEMD_RUN" >/dev/null 2>&1 || { echo "  systemd-run not found — --apply will refuse"; rc=3; }
  p="$(unfinished_applies)"
  [[ -z $p ]] || { echo "  an earlier apply has not finished — --apply will refuse: $(tr '\n' ' ' <<<"$p")— finish it: $(finish_hint "$p")"; rc=3; }
  echo "== sha256 of $VRX_STARTUP_CONF (--expect-sha256)"
  sha "$VRX_STARTUP_CONF"
  echo "== sha256 of the rendering (--expect-new-sha256)"
  sha "$tmp/new.conf"
  echo "dry run: nothing changed"
  return "$rc"
}

# ---------------------------------------------------------------- locks (review M1: a separate holder unit keeps them)
lock_holders() { tmo "$VRX_LSLOCKS" -o PID,COMMAND,MODE,PATH 2>/dev/null | grep -F -e "$VRX_VPP_LOCK" -e "$VRX_LAB_LOCK" | tr -s ' ' | tr '\n' ';' || true; }
proc_is() {  # <pid> <needle> — pid is alive and its command line contains needle (never kill a recycled PID)
  [[ $1 =~ ^[0-9]+$ ]] && tr '\0' ' ' 2>/dev/null < "/proc/$1/cmdline" | grep -qF -- "$2"
}
holder_alive() { local p; p="$(cat "$WORK/locks-held" 2>/dev/null || true)"; [[ -n $p ]] && proc_is "$p" "--stage hold --work $WORK"; }
start_holder() {  # → 0 when the holder unit owns both locks, 3 otherwise (no fallback: a holder must not be the run's child)
  local -a envs; mapfile -t envs < <(setenv_args)
  rm -f "$WORK/locks-held" "$WORK/locks-refused" "$WORK/release"
  if ! tmo "$VRX_SYSTEMD_RUN" --unit="$UNIT_PREFIX-lock-$(basename "$WORK")" --collect --quiet "${envs[@]}" \
      "$WORK/bin/apply-startup.sh" --stage hold --work "$WORK"; then
    say "systemd-run cannot start the lock holder unit"; return 3
  fi
  local end=$((SECONDS + LOCK_TIMEOUT + 10))
  while ((SECONDS < end)); do
    if [[ -s $WORK/locks-held ]]; then return 0; fi
    if [[ -e $WORK/locks-refused ]]; then say "locks busy for ${LOCK_TIMEOUT}s: $(cat "$WORK/locks-refused")"; return 3; fi
    sleep 0.2
  done
  touch "$WORK/release"; say "lock holder did not report within $((LOCK_TIMEOUT + 10))s"; return 3
}
release_locks() {
  local i p
  if ((OWN_LOCKS)); then exec 8>&- 9>&-; OWN_LOCKS=0; fi
  touch "$WORK/release" 2>/dev/null || true   # if even that fails (full disk), the holder is killed below
  p="$(cat "$WORK/locks-held" 2>/dev/null || true)"
  for ((i = 0; i < 50; i++)); do holder_alive || break; sleep 0.1; done
  if holder_alive; then kill -KILL "$p" 2>/dev/null || true; fi
  say "locks released"
}
stage_hold() {
  [[ -d $WORK ]] || die "--work $WORK missing"
  load_settings >/dev/null
  exec 8>"$VRX_VPP_LOCK" 9>"$VRX_LAB_LOCK"
  if ! flock -x -w "$LOCK_TIMEOUT" 8; then echo "$VRX_VPP_LOCK held by: $(lock_holders)" > "$WORK/locks-refused"; exit 3; fi
  if ! flock -x -w "$LOCK_TIMEOUT" 9; then echo "$VRX_LAB_LOCK held by: $(lock_holders)" > "$WORK/locks-refused"; exit 3; fi
  [[ ! -e $WORK/release ]] || exit 0
  echo "$$" > "$WORK/locks-held"
  # TD-6 V7: HOLD_MAX here comes from the default counts (the run has not snapshotted the host yet). Once the run has, it
  # writes hold-until (epoch seconds: its own dead-man deadline + lock wait + one rollback + margin); from then on that is the end.
  local end=$((EPOCHSECONDS + HOLD_MAX)) h why="maximum lifetime ${HOLD_MAX}s reached"
  while [[ ! -e $WORK/release ]]; do
    h="$(cat "$WORK/hold-until" 2>/dev/null || true)"
    if [[ $h =~ ^[0-9]{1,12}$ ]]; then end=$h why="hold-until reached"; fi
    ((EPOCHSECONDS < end)) || break
    sleep 1 8>&- 9>&- || true   # a killed sleep never ends the hold
  done
  if [[ ! -e $WORK/release ]]; then echo "apply-startup: lock holder: $why, releasing" >> "$WORK/log"; fi
}
secure_locks() {  # <who> — before VPP is touched: the holder's locks, or both locks taken here (exclusive, bounded); once
  # TD-6 V5: the run's own rollback after the holder died used to restart VPP without any lock (a queued `flock -s`
  # integration run got the lab lock first). Same rule as the dead-man: FORCED only when a foreign holder keeps them.
  ((LOCKS_SECURED)) && return 0
  LOCKS_SECURED=1
  if holder_alive; then say "$1: lock holder $(cat "$WORK/locks-held") still owns the locks"; return 0; fi
  exec 8>"$VRX_VPP_LOCK" 9>"$VRX_LAB_LOCK"
  OWN_LOCKS=1
  if flock -x -w "$DEADMAN_LOCK_TIMEOUT" 8 7>&- && flock -x -w "$DEADMAN_LOCK_TIMEOUT" 9 7>&-; then
    say "$1: lock holder gone — took the locks exclusively before touching VPP"
  else
    say "$1: FORCED — locks held by someone else for ${DEADMAN_LOCK_TIMEOUT}s (${VRX_LSLOCKS}: $(lock_holders)); rolling back without them (a lost management path outranks the lock)"
    syslog "$1 FORCED rollback without locks $WORK: $(lock_holders)"
  fi
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
mgmt_state_ok() {  # <if> — link UP and addresses + routes exactly as snapshotted (the restore plan, regenerated)
  local dev="$1" now="$WORK/mgmt/now" d
  tmo "$VRX_IP" -o link show dev "$dev" 2>/dev/null | grep -qE '[<,]UP[,>]' || { echo "management interface $dev is not up"; return 1; }
  rm -rf "$now"
  snapshot_mgmt "$dev" "$now" 2>/dev/null || true
  touch "$now/$dev.restore"
  d="$(comm -23 <(sort "$WORK/mgmt/$dev.restore") <(sort "$now/$dev.restore") | sed 's/ dev .*//' | tr '\n' ';')"
  [[ -z $d ]] || { echo "management interface $dev lost: $d"; return 1; }
}
check_mgmt() {  # every protected interface as snapshotted + same driver; then the reachability check (same as the baseline)
  local dev pci why
  while read -r dev; do
    mgmt_state_ok "$dev" || return 1
    pci="$(if_pci "$dev")"
    if [[ -n $pci ]]; then
      [[ $(pci_driver "$pci") == "$(awk -v p="$pci" '$1==p{print $2}' "$WORK/drivers")" ]] || { echo "management NIC $pci changed driver"; return 1; }
    fi
  done < "$WORK/mgmt.ifs"
  why="$(probe_ok "$(cat "$WORK/probe")")" || { echo "management path: $why"; return 1; }
}
vpp_identity_ok() {  # after the settle time: a NEW, complete VPP instance = the unit's MainPID, nothing restarted since
  local before now pid unit
  before="$(cat "$WORK/ident.before")"
  now="$(vppcheck bootid 2>/dev/null)" || { echo "VPP boot identity unreadable or incomplete"; return 1; }
  [[ $now != "$before" ]] || { echo "VPP did not restart (boot identity still $now)"; return 1; }
  pid="$(cut -d/ -f2 <<<"$now")"
  unit="$(unit_ident)"
  [[ $unit == "$(cat "$WORK/unit.restart")" ]] || { echo "vpp.service restarted during the settle time ($(cat "$WORK/unit.restart")→ $unit)"; return 1; }
  [[ $unit == *"MainPID=$pid "* ]] || { echo "the VPP answering (pid $pid) is not vpp.service's main process ($unit)"; return 1; }
  echo "$now" > "$WORK/ident.after"
}
check_health() {  # → prints the first problem, returns 1
  holder_alive || { echo "the lock holder is gone — the locks are not ours any more"; return 1; }
  tmo "$VRX_SYSTEMCTL" is-active --quiet vpp || { echo "vpp.service is not active"; return 1; }
  [[ $(unit_ident) == "$(cat "$WORK/unit.restart")" ]] || { echo "vpp.service restarted since the apply ($(cat "$WORK/unit.restart")→ $(unit_ident))"; return 1; }
  [[ -S $VRX_VPP_API_SOCKET ]] || { echo "no API socket $VRX_VPP_API_SOCKET"; return 1; }
  local id loaded p names out
  id="$(vppcheck bootid 2>/dev/null)" || { echo "VPP API does not answer within ${CMD_TIMEOUT}s (hung?) or identity incomplete"; return 1; }
  [[ $id == "$(cat "$WORK/ident.after")" ]] || { echo "VPP crashed and came back (boot identity $(cat "$WORK/ident.after") → $id)"; return 1; }
  echo x >> "$WORK/ident.reads"
  loaded="$(vppcheck plugins 2>/dev/null)" || { echo "VPP did not list its plugins within ${CMD_TIMEOUT}s (hung?)"; return 1; }
  for p in $(conf_plugins "$WORK/new.conf" enable); do grep -qx "$p" <<<"$loaded" || { echo "plugin $p enabled but not loaded"; return 1; }; done
  for p in $(conf_plugins "$WORK/new.conf" disable); do ! grep -qx "$p" <<<"$loaded" || { echo "plugin $p disabled but loaded"; return 1; }; done
  while read -r p; do
    [[ -n $p ]] || continue
    grep -qx "$p" <<<"$(conf_plugins "$WORK/new.conf" disable)" && continue
    # re-review N6 / D-084: a plugin the old file enabled and the new file no longer mentions may be
    # default-disabled in VPP (linux_cp, linux_nl, npt66): its absence is the intended result
    if grep -qx "$p" <<<"$(conf_plugins "$WORK/backup.conf" enable)" && ! grep -qE "^[[:space:]]*plugin[[:space:]]+$(ere_escape "$p")[[:space:]]*\\{" "$WORK/new.conf"; then continue; fi
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
    if why="$(mgmt_state_ok "$dev")"; then say "management interface $dev: addresses and routes intact"; continue; fi
    say "$why — re-applying the recorded addresses and routes"
    apply_restore_plan "$dev"
    if mgmt_state_ok "$dev" >/dev/null; then continue; fi
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
  local u; u="$(cat "$WORK/deadman-unit" 2>/dev/null || true)"
  if [[ -n $u ]]; then tmo "$VRX_SYSTEMCTL" stop "$u.timer" >/dev/null 2>&1 || true; fi
}
# ---------------------------------------------------------------- files and ownership (TD-6 V1, V2)
# startup.conf is never written in place: a copy is staged next to it (same directory, so the rename is atomic and
# needs no space) and renamed over it. The rollback copy is staged BEFORE the new file goes in, so a disk that fills
# up during the window cannot stop the restore.
staged_path() { echo "$(dirname "$VRX_STARTUP_CONF")/.$(basename "$VRX_STARTUP_CONF").$1-$(basename "$WORK")"; }   # <new|rollback>
stage_file() {  # <src> <new|rollback> — a verified copy next to the live file; prints why not
  local dst err; dst="$(staged_path "$2")"
  if err="$(install -m 0644 "$1" "$dst" 2>&1)" && cmp -s "$1" "$dst"; then return 0; fi
  rm -f "$dst" 2>/dev/null || true
  echo "cannot write $dst: ${err:-incomplete copy}"; return 1
}
place_file() {  # <new|rollback> — rename the staged copy over the live file; prints why not
  local err; err="$(mv -f "$(staged_path "$1")" "$VRX_STARTUP_CONF" 2>&1)" && return 0
  echo "cannot rename $(staged_path "$1") over $VRX_STARTUP_CONF: ${err:-failed}"; return 1
}
mark() {  # <marker> <text> — a result marker; on a full disk at least the empty file (needs no data block)
  { printf '%s\n' "$2" > "$WORK/$1"; } 2>/dev/null || touch "$WORK/$1" 2>/dev/null || true
}
finished() { [[ -e $WORK/committed || -e $WORK/rolled-back || -e $WORK/console-needed || -e $WORK/superseded ]]; }
unfinished_applies() {  # → "<dir> (dead-man <unit>.timer)" for every OTHER work dir that installed a file and has no result
  local w
  for w in "$VRX_APPLY_STATE"/*/; do
    w="${w%/}"
    [[ $w != "$WORK" && -e $w/installed ]] || continue
    [[ -e $w/committed || -e $w/rolled-back || -e $w/console-needed || -e $w/superseded ]] && continue
    echo "$w (dead-man $(cat "$w/deadman-unit" 2>/dev/null || echo none).timer)"
  done
}
finish_hint() {  # <unfinished_applies output> → the safe way to finish each of them now (TD-7 F1)
  # the dead-man's own unit: systemd starts a unit only once, so it can never run next to the timer's own start. Running
  # the rollback stage by hand is only the fallback when no unit was recorded (it refuses a second rollback of the apply).
  local line w u out=""
  while IFS= read -r line; do
    [[ -n $line ]] || continue
    w="${line%% (dead-man *}"
    u="$(cat "$w/deadman-unit" 2>/dev/null || true)"
    if [[ -n $u ]]; then out+="${out:+; }systemctl start $u.service"
    else out+="${out:+; }$w/bin/apply-startup.sh --stage rollback --work $w"; fi
  done <<<"$1"
  echo "$out"
}
claim_current() {  # under the locks, just before the new file goes in: this apply owns startup.conf now (V1)
  local w u
  printf '%s\n' "$WORK" > "$VRX_APPLY_STATE/.current.tmp" && mv -f "$VRX_APPLY_STATE/.current.tmp" "$VRX_APPLY_STATE/current" || return 1
  # disarm on supersede: an older apply that never installed (it cannot have: unfinished installs are refused) must not
  # keep an armed dead-man around
  for w in "$VRX_APPLY_STATE"/*/; do
    w="${w%/}"
    [[ $w != "$WORK" && -s $w/deadman-unit ]] || continue
    [[ -e $w/committed || -e $w/rolled-back || -e $w/console-needed || -e $w/superseded ]] && continue
    u="$(cat "$w/deadman-unit")"
    tmo "$VRX_SYSTEMCTL" stop "$u.timer" >/dev/null 2>&1 || true
    { printf 'superseded by %s before it installed anything\n' "$WORK" > "$w/superseded"; } 2>/dev/null || true
    say "disarmed the dead-man of the older apply $w ($u)"
  done
}
not_ours() {  # → 0 + why when startup.conf no longer belongs to this apply (V1): never roll back over someone else's change
  local cur live
  cur="$(cat "$VRX_APPLY_STATE/current" 2>/dev/null || true)"
  if [[ -n $cur && $cur != "$WORK" ]]; then echo "newer:a newer apply ($cur) installed $VRX_STARTUP_CONF after this one"; return 0; fi
  [[ -e $VRX_STARTUP_CONF ]] || return 1   # a missing file is ours to restore
  live="$(sha "$VRX_STARTUP_CONF")"
  if [[ -s $WORK/new.conf && $live != "$(sha "$WORK/new.conf")" && $live != "$(sha "$WORK/backup.conf")" ]]; then
    echo "foreign:$VRX_STARTUP_CONF (sha256 $live) is neither this apply's file nor its backup — changed by someone else"; return 0
  fi
  return 1
}

# ---------------------------------------------------------------- rollback
rollback() {  # the ONE rollback of this apply: restore, restart VPP once, verify like the snapshot; then always finish
  RB_MINE=1
  touch "$WORK/rollback-started" 2>/dev/null || true
  say "ROLLBACK: $1"
  syslog "ROLLBACK $WORK: $1"
  secure_locks rollback
  local why
  if why="$(not_ours)"; then
    if [[ $why == newer:* ]]; then
      mark superseded "${why#newer:}"
      say "SUPERSEDED: ${why#newer:} — nothing restored, VPP not touched"
    else
      mark console-needed "${why#foreign:}; not restored, VPP not touched"
      say "CONSOLE NEEDED: ${why#foreign:}; not restored, VPP not touched"
    fi
    syslog "rollback of $WORK skipped: ${why#*:}"
  else
    rollback_steps || true   # errexit is off in there: every step runs, whatever failed before it (V2)
  fi
  rm -f "$(staged_path new)" "$(staged_path rollback)" 2>/dev/null || true
  finished || mark console-needed "the rollback ended without a result (see $WORK/log)"
  cancel_deadman
  release_locks
}
rollback_steps() {
  local why="" err="" staged=0
  # V2: the old file must be next to the live one BEFORE VPP is stopped. Usually the run staged it before installing;
  # otherwise stage it now. If that is impossible VPP is not stopped: restarting it would only bring back the new file.
  if cmp -s "$WORK/backup.conf" "$(staged_path rollback)" || err="$(stage_file "$WORK/backup.conf" rollback)"; then staged=1; fi
  if ((!staged)); then
    if tmo "$VRX_SYSTEMCTL" is-active --quiet vpp; then why="VPP left running on the new file (not stopped)"
    else
      svc reset-failed vpp >/dev/null 2>&1; svc start vpp || true
      why="VPP was not running; started on the new file"
    fi
    mark console-needed "cannot restore $VRX_STARTUP_CONF: $err — $why; the old file is $WORK/backup.conf"
    say "ROLLBACK IMPOSSIBLE — CONSOLE NEEDED: $(cat "$WORK/console-needed" 2>/dev/null)"
    syslog "ROLLBACK IMPOSSIBLE $WORK: $err — console needed"
    return 0
  fi
  if ! svc stop vpp; then
    say "systemctl stop vpp failed or hung for ${SVC_TIMEOUT}s — SIGKILL"
    svc kill --signal=KILL vpp || true
  fi
  if err="$(place_file rollback)"; then err=""
  elif install -m 0644 "$WORK/backup.conf" "$VRX_STARTUP_CONF" 2>/dev/null && cmp -s "$WORK/backup.conf" "$VRX_STARTUP_CONF"; then
    say "($err — copied in place instead)"; err=""
  fi
  if [[ -z $err ]]; then
    say "restored $VRX_STARTUP_CONF from $WORK/backup.conf"
    rebind_drivers
    restore_mgmt
  fi
  svc reset-failed vpp >/dev/null 2>&1 || true   # re-review N11: never stuck in start-limit-hit
  svc start vpp || say "systemctl start vpp failed"
  if [[ -n $err ]]; then   # VPP was started again anyway: on the new file, but not left stopped
    mark console-needed "$VRX_STARTUP_CONF NOT restored ($err); VPP was started again on the new file; the old file is $WORK/backup.conf"
    say "ROLLBACK INCOMPLETE — CONSOLE NEEDED: $(cat "$WORK/console-needed" 2>/dev/null)"
    syslog "ROLLBACK INCOMPLETE $WORK: startup.conf not restored — console needed"
    return 0
  fi
  if ! wait_api; then why="VPP API did not come up"
  elif ! tmo "$VRX_SYSTEMCTL" is-active --quiet vpp; then why="vpp.service not active"
  else why="$(check_mgmt)" || true; fi
  if [[ -z $why ]]; then
    mark rolled-back "rolled back to $WORK/backup.conf"
    say "rolled back to $WORK/backup.conf; VPP and the management path are healthy ($(session_signal))"
    syslog "rolled back $WORK"
  else
    mark console-needed "$why"
    say "ROLLBACK INCOMPLETE ($why) — CONSOLE NEEDED: the file is restored, VPP is not restarted again; see $WORK/console-needed"
    syslog "ROLLBACK INCOMPLETE $WORK: $why — console needed"
  fi
}

# ---------------------------------------------------------------- budgets (M3, re-review N6: worst cases)
budgets() {
  local ndrv=7 nif=1 nplan=4 ngw=1 c=$((CMD_TIMEOUT + 2)) s=$((SVC_TIMEOUT + 5))
  if [[ -n $WORK && -s $WORK/drivers ]]; then ndrv=$(wc -l < "$WORK/drivers"); fi
  if [[ -n $WORK && -s $WORK/mgmt.ifs ]]; then
    nif=$(wc -l < "$WORK/mgmt.ifs"); nplan=$(cat "$WORK"/mgmt/*.restore 2>/dev/null | wc -l); ngw=$(cat "$WORK"/mgmt/*.gateways 2>/dev/null | wc -l)
  fi
  # one health round: holder, unit x3, bootid, plugins, ifaces, per interface link + 3 snapshot reads, per gateway nudge + neighbour polls
  ITER=$(((6 + 4 * nif) * c + ngw * (3 * c) + INTERVAL))
  # stop (+kill) + reset-failed + start, driver rebind (3 writes + driverctl), per interface netdev wait + link + checks +
  # the plan twice + a manager re-apply, then API wait + one health round
  # (+ both locks when the holder is gone: TD-6 V5)
  RB_BUDGET=$((2 * DEADMAN_LOCK_TIMEOUT + 4 * s + ndrv * 4 * c + nif * (10 + 8 * c) + 2 * nplan * c + API_WAIT + ITER + 30))
  # restart, the unit tuple (3 tries, TD-6 V3), API wait, settle, window
  RUN_BUDGET=$((s + 6 * c + 2 + API_WAIT + SETTLE + WINDOW + ITER))
  DEADMAN_AFTER=$((RUN_BUDGET + ITER + RB_BUDGET + 60))
  HOLD_MAX=$((DEADMAN_AFTER + 2 * DEADMAN_LOCK_TIMEOUT + RB_BUDGET + 600))
}

# ---------------------------------------------------------------- stages
write_settings() {
  local v
  for v in "${SETTINGS_VARS[@]}"; do
    [[ ${!v} != *$'\n'* ]] || die "setting $v contains a newline"
    printf '%s=%s\n' "$v" "${!v}"
  done > "$WORK/settings"
}
load_settings() {  # the work dir is authoritative; a differing environment is reported
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
  budgets
}
setenv_args() {  # → --setenv=NAME=value per setting: the unit never depends on systemd's own environment
  local v
  for v in "${SETTINGS_VARS[@]}"; do if [[ $v == VRX_* ]]; then printf -- '--setenv=%s=%s\n' "$v" "${!v}"; fi; done
  printf -- '--setenv=PATH=%s\n' "$PATH"
}

detach() {  # the planner: pin and seal everything into the work dir, then start "--stage run" as its own unit
  local stamp; stamp="$(date +%Y%m%d-%H%M%S)-$$"
  WORK="$VRX_APPLY_STATE/$stamp"
  install -d -m 0750 "$WORK" "$WORK/bin"
  install -m 0640 "$DOC" "$WORK/doc.json"
  install -m 0755 "$STARTUPGEN_BIN" "$WORK/bin/vrx-startupgen"
  install -m 0755 "$VPPCHECK_BIN" "$WORK/bin/vrx-vppcheck"
  install -m 0755 "$SELF" "$WORK/bin/apply-startup.sh"
  printf '%s\n' "${GEN_ARGS[@]}" > "$WORK/gen-args"
  write_settings
  echo "$GATE" > "$WORK/gate"
  plan_seal > "$WORK/plan.sha256"
  say "plan sealed: $(cat "$WORK/plan.sha256") (operator ${SUDO_USER:-${USER:-uid $(id -u)}})"
  local cmd=("$WORK/bin/apply-startup.sh" --stage run --work "$WORK")
  if ((FOREGROUND)); then "${cmd[@]}"; return; fi
  local -a envs; mapfile -t envs < <(setenv_args)
  if ! "$VRX_SYSTEMD_RUN" --unit="$UNIT_PREFIX-$stamp" --collect --quiet --property=KillMode=process "${envs[@]}" "${cmd[@]}"; then
    say "REFUSED: systemd-run cannot start the apply unit (there is no fallback; nothing was changed)"; exit 3
  fi
  echo "$UNIT_PREFIX-$stamp" > "$WORK/run-unit"
  say "started detached unit $UNIT_PREFIX-$stamp — follow: journalctl -fu $UNIT_PREFIX-$stamp  (log: $WORK/log)"
}

log_to_work() { exec > >(tee -a "$WORK/log") 2>&1; }
refuse() { say "REFUSED: $*"; rm -f "$(staged_path new)" "$(staged_path rollback)" 2>/dev/null || true; RUN_LOCKED=0; release_locks; }
run_exit() {  # EXIT trap of the run (set -e, SIGTERM from `systemctl stop <run unit>`): never leave the host half-way (TD-6 V3)
  local rc=$?
  trap - EXIT
  ((RUN_LOCKED)) || return "$rc"
  if finished || ((RB_MINE)) || [[ -e $WORK/deadman-fired ]]; then return "$rc"; fi   # done, or the dead-man's job now
  if ((RUN_INSTALLED)); then rollback "the run stopped unexpectedly (exit $rc) after installing" || true
  else
    say "the run stopped unexpectedly (exit $rc) before installing — nothing changed"
    rm -f "$(staged_path new)" "$(staged_path rollback)" 2>/dev/null || true
    cancel_deadman; release_locks
  fi
  return "$rc"
}

stage_run() {
  [[ -d $WORK ]] || die "--work $WORK missing"
  log_to_work
  load_settings
  echo "$$" > "$WORK/run-pid"
  [[ -s $WORK/plan.sha256 && $(plan_seal) == "$(cat "$WORK/plan.sha256")" ]] || { say "REFUSED: plan seal mismatch (work dir altered after planning)"; return 3; }
  local g; g="$(gate)" || { say "REFUSED: gate no longer holds: $g"; return 3; }
  [[ $g == "$(cat "$WORK/gate")" ]] || { say "REFUSED: gate changed since planning: now '$g'"; return 3; }
  [[ ! -e $WORK/installed && ! -e $WORK/rollback-started ]] || { say "REFUSED: this work dir was already used (installed before) — plan again"; return 3; }
  say "$g"
  syslog "apply $WORK started; $g"
  trap run_exit EXIT
  start_holder || { refuse "locks not held"; return 3; }
  RUN_LOCKED=1
  say "locks held by holder pid $(cat "$WORK/locks-held"): $VRX_VPP_LOCK, $VRX_LAB_LOCK"
  local busy; busy="$(unfinished_applies)"   # TD-6 V1, checked again under the locks
  [[ -z $busy ]] || { refuse "an earlier apply has not finished: $(tr '\n' ' ' <<<"$busy")— its dead-man may still roll back; wait for it, or run that one rollback now through the dead-man's own unit: $(finish_hint "$busy") (systemd starts a unit only once; a second rollback of the same apply is refused)"; return 3; }
  local now; now="$(sha "$VRX_STARTUP_CONF")"
  [[ $now == "$EXPECT" ]] || { refuse "$VRX_STARTUP_CONF changed since the review (sha256 $now, expected $EXPECT) — dry-run again"; return 3; }
  render "$WORK/new.conf" || { refuse "rendering failed"; return 3; }
  now="$(sha "$WORK/new.conf")"
  [[ $now == "$EXPECT_NEW" ]] || { refuse "the rendering differs from the reviewed one (sha256 $now, expected $EXPECT_NEW) — document, generator or host facts changed; dry-run again"; return 3; }
  local pre; pre="$(vppcheck ifaces local0 2>&1)" || { refuse "VPP preflight failed (vrx-vppcheck ifaces local0): $pre"; return 3; }
  say "preflight: $pre"
  if cmp -s "$VRX_STARTUP_CONF" "$WORK/new.conf"; then say "nothing to do: the rendering equals $VRX_STARTUP_CONF"; mark committed "nothing to do"; release_locks; return 0; fi

  mgmt_ifs > "$WORK/mgmt.ifs" || true
  [[ -s $WORK/mgmt.ifs ]] || { refuse "no management interface found (no default route, no --mgmt-if/--mgmt-peer)"; return 3; }
  local dev p probe
  while read -r dev; do snapshot_mgmt "$dev" || { refuse "cannot snapshot management interface $dev"; return 3; }; done < "$WORK/mgmt.ifs"
  probe="$(resolve_probe 2>&1)" || { refuse "$probe"; return 3; }
  echo "$probe" > "$WORK/probe"
  { conf_pcis "$VRX_STARTUP_CONF"; conf_pcis "$WORK/new.conf"; while read -r dev; do if_pci "$dev"; done < "$WORK/mgmt.ifs"; } \
    | sed '/^$/d' | sort -u | while read -r p; do echo "$p $(pci_driver "$p")"; done > "$WORK/drivers"
  vppcheck plugins > "$WORK/plugins.before" || { refuse "VPP did not list its plugins"; return 3; }
  vppcheck bootid > "$WORK/ident.before" || { refuse "VPP boot identity unreadable or incomplete"; return 3; }
  budgets
  # TD-6 V7: the holder started with the default counts; from now on it holds until this host's worst case
  echo $((EPOCHSECONDS + HOLD_MAX)) > "$WORK/hold-until" || { refuse "cannot write $WORK/hold-until"; return 3; }
  local err
  if ! err="$(cp -p "$VRX_STARTUP_CONF" "$WORK/backup.conf" 2>&1)" || ! cmp -s "$VRX_STARTUP_CONF" "$WORK/backup.conf"; then
    refuse "cannot write the backup $WORK/backup.conf: ${err:-incomplete copy}"; return 3
  fi
  cp -p "$VRX_STARTUP_CONF" "$VRX_STARTUP_CONF.bak-$(basename "$WORK")" 2>/dev/null || say "WARNING: no second backup next to $VRX_STARTUP_CONF (the work dir copy is kept)"
  # TD-6 V2: both files are staged next to the live one before anything changes — the install and the restore are renames
  err="$(stage_file "$WORK/new.conf" new)" || { refuse "$err (nothing was changed)"; return 3; }
  err="$(stage_file "$WORK/backup.conf" rollback)" || { refuse "$err (nothing was changed)"; return 3; }
  say "backup: $WORK/backup.conf and $VRX_STARTUP_CONF.bak-$(basename "$WORK"); staged $(staged_path new) and $(staged_path rollback)"
  say "unified diff:"; "$STARTUPGEN_BIN" --current "$VRX_STARTUP_CONF" --diff "$VRX_STARTUP_CONF" "${GEN_ARGS[@]}" "$DOC" 2>/dev/null || true
  say "semantic diff:"; "$STARTUPGEN_BIN" --current "$VRX_STARTUP_CONF" --diff "$VRX_STARTUP_CONF" --semantic "${GEN_ARGS[@]}" "$DOC" 2>/dev/null || true
  say "recorded drivers: $(tr '\n' ';' < "$WORK/drivers") VPP identity: $(cat "$WORK/ident.before") reachability check: $probe; $(session_signal)"
  while read -r dev; do
    say "management $dev ($(cat "$WORK/mgmt/$dev.netmgr")): $(tr '\n' ' ' < "$WORK/mgmt/$dev.addrs")gw $(tr '\n' ' ' < "$WORK/mgmt/$dev.gateways")"
  done < "$WORK/mgmt.ifs"

  local run_end=$((SECONDS + RUN_BUDGET)) deadman
  deadman="$UNIT_PREFIX-deadman-$(basename "$WORK")"
  local -a envs; mapfile -t envs < <(setenv_args)
  if ! tmo "$VRX_SYSTEMD_RUN" --unit="$deadman" --on-active="$DEADMAN_AFTER" --timer-property=AccuracySec=1s --collect --quiet "${envs[@]}" \
      "$WORK/bin/apply-startup.sh" --stage rollback --work "$WORK"; then
    refuse "systemd-run cannot arm the dead-man timer (there is no fallback; nothing was changed)"; return 3
  fi
  echo "$deadman" > "$WORK/deadman-unit"
  say "dead-man armed: rollback in ${DEADMAN_AFTER}s unless committed (run budget ${RUN_BUDGET}s, rollback budget ${RB_BUDGET}s)"

  holder_alive || { cancel_deadman; refuse "the lock holder is gone"; return 3; }
  touch "$WORK/installed" || { cancel_deadman; refuse "cannot write $WORK/installed"; return 3; }
  RUN_INSTALLED=1
  claim_current || { rollback "cannot record this apply as the current one in $VRX_APPLY_STATE/current"; return 1; }
  if ! err="$(place_file new)"; then
    if cmp -s "$VRX_STARTUP_CONF" "$WORK/backup.conf"; then   # a failed rename changed nothing: no restart needed
      mark rolled-back "nothing installed: $err"; say "REFUSED: $err — nothing was installed, VPP not restarted"
      rm -f "$(staged_path new)" "$(staged_path rollback)" 2>/dev/null || true
      cancel_deadman; release_locks; return 1
    fi
    rollback "cannot install the new file: $err"; return 1
  fi
  say "installed $VRX_STARTUP_CONF; restarting VPP"
  local why=""
  if ! svc restart vpp; then why="systemctl restart vpp failed or hung for ${SVC_TIMEOUT}s"
  elif why="$(read_unit_restart)"; then   # right after the restart job: complete, NRestarts 0, the new MainPID (V3, V4)
    why=""
    if ! wait_api; then why="VPP API did not come up within ${API_WAIT}s (hung or crashed)"
    else
      sleep "$SETTLE"
      if why="$(vpp_identity_ok)"; then
        why=""
        say "VPP restarted: identity $(cat "$WORK/ident.after"); unit $(cat "$WORK/unit.restart")"
        local end=$((SECONDS + WINDOW)) reads=0
        while :; do
          if ! why="$(check_health)"; then break; fi
          why=""; reads=$((reads + 1))
          ((SECONDS < run_end)) || { why="the run exceeded its time budget (${RUN_BUDGET}s)"; break; }
          ((SECONDS < end || reads < 2)) || break
          sleep "$INTERVAL"
        done
      fi
    fi
  fi
  if [[ -n $why ]]; then rollback "$why"; return 1; fi
  [[ ! -e $WORK/deadman-fired ]] || { say "dead-man already fired — not committing"; return 1; }
  holder_alive || { rollback "the lock holder is gone before the commit"; return 1; }
  mark committed "healthy for ${WINDOW}s"
  cancel_deadman
  rm -f "$(staged_path rollback)" 2>/dev/null || true
  release_locks
  say "COMMITTED: healthy for ${WINDOW}s ($(wc -l < "$WORK/ident.reads") identity reads; $(session_signal)); dead-man cancelled. Record it in docs/decisions/LOG.md (backup $WORK/backup.conf)"
  syslog "COMMITTED $WORK"
}

descendants() {  # <pid> → every descendant pid (deepest first)
  local c
  for c in $(ps -o pid= --ppid "$1" 2>/dev/null); do descendants "$c"; echo "$c"; done
}
kill_run() {  # the run is stuck or dead: kill it and everything it started (the holder is a separate unit)
  local pid c i
  read -r pid < "$WORK/run-pid" 2>/dev/null || return 0
  proc_is "$pid" "--stage run --work $WORK" || return 0
  say "dead-man: killing the run (pid $pid) and its children"
  kill -STOP "$pid" 2>/dev/null || true
  for c in $(descendants "$pid"); do kill -KILL "$c" 2>/dev/null || true; done
  kill -KILL "$pid" 2>/dev/null || true
  for ((i = 0; i < 50; i++)); do
    if [[ ! -e /proc/$pid ]]; then break; fi
    sleep 0.1
  done
}

stage_rollback() {  # the dead-man (or its unit started by hand): at most one rollback, then always finish
  [[ -d $WORK ]] || die "--work $WORK missing"
  log_to_work
  load_settings
  # TD-7 F1: one rollback stage per apply at a time. The timer can fire while the operator finishes the same apply by
  # hand; two rollbacks would each stop and start VPP (a second outage, and one's health check lands in the other's
  # stop). The lock is the work dir itself (keyed by the apply id; nothing to create, so a full disk cannot stop the
  # dead-man), taken without waiting: whoever holds it does the one rollback. Opened after log_to_work, so the log's
  # tee never inherits it; every child that can outlive this process closes fd 7 (tmo, svc, vppcheck, …).
  exec 7<"$WORK"
  if ! flock -n 7; then
    say "a rollback of $WORK is already running (pid $(cat "$WORK/rollback-pid" 2>/dev/null || echo '?')) — nothing to do"
    syslog "second rollback of $WORK refused: one is already running"
    return 0
  fi
  { echo "$$" > "$WORK/rollback-pid"; } 2>/dev/null || true
  if finished; then say "dead-man: nothing to do (the apply already finished)"; return 0; fi
  cancel_deadman   # TD-7 F1: disarm the timer before a rollback started by hand; harmless in the timer's own service
  touch "$WORK/deadman-fired"
  say "dead-man fired: the run did not finish in time"
  # review M1: the locks must never go free between the run and the rollback (taken before the run is touched)
  secure_locks dead-man
  kill_run
  if finished; then say "dead-man: the run finished while being stopped — nothing to do"; release_locks; return 0; fi
  if [[ ! -e $WORK/installed ]]; then
    say "dead-man: nothing was installed — nothing to roll back"; mark rolled-back "nothing installed"
    rm -f "$(staged_path new)" "$(staged_path rollback)" 2>/dev/null || true
    release_locks; return 0
  fi
  rollback "dead-man: the apply did not finish within ${DEADMAN_AFTER}s"
  return 1
}

main() {
  parse_args "$@"
  budgets
  case "$STAGE" in
    run) stage_run ;;
    rollback) stage_rollback ;;
    hold) stage_hold ;;
    "")
      [[ -n $DOC && -f $DOC ]] || die "--doc <document.json> is required"
      [[ -x $STARTUPGEN_BIN ]] || die "$STARTUPGEN_BIN not found (cd apps/agent && go build -o bin/vrx-startupgen ./cmd/vrx-startupgen)"
      [[ -x $VPPCHECK_BIN ]] || die "$VPPCHECK_BIN not found (cd apps/agent && go build -o bin/vrx-vppcheck ./cmd/vrx-vppcheck)"
      command -v jq >/dev/null 2>&1 || die "jq is required (management snapshot)"
      canon_root >/dev/null
      if [[ -n ${SSH_CONNECTION:-} ]]; then
        VRX_MGMT_PEERS="${VRX_MGMT_PEERS:+$VRX_MGMT_PEERS }${SSH_CONNECTION%% *}"
        ((!FOREGROUND || CONSOLE)) || die "--foreground over SSH would die with the session; run detached (default) or pass --console on a real console"
      fi
      GATE_REPO="$(cd "$HERE/../.." 2>/dev/null && pwd || true)"
      if ((APPLY)); then
        [[ $EXPECT =~ ^[0-9a-f]{64}$ ]] || die "--apply needs --expect-sha256 <live sum printed by the dry run>"
        [[ $EXPECT_NEW =~ ^[0-9a-f]{64}$ ]] || die "--apply needs --expect-new-sha256 <rendered sum printed by the dry run>"
        [[ -f $VRX_STARTUP_CONF ]] || die "$VRX_STARTUP_CONF not found"
        command -v "$VRX_SYSTEMD_RUN" >/dev/null 2>&1 || { echo "apply-startup: REFUSED: systemd-run not found (there is no fallback)" >&2; exit 3; }
        if ! GATE="$(gate)"; then echo "apply-startup: $GATE" >&2; exit 3; fi
        # TD-6 V1: an apply that installed a file and has no result yet still has an armed dead-man that would roll back
        # over this one — finish it first (its dead-man, or that same one rollback now through the dead-man's unit: TD-7 F1)
        local busy; busy="$(unfinished_applies)"
        if [[ -n $busy ]]; then
          echo "apply-startup: REFUSED: an earlier apply has not finished: $(tr '\n' ' ' <<<"$busy")— wait for its dead-man, or run that one rollback now through the dead-man's own unit: $(finish_hint "$busy") (systemd starts a unit only once; a second rollback of the same apply is refused)" >&2
          exit 3
        fi
        say "$GATE"
        syslog "$GATE (operator ${SUDO_USER:-${USER:-uid $(id -u)}})"
        detach
      else
        dry_run
      fi ;;
    *) die "unknown stage '$STAGE'" ;;
  esac
}

if [[ ${BASH_SOURCE[0]} == "$0" ]]; then main "$@"; fi
