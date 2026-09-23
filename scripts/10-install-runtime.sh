#!/usr/bin/env bash
# VRX appliance runtime packages. Nothing here is a compiler or a header file.
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }
export DEBIAN_FRONTEND=noninteractive

DATAPLANE=(vpp vpp-plugin-core vpp-plugin-dpdk)
NIC=(pciutils ethtool driverctl numactl hwloc rdma-core libibverbs1 ibverbs-providers)
# vfio-pci/uio_pci_generic used to need linux-modules-extra-$(uname -r); on the
# resolute generic kernel flavor that split package is gone - the modules are
# already part of linux-modules-$(uname -r)-generic, pulled in by the kernel itself.
ROUTING=(frr frr-pythontools)
VPN=(wireguard-tools openssl tpm2-tools opensc)
SERVICES=(kea-dhcp4-server kea-dhcp6-server kea-ctrl-agent unbound dns-root-data
          chrony snmpd libsnmp-base keepalived nftables)
CONTROL=(nodejs postgresql-18 postgresql-client-18 valkey-server nginx ca-certificates)
OBSERV=(prometheus-node-exporter rsyslog logrotate tcpdump iproute2 iputils-ping
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
  kea-ctrl-agent unbound snmpd keepalived 2>/dev/null || true

echo
echo "Runtime packages installed."
echo "STILL MISSING - strongSwan with the VPP plugins."
echo "  The Ubuntu package has no kernel-vpp/socket-vpp support. Build it from source"
echo "  (--enable-kernel-vpp --enable-socket-vpp), package it as your own .deb, and"
echo "  install that instead. See scripts/25-build-strongswan-vpp.sh."
