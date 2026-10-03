#!/usr/bin/env bash
# build-iso.sh — the VRX unattended installer ISO (P14): Ubuntu 26.04 live-server + autoinstall + an offline pool with
# vrx-meta and its whole dependency closure → vrx-<version>.iso + .sha256 + .asc (detached GPG signature).
#
#   deploy/image/iso/build-iso.sh --vrx-repo DIR --vrx-key-fpr FPR [options]
#
# Inputs
#   --base-iso FILE        Ubuntu 26.04.x live-server ISO          (default .scratch/base/ubuntu-26.04.1-live-server-amd64.iso)
#   --fetch-base           download it (+ SHA256SUMS, SHA256SUMS.gpg) from releases.ubuntu.com first (resumable)
#   --vrx-repo DIR         P10's signed APT repo                    (default /srv/vrx-artifacts/apt)
#   --vrx-key-fpr FPR      its signing key fingerprint (required — trust is never taken from the repo, D-202)
#   --vpp-manifest FILE    F-vpp-debs manifest.json of the VPP in the repo (default /srv/vrx-artifacts/vpp/<ver>/manifest.json)
#   --mirror URL           Ubuntu archive for the closure          (default http://archive.ubuntu.com/ubuntu)
#   --frr-uri URL          FRR repo (frr-stable)                    (default https://deb.frrouting.org/frr)
#   --frr-keyring FILE     keyring holding the pinned FRR key       (default /usr/share/keyrings/frrouting.gpg)
#   --ubuntu-keyring FILE  Ubuntu archive + CD image keys           (default /usr/share/keyrings/ubuntu-archive-keyring.gpg)
#   --lock FILE            a previous build's pool.manifest: fetch exactly those versions (reproducible rebuild)
# Image options
#   --source ID            ubuntu-server-minimal (default) | ubuntu-server
#   --luks                 vrx-log, vrx-pg, vrx-data on LUKS (per-install key, unlocked at boot without a prompt)
#   --serial-console       console=tty0 console=ttyS0,115200n8 for the installer and the installed system
#   --ssh-authorized-keys FILE   public keys for root on the installed system (default: none — no SSH login)
#   --hostname NAME        (default vrx)      --apt-uri URI  online VRX repo entry on the target (https:// or file:)
#   --version V            ISO version (default: vrx-meta's version in the repo)
# Signing
#   --gpg-home DIR         key home (default ~/.config/ngfw/iso-signing, 0700, outside the repo; never printed)
#   --gen-key              create a dedicated ed25519 signing key there if it holds none
# Work
#   --scratch DIR          (default <worktree>/.scratch)  --out DIR (default <scratch>/out)  --work DIR (<scratch>/work)
#   --chroot DIR           build chroot (default <scratch>/chroot)   --make-chroot  debootstrap it (resolute, from --mirror)
#   --stage S              stop after: verify | pool | tree | iso (default iso)
# Network is used only by --fetch-base, --make-chroot and the closure download (host apt, isolated root); every
# container step runs in systemd-nspawn --private-network. Nothing is mounted on the host; no loop device is used.
set -euo pipefail
export LC_ALL=C
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
WT=$(cd "$HERE/../../.." && pwd -P)
# shellcheck source=lib/common.sh
. "$HERE/lib/common.sh"
# shellcheck source=lib/pool.sh
. "$HERE/lib/pool.sh"
heavy() { if [[ -n ${VRX_ISO_HEAVY:-} ]]; then "$VRX_ISO_HEAVY" "$@"; else "$@"; fi; }   # e.g. VRX_ISO_HEAVY=tools/heavy.sh (D-224)
COMMON=$HERE/common; [[ -d $HERE/../common ]] && COMMON=$(cd "$HERE/../common" && pwd -P)   # after a move to deploy/image/common

BASE_URL=https://releases.ubuntu.com/26.04
BASE_NAME=ubuntu-26.04.1-live-server-amd64.iso
CD_SIGNER_FPR=843938DF228D22F7B3742BC0D94AA3F0EFE21092   # Ubuntu CD Image Automatic Signing Key (2012)
SCRATCH=$WT/.scratch BASE_ISO='' FETCH=0 VRX_REPO=/srv/vrx-artifacts/apt VRX_FPR=${VRX_APT_KEY_FPR:-} VPP_MANIFEST=''
MIRROR=http://archive.ubuntu.com/ubuntu FRR_URI=https://deb.frrouting.org/frr FRR_KEYRING=/usr/share/keyrings/frrouting.gpg
UB_KEYRING=/usr/share/keyrings/ubuntu-archive-keyring.gpg LOCK='' SOURCE_ID=ubuntu-server-minimal LUKS=0 SERIAL=0
SSH_KEYS='' HOST=vrx APT_URI='' VERSION='' GPG_HOME=$HOME/.config/ngfw/iso-signing GEN_KEY=0 OUT='' WORK='' CHROOT=''
MAKE_CHROOT=0 STAGE=iso
while (($#)); do
  case $1 in
    --base-iso) BASE_ISO=$2; shift ;; --fetch-base) FETCH=1 ;; --vrx-repo) VRX_REPO=$2; shift ;;
    --vrx-key-fpr) VRX_FPR=$2; shift ;; --vpp-manifest) VPP_MANIFEST=$2; shift ;; --mirror) MIRROR=$2; shift ;;
    --frr-uri) FRR_URI=$2; shift ;; --frr-keyring) FRR_KEYRING=$2; shift ;; --ubuntu-keyring) UB_KEYRING=$2; shift ;;
    --lock) LOCK=$2; shift ;; --source) SOURCE_ID=$2; shift ;; --luks) LUKS=1 ;; --serial-console) SERIAL=1 ;;
    --ssh-authorized-keys) SSH_KEYS=$2; shift ;; --hostname) HOST=$2; shift ;; --apt-uri) APT_URI=$2; shift ;;
    --version) VERSION=$2; shift ;; --gpg-home) GPG_HOME=$2; shift ;; --gen-key) GEN_KEY=1 ;;
    --scratch) SCRATCH=$2; shift ;; --out) OUT=$2; shift ;; --work) WORK=$2; shift ;; --chroot) CHROOT=$2; shift ;;
    --make-chroot) MAKE_CHROOT=1 ;; --stage) STAGE=$2; shift ;;
    -h|--help) sed -n '2,/^set -euo pipefail/{/^set -euo pipefail/!p}' "$0" | sed -E 's/^# ?//'; exit 0 ;;
    *) die "unknown argument '$1' (try --help)" ;;
  esac
  shift
done
[[ $EUID -eq 0 ]] || die "root only (systemd-nspawn, file ownership in the pool)"
export VRX_ISO_SCRATCH=$SCRATCH
OUT=${OUT:-$SCRATCH/out} WORK=${WORK:-$SCRATCH/work} CHROOT=${CHROOT:-$SCRATCH/chroot}
BASE_ISO=${BASE_ISO:-$SCRATCH/base/$BASE_NAME}
[[ $STAGE =~ ^(verify|pool|tree|iso)$ ]] || die "--stage: verify | pool | tree | iso"
[[ $SOURCE_ID =~ ^ubuntu-server(-minimal)?$ ]] || die "--source: ubuntu-server-minimal | ubuntu-server"
[[ $HOST =~ ^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$ ]] || die "--hostname '$HOST' is not a host name"
[[ -z $APT_URI || $APT_URI =~ ^(https|file):[^[:space:]]+$ ]] || die "--apt-uri: https:// or file: only (D-202)"
[[ ${VRX_FPR^^} =~ ^[0-9A-F]{40}$ ]] || die "--vrx-key-fpr <40 hex> is required (or VRX_APT_KEY_FPR)"
for d in "$OUT" "$WORK"; do vrx_guard_path "$d"; done
for t in xorriso gpg gpgv systemd-nspawn dpkg-deb dpkg-scanpackages apt-get python3; do command -v "$t" >/dev/null || die "missing tool $t"; done

vrx_rm_rf "$WORK"; vrx_owned_dir "$WORK"; vrx_owned_dir "$OUT"
export VRX_ISO_TMP=$WORK/tmp; mkdir -p "$VRX_ISO_TMP"
EVID=$OUT/evidence; mkdir -p "$EVID"
STARTED=$(date -u +%FT%TZ)

# ------------------------------------------------------------------ chroot (cloud-init schema, unsquashfs, apt simulation)
if ((MAKE_CHROOT)); then
  vrx_guard_path "$CHROOT"; [[ -e $CHROOT ]] && die "$CHROOT exists (remove it first)"
  log "debootstrap resolute → $CHROOT (private mount namespace: no mount survives it)"
  unshare --mount --propagation private -- debootstrap --variant=minbase --arch=amd64 \
    --include=cloud-init,squashfs-tools,apt-utils,ca-certificates,gpgv,python3-jsonschema,python3-yaml \
    --keyring="$UB_KEYRING" resolute "$CHROOT" "$MIRROR" > "$WORK/debootstrap.log" 2>&1 ||
    { tail -20 "$WORK/debootstrap.log"; die "debootstrap failed"; }
  : > "$CHROOT/.vrx-iso-owned"
fi
[[ -x $CHROOT/usr/bin/cloud-init && -x $CHROOT/usr/bin/unsquashfs ]] || die "build chroot $CHROOT incomplete (--make-chroot)"

# ------------------------------------------------------------------ base ISO
if ((FETCH)); then
  mkdir -p "$(dirname "$BASE_ISO")"
  ( cd "$(dirname "$BASE_ISO")" && curl -fsSLO "$BASE_URL/SHA256SUMS" && curl -fsSLO "$BASE_URL/SHA256SUMS.gpg" &&
    curl -fL --retry 20 --retry-all-errors -C - -o "$(basename "$BASE_ISO")" "$BASE_URL/$(basename "$BASE_ISO")" ) ||
    die "base ISO download failed"
fi
BD=$(dirname "$BASE_ISO")
[[ -f $BASE_ISO && -f $BD/SHA256SUMS && -f $BD/SHA256SUMS.gpg ]] || die "need $BASE_ISO with SHA256SUMS(.gpg) beside it (--fetch-base)"
vrx_pinned_keyring "$UB_KEYRING" "$CD_SIGNER_FPR" "$WORK/cdimage.gpg"
vrx_gpgv "$WORK/cdimage.gpg" "$BD/SHA256SUMS.gpg" "$BD/SHA256SUMS" > "$EVID/base-sums.gpgv" || { cat "$EVID/base-sums.gpgv"; die "SHA256SUMS signature BAD"; }
want=$(awk -v f="$(basename "$BASE_ISO")" '$2 == "*" f || $2 == f { print $1 }' "$BD/SHA256SUMS")
[[ -n $want ]] || die "$(basename "$BASE_ISO") not listed in SHA256SUMS"
log "base ISO: verifying sha256"
BASE_SHA=$(vrx_sha256 "$BASE_ISO"); [[ $BASE_SHA == "$want" ]] || die "base ISO sha256 $BASE_SHA != signed $want"
printf '%s  %s\n' "$BASE_SHA" "$(basename "$BASE_ISO")" > "$EVID/base-iso.sha256"
log "base ISO OK: $BASE_SHA (SHA256SUMS signed by $CD_SIGNER_FPR)"

B=$WORK/base; mkdir -p "$B"
xorriso -osirrox on -indev "$BASE_ISO" -extract /boot/grub/grub.cfg "$B/grub.cfg" -extract /boot/grub/loopback.cfg "$B/loopback.cfg" \
  -extract /md5sum.txt "$B/md5sum.txt" -extract /casper/install-sources.yaml "$B/install-sources.yaml" \
  -extract /.disk/info "$B/disk-info" > "$WORK/xorriso-extract.log" 2>&1 || { tail "$WORK/xorriso-extract.log"; die "cannot read the base ISO"; }
chmod u+w "$B"/*
layer=$SOURCE_ID.squashfs; [[ $SOURCE_ID == ubuntu-server ]] && layer=ubuntu-server-minimal.ubuntu-server.squashfs
log "base status: var/lib/dpkg/status of casper/$layer"
heavy xorriso -osirrox on -indev "$BASE_ISO" -extract "/casper/$layer" "$WORK/layer.squashfs" >> "$WORK/xorriso-extract.log" 2>&1
vrx_nspawn "$CHROOT" --bind="$WORK:/work" -- unsquashfs -q -n -d /work/layer -f /work/layer.squashfs var/lib/dpkg/status > /dev/null
rm -f "$WORK/layer.squashfs"
BASE_STATUS=$WORK/layer/var/lib/dpkg/status
[[ -s $BASE_STATUS ]] || die "no dpkg status in casper/$layer"
log "base layer: $(grep -c '^Package:' "$BASE_STATUS") packages"

# ------------------------------------------------------------------ VRX repo (P10) + VPP manifest (F-vpp-debs)
[[ -f $VRX_REPO/dists/resolute/Release ]] || die "$VRX_REPO is not P10's repo (dists/resolute/Release missing)"
VRXK=$WORK/vrx-archive-keyring.gpg
vrx_pinned_keyring "$VRX_REPO/vrx-archive-keyring.asc" "$VRX_FPR" "$VRXK"
vrx_gpgv "$VRXK" "$VRX_REPO/dists/resolute/InRelease" > "$EVID/vrx-repo.gpgv" || { cat "$EVID/vrx-repo.gpgv"; die "VRX repo InRelease signature BAD"; }
VRX_VERSION=$(awk '/^Package: vrx-meta$/ { m = 1 } m && /^Version:/ { print $2; exit }' "$VRX_REPO/dists/resolute/main/binary-amd64/Packages")
VPP_VERSION=$(awk '/^Package: vpp$/ { m = 1 } m && /^Version:/ { print $2; exit }' "$VRX_REPO/dists/resolute/main/binary-amd64/Packages")
[[ -n $VRX_VERSION && -n $VPP_VERSION ]] || die "vrx-meta / vpp not in $VRX_REPO"
VERSION=${VERSION:-$VRX_VERSION}
[[ $VERSION =~ ^[0-9A-Za-z.+~-]+$ ]] || die "version '$VERSION' has odd characters"
VPP_MANIFEST=${VPP_MANIFEST:-/srv/vrx-artifacts/vpp/$VPP_VERSION/manifest.json}
[[ -f $VPP_MANIFEST ]] || die "VPP manifest $VPP_MANIFEST missing (--vpp-manifest)"
python3 "$HERE/lib/verify-vpp-manifest.py" "$VPP_MANIFEST" "$VRX_REPO" "$VPP_VERSION" "$HERE/../../vpp/VERSION" > "$EVID/vpp-manifest-check.txt" || {
  cat "$EVID/vpp-manifest-check.txt"; die "VPP manifest does not match the repo"
}
tail -1 "$EVID/vpp-manifest-check.txt" >&2
[[ $STAGE == verify ]] && { log "stage verify done"; exit 0; }

# ------------------------------------------------------------------ pool
A=$WORK/apt
pool_apt_root "$A" "$BASE_STATUS" "$VRX_REPO" "$VRXK" "$MIRROR" "$FRR_URI" "$UB_KEYRING" "$FRR_KEYRING"
mapfile -t PKGS < <(grep -vE '^[[:space:]]*(#|$)' "$COMMON/packages.list" | awk '{print $1}')
if [[ -n $LOCK ]]; then
  log "pool: exact versions from $LOCK"
  pool_apt "$A" update > "$A/update.log" 2>&1 || die "apt-get update failed"
  awk '!/^#/ { print $1, $2 }' "$LOCK" > "$A/wanted.txt"
else
  log "pool: resolving ${PKGS[*]} on the base layer (mirror $MIRROR, $FRR_URI, VRX repo)"
  pool_resolve "$A" "${PKGS[@]}"
fi
log "pool: fetching $(wc -l < "$A/wanted.txt") packages"
pool_fetch "$A" "$A/wanted.txt" "$WORK/debs"
pool_manifest "$A" "$WORK/debs" "$WORK/pool.manifest"
if [[ -n $LOCK ]]; then
  diff <(awk '!/^#/ { print $1, $2, $3, $4 }' "$LOCK") <(awk '{ print $1, $2, $3, $4 }' "$WORK/pool.manifest") > "$EVID/lock-diff.txt" ||
    { cat "$EVID/lock-diff.txt"; die "fetched packages differ from the lock"; }
fi
awk '$6 == "unknown"' "$WORK/pool.manifest" | grep . && die "packages of unknown origin in the pool"
EPOCH=$(date -u -d "$(vrx_kv_get "$VRX_REPO/dists/resolute/Release" Date)" +%s)   # pool Release date = the VRX repo's: reproducible

# signing key (outside the repo; only fingerprints are ever printed)
install -d -m 0700 "$GPG_HOME"
KEY=$(gpg --batch --homedir "$GPG_HOME" --list-secret-keys --with-colons 2>/dev/null | awk -F: '$1 == "fpr" { print $10; exit }')
if [[ -z $KEY ]]; then
  ((GEN_KEY)) || die "no signing key in $GPG_HOME (--gen-key creates a dedicated one)"
  gpg --batch --homedir "$GPG_HOME" --passphrase '' --quick-gen-key "VRX installer media signing key" ed25519 sign never 2>/dev/null
  KEY=$(gpg --batch --homedir "$GPG_HOME" --list-secret-keys --with-colons | awk -F: '$1 == "fpr" { print $10; exit }')
  log "generated signing key $KEY in $GPG_HOME"
fi
log "signing key $KEY"
T=$WORK/tree; mkdir -p "$T/vrx/keyrings" "$T/nocloud"
pool_repo "$WORK/debs" "$T/vrx/apt" "$EPOCH"
gpg --batch --homedir "$GPG_HOME" --export "$KEY" > "$T/vrx/apt/vrx-iso-pool-keyring.gpg"
gpg --batch --homedir "$GPG_HOME" --armor --export "$KEY" > "$OUT/vrx-iso-signing-key.asc"
pool_sign "$T/vrx/apt" "$GPG_HOME" "$KEY"
cp "$VRXK" "$T/vrx/keyrings/vrx-archive-keyring.gpg"
cp "$WORK/pool.manifest" "$OUT/pool.manifest"

# offline proof: base layer status + this pool ONLY, no network at all
log "offline proof: apt-get install --simulate in nspawn --private-network against the pool only"
mkdir -p "$WORK/sim"
cp "$BASE_STATUS" "$WORK/sim/status"
cat > "$WORK/sim/run.sh" <<EOF
set -eu
o="-o Dir::State::status=/sim/status -o Dir::Etc::SourceList=/sim/sources.list -o Dir::Etc::SourceParts=/sim/none.d -o Dir::State::Lists=/sim/lists -o Dir::Cache=/sim/cache -o Acquire::Languages=none -o APT::Install-Recommends=true -o APT::Sandbox::User=root"
mkdir -p /sim/none.d /sim/lists/partial /sim/cache/archives/partial
echo "deb [signed-by=/pool/vrx-iso-pool-keyring.gpg] file:/pool resolute main" > /sim/sources.list
echo "== ip link (container: loopback only) =="; cat /proc/net/dev | awk 'NR > 2 { sub(":", "", \$1); print \$1 }'
echo "== apt-get update (pool only) =="; apt-get \$o update 2>&1 | grep -vE '^(Reading|Building)'
echo "== apt-get install --simulate ${PKGS[*]} =="; apt-get \$o -s install ${PKGS[*]} > /sim/install.txt 2>&1; rc=\$?
grep -cE '^Inst ' /sim/install.txt | sed 's/^/Inst lines: /'; tail -3 /sim/install.txt; echo "rc=\$rc"
purge="\$(for p in \$(awk -v p=iso '!/^[[:space:]]*(#|\$)/ { n = split(\$2, a, ","); for (i = 1; i <= n; i++) if (a[i] == "all" || a[i] == p) print \$1 }' /common/purge.list); do grep -qx "Package: \$p" /sim/status && printf '%s- ' "\$p"; done)"
echo "== apt-get --simulate --purge install ${PKGS[*]} \$purge (purge.list, profile iso) =="
apt-get \$o -s --purge install ${PKGS[*]} \$purge > /sim/purge.txt 2>&1; prc=\$?
grep -E '^(Purg|Remv) ' /sim/purge.txt | awk '{print \$1, \$2}' | tr '\n' ' '; echo; tail -1 /sim/purge.txt; echo "rc=\$prc"
if grep -E '^(Purg|Remv) (vrx-|vpp|libvppinfra|frr|postgresql|nginx|valkey)' /sim/purge.txt; then echo "purge removes appliance packages"; exit 1; fi
grep -q '^E:' /sim/install.txt /sim/purge.txt && exit 1
exit \$((rc + prc))
EOF
vrx_nspawn "$CHROOT" --bind="$WORK/sim:/sim" --bind-ro="$T/vrx/apt:/pool" --bind-ro="$COMMON:/common" -- sh /sim/run.sh \
  > "$EVID/offline-simulate.txt" 2>&1 || { cat "$EVID/offline-simulate.txt"; die "the pool does NOT resolve offline"; }
cp "$WORK/sim/install.txt" "$EVID/offline-simulate-install.txt"; cp "$WORK/sim/purge.txt" "$EVID/offline-simulate-purge.txt"
log "offline proof OK ($(grep -c '^Inst ' "$WORK/sim/install.txt") packages installed from the pool)"
[[ $STAGE == pool ]] && { log "stage pool done"; exit 0; }

# ------------------------------------------------------------------ autoinstall + tree
storage=$HERE/autoinstall/storage-plain.yaml; ((LUKS)) && storage=$HERE/autoinstall/storage-luks.yaml
python3 "$HERE/lib/render.py" user-data "$HERE/autoinstall/user-data.in" "$storage" "$T/nocloud/user-data" "$VERSION" "$SOURCE_ID"
printf 'instance-id: vrx-installer-%s\n' "$(printf '%s' "$VERSION" | sha256sum | cut -c1-12)" > "$T/nocloud/meta-data"
python3 -c 'import sys, yaml; d = yaml.safe_load(open(sys.argv[1])); assert d["autoinstall"]["version"] == 1' "$T/nocloud/user-data" ||
  die "user-data is not valid YAML"
log "cloud-init schema check (build chroot, no network)"
vrx_nspawn "$CHROOT" --bind-ro="$T/nocloud:/nocloud" -- cloud-init schema --config-file /nocloud/user-data > "$EVID/cloud-init-schema.txt" 2>&1 ||
  { cat "$EVID/cloud-init-schema.txt"; die "cloud-init schema rejects user-data"; }
cp "$T/nocloud/user-data" "$OUT/user-data"

mkdir -p "$T/vrx/installer" "$T/vrx/common" "$T/vrx/ssh" "$T/boot/grub"
install -m 0755 "$HERE/installer/vrx-early.sh" "$HERE/installer/vrx-late.sh" "$HERE/installer/vrx-disk-guard.sh" "$T/vrx/installer/"
cp -a "$COMMON/." "$T/vrx/common/"
echo iso > "$T/vrx/profile"; echo "$HOST" > "$T/vrx/hostname"
((LUKS)) && echo 1 > "$T/vrx/luks"
((SERIAL)) && echo 1 > "$T/vrx/serial-console"
[[ -n $APT_URI ]] && echo "$APT_URI" > "$T/vrx/apt-uri"
if [[ -n $SSH_KEYS ]]; then
  grep -E '^(ssh-(ed25519|rsa)|ecdsa-sha2-nistp(256|384|521)|sk-) ' "$SSH_KEYS" > "$T/vrx/ssh/authorized_keys" || die "--ssh-authorized-keys: no public key in $SSH_KEYS"
fi
cp "$WORK/pool.manifest" "$T/vrx/pool.manifest"

python3 - "$T/vrx/build-info.json" "$VPP_MANIFEST" <<PY
import json, sys
m = json.load(open(sys.argv[2]))
info = {
  "schema": "vrx.iso.build-info/v1", "version": "$VERSION", "built": "$STARTED",
  "base_iso": {"file": "$(basename "$BASE_ISO")", "sha256": "$BASE_SHA", "source": "$SOURCE_ID", "sums_key": "$CD_SIGNER_FPR"},
  "vrx_repo": {"vrx_meta": "$VRX_VERSION", "key": "${VRX_FPR^^}",
               "inrelease_sha256": "$(vrx_sha256 "$VRX_REPO/dists/resolute/InRelease")"},
  "vpp": {"version": m["version"], "variant": m.get("variant"), "upstream": {"tag": m["upstream"].get("tag"), "commit": m["upstream"]["commit"]},
          "patches": [{"name": p["name"], "sha256": p["sha256"]} for p in m.get("patches", [])],
          "build": {"inputs": m["build"].get("inputs"), "build_patches": m["build"].get("build_patches"), "source": m["build"].get("source")}},
  "pool": {"packages": $(wc -l < "$WORK/pool.manifest"), "manifest_sha256": "$(vrx_sha256 "$WORK/pool.manifest")", "signing_key": "$KEY"},
  "options": {"luks": bool($LUKS), "serial_console": bool($SERIAL), "ssh_keys": bool("$SSH_KEYS"), "hostname": "$HOST"}}
json.dump(info, open(sys.argv[1], "w"), indent=2, sort_keys=True); open(sys.argv[1], "a").write("\n")
PY
cp "$T/vrx/build-info.json" "$OUT/build-info.json"

# GRUB: the VRX entries first (default, 5 s), the original Ubuntu entries kept below (rescue)
extra=''; ((SERIAL)) && extra=' console=tty0 console=ttyS0,115200n8'
python3 "$HERE/lib/render.py" grub "$B/grub.cfg" "$T/boot/grub/grub.cfg" "$VERSION" "$extra"

# md5sum.txt: original entries for untouched files, fresh ones for ours (the installer's "check disc" stays valid)
find "$T" -type f -printf '%P\n' | LC_ALL=C sort > "$WORK/ours.txt"
{ awk 'NR == FNR { ours["./" $0] = 1; next } !($2 in ours)' "$WORK/ours.txt" "$B/md5sum.txt"
  ( cd "$T" && while read -r f; do printf '%s  ./%s\n' "$(md5sum < "$f" | cut -d' ' -f1)" "$f"; done < "$WORK/ours.txt" ); } |
  LC_ALL=C sort -k2 > "$T/md5sum.txt"
find "$T" -exec touch -h -d "@$EPOCH" {} +
[[ $STAGE == tree ]] && { log "stage tree done: $T"; exit 0; }

# ------------------------------------------------------------------ ISO
ISO=$OUT/vrx-$VERSION.iso; rm -f "$ISO" "$ISO.sha256" "$ISO.asc"
volid="VRX ${VERSION%%[~+]*} amd64"
log "xorriso: $ISO (boot setup replayed from the base ISO: BIOS El Torito + UEFI ESP)"
heavy xorriso -indev "$BASE_ISO" -outdev "$ISO" -volid "${volid:0:32}" \
  -map "$T/nocloud" /nocloud -map "$T/vrx" /vrx -map "$T/boot/grub/grub.cfg" /boot/grub/grub.cfg \
  -map "$T/md5sum.txt" /md5sum.txt \
  -boot_image any replay > "$EVID/xorriso-build.log" 2>&1 || { tail -20 "$EVID/xorriso-build.log"; die "xorriso failed"; }
( cd "$OUT" && sha256sum "$(basename "$ISO")" > "$(basename "$ISO").sha256" )
gpg --batch --quiet --homedir "$GPG_HOME" --local-user "$KEY" --armor --detach-sign -o "$ISO.asc" "$ISO"
gpg --batch --no-default-keyring --keyring "$T/vrx/apt/vrx-iso-pool-keyring.gpg" --verify "$ISO.asc" "$ISO" > "$EVID/iso.gpg-verify" 2>&1 ||
  die "signature check of the ISO failed"
xorriso -indev "$ISO" -report_el_torito plain > "$EVID/iso-el-torito.txt" 2>&1 || true
xorriso -indev "$ISO" -find /nocloud /vrx -maxdepth 2 > "$EVID/iso-listing.txt" 2>&1 || true
rm -rf -- "$VRX_ISO_TMP"
log "done: $ISO ($(stat -c %s "$ISO") bytes) sha256 $(cut -d' ' -f1 "$ISO.sha256")"
