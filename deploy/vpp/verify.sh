#!/usr/bin/env bash
# deploy/vpp/verify.sh — consistency gate for the VPP package pipeline (no clone, no build).
#
#   deploy/vpp/verify.sh                         static: VERSION (parsed as data) + series + build-patches + pydeps.lock
#                                                + script hygiene + lib unit tests (tests/run.sh) — a few seconds
#   deploy/vpp/verify.sh --require-files <out>   + a produced output dir: manifest.json vs VERSION/series/lock, SHA256SUMS,
#                                                  every .deb present (no missing, no extra), each .deb's Package/Version/
#                                                  Architecture fields and sha256 equal to its manifest entry
#   ... --require-files <out> --install-gate     + only an installable product build: version <base>+vrx<N>, no demo patch
#   --no-tests                                   skip tests/run.sh (the tests call verify.sh themselves)
#
# Exit 0 = OK, 1 = findings (printed as FAIL lines).
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=SCRIPTDIR/lib.sh
source "$HERE/lib.sh"
OUT=""; GATE=0; TESTS=1
[[ -z ${VRX_VPP_IN_TESTS:-} ]] || TESTS=0
while (($#)); do
  case "$1" in
    --require-files) OUT=${2:?}; shift ;;
    --install-gate) GATE=1 ;;
    --no-tests) TESTS=0 ;;
    -h|--help) sed -n '2,/^set -euo pipefail/{/^set -euo pipefail/!p}' "$0" | sed -E 's/^# ?//'; exit 0 ;;
    *) echo "verify.sh: unknown argument '$1'" >&2; exit 2 ;;
  esac
  shift
done
if ((GATE)) && [[ -z $OUT ]]; then echo "verify.sh: --install-gate needs --require-files <out>" >&2; exit 2; fi

errs=0
bad() { printf 'FAIL %s\n' "$*"; errs=$((errs + 1)); }
ok() { printf 'ok   %s\n' "$*"; }

# ------------------------------------------------------------------ VERSION (data; stop at the first format error)
if ! msg=$(vrx_parse_version "$HERE/VERSION" 2>&1); then bad "$msg"; echo "verify.sh: VERSION unusable — stopping"; exit 1; fi
vrx_parse_version "$HERE/VERSION"
ok "VERSION: $VPP_TAG $VPP_COMMIT → $VPP_DEB_VERSION (patched: $VPP_DEB_VERSION+vrx$VPP_LOCAL_REV), $(wc -w <<<"$VPP_PACKAGES") packages, ship $(wc -w <<<"$VPP_PACKAGES_SHIP")"

# ------------------------------------------------------------------ patch series
check_patch() {  # check_patch <path> <label> <allowed statuses regex> [build]
  local f=$1 l=$2 st h paths
  [[ -f $f ]] || { bad "patch $l listed but missing"; return; }
  for h in Subject Track Status Upstream; do grep -qE "^$h: " "$f" || bad "patch $l: header '$h:' missing"; done
  if ! grep -qE '^\+\+\+ b/' "$f" || ! grep -qE '^@@ ' "$f"; then bad "patch $l: no unified diff (--- a/ +++ b/ @@)"; fi
  st=$(vrx_patch_status "$f")
  [[ $st =~ ^($3)$ ]] || bad "patch $l: Status '$st' not allowed here ($3)"
  if [[ $l == *DEMO* && $st != demo ]]; then bad "patch $l: DEMO in the name needs 'Status: demo'"; fi
  if [[ $st == demo && $l != *DEMO* ]]; then bad "patch $l: 'Status: demo' needs DEMO in the file name"; fi
  if [[ ${4:-} == build ]]; then
    paths=$(sed -nE 's/^\+\+\+ b\/([^[:space:]]+).*/\1/p' "$f" | grep -v '^build/' || true)
    [[ -z $paths ]] || bad "build patch $l touches files outside build/: $paths"
  fi
}
SERIES_N=0
check_series() {  # check_series <dir> <statuses> [build]; sets SERIES_N
  local dir=$1 name popt seen=" " f
  SERIES_N=0
  [[ -f $dir/series ]] || { bad "missing $dir/series"; return; }
  while read -r name popt; do
    [[ $popt =~ ^-p[0-9]$ ]] || bad "${dir##*/}/series: bad option '$popt' for $name"
    [[ $name != */* && $name == *.patch ]] || bad "${dir##*/}/series: $name must be a *.patch directly in ${dir##*/}/"
    [[ $seen != *" $name "* ]] || bad "${dir##*/}/series: $name listed twice"
    seen+="$name "; SERIES_N=$((SERIES_N + 1))
    check_patch "$dir/$name" "${dir##*/}/$name" "$2" "${3:-}"
  done < <(vrx_series "$dir/series")
  for f in "$dir"/*.patch; do
    [[ -e $f ]] || continue
    [[ $seen == *" ${f##*/} "* ]] || bad "${dir##*/}/${f##*/} is not in the series (orphan)"
  done
}
shopt -s nullglob
e0=$errs
check_series "$HERE/patches" 'product|demo'; np=$SERIES_N
check_series "$HERE/build-patches" 'build' build; nb=$SERIES_N
for f in "$HERE"/patches/optional/*.patch; do check_patch "$f" "optional/${f##*/}" 'optional'; done
((errs > e0)) || ok "series: patches $np · build-patches $nb (build/ only) · optional $(cd "$HERE/patches" && echo optional/*.patch)"

# ------------------------------------------------------------------ pydeps.lock
e0=$errs
if ! entries=$(vrx_pydeps_parse "$HERE/pydeps.lock" 2>&1); then bad "$entries"; else
  dups=$(awk '{print tolower($1)}' <<<"$entries" | sort | uniq -d)
  [[ -z $dups ]] || bad "pydeps.lock: duplicate entries: $dups"
  for need in meson pyelftools; do
    grep -qx "$need" < <(awk '{print tolower($1)}' <<<"$entries") || bad "pydeps.lock: $need missing (the DPDK build needs it)"
  done
  ((errs > e0)) || ok "pydeps.lock: $(awk '{printf "%s==%s ", $1, $2}' <<<"$entries")(sha256-pinned)"
fi

# ------------------------------------------------------------------ scripts + repo hygiene
e0=$errs
for s in build.sh verify.sh tests/run.sh; do
  bash -n "$HERE/$s" || bad "$s: bash syntax error"
  [[ -x $HERE/$s ]] || bad "$s is not executable"
done
bash -n "$HERE/lib.sh" || bad "lib.sh: bash syntax error"
if [[ -z ${VRX_VPP_IN_TESTS:-} ]] && command -v shellcheck >/dev/null 2>&1; then
  (cd "$HERE" && shellcheck -x -S warning build.sh verify.sh lib.sh tests/run.sh >/dev/null) \
    || bad "shellcheck warnings (run: cd deploy/vpp && shellcheck -x build.sh verify.sh lib.sh tests/run.sh)"
fi
if top=$(git -C "$HERE" rev-parse --show-toplevel 2>/dev/null); then
  tracked=$(git -C "$top" ls-files -- '*.deb' '*.whl' 'deploy/vpp/.build' | head -5)
  [[ -z $tracked ]] || bad "built artefacts tracked in git: $tracked"
  for probe in deploy/vpp/.build/probe deploy/vpp/probe.deb; do
    git -C "$top" check-ignore -q "$probe" || bad ".gitignore must ignore $probe"
  done
fi
((errs > e0)) || ok "scripts parse + shellcheck; no .deb/.whl/.build in git; .gitignore covers deploy/vpp/.build and *.deb"
if ((TESTS)); then
  if tout=$("$HERE/tests/run.sh" 2>&1); then ok "tests/run.sh: $(tail -1 <<<"$tout")"
  else bad "tests/run.sh failed:"; grep -E '^not ok' <<<"$tout" | sed 's/^/     /' || true; fi
fi

# ------------------------------------------------------------------ produced output dir (review L1)
if [[ -n $OUT ]]; then
  series_lines=$(for d in build-patches patches; do
      while read -r name _; do
        printf '%s/%s\t%s\t%s\n' "$d" "$name" "$(vrx_sha256 "$HERE/$d/$name")" "$(vrx_patch_status "$HERE/$d/$name")"
      done < <(vrx_series "$HERE/$d/series")
    done
    for f in "$HERE"/patches/optional/*.patch; do printf 'patches/optional/%s\t%s\toptional\n' "${f##*/}" "$(vrx_sha256 "$f")"; done)
  out=$(VRX_OUT="$OUT" VRX_GATE="$GATE" VRX_SERIES="$series_lines" VRX_PYDEPS="$(vrx_pydeps_parse "$HERE/pydeps.lock")" \
    VPP_UPSTREAM_URL="$VPP_UPSTREAM_URL" VPP_TAG="$VPP_TAG" VPP_TAG_OBJECT="$VPP_TAG_OBJECT" VPP_COMMIT="$VPP_COMMIT" \
    VPP_DEB_VERSION="$VPP_DEB_VERSION" VPP_LOCAL_REV="$VPP_LOCAL_REV" VPP_PACKAGES="$VPP_PACKAGES" \
    VPP_PACKAGES_SHIP="$VPP_PACKAGES_SHIP" python3 - <<'PY'
import glob, hashlib, json, os, subprocess
e = os.environ
d = e["VRX_OUT"]
errs = []
fail = errs.append
def sha(p):
    h = hashlib.sha256()
    with open(p, "rb") as fh:
        for c in iter(lambda: fh.read(1 << 20), b""):
            h.update(c)
    return h.hexdigest()
try:
    m = json.load(open(os.path.join(d, "manifest.json")))
except Exception as x:  # noqa: BLE001
    print(f"FAIL output: cannot read {d}/manifest.json: {x}")
    raise SystemExit(0)
if m.get("schema") != "vrx.vpp-debs.manifest/v2":
    fail(f"schema is {m.get('schema')!r}, want vrx.vpp-debs.manifest/v2")
up = m.get("upstream", {})
for k, env in (("url", "VPP_UPSTREAM_URL"), ("tag", "VPP_TAG"), ("tag_object", "VPP_TAG_OBJECT"), ("commit", "VPP_COMMIT")):
    if up.get(k) != e[env]:
        fail(f"upstream.{k}={up.get(k)!r} != VERSION {e[env]!r}")
known = {}
for l in e["VRX_SERIES"].splitlines():
    if l.strip():
        n, s, k = l.split("\t")
        known[n] = (s, k)
build = m.get("build", {})
patches = m.get("patches", [])
for p in patches + build.get("build_patches", []):
    n = p.get("name")
    if n not in known:
        fail(f"patch {n!r} is not in the current series/optional set")
    elif (p.get("sha256"), p.get("kind")) != known[n]:
        fail(f"patch {n}: sha256/kind differ from the current file (stale manifest?)")
want_build = [n for n, (s, k) in known.items() if k == "build"]
got_build = [p.get("name") for p in build.get("build_patches", [])]
if got_build != want_build:
    fail(f"build_patches {got_build} != build-patches/series {want_build}")
if any(p.get("kind") == "build" for p in patches):
    fail("a build patch is listed under patches[]")
base, rev = e["VPP_DEB_VERSION"], e["VPP_LOCAL_REV"]
want_version = f"{base}+vrx{rev}" if patches else base
if m.get("version") != want_version:
    fail(f"version {m.get('version')!r}, but D-089 requires {want_version!r} for {len(patches)} applied patch(es)")
kinds = [p.get("kind") for p in patches]
if "demo" in kinds and not m.get("options", {}).get("demo"):
    fail("demo patch applied without options.demo")
if ("demo" in kinds) != ("demo" in str(m.get("variant", ""))):
    fail(f"variant {m.get('variant')!r} does not reflect the demo patches")
lock = [dict(zip(("name", "version", "sha256", "file", "url"), l.split())) for l in e["VRX_PYDEPS"].splitlines() if l.strip()]
if build.get("inputs", {}).get("python") != lock:
    fail("build.inputs.python differs from pydeps.lock")
venv = sorted(x.lower() for x in build.get("inputs", {}).get("dpdk_meson_venv", []))
if venv != sorted(f"{x['name']}=={x['version']}".lower() for x in lock):
    fail(f"build.inputs.dpdk_meson_venv {venv} != pydeps.lock")
pk = m.get("packages", [])
if sorted(p.get("package") for p in pk) != sorted(e["VPP_PACKAGES"].split()):
    fail("package set != VERSION VPP_PACKAGES")
ship = set(e["VPP_PACKAGES_SHIP"].split())
files = {}
for p in pk:
    f = p.get("file", "")
    if "/" in f or not f.endswith(".deb") or f.startswith("."):
        fail(f"bad file name {f!r}")
        continue
    files[f] = p
    if p.get("version") != m.get("version"):
        fail(f"{f}: manifest version {p.get('version')!r} != {m.get('version')!r}")
    if p.get("ship") != (p.get("package") in ship):
        fail(f"{f}: ship flag disagrees with VPP_PACKAGES_SHIP")
    path = os.path.join(d, f)
    if not os.path.isfile(path):
        fail(f"{f}: listed in manifest but missing")
        continue
    ctl = subprocess.run(["dpkg-deb", "-f", path, "Package", "Version", "Architecture"], capture_output=True, text=True)
    fields = dict(l.split(": ", 1) for l in ctl.stdout.splitlines() if ": " in l)
    for k, mk in (("Package", "package"), ("Version", "version"), ("Architecture", "architecture")):
        if fields.get(k) != p.get(mk):
            fail(f"{f}: control {k}={fields.get(k)!r} but manifest {mk}={p.get(mk)!r}")
    if sha(path) != p.get("sha256"):
        fail(f"{f}: sha256 on disk differs from manifest")
on_disk = {os.path.basename(x) for x in glob.glob(os.path.join(d, "*.deb"))}
for x in sorted(on_disk - set(files)):
    fail(f"{x}: .deb in the output dir but not in manifest (extra file)")
sums = {}
try:
    for l in open(os.path.join(d, "SHA256SUMS")):
        if l.strip():
            h, f = l.split(None, 1)
            sums[f.strip().lstrip("*")] = h
except OSError as x:
    fail(f"SHA256SUMS: {x}")
if sums != {f: p.get("sha256") for f, p in files.items()}:
    fail("SHA256SUMS and manifest.json disagree (file set or hashes)")
if e["VRX_GATE"] == "1":
    if not patches:
        fail("install gate: unpatched build (version == upstream) — never an install target (D-089)")
    if "demo" in kinds or "demo" in str(m.get("variant", "")):
        fail("install gate: demo patch in this build — never install it")
    if build.get("builder_dirty"):
        fail("install gate: built from an uncommitted builder tree")
for x in errs:
    print("FAIL output: " + x)
if not errs:
    print(f"ok   output {d}: {len(files)} .deb, version {m.get('version')}, variant {m.get('variant')}, "
          f"patches {[p.get('name') for p in patches]}; files/control fields/sha256/SHA256SUMS consistent"
          + ("; install gate passed" if e["VRX_GATE"] == "1" else ""))
PY
)
  printf '%s\n' "$out"
  n=$(grep -c '^FAIL' <<<"$out" || true)
  errs=$((errs + n))
fi

if ((errs)); then echo "verify.sh: $errs finding(s)"; exit 1; fi
echo "verify.sh: OK"
