#!/usr/bin/env bash
# test/integration/smoke/run.sh — run the P04 smoke test against the host VPP under the shared lab lock.
#   NGFW_TEST_PREFIX=w3 test/integration/smoke/run.sh [go test args]
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${NGFW_TEST_PREFIX:?set NGFW_TEST_PREFIX (your slot prefix, e.g. w3 — see tools/lab env <slot>)}"
export NGFW_INTEGRATION=1
cd "$HERE"
exec "$ROOT/tools/lab" lock shared go test -count=1 -v "$@" ./...
