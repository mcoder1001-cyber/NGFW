#!/usr/bin/env bash
# Developer / CI machine. Never run this on an appliance image.
set -euo pipefail
# These versions match the reviewed CI and agent module contracts.
GO_VER=1.26.0
PROTOC_GO_VER=v1.36.12
PROTOC_GRPC_VER=v1.6.2
GOVPP_VER=v0.13.0
# Official https://go.dev/dl/ go1.26.0 linux-amd64, checked 2026-10-02.
GO_SHA256=aac1b08a0fb0c4e0a7c1555beb7b59180b05dfc5a3d62e40e9de90cd42f88235
[[ ${VRX_GO_SHA256-$GO_SHA256} == "$GO_SHA256" ]] || {
  echo "REFUSED: VRX_GO_SHA256 override differs from the pinned official Go archive" >&2
  exit 1
}
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || { echo "only linux-amd64 build bootstrap is supported" >&2; exit 1; }
if [[ ${1:-} == --check-config && $# == 1 ]]; then
  printf 'Go=%s protoc-gen-go=%s protoc-gen-go-grpc=%s govpp=%s\n' "$GO_VER" "$PROTOC_GO_VER" "$PROTOC_GRPC_VER" "$GOVPP_VER"
  exit 0
fi
[[ $# == 0 ]] || { echo "usage: $0 [--check-config]" >&2; exit 1; }
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
if [[ ! -x /usr/local/go/bin/go ]] || [[ "$(/usr/local/go/bin/go version)" != "go version go${GO_VER} linux/amd64" ]]; then
  go_archive=$(mktemp /tmp/vrx-go.XXXXXXXX.tar.gz)
  trap 'rm -f -- "$go_archive"' EXIT
  curl -fsSL "https://go.dev/dl/go${GO_VER}.linux-amd64.tar.gz" -o "$go_archive"
  printf '%s  %s\n' "$GO_SHA256" "$go_archive" | sha256sum --check --status
  rm -rf /usr/local/go && tar -C /usr/local -xzf "$go_archive"
  rm -f -- "$go_archive"
  trap - EXIT
fi
echo 'export PATH=/usr/local/go/bin:$HOME/go/bin:$PATH' > /etc/profile.d/go.sh
export PATH=/usr/local/go/bin:$PATH
hash -r
[[ "$(go version)" == "go version go${GO_VER} linux/amd64" ]] || {
  echo "REFUSED: selected Go does not match the pinned version/platform" >&2
  exit 1
}
go install "google.golang.org/protobuf/cmd/protoc-gen-go@$PROTOC_GO_VER"
go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@$PROTOC_GRPC_VER"
go install "go.fd.io/govpp/cmd/binapi-generator@$GOVPP_VER"

corepack enable || npm install -g pnpm

echo
echo "Build toolchain installed."
echo "For VPP itself: git clone https://gerrit.fd.io/r/vpp && cd vpp && make install-dep"
echo "  - that target pulls every remaining VPP build dependency for this Ubuntu release."
