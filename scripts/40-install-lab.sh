#!/usr/bin/env bash
# VMware lab / test machine: native traffic tools, analysis, automation.
set -euo pipefail
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || {
  echo "only linux-amd64 lab bootstrap is supported" >&2; exit 1;
}
if [[ ${1:-} == --check-config && $# == 1 ]]; then
  printf 'lab=vmware native-tools=iperf3,netperf,tshark,tcpdump,frr\n'
  exit 0
fi
[[ $# == 0 ]] || { echo "usage: $0 [--check-config]" >&2; exit 1; }
[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }
export DEBIAN_FRONTEND=noninteractive

TRAFFIC=(iperf3 netperf)
ANALYSIS=(tshark tcpdump)
BASE=(python3-venv python3-pip git curl jq frr)

apt-get update
apt-get install -y "${TRAFFIC[@]}" "${ANALYSIS[@]}" "${BASE[@]}"

# Test automation in a venv - never into the system Python.
python3 -m venv /opt/ngfw-test
/opt/ngfw-test/bin/pip install --quiet --upgrade pip
/opt/ngfw-test/bin/pip install --quiet robotframework robotframework-sshlibrary scapy pytest requests

echo
echo "Lab tools installed. Activate test env: source /opt/ngfw-test/bin/activate"
echo "TRex is NOT packaged - download the official tarball from Cisco:"
echo "  https://trex-tgn.cisco.com/trex/release/  (extract to /opt/trex)"
echo "Without a TRex box you cannot defend any performance number - see docs/08 CapEx, month 4."
