#!/usr/bin/env bash
# Build reviewed NGFW packages, or upgrade explicitly named installed appliances.
# Usage: deploy/deploy-hosts.sh [-r REF] [-v VERIFIED_VPP_DIR] [-w NEW_WORK_DIR]
#        [-p PACKAGE_DIR] [-b] [-u SSH_USER] [--allow-divergent]
#        [--allow-no-default-route] [HOST ...]
# -b builds/verifies only. -p requires a complete SHA256SUMS; never creates one.
# Hosts are DNS names, IPv4 addresses or SSH configuration aliases; an alias with
# underscores must set HostName. Options must precede hosts. --allow-divergent
# installs, on exactly one reviewed host, a development build whose source is
# divergent from or unresolvable against the installed build's source; it never
# permits a rollback within one history or a stable-release downgrade.
# --allow-no-default-route upgrades, on exactly one declared host, a
# management-only appliance that has no default route. Needs python3 >= 3.11
# here and on the appliance. See docs/install/deploy-hosts.md for details.
set -euo pipefail
umask 077
export LC_ALL=C
REPO=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
REF=main
VPP_OUTPUT=${NGFW_VPP_OUTPUT:-}
WORK=
PKG_DIR=
BUILD_ONLY=0
ALLOW_DIVERGENT=0
ALLOW_NO_DEFAULT_ROUTE=0
SSH_USER=root
SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=15 -o StrictHostKeyChecking=yes -o ControlMaster=no -o ControlPath=none)
SNAPSHOT=
log() { printf '[%(%H:%M:%S)T] %s\n' -1 "$*" >&2; }
die() { log "ERROR: $*"; exit 1; }
usage() { sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'; }
cleanup() { if [[ -n $SNAPSHOT ]]; then rm -rf -- "$SNAPSHOT"; fi; }
trap cleanup EXIT
while getopts ':r:v:w:p:bu:h-:' opt; do
  case $opt in
    r) REF=$OPTARG ;;
    v) VPP_OUTPUT=$OPTARG ;;
    w) WORK=$OPTARG ;;
    p) PKG_DIR=$OPTARG ;;
    b) BUILD_ONLY=1 ;;
    u) SSH_USER=$OPTARG ;;
    h) usage; exit 0 ;;
    -) case $OPTARG in
         allow-divergent) ALLOW_DIVERGENT=1 ;;
         allow-no-default-route) ALLOW_NO_DEFAULT_ROUTE=1 ;;
         *) log "unknown option --$OPTARG"; usage >&2; exit 2 ;;
       esac ;;
    *) usage >&2; exit 2 ;;
  esac
done
shift $((OPTIND - 1))
HOSTS=("$@")
(( BUILD_ONLY || ${#HOSTS[@]} )) || die 'no hosts given (use -b to only build)'
[[ $SSH_USER =~ ^[a-z_][a-z0-9_-]*$ ]] || die 'invalid SSH user'
# DNS names never contain underscores; an SSH configuration alias may, and it
# then has to map to a HostName. ssh -G prints the effective configuration
# without connecting (-T: no pseudo-terminal notice). Host patterns match the
# name as typed; an unmatched name is echoed lowercased, so the comparison
# ignores case to recognise that echo as "no HostName".
configured_alias() {
  local hostname
  hostname=$(ssh -G -T "${SSH_OPTS[@]}" "$SSH_USER@$1" </dev/null | sed -n 's/^hostname //p') || return 1
  [[ -n $hostname && ${hostname,,} != "${1,,}" ]]
}
for host in "${HOSTS[@]}"; do
  [[ $host =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ && $host != *..* ]] || die 'invalid host; use a DNS name, IPv4 address or SSH alias (options must precede hosts)'
  [[ $host != *_* ]] || configured_alias "$host" || die "invalid host $host: underscores are valid only in an SSH configuration alias whose Host block sets HostName"
done
(( ! ALLOW_DIVERGENT || ${#HOSTS[@]} == 1 )) || die '--allow-divergent applies to exactly one reviewed host per run'
(( ! ALLOW_NO_DEFAULT_ROUTE || ${#HOSTS[@]} == 1 )) || die '--allow-no-default-route applies to exactly one declared host per run'
[[ $REF =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ && $REF != *..* && $REF != */ ]] || die 'invalid origin ref'
[[ -z $PKG_DIR || -z $WORK ]] || die '-p and -w cannot be combined'
# verify_packages uses hashlib.file_digest (Python 3.11). The appliance's
# interpreter is checked the same way in the remote preflight.
command -v python3 >/dev/null || die 'python3 >= 3.11 is required on this machine (hashlib.file_digest)'
python_version=$(python3 -I -c 'import sys; print("%d.%d" % sys.version_info[:2]); sys.exit(sys.version_info < (3, 11))') || die "python3 >= 3.11 is required on this machine (hashlib.file_digest); found ${python_version:-unknown}"

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

# Development versions embed the 12-character source commit (DEC-248, D-249) in
# prepare.sh's format VERSION=0.1.0~dev+git$SOURCE_COUNT.$COMMIT. Reachable commit
# counts order only descendant commits, so a stale branch with many checkpoints
# sorts above a newer main build. Trust dpkg ordering only when the new source
# descends from the installed source, verified in this repository.
FETCHED=0
OVERRIDE_USED=0
source_of_version() {
  [[ $1 =~ ^0\.1\.0~dev\+(git[1-9][0-9]*\.)?([a-f0-9]{12})$ ]] && printf '%s\n' "${BASH_REMATCH[2]}"
}
resolve_commit() {
  local commit
  commit=$(git -C "$REPO" rev-parse --verify --quiet "$1^{commit}") || return 1
  # A branch or tag named like the abbreviation must not shadow the commit.
  [[ $commit =~ ^[a-f0-9]{40}$ && $commit == "$1"* ]] && printf '%s\n' "$commit"
}
fetch_origin_once() {
  local error ssh_command
  (( FETCHED )) && return 0
  FETCHED=1
  log 'source commit not in local history; fetching origin branches (no tags, no prompts, 120 s limit)'
  # Keep the operator's own SSH command when one is configured.
  ssh_command=${GIT_SSH_COMMAND:-$(git -C "$REPO" config --get core.sshCommand 2>/dev/null || true)}
  if ! error=$(GIT_TERMINAL_PROMPT=0 GIT_SSH_COMMAND="${ssh_command:-ssh -o BatchMode=yes}" timeout 120 git -C "$REPO" fetch --quiet --no-tags --no-write-fetch-head origin 2>&1); then
    log "fetch from origin failed; using local history only: ${error##*$'\n'}"
  fi
}
# 0 ancestor, 1 not an ancestor, 2 git could not decide (caller refuses).
is_ancestor() {
  local rc=0
  git -C "$REPO" merge-base --is-ancestor "$1" "$2" || rc=$?
  (( rc <= 1 )) || return 2
  return "$rc"
}
# Returns 0 to proceed and 1 after logging a refusal. Sets OVERRIDE_USED when
# this host needed the operator's --allow-divergent.
check_lineage() {
  local host=$1 installed=$2 new_source installed_source new_commit installed_commit base counts rc
  new_source=$(source_of_version "$VERSION") || true
  installed_source=$(source_of_version "$installed") || true
  if [[ -z $new_source || -z $installed_source ]]; then
    if [[ ( $VERSION == *~dev* && -z $new_source ) || ( $installed == *~dev* && -z $installed_source ) ]]; then
      # An unparseable development version must not fall back to count ordering.
      (( ALLOW_DIVERGENT )) || { log "[$host] ERROR: development version $installed or $VERSION carries no recognisable source commit; refusing count-only ordering (--allow-divergent after review)"; return 1; }
      log "[$host] WARNING: unrecognised development version; --allow-divergent accepted by operator"
      OVERRIDE_USED=1
      return 0
    fi
    log "[$host] installed $installed; release ordering applies, remote downgrade guard decides"
    return 0
  fi
  if [[ -f $SNAPSHOT/build-info.json ]]; then
    new_source=$(python3 -I -c 'import json, sys; print(json.load(open(sys.argv[1]))["source_commit"])' "$SNAPSHOT/build-info.json") || new_source=
    [[ $new_source =~ ^[a-f0-9]{40}$ ]] || { log "[$host] ERROR: invalid build provenance"; return 1; }
  fi
  [[ $(git -C "$REPO" rev-parse --is-inside-work-tree 2>/dev/null) == true ]] || { log "[$host] ERROR: lineage check needs this script inside a Git checkout of the source repository ($REPO)"; return 1; }
  [[ $(git -C "$REPO" rev-parse --is-shallow-repository 2>/dev/null) == false ]] || { log "[$host] ERROR: lineage check requires complete Git history; shallow checkouts are refused"; return 1; }
  installed_commit=$(resolve_commit "$installed_source") || installed_commit=
  new_commit=$(resolve_commit "$new_source") || new_commit=
  if [[ -z $installed_commit || -z $new_commit ]]; then
    fetch_origin_once
    installed_commit=$(resolve_commit "$installed_source") || installed_commit=
    new_commit=$(resolve_commit "$new_source") || new_commit=
  fi
  if [[ -z $installed_commit || -z $new_commit ]]; then
    [[ -n $installed_commit ]] || log "[$host] installed source $installed_source is not in $REPO (deleted branch? git -C $REPO fetch origin pull/<N>/head:refs/remotes/origin/pr/<N>)"
    [[ -n $new_commit ]] || log "[$host] new source ${new_source:0:12} is not in $REPO; fetch the ref it was built from"
    (( ALLOW_DIVERGENT )) || { log "[$host] ERROR: source lineage unverifiable; refusing (fetch the missing commit, or --allow-divergent after review)"; return 1; }
    log "[$host] WARNING: source lineage unverifiable; --allow-divergent accepted by operator"
    OVERRIDE_USED=1
    return 0
  fi
  is_ancestor "$installed_commit" "$new_commit" && rc=0 || rc=$?
  if (( rc == 0 )); then
    log "[$host] installed source ${installed_commit:0:12} is an ancestor of ${new_commit:0:12}"
    return 0
  fi
  (( rc == 1 )) || { log "[$host] ERROR: git could not decide the ancestry of ${installed_commit:0:12} and ${new_commit:0:12}; repair the repository"; return 1; }
  is_ancestor "$new_commit" "$installed_commit" && rc=0 || rc=$?
  if (( rc == 0 )); then
    log "[$host] ERROR: new source ${new_commit:0:12} is older than installed ${installed_commit:0:12} in the same history; rollbacks are refused because migrations cannot be undone"
    return 1
  fi
  (( rc == 1 )) || { log "[$host] ERROR: git could not decide the ancestry of ${new_commit:0:12} and ${installed_commit:0:12}; repair the repository"; return 1; }
  base=$(git -C "$REPO" merge-base "$installed_commit" "$new_commit" 2>/dev/null) || base=none
  counts=$(git -C "$REPO" rev-list --left-right --count "$installed_commit...$new_commit" 2>/dev/null) || counts=$'?\t?'
  log "[$host] divergent histories: installed ${installed_commit:0:12}, new ${new_commit:0:12}, merge-base ${base:0:12}, commits only in installed/new: ${counts//$'\t'//}"
  log "[$host] review: git -C $REPO log --oneline ${installed_commit:0:12}..${new_commit:0:12}; git -C $REPO diff --stat ${installed_commit:0:12} ${new_commit:0:12}"
  (( ALLOW_DIVERGENT )) || { log "[$host] ERROR: commit counts do not order divergent histories; refusing (--allow-divergent after review, one host per run)"; return 1; }
  log "[$host] WARNING: divergent development build accepted; --allow-divergent given by operator"
  OVERRIDE_USED=1
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
# The launcher moves the script pipe to descriptor 3, gives the shell and every
# remote child /dev/null as stdin, and lets bash read the script through its own
# copy of the pipe (/dev/fd/3). A child that reads stdin therefore gets EOF and
# can never consume the rest of the script. Positional arguments pass through.
REMOTE_SHELL="env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin LC_ALL=C HOME=/root bash -c 'exec 3<&0 </dev/null && exec bash /dev/fd/3 \"\$@\"' ngfw-deploy"
read -r -d '' REMOTE_PREPARE <<'REMOTE' || true
set -euo pipefail
umask 077
exec 3<&-
ALLOW_NO_DEFAULT_ROUTE=$1
[[ $ALLOW_NO_DEFAULT_ROUTE == [01] ]]
[[ $(id -u) == 0 ]] || { echo 'target SSH user must already be root' >&2; exit 1; }
[[ $(dpkg --print-architecture) == amd64 ]]
for tool in apt-get python3 dpkg-deb sha256sum tar runuser pg_dump pg_restore ip curl systemctl flock nginx; do
  command -v "$tool" >/dev/null || { echo "missing tool on target: $tool" >&2; exit 1; }
done
python3 -I -c 'import sys; sys.exit(sys.version_info < (3, 11))' || { echo "target python3 must be >= 3.11 (hashlib.file_digest); found: $(python3 --version 2>&1)" >&2; exit 1; }
[[ -f /var/lib/ngfw/firstboot-complete && -f /etc/ngfw/api.env ]] || { echo 'target is not an initialized appliance' >&2; exit 1; }
installed=
for package in ngfw-agent ngfw-api ngfw-meta ngfw-web; do
  [[ $(dpkg-query -W -f='${Status}' "$package") == 'install ok installed' ]]
  version=$(dpkg-query -W -f='${Version}' "$package")
  [[ -z $installed || $version == "$installed" ]] || { echo 'mixed installed package versions require manual review (reinstall one version by hand first):' >&2; dpkg-query -W -f='${Package} ${Version}\n' ngfw-agent ngfw-api ngfw-meta ngfw-web >&2; exit 1; }
  installed=$version
done
for service in ngfw-agent.service ngfw-api.service nginx.service vpp.service; do
  systemctl is-active --quiet "$service"
done
# A site that already fails its test is not the new packages' fault; refuse it
# before anything is transferred.
nginx -t -q || { echo 'nginx configuration already fails its test before the upgrade; fix it first' >&2; exit 1; }
# Refuse a host without any default route before anything is transferred; the
# install step rechecks the routes under its lock and records the acceptance.
routes=$(ip -4 route show default; ip -6 route show default)
[[ $ALLOW_NO_DEFAULT_ROUTE == 1 || -n $routes ]] || { echo 'no management default route (a management-only appliance on a directly connected network needs --allow-no-default-route); nothing was transferred' >&2; exit 1; }
echo "$installed"
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
exec 3<&-
DIR=$1 EXPECTED_SUMS=$2 VERSION=$3 ALLOW_DIVERGENT=$4 EXPECTED_INSTALLED=$5 ALLOW_NO_DEFAULT_ROUTE=$6
BACKUP=${DIR%/packages}
[[ $(id -u) == 0 && -d $DIR && ! -L $DIR && ! -L $BACKUP && $ALLOW_DIVERGENT == [01] && $ALLOW_NO_DEFAULT_ROUTE == [01] ]]
[[ ${#EXPECTED_INSTALLED} -le 128 && $EXPECTED_INSTALLED =~ ^[0-9][A-Za-z0-9.+:~-]*$ ]]
[[ ! -L /root/ngfw-deploy/upgrade.lock ]]
if [[ -e /root/ngfw-deploy/upgrade.lock ]]; then
  [[ -f /root/ngfw-deploy/upgrade.lock && $(stat -c '%u:%a' /root/ngfw-deploy/upgrade.lock) == 0:600 ]]
fi
exec 9>>/root/ngfw-deploy/upgrade.lock
flock -n 9 || { echo 'another NGFW deployment is in progress' >&2; exit 1; }
for service in ngfw-agent.service ngfw-api.service nginx.service vpp.service; do
  systemctl is-active --quiet "$service"
done
trap 'rc=$?; if (( rc )); then systemctl status --no-pager --lines=50 ngfw-agent.service ngfw-api.service > "$BACKUP/service-status.txt" 2>&1 || true; echo "upgrade failed; preserve $BACKUP for manual recovery; no automatic rollback attempted" >&2; fi' EXIT
cd "$DIR"
actual=$(sha256sum SHA256SUMS); [[ ${actual%% *} == "$EXPECTED_SUMS" ]]
[[ $(verify_packages "$DIR") == "$VERSION" ]]
# The lineage verdict was taken on the preflight version; under the lock every
# package must still carry it. Development builds from divergent histories are
# not ordered by their commit counts; ALLOW_DIVERGENT is set only when the local
# check accepted this host's lineage with --allow-divergent (same format as
# prepare.sh's VERSION). Stable-release downgrades are always refused.
development='^0\.1\.0~dev\+(git[1-9][0-9]*\.)?[a-f0-9]{12}$'
downgrade_args=()
for package in ngfw-agent ngfw-api ngfw-meta ngfw-web; do
  [[ $(dpkg-query -W -f='${Status}' "$package") == 'install ok installed' ]]
  installed=$(dpkg-query -W -f='${Version}' "$package")
  [[ $installed == "$EXPECTED_INSTALLED" ]] || { echo "installed version changed since preflight: $package $installed, expected $EXPECTED_INSTALLED; rerun" >&2; exit 1; }
  dpkg --compare-versions "$VERSION" ge "$installed" && continue
  [[ $ALLOW_DIVERGENT == 1 && $VERSION =~ $development && $installed =~ $development ]] || { echo "downgrade refused: $package $installed" >&2; exit 1; }
  echo "divergent development build accepted over $package $installed" >&2
  downgrade_args=(--allow-downgrades)
done
[[ $ALLOW_DIVERGENT == 0 ]] || printf 'allow-divergent accepted by operator; apt %s\n' "${downgrade_args[*]:-without --allow-downgrades}" > "$BACKUP/override.txt"
# Compare routing intent only: route type, gateway, device, metric, source
# address and multipath next hops grouped with their route. RA-learned routes
# also print lifetimes (expires NNNsec) and attributes that are not route changes.
stable_routes() {
  ip "-$1" route show default | {
    local -a words
    local i line route=''
    while read -r -a words; do
      (( ${#words[@]} )) || continue
      line=" ${words[0]}"
      for ((i = 1; i < ${#words[@]}; i = i + 1)); do
        case ${words[i]} in
          default) line+=' default' ;;
          via)
            i=$((i + 1))
            if [[ ${words[i]-} == inet6 || ${words[i]-} == inet ]]; then i=$((i + 1)); fi
            line+=" via ${words[i]-}" ;;
          dev|metric|src|weight) line+=" ${words[i]} ${words[i + 1]-}"; i=$((i + 1)) ;;
        esac
      done
      if [[ ${words[0]} == nexthop ]]; then route+=$line; else [[ -z $route ]] || printf '%s\n' "${route# }"; route=$line; fi
    done
    [[ -z $route ]] || printf '%s\n' "${route# }"
  } | sort
}
route_before=$(stable_routes 4)
route6_before=$(stable_routes 6)
# A management-only appliance on a directly connected network has no default
# route; the operator declares that with --allow-no-default-route. The
# before/after comparison below still applies to it.
if [[ -z $route_before && -z $route6_before ]]; then
  [[ $ALLOW_NO_DEFAULT_ROUTE == 1 ]] || { echo 'no management default route (a management-only appliance on a directly connected network needs --allow-no-default-route)' >&2; exit 1; }
  printf 'no default route; --allow-no-default-route accepted by operator\n' > "$BACKUP/no-default-route.txt"
  echo 'no default route; --allow-no-default-route accepted by operator' >&2
fi
ip -4 route show default > "$BACKUP/default-route-v4.txt"
ip -6 route show default > "$BACKUP/default-route-v6.txt"
# Match the canonical firstboot peer-authenticated database; never source api.env.
[[ $(grep -Ec '^[[:space:]]*NGFW_DATABASE_URL[[:space:]]*=' /etc/ngfw/api.env) == 1 ]]
grep -Fxq 'NGFW_DATABASE_URL=postgresql:///ngfw?host=/var/run/postgresql&user=ngfw' /etc/ngfw/api.env || { echo 'database configuration requires manual backup review' >&2; exit 1; }
[[ $(grep -Ec '^[[:space:]]*NGFW_SECRET_KEY_FILE[[:space:]]*=' /etc/ngfw/api.env) == 1 ]]
grep -Fxq 'NGFW_SECRET_KEY_FILE=/var/lib/ngfw/secret.key' /etc/ngfw/api.env || { echo 'secret key configuration requires manual backup review' >&2; exit 1; }
[[ -f /var/lib/ngfw/secret.key && ! -L /var/lib/ngfw/secret.key && -f /etc/vpp/startup.conf ]]
dpkg-query -W -f='${Package} ${Version}\n' ngfw-agent ngfw-api ngfw-meta ngfw-web vpp > "$BACKUP/previous-versions.txt"
# Simulation must leave VPP and every dependency untouched. No removals, and no
# downloads: every configured repository is hidden, so APT sees only the staged
# archives and the installed packages, and a dependency that would need a
# download cannot be resolved. (--no-download would break the local archives on
# apt >= 3: their file: fetch, which resolves the absolute path handed to dpkg,
# never runs and apt fails with "Pathname to install is not absolute".)
apt_args=(-o Dir::Etc::SourceList=/dev/null -o Dir::Etc::SourceParts=/dev/null --no-remove --assume-yes -o APT::Install-Recommends=false -o Dpkg::Options::=--force-confold)
apt-get "${apt_args[@]}" "${downgrade_args[@]}" --simulate install ./ngfw-*.deb > "$BACKUP/apt-plan.txt"
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
DEBIAN_FRONTEND=noninteractive apt-get "${apt_args[@]}" "${downgrade_args[@]}" install ./ngfw-*.deb
systemctl daemon-reload
systemctl restart ngfw-agent.service ngfw-api.service
# Restart=always keeps a crash-looping daemon briefly active; count automatic
# restarts from the manual restart onwards and recheck after a stability window.
restart_counts() {
  local unit count attempt
  for unit in ngfw-agent.service ngfw-api.service; do
    count=
    for ((attempt = 0; attempt < 3; attempt = attempt + 1)); do
      if count=$(systemctl show -p NRestarts --value "$unit") && [[ $count =~ ^[0-9]+$ ]]; then break; fi
      sleep 1
    done
    [[ $count =~ ^[0-9]+$ ]] || { echo "cannot read restart count of $unit" >&2; exit 1; }
    echo "$unit NRestarts=$count"
  done
}
restarts_before=$(restart_counts)
# ngfw-web ships the nginx site (/etc/ngfw/nginx.conf, linked from
# sites-enabled by firstboot) and the static assets; a changed site applies only
# on reload. Test first, as firstboot does before enabling the site. On a
# failure the running nginx still serves the previous configuration, but the
# new site file is already on disk and the unit's start-time test would fail on
# the next restart or reboot, so the message names the recovery.
if ! nginx -t > "$BACKUP/nginx-test.txt" 2>&1; then
  cat "$BACKUP/nginx-test.txt" >&2
  printf 'nginx configuration test failed after the upgrade (output in %s/nginx-test.txt). The running nginx still serves the previous configuration, but the new site file is on disk and nginx will not start again until it is fixed; agent/API health was not checked. Restore or fix /etc/ngfw/nginx.conf (previous copy: member etc/ngfw/nginx.conf of %s/preupgrade-config.tar.gz; restore with: tar -C / -xzf %s/preupgrade-config.tar.gz etc/ngfw/nginx.conf), then run: nginx -t && systemctl reload nginx.service\n' "$BACKUP" "$BACKUP" "$BACKUP" >&2
  exit 1
fi
systemctl reload nginx.service
healthy=0
for ((attempt=0; attempt<30; attempt++)); do
  if curl --noproxy '*' -fsS --connect-timeout 2 --max-time 3 http://127.0.0.1:3000/api/v1/health >/dev/null 2>&1; then healthy=1; break; fi
  sleep 2
done
[[ $healthy == 1 ]] || { echo 'API health failed' >&2; exit 1; }
# Stability window after API health (RestartSec=3): a crash later than this is
# outside the check; nginx and VPP are only required to be active.
sleep 10
restarts_after=$(restart_counts)
for service in ngfw-agent.service ngfw-api.service nginx.service vpp.service; do
  systemctl is-active --quiet "$service"
done
[[ $restarts_after == "$restarts_before" ]] || { printf 'agent/API restarted after the upgrade restart (NRestarts before: %s; after: %s); fix the unit before redeploying\n' "${restarts_before//$'\n'/, }" "${restarts_after//$'\n'/, }" >&2; exit 1; }
route_after=$(stable_routes 4)
route6_after=$(stable_routes 6)
if [[ $route_after != "$route_before" || $route6_after != "$route6_before" ]]; then
  ip -4 route show default > "$BACKUP/default-route-v4-after.txt"
  ip -6 route show default > "$BACKUP/default-route-v6-after.txt"
  printf 'management default route changed\nbefore v4: %s\nafter v4: %s\nbefore v6: %s\nafter v6: %s\n' "$route_before" "$route_after" "$route6_before" "$route6_after" >&2
  exit 1
fi
for package in ngfw-agent ngfw-api ngfw-meta ngfw-web; do
  [[ $(dpkg-query -W -f='${Version}' "$package") == "$VERSION" ]]
done
REMOTE

deploy_host() {
  local host=$1 target=$SSH_USER@$1 dir backup digest preflight installed
  OVERRIDE_USED=0
  log "[$host] preflight"
  # Expand locally: REMOTE_SHELL is this script's fixed launcher and
  # ALLOW_NO_DEFAULT_ROUTE its own 0/1 literal, the preflight's only argument.
  # shellcheck disable=SC2029
  preflight=$(ssh "${SSH_OPTS[@]}" "$target" "$REMOTE_SHELL $ALLOW_NO_DEFAULT_ROUTE" <<<"$REMOTE_PREPARE") || die "$host: preflight failed"
  [[ $(wc -l <<<"$preflight") == 2 ]] || die "$host: unexpected preflight output"
  installed=${preflight%%$'\n'*}
  backup=${preflight#*$'\n'}
  [[ ${#installed} -le 128 && $installed =~ ^[0-9][A-Za-z0-9.+:~-]*$ ]] || die "$host: invalid installed package version"
  [[ $backup =~ ^/root/ngfw-deploy/run\.[A-Za-z0-9]{8}$ ]] || die "$host: invalid remote staging path"
  if ! check_lineage "$host" "$installed"; then
    # Expand locally: backup matched the fixed prefix and eight alphanumerics above.
    # shellcheck disable=SC2029
    ssh "${SSH_OPTS[@]}" "$target" "rmdir -- $backup" </dev/null || log "[$host] empty staging directory $backup left on the host"
    die "$host: source lineage refused; nothing was transferred"
  fi
  dir=$backup/packages
  # Expand locally: dir is the strictly validated backup path plus /packages.
  # shellcheck disable=SC2029
  ssh "${SSH_OPTS[@]}" "$target" "mkdir -m 700 -- $dir" </dev/null
  digest=$(sha256sum "$SNAPSHOT/SHA256SUMS"); digest=${digest%% *}
  [[ $digest =~ ^[a-f0-9]{64}$ ]] || die "$host: invalid package manifest digest"
  log "[$host] copying packages ($VERSION)"
  scp -q "${SSH_OPTS[@]}" "$SNAPSHOT"/* "$target:$dir/"
  log "[$host] installing; recovery directory: $backup"
  # Expand locally: REMOTE_SHELL is this script's fixed launcher, dir is
  # validated above, digest is hex, verify_packages restricts VERSION and the
  # preflight check restricts installed to a digit followed by [A-Za-z0-9.+:~-]
  # (no shell syntax, at most 128 characters), and OVERRIDE_USED and
  # ALLOW_NO_DEFAULT_ROUTE are this script's own 0/1 literals.
  # shellcheck disable=SC2029
  { declare -f verify_packages; printf '%s\n' "$REMOTE_INSTALL"; } | ssh "${SSH_OPTS[@]}" "$target" "$REMOTE_SHELL $dir $digest $VERSION $OVERRIDE_USED $installed $ALLOW_NO_DEFAULT_ROUTE" || die "$host: install/health failed; inspect $backup on the host"
  # Expand locally: backup matched the fixed prefix and eight alphanumerics above.
  # shellcheck disable=SC2029
  ssh "${SSH_OPTS[@]}" "$target" "umask 077; printf 'complete\n' > $backup/upgrade-complete" </dev/null || die "$host: SSH lost after upgrade; inspect $backup"
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
(( ALLOW_DIVERGENT )) && log 'WARNING: --allow-divergent set; a divergent or unverifiable development lineage on this host may be installed'
(( ALLOW_NO_DEFAULT_ROUTE )) && log 'WARNING: --allow-no-default-route set; this host is upgraded even without any default route'
for h in "${HOSTS[@]}"; do deploy_host "$h"; done
log "deployed to: ${HOSTS[*]}"
