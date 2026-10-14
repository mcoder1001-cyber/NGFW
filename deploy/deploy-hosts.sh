#!/usr/bin/env bash
# Build reviewed NGFW packages, or upgrade explicitly named installed appliances.
# Usage: deploy/deploy-hosts.sh [-r REF] [-v VERIFIED_VPP_DIR] [-w NEW_WORK_DIR]
#        [-p PACKAGE_DIR] [-b] [-u SSH_USER] [HOST ...]
# -b builds/verifies only. -p requires a complete SHA256SUMS; never creates one.
# Hosts are DNS names or IPv4 addresses (SSH configuration aliases are supported).
# See docs/install/deploy-hosts.md for trust, maintenance and recovery requirements.
set -euo pipefail
umask 077
export LC_ALL=C
REPO=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
REF=main
VPP_OUTPUT=${NGFW_VPP_OUTPUT:-}
WORK=
PKG_DIR=
BUILD_ONLY=0
SSH_USER=root
SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=15 -o StrictHostKeyChecking=yes -o ControlMaster=no -o ControlPath=none)
SNAPSHOT=
log() { printf '[%(%H:%M:%S)T] %s\n' -1 "$*" >&2; }
die() { log "ERROR: $*"; exit 1; }
usage() { sed -n '2,7p' "$0" | sed 's/^# \{0,1\}//'; }
cleanup() { if [[ -n $SNAPSHOT ]]; then rm -rf -- "$SNAPSHOT"; fi; }
trap cleanup EXIT
while getopts ':r:v:w:p:bu:h' opt; do
  case $opt in
    r) REF=$OPTARG ;;
    v) VPP_OUTPUT=$OPTARG ;;
    w) WORK=$OPTARG ;;
    p) PKG_DIR=$OPTARG ;;
    b) BUILD_ONLY=1 ;;
    u) SSH_USER=$OPTARG ;;
    h) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done
shift $((OPTIND - 1))
HOSTS=("$@")
(( BUILD_ONLY || ${#HOSTS[@]} )) || die 'no hosts given (use -b to only build)'
[[ $SSH_USER =~ ^[a-z_][a-z0-9_-]*$ ]] || die 'invalid SSH user'
for host in "${HOSTS[@]}"; do
  [[ $host =~ ^[A-Za-z0-9][A-Za-z0-9.-]*$ && $host != *..* ]] || die 'invalid host; use a DNS name, IPv4 address or SSH alias'
done
[[ $REF =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ && $REF != *..* && $REF != */ ]] || die 'invalid origin ref'
[[ -z $PKG_DIR || -z $WORK ]] || die '-p and -w cannot be combined'

# Shared local/remote validation: no symlinks, extra archives, mixed versions,
# arbitrary checksum paths, unlisted bytes or package identity by filename alone.
verify_packages() {
  python3 - "$1" "${2:-}" "${3:-}" "${4:-}" <<'PY'
import hashlib, json, os, pathlib, re, stat, subprocess, sys
root = pathlib.Path(sys.argv[1])
expected = {'ngfw-agent': 'amd64', 'ngfw-api': 'amd64', 'ngfw-web': 'all', 'ngfw-meta': 'all'}
try:
    entries = list(root.iterdir())
    if len(entries) not in (5, 6) or any(not stat.S_ISREG(p.lstat().st_mode) for p in entries):
        raise ValueError('expected four regular packages, SHA256SUMS and optional build-info.json')
    names = {p.name for p in entries}
    if 'SHA256SUMS' not in names:
        raise ValueError('missing SHA256SUMS')
    if (root / 'SHA256SUMS').stat().st_size > 8192:
        raise ValueError('oversized checksum manifest')
    sums = {}
    for line in (root / 'SHA256SUMS').read_text().splitlines():
        match = re.fullmatch(r'([a-f0-9]{64}) [ *]([A-Za-z0-9_.+~:-]+)', line)
        if not match or match[2] in sums or match[2] in ('.', '..', 'SHA256SUMS'):
            raise ValueError('invalid or duplicate checksum entry')
        sums[match[2]] = match[1]
    if set(sums) != names - {'SHA256SUMS'}:
        raise ValueError('checksum manifest does not cover the exact package set')
    for name, digest in sums.items():
        with (root / name).open('rb') as stream:
            actual = hashlib.file_digest(stream, 'sha256').hexdigest()
        if digest != actual:
            raise ValueError('package checksum mismatch')
    versions, found = set(), set()
    for path in sorted(entries):
        if path.name in ('SHA256SUMS', 'build-info.json'):
            continue
        fields = subprocess.check_output(['dpkg-deb', '-f', str(path), 'Package', 'Version', 'Architecture'], text=True)
        data = dict(line.split(': ', 1) for line in fields.splitlines())
        name, version, arch = data['Package'], data['Version'], data['Architecture']
        if name not in expected or name in found or arch != expected[name]:
            raise ValueError('unexpected, duplicate or wrong-architecture package')
        if not re.fullmatch(r'[0-9][A-Za-z0-9.+:~\-]*', version):
            raise ValueError('unsafe package version')
        subprocess.run(['dpkg', '--validate-version', version], check=True, capture_output=True)
        if path.name != f'{name}_{version.split(":", 1)[-1]}_{arch}.deb':
            raise ValueError('package filename does not match metadata')
        versions.add(version)
        found.add(name)
    if found != set(expected) or len(versions) != 1:
        raise ValueError('incomplete package set or mixed versions')
    if 'build-info.json' in names:
        if (root / 'build-info.json').stat().st_size > 4096:
            raise ValueError('oversized build provenance')
        info = json.loads((root / 'build-info.json').read_text())
        if set(info) != {'source_commit', 'source_count', 'vpp_manifest_sha256'} or not re.fullmatch(r'[a-f0-9]{40}', info['source_commit']) or not re.fullmatch(r'[a-f0-9]{64}', info['vpp_manifest_sha256']):
            raise ValueError('invalid build provenance')
        if type(info['source_count']) is not int or info['source_count'] < 1:
            raise ValueError('invalid source history count')
        if versions != {f"0.1.0~dev+git{info['source_count']}.{info['source_commit'][:12]}"}:
            raise ValueError('package version does not match build provenance')
    else:
        info = {}
    if sys.argv[2] and (info.get('source_commit') != sys.argv[2] or info.get('vpp_manifest_sha256') != sys.argv[3] or str(info.get('source_count')) != sys.argv[4]):
        raise ValueError('cache does not match requested source/VPP input')
    print(next(iter(versions)))
except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError) as error:
    sys.exit(f'package validation failed: {error}')
PY
}

build_packages() {
  local sha source_count vpp_digest src p
  [[ -n $VPP_OUTPUT ]] || die 'build requires -v VERIFIED_VPP_DIR or NGFW_VPP_OUTPUT'
  VPP_OUTPUT=$(realpath -e -- "$VPP_OUTPUT")
  "$REPO/deploy/vpp/verify.sh" --require-files "$VPP_OUTPUT" --install-gate >&2
  vpp_digest=$(sha256sum "$VPP_OUTPUT/manifest.json"); vpp_digest=${vpp_digest%% *}
  git -C "$REPO" fetch --quiet origin "$REF" || die "cannot fetch origin $REF"
  [[ $(git -C "$REPO" rev-parse --is-shallow-repository) == false ]] || die 'build requires complete Git history for version ordering'
  sha=$(git -C "$REPO" rev-parse --verify 'FETCH_HEAD^{commit}')
  [[ $sha =~ ^[a-f0-9]{40}$ ]] || die 'invalid source commit'
  source_count=$(git -C "$REPO" rev-list --count "$sha")
  [[ $source_count =~ ^[1-9][0-9]*$ ]] || die 'invalid source history count'
  if [[ -z $WORK ]]; then
    WORK=$(mktemp -d /var/tmp/ngfw-build.XXXXXXXX)
  else
    [[ ! -L $WORK ]] || die 'work directory cannot be a symlink'
    WORK=$(realpath -m -- "$WORK")
    [[ ! -L $WORK ]] || die 'work directory cannot be a symlink'
    if [[ -e $WORK ]]; then
      [[ -d $WORK/packages ]] || die 'existing work directory has no reusable package cache; use a new -w directory'
      verify_packages "$WORK/packages" "$sha" "$vpp_digest" "$source_count" >/dev/null || die 'cache mismatch; use a new -w directory'
      PKG_DIR=$WORK/packages
      return
    fi
    mkdir -m 700 -- "$WORK"
  fi
  src=$WORK/src
  PKG_DIR=$WORK/packages
  log "building origin/$REF at $sha in $WORK"
  git -C "$REPO" worktree add --detach "$src" "$sha" >&2
  (cd "$src" && "$src/tools/heavy.sh" pnpm install --frozen-lockfile --prefer-offline) >&2
  (cd "$src" && GOMAXPROCS=4 "$src/tools/heavy.sh" deploy/debian/ngfw/prepare.sh "$VPP_OUTPUT" "$WORK/pkgsrc") >&2
  (cd "$WORK/pkgsrc" && GOMAXPROCS=4 "$src/tools/heavy.sh" dpkg-buildpackage -us -uc -b) >&2
  mkdir -m 700 -- "$PKG_DIR"
  for p in ngfw-agent ngfw-api ngfw-meta ngfw-web; do
    local matches=("$WORK"/"${p}"_*.deb)
    [[ ${#matches[@]} == 1 && -f ${matches[0]} ]] || die "missing or duplicate built package $p"
    cp -- "${matches[0]}" "$PKG_DIR/"
  done
  printf '{"source_commit":"%s","source_count":%s,"vpp_manifest_sha256":"%s"}\n' "$sha" "$source_count" "$vpp_digest" > "$PKG_DIR/build-info.json"
  (cd "$PKG_DIR" && sha256sum -- *.deb build-info.json > SHA256SUMS)
  verify_packages "$PKG_DIR" "$sha" "$vpp_digest" "$source_count" >/dev/null
  git -C "$REPO" worktree remove "$src" >&2
  log "packages ready in $PKG_DIR"
}

# Fixed code over SSH stdin; command arguments are separately restricted below.
read -r -d '' REMOTE_PREPARE <<'REMOTE' || true
set -euo pipefail
umask 077
[[ $(id -u) == 0 ]] || { echo 'target SSH user must already be root' >&2; exit 1; }
[[ $(dpkg --print-architecture) == amd64 ]]
for tool in apt-get python3 dpkg-deb sha256sum tar runuser pg_dump pg_restore ip curl systemctl flock; do
  command -v "$tool" >/dev/null
done
[[ -f /var/lib/ngfw/firstboot-complete && -f /etc/ngfw/api.env ]] || { echo 'target is not an initialized appliance' >&2; exit 1; }
for package in ngfw-agent ngfw-api ngfw-meta ngfw-web; do
  [[ $(dpkg-query -W -f='${Status}' "$package") == 'install ok installed' ]]
done
for service in ngfw-agent.service ngfw-api.service nginx.service vpp.service; do
  systemctl is-active --quiet "$service"
done
[[ ! -L /root/ngfw-deploy ]]
if [[ -e /root/ngfw-deploy ]]; then
  [[ $(stat -c '%u:%a' /root/ngfw-deploy) == 0:700 ]] || { echo 'untrusted deployment directory' >&2; exit 1; }
else
  install -d -m 700 /root/ngfw-deploy
fi
mktemp -d /root/ngfw-deploy/run.XXXXXXXX
REMOTE
read -r -d '' REMOTE_INSTALL <<'REMOTE' || true
set -euo pipefail
umask 077
DIR=$1 EXPECTED_SUMS=$2 VERSION=$3
BACKUP=${DIR%/packages}
[[ $(id -u) == 0 && -d $DIR && ! -L $DIR && ! -L $BACKUP ]]
[[ ! -L /root/ngfw-deploy/upgrade.lock ]]
if [[ -e /root/ngfw-deploy/upgrade.lock ]]; then
  [[ -f /root/ngfw-deploy/upgrade.lock && $(stat -c '%u:%a' /root/ngfw-deploy/upgrade.lock) == 0:600 ]]
fi
exec 9>>/root/ngfw-deploy/upgrade.lock
flock -n 9 || { echo 'another NGFW deployment is in progress' >&2; exit 1; }
for service in ngfw-agent.service ngfw-api.service nginx.service vpp.service; do
  systemctl is-active --quiet "$service"
done
trap 'rc=$?; if (( rc )); then echo "upgrade failed; preserve $BACKUP for manual recovery; no automatic rollback attempted" >&2; fi' EXIT
cd "$DIR"
actual=$(sha256sum SHA256SUMS); [[ ${actual%% *} == "$EXPECTED_SUMS" ]]
[[ $(verify_packages "$DIR") == "$VERSION" ]]
for package in ngfw-agent ngfw-api ngfw-meta ngfw-web; do
  [[ $(dpkg-query -W -f='${Status}' "$package") == 'install ok installed' ]]
  installed=$(dpkg-query -W -f='${Version}' "$package")
  dpkg --compare-versions "$VERSION" ge "$installed" || { echo "downgrade refused: $package" >&2; exit 1; }
done
route_before=$(ip -4 route show default)
route6_before=$(ip -6 route show default)
[[ -n $route_before || -n $route6_before ]] || { echo 'no management default route' >&2; exit 1; }
printf '%s\n' "$route_before" > "$BACKUP/default-route-v4.txt"
printf '%s\n' "$route6_before" > "$BACKUP/default-route-v6.txt"
# Match the canonical firstboot peer-authenticated database; never source api.env.
[[ $(grep -Ec '^[[:space:]]*NGFW_DATABASE_URL[[:space:]]*=' /etc/ngfw/api.env) == 1 ]]
grep -Fxq 'NGFW_DATABASE_URL=postgresql:///ngfw?host=/var/run/postgresql&user=ngfw' /etc/ngfw/api.env || { echo 'database configuration requires manual backup review' >&2; exit 1; }
[[ $(grep -Ec '^[[:space:]]*NGFW_SECRET_KEY_FILE[[:space:]]*=' /etc/ngfw/api.env) == 1 ]]
grep -Fxq 'NGFW_SECRET_KEY_FILE=/var/lib/ngfw/secret.key' /etc/ngfw/api.env || { echo 'secret key configuration requires manual backup review' >&2; exit 1; }
[[ -f /var/lib/ngfw/secret.key && ! -L /var/lib/ngfw/secret.key && -f /etc/vpp/startup.conf ]]
dpkg-query -W -f='${Package} ${Version}\n' ngfw-agent ngfw-api ngfw-meta ngfw-web vpp > "$BACKUP/previous-versions.txt"
# Simulation must leave VPP and every dependency untouched. No removals/downloads.
apt_args=(--no-download --no-remove --assume-yes -o APT::Install-Recommends=false -o Dpkg::Options::=--force-confold)
apt-get "${apt_args[@]}" --simulate install ./ngfw-*.deb > "$BACKUP/apt-plan.txt"
python3 - "$BACKUP/apt-plan.txt" <<'PYPLAN'
import pathlib, sys
allowed = {'ngfw-agent', 'ngfw-api', 'ngfw-meta', 'ngfw-web'}
for line in pathlib.Path(sys.argv[1]).read_text().splitlines():
    fields = line.split()
    if fields and (fields[0] == 'Remv' or (fields[0] in ('Inst', 'Conf') and (len(fields) < 2 or fields[1].split(':')[0] not in allowed))):
        sys.exit('APT would change another package; review dependencies separately')
PYPLAN
echo "backup -> $BACKUP"
tar -C / -czf "$BACKUP/preupgrade-config.tar.gz" etc/ngfw etc/vpp/startup.conf var/lib/ngfw
tar -tzf "$BACKUP/preupgrade-config.tar.gz" >/dev/null
runuser -u ngfw -- pg_dump --format=custom --dbname='postgresql:///ngfw?host=/var/run/postgresql&user=ngfw' > "$BACKUP/preupgrade-ngfw.dump"
[[ -s $BACKUP/preupgrade-ngfw.dump ]]
pg_restore --list "$BACKUP/preupgrade-ngfw.dump" >/dev/null
# Recheck the private staged bytes after backups, immediately before installation.
actual=$(sha256sum SHA256SUMS); [[ ${actual%% *} == "$EXPECTED_SUMS" ]]
[[ $(verify_packages "$DIR") == "$VERSION" ]]
echo 'installing packages'
DEBIAN_FRONTEND=noninteractive apt-get "${apt_args[@]}" install ./ngfw-*.deb
systemctl daemon-reload
systemctl restart ngfw-agent.service ngfw-api.service
healthy=0
for ((attempt=0; attempt<30; attempt++)); do
  if curl --noproxy '*' -fsS --connect-timeout 2 --max-time 3 http://127.0.0.1:3000/api/v1/health >/dev/null 2>&1; then healthy=1; break; fi
  sleep 2
done
[[ $healthy == 1 ]] || { echo 'API health failed' >&2; exit 1; }
for service in ngfw-agent.service ngfw-api.service nginx.service vpp.service; do
  systemctl is-active --quiet "$service"
done
[[ $(ip -4 route show default) == "$route_before" && $(ip -6 route show default) == "$route6_before" ]] || { echo 'management default route changed' >&2; exit 1; }
for package in ngfw-agent ngfw-api ngfw-meta ngfw-web; do
  [[ $(dpkg-query -W -f='${Version}' "$package") == "$VERSION" ]]
done
REMOTE

deploy_host() {
  local host=$1 target=$SSH_USER@$1 dir backup digest
  log "[$host] preflight"
  backup=$(ssh "${SSH_OPTS[@]}" "$target" 'env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin LC_ALL=C HOME=/root bash -s' <<<"$REMOTE_PREPARE") || die "$host: preflight failed"
  [[ $backup =~ ^/root/ngfw-deploy/run\.[A-Za-z0-9]{8}$ ]] || die "$host: invalid remote staging path"
  dir=$backup/packages
  # Expand locally: dir is the strictly validated backup path plus /packages.
  # shellcheck disable=SC2029
  ssh "${SSH_OPTS[@]}" "$target" "mkdir -m 700 -- $dir"
  digest=$(sha256sum "$SNAPSHOT/SHA256SUMS"); digest=${digest%% *}
  [[ $digest =~ ^[a-f0-9]{64}$ ]] || die "$host: invalid package manifest digest"
  log "[$host] copying packages ($VERSION)"
  scp -q "${SSH_OPTS[@]}" "$SNAPSHOT"/* "$target:$dir/"
  log "[$host] installing; recovery directory: $backup"
  # Expand locally: dir is validated above, digest is hex, and verify_packages
  # restricts VERSION to a digit followed by [A-Za-z0-9.+:~-], with no shell syntax.
  # shellcheck disable=SC2029
  { declare -f verify_packages; printf '%s\n' "$REMOTE_INSTALL"; } | ssh "${SSH_OPTS[@]}" "$target" "env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin LC_ALL=C HOME=/root bash -s -- $dir $digest $VERSION" || die "$host: install/health failed; inspect $backup on the host"
  # Expand locally: backup matched the fixed prefix and eight alphanumerics above.
  # shellcheck disable=SC2029
  ssh "${SSH_OPTS[@]}" "$target" "umask 077; printf 'complete\n' > $backup/upgrade-complete" || die "$host: SSH lost after upgrade; inspect $backup"
  log "[$host] OK"
}

if [[ -n $PKG_DIR ]]; then
  [[ -d $PKG_DIR && ! -L $PKG_DIR ]] || die 'package directory must be a real directory'
  PKG_DIR=$(realpath -e -- "$PKG_DIR")
else
  build_packages
fi
VERSION=$(verify_packages "$PKG_DIR")
(( BUILD_ONLY )) && { echo "$PKG_DIR"; exit 0; }
# Use a private snapshot so replacement of the operator's delivery cannot change
# later hosts. Reject links before copying, then verify the copied bytes again.
input_sums=$(sha256sum "$PKG_DIR/SHA256SUMS"); input_sums=${input_sums%% *}
SNAPSHOT=$(mktemp -d /var/tmp/ngfw-deploy.XXXXXXXX)
cp -P -- "$PKG_DIR"/* "$SNAPSHOT/"
copied_sums=$(sha256sum "$SNAPSHOT/SHA256SUMS"); [[ ${copied_sums%% *} == "$input_sums" ]] || die 'delivery changed during snapshot'
[[ $(verify_packages "$SNAPSHOT") == "$VERSION" ]]
for h in "${HOSTS[@]}"; do deploy_host "$h"; done
log "deployed to: ${HOSTS[*]}"
