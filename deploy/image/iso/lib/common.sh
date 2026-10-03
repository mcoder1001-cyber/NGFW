# shellcheck shell=bash
# deploy/image/iso/lib/common.sh — helpers shared by build-iso.sh and tests/run.sh (P14). Sourced, never executed.
# Nothing here touches the build host outside the directories it is given: every write goes below a directory that
# carries the `.vrx-iso-owned` marker (vrx_rm_rf, vrx_owned_dir), GPG runs with a private GNUPGHOME, and container work
# runs in systemd-nspawn with --private-network.

VRX_ISO_LOG_PREFIX=${VRX_ISO_LOG_PREFIX:-build-iso}

log() { printf '[%s %s] %s\n' "$VRX_ISO_LOG_PREFIX" "$(date +%H:%M:%S)" "$*" >&2; }
warn() { printf '[%s] WARNING: %s\n' "$VRX_ISO_LOG_PREFIX" "$*" >&2; }
die() { printf '[%s] ERROR: %s\n' "$VRX_ISO_LOG_PREFIX" "$*" >&2; exit 1; }

# vrx_guard_path <path>: refuse the host's system trees and anything that is not below an allowed base. The bases are
# the worktree's .scratch (default) plus VRX_ISO_ALLOW_DIRS (colon separated, absolute) for builds elsewhere.
vrx_guard_path() {
  local p=$1 real base
  [[ $p == /* ]] || die "path '$p' must be absolute"
  real=$(realpath -m -- "$p")
  case $real in
    /|/bin|/bin/*|/boot|/boot/*|/dev|/dev/*|/etc|/etc/*|/lib|/lib/*|/lib64|/lib64/*|/proc|/proc/*|/root|/run|/run/*|\
    /sbin|/sbin/*|/srv|/sys|/sys/*|/tmp|/usr|/usr/*|/var|/var/lib|/var/lib/*|/home|/root/vpp|/root/vpp/*|\
    /root/ngfw|/root/ngfw/*|/root/NGFW|/root/NGFW/*)
      die "refused path '$p' (host system tree or main checkout)" ;;
  esac
  local IFS=:
  for base in ${VRX_ISO_SCRATCH:-} ${VRX_ISO_ALLOW_DIRS:-}; do
    [[ -n $base ]] || continue
    base=$(realpath -m -- "$base")
    if [[ $real == "$base" || $real == "$base"/* ]]; then return 0; fi
  done
  die "refused path '$p': not below the scratch dir (${VRX_ISO_SCRATCH:-unset}) or VRX_ISO_ALLOW_DIRS"
}

# vrx_owned_dir <dir>: create (if needed) a work directory and mark it as ours so that vrx_rm_rf may later empty it
vrx_owned_dir() {
  vrx_guard_path "$1"
  mkdir -p -- "$1"
  : > "$1/.vrx-iso-owned"
}

# vrx_rm_rf <dir>: delete a directory tree only if it is guarded AND carries our marker (never a foreign directory)
vrx_rm_rf() {
  local d=$1
  [[ -e $d ]] || return 0
  vrx_guard_path "$d"
  [[ -f $d/.vrx-iso-owned ]] || die "refusing to delete $d: no .vrx-iso-owned marker"
  rm -rf --one-file-system -- "$d"
}

# vrx_fprs <keyring-or-armored-key>: the primary key fingerprints in it, one per line (upper case)
vrx_fprs() {
  gpg --batch --no-default-keyring --show-keys --with-colons -- "$1" 2>/dev/null |
    awk -F: '$1 == "pub" { want = 1; next } $1 == "fpr" && want { print toupper($10); want = 0 }'
}

# vrx_pinned_keyring <source keyring/armored key> <fingerprint> <out.gpg>: write a binary keyring holding exactly the
# key with that fingerprint (trust comes from the pin, never from the file's origin — D-202)
vrx_pinned_keyring() {
  local src=$1 fpr=${2^^} out=$3 gh
  [[ $fpr =~ ^[0-9A-F]{40}$ ]] || die "fingerprint '$2' is not 40 hex digits"
  [[ -f $src ]] || die "keyring $src missing"
  vrx_fprs "$src" | grep -qx "$fpr" || die "$src does not carry the pinned key $fpr"
  gh=$(mktemp -d "${VRX_ISO_TMP:?}/gnupg.XXXXXX")
  gpg --batch --quiet --homedir "$gh" --import -- "$src" 2>/dev/null || die "cannot import $src"
  gpg --batch --quiet --homedir "$gh" --export "$fpr" > "$out.tmp" || die "cannot export $fpr"
  rm -rf -- "$gh"
  [[ -s $out.tmp ]] || die "export of $fpr from $src is empty"
  [[ $(vrx_fprs "$out.tmp") == "$fpr" ]] || die "exported keyring for $fpr holds other keys"
  mv -f "$out.tmp" "$out"
}

# vrx_gpgv <keyring.gpg> <signature> [<data>]: gpgv with exactly that keyring; prints the gpgv lines
vrx_gpgv() {
  gpgv --keyring "$1" -- "${@:2}" 2>&1
}

# vrx_sha256 <file>
vrx_sha256() { sha256sum -- "$1" | cut -d' ' -f1; }

# vrx_gzip_n <in> <out.gz>: reproducible gzip (no name, no time stamp)
vrx_gzip_n() { gzip -9n < "$1" > "$2"; }

# vrx_nspawn <chroot> <bind-spec...> -- <command...>: run a command in the build chroot with no network at all
# (--private-network: loopback only), no host time zone/resolv.conf, not registered with machined. Bind specs are
# passed through (--bind-ro=SRC:DST / --bind=SRC:DST). The machine name carries the slot prefix (VRX_ISO_MACHINE).
vrx_nspawn() {
  local root=$1; shift
  local binds=()
  while (($#)) && [[ $1 != -- ]]; do binds+=("$1"); shift; done
  [[ ${1:-} == -- ]] && shift
  [[ -x $root/usr/bin/env ]] || die "build chroot $root missing (build-iso.sh --make-chroot)"
  systemd-nspawn -q --register=no --private-network --timezone=off --resolv-conf=off \
    -M "${VRX_ISO_MACHINE:-w6-p14}" -D "$root" "${binds[@]}" --setenv=LC_ALL=C.UTF-8 -- "$@"
}

# vrx_kv_get <file> <key>: value of a "Key: value" line (deb822 / Release style), first match
vrx_kv_get() { awk -v k="$2" -F': ' '$1 == k { sub(/^[^:]*: /, ""); print; exit }' "$1"; }
