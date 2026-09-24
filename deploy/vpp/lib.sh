# shellcheck shell=bash
# deploy/vpp/lib.sh — shared, unit-tested helpers for build.sh / verify.sh (tests: deploy/vpp/tests/run.sh).
# Sourced, never executed. No function here installs anything or touches anything outside the paths it is given.

# Trees nobody in this pipeline may ever write to or delete, whatever the arguments say (D-012, envelope).
VRX_PROTECTED_TREES=(/root/vpp /etc/vpp /usr /var/lib/dpkg /boot /root/ngfw)
VRX_OWNED_MARKER=.vrx-owned

# ------------------------------------------------------------------ VERSION: data, never sourced (review L2)
# vrx_parse_version <file>: validates every line against a strict per-key format and sets the VPP_* globals via
# printf -v (no evaluation). Returns 1 (with messages on stderr) at the first problem, before any value is used.
vrx_parse_version() {
  local file=$1 line k v pat n=0
  local pkgs='^[a-z0-9][a-z0-9.+-]*( [a-z0-9][a-z0-9.+-]*)*$'
  declare -A seen=()
  [[ -f $file ]] || { echo "VERSION: $file not found" >&2; return 1; }
  while IFS= read -r line || [[ -n $line ]]; do
    n=$((n + 1))
    [[ $line =~ ^[[:space:]]*(#.*)?$ ]] && continue
    if [[ ! $line =~ ^(VPP_[A-Z_]+)=(.*)$ ]]; then echo "VERSION:$n: not a KEY=VALUE line: $line" >&2; return 1; fi
    k=${BASH_REMATCH[1]}; v=${BASH_REMATCH[2]}
    if [[ $v =~ ^\"(.*)\"$ ]]; then v=${BASH_REMATCH[1]}; fi
    case $k in
      VPP_UPSTREAM_URL) pat='^https://[A-Za-z0-9.-]+(/[A-Za-z0-9._/-]*)?$' ;;
      VPP_BRANCH) pat='^stable/[0-9]{4}$' ;;
      VPP_TAG) pat='^v[0-9]{2}\.[0-9]{2}(\.[0-9]+)?$' ;;
      VPP_TAG_OBJECT|VPP_COMMIT) pat='^[0-9a-f]{40}$' ;;
      VPP_DEB_VERSION) pat='^[0-9]{2}\.[0-9]{2}(\.[0-9]+)?-release$' ;;
      VPP_LOCAL_REV) pat='^[1-9][0-9]{0,3}$' ;;
      VPP_PACKAGES|VPP_PACKAGES_INSTALLED|VPP_PACKAGES_SHIP) pat=$pkgs ;;
      *) echo "VERSION:$n: unknown key $k" >&2; return 1 ;;
    esac
    if [[ ! $v =~ $pat ]]; then echo "VERSION:$n: $k='$v' does not match $pat" >&2; return 1; fi
    if [[ -n ${seen[$k]:-} ]]; then echo "VERSION:$n: $k defined twice" >&2; return 1; fi
    seen[$k]=1
    printf -v "$k" '%s' "$v"
  done <"$file"
  for k in VPP_UPSTREAM_URL VPP_BRANCH VPP_TAG VPP_TAG_OBJECT VPP_COMMIT VPP_DEB_VERSION VPP_LOCAL_REV \
           VPP_PACKAGES VPP_PACKAGES_INSTALLED VPP_PACKAGES_SHIP; do
    [[ -n ${seen[$k]:-} ]] || { echo "VERSION: $k missing" >&2; return 1; }
  done
  local tv=${VPP_TAG#v}
  [[ $VPP_DEB_VERSION == "$tv-release" ]] || { echo "VERSION: VPP_DEB_VERSION=$VPP_DEB_VERSION but $VPP_TAG builds $tv-release" >&2; return 1; }
  [[ $VPP_BRANCH == "stable/${tv:0:2}${tv:3:2}" ]] || { echo "VERSION: VPP_BRANCH=$VPP_BRANCH does not match $VPP_TAG" >&2; return 1; }
  local p
  for p in $VPP_PACKAGES_INSTALLED $VPP_PACKAGES_SHIP; do
    [[ " $VPP_PACKAGES " == *" $p "* ]] || { echo "VERSION: $p is not in VPP_PACKAGES" >&2; return 1; }
  done
  return 0
}

# vrx_local_version <base> <rev> <n-applied-patches>: D-089 — patched builds are <base>+vrx<rev>
vrx_local_version() { if (($3 > 0)); then printf '%s+vrx%s\n' "$1" "$2"; else printf '%s\n' "$1"; fi; }

# ------------------------------------------------------------------ path guards (review M2)
vrx_realpath() { realpath -m -- "$1"; }
# vrx_is_under <path> <root>: path (resolved) equals root or lies below it
vrx_is_under() {
  local p r; p=$(vrx_realpath "$1"); r=$(vrx_realpath "$2")
  [[ $p == "$r" || $p == "$r"/* ]]
}
# vrx_is_protected <path>: path is /, $HOME, or equal to/inside/an ancestor of a protected tree
vrx_is_protected() {
  local p t; p=$(vrx_realpath "$1")
  [[ $p == / || $p == "$(vrx_realpath "${HOME:-/root}")" ]] && return 0
  for t in "${VRX_PROTECTED_TREES[@]}"; do
    [[ $p == "$t" || $p == "$t"/* || $t == "$p"/* ]] && return 0
  done
  return 1
}
# vrx_guard_dir <path> <allowed-root> <label>: prints the resolved path, or fails when it is outside the allowed root
vrx_guard_dir() {
  local p; p=$(vrx_realpath "$1")
  if vrx_is_protected "$p"; then echo "$3: $p is a protected path — refusing" >&2; return 1; fi
  if ! vrx_is_under "$p" "$2"; then echo "$3: $p is outside $(vrx_realpath "$2") — refusing" >&2; return 1; fi
  printf '%s\n' "$p"
}
# vrx_rm_rf <path> <owned-root>: rm -rf only strictly below a root that carries our ownership marker
vrx_rm_rf() {
  local p r; p=$(vrx_realpath "$1"); r=$(vrx_realpath "$2")
  [[ -e $p ]] || return 0
  if vrx_is_protected "$p" || [[ $p == "$r" || $p != "$r"/* ]]; then echo "rm -rf $p refused: not strictly below $r" >&2; return 1; fi
  [[ -f $r/$VRX_OWNED_MARKER ]] || { echo "rm -rf $p refused: $r has no $VRX_OWNED_MARKER marker" >&2; return 1; }
  rm -rf -- "$p"
}
# vrx_out_dir_is_ours <dir>: empty/nonexistent, or contains only artefacts build.sh writes
vrx_out_dir_is_ours() {
  local f
  [[ -d $1 ]] || return 0
  for f in "$1"/* "$1"/.[!.]*; do
    [[ -e $f ]] || continue
    case ${f##*/} in *.deb|*.buildinfo|*.changes|SHA256SUMS|manifest.json) ;; *) echo "out dir $1 contains foreign file ${f##*/}" >&2; return 1 ;; esac
  done
}

# ------------------------------------------------------------------ patches (review M1): strict, no fuzz
# vrx_apply_patch <src-git-tree> <patch-file> [-pN]: `git apply --check` then `git apply` (exact context, no fuzz)
vrx_apply_patch() {
  local src=$1 f p=${3:--p1}
  f=$(vrx_realpath "$2")
  git -C "$src" apply --check "$p" -- "$f" || { echo "patch ${2##*/} does not apply exactly (git apply --check)" >&2; return 1; }
  git -C "$src" apply "$p" -- "$f"
}
# vrx_series <series-file>: prints "name -pN" per entry (comments/blank ignored)
vrx_series() {
  local line name popt
  while IFS= read -r line || [[ -n $line ]]; do
    line=${line%%#*}; read -r name popt _ <<<"$line" || true
    [[ -n ${name:-} ]] || continue
    printf '%s %s\n' "$name" "${popt:--p1}"
  done <"$1"
}
# vrx_patch_status <patch-file>: the value of its "Status:" header
vrx_patch_status() { sed -nE 's/^Status: ([a-z]+).*/\1/p' "$1" | head -1; }

# vrx_write_version_script <src> <version>: replace src/scripts/version so cmake/debian use the local version
vrx_write_version_script() {
  cat >"$1/src/scripts/version" <<EOF
#!/usr/bin/env bash
# generated by deploy/vpp/build.sh (D-089): local version of a patched build — the upstream script is replaced
case "\${1:-}" in
  rpm-version) echo "${2%%-*}" ;;
  rpm-release) echo "${2#*-}" ;;
  *) echo "$2" ;;
esac
EOF
  chmod 0755 "$1/src/scripts/version"
}

# ------------------------------------------------------------------ build dependencies (review M3)
# vrx_apt_missing <pkg...>: prints the packages apt would install; returns 2 (message on stderr) when apt itself fails
vrx_apt_missing() {
  local out rc=0
  out=$(apt-get install -s -qq "$@" 2>&1) || rc=$?
  if ((rc != 0)); then
    printf 'apt-get install -s failed (rc=%s): %s\n' "$rc" "$(tail -n 5 <<<"$out" | tr '\n' ' ')" >&2
    return 2
  fi
  awk '/^Inst /{print $2}' <<<"$out" | sort -u | tr '\n' ' '
}

# ------------------------------------------------------------------ Python build deps (review H1)
# vrx_pydeps_parse <lock>: prints "name version sha256 file url" per entry; fails on any malformed line
vrx_pydeps_parse() {
  local line n=0 re='^([A-Za-z0-9._-]+)==([A-Za-z0-9.+!-]+) --hash=sha256:([0-9a-f]{64})[[:space:]]+#[[:space:]]+([A-Za-z0-9._+-]+\.(whl|tar\.gz))[[:space:]]+(https://files\.pythonhosted\.org/[A-Za-z0-9._/+-]+)$'
  while IFS= read -r line || [[ -n $line ]]; do
    n=$((n + 1))
    [[ $line =~ ^[[:space:]]*(#.*)?$ ]] && continue
    if [[ ! $line =~ $re ]]; then echo "pydeps.lock:$n: malformed (want: name==ver --hash=sha256:<64hex>  # <file> <https url>)" >&2; return 1; fi
    [[ ${BASH_REMATCH[6]##*/} == "${BASH_REMATCH[4]}" ]] || { echo "pydeps.lock:$n: url does not end in ${BASH_REMATCH[4]}" >&2; return 1; }
    printf '%s %s %s %s %s\n' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}" "${BASH_REMATCH[4]}" "${BASH_REMATCH[6]}"
  done <"$1"
}
vrx_sha256() { sha256sum -- "$1" | cut -d' ' -f1; }
# vrx_pydeps_prepare <lock> <wheelhouse> <owned-root> <offline 0|1>: the wheelhouse ends up holding exactly the locked
# files, each sha256-verified; missing ones are fetched over https (never from /root/vpp or ~/Downloads) unless offline.
# Prints "fetched|cached <file>" per entry.
vrx_pydeps_prepare() {
  local lock=$1 wh=$2 root=$3 offline=$4 entries sha file url f keep
  entries=$(vrx_pydeps_parse "$lock") || return 1
  mkdir -p "$wh"
  for f in "$wh"/* "$wh"/.[!.]*; do                           # drop anything not in the lock (stale/foreign files)
    [[ -e $f ]] || continue
    keep=0
    while read -r _ _ _ file _; do [[ ${f##*/} == "$file" ]] && keep=1; done <<<"$entries"
    ((keep)) || vrx_rm_rf "$f" "$root" || return 1
  done
  while read -r _ _ sha file url; do
    f=$wh/$file
    if [[ -f $f && $(vrx_sha256 "$f") == "$sha" ]]; then echo "cached $file"; continue; fi
    [[ -e $f ]] && { vrx_rm_rf "$f" "$root" || return 1; }
    if ((offline)); then echo "$file missing/mismatched in $wh and offline — cannot fetch" >&2; return 1; fi
    curl -fsSL --proto '=https' --retry 3 --connect-timeout 15 --max-time 300 -o "$f.part" "$url" \
      || { rm -f -- "$f.part"; echo "download failed: $url" >&2; return 1; }
    if [[ $(vrx_sha256 "$f.part") != "$sha" ]]; then rm -f -- "$f.part"; echo "sha256 mismatch for $url" >&2; return 1; fi
    mv -- "$f.part" "$f"; echo "fetched $file"
  done <<<"$entries"
}
# vrx_pydeps_verify <lock> <wheelhouse>: every locked file present with the locked hash, nothing else
vrx_pydeps_verify() {
  local entries sha file n=0 f
  entries=$(vrx_pydeps_parse "$1") || return 1
  while read -r _ _ sha file _; do
    [[ -f $2/$file && $(vrx_sha256 "$2/$file") == "$sha" ]] || { echo "pydeps: $file missing or hash mismatch in $2" >&2; return 1; }
    n=$((n + 1))
  done <<<"$entries"
  for f in "$2"/*; do [[ -e $f ]] && n=$((n - 1)); done
  ((n == 0)) || { echo "pydeps: $2 holds files that are not in the lock" >&2; return 1; }
}
