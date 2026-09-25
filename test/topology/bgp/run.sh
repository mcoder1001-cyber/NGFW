#!/usr/bin/env bash
# test/topology/bgp/run.sh — P12 topology test on the host VPP: the in-process agent with linux-cp pairs on the slot's
# af_packet rig, the slot's FRR (frrtest, ns-<prefix>-frr) and two FRR peers in the rig namespaces (eBGP, 100 prefixes
# each). The test itself lives in apps/agent/internal/agent/p12_topology_integration_test.go (it needs the agent-internal
# frrtest harness, which a module under test/ cannot import).
#
#   eval "$(tools/lab env <slot>)"; test/topology/bgp/run.sh [go test args]
#   VRX_P12_LINUXNL=1 test/topology/bgp/run.sh   # + the linux-nl FIB proof — ONLY in a manager window (P12-questions Q1:
#                                                 #   it sets the VPP-global lcp default netns for < 1 s, globals lock held)
#
# Holds the shared lab lock for the run only (D-094); one host test package at a time (D-087).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${VRX_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
[[ "$VRX_TEST_PREFIX" =~ ^w([0-9]{1,2})$ ]] || { echo "run.sh: VRX_TEST_PREFIX must be w<N>" >&2; exit 1; }
[[ -d /run/vrx-test ]] || install -d -m 0755 /run/vrx-test
[[ -d "/run/vrx-test/$VRX_TEST_PREFIX" ]] || install -d -m 0755 "/run/vrx-test/$VRX_TEST_PREFIX"
export VRX_INTEGRATION=1 VRX_P12_TOPOLOGY=1
cd "$ROOT/apps/agent"
exec "$ROOT/tools/lab" lock shared go test -count=1 -v -timeout 20m -run TestP12TopologyOnHost "$@" ./internal/agent/
