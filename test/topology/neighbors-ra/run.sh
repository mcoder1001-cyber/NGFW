#!/usr/bin/env bash
# test/topology/neighbors-ra/run.sh — F-neighbors-ra host check on the host VPP (prefixed loopbacks, no rig, no packets).
#   eval "$(tools/lab env <slot>)"; test/topology/neighbors-ra/run.sh [go test args]
# Builds the agent binary (apps/agent/bin, git-ignored) and apps/api/dist, then runs the test under the shared lab lock
# (the test takes it itself as well, D-094: only for the run).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${NGFW_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
[[ "$NGFW_TEST_PREFIX" =~ ^w([0-9]{1,2})$ ]] || { echo "run.sh: NGFW_TEST_PREFIX must be w<N>" >&2; exit 1; }
RUN="/run/ngfw-test/$NGFW_TEST_PREFIX"
[[ -d /run/ngfw-test ]] || install -d -m 0755 /run/ngfw-test
[[ -d "$RUN" ]] || install -d -m 0755 "$RUN"
( cd "$ROOT/apps/agent" && go build -o bin/ngfw-agent ./cmd/ngfw-agent )
( cd "$ROOT/apps/api" && pnpm build >/dev/null )
export NGFW_NRA_AGENT_BIN="$ROOT/apps/agent/bin/ngfw-agent" NGFW_INTEGRATION=1
cd "$HERE"
exec "$ROOT/tools/lab" lock shared go test -count=1 -v -timeout 15m "$@" .
