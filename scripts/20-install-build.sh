#!/usr/bin/env bash
# Developer / CI machine. Never run this on an appliance image.
set -euo pipefail
# These versions match the reviewed CI and agent module contracts.
PINNED_GO_VER=1.26.0
SCRIPT_DIR=$(cd -- "$(/usr/bin/dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
GO_MODULE="$SCRIPT_DIR/../apps/agent/go.mod"
[[ -f $GO_MODULE && ! -L $GO_MODULE ]] || { echo "REFUSED: canonical agent go.mod must be a regular file" >&2; exit 1; }
# Only one canonical numeric go directive is supported; never evaluate module text.
if ! GO_VER=$(/usr/bin/awk '
  /^[ \t]*go([ \t]|$)/ {
    count++
    if ($0 ~ /^go[ \t]+[1-9][0-9]*\.(0|[1-9][0-9]*)(\.(0|[1-9][0-9]*))?[ \t]*$/) {
      valid++; version=$2
    }
  }
  END { if (count == 1 && valid == 1) print version; else exit 1 }
' "$GO_MODULE"); then
  echo "REFUSED: canonical agent go.mod needs exactly one valid go directive" >&2
  exit 1
fi
[[ $GO_VER =~ ^[0-9]+\.[0-9]+$ ]] && GO_VER+=.0
[[ $GO_VER == "$PINNED_GO_VER" ]] || {
  echo "REFUSED: agent Go directive has no reviewed official archive/version pin" >&2
  exit 1
}
PROTOC_GO_VER=v1.36.12
PROTOC_GRPC_VER=v1.6.2
GOVPP_VER=v0.13.0
# Official https://go.dev/dl/ go1.26.0 linux-amd64, checked 2026-10-02.
GO_SHA256=aac1b08a0fb0c4e0a7c1555beb7b59180b05dfc5a3d62e40e9de90cd42f88235
[[ ${NGFW_GO_SHA256-$GO_SHA256} == "$GO_SHA256" ]] || {
  echo "REFUSED: NGFW_GO_SHA256 override differs from the pinned official Go archive" >&2
  exit 1
}
[[ $(/usr/bin/uname -s) == Linux && $(/usr/bin/uname -m) == x86_64 ]] || { echo "only linux-amd64 build bootstrap is supported" >&2; exit 1; }
if [[ ${1:-} == --check-config && $# == 1 ]]; then
  printf 'Go=%s protoc-gen-go=%s protoc-gen-go-grpc=%s govpp=%s\n' "$GO_VER" "$PROTOC_GO_VER" "$PROTOC_GRPC_VER" "$GOVPP_VER"
  exit 0
fi
# shellcheck source=install-common.sh
source "$SCRIPT_DIR/install-common.sh"
ngfw_install_init "$@"
PNPM_PIN=$(python3 - "$SCRIPT_DIR/../package.json" <<'PYPNPM'
import json, re, sys
value = json.load(open(sys.argv[1]))['packageManager']
if not re.fullmatch(r'pnpm@[0-9]+\.[0-9]+\.[0-9]+', value):
    raise SystemExit('REFUSED: exact pnpm packageManager contract required')
print(value)
PYPNPM
)
BUILD=(build-essential cmake ninja-build clang lld ccache git pkg-config chrpath nasm
       python3-dev python3-venv python3-pip)
VPPDEP=(libnuma-dev libssl-dev libelf-dev libpcap-dev libmnl-dev uuid-dev libapr1-dev)
SSWAN=(libgmp-dev libsystemd-dev gperf bison flex autoconf automake libtool)
GO=(protobuf-compiler)
NODE=(nodejs)
PKG=(devscripts debhelper dh-make dpkg-dev fakeroot reprepro gnupg)
IMAGE=(xorriso isolinux squashfs-tools cloud-image-utils qemu-utils debootstrap)
QUALITY=(shellcheck jq)


if [[ $NGFW_INSTALL_DRY_RUN == 1 ]]; then
  ngfw_install_plan "APT update/install ${BUILD[*]} ${VPPDEP[*]} ${SSWAN[*]} ${GO[*]} ${NODE[*]} ${PKG[*]} ${IMAGE[*]} ${QUALITY[*]}; verify Go $GO_VER SHA256 before extraction under /usr/local; generators $PROTOC_GO_VER/$PROTOC_GRPC_VER/$GOVPP_VER; install exact $PNPM_PIN; profile /etc/profile.d/go.sh; tool homes under /opt/ngfw-build"
  exit 0
fi
local_dir=$(ngfw_install_path /usr/local)
go_dir=$(ngfw_install_path /usr/local/go)
profile_dir=$(ngfw_install_path /etc/profile.d)
profile_file=$(ngfw_install_path /etc/profile.d/go.sh)
work_dir=$(ngfw_install_path /tmp)
build_home=$(ngfw_install_path /opt/ngfw-build)
mkdir -p "$local_dir/bin" "$profile_dir" "$work_dir" "$build_home"
export GOPATH="$build_home/go" GOBIN="$build_home/go/bin" GOCACHE="$build_home/go-cache"
export COREPACK_HOME="$build_home/corepack" npm_config_cache="$build_home/npm-cache"
export npm_config_prefix="$local_dir"
export XDG_CACHE_HOME="$build_home/cache" TMPDIR="$work_dir"
export DEBIAN_FRONTEND=noninteractive


apt-get update
apt-get install -y "${BUILD[@]}" "${VPPDEP[@]}" "${SSWAN[@]}" "${GO[@]}" "${NODE[@]}" \
                   "${PKG[@]}" "${IMAGE[@]}" "${QUALITY[@]}"

# Go from the official tarball - the distro package lags and govpp tracks new releases.
if [[ ! -x $go_dir/bin/go ]] || [[ "$("$go_dir/bin/go" version)" != "go version go${GO_VER} linux/amd64" ]]; then
  go_archive=$(mktemp "$work_dir"/ngfw-go.XXXXXXXX.tar.gz)
  trap 'rm -f -- "$go_archive"' EXIT
  curl -fsSL "https://go.dev/dl/go${GO_VER}.linux-amd64.tar.gz" -o "$go_archive"
  printf '%s  %s\n' "$GO_SHA256" "$go_archive" | sha256sum --check --status
  rm -rf -- "$go_dir" && tar -C "$local_dir" -xzf "$go_archive"
  rm -f -- "$go_archive"
  trap - EXIT
fi
cat > "$profile_file" <<'GO_PROFILE'
export PATH=/usr/local/go/bin:/opt/ngfw-build/go/bin:$PATH
GO_PROFILE
if [[ $NGFW_INSTALL_ROOT == / ]]; then
  export PATH="$go_dir/bin:$GOBIN:$PATH"
fi
hash -r
[[ "$(go version)" == "go version go${GO_VER} linux/amd64" ]] || {
  echo "REFUSED: selected Go does not match the pinned version/platform" >&2
  exit 1
}
go install "google.golang.org/protobuf/cmd/protoc-gen-go@$PROTOC_GO_VER"
go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@$PROTOC_GRPC_VER"
go install "go.fd.io/govpp/cmd/binapi-generator@$GOVPP_VER"

if ! corepack enable --install-directory "$local_dir/bin" || ! corepack prepare "$PNPM_PIN" --activate; then
  npm install -g "$PNPM_PIN"
fi

echo
echo "Build toolchain installed."
echo "For VPP itself: git clone https://gerrit.fd.io/r/vpp && cd vpp && make install-dep"
echo "  - that target pulls every remaining VPP build dependency for this Ubuntu release."
