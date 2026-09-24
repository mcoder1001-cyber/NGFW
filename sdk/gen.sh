#!/usr/bin/env bash
# sdk/gen.sh — regenerate the automation clients from the API's OpenAPI document.
#   sdk/gen.sh                  build @ngfw/api (+ deps), export OpenAPI → sdk/openapi.json (git-ignored), regenerate
#                               sdk/python/vrx/_generated/* and sdk/terraform/internal/provider/zz_*_gen.go
#   sdk/gen.sh --check          same, then fail if the committed generated files differ (the "generated output is clean" gate)
#   sdk/gen.sh --openapi <f>    use an existing OpenAPI JSON instead of building the API (fast; e.g. packages/api-client/openapi.json after `pnpm gen`)
# Not hooked into tools/ci.sh (not owned by this task; the API build takes ~1 min) — run it with sdk/test.sh before
# merging anything that changes packages/schema or the API routes (docs/user/system/sdk-terraform-ansible.md).
set -euo pipefail
SDK="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(git -C "$SDK" rev-parse --show-toplevel)"
export GOTOOLCHAIN=local
CHECK=0; SPEC=""
while (($#)); do
  case "$1" in
    --check) CHECK=1 ;;
    --openapi) SPEC=$(realpath "$2"); shift ;;
    *) sed -n '2,8p' "$0" | sed -E 's/^# ?//'; exit 2 ;;
  esac
  shift
done

if [[ -z $SPEC ]]; then
  echo "gen: building @ngfw/api and its workspace deps"
  (cd "$ROOT" && pnpm --filter "@ngfw/api..." run build >/dev/null)
  (cd "$ROOT/apps/api" && node dist/openapi.js "$SDK/openapi.json")
  SPEC="$SDK/openapi.json"
fi

python3 "$SDK/python/tools/gen.py" "$SPEC" "$SDK/python/vrx/_generated"
go -C "$SDK/terraform" run ./tools/genschema -openapi "$SPEC" -out internal/provider

if ((CHECK)); then
  dirty=$(git -C "$ROOT" status --porcelain -- sdk/python/vrx/_generated 'sdk/terraform/internal/provider/zz_*_gen.go')
  if [[ -n $dirty ]]; then
    echo "gen: generated SDK files are not up to date with the OpenAPI document — commit them:" >&2
    echo "$dirty" >&2
    exit 1
  fi
  echo "gen: clean — sdk/python/vrx/_generated sdk/terraform/internal/provider/zz_*_gen.go"
fi
