#!/usr/bin/env bash
# Build strongSwan with the VPP plugins and package it as a .deb.
# The Ubuntu package cannot drive VPP's IPsec data plane - this is not optional.
# Run on a BUILD machine (scripts/20-install-build.sh first), never on an appliance.
set -euo pipefail
SSWAN_VER="${SSWAN_VER:-5.9.14}"
WORK="${WORK:-/tmp/sswan-build}"
PREFIX="${PREFIX:-/usr}"

mkdir -p "$WORK" && cd "$WORK"
[[ -f "strongswan-${SSWAN_VER}.tar.bz2" ]] || \
  curl -fLO "https://download.strongswan.org/strongswan-${SSWAN_VER}.tar.bz2"
tar xf "strongswan-${SSWAN_VER}.tar.bz2"
cd "strongswan-${SSWAN_VER}"

# kernel-vpp replaces the Linux XFRM backend; socket-vpp moves IKE packets over
# VPP's punt socket. Both need VPP headers from the vpp-dev package.
./configure \
  --prefix="$PREFIX" --sysconfdir=/etc --libexecdir="$PREFIX/lib" \
  --enable-systemd --enable-swanctl --enable-vici \
  --enable-kernel-vpp --enable-socket-vpp \
  --disable-kernel-netlink \
  --enable-openssl --enable-eap-identity --enable-eap-md5 --enable-eap-mschapv2 \
  --enable-eap-tls --enable-eap-radius --enable-xauth-eap \
  --enable-cmd --enable-pkcs11 --enable-curl

make -j"$(nproc)"

echo
echo "Build complete. Package it rather than 'make install':"
echo "  checkinstall --pkgname=vrx-strongswan --pkgversion=${SSWAN_VER} --provides=strongswan"
echo "or write a proper debian/ directory - the product needs a signed, versioned package"
echo "so that A/B upgrade and rollback work (work item D0.5)."
echo
echo "Requires: apt-get install vpp-dev libvppinfra-dev on the build machine."
