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
if not re.fullmatch(r'26\.06-release\+vrx[1-9][0-9]*', version):
    sys.exit('non-product VPP version')
print(version)
PY
)
# Refuse to package stale or missing compiled applications. Run the unchanged
# quick gate before this command, then deploy from the same reviewed checkout.
for input in apps/api/dist/main.js apps/web/dist/index.html; do
  [[ -f $ROOT/$input ]] || { echo "missing build input: $input" >&2; exit 1; }
done
[[ -z $(git -C "$ROOT" status --porcelain) ]] || { echo 'refusing dirty source checkout' >&2; exit 1; }
command -v go >/dev/null
command -v pnpm >/dev/null
mkdir -p -- "$OUTPUT"
cp -a -- "$ROOT/deploy/debian/vrx/debian" "$ROOT/deploy/debian/vrx/tests" "$ROOT/deploy/debian/vrx/assets" "$OUTPUT/"
STAGE=$OUTPUT/stage
mkdir -p "$STAGE/usr/sbin" "$STAGE/usr/lib/vrx" "$STAGE/usr/share/vrx/web" "$STAGE/usr/lib/systemd/system"
for binary in vrx-agent vrx-startupgen vrx-vppcheck; do
  (cd "$ROOT/apps/agent" && CGO_ENABLED=0 go build -trimpath -o "$STAGE/usr/sbin/$binary" "./cmd/$binary")
done
(cd "$ROOT" && pnpm --filter @ngfw/api deploy --prod "$STAGE/usr/lib/vrx/api")
[[ -f $STAGE/usr/lib/vrx/api/dist/main.js ]] || { echo 'pnpm deploy omitted compiled API' >&2; exit 1; }
cp -a "$ROOT/apps/api/migrations" "$STAGE/usr/lib/vrx/api/"
cp -a "$ROOT/apps/web/dist/." "$STAGE/usr/share/vrx/web/"
install -m 0755 "$ROOT/deploy/vpp/apply-startup.sh" "$STAGE/usr/lib/vrx/apply-startup.sh"
install -m 0644 "$ROOT/deploy/systemd/vrx-agent.service" "$ROOT/deploy/systemd/vrx-api.service" "$ROOT/deploy/systemd/vrx-firstboot.service" "$STAGE/usr/lib/systemd/system/"
printf '%s\n' "$VPP_VERSION" > "$STAGE/VPP_VERSION"
COMMIT=$(git -C "$ROOT" rev-parse --short=12 HEAD)
[[ -z $(git -C "$ROOT" status --porcelain) ]] || { echo 'refusing dirty source checkout' >&2; exit 1; }
VERSION=0.1.0~dev+$COMMIT
sed -i "1s/(.*)/($VERSION)/" "$OUTPUT/debian/changelog"
echo "Prepared source package: $OUTPUT"
echo 'Firstboot/TLS/base policy are unfinished; this checkpoint is not a release.'
