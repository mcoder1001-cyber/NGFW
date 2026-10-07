#!/usr/bin/env bash
# VMware lab / test machine: native traffic tools, analysis, automation.
set -euo pipefail
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || {
  echo "only linux-amd64 lab bootstrap is supported" >&2; exit 1;
}
if [[ ${1:-} == --check-config && $# == 1 ]]; then
  printf 'lab=vmware native-tools=iperf3,netperf,tshark,tcpdump,frr\n'
  exit 0
fi
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
# shellcheck source=install-common.sh
source "$SCRIPT_DIR/install-common.sh"
ngfw_install_init "$@"
if [[ $NGFW_INSTALL_DRY_RUN == 1 && -z ${NGFW_LAB_REQUIREMENTS:-} ]]; then
  ngfw_install_plan 'APT native traffic/analysis tools; automation venv /opt/ngfw-test BLOCKED: no reviewed Python lab lock'
  exit 0
fi
# Caller supplies a reviewed complete hashed pip lock; no floating fallback.
# The SDK pytest lock is not a lock for this independent automation environment.
[[ -n ${NGFW_LAB_REQUIREMENTS:-} ]] || {
  echo 'REFUSED: NGFW_LAB_REQUIREMENTS reviewed complete Python lab lock required' >&2; exit 1;
}
lab_requirements=$(python3 - "$NGFW_LAB_REQUIREMENTS" <<'PYLOCK'
import os, re, stat, sys
fd = os.open(sys.argv[1], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
with os.fdopen(fd) as stream:
    info = os.fstat(stream.fileno())
    if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= 1024 * 1024:
        raise SystemExit('REFUSED: bounded regular lab lock required')
    data = stream.read(1024 * 1024 + 1)
if len(data) > 1024 * 1024:
    raise SystemExit('REFUSED: lab lock too large')
packages = set()
for line in data.replace('\\\n', ' ').splitlines():
    line = line.strip()
    if not line or line.startswith('#'):
        continue
    match = re.fullmatch(r'([A-Za-z0-9][A-Za-z0-9_.-]*)==[A-Za-z0-9.!+_-]+(?:[ \t]+--hash=sha256:[0-9a-f]{64})+', line)
    if not match:
        raise SystemExit('REFUSED: lab lock requires exact versions and SHA256 hashes, without directives/URLs')
    name = re.sub(r'[-_.]+', '-', match[1]).lower()
    if name in packages:
        raise SystemExit('REFUSED: duplicate lab dependency')
    packages.add(name)
required = {'pip', 'robotframework', 'robotframework-sshlibrary', 'scapy', 'pytest', 'requests'}
if not required <= packages:
    raise SystemExit('REFUSED: lab lock missing required automation packages')
print(data, end='')
PYLOCK
)
if [[ $NGFW_INSTALL_DRY_RUN == 1 ]]; then
  ngfw_install_plan 'APT native traffic/analysis tools; venv /opt/ngfw-test; supplied version/hash lab lock validated; require-hashes dependency closure during apply'
  exit 0
fi
venv_dir=$(ngfw_install_path /opt/ngfw-test)
work_dir=$(ngfw_install_path /tmp)
mkdir -p "$work_dir" "$(dirname -- "$venv_dir")"
lock_snapshot=$(mktemp "$work_dir/ngfw-lab-lock.XXXXXXXX")
trap 'rm -f -- "$lock_snapshot"' EXIT
printf '%s\n' "$lab_requirements" > "$lock_snapshot"
export DEBIAN_FRONTEND=noninteractive
TRAFFIC=(iperf3 netperf)
ANALYSIS=(tshark tcpdump)
BASE=(python3-venv python3-pip git curl jq frr)
apt-get update
apt-get install -y "${TRAFFIC[@]}" "${ANALYSIS[@]}" "${BASE[@]}"
python3 -m venv "$venv_dir"
export TMPDIR="$work_dir"
"$venv_dir/bin/pip" --isolated --cache-dir "$work_dir/pip-cache" install --quiet --upgrade --require-hashes -r "$lock_snapshot"
echo "Lab tools installed. Activate test env: source $venv_dir/bin/activate"
