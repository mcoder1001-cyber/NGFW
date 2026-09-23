#!/usr/bin/env bash
# Prepare the shared development / lab host (run as root on the dev server).
# Idempotent. Grows the root filesystem, installs the VM lab and build toolchain.
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

echo "== 1. grow root filesystem to the full disk (non-destructive) =="
ROOT_DEV="$(findmnt -n -o SOURCE /)"; DISK="/dev/$(lsblk -no PKNAME "$ROOT_DEV")"; PART="${ROOT_DEV##*[a-z]}"
apt-get install -y -qq cloud-guest-utils >/dev/null
growpart "$DISK" "$PART" 2>&1 | grep -v "NOCHANGE" || true
resize2fs "$ROOT_DEV" >/dev/null 2>&1 || true
df -h / | tail -1

echo "== 2. VM lab host: QEMU/KVM + libvirt =="
apt-get update -qq
apt-get install -y -qq qemu-system-x86 qemu-utils libvirt-daemon-system libvirt-clients virtinst \
  cloud-image-utils genisoimage bridge-utils ovmf swtpm swtpm-tools >/dev/null
systemctl enable --now libvirtd >/dev/null 2>&1 || true

echo "== 3. build toolchain =="
apt-get install -y -qq build-essential cmake ninja-build clang lld ccache git curl jq pkg-config \
  python3-venv python3-pip protobuf-compiler devscripts debhelper dpkg-dev fakeroot reprepro \
  shellcheck tmux htop iperf3 tcpdump rsync unzip >/dev/null

echo "== 4. Go (latest stable) =="
GO_VER="$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -1)"
if ! command -v /usr/local/go/bin/go >/dev/null || [[ "$(/usr/local/go/bin/go version)" != *"$GO_VER"* ]]; then
  curl -fsSL "https://go.dev/dl/${GO_VER}.linux-amd64.tar.gz" -o /tmp/go.tgz
  rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz && rm /tmp/go.tgz
fi
echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' > /etc/profile.d/go.sh
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest >/dev/null 2>&1
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest >/dev/null 2>&1
go install go.fd.io/govpp/cmd/binapi-generator@latest >/dev/null 2>&1
go version

echo "== 5. Node / pnpm =="
node --version; corepack enable >/dev/null 2>&1 || npm install -g pnpm >/dev/null; pnpm --version || true

echo "== 6. nested virtualization check =="
if [[ -e /dev/kvm ]]; then echo "KVM: available"; else
  echo "KVM: NOT AVAILABLE - CPU virtualization extensions are not exposed to this VM."
  echo "     In vSphere: power off the VM -> Edit Settings -> CPU -> tick"
  echo "     'Expose hardware assisted virtualization to the guest OS' -> power on."
fi
echo "done."
