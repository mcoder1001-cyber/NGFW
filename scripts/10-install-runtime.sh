#!/usr/bin/env bash
# VRX appliance runtime packages. Nothing here is a compiler or a header file.
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }
[[ ${VRX_INSTALL_APPLIANCE:-} == 1 ]] || { echo "explicit VRX_INSTALL_APPLIANCE=1 required; never run on shared development host" >&2; exit 1; }
[[ -n ${VRX_VPP_ARTIFACTS:-} ]] || { echo "VRX_VPP_ARTIFACTS must name verified product build output" >&2; exit 1; }
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
VPP_OUTPUT=$(realpath -e -- "$VRX_VPP_ARTIFACTS")
"$ROOT/deploy/vpp/verify.sh" --require-files "$VPP_OUTPUT" --install-gate
mapfile -t DATAPLANE < <(python3 - "$VPP_OUTPUT" <<'PYVPP'
import json, pathlib, sys
root = pathlib.Path(sys.argv[1])
manifest = json.load(open(root / 'manifest.json', encoding='utf-8'))
for package in manifest['packages']:
    if package['ship']:
        print(root / package['file'])
PYVPP
)
[[ ${#DATAPLANE[@]} == 7 ]] || { echo "expected seven verified shipping VPP runtimes" >&2; exit 1; }
export DEBIAN_FRONTEND=noninteractive
NIC=(pciutils ethtool driverctl numactl hwloc rdma-core libibverbs1 ibverbs-providers)
# vfio-pci/uio_pci_generic used to need linux-modules-extra-$(uname -r); on the
# resolute generic kernel flavor that split package is gone - the modules are
# already part of linux-modules-$(uname -r)-generic, pulled in by the kernel itself.
ROUTING=(frr frr-pythontools)
VPN=(wireguard-tools openssl tpm2-tools opensc)
SERVICES=(kea-dhcp4-server kea-dhcp6-server unbound dns-root-data
          chrony snmpd libsnmp-base keepalived nftables)
CONTROL=(nodejs postgresql-18 postgresql-client-18 valkey-server nginx ca-certificates)
OBSERV=(prometheus-node-exporter rsyslog rsyslog-openssl logrotate tcpdump iproute2 iputils-ping
        traceroute mtr-tiny smartmontools lm-sensors ipmitool dmidecode)
TOOLS=(jq curl gnupg unzip rsync)

apt-get update
apt-get install -y --no-install-recommends \
  "${DATAPLANE[@]}" "${NIC[@]}" "${ROUTING[@]}" "${VPN[@]}" \
  "${SERVICES[@]}" "${CONTROL[@]}" "${OBSERV[@]}" "${TOOLS[@]}"

# Packages that actively fight the data plane or the upgrade mechanism.
apt-get purge -y network-manager unattended-upgrades snapd ufw cloud-init 2>/dev/null || true
systemctl disable --now irqbalance 2>/dev/null || true
apt-get autoremove -y

# Daemons are driven by vrx-agent, which renders their configs. Do not let them
# start with distro defaults on first boot.
systemctl disable --now frr strongswan-starter kea-dhcp4-server kea-dhcp6-server \
  unbound snmpd keepalived 2>/dev/null || true

echo
echo "Runtime packages installed."
echo "strongSwan VPP integration is supplied by the reviewed P11 vrx-strongswan package."
echo "Do not install upstream strongSwan in place of that package."
