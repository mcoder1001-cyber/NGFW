#!/usr/bin/env bash
# Configure non-VPP repositories only after verified product artifact preflight.
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
preflight_artifacts() {
  local output=${1:?artifact directory required}
  output=$(realpath -e -- "$output")
  # Reject indirect/nonregular inputs before the original verifier reads them.
  python3 - "$output" <<'PYPATHS'
import json, pathlib, re, stat, sys
root = pathlib.Path(sys.argv[1])
def regular(path):
    try:
        mode = path.lstat().st_mode
    except FileNotFoundError:
        raise SystemExit('artifact metadata/package missing')
    if not stat.S_ISREG(mode):
        raise SystemExit('nonregular or symlink artifact refused')
for name in ['manifest.json', 'SHA256SUMS']:
    regular(root / name)
if (root / 'manifest.json').stat().st_size > 1024 * 1024:
    raise SystemExit('artifact manifest too large')
manifest = json.loads((root / 'manifest.json').read_text(encoding='utf-8'))
for entry in manifest['packages']:
    name = entry.get('file', '')
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_.+~-]*\.deb', name):
        raise SystemExit('unsafe artifact filename refused')
    regular(root / name)
PYPATHS
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
# Pins come from a trusted administrator, not from the downloaded key server.
check_key_pins() {
  python3 - "${VRX_FRR_KEY_FINGERPRINTS:-}" "${VRX_NODESOURCE_KEY_FINGERPRINTS:-}" <<'PYPINS'
import re, sys
for name, value in zip(['VRX_FRR_KEY_FINGERPRINTS', 'VRX_NODESOURCE_KEY_FINGERPRINTS'], sys.argv[1:]):
    values = value.split(',')
    if not 1 <= len(values) <= 8 or len(set(values)) != len(values) or any(not re.fullmatch(r'(?:[0-9A-F]{40}|[0-9A-F]{64})', item) for item in values):
        raise SystemExit(name + ': trusted exact primary fingerprint set required (uppercase comma-separated full fingerprints)')
PYPINS
}
verify_repo_key() {
  local key=$1 expected=$2 home=$3 output=$4
  python3 - "$key" <<'PYKEYFILE'
import pathlib, stat, sys
p = pathlib.Path(sys.argv[1]); info = p.lstat()
if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= 1024 * 1024:
    raise SystemExit('key input must be a bounded nonempty regular public-key file')
PYKEYFILE
  # A private GPG home avoids user configuration and global keyring changes.
  gpg --no-options --homedir "$home" --batch --list-packets "$key" > "$home/packets"
  python3 - "$home/packets" <<'PYPACKETS'
import pathlib, sys
packets = pathlib.Path(sys.argv[1]).read_text()
if ':secret key packet:' in packets or ':secret sub key packet:' in packets:
    raise SystemExit('secret key material refused')
PYPACKETS
  gpg --no-options --homedir "$home" --batch --with-colons --with-fingerprint --show-keys "$key" > "$home/identities"
  python3 - "$home/identities" "$expected" <<'PYIDENTITY'
import pathlib, re, sys
primaries = []; pending = False
for line in pathlib.Path(sys.argv[1]).read_text().splitlines():
    fields = line.split(':'); kind = fields[0]
    if kind in ['sec', 'ssb']: raise SystemExit('secret key identity refused')
    if kind == 'pub':
        if pending or len(fields) < 2 or fields[1] in ['r', 'e']:
            raise SystemExit('invalid/revoked/expired primary key')
        pending = True
    elif kind == 'fpr' and pending:
        if len(fields) <= 9 or not re.fullmatch(r'(?:[0-9A-F]{40}|[0-9A-F]{64})', fields[9]):
            raise SystemExit('missing full primary fingerprint')
        primaries.append(fields[9]); pending = False
    elif kind in ['sub', 'uid'] and pending:
        raise SystemExit('primary fingerprint missing before key children')
if pending or not primaries or len(set(primaries)) != len(primaries) or set(primaries) != set(sys.argv[2].split(',')):
    raise SystemExit('downloaded primary key set differs from trusted pins')
PYIDENTITY
  gpg --no-options --homedir "$home" --batch --yes --dearmor --output "$output" "$key"
}
if [[ ${1:-} == --check-artifacts ]]; then
  [[ $# == 2 ]] || { echo 'usage: 00-add-repos.sh --check-artifacts DIRECTORY' >&2; exit 2; }
  preflight_artifacts "$2"
  exit 0
fi
[[ $# == 0 ]] || { echo 'unknown repository setup argument' >&2; exit 2; }
[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }
[[ -n ${VRX_VPP_ARTIFACTS:-} ]] || { echo 'VRX_VPP_ARTIFACTS required before repository setup' >&2; exit 1; }
check_key_pins
preflight_artifacts "$VRX_VPP_ARTIFACTS" >/dev/null
CODENAME="$(. /etc/os-release && echo "$VERSION_CODENAME")"
[[ "$CODENAME" == "resolute" ]] || echo "WARNING: tested on Ubuntu 26.04 (resolute); found '$CODENAME'"

# Bootstrap tools must be preinstalled; no APT or network without valid pins.
for prerequisite in curl gpg python3 install mktemp; do
  command -v "$prerequisite" >/dev/null || { echo "missing bootstrap prerequisite: $prerequisite" >&2; exit 1; }
done
repo_work=$(mktemp -d /tmp/vrx-repo-keys.XXXXXXXX)
repo_target=
trap 'rm -rf -- "$repo_work"; [[ -z "$repo_target" ]] || rm -f -- "$repo_target"' EXIT
mkdir -m 0700 "$repo_work/frr-home" "$repo_work/node-home"
curl -fsSL --max-filesize 1048576 https://deb.frrouting.org/frr/keys.gpg -o "$repo_work/frr.key"
curl -fsSL --max-filesize 1048576 https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key -o "$repo_work/node.key"
verify_repo_key "$repo_work/frr.key" "$VRX_FRR_KEY_FINGERPRINTS" "$repo_work/frr-home" "$repo_work/frr.gpg"
verify_repo_key "$repo_work/node.key" "$VRX_NODESOURCE_KEY_FINGERPRINTS" "$repo_work/node-home" "$repo_work/node.gpg"
# Both exact public-key sets must validate before any global mutation.
apt-get update
apt-get install -y ca-certificates lsb-release apt-transport-https
for name in frr nodesource; do
  source_name=$name
  [[ $name != nodesource ]] || source_name=node
  repo_target=$(mktemp "/usr/share/keyrings/.vrx-${name}.XXXXXXXX")
  install -m 0644 "$repo_work/$source_name.gpg" "$repo_target"
  target_name=$name
  [[ $name != frr ]] || target_name=frrouting
  mv -fT -- "$repo_target" "/usr/share/keyrings/$target_name.gpg"
  repo_target=
done

# VPP comes only from the verified local product manifest. Never configure an
# upstream FD.io repository or execute its installer (D-001 / TD-19).

# --- FRRouting -------------------------------------------------------------
# resolute is a new LTS (Apr 2026); if deb.frrouting.org hasn't published a
# 'resolute' suite yet, the apt-get update below 404s on this repo - fall back
# to Ubuntu's own 'frr' package (present in the resolute archive) until it does.
echo "deb [signed-by=/usr/share/keyrings/frrouting.gpg] https://deb.frrouting.org/frr ${CODENAME} frr-stable" \
  > /etc/apt/sources.list.d/frr.list

# --- Node.js 22 LTS --------------------------------------------------------
echo "deb [signed-by=/usr/share/keyrings/nodesource.gpg] https://deb.nodesource.com/node_22.x nodistro main" \
  > /etc/apt/sources.list.d/nodesource.list

apt-get update
echo "non-VPP repositories added: frr-stable, nodesource node_22.x"
echo "NOTE: Kea and Valkey need no extra repo on resolute - kea-dhcp4-server (3.0.x)"
echo "      and valkey-server are already in the Ubuntu archive. See docs/09-os-packages.md."
