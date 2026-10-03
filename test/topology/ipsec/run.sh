#!/usr/bin/env bash
# test/topology/ipsec/run.sh — P11 IPsec site-to-site topology test. NOT IMPLEMENTED.
#
# The executable test (built kernel-vpp charon on the slot's af_packet rig against a stock peer: ESP-only
# tcpdump, rising `vppctl show ipsec sa` counters, agent-restart simulation, rollback) needs the fixed
# ngfw-strongswan build (row P11-pkg: the upstream plugin cannot load against VPP 26.06 and allocates SPD/SA
# ids from 1 in the shared VPP) and is row P11-host's. The procedure is in README.md. This script exits
# non-zero so nothing mistakes it for a passing test.
echo "test/topology/ipsec: NOT IMPLEMENTED — the host run is row P11-host (after P11-pkg); see README.md" >&2
exit 2
