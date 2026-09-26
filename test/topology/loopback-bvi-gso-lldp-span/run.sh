#!/usr/bin/env bash
# test/topology/loopback-bvi-gso-lldp-span/run.sh — F-loopback-bvi-gso-lldp-span topology + restart-safety test on the host VPP (slot loopbacks and an ERSPAN GRE fixture, no packets).
#   eval "$(tools/lab env <slot>)"; test/topology/loopback-bvi-gso-lldp-span/run.sh [go test args]
# Builds the agent binary (apps/agent/bin, git-ignored; /run is noexec) and apps/api/dist, then runs the test under
# the shared lab lock (the test takes it itself as well, D-094: only for the run).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${VRX_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
[[ "$VRX_TEST_PREFIX" =~ ^w([0-9]{1,2})$ ]] || { echo "run.sh: VRX_TEST_PREFIX must be w<N>" >&2; exit 1; }
# the slot run dir stays 0755 and is never re-moded (frr/_chrony test daemons traverse it, D-106/D-107): create it
# only when missing; the test keeps its own files private in "$RUN/lbgs" (0700)
RUN="/run/vrx-test/$VRX_TEST_PREFIX"
[[ -d /run/vrx-test ]] || install -d -m 0755 /run/vrx-test
[[ -d "$RUN" ]] || install -d -m 0755 "$RUN"
( cd "$ROOT/apps/agent" && go build -o bin/vrx-agent ./cmd/vrx-agent )
( cd "$ROOT/apps/api" && pnpm build >/dev/null )
export VRX_LBGS_AGENT_BIN="$ROOT/apps/agent/bin/vrx-agent" VRX_INTEGRATION=1
cd "$HERE"
exec "$ROOT/tools/lab" lock shared go test -count=1 -v -timeout 15m "$@" .
