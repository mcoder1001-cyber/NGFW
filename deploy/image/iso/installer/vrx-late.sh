#!/usr/bin/env bash
# vrx-late.sh — autoinstall late-commands of the VRX ISO (P14): turns the freshly installed Ubuntu base in /target into
# shellcheck disable=SC2016  # dpkg-query ${…} formats are meant literally
# a VRX appliance using ONLY the ISO (works with the network cable unplugged).
#
#   vrx-late.sh /target
#
#  1. the embedded pool (/cdrom/vrx/apt, signed by the ISO key) is bind-mounted into the target and used as the ONLY
#     apt source through private -o options (nothing persists in the target's apt configuration)
#  2. common/packages.list installed with Recommends and --force-confold (exactly like scripts/10-install-runtime.sh)
#  3. common/purge.list (profile iso) purged, common/units-disable.list disabled, apt-get autoremove (same as P10)
#  4. common/bin/vrx-image-setup: banner + bootstrap-password units, kernel cmdline/sysctl/modules defaults; update-grub
#  5. the VRX archive keyring (P10's repo key) at /usr/share/keyrings/vrx-archive-keyring.gpg, optional online repo
#     entry (/cdrom/vrx/apt-uri), optional root SSH keys (/cdrom/vrx/ssh/authorized_keys), host name
#  6. /etc/vrx/bootstrap.env with a random admin password (vrx-bootstrap-password; never printed or logged)
#  7. --luks ISOs: the per-install key moves from RAM to /etc/vrx/luks/vrx-luks.key (0400) and crypttab uses it
#  8. checks: vrx-meta + vpp installed at the pool's versions, /etc/vrx/appliance present, firstboot enabled
# Log: /var/log/vrx-late.log in the installer, copied to /target/var/log/installer/vrx-late.log (no secrets in it).
set -euo pipefail
export LC_ALL=C DEBIAN_FRONTEND=noninteractive
T=${1:?usage: vrx-late.sh /target}
MEDIA=/cdrom/vrx
LOG=/var/log/vrx-late.log
exec > >(tee -a "$LOG") 2>&1
say() { echo "vrx-late $(date +%H:%M:%S): $*"; }
die() { echo "vrx-late: ERROR: $*"; exit 1; }
[[ -d $T/etc && -d $MEDIA/apt ]] || die "target $T or pool $MEDIA/apt missing"
COMMON=$MEDIA/common
read -r PROFILE < "$MEDIA/profile" 2>/dev/null || PROFILE=iso

# --- chroot runner: curtin in-target when available (it mounts /dev /proc /sys /run and blocks daemon starts)
if command -v curtin >/dev/null 2>&1; then
  in_target() { curtin in-target --target="$T" -- "$@"; }
else
  for m in dev proc sys run; do mount --bind "/$m" "$T/$m"; done
  printf '#!/bin/sh\nexit 101\n' > "$T/usr/sbin/policy-rc.d"; chmod 0755 "$T/usr/sbin/policy-rc.d"
  in_target() { chroot "$T" "$@"; }
fi

W=/var/lib/vrx-install            # inside the target; removed at the end
cleanup() {
  umount "$T$W/pool" 2>/dev/null || true
  rm -rf -- "${T:?}$W"
  if ! command -v curtin >/dev/null 2>&1; then
    rm -f "$T/usr/sbin/policy-rc.d"
    for m in run sys proc dev; do umount "$T/$m" 2>/dev/null || true; done
  fi
  cp -f "$LOG" "$T/var/log/installer/vrx-late.log" 2>/dev/null || true
}
trap cleanup EXIT
mkdir -p "$T$W/pool" "$T$W/lists/partial" "$T$W/parts.d" "$T$W/prefs.d"
mount --bind "$MEDIA/apt" "$T$W/pool"
mount -o remount,bind,ro "$T$W/pool" 2>/dev/null || true
install -m 0644 "$MEDIA/apt/vrx-iso-pool-keyring.gpg" "$T$W/pool-keyring.gpg"
echo "deb [signed-by=$W/pool-keyring.gpg] file:$W/pool resolute main" > "$T$W/sources.list"
APT=(apt-get -q -o Dir::Etc::SourceList="$W/sources.list" -o Dir::Etc::SourceParts="$W/parts.d"
  -o Dir::Etc::PreferencesParts="$W/prefs.d" -o Dir::State::Lists="$W/lists" -o APT::Get::List-Cleanup=0
  -o Acquire::Languages=none -o APT::Install-Recommends=true -o Dpkg::Options::=--force-confold)

say "pool: $(grep -c '^Package:' "$MEDIA/apt/dists/resolute/main/binary-amd64/Packages") packages, offline source only"
in_target "${APT[@]}" update
mapfile -t PKGS < <(grep -vE '^[[:space:]]*(#|$)' "$COMMON/packages.list" | awk '{print $1}')
say "install: ${PKGS[*]}"
in_target env DEBIAN_FRONTEND=noninteractive "${APT[@]}" install -y "${PKGS[@]}"

# profile filter for the two lists: "<name> <profiles> <reason…>"
listed() { awk -v p="$PROFILE" '!/^[[:space:]]*(#|$)/ { n = split($2, a, ","); for (i = 1; i <= n; i++) if (a[i] == "all" || a[i] == p) { print $1; break } }' "$1"; }
mapfile -t PURGE < <(listed "$COMMON/purge.list")
INSTALLED=()
for p in "${PURGE[@]}"; do
  if in_target dpkg-query -W -f='${db:Status-Abbrev}' "$p" 2>/dev/null | grep -q '^ii'; then INSTALLED+=("$p"); fi
done
if ((${#INSTALLED[@]})); then
  say "purge: ${INSTALLED[*]}"
  in_target env DEBIAN_FRONTEND=noninteractive "${APT[@]}" purge -y "${INSTALLED[@]}"
fi
in_target env DEBIAN_FRONTEND=noninteractive "${APT[@]}" autoremove -y
[[ " ${INSTALLED[*]} " == *" cloud-init "* ]] && rm -rf -- "$T/etc/cloud" "$T/var/lib/cloud"
for u in $(listed "$COMMON/units-disable.list"); do
  say "disable $u"; in_target systemctl disable "$u" 2>&1 || true
done

say "image defaults (profile $PROFILE)"
SETUP_ARGS=(--root "$T" --profile "$PROFILE" --build-info "$MEDIA/build-info.json")
[[ -f $MEDIA/serial-console ]] && SETUP_ARGS+=(--serial-console)
bash "$COMMON/bin/vrx-image-setup" "${SETUP_ARGS[@]}"
in_target update-grub

install -D -m 0644 "$MEDIA/keyrings/vrx-archive-keyring.gpg" "$T/usr/share/keyrings/vrx-archive-keyring.gpg"
if [[ -s $MEDIA/apt-uri ]]; then
  read -r uri < "$MEDIA/apt-uri"
  [[ $uri =~ ^(https|file):[^[:space:]]+$ ]] || die "apt-uri '$uri' refused (https:// or file: only, D-202)"
  echo "deb [signed-by=/usr/share/keyrings/vrx-archive-keyring.gpg] $uri resolute main" > "$T/etc/apt/sources.list.d/vrx.list"
fi
if [[ -s $MEDIA/ssh/authorized_keys ]]; then
  install -d -m 0700 "$T/root/.ssh"
  install -m 0600 "$MEDIA/ssh/authorized_keys" "$T/root/.ssh/authorized_keys"
  say "root SSH keys installed ($(grep -c . "$MEDIA/ssh/authorized_keys"))"
fi
read -r host < "$MEDIA/hostname" 2>/dev/null || host=vrx
[[ $host =~ ^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$ ]] || host=vrx
echo "$host" > "$T/etc/hostname"
sed -i "/^127\.0\.1\.1[[:space:]]/d" "$T/etc/hosts"; echo "127.0.1.1 $host" >> "$T/etc/hosts"

bash "$COMMON/bin/vrx-bootstrap-password" --root "$T" --force

if [[ -f $MEDIA/luks && -f /run/vrx-luks.key ]]; then
  install -d -m 0700 "$T/etc/vrx/luks"
  install -m 0400 /run/vrx-luks.key "$T/etc/vrx/luks/vrx-luks.key"
  sed -i -E 's#^([^[:space:]#]+_crypt[[:space:]]+[^[:space:]]+)[[:space:]]+[^[:space:]]+#\1 /etc/vrx/luks/vrx-luks.key#' "$T/etc/crypttab"
  say "LUKS: key in /etc/vrx/luks (0400), crypttab: $(grep -c _crypt "$T/etc/crypttab") volumes unlock without a prompt"
  in_target update-initramfs -u
fi

say "checks"
for p in vrx-meta vrx-agent vrx-api vrx-web vpp; do
  v=$(in_target dpkg-query -W -f='${Version} ${db:Status-Abbrev}' "$p") || die "$p not installed"
  [[ $v == *" ii "* ]] || die "$p: $v"
  say "  $p $v"
done
[[ -f $T/etc/vrx/appliance && $(stat -c %u "$T/etc/vrx/appliance") == 0 ]] || die "/etc/vrx/appliance missing"
[[ -L $T/etc/systemd/system/multi-user.target.wants/vrx-firstboot.service ]] || say "WARNING: vrx-firstboot.service not enabled"
[[ -f $T/etc/vrx/bootstrap.env ]] || die "bootstrap.env missing"
say "VRX appliance installed; first boot runs vrx-firstboot and shows the admin password on the console"
