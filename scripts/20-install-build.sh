#!/usr/bin/env bash
# Developer / CI machine. Never run this on an appliance image.
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }
export DEBIAN_FRONTEND=noninteractive

BUILD=(build-essential cmake ninja-build clang lld ccache git pkg-config chrpath nasm
       python3-dev python3-venv python3-pip)
VPPDEP=(libnuma-dev libssl-dev libelf-dev libpcap-dev libmnl-dev uuid-dev libapr1-dev)
SSWAN=(libgmp-dev libsystemd-dev gperf bison flex autoconf automake libtool)
GO=(protobuf-compiler)
NODE=(nodejs)
PKG=(devscripts debhelper dh-make dpkg-dev fakeroot reprepro gnupg)
IMAGE=(xorriso isolinux squashfs-tools cloud-image-utils qemu-utils debootstrap)
CONTAINER=(docker.io docker-compose-v2)
QUALITY=(shellcheck jq)

apt-get update
apt-get install -y "${BUILD[@]}" "${VPPDEP[@]}" "${SSWAN[@]}" "${GO[@]}" "${NODE[@]}" \
                   "${PKG[@]}" "${IMAGE[@]}" "${CONTAINER[@]}" "${QUALITY[@]}"

# Go from the official tarball - the distro package lags and govpp tracks new releases.
GO_VER="${GO_VER:-1.23.4}"
if ! command -v go >/dev/null || [[ "$(go version)" != *"$GO_VER"* ]]; then
  curl -fsSL "https://go.dev/dl/go${GO_VER}.linux-amd64.tar.gz" -o /tmp/go.tgz
  rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz && rm /tmp/go.tgz
  echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' > /etc/profile.d/go.sh
fi
export PATH=$PATH:/usr/local/go/bin
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install go.fd.io/govpp/cmd/binapi-generator@latest

corepack enable || npm install -g pnpm

echo
echo "Build toolchain installed."
echo "For VPP itself: git clone https://gerrit.fd.io/r/vpp && cd vpp && make install-dep"
echo "  - that target pulls every remaining VPP build dependency for this Ubuntu release."
