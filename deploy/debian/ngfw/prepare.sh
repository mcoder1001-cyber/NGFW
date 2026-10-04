#!/usr/bin/env bash
# Prepare a clean source-package tree; never install packages on the build host.
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../../.." && pwd)
if [[ $# != 2 ]]; then
  echo 'usage: prepare.sh VERIFIED_VPP_OUTPUT NEW_OUTPUT_DIRECTORY' >&2
  exit 2
fi
VPP_OUTPUT=$(realpath -e -- "$1")
OUTPUT=$(realpath -m -- "$2")
[[ ! -e $OUTPUT ]] || { echo 'output must not exist' >&2; exit 1; }
"$ROOT/deploy/vpp/verify.sh" --require-files "$VPP_OUTPUT" --install-gate
VPP_VERSION=$(python3 - "$VPP_OUTPUT/manifest.json" <<'PY'
import json, re, sys
manifest = json.load(open(sys.argv[1], encoding='utf-8'))
version = manifest['version']
if not re.fullmatch(r'26\.06-release\+ngfw[1-9][0-9]*', version):
    sys.exit('non-product VPP version')
print(version)
PY
)
[[ -z $(git -C "$ROOT" status --porcelain) ]] || { echo 'refusing dirty source checkout' >&2; exit 1; }
command -v go >/dev/null
command -v pnpm >/dev/null
# Build from this checkout before staging: existing ignored dist files can belong
# to an older source SHA even when git status is clean. A build failure must not
# publish an output directory containing the previous compiled application.
(cd "$ROOT" && pnpm exec turbo run build --filter=@ngfw/api --filter=@ngfw/web --concurrency=2)
for input in apps/api/dist/main.js apps/web/dist/index.html; do
  [[ -f $ROOT/$input ]] || { echo "missing build input: $input" >&2; exit 1; }
done
[[ -z $(git -C "$ROOT" status --porcelain) ]] || { echo 'build changed tracked source; regenerate and review first' >&2; exit 1; }
mkdir -p -- "$OUTPUT"
cp -a -- "$ROOT/deploy/debian/ngfw/debian" "$ROOT/deploy/debian/ngfw/tests" "$ROOT/deploy/debian/ngfw/assets" "$OUTPUT/"
STAGE=$OUTPUT/stage
mkdir -p "$STAGE/usr/sbin" "$STAGE/usr/lib/ngfw" "$STAGE/usr/share/ngfw/web" "$STAGE/usr/lib/systemd/system"
for binary in ngfw-agent ngfw-startupgen ngfw-vppcheck; do
  (cd "$ROOT/apps/agent" && CGO_ENABLED=0 go build -trimpath -o "$STAGE/usr/sbin/$binary" "./cmd/$binary")
done
(cd "$ROOT" && pnpm --filter @ngfw/api deploy --prod "$STAGE/usr/lib/ngfw/api")
[[ -f $STAGE/usr/lib/ngfw/api/dist/main.js ]] || { echo 'pnpm deploy omitted compiled API' >&2; exit 1; }
cp -a "$ROOT/apps/api/migrations" "$STAGE/usr/lib/ngfw/api/"
cp -a "$ROOT/apps/web/dist/." "$STAGE/usr/share/ngfw/web/"
install -m 0755 "$ROOT/deploy/vpp/apply-startup.sh" "$STAGE/usr/lib/ngfw/apply-startup.sh"
install -m 0644 "$ROOT/deploy/systemd/ngfw-agent.service" "$ROOT/deploy/systemd/ngfw-api.service" "$ROOT/deploy/systemd/ngfw-firstboot.service" "$ROOT/deploy/systemd/ngfw-firewall-bootstrap.service" "$STAGE/usr/lib/systemd/system/"
install -m 0755 "$ROOT/deploy/vpp/apply-executor.py" "$STAGE/usr/lib/ngfw/apply-executor.py"
install -m 0644 "$ROOT/deploy/vpp/apply-executor.service" "$ROOT/deploy/vpp/apply-executor.socket" "$STAGE/usr/lib/systemd/system/"
printf '%s\n' "$VPP_VERSION" > "$STAGE/VPP_VERSION"
COMMIT=$(git -C "$ROOT" rev-parse --short=12 HEAD)
[[ -z $(git -C "$ROOT" status --porcelain) ]] || { echo 'refusing dirty source checkout' >&2; exit 1; }
VERSION=0.1.0~dev+$COMMIT
sed -i "1s/(.*)/($VERSION)/" "$OUTPUT/debian/changelog"
echo "Prepared source package: $OUTPUT"
echo 'Release gates remain: licensing, dynamic punt admission, privilege compatibility and appliance acceptance; this checkpoint is not a release.'
