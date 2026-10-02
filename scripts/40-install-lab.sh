#!/usr/bin/env bash
# Lab / test machine: topology, traffic generation, analysis, automation.
set -euo pipefail
# Official srl-labs/containerlab release asset digest, checked 2026-10-02:
# https://api.github.com/repos/srl-labs/containerlab/releases/tags/v0.79.0
CONTAINERLAB_VER=0.79.0
CONTAINERLAB_SHA256=f90d36d58bb6c4afd3b3a4dca006b81594c6d16f7a04be0184b03f44291085a2
CONTAINERLAB_URL="https://github.com/srl-labs/containerlab/releases/download/v${CONTAINERLAB_VER}/containerlab_${CONTAINERLAB_VER}_linux_amd64.tar.gz"
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || {
  echo "only linux-amd64 lab bootstrap is supported" >&2; exit 1;
}
if [[ ${1:-} == --check-config && $# == 1 ]]; then
  printf 'containerlab=%s sha256=%s\n' "$CONTAINERLAB_VER" "$CONTAINERLAB_SHA256"
  exit 0
fi
[[ $# == 0 ]] || { echo "usage: $0 [--check-config]" >&2; exit 1; }
[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }
export DEBIAN_FRONTEND=noninteractive

VIRT=(qemu-kvm libvirt-daemon-system libvirt-clients virtinst bridge-utils ovmf)
TRAFFIC=(iperf3 netperf)
ANALYSIS=(tshark tcpdump)
BASE=(python3-venv python3-pip git curl jq docker.io frr)

# Bootstrap prerequisites must already exist; do not run APT before validating
# the fixed archive. The temporary directory is private and never reused.
for prerequisite in curl python3 sha256sum install mktemp; do
  command -v "$prerequisite" >/dev/null || { echo "missing bootstrap prerequisite: $prerequisite" >&2; exit 1; }
done
# BEGIN VERIFIED CONTAINERLAB
containerlab_work=$(mktemp -d /tmp/vrx-containerlab.XXXXXXXX)
containerlab_target=
trap 'rm -rf -- "$containerlab_work"; [[ -z "$containerlab_target" ]] || rm -f -- "$containerlab_target"' EXIT
curl -fsSL "$CONTAINERLAB_URL" -o "$containerlab_work/archive.tar.gz"
printf '%s  %s\n' "$CONTAINERLAB_SHA256" "$containerlab_work/archive.tar.gz" | sha256sum --check --status
# Extract only the exact regular binary, never archive paths or links.
python3 - "$containerlab_work" <<'PYBIN'
import pathlib, shutil, sys, tarfile
root = pathlib.Path(sys.argv[1])
with tarfile.open(root / "archive.tar.gz", "r:gz") as archive:
    selected = [member for member in archive.getmembers() if member.name == "containerlab"]
    if len(selected) != 1 or not selected[0].isreg() or not 0 < selected[0].size <= 256 * 1024 * 1024:
        raise SystemExit("archive must contain one bounded regular containerlab binary")
    with archive.extractfile(selected[0]) as source, (root / "containerlab").open("xb") as target:
        shutil.copyfileobj(source, target)
PYBIN
# Only verified archive content reaches package installation or target changes.
apt-get update
apt-get install -y "${VIRT[@]}" "${TRAFFIC[@]}" "${ANALYSIS[@]}" "${BASE[@]}"
install -d -m 0755 /usr/local/bin
containerlab_target=$(mktemp /usr/local/bin/.containerlab.XXXXXXXX)
install -m 0755 "$containerlab_work/containerlab" "$containerlab_target"
mv -fT -- "$containerlab_target" /usr/local/bin/containerlab
containerlab_target=
# END VERIFIED CONTAINERLAB

# Test automation in a venv - never into the system Python.
python3 -m venv /opt/vrx-test
/opt/vrx-test/bin/pip install --quiet --upgrade pip
/opt/vrx-test/bin/pip install --quiet robotframework robotframework-sshlibrary scapy pytest requests

echo
echo "Lab tools installed. Activate test env: source /opt/vrx-test/bin/activate"
echo "TRex is NOT packaged - download the official tarball from Cisco:"
echo "  https://trex-tgn.cisco.com/trex/release/  (extract to /opt/trex)"
echo "Without a TRex box you cannot defend any performance number - see docs/08 CapEx, month 4."
