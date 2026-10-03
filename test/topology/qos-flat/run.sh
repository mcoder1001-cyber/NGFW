#!/usr/bin/env bash
# test/topology/qos-flat/run.sh — F-qos-flat host test: the real ngfw-agent (built from this tree, owner = slot prefix)
# against the host VPP, on the slot's loopbacks only (no af_packet, no packets: no rig, no V19 preflight needed).
#   eval "$(tools/lab env <slot>)"; test/topology/qos-flat/run.sh [go test args]
# Host runs on the shared VPP wait for TD-25 (manager, 2026-09-25). One host test package at a time (D-087).
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
export NGFW_QOS_AGENT_BIN="$ROOT/apps/agent/bin/ngfw-agent" NGFW_INTEGRATION=1
cd "$HERE"
exec "$ROOT/tools/lab" lock shared go test -count=1 -v -timeout 10m "$@" .
