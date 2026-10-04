#!/usr/bin/env bash
# test/topology/ospf/run.sh — F-ospf topology test on the host VPP: the in-process agent with linux-cp pairs on the slot's
# af_packet rig, the slot's FRR (mgmtd, zebra, staticd, ospfd) and two ospfd peers in the rig namespaces (area 0, 50
# redistributed prefixes each). The test itself lives in apps/agent/internal/agent/ospf_topology_integration_test.go (it
# needs the agent-internal frrtest harness, which a module under test/ cannot import).
#
#   eval "$(tools/lab env <slot>)"; test/topology/ospf/run.sh [go test args]
#       FRR's RIB is checked; the NGFW-side FRR runs in ns-<prefix>-frr (VPP's linux_nl cannot hear it, P12 Q1)
#   NGFW_OSPF_FIB=root test/topology/ospf/run.sh
#       + the VPP FIB checks: the NGFW-side FRR runs in the ROOT network namespace with the slot pathspace (D-119 M3: a
#         root-netns zebra one slot at a time — run it only as the single FRR host row), under the exclusive globals lock
#         inside the shared lab lock (D-167); fails closed when an LCP pair exists, the lcp default netns is set, an
#         FRR-protocol route is in the root kernel table or the system frr unit is active
#
# Holds the shared lab lock for the run only (D-094); one host test package at a time (D-087). Every lock fd is closed in
# background children by the harness (none are spawned here).
set -euo pipefail
HERE="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
: "${NGFW_TEST_PREFIX:?eval \"\$(tools/lab env <slot>)\" first}"
[[ "$NGFW_TEST_PREFIX" =~ ^w([0-9]{1,2})$ ]] || { echo "run.sh: NGFW_TEST_PREFIX must be w<N>" >&2; exit 1; }
[[ -d /run/ngfw-test ]] || install -d -m 0755 /run/ngfw-test
[[ -d "/run/ngfw-test/$NGFW_TEST_PREFIX" ]] || install -d -m 0755 "/run/ngfw-test/$NGFW_TEST_PREFIX"
export NGFW_INTEGRATION=1 NGFW_OSPF_TOPOLOGY=1
echo "run.sh: $(date +%FT%T) VPP $(systemctl show vpp -p NRestarts) before, mode ${NGFW_OSPF_FIB:-netns}"
cd "$ROOT/apps/agent"
if "$ROOT/tools/lab" lock shared go test -count=1 -v -timeout 20m -run TestOSPFTopologyOnHost "$@" ./internal/agent/; then rc=0; else rc=$?; fi
echo "run.sh: $(date +%FT%T) VPP $(systemctl show vpp -p NRestarts) after, exit $rc"
exit $rc
