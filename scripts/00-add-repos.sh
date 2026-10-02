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
# GnuPG doc/DETAILS: support ordinary unknown/undefined and valid trust levels.
# Invalid, disabled, revoked, expired, not-valid and special/unknown states fail closed.
primaries = []; pending = False
allowed_validity = {'-', 'o', 'q', 'm', 'f', 'u'}
for line in pathlib.Path(sys.argv[1]).read_text().splitlines():
    fields = line.split(':'); kind = fields[0]
    if kind in ['sec', 'ssb']: raise SystemExit('secret key identity refused')
    if kind == 'pub':
        if (pending or len(fields) < 12 or fields[1] not in allowed_validity
                or not re.fullmatch(r'[1-9][0-9]*', fields[2])
                or not re.fullmatch(r'[1-9][0-9]*', fields[3])
                or not re.fullmatch(r'[0-9A-F]{16}', fields[4])
                or not re.fullmatch(r'[1-9][0-9]*', fields[5])
                or (fields[6] and not re.fullmatch(r'[0-9]+', fields[6]))
                or not re.fullmatch(r'[escaESCA]+', fields[11])
                or not ('s' in fields[11] or 'S' in fields[11])):
            raise SystemExit('unsupported validity or malformed/disabled/non-signing primary key')
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
# Distinct FRR preprocessing policy; the raw exact-set verifier stays unchanged.
check_frr_selection_pins() {
  python3 - "$1" <<'PYFRRPINS'
import re, sys
pins = sys.argv[1].split(',')
if not 1 <= len(pins) <= 8 or len(set(pins)) != len(pins) or any(not re.fullmatch('[0-9A-F]{40}', pin) for pin in pins):
    raise SystemExit('FRR selection requires explicit authorized full40 primary fingerprints')
PYFRRPINS
}
select_frr_certificates() (
  set -euo pipefail
  local key=$1 expected=$2 output=$3 selection_work
  check_frr_selection_pins "$expected"
  python3 - "$key" <<'PYFRRRAW'
import pathlib, stat, sys
info = pathlib.Path(sys.argv[1]).lstat()
if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= 1024 * 1024:
    raise SystemExit('raw FRR input must be a bounded nonempty regular public-key file')
PYFRRRAW
  selection_work=$(mktemp -d /tmp/vrx-frr-selection.XXXXXXXX)
  trap 'rm -rf -- "$selection_work"' EXIT
  mkdir -m 0700 "$selection_work/import" "$selection_work/verify"
  # Inspect ALL raw material before importing or discarding unselected primaries.
  gpg --no-options --homedir "$selection_work/import" --batch --no-auto-key-retrieve --auto-key-locate clear --list-packets "$key" > "$selection_work/packets"
  python3 - "$selection_work/packets" <<'PYFRRPACKETS'
import pathlib, sys
packets = pathlib.Path(sys.argv[1]).read_text()
if ':secret key packet:' in packets or ':secret sub key packet:' in packets:
    raise SystemExit('raw FRR secret key material refused')
PYFRRPACKETS
  gpg --no-options --homedir "$selection_work/import" --batch --no-auto-key-retrieve --auto-key-locate clear --with-colons --with-fingerprint --show-keys "$key" > "$selection_work/identities"
  python3 - "$selection_work/identities" <<'PYFRRIDENTITY'
import pathlib, sys
if any(line.split(':', 1)[0] in {'sec', 'ssb'} for line in pathlib.Path(sys.argv[1]).read_text().splitlines()):
    raise SystemExit('raw FRR secret identity refused')
PYFRRIDENTITY
  # ANY nonzero import aborts, even if GPG partially populated its private ring.
  gpg --no-options --homedir "$selection_work/import" --batch --no-auto-key-retrieve --auto-key-locate clear --import "$key"
  local -a fingerprints
  IFS=',' read -r -a fingerprints <<< "$expected"
  # Complete public export: no clean/minimal/filter options that lose revocations.
  gpg --no-options --homedir "$selection_work/import" --batch --no-auto-key-retrieve --auto-key-locate clear --export "${fingerprints[@]}" > "$selection_work/selected.key"
  verify_repo_key "$selection_work/selected.key" "$expected" "$selection_work/verify" "$selection_work/verified.gpg"
  # Publish only validated bytes to our caller's private staging directory.
  python3 - "$selection_work/verified.gpg" "$output" <<'PYFRROUT'
import os, pathlib, stat, sys
source, output = map(pathlib.Path, sys.argv[1:])
parent = output.parent.lstat()
if not stat.S_ISDIR(parent.st_mode) or parent.st_uid != os.geteuid() or stat.S_IMODE(parent.st_mode) != 0o700:
    raise SystemExit('FRR selected output requires an owned private staging directory')
data = source.read_bytes()
if not 0 < len(data) <= 1024 * 1024:
    raise SystemExit('verified FRR output must remain bounded')
fd = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600)
try:
    with os.fdopen(fd, 'wb') as target:
        target.write(data)
        target.flush()
        os.fsync(target.fileno())
except BaseException:
    output.unlink()
    raise
PYFRROUT
)
if [[ ${1:-} == --check-artifacts ]]; then
  [[ $# == 2 ]] || { echo 'usage: 00-add-repos.sh --check-artifacts DIRECTORY' >&2; exit 2; }
  preflight_artifacts "$2"
  exit 0
fi
[[ $# == 0 ]] || { echo 'unknown repository setup argument' >&2; exit 2; }
[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }
[[ -n ${VRX_VPP_ARTIFACTS:-} ]] || { echo 'VRX_VPP_ARTIFACTS required before repository setup' >&2; exit 1; }
check_key_pins
check_frr_selection_pins "$VRX_FRR_KEY_FINGERPRINTS"
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
curl -fsSL --connect-timeout 10 --max-time 60 --max-filesize 1048576 https://deb.frrouting.org/frr/keys.gpg -o "$repo_work/frr.key"
curl -fsSL --connect-timeout 10 --max-time 60 --max-filesize 1048576 https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key -o "$repo_work/node.key"
select_frr_certificates "$repo_work/frr.key" "$VRX_FRR_KEY_FINGERPRINTS" "$repo_work/frr.gpg"
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
