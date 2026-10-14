#!/usr/bin/env bash
# Build NGFW packages from the latest origin ref and upgrade them on installed hosts.
#
#   deploy/deploy-hosts.sh [options] HOST [HOST...]
#
#   -r REF      git ref on origin to build (default: main)
#   -v DIR      verified VPP output directory consumed by prepare.sh
#               (default: $NGFW_VPP_OUTPUT or the 26.06-release+ngfw3 lab build)
#   -w DIR      scratch directory for checkout/build (default: /dev/shm/ngfw-deploy-<sha>)
#   -p DIR      skip the build and deploy the ngfw-*.deb files already in DIR
#   -b          build only, do not deploy
#   -u USER     SSH user (default: root)
#
# Hosts are upgraded one at a time; the first failing host stops the run.
# On every host: management default route is recorded, /etc/ngfw, /etc/vpp/startup.conf,
# /var/lib/ngfw and a PostgreSQL dump are backed up under /root/ngfw-deploy/<version>/,
# packages are checksum-verified, installed with apt, services restarted, and API health,
# service state and the unchanged default route are checked. VPP packages are not touched.
set -euo pipefail

REPO=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
REF=main
VPP_OUTPUT=${NGFW_VPP_OUTPUT:-/root/Documents/Codex/2026-10-04/debian-latest/outputs/NGFW-Debian-latest-development/VPP-26.06-release+ngfw3-amd64}
WORK=
PKG_DIR=
BUILD_ONLY=0
SSH_USER=root
SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=15)

log() { printf '[%(%H:%M:%S)T] %s\n' -1 "$*" >&2; }
die() { log "ERROR: $*"; exit 1; }

while getopts 'r:v:w:p:bu:h' opt; do
  case $opt in
    r) REF=$OPTARG ;;
    v) VPP_OUTPUT=$OPTARG ;;
    w) WORK=$OPTARG ;;
    p) PKG_DIR=$(realpath -e -- "$OPTARG") ;;
    b) BUILD_ONLY=1 ;;
    u) SSH_USER=$OPTARG ;;
    *) sed -n '2,19p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
  esac
done
shift $((OPTIND - 1))
HOSTS=("$@")
(( BUILD_ONLY || ${#HOSTS[@]} )) || die 'no hosts given (use -b to only build)'

# ---------------------------------------------------------------- build
fetch_ref() {
  # The configured SOCKS proxy is not always up; fall back to a direct connection.
  git -C "$REPO" fetch --quiet origin "$REF" 2>/dev/null \
    || git -C "$REPO" -c http.proxy= fetch --quiet origin "$REF" \
    || die "cannot fetch origin $REF"
  git -C "$REPO" rev-parse FETCH_HEAD
}

build_packages() {
  local sha short src
  sha=$(fetch_ref)
  short=${sha:0:12}
  log "building origin/$REF at $sha"
  WORK=${WORK:-/dev/shm/ngfw-deploy-$short}
  src=$WORK/src
  PKG_DIR=$WORK/packages
  if [[ -f $PKG_DIR/SHA256SUMS ]] && (cd "$PKG_DIR" && sha256sum --quiet -c SHA256SUMS); then
    log "reusing existing packages in $PKG_DIR"
    return
  fi
  mkdir -p "$WORK"
  if [[ ! -d $src ]]; then
    git -C "$REPO" worktree add --detach "$src" "$sha" >&2
  fi
  [[ $(git -C "$src" rev-parse HEAD) == "$sha" ]] || die "$src is not at $sha"
  rm -rf -- "$WORK/pkgsrc" "$PKG_DIR"
  (cd "$src" && "$src/tools/heavy.sh" pnpm install --frozen-lockfile --prefer-offline) >&2
  (cd "$src" && GOMAXPROCS=4 "$src/tools/heavy.sh" deploy/debian/ngfw/prepare.sh "$VPP_OUTPUT" "$WORK/pkgsrc") >&2
  (cd "$WORK/pkgsrc" && GOMAXPROCS=4 "$src/tools/heavy.sh" dpkg-buildpackage -us -uc -b) >&2
  mkdir -p "$PKG_DIR"
  local p
  for p in ngfw-agent ngfw-api ngfw-meta ngfw-web; do
    compgen -G "$WORK/${p}_*.deb" >/dev/null || die "missing built package $p"
    cp -- "$WORK"/${p}_*.deb "$PKG_DIR/"
  done
  (cd "$PKG_DIR" && sha256sum -- *.deb > SHA256SUMS)
  git -C "$REPO" worktree remove --force "$src" >&2 || true
  log "packages ready in $PKG_DIR"
}

# --------------------------------------------------------------- deploy
# Runs on the target as root. $1 = version, $2 = remote package directory.
read -r -d '' REMOTE_INSTALL <<'REMOTE' || true
set -euo pipefail
VERSION=$1 DIR=$2
BACKUP=/root/ngfw-deploy/$VERSION
route_before=$(ip -4 route show default)
[[ -n $route_before ]] || { echo 'no default route on target' >&2; exit 1; }
cd "$DIR"
sha256sum --quiet -c SHA256SUMS
echo "backup -> $BACKUP"
tar czf "$BACKUP/preupgrade-config.tar.gz" --ignore-failed-read \
  /etc/ngfw /etc/vpp/startup.conf /var/lib/ngfw 2>/dev/null || true
if [[ -r /etc/ngfw/api.env ]]; then
  db=$(sed -n 's/^NGFW_DATABASE_URL=//p' /etc/ngfw/api.env | tr -d "\"'")
  [[ -z $db ]] || pg_dump -Fc -f "$BACKUP/preupgrade-ngfw.dump" "$db"
fi
dpkg-query -W -f='${Package} ${Version}\n' 'ngfw-*' > "$BACKUP/previous-versions.txt" 2>/dev/null || true
echo 'installing packages'
DEBIAN_FRONTEND=noninteractive apt-get install -y --allow-downgrades \
  -o Dpkg::Options::=--force-confold ./ngfw-*.deb
systemctl daemon-reload
systemctl restart ngfw-agent.service ngfw-api.service
for _ in $(seq 1 30); do
  curl -fsk -m 3 https://127.0.0.1/api/v1/health >/dev/null 2>&1 && break
  sleep 2
done
curl -fsk -m 5 https://127.0.0.1/api/v1/health; echo
systemctl is-active ngfw-agent.service ngfw-api.service vpp.service
[[ $(ip -4 route show default) == "$route_before" ]] || { echo 'default route changed!' >&2; exit 1; }
dpkg-query -W -f='${Package} ${Version}\n' 'ngfw-*'
REMOTE

deploy_host() {
  local host=$1 target=$SSH_USER@$1 version dir
  version=$(dpkg-deb -f "$(compgen -G "$PKG_DIR/ngfw-api_*.deb" | head -1)" Version)
  dir=/root/ngfw-deploy/$version/packages
  log "[$host] preflight"
  ssh "${SSH_OPTS[@]}" "$target" 'dpkg-query -W ngfw-api >/dev/null && command -v apt-get >/dev/null' \
    || die "$host: unreachable or NGFW not installed (this script upgrades installed appliances)"
  ssh "${SSH_OPTS[@]}" "$target" "mkdir -p '$dir' && chmod 700 /root/ngfw-deploy"
  log "[$host] copying packages ($version)"
  scp -q "${SSH_OPTS[@]}" "$PKG_DIR"/ngfw-*.deb "$PKG_DIR/SHA256SUMS" "$target:$dir/"
  log "[$host] installing"
  ssh "${SSH_OPTS[@]}" "$target" bash -s -- "$version" "$dir" <<<"$REMOTE_INSTALL" \
    || die "$host: install/health failed; backup in /root/ngfw-deploy/$version on the host"
  # A fresh connection proves management access survived the upgrade.
  ssh "${SSH_OPTS[@]}" "$target" true || die "$host: SSH lost after upgrade"
  log "[$host] OK"
}

if [[ -n $PKG_DIR ]]; then
  [[ -f $PKG_DIR/SHA256SUMS ]] || (cd "$PKG_DIR" && sha256sum -- ngfw-*.deb > SHA256SUMS)
else
  build_packages
fi
(( BUILD_ONLY )) && { echo "$PKG_DIR"; exit 0; }
for h in "${HOSTS[@]}"; do deploy_host "$h"; done
log "deployed to: ${HOSTS[*]}"
