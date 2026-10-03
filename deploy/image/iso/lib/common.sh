# shellcheck shell=bash
# deploy/image/iso/lib/common.sh — helpers shared by build-iso.sh and tests/run.sh (P14). Sourced, never executed.
# Nothing here touches the build host outside the directories it is given: every write goes below a directory that
# carries the `.ngfw-iso-owned` marker (ngfw_rm_rf, ngfw_owned_dir), GPG runs with a private GNUPGHOME, and container work
# runs in systemd-nspawn with --private-network.

NGFW_ISO_LOG_PREFIX=${NGFW_ISO_LOG_PREFIX:-build-iso}

log() { printf '[%s %s] %s\n' "$NGFW_ISO_LOG_PREFIX" "$(date +%H:%M:%S)" "$*" >&2; }
warn() { printf '[%s] WARNING: %s\n' "$NGFW_ISO_LOG_PREFIX" "$*" >&2; }
die() { printf '[%s] ERROR: %s\n' "$NGFW_ISO_LOG_PREFIX" "$*" >&2; exit 1; }

# ngfw_guard_path <path>: refuse the host's system trees and anything that is not below an allowed base. The bases are
# the worktree's .scratch (default) plus NGFW_ISO_ALLOW_DIRS (colon separated, absolute) for builds elsewhere.
ngfw_guard_path() {
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
  for base in ${NGFW_ISO_SCRATCH:-} ${NGFW_ISO_ALLOW_DIRS:-}; do
    [[ -n $base ]] || continue
    base=$(realpath -m -- "$base")
    if [[ $real == "$base" || $real == "$base"/* ]]; then return 0; fi
  done
  die "refused path '$p': not below the scratch dir (${NGFW_ISO_SCRATCH:-unset}) or NGFW_ISO_ALLOW_DIRS"
}

# ngfw_owned_dir <dir>: create (if needed) a work directory and mark it as ours so that ngfw_rm_rf may later empty it
ngfw_owned_dir() {
  ngfw_guard_path "$1"
  mkdir -p -- "$1"
  : > "$1/.ngfw-iso-owned"
}

# ngfw_rm_rf <dir>: delete a directory tree only if it is guarded AND carries our marker (never a foreign directory)
ngfw_rm_rf() {
  local d=$1
  [[ -e $d ]] || return 0
  ngfw_guard_path "$d"
  [[ -f $d/.ngfw-iso-owned ]] || die "refusing to delete $d: no .ngfw-iso-owned marker"
  rm -rf --one-file-system -- "$d"
}

# ngfw_fprs <keyring-or-armored-key>: the primary key fingerprints in it, one per line (upper case)
ngfw_fprs() {
  gpg --batch --no-default-keyring --show-keys --with-colons -- "$1" 2>/dev/null |
    awk -F: '$1 == "pub" { want = 1; next } $1 == "fpr" && want { print toupper($10); want = 0 }'
}

# ngfw_pinned_keyring <source keyring/armored key> <fingerprint> <out.gpg>: write a binary keyring holding exactly the
# key with that fingerprint (trust comes from the pin, never from the file's origin — D-202)
ngfw_pinned_keyring() {
  local src=$1 fpr=${2^^} out=$3 gh
  [[ $fpr =~ ^[0-9A-F]{40}$ ]] || die "fingerprint '$2' is not 40 hex digits"
  [[ -f $src ]] || die "keyring $src missing"
  ngfw_fprs "$src" | grep -qx "$fpr" || die "$src does not carry the pinned key $fpr"
  gh=$(mktemp -d "${NGFW_ISO_TMP:?}/gnupg.XXXXXX")
  gpg --batch --quiet --homedir "$gh" --import -- "$src" 2>/dev/null || die "cannot import $src"
  gpg --batch --quiet --homedir "$gh" --export "$fpr" > "$out.tmp" || die "cannot export $fpr"
  rm -rf -- "$gh"
  [[ -s $out.tmp ]] || die "export of $fpr from $src is empty"
  [[ $(ngfw_fprs "$out.tmp") == "$fpr" ]] || die "exported keyring for $fpr holds other keys"
  mv -f "$out.tmp" "$out"
}

# ngfw_gpgv <keyring.gpg> <signature> [<data>]: gpgv with exactly that keyring; prints the gpgv lines
ngfw_gpgv() {
  gpgv --keyring "$1" -- "${@:2}" 2>&1
}

# ngfw_sha256 <file>
ngfw_sha256() { sha256sum -- "$1" | cut -d' ' -f1; }

# ngfw_gzip_n <in> <out.gz>: reproducible gzip (no name, no time stamp)
ngfw_gzip_n() { gzip -9n < "$1" > "$2"; }

# ngfw_nspawn <chroot> <bind-spec...> -- <command...>: run a command in the build chroot with no network at all
# (--private-network: loopback only), no host time zone/resolv.conf, not registered with machined. Bind specs are
# passed through (--bind-ro=SRC:DST / --bind=SRC:DST). The machine name carries the slot prefix (NGFW_ISO_MACHINE).
ngfw_nspawn() {
  local root=$1; shift
  local binds=()
  while (($#)) && [[ $1 != -- ]]; do binds+=("$1"); shift; done
  [[ ${1:-} == -- ]] && shift
  [[ -x $root/usr/bin/env ]] || die "build chroot $root missing (build-iso.sh --make-chroot)"
  systemd-nspawn -q --register=no --private-network --timezone=off --resolv-conf=off \
    -M "${NGFW_ISO_MACHINE:-w6-p14}" -D "$root" "${binds[@]}" --setenv=LC_ALL=C.UTF-8 -- "$@"
}

# ngfw_kv_get <file> <key>: value of a "Key: value" line (deb822 / Release style), first match
ngfw_kv_get() { awk -v k="$2" -F': ' '$1 == k { sub(/^[^:]*: /, ""); print; exit }' "$1"; }
