# shellcheck shell=bash
# deploy/image/iso/lib/pool.sh — the offline pool of the ISO (P14). Sourced by build-iso.sh after lib/common.sh.
#
# Resolution runs the BUILD HOST's apt-get, fully isolated: APT_CONFIG points at a file that sets Dir to a private
# root under the work dir, so no host apt configuration, list, cache, lock or dpkg database is read or written.
# Its dpkg status is the ISO's base layer (var/lib/dpkg/status of the squashfs the installer lays down), so apt
# resolves exactly what the target is missing. This is the only networked step (mirror, deb.frrouting.org); the
# NGFW repository is a local file: source. Trust: the Ubuntu archive key and the FRR key are pinned by fingerprint,
# the NGFW key by --ngfw-key-fpr (D-202: never taken from the repo itself).

UBUNTU_ARCHIVE_FPRS=(F6ECB3762474EDA9D21B7022871920D1991BC93C)   # Ubuntu Archive Automatic Signing Key (2018)
FRR_SIGNER_FPR=4A56C7738BB3F81595A805D2A832769908F13ED1              # docs/install/bare-metal.md (pinned there too)
POOL_SUITE=resolute

# pool_apt_root <aptroot> <base-status> <ngfw-repo> <ngfw-keyring> <mirror> <frr-uri> <ubuntu-keyring> <frr-keyring>
pool_apt_root() {
  local A=$1 status=$2 repo=$3 ngfwk=$4 mirror=$5 frr=$6 ubk=$7 frrk=$8 f
  ngfw_owned_dir "$A"
  mkdir -p "$A"/etc/apt/{apt.conf.d,sources.list.d,preferences.d,trusted.gpg.d} "$A"/var/lib/apt/lists/partial \
    "$A"/var/cache/apt/archives/partial "$A"/var/lib/dpkg "$A"/var/log/apt "$A"/keys
  cp -f -- "$status" "$A/var/lib/dpkg/status"
  for f in "${UBUNTU_ARCHIVE_FPRS[@]}"; do ngfw_fprs "$ubk" | grep -qx "$f" || die "$ubk lacks the Ubuntu archive key $f"; done
  ngfw_pinned_keyring "$ubk" "${UBUNTU_ARCHIVE_FPRS[0]}" "$A/keys/ubuntu.gpg"
  ngfw_pinned_keyring "$frrk" "$FRR_SIGNER_FPR" "$A/keys/frr.gpg"
  cp -f -- "$ngfwk" "$A/keys/ngfw.gpg"
  cat > "$A/apt.conf" <<EOF
Dir "$A/";
Dir::State::status "$A/var/lib/dpkg/status";
APT::Architecture "amd64";
APT::Architectures { "amd64"; };
Acquire::Languages "none";
APT::Install-Recommends "true";
APT::Sandbox::User "root";
EOF
  {
    for p in "$POOL_SUITE" "$POOL_SUITE-updates" "$POOL_SUITE-security"; do
      echo "deb [signed-by=$A/keys/ubuntu.gpg] $mirror $p main restricted universe multiverse"
    done
    echo "deb [signed-by=$A/keys/frr.gpg] $frr $POOL_SUITE frr-stable"
    echo "deb [signed-by=$A/keys/ngfw.gpg] file:$repo $POOL_SUITE main"
  } > "$A/etc/apt/sources.list"
}

pool_apt() { APT_CONFIG="$1/apt.conf" apt-get -q "${@:2}"; }

# pool_inst_lines <sim-output>: "<package> <version>" for every Inst line of an `apt-get -s` run, sorted
pool_inst_lines() {
  awk '$1 == "Inst" { v = ($3 ~ /^\[/) ? $4 : $3; sub(/^\(/, "", v); print $2, v }' "$1" | LC_ALL=C sort -u
}

# pool_resolve <aptroot> <packages...>: writes <aptroot>/wanted.txt ("pkg version"), the full closure on the base
pool_resolve() {
  local A=$1; shift
  pool_apt "$A" update > "$A/update.log" 2>&1 || { tail -20 "$A/update.log" >&2; die "apt-get update failed (mirror/FRR reachable?)"; }
  grep -E '^(W|E):' "$A/update.log" | grep -v 'unsandboxed' >&2 || true
  pool_apt "$A" -s install "$@" > "$A/resolve-sim.txt" 2>&1 || { tail -20 "$A/resolve-sim.txt" >&2; die "the closure does not resolve"; }
  pool_inst_lines "$A/resolve-sim.txt" > "$A/wanted.txt"
  [[ -s $A/wanted.txt ]] || die "empty closure"
}

# pool_fetch <aptroot> <wanted.txt> <dest-dir>: apt-get download each pkg=version (hashes checked by apt against the
# signed indexes), then record pool.manifest: package version arch sha256 size origin file
pool_fetch() {
  local A=$1 wanted=$2 dest=$3 specs=() p v
  mkdir -p "$dest"
  while read -r p v; do specs+=("$p=$v"); done < "$wanted"
  ( cd "$dest" && APT_CONFIG="$A/apt.conf" apt-get -q download "${specs[@]}" ) > "$A/download.log" 2>&1 ||
    { tail -20 "$A/download.log" >&2; die "download failed"; }
}

# pool_manifest <aptroot> <deb-dir> <out>: one line per .deb, sorted; the origin comes from apt's policy line
pool_manifest() {
  local A=$1 dir=$2 out=$3 d pkg ver arch origin
  : > "$out.tmp"
  for d in "$dir"/*.deb; do
    pkg=$(dpkg-deb -f "$d" Package); ver=$(dpkg-deb -f "$d" Version); arch=$(dpkg-deb -f "$d" Architecture)
    origin=$(APT_CONFIG="$A/apt.conf" apt-cache madison "$pkg" 2>/dev/null |
      awk -F' [|] ' -v v="$ver" '$2 == v { split($3, a, " "); print a[1]; exit }')
    case $origin in
      file:*) origin=ngfw ;; *frrouting*) origin=frr ;; '') origin=unknown ;; *) origin=ubuntu ;;
    esac
    printf '%s %s %s %s %s %s %s\n' "$pkg" "$ver" "$arch" "$(ngfw_sha256 "$d")" "$(stat -c %s "$d")" "$origin" "$(basename "$d")" >> "$out.tmp"
  done
  LC_ALL=C sort "$out.tmp" > "$out"; rm -f "$out.tmp"
}

# pool_repo <deb-dir> <repo-dir> <epoch>: Debian layout + Packages/Release, reproducible (sorted, gzip -n, fixed Date)
pool_repo() {
  local src=$1 R=$2 epoch=$3 d pkg dir b
  mkdir -p "$R/dists/$POOL_SUITE/main/binary-amd64"
  for d in "$src"/*.deb; do
    pkg=$(dpkg-deb -f "$d" Package); b=${pkg:0:1}; [[ $pkg == lib?* ]] && b=${pkg:0:4}
    dir=$R/pool/main/$b/$pkg; mkdir -p "$dir"; cp -f -- "$d" "$dir/"
  done
  ( cd "$R" && dpkg-scanpackages --multiversion pool /dev/null 2>/dev/null ) > "$R/dists/$POOL_SUITE/main/binary-amd64/Packages"
  ngfw_gzip_n "$R/dists/$POOL_SUITE/main/binary-amd64/Packages" "$R/dists/$POOL_SUITE/main/binary-amd64/Packages.gz"
  printf 'Archive: %s\nComponent: main\nOrigin: NGFW\nLabel: NGFW ISO pool\nArchitecture: amd64\n' "$POOL_SUITE" \
    > "$R/dists/$POOL_SUITE/main/binary-amd64/Release"
  ( cd "$R/dists/$POOL_SUITE" && {
      printf 'Origin: NGFW\nLabel: NGFW ISO pool\nSuite: %s\nCodename: %s\nDate: %s\nArchitectures: amd64\nComponents: main\n' \
        "$POOL_SUITE" "$POOL_SUITE" "$(LC_ALL=C date -u -d "@$epoch" '+%a, %d %b %Y %H:%M:%S UTC')"
      printf 'Description: offline pool of the NGFW installer ISO (ngfw-meta and its dependency closure)\nSHA256:\n'
      for f in main/binary-amd64/Packages main/binary-amd64/Packages.gz main/binary-amd64/Release; do
        printf ' %s %s %s\n' "$(ngfw_sha256 "$f")" "$(stat -c %s "$f")" "$f"
      done
    } > Release )
}

# pool_sign <repo-dir> <gnupg-home> <key-fpr>: InRelease + Release.gpg (the only non-reproducible bytes of the pool)
pool_sign() {
  local R=$1/dists/$POOL_SUITE gh=$2 fpr=$3
  rm -f "$R/InRelease" "$R/Release.gpg"
  gpg --batch --quiet --homedir "$gh" --local-user "$fpr" --clearsign -o "$R/InRelease" "$R/Release"
  gpg --batch --quiet --homedir "$gh" --local-user "$fpr" -abs -o "$R/Release.gpg" "$R/Release"
}
