#!/usr/bin/env bash
# test/topology/nat44-ed-sessions/run.sh — F-nat44-ed-sessions topology + packet + restart-safety test on the host VPP
# (af_packet rig).  eval "$(tools/lab env <slot>)"; test/topology/nat44-ed-sessions/run.sh [go test args]
# Builds the agent and the V19 preflight (apps/agent/bin, git-ignored; /run is noexec) and apps/api/dist, runs the
# preflight (D-095: it must exit 0 before any packet), then the test under the shared lab lock (the test takes it
# itself as well, D-094: only for the run).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${VRX_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
[[ "$VRX_TEST_PREFIX" =~ ^w([0-9]{1,2})$ ]] || { echo "run.sh: VRX_TEST_PREFIX must be w<N>" >&2; exit 1; }
RUN="/run/vrx-test/$VRX_TEST_PREFIX"
[[ -d /run/vrx-test ]] || install -d -m 0755 /run/vrx-test
[[ -d "$RUN" ]] || install -d -m 0755 "$RUN"
( cd "$ROOT/apps/agent" && go build -o bin/vrx-agent ./cmd/vrx-agent && go build -o bin/vrx-vpp-preflight ./cmd/vrx-vpp-preflight )
( cd "$ROOT/apps/api" && pnpm build >/dev/null )
"$ROOT/apps/agent/bin/vrx-vpp-preflight"
export VRX_NAT_AGENT_BIN="$ROOT/apps/agent/bin/vrx-agent" VRX_PREFLIGHT_BIN="$ROOT/apps/agent/bin/vrx-vpp-preflight" VRX_INTEGRATION=1
cd "$HERE"
exec "$ROOT/tools/lab" lock shared go test -count=1 -v -timeout 20m "$@" .
