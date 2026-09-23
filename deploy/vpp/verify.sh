#!/usr/bin/env bash
# deploy/vpp/verify.sh — cheap consistency gate for the VPP package pipeline (no clone, no build; < 1 s).
#
#   deploy/vpp/verify.sh                                   VERSION + patches/series + repo hygiene
#   deploy/vpp/verify.sh --manifest <manifest.json> [--sums <SHA256SUMS>]
#                                                          + a produced manifest against VERSION/series; with --sums also
#                                                            SHA256SUMS vs manifest, and the .deb files next to it if present
#
# Checks: VERSION keys/format and tag↔version coherence; every series entry exists, every top-level patch is listed,
# no duplicates, every patch (series + optional/) has the Subject/Track/Status/Upstream header and a unified diff;
# demo patches are marked Status: demo; build.sh/verify.sh parse (bash -n, shellcheck when installed);
# no .deb tracked in git and deploy/vpp/.build is git-ignored. Exit 0 = OK, 1 = findings (printed).
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
MANIFEST=""; SUMS=""
while (($#)); do
  case "$1" in
    --manifest) MANIFEST=${2:?}; shift ;;
    --sums) SUMS=${2:?}; shift ;;
    -h|--help) sed -n '2,/^set -euo pipefail/{/^set -euo pipefail/!p}' "$0" | sed -E 's/^# ?//'; exit 0 ;;
    *) echo "verify.sh: unknown argument '$1'" >&2; exit 2 ;;
  esac
  shift
done

errs=0
bad() { printf 'FAIL %s\n' "$*"; errs=$((errs + 1)); }
ok() { printf 'ok   %s\n' "$*"; }

# ------------------------------------------------------------------ VERSION
V="$HERE/VERSION"
[[ -f $V ]] || { bad "missing $V"; exit 1; }
if grep -vE '^(#.*|[[:space:]]*|VPP_[A-Z_]+=("[^"]*"|[^[:space:]"]*))$' "$V" | grep -q .; then
  bad "VERSION has lines that are not comments or KEY=VALUE: $(grep -vE '^(#.*|[[:space:]]*|VPP_[A-Z_]+=("[^"]*"|[^[:space:]"]*))$' "$V" | head -3 | tr '\n' '|')"
fi
# shellcheck source=/dev/null
source <(grep -E '^VPP_[A-Z_]+=' "$V")
for k in VPP_UPSTREAM_URL VPP_BRANCH VPP_TAG VPP_TAG_OBJECT VPP_COMMIT VPP_DEB_VERSION VPP_PACKAGES VPP_PACKAGES_INSTALLED; do
  [[ -n ${!k:-} ]] || bad "VERSION: $k is missing or empty"
done
[[ ${VPP_UPSTREAM_URL:-} =~ ^https://[^[:space:]]+$ ]] || bad "VERSION: VPP_UPSTREAM_URL must be an https URL"
[[ ${VPP_TAG_OBJECT:-} =~ ^[0-9a-f]{40}$ ]] || bad "VERSION: VPP_TAG_OBJECT must be a full 40-hex hash"
[[ ${VPP_COMMIT:-} =~ ^[0-9a-f]{40}$ ]] || bad "VERSION: VPP_COMMIT must be a full 40-hex hash"
if [[ ${VPP_TAG:-} =~ ^v([0-9]{2}\.[0-9]{2}(\.[0-9]+)?)$ ]]; then
  [[ ${VPP_DEB_VERSION:-} == "${BASH_REMATCH[1]}-release" ]] \
    || bad "VERSION: VPP_DEB_VERSION=$VPP_DEB_VERSION but tag $VPP_TAG builds ${BASH_REMATCH[1]}-release (src/scripts/version)"
  [[ ${VPP_BRANCH:-} == "stable/${BASH_REMATCH[1]:0:2}${BASH_REMATCH[1]:3:2}" ]] \
    || bad "VERSION: VPP_BRANCH=$VPP_BRANCH does not match tag $VPP_TAG (expected stable/YYMM)"
else
  bad "VERSION: VPP_TAG must be a release tag vYY.MM[.N] (got '${VPP_TAG:-}')"
fi
for p in ${VPP_PACKAGES_INSTALLED:-}; do
  [[ " ${VPP_PACKAGES:-} " == *" $p "* ]] || bad "VERSION: installed package $p is not in VPP_PACKAGES"
done
((errs)) || ok "VERSION: $VPP_TAG $VPP_COMMIT → $VPP_DEB_VERSION, $(wc -w <<<"$VPP_PACKAGES") packages"

# ------------------------------------------------------------------ patches/series
S="$HERE/patches/series"
declare -a SERIES=()
if [[ -f $S ]]; then
  while IFS= read -r line || [[ -n $line ]]; do
    line=${line%%#*}; read -r name popt extra <<<"$line" || true
    [[ -n ${name:-} ]] || continue
    [[ -z ${extra:-} ]] || bad "series: trailing garbage after $name"
    [[ -z ${popt:-} || $popt =~ ^-p[0-9]$ ]] || bad "series: bad option '$popt' for $name (only -pN)"
    [[ $name != */* && $name == *.patch ]] || bad "series: $name must be a *.patch file directly in patches/"
    [[ " ${SERIES[*]} " != *" $name "* ]] || bad "series: $name listed twice"
    SERIES+=("$name")
  done <"$S"
else
  bad "missing $S"
fi
check_patch() {  # check_patch <path> <label>
  local f=$1 l=$2 h
  [[ -f $f ]] || { bad "patch $l listed but missing"; return; }
  for h in Subject Track Status Upstream; do
    grep -qE "^$h: " "$f" || bad "patch $l: header '$h:' missing"
  done
  if ! grep -qE '^\+\+\+ b/' "$f" || ! grep -qE '^@@ ' "$f"; then bad "patch $l: no unified diff (--- a/ +++ b/ @@) found"; fi
  if [[ $l == *DEMO* ]]; then grep -qE '^Status: demo' "$f" || bad "patch $l: DEMO patch must say 'Status: demo'"; fi
  if grep -qE '^Status: demo' "$f" && [[ $l != *DEMO* ]]; then bad "patch $l: Status: demo but name lacks DEMO"; fi
}
for n in "${SERIES[@]}"; do check_patch "$HERE/patches/$n" "$n"; done
shopt -s nullglob
for f in "$HERE"/patches/*.patch; do
  [[ " ${SERIES[*]} " == *" $(basename "$f") "* ]] || bad "patches/$(basename "$f") exists but is not in series (orphan)"
done
for f in "$HERE"/patches/optional/*.patch; do check_patch "$f" "optional/$(basename "$f")"; done
((errs)) || ok "series: ${#SERIES[@]} patch(es) [${SERIES[*]}], optional: $(cd "$HERE/patches" && echo optional/*.patch)"

# ------------------------------------------------------------------ scripts + repo hygiene
for s in build.sh verify.sh; do
  bash -n "$HERE/$s" || bad "$s: bash syntax error"
  [[ -x $HERE/$s ]] || bad "$s is not executable"
done
if command -v shellcheck >/dev/null 2>&1; then
  shellcheck -S warning "$HERE/build.sh" "$HERE/verify.sh" >/dev/null || bad "shellcheck warnings in build.sh/verify.sh (run shellcheck -S warning)"
fi
if git -C "$HERE" rev-parse --show-toplevel >/dev/null 2>&1; then
  top=$(git -C "$HERE" rev-parse --show-toplevel)
  tracked=$(git -C "$top" ls-files -- '*.deb' 'deploy/vpp/.build' | head -5)
  [[ -z $tracked ]] || bad "built artefacts tracked in git: $tracked"
  for probe in deploy/vpp/.build/probe deploy/vpp/probe.deb; do
    git -C "$top" check-ignore -q "$probe" || bad ".gitignore must ignore $probe (deploy/vpp/.build/ and *.deb)"
  done
fi
((errs)) || ok "scripts parse; no .deb/.build in git; .gitignore covers deploy/vpp/.build and *.deb"

# ------------------------------------------------------------------ produced manifest (optional)
if [[ -n $MANIFEST ]]; then
  series_lines=$(for n in "${SERIES[@]}"; do printf '%s\t%s\n' "$n" "$(sha256sum "$HERE/patches/$n" | cut -d' ' -f1)"; done)
  opt_lines=$(for f in "$HERE"/patches/optional/*.patch; do printf 'optional/%s\t%s\n' "$(basename "$f")" "$(sha256sum "$f" | cut -d' ' -f1)"; done)
  out=$(VRX_M="$MANIFEST" VRX_S="$SUMS" VRX_SERIES="$series_lines" VRX_OPT="$opt_lines" \
    VPP_TAG="$VPP_TAG" VPP_TAG_OBJECT="$VPP_TAG_OBJECT" VPP_COMMIT="$VPP_COMMIT" VPP_DEB_VERSION="$VPP_DEB_VERSION" \
    VPP_PACKAGES="$VPP_PACKAGES" VPP_UPSTREAM_URL="$VPP_UPSTREAM_URL" python3 - <<'PY'
import hashlib, json, os, sys
e = os.environ
errs = []
try:
    m = json.load(open(e["VRX_M"]))
except Exception as x:  # noqa: BLE001
    print(f"FAIL manifest: cannot parse {e['VRX_M']}: {x}"); sys.exit(0)
if m.get("schema") != "vrx.vpp-debs.manifest/v1": errs.append(f"schema is {m.get('schema')!r}")
up = m.get("upstream", {})
for k, env in (("url", "VPP_UPSTREAM_URL"), ("tag", "VPP_TAG"), ("tag_object", "VPP_TAG_OBJECT"), ("commit", "VPP_COMMIT")):
    if up.get(k) != e[env]: errs.append(f"upstream.{k}={up.get(k)!r} != VERSION {e[env]!r}")
if m.get("version") != e["VPP_DEB_VERSION"]: errs.append(f"version {m.get('version')!r} != {e['VPP_DEB_VERSION']!r}")
pk = m.get("packages", [])
names = sorted(p.get("package") for p in pk)
if names != sorted(e["VPP_PACKAGES"].split()): errs.append(f"package set {names} != VERSION VPP_PACKAGES")
for p in pk:
    if p.get("version") != e["VPP_DEB_VERSION"]: errs.append(f"{p.get('package')}: version {p.get('version')!r}")
    if len(p.get("sha256", "")) != 64: errs.append(f"{p.get('package')}: bad sha256")
series = [tuple(l.split("\t")) for l in e["VRX_SERIES"].splitlines() if l]
opt = dict(tuple(l.split("\t")) for l in e["VRX_OPT"].splitlines() if l)
want = list(series)
if m.get("options", {}).get("trace_plugins") == "core":
    want.append(("optional/trace-plugins-core.patch", opt.get("optional/trace-plugins-core.patch", "?")))
got = [(p.get("name"), p.get("sha256")) for p in m.get("patches", [])]
if got != want: errs.append(f"patches {got} != series+options {want} (manifest from another series revision?)")
if e["VRX_S"]:
    sums = {}
    for l in open(e["VRX_S"]):
        h, f = l.split(None, 1); sums[f.strip().lstrip("*")] = h
    man = {p["file"]: p["sha256"] for p in pk}
    if sums != man: errs.append("SHA256SUMS and manifest.json disagree")
    d = os.path.dirname(os.path.abspath(e["VRX_S"]))
    for f, h in sums.items():
        path = os.path.join(d, f)
        if os.path.exists(path):
            hh = hashlib.sha256(open(path, "rb").read()).hexdigest()
            if hh != h: errs.append(f"{f}: sha256 on disk {hh} != SHA256SUMS {h}")
for x in errs: print("FAIL manifest: " + x)
if not errs: print(f"ok   manifest: {len(pk)} packages {e['VPP_DEB_VERSION']}, patches {[g[0] for g in got]}" + (", SHA256SUMS consistent" if e["VRX_S"] else ""))
PY
)
  printf '%s\n' "$out"
  grep -q '^FAIL' <<<"$out" && errs=$((errs + $(grep -c '^FAIL' <<<"$out")))
fi

if ((errs)); then echo "verify.sh: $errs finding(s)"; exit 1; fi
echo "verify.sh: OK"
