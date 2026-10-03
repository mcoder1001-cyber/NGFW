#!/usr/bin/env bash
# Lab / test machine: topology, traffic generation, analysis, automation.
set -euo pipefail
# Official srl-labs/containerlab release asset digest, checked 2026-10-02:
# https://api.github.com/repos/srl-labs/containerlab/releases/tags/v0.79.0
CONTAINERLAB_VER=0.79.0
CONTAINERLAB_SHA256=a399d92a622b4664d8d1231bc9b7f53a1d210255a0306fa091c3f63779f65f13
CONTAINERLAB_URL="https://github.com/srl-labs/containerlab/releases/download/v${CONTAINERLAB_VER}/containerlab_${CONTAINERLAB_VER}_linux_amd64.deb"
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
for prerequisite in curl dpkg-deb dpkg-query sha256sum mktemp; do
  command -v "$prerequisite" >/dev/null || { echo "missing bootstrap prerequisite: $prerequisite" >&2; exit 1; }
done
# BEGIN VERIFIED CONTAINERLAB
containerlab_work=$(mktemp -d /tmp/ngfw-containerlab.XXXXXXXX)
trap 'rm -rf -- "$containerlab_work"' EXIT
containerlab_deb="$containerlab_work/containerlab_${CONTAINERLAB_VER}_linux_amd64.deb"
curl -fsSL "$CONTAINERLAB_URL" -o "$containerlab_deb"
printf '%s  %s\n' "$CONTAINERLAB_SHA256" "$containerlab_deb" | sha256sum --check --status
# Never inspect or install an unverified package. Fields are DATA, not commands.
[[ "$(dpkg-deb -f "$containerlab_deb" Package)" == containerlab &&
   "$(dpkg-deb -f "$containerlab_deb" Version)" == "$CONTAINERLAB_VER" &&
   "$(dpkg-deb -f "$containerlab_deb" Architecture)" == amd64 ]] || {
  echo "REFUSED: containerlab Debian package identity does not match the pin" >&2
  exit 1
}
apt-get update
apt-get install -y "${VIRT[@]}" "${TRAFFIC[@]}" "${ANALYSIS[@]}" "${BASE[@]}" "$containerlab_deb"
[[ "$(dpkg-query -W -f='${Version}' containerlab)" == "$CONTAINERLAB_VER" ]] || {
  echo "REFUSED: installed containerlab version differs from the pin" >&2
  exit 1
}
# END VERIFIED CONTAINERLAB

# Test automation in a venv - never into the system Python.
python3 -m venv /opt/ngfw-test
/opt/ngfw-test/bin/pip install --quiet --upgrade pip
/opt/ngfw-test/bin/pip install --quiet robotframework robotframework-sshlibrary scapy pytest requests

echo
echo "Lab tools installed. Activate test env: source /opt/ngfw-test/bin/activate"
echo "TRex is NOT packaged - download the official tarball from Cisco:"
echo "  https://trex-tgn.cisco.com/trex/release/  (extract to /opt/trex)"
echo "Without a TRex box you cannot defend any performance number - see docs/08 CapEx, month 4."
