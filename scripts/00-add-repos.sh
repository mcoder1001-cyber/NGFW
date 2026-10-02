#!/usr/bin/env bash
# Configure non-VPP repositories only after verified product artifact preflight.
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
preflight_artifacts() {
  local output=${1:?artifact directory required}
  output=$(realpath -e -- "$output")
  "$ROOT/deploy/vpp/verify.sh" --require-files "$output" --install-gate >&2
  # The original data parser owns the package set; never source VERSION.
  # shellcheck source=../deploy/vpp/lib.sh
  source "$ROOT/deploy/vpp/lib.sh"
  vrx_parse_version "$ROOT/deploy/vpp/VERSION"
  python3 - "$output" "$VPP_PACKAGES_SHIP" "$VPP_DEB_VERSION" <<'PYARTIFACT'
import json, pathlib, re, sys
root = pathlib.Path(sys.argv[1])
manifest = json.loads((root / 'manifest.json').read_text(encoding='utf-8'))
version = manifest.get('version', '')
if not re.fullmatch(re.escape(sys.argv[3]) + r'\+vrx[1-9][0-9]*', version):
    raise SystemExit('patched product VPP version required')
packages = [entry for entry in manifest['packages'] if entry.get('ship') is True]
expected = set(sys.argv[2].split())
if len(packages) != 7 or len(expected) != 7 or {p['package'] for p in packages} != expected:
    raise SystemExit('exact seven shipping product runtimes required')
for package in packages:
    name = package.get('file', '')
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_.+~-]*\.deb', name):
        raise SystemExit('unsafe artifact filename refused')
    if package.get('version') != version or package.get('architecture') != 'amd64':
        raise SystemExit('shipping runtime version/architecture mismatch')
    if not re.fullmatch(r'[0-9a-f]{64}', package.get('sha256', '')):
        raise SystemExit('shipping runtime digest missing')
print(json.dumps(dict(version=version, packages=sorted(packages, key=lambda p: p['package']))))
PYARTIFACT
}
if [[ ${1:-} == --check-artifacts ]]; then
  [[ $# == 2 ]] || { echo 'usage: 00-add-repos.sh --check-artifacts DIRECTORY' >&2; exit 2; }
  preflight_artifacts "$2"
  exit 0
fi
[[ $# == 0 ]] || { echo 'unknown repository setup argument' >&2; exit 2; }
[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }
[[ -n ${VRX_VPP_ARTIFACTS:-} ]] || { echo 'VRX_VPP_ARTIFACTS required before repository setup' >&2; exit 1; }
preflight_artifacts "$VRX_VPP_ARTIFACTS" >/dev/null
CODENAME="$(. /etc/os-release && echo "$VERSION_CODENAME")"
[[ "$CODENAME" == "resolute" ]] || echo "WARNING: tested on Ubuntu 26.04 (resolute); found '$CODENAME'"

apt-get update
apt-get install -y curl gnupg ca-certificates lsb-release apt-transport-https

# VPP comes only from the verified local product manifest. Never configure an
# upstream FD.io repository or execute its installer (D-001 / TD-19).

# --- FRRouting -------------------------------------------------------------
# resolute is a new LTS (Apr 2026); if deb.frrouting.org hasn't published a
# 'resolute' suite yet, the apt-get update below 404s on this repo - fall back
# to Ubuntu's own 'frr' package (present in the resolute archive) until it does.
curl -s https://deb.frrouting.org/frr/keys.gpg | tee /usr/share/keyrings/frrouting.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/frrouting.gpg] https://deb.frrouting.org/frr ${CODENAME} frr-stable" \
  > /etc/apt/sources.list.d/frr.list

# --- Node.js 22 LTS --------------------------------------------------------
curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
  | gpg --dearmor -o /usr/share/keyrings/nodesource.gpg
echo "deb [signed-by=/usr/share/keyrings/nodesource.gpg] https://deb.nodesource.com/node_22.x nodistro main" \
  > /etc/apt/sources.list.d/nodesource.list

apt-get update
echo "non-VPP repositories added: frr-stable, nodesource node_22.x"
echo "NOTE: Kea and Valkey need no extra repo on resolute - kea-dhcp4-server (3.0.x)"
echo "      and valkey-server are already in the Ubuntu archive. See docs/09-os-packages.md."
