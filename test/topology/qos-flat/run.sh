#!/usr/bin/env bash
# test/topology/qos-flat/run.sh — F-qos-flat host test: the real vrx-agent (built from this tree, owner = slot prefix)
# against the host VPP, on the slot's loopbacks only (no af_packet, no packets: no rig, no V19 preflight needed).
#   eval "$(tools/lab env <slot>)"; test/topology/qos-flat/run.sh [go test args]
# Host runs on the shared VPP wait for TD-25 (manager, 2026-09-25). One host test package at a time (D-087).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${VRX_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
[[ "$VRX_TEST_PREFIX" =~ ^w([0-9]{1,2})$ ]] || { echo "run.sh: VRX_TEST_PREFIX must be w<N>" >&2; exit 1; }
RUN="/run/vrx-test/$VRX_TEST_PREFIX"
[[ -d /run/vrx-test ]] || install -d -m 0755 /run/vrx-test
[[ -d "$RUN" ]] || install -d -m 0755 "$RUN"
( cd "$ROOT/apps/agent" && go build -o bin/vrx-agent ./cmd/vrx-agent )
export VRX_QOS_AGENT_BIN="$ROOT/apps/agent/bin/vrx-agent" VRX_INTEGRATION=1
cd "$HERE"
exec "$ROOT/tools/lab" lock shared go test -count=1 -v -timeout 10m "$@" .
