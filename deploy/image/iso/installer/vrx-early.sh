#!/usr/bin/env bash
# vrx-early.sh — autoinstall early-commands of the VRX ISO (P14). Runs in the live installer as root, before disks are
# probed; subiquity re-reads /autoinstall.yaml afterwards, so changes made here take effect.
#  1. Reinstall guard: if any disk already holds a VRX installation (a partition named/labelled vrx-rootA) and the
#     kernel command line lacks vrx.reinstall=1, print why and power off. This stops the loop "install → reboot →
#     the ISO boots again → wipes the fresh install" when the medium stays attached. The boot menu entry
#     "Reinstall VRX" sets vrx.reinstall=1.
#  2. Minimum disk size: the largest disk must hold the §7 layout (>= 96 GiB), else power off with the reason.
#  3. Boot mode: the storage config targets UEFI (grub_device on the ESP); in BIOS mode GRUB goes to the disk instead
#     (the 1 MiB bios_grub partition holds its core image).
#  4. --luks ISOs: a random per-install LUKS key in /run/vrx-luks.key (RAM only; vrx-late.sh moves it to the target).
set -euo pipefail
export LC_ALL=C
LOG=/var/log/vrx-early.log
exec > >(tee -a "$LOG") 2>&1
say() { echo "vrx-early: $*"; }
stop() { # print the reason on every console and power off (never wipes anything)
  local c
  for c in /dev/console /dev/tty1 /dev/ttyS0; do
    [[ -w $c ]] && printf '\n\n*** VRX installer stopped: %s ***\n*** The machine powers off in 60 s. ***\n\n' "$*" > "$c" 2>/dev/null || true
  done
  say "STOP: $*"; sleep 60; systemctl poweroff --force || poweroff -f
  exit 1
}

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
# shellcheck source=vrx-disk-guard.sh
. "$HERE/vrx-disk-guard.sh"
cmdline=$(cat /proc/cmdline)
if ! vrx_check_reinstall "$cmdline"; then
  stop "disk safety verification failed (inventory unavailable or prior VRX installation found); see /var/log/vrx-early.log"
fi

min=$((96 * 1024 * 1024 * 1024))
largest=$(lsblk -bdnro SIZE,TYPE,RM 2>/dev/null | awk '$2 == "disk" && $3 == 0 { if ($1 > m) m = $1 } END { print m + 0 }')
if ((largest < min)); then
  stop "the largest fixed disk has $((largest / 1024 / 1024 / 1024)) GiB; the VRX layout needs at least 96 GiB"
fi

if [[ ! -d /sys/firmware/efi ]]; then
  say "BIOS boot: GRUB goes to the disk (bios_grub partition), not the ESP"
  python3 - /autoinstall.yaml <<'PY'
import sys, yaml
p = sys.argv[1]
doc = yaml.safe_load(open(p))
root = doc.get('autoinstall', doc)
for a in root['storage']['config']:
    if a.get('id') == 'disk0':
        a['grub_device'] = True
    elif a.get('type') == 'partition' and a.get('grub_device'):
        a['grub_device'] = False
with open(p, 'w') as f:
    yaml.safe_dump(doc, f, default_flow_style=False, sort_keys=False)
PY
else
  say "UEFI boot: GRUB goes to the ESP (VRX-EFI)"
fi

if [[ -f /cdrom/vrx/luks ]]; then
  ( umask 077; head -c 64 /dev/urandom > /run/vrx-luks.key )
  say "LUKS: per-install key generated in RAM (/run/vrx-luks.key)"
fi
say "done"
