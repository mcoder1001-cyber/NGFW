#!/usr/bin/env bash
# Build a separate, patched engine into a NEW offline root; never install on host.
set -euo pipefail
[[ $# == 2 ]] || { echo 'usage: build-engine.sh NEW_OFFLINE_ROOT TARGET_OS_RELEASE' >&2; exit 2; }
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
repo_root=$(cd -- "$script_dir/../.." && pwd -P)
[[ -n ${NGFW_HEAVY_HELD:-} ]] || exec "$repo_root/tools/heavy.sh" "$0" "$@"
artifact_root=$(realpath -m -- "$1")
[[ $artifact_root != / && ! -e $artifact_root && ! -L $1 ]] || { echo 'new offline root required' >&2; exit 2; }
for tool in curl gpg gpgv sha256sum tar make pkg-config python3 gcc getconf; do command -v "$tool" >/dev/null || { echo "missing build tool: $tool" >&2; exit 1; }; done
python3 - "$2" <<'PY'
import shlex,sys,platform
def release(path):
    result={}
    for line in open(path):
        if '=' in line and not line.startswith('#'):
            key,value=line.rstrip().split('=',1)
            words=shlex.split(value)
            if len(words)==1: result[key]=words[0]
    return {key:result.get(key) for key in ('ID','VERSION_ID')}
actual,target=release('/etc/os-release'),release(sys.argv[1])
if actual != target or not all(target.values()) or platform.machine()!='x86_64':
    raise SystemExit('builder OS/version/amd64 does not match explicit appliance target')
PY
pkg-config --exists openssl libsystemd || { echo 'OpenSSL and libsystemd development packages required in isolated builder' >&2; exit 1; }
mkdir -m700 -- "$artifact_root"
build_work=$(mktemp -d --tmpdir="${TMPDIR:-/tmp}" ngfw-ra-build.XXXXXXXX)
trap 'rm -rf -- "$build_work"' EXIT
export GNUPGHOME="$build_work/gnupg"
mkdir -m700 "$GNUPGHOME"
fingerprint=$(gpg --batch --show-keys --with-colons "$script_dir/upstream-release-key.asc" | awk -F: '$1=="fpr" {print $10;exit}')
[[ $fingerprint == 948F158A4E76A27BF3D07532DF42C170B34DBA77 ]] || { echo 'unexpected upstream release key' >&2; exit 1; }
gpg --batch --dearmor --output "$build_work/release-key.gpg" "$script_dir/upstream-release-key.asc"
for suffix in '' .sig; do curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 --max-time 120 --output "$build_work/strongswan-6.1.0.tar.bz2$suffix" "https://download.strongswan.org/strongswan-6.1.0.tar.bz2$suffix"; done
printf '%s  %s\n' fe6c97481298767213cfc2e9a1da29fdd8018d481ff4cb9cf0283099654f20d4 "$build_work/strongswan-6.1.0.tar.bz2" | sha256sum --check --status
gpgv --keyring "$build_work/release-key.gpg" "$build_work/strongswan-6.1.0.tar.bz2.sig" "$build_work/strongswan-6.1.0.tar.bz2"
tar --extract --bzip2 --file "$build_work/strongswan-6.1.0.tar.bz2" --directory "$build_work" --no-same-owner
cd "$build_work/strongswan-6.1.0"
./configure --prefix=/opt/ngfw-ra --sysconfdir=/opt/ngfw-ra/etc \
 --with-systemdsystemunitdir=/opt/ngfw-ra/lib/systemd/system \
 --disable-defaults --enable-systemd --enable-swanctl --enable-vici \
 --enable-kernel-netlink --enable-socket-default --enable-openssl \
 --enable-random --enable-nonce --enable-pem --enable-pkcs1 --enable-pkcs8 \
 --enable-x509 --enable-pubkey --enable-revocation --enable-constraints \
 --enable-eap-identity --enable-eap-mschapv2 --enable-eap-tls \
 --enable-eap-radius --enable-md4
make -j2
make DESTDIR="$artifact_root" install
mkdir -p "$artifact_root/opt/ngfw-ra/share/ngfw"
printf 'version=6.1.0\nsource_sha256=fe6c97481298767213cfc2e9a1da29fdd8018d481ff4cb9cf0283099654f20d4\nrelease_fingerprint=948F158A4E76A27BF3D07532DF42C170B34DBA77\n' > "$artifact_root/opt/ngfw-ra/share/ngfw/engine-build.txt"
python3 - "$artifact_root/opt/ngfw-ra/share/ngfw/engine-abi.json" <<'PY'
import json,platform,shlex,subprocess,sys
release={}
for line in open('/etc/os-release'):
    if '=' in line and not line.startswith('#'):
        key,value=line.rstrip().split('=',1)
        words=shlex.split(value)
        if len(words)==1: release[key]=words[0]
def version(argv):return subprocess.check_output(argv,text=True).strip()
json.dump({'format':1,'os':{key:release[key] for key in ('ID','VERSION_ID')},
 'architecture':platform.machine(),'compiler':version(['gcc','-dumpfullversion']),
 'libc':version(['getconf','GNU_LIBC_VERSION']),
 'openssl':version(['pkg-config','--modversion','openssl']),
 'systemd':version(['pkg-config','--modversion','libsystemd'])},open(sys.argv[1],'w'),sort_keys=True)
PY
mkdir -p "$artifact_root/opt/ngfw-ra/share/doc"
install -m644 COPYING "$artifact_root/opt/ngfw-ra/share/doc/COPYING"
# Artifact must be built on the explicit appliance ABI, never copied
# from a newer shared-host distribution. No host ldconfig or unit activation.
