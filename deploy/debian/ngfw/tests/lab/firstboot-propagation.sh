#!/usr/bin/env bash
# Observe D-254 in a DISPOSABLE systemd instance: stand-in units named ngfwlab-* carry the
# shipped [Unit] dependency lines of ngfw-firstboot.service, the vpp.service and
# nftables.service drop-ins, ngfw-firewall-bootstrap.service, ngfw-agent.service and
# ngfw-api.service; PostgreSQL, Valkey, VPP, agent, API and nginx are sleep daemons, the
# firstboot stand-in runs the same per-unit is-active gate as firstboot.sh. The script
# starts, restarts, fails and stops those units and removes them again. Never point it at
# a manager whose units matter: the shared dev host runs VPP under the system manager.
#
# usage: firstboot-propagation.sh [--manager user|system] [--firstboot-unit FILE]
#                                 [--expect d254|previous] [--render-only DIR] [--keep]
#   --manager user      (default) the calling user's `systemctl --user` manager; units go
#                       to $XDG_RUNTIME_DIR/systemd/user and vanish at logout or reboot.
#   --manager system    /run/systemd/system of a disposable VM or container; refused
#                       unless NGFW_LAB_DISPOSABLE=1 is set.
#   --firstboot-unit F  the ngfw-firstboot.service text to model (default: the repository's);
#                       the pre-D-254 text is the positive control:
#                       git show origin/main:deploy/systemd/ngfw-firstboot.service > /tmp/previous
#   --expect d254       (default) exit 1 unless the observations match D-254;
#   --expect previous   exit 1 unless they match the pre-D-254 propagation (Requires=).
#   --render-only DIR   only write the stand-in units and the driver script into DIR.
#   --keep              leave the units installed afterwards.
set -euo pipefail
export LC_ALL=C
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../../../../.." && pwd)
UNITS=$ROOT/deploy/systemd
ASSETS=$ROOT/deploy/debian/ngfw/assets
MANAGER=user FIRSTBOOT_UNIT=$UNITS/ngfw-firstboot.service EXPECT=d254 RENDER_ONLY='' KEEP=''
while (($#)); do
  case $1 in
    --manager) MANAGER=$2; shift 2;;
    --firstboot-unit) FIRSTBOOT_UNIT=$(realpath -e -- "$2"); shift 2;;
    --expect) EXPECT=$2; shift 2;;
    --render-only) RENDER_ONLY=$2; shift 2;;
    --keep) KEEP=1; shift;;
    *) echo "unknown argument: $1" >&2; exit 2;;
  esac
done
[[ $MANAGER == user || $MANAGER == system ]] || { echo 'manager must be user or system' >&2; exit 2; }
[[ $EXPECT == d254 || $EXPECT == previous ]] || { echo 'expect must be d254 or previous' >&2; exit 2; }
if [[ $MANAGER == system && -z $RENDER_ONLY && ${NGFW_LAB_DISPOSABLE:-} != 1 ]]; then
  echo 'refusing the system manager without NGFW_LAB_DISPOSABLE=1 (disposable VM or container only)' >&2; exit 2
fi
# The real units the stand-ins are rendered from; every name below is rewritten to ngfwlab-<name>.
LAB_NAMES=(postgresql valkey-server nftables ngfw-firewall-bootstrap ngfw-firstboot vpp ngfw-agent ngfw-api nginx)
DAEMONS=(postgresql valkey-server vpp ngfw-agent ngfw-api nginx)
GATED=(postgresql valkey-server nftables)
ALL_UNITS=()
for name in "${LAB_NAMES[@]}"; do ALL_UNITS+=("ngfwlab-$name.service"); done
if [[ $MANAGER == user ]]; then
  SCOPE=--user; UNIT_DIR=${XDG_RUNTIME_DIR:?XDG_RUNTIME_DIR is required for the user manager}/systemd/user
  LAB=${XDG_RUNTIME_DIR}/ngfwlab
else
  SCOPE=--system; UNIT_DIR=/run/systemd/system; LAB=/run/ngfwlab
fi
if [[ -n $RENDER_ONLY ]]; then UNIT_DIR=$RENDER_ONLY/units; LAB=$RENDER_ONLY/lab; fi
LOG=$LAB/log

map_names() {
  # Rewrite the product unit names in a dependency value to their stand-ins.
  local text=$1 name
  for name in "${LAB_NAMES[@]}"; do text=${text//"$name.service"/"ngfwlab-$name.service"}; done
  printf '%s' "$text"
}

render_unit_section() {
  # The [Unit] lines of a shipped unit or drop-in that matter for propagation and ordering;
  # the marker condition is redirected into the lab directory, mount requirements dropped.
  local file=$1 section='' key value line
  while IFS= read -r line; do
    line=${line%%#*}; line=${line#"${line%%[![:space:]]*}"}; line=${line%"${line##*[![:space:]]}"}
    [[ -n $line ]] || continue
    if [[ $line == \[*\] ]]; then section=${line:1:${#line}-2}; continue; fi
    [[ $section == Unit ]] || continue
    key=${line%%=*}; value=${line#*=}
    case $key in
      Requires|Wants|After|Before|Requisite|BindsTo|PartOf) printf '%s=%s\n' "$key" "$(map_names "$value")";;
      ConditionPathExists) printf 'ConditionPathExists=%s\n' "${value//\/var\/lib\/ngfw\/firstboot-complete/$LAB/firstboot-complete}";;
      DefaultDependencies) printf '%s=%s\n' "$key" "$value";;
    esac
  done < "$file"
}

write_unit() {
  # write_unit <name> <description> <service body> [unit sources...]
  local name=$1 description=$2 body=$3 source; shift 3
  {
    printf '[Unit]\nDescription=ngfwlab stand-in: %s\n' "$description"
    for source in "$@"; do render_unit_section "$source"; done
    printf '\n[Service]\n%s\n' "$body"
  } > "$UNIT_DIR/ngfwlab-$name.service"
}

render() {
  mkdir -p "$UNIT_DIR" "$LAB"
  cat > "$LAB/stand-in" <<SH
#!/bin/sh
# ngfwlab stand-in process: \$1 = unit name, \$2 = daemon|oneshot. A file \$LAB/fail-<name>
# makes the start fail, as a daemon that cannot come up.
set -eu
LAB=$LAB
name=\$1; kind=\$2
echo "\$(date +%s.%N) start \$name" >> "\$LAB/log"
if [ -e "\$LAB/fail-\$name" ]; then echo "\$(date +%s.%N) fail \$name" >> "\$LAB/log"; exit 1; fi
[ "\$kind" = oneshot ] && exit 0
systemd-notify --ready
exec sleep infinity
SH
  cat > "$LAB/firstboot" <<SH
#!/bin/sh
# ngfwlab firstboot stand-in: the marker short-circuit and the per-unit is-active gate of
# deploy/debian/ngfw/assets/firstboot.sh (D-254), then a logged "run" instead of provisioning.
set -eu
LAB=$LAB
log() { echo "\$(date +%s.%N) \$*" >> "\$LAB/log"; }
if [ -e "\$LAB/firstboot-complete" ]; then log firstboot skip; exit 0; fi
for unit in ngfwlab-postgresql.service ngfwlab-valkey-server.service ngfwlab-nftables.service; do
  systemctl $SCOPE --quiet is-active "\$unit" || { log "firstboot refuse \$unit"; echo "first boot requires an active \$unit" >&2; exit 1; }
done
log firstboot run
: > "\$LAB/firstboot-complete"
SH
  chmod 0755 "$LAB/stand-in" "$LAB/firstboot"
  local name
  for name in "${DAEMONS[@]}"; do
    local sources=()
    case $name in
      vpp) sources=("$ASSETS/vpp-firstboot.conf");;
      ngfw-agent|ngfw-api) sources=("$UNITS/$name.service");;
    esac
    write_unit "$name" "$name daemon" "Type=notify
NotifyAccess=all
ExecStart=$LAB/stand-in $name daemon" "${sources[@]}"
  done
  # Debian's postgresql.service is itself a RemainAfterExit oneshot wrapper around the cluster units.
  write_unit postgresql "postgresql wrapper oneshot" "Type=oneshot
RemainAfterExit=yes
ExecStart=$LAB/stand-in postgresql oneshot"
  write_unit nftables "nftables ruleset oneshot" "Type=oneshot
RemainAfterExit=yes
ExecStart=$LAB/stand-in nftables oneshot" "$ASSETS/nftables-ngfw.conf"
  write_unit ngfw-firewall-bootstrap "early firewall oneshot" "Type=oneshot
RemainAfterExit=yes
ExecStart=$LAB/stand-in ngfw-firewall-bootstrap oneshot" "$UNITS/ngfw-firewall-bootstrap.service"
  write_unit ngfw-firstboot "firstboot oneshot with the D-254 gate" "Type=oneshot
RemainAfterExit=yes
ExecStart=$LAB/firstboot" "$FIRSTBOOT_UNIT"
  # What multi-user.target pulls in on an appliance: the enabled daemons and the enabled firstboot;
  # nftables and ngfw-firewall-bootstrap arrive through the dependencies only.
  printf '[Unit]\nDescription=ngfwlab boot\nWants=ngfwlab-postgresql.service ngfwlab-valkey-server.service ngfwlab-ngfw-firstboot.service ngfwlab-vpp.service ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service ngfwlab-nginx.service\n' > "$UNIT_DIR/ngfwlab-boot.target"
}

ctl() { systemctl "$SCOPE" --no-pager "$@"; }
invocations() { local unit; for unit in "${ALL_UNITS[@]}"; do printf '%s %s\n' "$unit" "$(ctl show -p InvocationID --value "$unit")"; done; }
changed_since() {
  # Units whose InvocationID differs from the snapshot in $1: restarted (or newly started).
  comm -13 <(sort "$1") <(invocations | sort) | awk '{print $1}' | sort | tr '\n' ' '
}
reset_lab() {
  ctl stop ngfwlab-boot.target "${ALL_UNITS[@]}" >/dev/null 2>&1 || true
  ctl reset-failed "${ALL_UNITS[@]}" >/dev/null 2>&1 || true
  rm -f "$LAB"/fail-* "$LOG"
}
log_order() { awk '{print $2 (NF>2 ? " " $3 : "")}' "$LOG" | tr '\n' ',' ; }
states() { local unit; for unit in "${ALL_UNITS[@]}"; do printf '%s=%s\n' "${unit#ngfwlab-}" "$(ctl is-active "$unit" || true)"; done; }
inactive_count() { states | grep -c -v -- '=active$' || true; }
FAILURES=0
expect() { # expect <label> <observed> <expected>
  if [[ $2 == "$3" ]]; then printf 'OK   %s: %s\n' "$1" "$2"; else printf 'FAIL %s: observed [%s] expected [%s]\n' "$1" "$2" "$3"; FAILURES=$((FAILURES + 1)); fi
}
firstboot_invocations() { grep -c 'firstboot \(run\|skip\|refuse\)' "$LOG" || true; }
restart_and_report() { # restart_and_report <name> <expected changed set for $EXPECT>
  local snapshot; snapshot=$(mktemp); invocations > "$snapshot"
  local runs_before invocations_before; runs_before=$(grep -c 'firstboot run' "$LOG" || true); invocations_before=$(firstboot_invocations)
  ctl restart "ngfwlab-$1.service"
  sleep 1
  local changed; changed=$(changed_since "$snapshot"); rm -f "$snapshot"
  expect "restart $1 -> restarted units" "$changed" "$2"
  # A propagated restart re-executes firstboot.sh, which exits 0 on the completion marker: an
  # invocation without a provisioning run.
  expect "restart $1 -> firstboot.sh invocations" "$(( $(firstboot_invocations) - invocations_before ))" "$([[ $2 == *ngfw-firstboot* ]] && echo 1 || echo 0)"
  expect "restart $1 -> firstboot.sh provisioning runs" "$(( $(grep -c 'firstboot run' "$LOG" || true) - runs_before ))" 0
}

render
if [[ -n $RENDER_ONLY ]]; then echo "rendered into $RENDER_ONLY"; exit 0; fi
trap 'if [[ -z $KEEP ]]; then reset_lab; rm -f "$UNIT_DIR"/ngfwlab-*; ctl daemon-reload; fi' EXIT
ctl daemon-reload
reset_lab; rm -f "$LAB/firstboot-complete"
echo "== fresh boot ($EXPECT, firstboot unit $FIRSTBOOT_UNIT)"
ctl start ngfwlab-boot.target
order=$(log_order)
echo "order: $order"
for name in postgresql valkey-server nftables; do
  expect "firstboot after $name" "$([[ ${order%%firstboot run*} == *"start $name"* ]] && echo yes || echo no)" yes
done
expect "firewall-bootstrap before nftables" "$([[ ${order%%start nftables*} == *"start ngfw-firewall-bootstrap"* ]] && echo yes || echo no)" yes
for name in vpp ngfw-agent ngfw-api nginx; do
  expect "$name after firstboot" "$([[ ${order#*firstboot run} == *"start $name"* ]] && echo yes || echo no)" yes
done
expect "vpp before agent before api" "$([[ ${order#*start vpp} == *"start ngfw-agent"*"start ngfw-api"* ]] && echo yes || echo no)" yes
expect "all active after the first boot" "$(inactive_count)" 0
echo "== explicit restarts on the freshly booted appliance"
if [[ $EXPECT == d254 ]]; then
  restart_and_report valkey-server 'ngfwlab-valkey-server.service '
  restart_and_report postgresql 'ngfwlab-postgresql.service '
  restart_and_report nftables 'ngfwlab-nftables.service ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service '
else
  restart_and_report valkey-server 'ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service ngfwlab-ngfw-firstboot.service ngfwlab-valkey-server.service ngfwlab-vpp.service '
  restart_and_report postgresql 'ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service ngfwlab-ngfw-firstboot.service ngfwlab-postgresql.service ngfwlab-vpp.service '
  restart_and_report nftables 'ngfwlab-nftables.service ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service ngfwlab-ngfw-firstboot.service ngfwlab-vpp.service '
fi
echo "== fail-closed first boot: one gated daemon cannot start"
for name in "${GATED[@]}"; do
  reset_lab; rm -f "$LAB/firstboot-complete"; : > "$LAB/fail-$name"
  ctl start ngfwlab-boot.target >/dev/null 2>&1 || true
  sleep 1
  expect "$name down -> firstboot not active" "$(ctl is-active ngfwlab-ngfw-firstboot.service || true)" "$([[ $EXPECT == d254 ]] && echo failed || echo inactive)"
  expect "$name down -> marker absent" "$([[ -e $LAB/firstboot-complete ]] && echo present || echo absent)" absent
  expect "$name down -> vpp, agent, api not started" "$(for unit in vpp ngfw-agent ngfw-api; do ctl is-active "ngfwlab-$unit.service" || true; done | tr '\n' ' ')" 'inactive inactive inactive '
  rm -f "$LAB/fail-$name"
  ctl reset-failed "${ALL_UNITS[@]}" >/dev/null 2>&1 || true
  ctl start "ngfwlab-$name.service" ngfwlab-ngfw-firstboot.service ngfwlab-boot.target
  expect "$name back -> boot completes" "$(inactive_count)" 0
done
echo "== provisioned appliance: reboot with the marker, then an explicit Valkey restart"
reset_lab
ctl start ngfwlab-boot.target
expect "provisioned boot skips the gate" "$(grep -c 'firstboot skip' "$LOG" || true)" 1
expect "provisioned boot runs no firstboot.sh" "$(grep -c 'firstboot run' "$LOG" || true)" 0
if [[ $EXPECT == d254 ]]; then
  restart_and_report valkey-server 'ngfwlab-valkey-server.service '
else
  restart_and_report valkey-server 'ngfwlab-ngfw-agent.service ngfwlab-ngfw-api.service ngfwlab-ngfw-firstboot.service ngfwlab-valkey-server.service ngfwlab-vpp.service '
fi
echo "== $FAILURES mismatch(es) against the $EXPECT expectation"
exit $(( FAILURES > 0 ))
