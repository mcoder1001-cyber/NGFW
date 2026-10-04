# OSPF topology MTU closeout

Branch `codex/closeout-ospf`, base `f41491d06`; source checkpoint `904362b4463828600224a77b1c08a0d8841e70da`. Remote publication pending manager connector, not yet published.

Owned source: `apps/agent/internal/agent/ospf_topology_integration_test.go`.

The actual parent netns run had both OSPF peers stuck Exchange for 90 seconds. Diagnostics show NGFW LCP TAPs MTU 9000 versus peer veth MTU 1500, with MTU mismatch detection enabled. Pinned VPP linux-cp source copies the underlying VPP interface's L3 MTU onto its TAP; the AF_PACKET default is 9000 while Linux veth defaults to 1500. This fixture never declared matching MTUs.

The fixture now explicitly sets both ends of each owned rig veth to 9000 before handing the interfaces to the agent and starting FRR. It neither disables OSPF MTU checking nor changes product code. All original Full adjacency, 100-route, restart recovery and rollback assertions remain.

Actual validation: **PASS**, private slot-7 netns `TestOSPFTopologyOnHost` 63.99 seconds, exit 0. Both neighbors reached Full, 100 OSPF routes were learned, agent restart with pair loss recovered in 7.4 seconds (target ≤30 seconds), and rollback removed all OSPF routes. Shared VPP NRestarts remained 0. Full log: `closeout-ospf-evidence/netns.log`. Static `tools/ci.sh check --base f41491d06` PASS, see `closeout-ospf-evidence/check.log`. Exact command:

```
env $(tools/lab env 7 | sed 's/^export //') NGFW_ISOLATED_TEST_RUN=1 tools/heavy.sh python3 test/topology/hardware-smoke/isolated-vpp.py test/topology/ospf/run.sh
```

During the live run a read-only `nsenter -t 3632753 -m` inspection independently confirmed the LAN root veth, peer veth, LCP TAP and VPP interface all had MTU 9000. Both LAN/WAN set commands succeeded; OSPF MTU mismatch detection was not disabled. This verifies namespaced FRR RIB acceptance; VPP dynamic-route FIB proof from root-FRR mode remains separate and was not executed. Full quick and independent review remain integration requirements.
