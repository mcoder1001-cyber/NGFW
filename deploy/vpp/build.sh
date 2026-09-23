#!/usr/bin/env bash
# deploy/vpp/build.sh — reproducible VPP .deb build from the pinned upstream tag + our patch series (WBS D0.2, F-vpp-debs).
#
#   deploy/vpp/build.sh [options]
#
# Steps: verify.sh (static) → parse VERSION as data → clone/fetch into deploy/vpp/.build (never builds inside /root/vpp)
# → verify tag object + commit → pristine checkout → build-patches/series (build infrastructure only, always) →
# patches/series (product; "Status: demo" patches only with --demo) + optional patches → D-089 local version
# "<VPP_DEB_VERSION>+vrx<VPP_LOCAL_REV>" when any such patch was applied → build-dependency check (report, never
# installs; an apt failure is fatal) → hash-locked Python deps (pydeps.lock) in a verified wheelhouse + a
# --require-hashes install check in a scratch venv → free-disk check → `make pkg-deb` (upstream flags, capped
# parallelism) → .deb + SHA256SUMS + manifest.json → verify.sh --require-files on the output.
#
# Options:
#   --build-dir DIR        scratch dir; must resolve inside deploy/vpp/.build (default: that dir). Source at DIR/src/vpp
#   --out DIR              artefact dir; must resolve inside the build dir (default DIR/out/<version>[-<variant>]);
#                          emptied first, and only if it holds nothing but build.sh artefacts
#   --reference DIR        local VPP clone used read-only to borrow git objects and external source tarballs
#                          (default /root/vpp). Never used for Python wheels.
#   --offline-reference    no network: clone from --reference instead of VPP_UPSTREAM_URL (hashes still verified),
#                          Python deps only from the verified wheelhouse. Without it an unreachable upstream is fatal.
#   --jobs N               parallel jobs, 1..8 (default 8; shared host rule). The whole build also runs under taskset
#   --demo                 also apply patches whose header says "Status: demo" (default: skipped); variant "demo"
#   --trace-plugins MODE   devtools (default, upstream packaging) or core (patches/optional/trace-plugins-core.patch; V18)
#   --prepare-only         stop before the compile (clone, verify, patch, version, deps, pydeps)
#   --strict-deps          fail when a DEB_DEPENDS package is missing (default: report and continue)
#   -h, --help
#
# Nothing here installs packages system-wide, touches /etc/vpp, the running VPP or /root/vpp. Install procedure for the
# manager after handover: deploy/vpp/README.md.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=SCRIPTDIR/lib.sh
source "$HERE/lib.sh"
ALLOWED_ROOT="$HERE/.build"
BUILD_DIR="$ALLOWED_ROOT"
OUT_DIR=""
REFERENCE=/root/vpp
OFFLINE=0
JOBS=8
DEMO=0
TRACE_PLUGINS=devtools
PREPARE_ONLY=0
STRICT_DEPS=0
MAX_JOBS=8
MIN_FREE_GB=40

usage() { sed -n '2,/^set -euo pipefail/{/^set -euo pipefail/!p}' "$0" | sed -E 's/^# ?//'; }
log() { printf '[build.sh %s] %s\n' "$(date +%H:%M:%S)" "$*"; }
die() { printf '[build.sh] ERROR: %s\n' "$*" >&2; exit 1; }

while (($#)); do
  case "$1" in
    --build-dir) BUILD_DIR=${2:?}; shift ;;
    --out) OUT_DIR=${2:?}; shift ;;
    --reference) REFERENCE=${2:?}; shift ;;
    --offline-reference) OFFLINE=1 ;;
    --jobs) JOBS=${2:?}; shift ;;
    --demo|--with-demo) DEMO=1 ;;
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

# ------------------------------------------------------------------ 0. path guards first (M2), then static checks
BUILD_DIR=$(vrx_guard_dir "$BUILD_DIR" "$ALLOWED_ROOT" --build-dir) || die "--build-dir must resolve inside $ALLOWED_ROOT"
if [[ -n $OUT_DIR ]]; then
  OUT_DIR=$(vrx_guard_dir "$OUT_DIR" "$BUILD_DIR" --out) || die "--out must resolve inside the build dir $BUILD_DIR"
fi
[[ -z ${VRX_VPP_IN_TESTS:-} ]] || die "path guards passed inside tests/run.sh — refusing to continue (test expected a refusal)"
"$HERE/verify.sh" >/dev/null || die "deploy/vpp/verify.sh failed — fix VERSION/series/lock first (run it for details)"
vrx_parse_version "$HERE/VERSION" || die "VERSION rejected"

mkdir -p "$BUILD_DIR/src"
[[ -f $ALLOWED_ROOT/$VRX_OWNED_MARKER ]] || echo "scratch dir of deploy/vpp/build.sh — rm -rf targets must lie below it" >"$ALLOWED_ROOT/$VRX_OWNED_MARKER"
[[ -f $BUILD_DIR/$VRX_OWNED_MARKER ]] || cp "$ALLOWED_ROOT/$VRX_OWNED_MARKER" "$BUILD_DIR/$VRX_OWNED_MARKER"
SRC="$BUILD_DIR/src/vpp"

REAL_REF=""
if [[ -n $REFERENCE && -d $REFERENCE/.git ]]; then REAL_REF="$(vrx_realpath "$REFERENCE")"; fi
if ((OFFLINE)) && [[ -z $REAL_REF ]]; then die "--offline-reference needs a --reference git clone"; fi

# ------------------------------------------------------------------ 1. which patches (decided before touching anything)
declare -a PLAN_FILES=() PLAN_P=() PLAN_KIND=() SKIPPED=()
while read -r name popt; do
  PLAN_FILES+=("build-patches/$name"); PLAN_P+=("$popt"); PLAN_KIND+=(build)
done < <(vrx_series "$HERE/build-patches/series")
while read -r name popt; do
  st=$(vrx_patch_status "$HERE/patches/$name")
  if [[ $st == demo && $DEMO == 0 ]]; then SKIPPED+=("$name"); continue; fi
  PLAN_FILES+=("patches/$name"); PLAN_P+=("$popt"); PLAN_KIND+=("$st")
done < <(vrx_series "$HERE/patches/series")
if [[ $TRACE_PLUGINS == core ]]; then PLAN_FILES+=(patches/optional/trace-plugins-core.patch); PLAN_P+=(-p1); PLAN_KIND+=(optional); fi
n_version_patches=0
for k in "${PLAN_KIND[@]}"; do if [[ $k != build ]]; then n_version_patches=$((n_version_patches + 1)); fi; done
EXPECT_VERSION=$(vrx_local_version "$VPP_DEB_VERSION" "$VPP_LOCAL_REV" "$n_version_patches")
VARIANT=""
if [[ " ${PLAN_KIND[*]} " == *" demo "* ]]; then VARIANT+="-demo"; fi
if [[ $TRACE_PLUGINS == core ]]; then VARIANT+="-trace-core"; fi
if [[ -z $OUT_DIR ]]; then
  OUT_DIR=$(vrx_guard_dir "$BUILD_DIR/out/${EXPECT_VERSION}${VARIANT}" "$BUILD_DIR" --out) || die "bad default out dir"
fi
[[ $OUT_DIR != "$BUILD_DIR" && $OUT_DIR != "$SRC" && $OUT_DIR != "$SRC"/* ]] || die "--out must not be the build dir or the source tree"

log "pinned: $VPP_UPSTREAM_URL $VPP_TAG ($VPP_COMMIT)"
log "expect: version $EXPECT_VERSION ($n_version_patches version-relevant patch(es)); packages: $VPP_PACKAGES"
((${#SKIPPED[@]} == 0)) || log "skipped demo patch(es) (use --demo): ${SKIPPED[*]}"
log "build dir: $BUILD_DIR  out: $OUT_DIR  jobs: $JOBS  trace-plugins: $TRACE_PLUGINS  reference: ${REAL_REF:-<none>}  offline: $OFFLINE"

# ------------------------------------------------------------------ 2. source: upstream clone (or explicit offline reference, L5)
have_commit() { git -C "$SRC" cat-file -e "$VPP_COMMIT^{commit}" 2>/dev/null && git -C "$SRC" rev-parse -q --verify "refs/tags/$VPP_TAG" >/dev/null; }
SOURCE_KIND=upstream; ((OFFLINE == 0)) || SOURCE_KIND=reference
if [[ ! -d $SRC/.git ]]; then
  vrx_rm_rf "$SRC" "$BUILD_DIR" || die "refusing to clear $SRC"
  if ((OFFLINE)); then
    log "cloning read-only from $REAL_REF (--offline-reference)"
    git clone --no-checkout --no-hardlinks "$REAL_REF" "$SRC"
  elif [[ -n $REAL_REF ]]; then
    log "cloning $VPP_UPSTREAM_URL (objects borrowed from $REAL_REF, then dissociated)"
    git clone --no-checkout --reference-if-able "$REAL_REF" --dissociate "$VPP_UPSTREAM_URL" "$SRC" \
      || die "upstream clone failed — no silent fallback; rerun with --offline-reference to build from $REAL_REF"
  else
    git clone --no-checkout "$VPP_UPSTREAM_URL" "$SRC" || die "upstream clone failed"
  fi
fi
if ! have_commit; then
  ((OFFLINE == 0)) || die "$VPP_COMMIT / $VPP_TAG not in $SRC and --offline-reference given"
  log "fetching $VPP_TAG from $VPP_UPSTREAM_URL"
  git -C "$SRC" fetch --no-tags "$VPP_UPSTREAM_URL" "refs/tags/$VPP_TAG:refs/tags/$VPP_TAG" || die "fetch of $VPP_TAG failed"
fi

# ------------------------------------------------------------------ 3. verify the pin (tag object and commit)
tag_obj=$(git -C "$SRC" rev-parse "refs/tags/$VPP_TAG")
tag_commit=$(git -C "$SRC" rev-parse "refs/tags/$VPP_TAG^{commit}")
[[ $tag_obj == "$VPP_TAG_OBJECT" ]] || die "tag $VPP_TAG is $tag_obj, VERSION pins tag object $VPP_TAG_OBJECT (moved tag?)"
[[ $tag_commit == "$VPP_COMMIT" ]] || die "tag $VPP_TAG points to $tag_commit, VERSION pins $VPP_COMMIT"
[[ $(git -C "$SRC" cat-file -t "$tag_obj") == tag ]] || die "$VPP_TAG must be an annotated tag (src/scripts/version uses git describe)"
log "verified: $VPP_TAG = tag object $tag_obj → commit $tag_commit (source: $SOURCE_KIND)"

# ------------------------------------------------------------------ 4. pristine checkout (keeps ccache + tarball cache)
git -C "$SRC" checkout -q -f --detach "$VPP_COMMIT"
git -C "$SRC" clean -q -ffdx -e /build-root/.ccache -e /build/external/downloads
[[ -z $(git -C "$SRC" status --porcelain) ]] || die "source tree not pristine after checkout/clean"
[[ $(git -C "$SRC" rev-parse HEAD) == "$VPP_COMMIT" ]] || die "HEAD is not $VPP_COMMIT"
upstream_version=$("$SRC/src/scripts/version")
[[ $upstream_version == "$VPP_DEB_VERSION" ]] || die "pristine src/scripts/version = $upstream_version, VERSION says $VPP_DEB_VERSION"
log "checked out $(git -C "$SRC" describe --long --match 'v*') (pristine, upstream version $upstream_version)"

# ------------------------------------------------------------------ 5. apply patches: strict git apply, no fuzz (M1)
declare -a APPLIED=()
for i in "${!PLAN_FILES[@]}"; do
  f="$HERE/${PLAN_FILES[$i]}"
  vrx_apply_patch "$SRC" "$f" "${PLAN_P[$i]}" || die "${PLAN_FILES[$i]} does not apply exactly on $VPP_TAG"
  APPLIED+=("${PLAN_FILES[$i]}	$(vrx_sha256 "$f")	${PLAN_KIND[$i]}")
  log "applied ${PLAN_FILES[$i]} (${PLAN_KIND[$i]})"
done

# ------------------------------------------------------------------ 6. D-089 local version (before any compile)
if [[ $EXPECT_VERSION != "$VPP_DEB_VERSION" ]]; then vrx_write_version_script "$SRC" "$EXPECT_VERSION"; fi
tree_version=$("$SRC/src/scripts/version")
[[ $tree_version == "$EXPECT_VERSION" ]] || die "src/scripts/version = $tree_version, expected $EXPECT_VERSION"
git -C "$SRC" status --porcelain | sed 's/^/    /'
log "tree version: $tree_version"

# ------------------------------------------------------------------ 7. build dependencies: report, never install (M3)
# shellcheck disable=SC2016  # $(DEB_DEPENDS) is make syntax, expanded by make
deps=$(make -s -C "$SRC" -f Makefile -f <(printf 'vrx-print-deps:\n\t@echo $(DEB_DEPENDS)\n') vrx-print-deps 2>/dev/null) \
  || die "could not read DEB_DEPENDS from the VPP Makefile"
# shellcheck disable=SC2086  # word splitting of the package list is intended
missing=$(vrx_apt_missing $deps) || die "build-dependency check failed (apt-get -s error above) — not guessing"
if [[ -n ${missing// /} ]]; then
  log "MISSING build dependencies (not installed; install-dep is never run here): $missing"
  ((STRICT_DEPS == 0)) || die "missing build dependencies with --strict-deps"
else
  log "build dependencies: all $(wc -w <<<"$deps") DEB_DEPENDS entries satisfied (apt-get -s rc=0)"
fi

# ------------------------------------------------------------------ 8. Python build deps: hash-locked (H1)
PYDEPS_DIR="$BUILD_DIR/pydeps/wheelhouse"
DL_CACHE="$BUILD_DIR/pydeps/empty-dl-cache"     # upstream DL_CACHE_DIR defaults to ~/Downloads — never read it
mkdir -p "$DL_CACHE"
pyout=$(vrx_pydeps_prepare "$HERE/pydeps.lock" "$PYDEPS_DIR" "$BUILD_DIR" "$OFFLINE") || die "Python build deps could not be prepared"
while IFS= read -r l; do log "pydeps: $l"; done <<<"$pyout"
vrx_pydeps_verify "$HERE/pydeps.lock" "$PYDEPS_DIR" || die "wheelhouse verification failed"
CHECK_VENV="$BUILD_DIR/pydeps/check-venv"
vrx_rm_rf "$CHECK_VENV" "$BUILD_DIR" || die "cannot reset $CHECK_VENV"
python3 -m venv "$CHECK_VENV"
PIP_NO_CACHE_DIR=1 "$CHECK_VENV/bin/pip" install -q --disable-pip-version-check --require-hashes --no-index --find-links "$PYDEPS_DIR" \
  -r "$HERE/pydeps.lock" || die "pip --require-hashes install of pydeps.lock failed"
log "pydeps: pip install --require-hashes --no-index --find-links $PYDEPS_DIR OK: $("$CHECK_VENV/bin/pip" list --format=freeze --disable-pip-version-check 2>/dev/null | grep -Ev '^pip==' | tr '\n' ' ')"
# External source tarballs may be seeded from the reference tree (upstream re-verifies their sha256); wheels/sdists never.
pyfiles=" $(vrx_pydeps_parse "$HERE/pydeps.lock" | awk '{print $4}' | tr '\n' ' ') "
DLD="$SRC/build/external/downloads"
mkdir -p "$DLD"
for t in "$DLD"/*; do
  b=${t##*/}
  if [[ -f $t && ( $b == *.whl || $pyfiles == *" $b "* ) ]]; then rm -f -- "$t"; fi
done
if [[ -n $REAL_REF && -d $REAL_REF/build/external/downloads ]]; then
  for t in "$REAL_REF"/build/external/downloads/*; do
    b=${t##*/}
    [[ -f $t && $b != *.whl && $pyfiles != *" $b "* ]] || continue
    cp -n -- "$t" "$DLD/" 2>/dev/null || true
  done
fi
log "external tarball cache: $(find "$DLD" -maxdepth 1 -type f -printf '%f ')"

if ((PREPARE_ONLY)); then log "--prepare-only: stopping before the compile"; exit 0; fi

# ------------------------------------------------------------------ 9. free disk (L3)
avail_gb=$(( $(df --output=avail -k "$BUILD_DIR" | tail -1) / 1024 / 1024 ))
((avail_gb >= MIN_FREE_GB)) || die "only ${avail_gb} GB free under $BUILD_DIR, need >= $MIN_FREE_GB GB"
log "free disk: ${avail_gb} GB (>= $MIN_FREE_GB)"

# ------------------------------------------------------------------ 10. make pkg-deb (upstream flags, capped parallelism)
ncpu=$(nproc --all)
cpus="$((ncpu - JOBS))-$((ncpu - 1))"; ((ncpu > JOBS)) || cpus="0-$((ncpu - 1))"
SOURCE_DATE_EPOCH=$(git -C "$SRC" log -1 --format=%ct "$VPP_COMMIT")
export SOURCE_DATE_EPOCH
# Every pip the upstream build runs (DPDK meson venv via build-patches/0001; python3-vpp-api's PEP 517 build isolation,
# which needs setuptools>=61) resolves only from the verified wheelhouse: no index, no ~/.cache/pip, no ~/Downloads.
export VRX_PYDEPS_LOCK="$HERE/pydeps.lock" VRX_PYDEPS_DIR="$PYDEPS_DIR"
export PIP_NO_INDEX=1 PIP_FIND_LINKS="$PYDEPS_DIR" PIP_NO_CACHE_DIR=1 PIP_DISABLE_PIP_VERSION_CHECK=1
unset MAKEFLAGS MFLAGS
MAKE_ARGS=(pkg-deb MAKE_PARALLEL_JOBS="$JOBS" JOBS="$JOBS" DL_CACHE_DIR="$DL_CACHE"
           VRX_PYDEPS_LOCK="$VRX_PYDEPS_LOCK" VRX_PYDEPS_DIR="$VRX_PYDEPS_DIR")
log "make -C $SRC ${MAKE_ARGS[*]} (taskset -c $cpus, nice 10, SOURCE_DATE_EPOCH=$SOURCE_DATE_EPOCH)"
t0=$SECONDS
taskset -c "$cpus" nice -n 10 make -C "$SRC" "${MAKE_ARGS[@]}"
build_secs=$((SECONDS - t0))
log "make pkg-deb finished in $((build_secs / 60))m$((build_secs % 60))s"

# the DPDK meson venv must contain exactly the locked versions
DPDK_VENV="$SRC/build-root/build-vpp-native/external/dpdk-meson-venv"
venv_freeze=$("$DPDK_VENV/bin/pip" list --format=freeze --disable-pip-version-check 2>/dev/null | grep -Ev '^pip==' | sort | tr '\n' ' ')
lock_freeze=$(vrx_pydeps_parse "$HERE/pydeps.lock" | awk '{print $1"=="$2}' | sort | tr '\n' ' ')
[[ ${venv_freeze,,} == "${lock_freeze,,}" ]] || die "DPDK meson venv = [$venv_freeze], pydeps.lock = [$lock_freeze]"
log "DPDK meson venv matches pydeps.lock: $venv_freeze"

# ------------------------------------------------------------------ 11. collect + SHA256SUMS + manifest.json
vrx_out_dir_is_ours "$OUT_DIR" || die "refusing to empty $OUT_DIR"
vrx_rm_rf "$OUT_DIR" "$BUILD_DIR" || die "cannot clear $OUT_DIR"
mkdir -p "$OUT_DIR"
shopt -s nullglob
debs=("$SRC"/build-root/*.deb)
((${#debs[@]})) || die "no .deb produced in $SRC/build-root"
cp "${debs[@]}" "$OUT_DIR/"
cp "$SRC"/build-root/*.buildinfo "$SRC"/build-root/*.changes "$OUT_DIR/" 2>/dev/null || true
(cd "$OUT_DIR" && sha256sum -- *.deb | sort -k2 >SHA256SUMS)

VRX_OUT="$OUT_DIR" VRX_APPLIED="$(printf '%s\n' "${APPLIED[@]}")" VRX_PYDEPS="$(vrx_pydeps_parse "$HERE/pydeps.lock")" \
VRX_TRACE="$TRACE_PLUGINS" VRX_DEMO="$DEMO" VRX_JOBS="$JOBS" VRX_SECS="$build_secs" VRX_MISSING="${missing:-}" \
VRX_SOURCE="$SOURCE_KIND" VRX_VARIANT="${VARIANT#-}" VRX_VERSION="$EXPECT_VERSION" VRX_VENV="$venv_freeze" \
VRX_DESCRIBE="$(git -C "$SRC" describe --long --match 'v*')" \
VRX_BUILDER_REV="$(git -C "$HERE" rev-parse HEAD 2>/dev/null || echo unknown)" \
VRX_BUILDER_DIRTY="$(if [[ -n $(git -C "$HERE" status --porcelain -- . 2>/dev/null) ]]; then echo true; else echo false; fi)" \
VPP_UPSTREAM_URL="$VPP_UPSTREAM_URL" VPP_BRANCH="$VPP_BRANCH" VPP_TAG="$VPP_TAG" VPP_TAG_OBJECT="$VPP_TAG_OBJECT" \
VPP_COMMIT="$VPP_COMMIT" VPP_DEB_VERSION="$VPP_DEB_VERSION" VPP_LOCAL_REV="$VPP_LOCAL_REV" \
VPP_PACKAGES_INSTALLED="$VPP_PACKAGES_INSTALLED" VPP_PACKAGES_SHIP="$VPP_PACKAGES_SHIP" \
python3 - <<'PY'
import hashlib, json, os, platform, subprocess, glob, datetime
e = os.environ
out = e["VRX_OUT"]
def field(deb, f):
    return subprocess.run(["dpkg-deb", "-f", deb, f], check=True, capture_output=True, text=True).stdout.strip()
installed, ship = set(e["VPP_PACKAGES_INSTALLED"].split()), set(e["VPP_PACKAGES_SHIP"].split())
pkgs = []
for deb in sorted(glob.glob(os.path.join(out, "*.deb"))):
    h = hashlib.sha256()
    with open(deb, "rb") as fh:
        for chunk in iter(lambda: fh.read(1 << 20), b""):
            h.update(chunk)
    name = field(deb, "Package")
    pkgs.append({"package": name, "version": field(deb, "Version"), "architecture": field(deb, "Architecture"),
                 "file": os.path.basename(deb), "size": os.path.getsize(deb), "sha256": h.hexdigest(),
                 "ship": name in ship, "installed_on_vrx_a": name in installed})
applied = [dict(zip(("name", "sha256", "kind"), l.split("\t"))) for l in e["VRX_APPLIED"].splitlines() if l.strip()]
pydeps = [dict(zip(("name", "version", "sha256", "file", "url"), l.split())) for l in e["VRX_PYDEPS"].splitlines() if l.strip()]
osr = dict(l.rstrip("\n").split("=", 1) for l in open("/etc/os-release") if "=" in l)
patched = e["VRX_VERSION"] != e["VPP_DEB_VERSION"]
manifest = {
    "schema": "vrx.vpp-debs.manifest/v2",
    "upstream": {"url": e["VPP_UPSTREAM_URL"], "branch": e["VPP_BRANCH"], "tag": e["VPP_TAG"],
                 "tag_object": e["VPP_TAG_OBJECT"], "commit": e["VPP_COMMIT"], "describe": e["VRX_DESCRIBE"]},
    "version": e["VRX_VERSION"],
    "upstream_version": e["VPP_DEB_VERSION"],
    "local_rev": int(e["VPP_LOCAL_REV"]) if patched else None,
    "variant": e["VRX_VARIANT"] or "default",
    "patches": [p for p in applied if p["kind"] != "build"],
    "options": {"trace_plugins": e["VRX_TRACE"], "demo": e["VRX_DEMO"] == "1"},
    "build": {"builder": "deploy/vpp/build.sh", "builder_commit": e["VRX_BUILDER_REV"],
              "builder_dirty": e["VRX_BUILDER_DIRTY"] == "true", "source": e["VRX_SOURCE"],
              "command": "make pkg-deb (upstream defaults: CMAKE_BUILD_TYPE=release, LTO, all plugins)",
              "build_patches": [p for p in applied if p["kind"] == "build"],
              "inputs": {"python": pydeps, "dpdk_meson_venv": e["VRX_VENV"].split()},
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

"$HERE/verify.sh" --no-tests --require-files "$OUT_DIR" >/dev/null || die "verify.sh --require-files rejected the output (run it for details)"
log "artefacts in $OUT_DIR:"
(cd "$OUT_DIR" && ls -l)
log "OK: ${#debs[@]} packages, version $EXPECT_VERSION, variant ${VARIANT#-}; $(du -sh "$BUILD_DIR" | cut -f1) used under $BUILD_DIR"
