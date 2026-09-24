#!/usr/bin/env bash
# Lab / test machine: topology, traffic generation, analysis, automation.
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }
export DEBIAN_FRONTEND=noninteractive

VIRT=(qemu-kvm libvirt-daemon-system libvirt-clients virtinst bridge-utils ovmf)
TRAFFIC=(iperf3 netperf)
ANALYSIS=(tshark tcpdump)
BASE=(python3-venv python3-pip git curl jq docker.io frr)

apt-get update
apt-get install -y "${VIRT[@]}" "${TRAFFIC[@]}" "${ANALYSIS[@]}" "${BASE[@]}"

# containerlab (official installer, not in apt)
command -v containerlab >/dev/null || bash -c "$(curl -sL https://get.containerlab.dev)"

# Test automation in a venv - never into the system Python.
python3 -m venv /opt/vrx-test
/opt/vrx-test/bin/pip install --quiet --upgrade pip
/opt/vrx-test/bin/pip install --quiet robotframework robotframework-sshlibrary scapy pytest requests

echo
echo "Lab tools installed. Activate test env: source /opt/vrx-test/bin/activate"
echo "TRex is NOT packaged - download the official tarball from Cisco:"
echo "  https://trex-tgn.cisco.com/trex/release/  (extract to /opt/trex)"
echo "Without a TRex box you cannot defend any performance number - see docs/08 CapEx, month 4."
