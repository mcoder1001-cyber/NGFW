#!/usr/bin/env bash
# NGFW appliance runtime packages. Nothing here is a compiler or a header file.
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }
[[ ${NGFW_INSTALL_APPLIANCE:-} == 1 ]] || { echo "explicit NGFW_INSTALL_APPLIANCE=1 required; never run on shared development host" >&2; exit 1; }
[[ -n ${NGFW_VPP_ARTIFACTS:-} ]] || { echo "NGFW_VPP_ARTIFACTS must name verified product build output" >&2; exit 1; }
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
VPP_OUTPUT=$(realpath -e -- "$NGFW_VPP_ARTIFACTS")
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

# Package maintainer scripts must not start distro-default VPP or management
# services before the complete product firstboot configuration is installed.
exec 9>/run/lock/ngfw-runtime-install.lock
flock -x 9
POLICY=/usr/sbin/policy-rc.d
# Same-filesystem persistent recovery state makes guard publication/restoration
# atomic. A stale record requires operator recovery before another installation.
POLICY_BACKUP=/usr/sbin/.ngfw-runtime-policy-recovery
[[ ! -e $POLICY_BACKUP && ! -L $POLICY_BACKUP ]] || {
  echo "runtime policy recovery pending at $POLICY_BACKUP; refusing installation" >&2
  exit 1
}
mkdir -m 0700 -- "$POLICY_BACKUP"
HAD_POLICY=0
GUARD_INSTALLED=0
if [[ -e $POLICY || -L $POLICY ]]; then
  cp -a -- "$POLICY" "$POLICY_BACKUP/original"
  HAD_POLICY=1
fi
printf '%s\n' "$HAD_POLICY" > "$POLICY_BACKUP/had-policy"
printf '#!/bin/sh\nexit 101\n' > "$POLICY_BACKUP/guard"
chmod 0755 "$POLICY_BACKUP/guard"
GUARD_SHA=$(sha256sum "$POLICY_BACKUP/guard" | cut -d ' ' -f 1)
restore_policy() {
  local status=$1 current=
  trap - EXIT
  if [[ $GUARD_INSTALLED == 0 ]]; then
    # Before atomic publication the original policy was never changed.
    rm -rf -- "$POLICY_BACKUP"
    exit "$status"
  fi
  if [[ -f $POLICY && ! -L $POLICY ]]; then
    current=$(sha256sum "$POLICY" | cut -d ' ' -f 1)
  fi
  if [[ $current != "$GUARD_SHA" ]]; then
    echo "runtime start policy changed; original retained at $POLICY_BACKUP/original for manual recovery" >&2
    exit 1
  fi
  if [[ $HAD_POLICY == 1 ]]; then
    mv -Tf -- "$POLICY_BACKUP/original" "$POLICY"
  else
    rm -- "$POLICY"
  fi
  rm -- "$POLICY_BACKUP/guard" 2>/dev/null || true
  rm -- "$POLICY_BACKUP/had-policy"
  sync -f "$(dirname -- "$POLICY")"
  rmdir -- "$POLICY_BACKUP"
  exit "$status"
}
# Arm before publication. Persist the original/metadata before replacing policy.
trap 'restore_policy $?' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
sync -f "$POLICY_BACKUP"
# Set the flag before rename so an interrupted/failed rename retains evidence
# instead of deleting the backup; restoration refuses any unexpected policy.
GUARD_INSTALLED=1
mv -T -- "$POLICY_BACKUP/guard" "$POLICY"
sync -f "$(dirname -- "$POLICY")"
apt-get update
apt-get install -y --no-install-recommends \
  "${DATAPLANE[@]}" "${NIC[@]}" "${ROUTING[@]}" "${VPN[@]}" \
  "${SERVICES[@]}" "${CONTROL[@]}" "${OBSERV[@]}" "${TOOLS[@]}"

# Management-network/package removals and IRQ policy belong to the separate
# hardening task. A runtime dependency installer must not reconfigure them.
# Daemons are driven by ngfw-agent, which renders their configs. Do not let them
# start with distro defaults on first boot.
systemctl disable vpp frr strongswan-starter kea-dhcp4-server kea-dhcp6-server \
  unbound snmpd keepalived 2>/dev/null || true

echo
echo "Runtime packages installed."
echo "strongSwan VPP integration is supplied by the reviewed P11 ngfw-strongswan package."
echo "Do not install upstream strongSwan in place of that package."
