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
  ngfw_parse_version "$ROOT/deploy/vpp/VERSION"
  python3 - "$output" "$VPP_PACKAGES_SHIP" "$VPP_DEB_VERSION" <<'PYARTIFACT'
import json, pathlib, re, sys
root = pathlib.Path(sys.argv[1])
manifest = json.loads((root / 'manifest.json').read_text(encoding='utf-8'))
version = manifest.get('version', '')
if not re.fullmatch(re.escape(sys.argv[3]) + r'\+ngfw[1-9][0-9]*', version):
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
# Pins are owner-authorized source constants (D-238), never taken from the
# downloaded key server; an optional administrator override must match exactly.
check_key_pins() {
  python3 - "${NGFW_FRR_KEY_FINGERPRINTS:-}" "${NGFW_NODESOURCE_KEY_FINGERPRINTS:-}" <<'PYPINS'
import re, sys
for name, value in zip(['NGFW_FRR_KEY_FINGERPRINTS', 'NGFW_NODESOURCE_KEY_FINGERPRINTS'], sys.argv[1:]):
    values = value.split(',')
    if not 1 <= len(values) <= 8 or len(set(values)) != len(values) or any(not re.fullmatch(r'(?:[0-9A-F]{40}|[0-9A-F]{64})', item) for item in values):
        raise SystemExit('REFUSED: ' + name + ': trusted exact primary fingerprint set required (uppercase comma-separated full fingerprints equal to the owner-authorized D-238 pins)')
PYPINS
}
# The single primary-identity parser for every trust decision (D-238): raw
# downloads and canonical exports apply identical validity, capability and
# fingerprint rules. Mode "canonical" tolerates repeated copies of one pinned
# primary (only for raw input that the private import/export canonicalizes);
# mode "exact" refuses duplicates.
check_primary_set() {
  local identities=$1 expected=$2 label=$3 mode=$4
  python3 - "$identities" "$expected" "$label" "$mode" <<'PYIDENTITY'
import pathlib, re, sys
# GnuPG doc/DETAILS: support ordinary unknown/undefined and valid trust levels.
# Invalid, disabled, revoked, expired, not-valid and special/unknown states fail closed.
identities, expected, label, mode = sys.argv[1:]
if mode not in {'canonical', 'exact'}:
    raise SystemExit('internal: unknown primary-set mode')
primaries = []; pending = False
allowed_validity = {'-', 'o', 'q', 'm', 'f', 'u'}
for line in pathlib.Path(identities).read_text().splitlines():
    fields = line.split(':'); kind = fields[0]
    if kind in ['sec', 'ssb']: raise SystemExit(label + ': secret key identity refused')
    if kind == 'pub':
        if (pending or len(fields) < 12 or fields[1] not in allowed_validity
                or not re.fullmatch(r'[1-9][0-9]*', fields[2])
                or not re.fullmatch(r'[1-9][0-9]*', fields[3])
                or not re.fullmatch(r'[0-9A-F]{16}', fields[4])
                or not re.fullmatch(r'[1-9][0-9]*', fields[5])
                or (fields[6] and not re.fullmatch(r'[0-9]+', fields[6]))
                or not re.fullmatch(r'[escaESCA]+', fields[11])
                or not ('s' in fields[11] or 'S' in fields[11])):
            raise SystemExit(label + ': unsupported validity or malformed/disabled/non-signing primary key')
        pending = True
    elif kind == 'fpr' and pending:
        if len(fields) <= 9 or not re.fullmatch(r'(?:[0-9A-F]{40}|[0-9A-F]{64})', fields[9]):
            raise SystemExit(label + ': missing full primary fingerprint')
        primaries.append(fields[9]); pending = False
    elif kind in ['sub', 'uid'] and pending:
        raise SystemExit(label + ': primary fingerprint missing before key children')
duplicated = len(set(primaries)) != len(primaries)
if pending or not primaries or (mode == 'exact' and duplicated) or set(primaries) != set(expected.split(',')):
    raise SystemExit('REFUSED: downloaded ' + label + ' primary key set differs from the owner-authorized D-238 pins; changed identities require a new owner decision')
PYIDENTITY
}
verify_repo_key() {
  local key=$1 expected=$2 home=$3 output=$4 label=${5:-repository}
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
  check_primary_set "$home/identities" "$expected" "$label" exact
  gpg --no-options --homedir "$home" --batch --yes --dearmor --output "$output" "$key"
}
# Selection preprocessing policy; the exact-set verifier stays the authority.
check_selection_pins() {
  python3 - "$1" "${2:-FRR}" <<'PYSELECTPINS'
import re, sys
pins = sys.argv[1].split(',')
if not 1 <= len(pins) <= 8 or len(set(pins)) != len(pins) or any(not re.fullmatch('[0-9A-F]{40}', pin) for pin in pins):
    raise SystemExit(sys.argv[2] + ' selection requires explicit authorized full40 primary fingerprints')
PYSELECTPINS
}
# Only GnuPG-validated material of the pinned primaries reaches a keyring:
# private import, complete export of exactly the pins, then exact verification.
select_pinned_certificates() (
  set -euo pipefail
  local key=$1 expected=$2 output=$3 label=$4 selection_work
  check_selection_pins "$expected" "$label"
  selection_work=$(mktemp -d "${work_dir:-${TMPDIR:-/tmp}}"/ngfw-frr-selection.XXXXXXXX)
  trap 'rm -rf -- "$selection_work"' EXIT
  mkdir -m 0700 "$selection_work/import" "$selection_work/verify"
  # Open once without following the caller's final symlink; all GPG operations
  # consume this bounded private snapshot rather than reopening caller input.
  python3 - "$key" "$selection_work/raw.key" "$label" <<'PYFRRRAW'
import os, pathlib, stat, sys
key = pathlib.Path(sys.argv[1]); parent = key.parent.lstat(); label = sys.argv[3]
if (not stat.S_ISDIR(parent.st_mode) or parent.st_uid != os.geteuid()
        or stat.S_IMODE(parent.st_mode) != 0o700):
    raise SystemExit('raw ' + label + ' input requires an owned private staging directory')
fd = os.open(key, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC | os.O_NONBLOCK)
try:
    before = os.fstat(fd)
    if (not stat.S_ISREG(before.st_mode) or before.st_uid != os.geteuid()
            or not 0 < before.st_size <= 1024 * 1024):
        raise SystemExit('raw ' + label + ' input must be a bounded nonempty regular public-key file')
    with os.fdopen(fd, 'rb', closefd=False) as source:
        data = source.read(1024 * 1024 + 1)
    after = os.fstat(fd)
    identity = lambda info: (info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns)
    if not 0 < len(data) <= 1024 * 1024 or len(data) != before.st_size or identity(before) != identity(after):
        raise SystemExit('raw ' + label + ' input changed during bounded snapshot')
    target_fd = os.open(sys.argv[2], os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600)
    with os.fdopen(target_fd, 'wb') as target:
        target.write(data)
finally:
    os.close(fd)
PYFRRRAW
  key="$selection_work/raw.key"
  # Inspect ALL raw material before importing or discarding unselected primaries.
  gpg --no-options --homedir "$selection_work/import" --batch --no-auto-key-retrieve --auto-key-locate clear --list-packets "$key" > "$selection_work/packets"
  python3 - "$selection_work/packets" "$label" <<'PYFRRPACKETS'
import pathlib, sys
packets = pathlib.Path(sys.argv[1]).read_text()
if ':secret key packet:' in packets or ':secret sub key packet:' in packets:
    raise SystemExit('raw ' + sys.argv[2] + ' secret key material refused')
PYFRRPACKETS
  gpg --no-options --homedir "$selection_work/import" --batch --no-auto-key-retrieve --auto-key-locate clear --with-colons --with-fingerprint --show-keys "$key" > "$selection_work/identities"
  python3 - "$selection_work/identities" "$label" <<'PYFRRIDENTITY'
import pathlib, sys
if any(line.split(':', 1)[0] in {'sec', 'ssb'} for line in pathlib.Path(sys.argv[1]).read_text().splitlines()):
    raise SystemExit('raw ' + sys.argv[2] + ' secret identity refused')
PYFRRIDENTITY
  # ANY nonzero import aborts, even if GPG partially populated its private ring.
  gpg --no-options --homedir "$selection_work/import" --batch --no-auto-key-retrieve --auto-key-locate clear --import "$key"
  local -a fingerprints
  IFS=',' read -r -a fingerprints <<< "$expected"
  # Complete public export: no clean/minimal/filter options that lose revocations.
  gpg --no-options --homedir "$selection_work/import" --batch --no-auto-key-retrieve --auto-key-locate clear --export "${fingerprints[@]}" > "$selection_work/selected.key"
  verify_repo_key "$selection_work/selected.key" "$expected" "$selection_work/verify" "$selection_work/verified.gpg" "$label"
  # Publish only validated bytes to our caller's private staging directory.
  python3 - "$selection_work/verified.gpg" "$output" <<'PYFRROUT'
import os, pathlib, stat, sys
source, output = map(pathlib.Path, sys.argv[1:])
parent = output.parent.lstat()
if not stat.S_ISDIR(parent.st_mode) or parent.st_uid != os.geteuid() or stat.S_IMODE(parent.st_mode) != 0o700:
    raise SystemExit('selected output requires an owned private staging directory')
data = source.read_bytes()
if not 0 < len(data) <= 1024 * 1024:
    raise SystemExit('verified selected output must remain bounded')
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
select_frr_certificates() { select_pinned_certificates "$1" "$2" "$3" FRR; }
# D-238 (owner, 2026-10-06): the official HTTPS key endpoints are initial trust
# anchors for exactly these observed primaries (TD-19 trust material 2026-10-04).
# A changed, extra or missing identity is never accepted automatically; a new
# set requires a new recorded owner decision and a reviewed source change.
readonly NGFW_FRR_AUTHORIZED_PRIMARIES=4A56C7738BB3F81595A805D2A832769908F13ED1,3D9968AC9AE7BE1169288DDB1FD5839895F57FDA,BBC9ACA9D13025A2C186FF7F741E92A1F6E3975B,A90FC36D9429409798E9C2D874DEED43AB194DBF
readonly NGFW_NODESOURCE_AUTHORIZED_PRIMARIES=6F71F525282841EEDAF851B42F59B5F99B1BE0B4
require_authorized_pins() {
  python3 - "$NGFW_FRR_KEY_FINGERPRINTS" "$NGFW_FRR_AUTHORIZED_PRIMARIES" "$NGFW_NODESOURCE_KEY_FINGERPRINTS" "$NGFW_NODESOURCE_AUTHORIZED_PRIMARIES" <<'PYAUTHORIZED'
import sys
for name, value, authorized in (('NGFW_FRR_KEY_FINGERPRINTS', *sys.argv[1:3]), ('NGFW_NODESOURCE_KEY_FINGERPRINTS', *sys.argv[3:5])):
    pins, allowed = value.split(','), authorized.split(',')
    if len(set(pins)) != len(pins) or set(pins) != set(allowed):
        raise SystemExit('REFUSED: ' + name + ' differs from the owner-authorized D-238 primary set; changed identities require a new owner decision')
PYAUTHORIZED
}
# Raw download gate before selection: the downloaded bundle must carry exactly
# the authorized primaries, under the same parser and rules as the final check
# (check_primary_set). FRR repeats one authorized certificate, so FRR uses
# "canonical" (copies merged by the private import/export); NodeSource "exact".
check_raw_primaries() {
  local key=$1 expected=$2 home=$3 label=$4 mode=$5
  gpg --no-options --homedir "$home" --batch --no-auto-key-retrieve --auto-key-locate clear --with-colons --with-fingerprint --show-keys "$key" > "$home/raw-identities"
  check_primary_set "$home/raw-identities" "$expected" "$label" "$mode"
}
if [[ ${1:-} == --check-artifacts ]]; then
  [[ $# == 2 ]] || { echo 'usage: 00-add-repos.sh --check-artifacts DIRECTORY' >&2; exit 2; }
  preflight_artifacts "$2"
  exit 0
fi
# shellcheck source=install-common.sh
source "$ROOT/scripts/install-common.sh"
ngfw_install_init "$@"
[[ -n ${NGFW_VPP_ARTIFACTS:-} ]] || { echo 'NGFW_VPP_ARTIFACTS required before repository setup' >&2; exit 1; }
NGFW_FRR_KEY_FINGERPRINTS=${NGFW_FRR_KEY_FINGERPRINTS-$NGFW_FRR_AUTHORIZED_PRIMARIES}
NGFW_NODESOURCE_KEY_FINGERPRINTS=${NGFW_NODESOURCE_KEY_FINGERPRINTS-$NGFW_NODESOURCE_AUTHORIZED_PRIMARIES}
check_key_pins
require_authorized_pins
check_selection_pins "$NGFW_FRR_KEY_FINGERPRINTS" FRR
check_selection_pins "$NGFW_NODESOURCE_KEY_FINGERPRINTS" NodeSource
if [[ $NGFW_INSTALL_DRY_RUN == 1 ]]; then
  ngfw_install_plan 'verify seven signed product artifacts; verify both D-238 exact repository key sets; APT bootstrap; write /usr/share/keyrings and /etc/apt/sources.list.d; APT refresh (no FD.io repo)'
  exit 0
fi
preflight_artifacts "$NGFW_VPP_ARTIFACTS" >/dev/null
os_release=$(ngfw_install_path /etc/os-release)
keyring_dir=$(ngfw_install_path /usr/share/keyrings)
sources_dir=$(ngfw_install_path /etc/apt/sources.list.d)
work_dir=$(ngfw_install_path /tmp)
mkdir -p "$work_dir"
# shellcheck disable=SC1090
CODENAME="$(. "$os_release" && echo "$VERSION_CODENAME")"
[[ "$CODENAME" == "resolute" ]] || echo "WARNING: tested on Ubuntu 26.04 (resolute); found '$CODENAME'"

# Bootstrap tools must be preinstalled; no APT or network without authorized pins.
for prerequisite in curl gpg python3 install mktemp; do
  command -v "$prerequisite" >/dev/null || { echo "missing bootstrap prerequisite: $prerequisite" >&2; exit 1; }
done
repo_work=$(mktemp -d "$work_dir"/ngfw-repo-keys.XXXXXXXX)
repo_target=
trap 'rm -rf -- "$repo_work"; [[ -z "$repo_target" ]] || rm -f -- "$repo_target"' EXIT
mkdir -m 0700 "$repo_work/frr-home" "$repo_work/node-home"
curl -fsSL --connect-timeout 10 --max-time 60 --max-filesize 1048576 https://deb.frrouting.org/frr/keys.gpg -o "$repo_work/frr.key"
curl -fsSL --connect-timeout 10 --max-time 60 --max-filesize 1048576 https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key -o "$repo_work/node.key"
check_raw_primaries "$repo_work/frr.key" "$NGFW_FRR_KEY_FINGERPRINTS" "$repo_work/frr-home" FRR canonical
select_pinned_certificates "$repo_work/frr.key" "$NGFW_FRR_KEY_FINGERPRINTS" "$repo_work/frr.gpg" FRR
check_raw_primaries "$repo_work/node.key" "$NGFW_NODESOURCE_KEY_FINGERPRINTS" "$repo_work/node-home" NodeSource exact
select_pinned_certificates "$repo_work/node.key" "$NGFW_NODESOURCE_KEY_FINGERPRINTS" "$repo_work/node.gpg" NodeSource
# Both exact public-key sets must validate before any global mutation.
apt-get update
apt-get install -y ca-certificates lsb-release apt-transport-https
mkdir -p "$keyring_dir" "$sources_dir"
for name in frr nodesource; do
  source_name=$name
  [[ $name != nodesource ]] || source_name=node
  repo_target=$(mktemp "$keyring_dir/.ngfw-${name}.XXXXXXXX")
  install -m 0644 "$repo_work/$source_name.gpg" "$repo_target"
  target_name=$name
  [[ $name != frr ]] || target_name=frrouting
  mv -fT -- "$repo_target" "$keyring_dir/$target_name.gpg"
  repo_target=
done

# VPP comes only from the verified local product manifest. Never configure an
# upstream FD.io repository or execute its installer (D-001 / TD-19).

# --- FRRouting -------------------------------------------------------------
# resolute is a new LTS (Apr 2026); if deb.frrouting.org hasn't published a
# 'resolute' suite yet, the apt-get update below 404s on this repo - fall back
# to Ubuntu's own 'frr' package (present in the resolute archive) until it does.
echo "deb [signed-by=/usr/share/keyrings/frrouting.gpg] https://deb.frrouting.org/frr ${CODENAME} frr-stable" \
  > "$sources_dir/frr.list"

# --- Node.js 22 LTS --------------------------------------------------------
echo "deb [signed-by=/usr/share/keyrings/nodesource.gpg] https://deb.nodesource.com/node_22.x nodistro main" \
  > "$sources_dir/nodesource.list"

apt-get update
echo "non-VPP repositories added: frr-stable, nodesource node_22.x"
echo "NOTE: Kea and Valkey need no extra repo on resolute - kea-dhcp4-server (3.0.x)"
echo "      and valkey-server are already in the Ubuntu archive. See docs/09-os-packages.md."
