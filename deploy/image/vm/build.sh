#!/usr/bin/env bash
# Build within this worktree's scratch; never install packages or bootloaders on the host.
set -euo pipefail
umask 022
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
REPO=$(cd "$HERE/../../.." && pwd -P)
help() { cat <<'EOF'
usage: NGFW_INTEGRATION=1 deploy/image/vm/build.sh --pool DIR --pool-key FILE
       --pool-key-fpr FINGERPRINT --vpp-manifest FILE --management-interface NIC [--profile vm|aws|azure|gcp]
       [--size-gib 96] [--mirror https://archive.ubuntu.com/ubuntu]
       [--ubuntu-keyring /usr/share/keyrings/ubuntu-archive-keyring.gpg]

POOL is a signed resolute APT dependency repository prepared by P14, extended to
include linux-image-generic, grub-pc-bin, grub-efi-amd64-bin, dosfstools,
cloud-init and openssh-server. Its trusted key fingerprint must be supplied
independently of the repository. No host installs or cloud import occur.
Outputs remain in .scratch/w16-images. Each profile requires its own build.
EOF
}
die() { echo "image-build: $*" >&2; exit 1; }
POOL='' KEY='' FPR='' VPP='' PROFILE=vm SIZE=96 MIRROR=https://archive.ubuntu.com/ubuntu UBKEY=/usr/share/keyrings/ubuntu-archive-keyring.gpg MGMT=''
while (($#)); do
  case $1 in
    --pool) POOL=${2:?}; shift ;; --pool-key) KEY=${2:?}; shift ;;
    --pool-key-fpr) FPR=${2:?}; shift ;; --vpp-manifest) VPP=${2:?}; shift ;;
    --management-interface) MGMT=${2:?}; shift ;; --profile) PROFILE=${2:?}; shift ;; --size-gib) SIZE=${2:?}; shift ;;
    --mirror) MIRROR=${2:?}; shift ;; --ubuntu-keyring) UBKEY=${2:?}; shift ;;
    -h|--help) help; exit 0 ;; *) die "unknown option $1" ;;
  esac; shift
done
[[ ${NGFW_INTEGRATION:-0} == 1 && $EUID == 0 ]] || die 'root image builds require NGFW_INTEGRATION=1'
[[ $PROFILE =~ ^(vm|aws|azure|gcp)$ && $SIZE =~ ^[0-9]+$ ]] || die 'invalid profile/size'
[[ $MGMT =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,14}$ && $MGMT != lo ]] || die 'explicit --management-interface required'
[[ $SIZE -ge 96 && $SIZE -le 4096 ]] || die 'minimum disk 96 GiB, maximum 4096 GiB'
[[ ${FPR^^} =~ ^[0-9A-F]{40}$ ]] || die 'independently pinned --pool-key-fpr required'
[[ -d $POOL && -f $KEY && -f $VPP && -f $UBKEY ]] || die 'signed pool, keyrings and VPP manifest must exist'
POOL=$(realpath -e "$POOL"); KEY=$(realpath -e "$KEY"); VPP=$(realpath -e "$VPP"); UBKEY=$(realpath -e "$UBKEY")
for cmd in debootstrap qemu-img sfdisk losetup mkfs.ext4 unshare systemd-nspawn mount umount gpg gpgv python3; do command -v "$cmd" >/dev/null || die "missing tool $cmd (do not install on host)"; done
SCRATCH=$(realpath -m "$REPO/.scratch")
[[ $SCRATCH == "$REPO/.scratch" ]] || die 'scratch symlink refused'
WORK=$SCRATCH/w16-images-$PROFILE
[[ ! -e $WORK ]] || die "$WORK already exists; preserve previous evidence and choose a clean worktree"
[[ $(df --output=avail -B1 "$REPO" | tail -1) -ge $((40 * 1024 * 1024 * 1024)) ]] || die 'at least 40 GiB free required'
# Ubuntu archive pin is shared with P14, not trusted from arbitrary keyring origin.
export NGFW_ISO_SCRATCH=$SCRATCH NGFW_ISO_TMP=$WORK/tmp
# shellcheck source=deploy/image/iso/lib/common.sh
. "$REPO/deploy/image/iso/lib/common.sh"
mkdir -p "$WORK/tmp" "$WORK/output" "$WORK/root"
ngfw_pinned_keyring "$KEY" "$FPR" "$WORK/pool.gpg"
ngfw_pinned_keyring "$UBKEY" F6ECB3762474EDA9D21B7022871920D1991BC93C "$WORK/ubuntu.gpg"
gpgv --keyring "$WORK/pool.gpg" "$POOL/dists/resolute/InRelease" > "$WORK/output/pool-signature.txt" 2>&1 || die 'pool signature invalid'
VPP_VERSION=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$VPP")
python3 "$REPO/deploy/image/iso/lib/verify-vpp-manifest.py" "$VPP" "$POOL" "$VPP_VERSION" "$REPO/deploy/vpp/VERSION" > "$WORK/output/vpp-contract.txt"
# No shared host mount propagation. Subsequent root operations execute in a private namespace.
exec unshare --mount --propagation private "$HERE/build-root.sh" "$WORK" "$POOL" "$PROFILE" "$SIZE" "$MIRROR" "$VPP" "$MGMT"
