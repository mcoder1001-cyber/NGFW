#!/usr/bin/env bash
# Prepare a signed repository locally. Serving/deploying it is manager-owned.
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
[[ $# == 3 ]] || { echo 'usage: publish-apt.sh VERIFIED_VPP_OUTPUT VRX_DEB_DIRECTORY NEW_REPOSITORY_DIRECTORY' >&2; exit 2; }
VPP_OUTPUT=$(realpath -e -- "$1")
VRX_DEBS=$(realpath -e -- "$2")
OUTPUT=$(realpath -m -- "$3")
SIGNING_HOME=$(realpath -m -- "${XDG_CONFIG_HOME:-$HOME/.config}/ngfw/apt-signing")
# A publication tree must never contain a signing home, nor be nested in one.
case "$OUTPUT/" in "$SIGNING_HOME/"*) echo 'repository/signing-home overlap refused' >&2; exit 1;; esac
case "$SIGNING_HOME/" in "$OUTPUT/"*) echo 'repository/signing-home overlap refused' >&2; exit 1;; esac
case "$OUTPUT/" in "$ROOT/"*|/etc/*|/usr/*|/bin/*|/sbin/*|/var/lib/*) echo 'repository output must be outside source and system directories' >&2; exit 1;; esac
[[ $OUTPUT != / && ! -e $OUTPUT ]] || { echo 'output must be a new directory' >&2; exit 1; }
for tool in reprepro gpg dpkg-deb python3; do
  command -v "$tool" >/dev/null || { echo "missing publication tool: $tool" >&2; exit 1; }
done
"$ROOT/deploy/vpp/verify.sh" --require-files "$VPP_OUTPUT" --install-gate
VPP_VERSION=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$VPP_OUTPUT/manifest.json")
# Validate every product package and version before creating a signing key or repo.
DEBS=()
VERSION=
for package in vrx-agent vrx-api vrx-web vrx-meta; do
  MATCHES=()
  for candidate in "$VRX_DEBS"/*.deb; do
    [[ -f $candidate ]] || continue
    [[ $(dpkg-deb -f "$candidate" Package) == "$package" ]] && MATCHES+=("$candidate")
  done
  [[ ${#MATCHES[@]} == 1 ]] || { echo "expected exactly one $package package" >&2; exit 1; }
  candidate=${MATCHES[0]}
  CURRENT=$(dpkg-deb -f "$candidate" Version)
  [[ -z $VERSION || $VERSION == "$CURRENT" ]] || { echo 'mixed VRX package versions' >&2; exit 1; }
  VERSION=$CURRENT
  ARCH=$(dpkg-deb -f "$candidate" Architecture)
  [[ $ARCH == amd64 || $ARCH == all ]] || { echo 'unsupported VRX architecture' >&2; exit 1; }
  if [[ $package == vrx-meta ]]; then
    DEPENDS=$(dpkg-deb -f "$candidate" Depends)
    python3 - "$VPP_VERSION" "$DEPENDS" <<'PYDEPS'
import re, sys
version, depends = sys.argv[1:]
vpp_groups = []
for group in depends.split(','):
    names = [re.match(r'\s*([a-z0-9][a-z0-9+.-]*)', item) for item in group.split('|')]
    if any(name and name.group(1) == 'vpp' for name in names):
        vpp_groups.append(group.strip())
if len(vpp_groups) != 1 or not re.fullmatch(r'vpp\s*\(=\s*' + re.escape(version) + r'\s*\)', vpp_groups[0]):
    sys.exit('vrx-meta VPP dependency must be one exact standalone verified version')
PYDEPS
  fi
  DEBS+=("$candidate")
done
mapfile -t VPP_DEBS < <(python3 - "$VPP_OUTPUT" <<'PY'
import json, pathlib, sys
root = pathlib.Path(sys.argv[1])
manifest = json.load(open(root / 'manifest.json', encoding='utf-8'))
for package in manifest['packages']:
    if package['ship']:
        print(root / package['file'])
PY
)
[[ ${#VPP_DEBS[@]} == 7 ]] || { echo 'verified runtime package set must contain seven packages' >&2; exit 1; }
DEBS+=("${VPP_DEBS[@]}")
umask 077
install -d -m 0700 "$SIGNING_HOME"
# Secret key material and gpg diagnostics never go to the console or repository.
FINGERPRINT=$(gpg --homedir "$SIGNING_HOME" --batch --with-colons --list-secret-keys 2>/dev/null | awk -F: '$1=="fpr" && !seen {print $10; seen=1}')
if [[ -z $FINGERPRINT ]]; then
  gpg --homedir "$SIGNING_HOME" --batch --pinentry-mode loopback --passphrase '' \
    --quick-generate-key 'VRX APT signing' rsa3072 sign 2y >/dev/null 2>&1
  FINGERPRINT=$(gpg --homedir "$SIGNING_HOME" --batch --with-colons --list-secret-keys 2>/dev/null | awk -F: '$1=="fpr" && !seen {print $10; seen=1}')
fi
[[ $FINGERPRINT =~ ^[A-F0-9]{40}$ ]] || { echo 'no valid signing key' >&2; exit 1; }
mkdir -p "$OUTPUT/conf"
sed "s/@SIGNING_FINGERPRINT@/$FINGERPRINT/" "$ROOT/deploy/apt/distributions.in" > "$OUTPUT/conf/distributions"
# Public trust anchor only; private keys stay in SIGNING_HOME.
gpg --homedir "$SIGNING_HOME" --batch --export "$FINGERPRINT" > "$OUTPUT/vrx-archive-keyring.gpg"
for package in "${DEBS[@]}"; do
  GNUPGHOME=$SIGNING_HOME reprepro --basedir "$OUTPUT" includedeb resolute "$package" >/dev/null
done
gpg --homedir "$SIGNING_HOME" --batch --verify "$OUTPUT/dists/resolute/InRelease" >/dev/null 2>&1
printf '%s\n' "$VERSION" > "$OUTPUT/RELEASE-READY"
chmod -R a+rX "$OUTPUT"
echo 'Signed repository prepared. Verify with the public trust anchor before manager publication.'
