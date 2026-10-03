#!/usr/bin/env bash
# test/topology/bonding/run.sh — F-bonding topology + restart-safety test on the host VPP (fixture taps, no packets).
#   eval "$(tools/lab env <slot>)"; test/topology/bonding/run.sh [go test args]
# Builds the agent and the read-only VPP pre-flight (apps/agent/bin, git-ignored; /run is noexec) and apps/api/dist, runs
# the pre-flight (D-095 d; it never changes VPP), then the test under the shared lab lock (the test takes it itself as
# well, D-094: only for the run). Screenshots: NGFW_F_BONDING_SHOTS=<node script> NGFW_F_BONDING_SHOTS_OUT=<dir> (web build
# apps/web/dist required; the script is kept outside the repository).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${NGFW_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
[[ "$NGFW_TEST_PREFIX" =~ ^w([0-9]{1,2})$ ]] || { echo "run.sh: NGFW_TEST_PREFIX must be w<N>" >&2; exit 1; }
RUN="/run/ngfw-test/$NGFW_TEST_PREFIX"
[[ -d /run/ngfw-test ]] || install -d -m 0755 /run/ngfw-test
[[ -d "$RUN" ]] || install -d -m 0755 "$RUN"
( cd "$ROOT/apps/agent" && go build -o bin/ngfw-agent ./cmd/ngfw-agent && go build -o bin/ngfw-vpp-preflight ./cmd/ngfw-vpp-preflight )
( cd "$ROOT/apps/api" && pnpm build >/dev/null )
"$ROOT/apps/agent/bin/ngfw-vpp-preflight"
export NGFW_F_BONDING_AGENT_BIN="$ROOT/apps/agent/bin/ngfw-agent" NGFW_INTEGRATION=1
cd "$HERE"
exec "$ROOT/tools/lab" lock shared go test -count=1 -v -timeout 20m "$@" .
