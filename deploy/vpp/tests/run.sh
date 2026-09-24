#!/usr/bin/env bash
# deploy/vpp/tests/run.sh — unit-style tests for lib.sh, build.sh guards and verify.sh --require-files.
# No clone, no compile, no network, nothing installed; works in a scratch dir under $TMPDIR and removes it.
# Output: one "ok N - …" / "not ok N - …" line per test, last line "N passed, M failed". Exit 1 if any failed.
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
# shellcheck source=SCRIPTDIR/../lib.sh
source "$HERE/lib.sh"
export VRX_VPP_IN_TESTS=1
T=$(mktemp -d "${TMPDIR:-/tmp}/vrx-vpp-tests.XXXXXX")
trap 'rm -rf -- "$T"' EXIT
N=0; PASS=0; FAILN=0
check() {  # check "<name>" <command...>: passes when the command succeeds
  local name=$1; shift
  N=$((N + 1))
  if "$@" >"$T/last.out" 2>&1; then PASS=$((PASS + 1)); echo "ok $N - $name"
  else FAILN=$((FAILN + 1)); echo "not ok $N - $name"; sed 's/^/#   /' "$T/last.out" | head -8; fi
}
refuses() { ! "$@"; }                    # the command must fail
out_has() { grep -qE -- "$1" "$T/last.run"; }
run_capture() { "$@" >"$T/last.run" 2>&1; }

# ---------------------------------------------------------------- VERSION parsed as data (L2)
check "real VERSION parses" vrx_parse_version "$HERE/VERSION"
mkv() { grep -v "^$1=" "$HERE/VERSION" >"$T/V"; printf '%s\n' "$2" >>"$T/V"; }
mkv VPP_TAG "VPP_TAG=\$(touch $T/pwned)"
check "VERSION with \$(cmd) is rejected" refuses vrx_parse_version "$T/V"
check "... and the command never ran" test ! -e "$T/pwned"
# shellcheck disable=SC2016  # literal backticks on purpose
mkv VPP_TAG 'VPP_TAG=`id`'
check "VERSION with backticks is rejected" refuses vrx_parse_version "$T/V"
mkv VPP_EVIL 'VPP_EVIL=1'
check "unknown key is rejected" refuses vrx_parse_version "$T/V"
cp "$HERE/VERSION" "$T/V"; echo 'VPP_LOCAL_REV=2' >>"$T/V"
check "duplicate key is rejected" refuses vrx_parse_version "$T/V"
mkv VPP_DEB_VERSION 'VPP_DEB_VERSION=26.10-release'
check "tag/version mismatch is rejected" refuses vrx_parse_version "$T/V"
mkv VPP_COMMIT 'VPP_COMMIT=c3200b88d'
check "short commit hash is rejected" refuses vrx_parse_version "$T/V"
mkv VPP_PACKAGES_SHIP 'VPP_PACKAGES_SHIP="vpp not-built-pkg"'
check "ship list outside VPP_PACKAGES is rejected" refuses vrx_parse_version "$T/V"

# ---------------------------------------------------------------- D-089 local version (H2)
check "no patches → upstream version" test "$(vrx_local_version 26.06-release 1 0)" = 26.06-release
check "patches → +vrx<N>" test "$(vrx_local_version 26.06-release 3 2)" = 26.06-release+vrx3
check "+vrx sorts above the unpatched version (dpkg)" dpkg --compare-versions 26.06-release+vrx1 gt 26.06-release
mkdir -p "$T/vs/src/scripts"
vrx_write_version_script "$T/vs" 26.06-release+vrx1
check "generated src/scripts/version prints the local version" test "$("$T/vs/src/scripts/version")" = 26.06-release+vrx1
check "... and the upstream lib version prefix is unchanged" test "$("$T/vs/src/scripts/version" | cut -d- -f1)" = 26.06

# ---------------------------------------------------------------- path guards (M2)
R="$T/root/.build"; mkdir -p "$R/sub/x" "$T/outside"; echo m >"$R/$VRX_OWNED_MARKER"
check "guard: path inside root accepted" vrx_guard_dir "$R/sub" "$R" t
check "guard: root itself accepted as build dir" vrx_guard_dir "$R" "$R" t
check "guard: ../ escape refused" refuses vrx_guard_dir "$R/../../outside" "$R" t
check "guard: /root/vpp refused" refuses vrx_guard_dir /root/vpp "$R" t
check "guard: / refused" refuses vrx_guard_dir / / t
check "guard: \$HOME refused" refuses vrx_guard_dir "$HOME" / t
check "guard: /root (ancestor of /root/vpp) refused" refuses vrx_guard_dir /root / t
check "rm_rf: strictly below marked root works" vrx_rm_rf "$R/sub/x" "$R"
check "... and removed it" test ! -e "$R/sub/x"
check "rm_rf: the root itself refused" refuses vrx_rm_rf "$R" "$R"
check "rm_rf: outside refused" refuses vrx_rm_rf "$T/outside" "$R"
check "... outside still exists" test -d "$T/outside"
mkdir -p "$T/nomark/y"
check "rm_rf: root without ownership marker refused" refuses vrx_rm_rf "$T/nomark/y" "$T/nomark"
mkdir -p "$R/o1" && touch "$R/o1/vpp_1_amd64.deb" "$R/o1/SHA256SUMS"
check "out dir with only artefacts is ours" vrx_out_dir_is_ours "$R/o1"
touch "$R/o1/notes.txt"
check "out dir with a foreign file is refused" refuses vrx_out_dir_is_ours "$R/o1"
for bad in "--build-dir /root/vpp" "--build-dir /root/vpp/build-root" "--build-dir /tmp" "--build-dir $HERE/.build/../.." \
           "--out /" "--out $HOME" "--out /root/vpp/build-root" "--out $HERE" "--out $HERE/.build/../x"; do
  # shellcheck disable=SC2086  # the option pairs are split on purpose
  run_capture "$HERE/build.sh" $bad --prepare-only
  check "build.sh $bad refuses before doing anything" out_has 'refusing|must resolve inside'
done

# ---------------------------------------------------------------- strict patch apply (M1)
G="$T/g"; mkdir -p "$G"; git -C "$G" init -q
printf '%s\n' one two three four five six seven >"$G/f.txt"; git -C "$G" add f.txt
git -C "$G" -c user.name=t -c user.email=t@t commit -qm init
cat >"$T/good.patch" <<'EOF'
Subject: [PATCH] test
--- a/f.txt
+++ b/f.txt
@@ -2,5 +2,5 @@
 two
 three
-four
+FOUR
 five
 six
EOF
sed 's/^ two$/ TWO-CHANGED-CONTEXT/' "$T/good.patch" >"$T/fuzzed.patch"
check "fuzzed patch: GNU patch would accept it (the old behaviour)" patch -d "$G" -p1 --forward --batch --dry-run --quiet -i "$T/fuzzed.patch"
check "fuzzed patch: vrx_apply_patch refuses it" refuses vrx_apply_patch "$G" "$T/fuzzed.patch"
check "... and the tree is unchanged" test -z "$(git -C "$G" status --porcelain)"
check "exact patch applies" vrx_apply_patch "$G" "$T/good.patch"
check "... with the expected result" grep -qx FOUR "$G/f.txt"
check "already-applied patch is refused" refuses vrx_apply_patch "$G" "$T/good.patch"

# ---------------------------------------------------------------- dependency check propagates apt failure (M3)
check "apt-get -s failure → rc 2" bash -c "source '$HERE/lib.sh'; vrx_apt_missing vrx-nonexistent-pkg-xyz; test \$? -eq 2"
check "installed package → nothing missing" test -z "$(vrx_apt_missing bash coreutils | tr -d ' ')"

# ---------------------------------------------------------------- Python deps lock (H1)
check "pydeps.lock parses (5 entries)" test "$(vrx_pydeps_parse "$HERE/pydeps.lock" | wc -l)" -eq 5
printf 'meson==0.57.2\n' >"$T/bad.lock"
check "unhashed lock entry is rejected" refuses vrx_pydeps_parse "$T/bad.lock"
W="$T/pyroot"; mkdir -p "$W/wh"; echo m >"$W/$VRX_OWNED_MARKER"
printf 'fake wheel\n' >"$W/wh/demo_pkg-1.0-py3-none-any.whl"
h=$(vrx_sha256 "$W/wh/demo_pkg-1.0-py3-none-any.whl")
printf 'demo-pkg==1.0 --hash=sha256:%s  # demo_pkg-1.0-py3-none-any.whl https://files.pythonhosted.org/packages/xx/demo_pkg-1.0-py3-none-any.whl\n' "$h" >"$T/t.lock"
echo stale >"$W/wh/stale-9.9-py3-none-any.whl"
check "prepare (offline): cached verified file kept" vrx_pydeps_prepare "$T/t.lock" "$W/wh" "$W" 1
check "... foreign/stale file removed" test ! -e "$W/wh/stale-9.9-py3-none-any.whl"
check "verify: wheelhouse matches lock" vrx_pydeps_verify "$T/t.lock" "$W/wh"
echo tampered >>"$W/wh/demo_pkg-1.0-py3-none-any.whl"
check "verify: tampered file detected" refuses vrx_pydeps_verify "$T/t.lock" "$W/wh"
check "prepare (offline): tampered file removed and not replaced → fails" refuses vrx_pydeps_prepare "$T/t.lock" "$W/wh" "$W" 1
check "... tampered file is gone" test ! -e "$W/wh/demo_pkg-1.0-py3-none-any.whl"

# ---------------------------------------------------------------- verify.sh --require-files (L1, H2)
vrx_parse_version "$HERE/VERSION"
O="$T/out"; mkdir -p "$O"
mkdeb() {  # mkdeb <package> <version> <file>
  local d="$T/pkg/$1"; mkdir -p "$d/DEBIAN"
  printf 'Package: %s\nVersion: %s\nArchitecture: amd64\nMaintainer: t <t@t>\nDescription: test\n' "$1" "$2" >"$d/DEBIAN/control"
  dpkg-deb --root-owner-group -Zgzip --build "$d" "$O/$3" >/dev/null
}
mkout() {  # mkout <version> <patches-json> <variant> <demo:true|false>
  rm -rf "$O" "$T/pkg"; mkdir -p "$O"
  for p in $VPP_PACKAGES; do mkdeb "$p" "$1" "${p}_${1}_amd64.deb"; done
  (cd "$O" && sha256sum -- *.deb | sort -k2 >SHA256SUMS)
  local bp; bp=$(while read -r n _; do printf '{"name":"build-patches/%s","sha256":"%s","kind":"build"},' "$n" "$(vrx_sha256 "$HERE/build-patches/$n")"; done < <(vrx_series "$HERE/build-patches/series"))
  VRX_O="$O" VRX_V="$1" VRX_P="$2" VRX_VAR="$3" VRX_DEMO="$4" VRX_BP="[${bp%,}]" VRX_PY="$(vrx_pydeps_parse "$HERE/pydeps.lock")" \
  VPP_UPSTREAM_URL="$VPP_UPSTREAM_URL" VPP_TAG="$VPP_TAG" VPP_TAG_OBJECT="$VPP_TAG_OBJECT" VPP_COMMIT="$VPP_COMMIT" \
  VPP_DEB_VERSION="$VPP_DEB_VERSION" VPP_PACKAGES_SHIP="$VPP_PACKAGES_SHIP" python3 - <<'PY'
import glob, hashlib, json, os, subprocess
e = os.environ
ship = set(e["VPP_PACKAGES_SHIP"].split())
py = [dict(zip(("name", "version", "sha256", "file", "url"), l.split())) for l in e["VRX_PY"].splitlines() if l.strip()]
pk = []
for f in sorted(glob.glob(os.path.join(e["VRX_O"], "*.deb"))):
    name = subprocess.run(["dpkg-deb", "-f", f, "Package"], capture_output=True, text=True).stdout.strip()
    pk.append({"package": name, "version": e["VRX_V"], "architecture": "amd64", "file": os.path.basename(f),
               "size": os.path.getsize(f), "sha256": hashlib.sha256(open(f, "rb").read()).hexdigest(),
               "ship": name in ship, "installed_on_vrx_a": False})
m = {"schema": "vrx.vpp-debs.manifest/v2",
     "upstream": {"url": e["VPP_UPSTREAM_URL"], "branch": "x", "tag": e["VPP_TAG"], "tag_object": e["VPP_TAG_OBJECT"],
                  "commit": e["VPP_COMMIT"], "describe": "x"},
     "version": e["VRX_V"], "upstream_version": e["VPP_DEB_VERSION"], "variant": e["VRX_VAR"],
     "patches": json.loads(e["VRX_P"]), "options": {"trace_plugins": "devtools", "demo": e["VRX_DEMO"] == "true"},
     "build": {"builder_dirty": False, "build_patches": json.loads(e["VRX_BP"]),
               "inputs": {"python": py, "dpdk_meson_venv": [f"{x['name']}=={x['version']}" for x in py]}},
     "packages": pk}
json.dump(m, open(os.path.join(e["VRX_O"], "manifest.json"), "w"), indent=2)
PY
}
V="$HERE/verify.sh"
DEMO_NAME=$(vrx_series "$HERE/patches/series" | awk 'NR==1{print $1}')
DEMO_JSON="[{\"name\":\"patches/$DEMO_NAME\",\"sha256\":\"$(vrx_sha256 "$HERE/patches/$DEMO_NAME")\",\"kind\":\"demo\"}]"
BV="$VPP_DEB_VERSION+vrx$VPP_LOCAL_REV"
mkout "$BV" '[]' default false
check "default build (build-patches only) as +vrx passes --require-files (D-092)" "$V" --no-tests --require-files "$O"
check "... and passes the install gate" "$V" --no-tests --require-files "$O" --install-gate
mkout "$VPP_DEB_VERSION" '[]' default false
check "build-patches applied but unsuffixed version rejected (D-092)" refuses "$V" --no-tests --require-files "$O"
check "unsuffixed build fails the install gate" refuses "$V" --no-tests --require-files "$O" --install-gate
mkout "$BV" '[]' default false; rm "$O/vpp-dbg_${BV}_amd64.deb"
check "missing .deb detected" refuses "$V" --no-tests --require-files "$O"
mkout "$BV" '[]' default false; mkdeb vpp-extra "$BV" "vpp-extra_${BV}_amd64.deb"
check "extra .deb detected" refuses "$V" --no-tests --require-files "$O"
mkout "$BV" '[]' default false
python3 - "$O/manifest.json" <<'PY'
import json, sys
m = json.load(open(sys.argv[1]))
a = next(p for p in m["packages"] if p["package"] == "vpp"); b = next(p for p in m["packages"] if p["package"] == "vpp-plugin-dpdk")
a["file"], b["file"], a["sha256"], b["sha256"], a["size"], b["size"] = b["file"], a["file"], b["sha256"], a["sha256"], b["size"], a["size"]
json.dump(m, open(sys.argv[1], "w"))
PY
check "manifest package name not matching the .deb control field detected" refuses "$V" --no-tests --require-files "$O"
mkout "$VPP_DEB_VERSION" "$DEMO_JSON" demo true
check "demo-patched build with the unsuffixed version rejected (D-089)" refuses "$V" --no-tests --require-files "$O"
mkout "$VPP_DEB_VERSION+vrx$VPP_LOCAL_REV" "$DEMO_JSON" demo true
check "demo-patched build with +vrx passes --require-files" "$V" --no-tests --require-files "$O"
check "... but never passes the install gate" refuses "$V" --no-tests --require-files "$O" --install-gate
mkout "$VPP_DEB_VERSION+vrx$VPP_LOCAL_REV" "$DEMO_JSON" default true
check "demo patch without demo variant rejected" refuses "$V" --no-tests --require-files "$O"
mkout "$BV" '[]' default false; echo "0000  vpp_x.deb" >>"$O/SHA256SUMS"
check "SHA256SUMS with an extra entry rejected" refuses "$V" --no-tests --require-files "$O"

echo "$PASS passed, $FAILN failed"
((FAILN == 0))
