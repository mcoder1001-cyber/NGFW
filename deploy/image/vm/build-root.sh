#!/usr/bin/env bash
# Internal builder: invoked only after build.sh's trust and namespace checks.
set -euo pipefail
[[ $# == 7 && ${NGFW_INTEGRATION:-0} == 1 && $EUID == 0 ]] || exit 2
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
REPO=$(cd "$HERE/../../.." && pwd -P)
WORK=$(realpath -e "$1") POOL=$(realpath -e "$2") PROFILE=$3 SIZE=$4 MIRROR=$5 VPP=$6 MGMT=$7
[[ $WORK == "$REPO/.scratch/w16-images-$PROFILE" && $PROFILE =~ ^(vm|aws|azure|gcp)$ && $SIZE =~ ^[0-9]+$ && $SIZE -ge 96 && $SIZE -le 4096 ]] || exit 2
[[ $(readlink /proc/self/ns/mnt) != "$(readlink /proc/1/ns/mnt)" ]] || { echo 'private mount namespace required' >&2; exit 2; }
COMMON=$REPO/deploy/image/iso/common ROOT=$WORK/root RAW=$WORK/w16-disk.raw OUT=$WORK/output LOOP=''
MOUNTS=()
cleanup() {
  local rc=$? failed=0
  trap - EXIT
  for ((i=${#MOUNTS[@]}-1; i>=0; i--)); do umount "${MOUNTS[i]}" || failed=1; done
  if [[ -n $LOOP ]]; then losetup -d "$LOOP" || failed=1; fi
  if ((failed)); then echo 'cleanup failure: preserve scratch and inspect owned mount/loop evidence' >&2; exit 1; fi
  if ((rc == 0)); then rm -f -- "$RAW"; fi
  exit "$rc"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
mount_owned() { mount "$@"; MOUNTS+=("${@: -1}"); }
# Bootstrap into image-mounted root, not the running system. Ubuntu Release verification remains enabled.
truncate -s "${SIZE}G" "$RAW"
python3 "$HERE/image.py" partition --size "$SIZE" | sfdisk "$RAW" > "$OUT/partition.txt"
LOOP=$(losetup --find --show --partscan "$RAW")
for ((i=1; i<=50; i++)); do [[ -b ${LOOP}p7 ]] && break; sleep 0.1; done
[[ -b ${LOOP}p7 ]] || { echo 'partition nodes unavailable' >&2; exit 1; }
# debootstrap includes dosfstools so FAT formatting can run from the target without host package installs.
# A temporary bootstrap tree is populated before filesystem mounts are ready.
BOOT=$WORK/bootstrap
mkdir "$BOOT"
debootstrap --arch=amd64 --variant=minbase --keyring="$WORK/ubuntu.gpg" --include=dosfstools resolute "$BOOT" "$MIRROR"
# FAT formatter and its libraries are executed in the build tree. Only our loop partition is exposed.
mkdir -p "$BOOT/dev"
touch "$BOOT/dev/ngfw-efi"
mount_owned --bind "${LOOP}p2" "$BOOT/dev/ngfw-efi"
chroot "$BOOT" /sbin/mkfs.vfat -F 32 -n NGFW-EFI /dev/ngfw-efi
umount "$BOOT/dev/ngfw-efi"; unset "MOUNTS[$((${#MOUNTS[@]}-1))]"
for spec in '3 ngfw-rootA' '4 ngfw-rootB' '5 ngfw-log' '6 ngfw-pg' '7 ngfw-data'; do
  read -r number label <<< "$spec"
  mkfs.ext4 -q -F -L "$label" "${LOOP}p$number"
done
mount_owned "${LOOP}p3" "$ROOT"
for spec in '2 /boot/efi' '5 /var/log' '6 /var/lib/postgresql' '7 /data'; do
  read -r number path <<< "$spec"
  mkdir -p "$ROOT$path"; mount_owned "${LOOP}p$number" "$ROOT$path"
done
cp -a "$BOOT/." "$ROOT/"
rm -rf --one-file-system -- "$BOOT"
printf '#!/bin/sh\nexit 101\n' > "$ROOT/usr/sbin/policy-rc.d"; chmod 0755 "$ROOT/usr/sbin/policy-rc.d"
# Install only from the signed pool with no network, in a non-booting private container.
install -D -m 0644 "$WORK/pool.gpg" "$ROOT/usr/share/keyrings/ngfw-image-pool.gpg"
mkdir -p "$ROOT/etc/apt/image-empty"
echo 'deb [signed-by=/usr/share/keyrings/ngfw-image-pool.gpg] file:/pool resolute main' > "$ROOT/etc/apt/image.list"
APT=(apt-get -q -o Dir::Etc::sourcelist=/etc/apt/image.list -o Dir::Etc::sourceparts=/etc/apt/image-empty -o APT::Install-Recommends=true -o Dpkg::Options::=--force-confold)
run_target() { systemd-nspawn -q --register=no --private-network --timezone=off --resolv-conf=off -M w16-img -D "$ROOT" --bind-ro="$POOL:/pool" --setenv=DEBIAN_FRONTEND=noninteractive -- "$@"; }
run_target "${APT[@]}" update
mapfile -t PACKAGES < <(awk '!/^[[:space:]]*(#|$)/ {print $1}' "$COMMON/packages.list")
run_target "${APT[@]}" install -y "${PACKAGES[@]}" linux-image-generic grub-pc-bin grub-efi-amd64-bin grub2-common cloud-init openssh-server netplan.io systemd-resolved
# Apply P14 removal/disable policy; VM keeps cloud-init.
POLICY_PROFILE=vm; [[ $PROFILE == vm ]] || POLICY_PROFILE=cloud
listed() { awk -v p="$POLICY_PROFILE" '!/^[[:space:]]*(#|$)/ { n=split($2,a,","); for(i=1;i<=n;i++) if(a[i]=="all"||a[i]==p) { print $1; break } }' "$1"; }
mapfile -t PURGE < <(listed "$COMMON/purge.list")
run_target "${APT[@]}" purge -y "${PURGE[@]}"
for unit in $(listed "$COMMON/units-disable.list"); do systemctl --root="$ROOT" disable "$unit"; done
bash "$COMMON/bin/ngfw-image-setup" --root "$ROOT" --profile "$POLICY_PROFILE" --serial-console
python3 "$HERE/image.py" configure --root "$ROOT" --profile "$PROFILE" --management-interface "$MGMT"
run_target update-initramfs -u -k all
# Minimal target /dev for bootloader installation: no host disks or host /boot exposed.
mount_owned -t tmpfs -o mode=755 tmpfs "$ROOT/dev"
for node in "$LOOP" "${LOOP}"p{1..7}; do
  touch "$ROOT/dev/${node##*/}"; mount_owned --bind "$node" "$ROOT/dev/${node##*/}"
done
for spec in 'null 1 3' 'zero 1 5' 'random 1 8' 'urandom 1 9'; do
  read -r name major minor <<< "$spec"; mknod -m 666 "$ROOT/dev/$name" c "$major" "$minor"
done
mount_owned -t proc proc "$ROOT/proc"
mount_owned -t sysfs -o ro sysfs "$ROOT/sys"
# Every call has explicit target directories and --no-nvram. Both programs run from target packages.
chroot "$ROOT" grub-install --target=i386-pc --boot-directory=/boot --efi-directory=/boot/efi --no-nvram "$LOOP"
chroot "$ROOT" grub-install --target=x86_64-efi --boot-directory=/boot --efi-directory=/boot/efi --no-nvram --removable "$LOOP"
python3 "$HERE/image.py" grub --root "$ROOT"
# Remove build-only APT paths, keys, identity, logs and cached state. No secrets are generated during the build.
rm -f "$ROOT/usr/sbin/policy-rc.d" "$ROOT/etc/apt/image.list" "$ROOT/usr/share/keyrings/ngfw-image-pool.gpg"
rm -rf -- "$ROOT/var/lib/cloud" "$ROOT/var/cache/apt/archives" "$ROOT/var/lib/apt/lists" "$ROOT/var/log/installer"
find "$ROOT/var/log" -type f -exec truncate -s 0 {} +
# bootstrap may have left an image-only resolv.conf; leave systemd-resolved's standard runtime link.
rm -f "$ROOT/etc/resolv.conf"; ln -s /run/systemd/resolve/stub-resolv.conf "$ROOT/etc/resolv.conf"
: > "$ROOT/etc/machine-id"
python3 "$HERE/image.py" inspect --root "$ROOT" --profile "$PROFILE" > "$OUT/inspection.json"
# Assert installed VPP version equals verified manifest, not just 26.06 prefix.
python3 - "$OUT/inspection.json" "$VPP" <<'CHECK'
import json,sys
inspection=json.load(open(sys.argv[1])); vpp=json.load(open(sys.argv[2]))
assert inspection['packages']['vpp'] == vpp['version'], 'installed VPP differs from input manifest'
CHECK
sync
for ((i=${#MOUNTS[@]}-1; i>=0; i--)); do umount "${MOUNTS[i]}"; done
MOUNTS=()
losetup -d "$LOOP"; LOOP=''
sfdisk --json "$RAW" > "$OUT/partition.json"
"$HERE/convert.sh" "$RAW" "$OUT" "$PROFILE" "$SIZE"
python3 "$HERE/image.py" manifest --output "$OUT" --profile "$PROFILE" --size "$SIZE" --vpp-manifest "$VPP"
(cd "$OUT" && sha256sum -c SHA256SUMS) > "$WORK/checksums-verified.txt"
echo "Artifacts inspected and converted without booting: $OUT"
