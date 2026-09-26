#!/usr/bin/env bash
# test/topology/bgp/run.sh — P12 topology test on the host VPP: the in-process agent with linux-cp pairs on the slot's
# af_packet rig, the slot's FRR (frrtest, ns-<prefix>-frr) and two FRR peers in the rig namespaces (eBGP, 100 prefixes
# each). The test itself lives in apps/agent/internal/agent/p12_topology_integration_test.go (it needs the agent-internal
# frrtest harness, which a module under test/ cannot import).
#
#   eval "$(tools/lab env <slot>)"; test/topology/bgp/run.sh [go test args]
#   VRX_P12_FIB=private VRX_VPP_API_SOCKET=<private api.sock> test/topology/bgp/run.sh
#       + the VPP FIB checks (row P12-fib-proof, docs/status/tasks/P12.md): ONLY on a VPP of the slot's own
#         (LAB-vpp-per-slot) whose startup.conf has `linux-cp { default netns ns-<prefix>-frr }`; the test refuses the
#         shared VPP and never changes a VPP-global setting (D-071, manager decision on review H3)
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
