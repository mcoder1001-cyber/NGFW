#!/usr/bin/env bash
# Build and stage the shared library locally; never publish to a registry.
set -euo pipefail
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
source_dir=${1:-"$repo/../common-ui"}
source_dir=$(cd "$source_dir" && pwd)
if [[ ! -f "$source_dir/package.json" ]]; then
  echo "Common UI package.json not found: $source_dir" >&2
  exit 1
fi
npm --prefix "$source_dir" run build
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
(cd "$source_dir" && npm pack --ignore-scripts --pack-destination "$stage" >/dev/null)
tar -xzf "$stage"/*.tgz -C "$stage"
node -e 'const p=require(process.argv[1]); if(p.name!=="@amnafzar/common-ui-lib" || !p.exports?.["./theme"] || !p.exports?.["./grid"]) process.exit(1)' "$stage/package/package.json"
rm -rf "$repo/.yalc/@amnafzar/common-ui-lib"
mkdir -p "$repo/.yalc/@amnafzar/common-ui-lib"
cp -R "$stage/package/." "$repo/.yalc/@amnafzar/common-ui-lib/"
(cd "$repo" && pnpm install --no-frozen-lockfile --prefer-offline)
