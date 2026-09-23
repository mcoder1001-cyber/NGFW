#!/usr/bin/env bash
# deploy/vpp/build.sh — reproducible VPP .deb build from the pinned upstream tag + our patch series (WBS D0.2, F-vpp-debs).
#
#   deploy/vpp/build.sh [options]
#
# Steps: static checks (verify.sh) → clone/fetch into a scratch dir (never builds inside /root/vpp) → verify tag object +
# commit hash → pristine checkout → apply patches/series (+ optional patches) uncommitted → verify the Debian version the
# tree will produce → build-dependency check (report only, never installs) → `make pkg-deb` (upstream flags, capped
# parallelism) → collect .deb + SHA256SUMS + manifest.json into the output dir → check package names/versions vs VERSION.
#
# Options:
#   --build-dir DIR        scratch dir (default deploy/vpp/.build, git-ignored); source tree at DIR/src/vpp
#   --out DIR              artefact dir (default <build-dir>/out/<VPP_DEB_VERSION>[+trace-core]); emptied first
#   --reference DIR        local VPP clone used read-only as object/download cache (default /root/vpp when it exists)
#   --offline              do not contact VPP_UPSTREAM_URL; clone from --reference only (commit hash is still verified)
#   --jobs N               parallel jobs, 1..8 (default 8; shared host rule). The whole build also runs under taskset on N CPUs
#   --trace-plugins MODE   devtools (default, upstream packaging: tracedump/tracenode in vpp-plugin-devtools) or
#                          core (applies patches/optional/trace-plugins-core.patch → they ship in vpp-plugin-core; V18/D-077)
#   --prepare-only         stop after the dependency check (clone, verify, patch, version check) — no compile
#   --strict-deps          fail when a build dependency from VPP's DEB_DEPENDS is missing (default: report and continue)
#   -h, --help
#
# Nothing here installs packages, touches /etc/vpp, the running VPP or /root/vpp (read-only reference). Install
# procedure for the manager after handover: deploy/vpp/README.md.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
BUILD_DIR="$HERE/.build"
OUT_DIR=""
REFERENCE=/root/vpp
OFFLINE=0
JOBS=8
TRACE_PLUGINS=devtools
PREPARE_ONLY=0
STRICT_DEPS=0
MAX_JOBS=8

usage() { sed -n '2,/^set -euo pipefail/{/^set -euo pipefail/!p}' "$0" | sed -E 's/^# ?//'; }
log() { printf '[build.sh %s] %s\n' "$(date +%H:%M:%S)" "$*"; }
die() { printf '[build.sh] ERROR: %s\n' "$*" >&2; exit 1; }

while (($#)); do
  case "$1" in
    --build-dir) BUILD_DIR=${2:?}; shift ;;
    --out) OUT_DIR=${2:?}; shift ;;
    --reference) REFERENCE=${2:?}; shift ;;
    --offline) OFFLINE=1 ;;
    --jobs) JOBS=${2:?}; shift ;;
    --trace-plugins) TRACE_PLUGINS=${2:?}; shift ;;
    --trace-plugins=*) TRACE_PLUGINS=${1#*=} ;;
    --prepare-only) PREPARE_ONLY=1 ;;
    --strict-deps) STRICT_DEPS=1 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument '$1' (try --help)" ;;
  esac
  shift
done

if ! [[ $JOBS =~ ^[0-9]+$ ]] || ((JOBS < 1 || JOBS > MAX_JOBS)); then die "--jobs must be 1..$MAX_JOBS (shared host)"; fi
case $TRACE_PLUGINS in devtools|core) ;; *) die "--trace-plugins must be devtools or core" ;; esac

# ------------------------------------------------------------------ 0. static checks + pinned inputs
"$HERE/verify.sh" >/dev/null || die "deploy/vpp/verify.sh failed — fix VERSION/series first (run it for details)"
# shellcheck source=/dev/null
source <(grep -E '^VPP_[A-Z_]+=' "$HERE/VERSION")

SRC="$BUILD_DIR/src/vpp"
SUFFIX=""; [[ $TRACE_PLUGINS == core ]] && SUFFIX="+trace-core"
OUT_DIR="${OUT_DIR:-$BUILD_DIR/out/${VPP_DEB_VERSION}${SUFFIX}}"
mkdir -p "$BUILD_DIR/src"

REAL_REF=""
if [[ -n $REFERENCE && -d $REFERENCE/.git ]]; then REAL_REF="$(cd "$REFERENCE" && pwd -P)"; fi
if [[ -n $REAL_REF && "$(cd "$BUILD_DIR" && pwd -P)/" == "$REAL_REF"/* ]]; then
  die "--build-dir must not be inside the reference tree $REAL_REF (read-only)"
fi

log "pinned: $VPP_UPSTREAM_URL $VPP_TAG ($VPP_COMMIT) → expect $VPP_PACKAGES version $VPP_DEB_VERSION"
log "build dir: $BUILD_DIR  out: $OUT_DIR  jobs: $JOBS  trace-plugins: $TRACE_PLUGINS  reference: ${REAL_REF:-<none>}"

# ------------------------------------------------------------------ 1. source: clone or fetch (never inside the reference)
have_commit() { git -C "$SRC" cat-file -e "$VPP_COMMIT^{commit}" 2>/dev/null && git -C "$SRC" rev-parse -q --verify "refs/tags/$VPP_TAG" >/dev/null; }
if [[ ! -d $SRC/.git ]]; then
  rm -rf "$SRC"
  if ((OFFLINE == 0)); then
    log "cloning $VPP_UPSTREAM_URL${REAL_REF:+ (objects borrowed from $REAL_REF, then dissociated)}"
    if [[ -n $REAL_REF ]]; then
      git clone --no-checkout --reference-if-able "$REAL_REF" --dissociate "$VPP_UPSTREAM_URL" "$SRC" \
        || { rm -rf "$SRC"; log "upstream clone failed — falling back to a read-only clone of $REAL_REF"; }
    else
      git clone --no-checkout "$VPP_UPSTREAM_URL" "$SRC"
    fi
  fi
  if [[ ! -d $SRC/.git ]]; then
    [[ -n $REAL_REF ]] || die "no source: upstream unreachable/--offline and no --reference"
    git clone --no-checkout --no-hardlinks "$REAL_REF" "$SRC"
  fi
fi
if ! have_commit; then
  ((OFFLINE == 0)) || die "$VPP_COMMIT / $VPP_TAG not in $SRC and --offline given"
  log "fetching $VPP_TAG from $VPP_UPSTREAM_URL"
  git -C "$SRC" fetch --no-tags "$VPP_UPSTREAM_URL" "refs/tags/$VPP_TAG:refs/tags/$VPP_TAG"
fi

# ------------------------------------------------------------------ 2. verify the pin (tag object and commit)
tag_obj=$(git -C "$SRC" rev-parse "refs/tags/$VPP_TAG")
tag_commit=$(git -C "$SRC" rev-parse "refs/tags/$VPP_TAG^{commit}")
[[ $tag_obj == "$VPP_TAG_OBJECT" ]] || die "tag $VPP_TAG is $tag_obj, VERSION pins tag object $VPP_TAG_OBJECT (moved tag?)"
[[ $tag_commit == "$VPP_COMMIT" ]] || die "tag $VPP_TAG points to $tag_commit, VERSION pins $VPP_COMMIT"
[[ $(git -C "$SRC" cat-file -t "$tag_obj") == tag ]] || die "$VPP_TAG must be an annotated tag (src/scripts/version uses git describe)"
log "verified: $VPP_TAG = tag object $tag_obj → commit $tag_commit"

# ------------------------------------------------------------------ 3. pristine checkout (keeps ccache + download cache)
git -C "$SRC" checkout -q -f --detach "$VPP_COMMIT"
git -C "$SRC" clean -q -ffdx -e /build-root/.ccache -e /build/external/downloads
[[ -z $(git -C "$SRC" status --porcelain) ]] || die "source tree not pristine after checkout/clean"
[[ $(git -C "$SRC" rev-parse HEAD) == "$VPP_COMMIT" ]] || die "HEAD is not $VPP_COMMIT"
log "checked out $(git -C "$SRC" describe --long --match 'v*') (pristine)"

# ------------------------------------------------------------------ 4. apply the patch series (uncommitted, quilt order)
declare -a PATCH_NAMES=() PATCH_SHAS=() PATCH_KINDS=()
apply_patch() {  # apply_patch <file relative to patches/> <pN> <kind>
  local f="$HERE/patches/$1" p=$2
  [[ -f $f ]] || die "series lists $1 but $f does not exist"
  patch -d "$SRC" "$p" --forward --batch --dry-run --quiet <"$f" >/dev/null || die "patch $1 does not apply cleanly on $VPP_TAG"
  patch -d "$SRC" "$p" --forward --batch --quiet --no-backup-if-mismatch <"$f"
  PATCH_NAMES+=("$1"); PATCH_SHAS+=("$(sha256sum "$f" | cut -d' ' -f1)"); PATCH_KINDS+=("$3")
  log "applied $1 ($p)"
}
while IFS= read -r line || [[ -n $line ]]; do
  line=${line%%#*}; read -r name popt _ <<<"$line" || true
  [[ -n ${name:-} ]] || continue
  kind=product; [[ $name == *DEMO* ]] && kind=demo
  apply_patch "$name" "${popt:--p1}" "$kind"
done <"$HERE/patches/series"
[[ $TRACE_PLUGINS == core ]] && apply_patch optional/trace-plugins-core.patch -p1 optional
git -C "$SRC" status --porcelain | sed 's/^/    /'

# ------------------------------------------------------------------ 5. version the tree will produce (before any compile)
describe=$(git -C "$SRC" describe --long --match 'v*')
[[ $describe == "$VPP_TAG-0-g"* ]] || die "git describe = $describe, expected $VPP_TAG-0-g… (patches must stay uncommitted)"
tree_version=$("$SRC/src/scripts/version")
[[ $tree_version == "$VPP_DEB_VERSION" ]] || die "src/scripts/version = $tree_version, VERSION expects $VPP_DEB_VERSION"
log "tree version: $tree_version (git describe $describe)"

# ------------------------------------------------------------------ 6. build dependencies: report, never install
# DEB_DEPENDS is upstream's own list (Makefile, per OS release); `apt-get -s` is a simulation (no root, no changes) —
# the same check upstream's $(BR)/.deps.ok target runs.
# shellcheck disable=SC2016  # $(DEB_DEPENDS) is make syntax, expanded by make
deps=$(make -s -C "$SRC" -f Makefile -f <(printf 'vrx-print-deps:\n\t@echo $(DEB_DEPENDS)\n') vrx-print-deps 2>/dev/null) \
  || die "could not read DEB_DEPENDS from the VPP Makefile"
# shellcheck disable=SC2086  # word splitting of the package list is intended
missing=$(apt-get install -s -qq $deps 2>/dev/null | awk '/^Inst /{print $2}' | sort -u | tr '\n' ' ' || true)
if [[ -n ${missing// /} ]]; then
  log "MISSING build dependencies (not installed; install-dep is never run here): $missing"
  ((STRICT_DEPS == 0)) || die "missing build dependencies with --strict-deps"
else
  log "build dependencies: all $(wc -w <<<"$deps") DEB_DEPENDS entries satisfied"
fi
printf '%s\n' "${missing:-}" >"$BUILD_DIR/missing-deps.txt"

if ((PREPARE_ONLY)); then log "--prepare-only: stopping before the compile"; exit 0; fi

# ------------------------------------------------------------------ 7. make pkg-deb (upstream flags, capped parallelism)
# Download cache: seed from the reference tree's already-downloaded, checksum-verified tarballs (VPP re-verifies them).
if [[ -n $REAL_REF && -d $REAL_REF/build/external/downloads ]]; then
  mkdir -p "$SRC/build/external/downloads"
  cp -n "$REAL_REF"/build/external/downloads/* "$SRC/build/external/downloads/" 2>/dev/null || true
fi
ncpu=$(nproc --all)
cpus="$((ncpu - JOBS))-$((ncpu - 1))"; ((ncpu > JOBS)) || cpus="0-$((ncpu - 1))"
SOURCE_DATE_EPOCH=$(git -C "$SRC" log -1 --format=%ct "$VPP_COMMIT")
export SOURCE_DATE_EPOCH
unset MAKEFLAGS MFLAGS
MAKE_ARGS=(pkg-deb MAKE_PARALLEL_JOBS="$JOBS" JOBS="$JOBS")
log "make -C $SRC ${MAKE_ARGS[*]} (taskset -c $cpus, nice 10, SOURCE_DATE_EPOCH=$SOURCE_DATE_EPOCH)"
t0=$SECONDS
taskset -c "$cpus" nice -n 10 make -C "$SRC" "${MAKE_ARGS[@]}"
build_secs=$((SECONDS - t0))
log "make pkg-deb finished in $((build_secs / 60))m$((build_secs % 60))s"

# ------------------------------------------------------------------ 8. collect + check + SHA256SUMS + manifest.json
rm -rf "$OUT_DIR"; mkdir -p "$OUT_DIR"
shopt -s nullglob
debs=("$SRC"/build-root/*.deb)
((${#debs[@]})) || die "no .deb produced in $SRC/build-root"
cp "${debs[@]}" "$OUT_DIR/"
cp "$SRC"/build-root/*.buildinfo "$SRC"/build-root/*.changes "$OUT_DIR/" 2>/dev/null || true
(cd "$OUT_DIR" && sha256sum -- *.deb | sort -k2 >SHA256SUMS)

got=$(for d in "$OUT_DIR"/*.deb; do dpkg-deb -f "$d" Package; done | sort | tr '\n' ' ')
want=$(tr ' ' '\n' <<<"$VPP_PACKAGES" | sort | tr '\n' ' ')
[[ $got == "$want" ]] || die "package set differs from VERSION: got [$got] want [$want]"
for d in "$OUT_DIR"/*.deb; do
  v=$(dpkg-deb -f "$d" Version)
  [[ $v == "$VPP_DEB_VERSION" ]] || die "$(basename "$d") has version $v, expected $VPP_DEB_VERSION"
done

patches_json=$(for i in "${!PATCH_NAMES[@]}"; do printf '%s\t%s\t%s\n' "${PATCH_NAMES[$i]}" "${PATCH_SHAS[$i]}" "${PATCH_KINDS[$i]}"; done)
VRX_OUT="$OUT_DIR" VRX_PATCHES="$patches_json" VRX_TRACE="$TRACE_PLUGINS" VRX_JOBS="$JOBS" VRX_SECS="$build_secs" \
VRX_MISSING="${missing:-}" VRX_DESCRIBE="$describe" VRX_BUILDER_REV="$(git -C "$HERE" rev-parse HEAD 2>/dev/null || echo unknown)" \
VRX_BUILDER_DIRTY="$(git -C "$HERE" status --porcelain -- . 2>/dev/null | grep -qv '^??' && echo true || echo false)" \
VPP_UPSTREAM_URL="$VPP_UPSTREAM_URL" VPP_BRANCH="$VPP_BRANCH" VPP_TAG="$VPP_TAG" VPP_TAG_OBJECT="$VPP_TAG_OBJECT" \
VPP_COMMIT="$VPP_COMMIT" VPP_DEB_VERSION="$VPP_DEB_VERSION" VPP_PACKAGES_INSTALLED="$VPP_PACKAGES_INSTALLED" \
python3 - <<'PY'
import hashlib, json, os, platform, subprocess, glob, datetime
e = os.environ
out = e["VRX_OUT"]
def field(deb, f):
    return subprocess.run(["dpkg-deb", "-f", deb, f], check=True, capture_output=True, text=True).stdout.strip()
installed = set(e["VPP_PACKAGES_INSTALLED"].split())
pkgs = []
for deb in sorted(glob.glob(os.path.join(out, "*.deb"))):
    h = hashlib.sha256()
    with open(deb, "rb") as fh:
        for chunk in iter(lambda: fh.read(1 << 20), b""):
            h.update(chunk)
    name = field(deb, "Package")
    pkgs.append({"package": name, "version": field(deb, "Version"), "architecture": field(deb, "Architecture"),
                 "file": os.path.basename(deb), "size": os.path.getsize(deb), "sha256": h.hexdigest(),
                 "installed_on_vrx_a": name in installed})
patches = []
for line in e["VRX_PATCHES"].splitlines():
    if line.strip():
        n, s, k = line.split("\t")
        patches.append({"name": n, "sha256": s, "kind": k})
osr = dict(l.rstrip("\n").split("=", 1) for l in open("/etc/os-release") if "=" in l)
manifest = {
    "schema": "vrx.vpp-debs.manifest/v1",
    "upstream": {"url": e["VPP_UPSTREAM_URL"], "branch": e["VPP_BRANCH"], "tag": e["VPP_TAG"],
                 "tag_object": e["VPP_TAG_OBJECT"], "commit": e["VPP_COMMIT"], "describe": e["VRX_DESCRIBE"]},
    "version": e["VPP_DEB_VERSION"],
    "patches": patches,
    "options": {"trace_plugins": e["VRX_TRACE"]},
    "build": {"builder": "deploy/vpp/build.sh", "builder_commit": e["VRX_BUILDER_REV"],
              "builder_dirty": e["VRX_BUILDER_DIRTY"] == "true",
              "command": "make pkg-deb (upstream defaults: CMAKE_BUILD_TYPE=release, LTO, all plugins)",
              "jobs": int(e["VRX_JOBS"]), "seconds": int(e["VRX_SECS"]),
              "source_date_epoch": int(e["SOURCE_DATE_EPOCH"]),
              "os": osr.get("PRETTY_NAME", "").strip('"'), "arch": platform.machine(),
              "finished": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
              "missing_build_deps": e["VRX_MISSING"].split()},
    "packages": pkgs,
}
with open(os.path.join(out, "manifest.json"), "w") as fh:
    json.dump(manifest, fh, indent=2)
    fh.write("\n")
PY

"$HERE/verify.sh" --manifest "$OUT_DIR/manifest.json" --sums "$OUT_DIR/SHA256SUMS" >/dev/null \
  || die "verify.sh rejected the produced manifest"
log "artefacts in $OUT_DIR:"
(cd "$OUT_DIR" && ls -l)
log "OK: ${#debs[@]} packages, version $VPP_DEB_VERSION, patches: ${PATCH_NAMES[*]:-none}"
