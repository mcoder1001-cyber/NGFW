#!/usr/bin/env bash
# test/topology/object-model/run.sh — F-object-model topology + restart-safety test on slot N (real agent, real API, slot
# PostgreSQL; no VPP object is created, no rig). The FQDN objects are served by the test's own DNS responder on
# 127.0.0.1:<ephemeral> (never port 53, never the host's resolver configuration).
#   eval "$(tools/lab env <slot>)"; test/topology/object-model/run.sh [go test args]
# Builds the agent and vrx-agentctl (apps/agent/bin, git-ignored; /run is noexec) and apps/api/dist, then runs the test
# under the shared lab lock (the test takes it itself as well, D-094: only for the run).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${VRX_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
[[ "$VRX_TEST_PREFIX" =~ ^w([0-9]{1,2})$ ]] || { echo "run.sh: VRX_TEST_PREFIX must be w<N>" >&2; exit 1; }
RUN="/run/vrx-test/$VRX_TEST_PREFIX"
[[ -d /run/vrx-test ]] || install -d -m 0755 /run/vrx-test
[[ -d "$RUN" ]] || install -d -m 0755 "$RUN"
( cd "$ROOT/apps/agent" && go build -o bin/vrx-agent ./cmd/vrx-agent && go build -o bin/vrx-agentctl ./cmd/vrx-agentctl )
( cd "$ROOT/apps/api" && pnpm build >/dev/null )
export VRX_OM_AGENT_BIN="$ROOT/apps/agent/bin/vrx-agent" VRX_OM_AGENTCTL_BIN="$ROOT/apps/agent/bin/vrx-agentctl" VRX_INTEGRATION=1
cd "$HERE"
exec "$ROOT/tools/lab" lock shared go test -count=1 -v -timeout 20m "$@" .
