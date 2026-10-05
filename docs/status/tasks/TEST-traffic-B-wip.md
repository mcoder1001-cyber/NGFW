# TEST-traffic-B WIP

Branch codex/test-traffic-b-20261005; worktree /root/ngfw-wt/TEST-traffic-B-20261005.
Base/local parent d314f0728. Remote checkpoint publication pending manager connector.
Owned files traffic-b/** and TEST-traffic-B* only.

Implemented slot-parametric API GRE/VXLAN kernel peer campaign with ping, scoped text tcpdump, state verification and rollback. Added full phase inventory and strict run-bound acceptance envelope that never treats adapter assertions as independent packet acceptance. Generic orchestration cannot pass a whole wave.

Remaining: execute tunnel campaign against own stack, compose actual WG/IPsec/routing/DHCP packet fixtures, review and full quick. Existing fixed-slot8 IPsec and slot6 OSPF wrapper cannot silently consume assigned slot27. Native route-based DEC replaces historical kernel-vpp prerequisite.

Exact next command: python3 -m unittest discover -s test/topology/traffic-b -v
No packet acceptance claimed yet. No system services changed.

2026-10-05 checkpoint2: local f3426976e published exactly as remote76d58a0327d76b02a97dbfc7e798f81c79a27a06 (manager verified identical tree); CLI push403. Own extra manager-approved hunks p12_topology_integration_test.go, ospf_topology_integration_test.go, rpc_wireguard_integration_test.go, traffic_b_packet_integration_test.go and kea-dhcp-relay/dhcp_test.go: opt-in private-VPP packet hooks only. Ordinary fixtures unchanged when NGFW_TRAFFIC_B is unset.

Built private mount+network isolation, tcpdump readiness via stderr listening signal, BGP/OSPF served learned-prefix ICMP+exact TCP echo, WG UDP capture and ping, DHCP relay DISCOVER giaddr capture/lease proof, owned API/agent stack launcher. Private VPP lifecycle smoke passed and shared PID/NRestarts unchanged. First BGP failed before probes because own redirected run inherited restrictive umask preventing FRR traversal; corrected private wrapper to daemon-compatible022 directories with explicit0600 diagnostic logs. Rerun currently executing. Quick gate currently executing, not claimed green.

2026-10-05 checkpoint3: previous localc27b0b470 exactly published as remoteea970ae4ef618f80329988f7f04a270bfe411ded. Full unchanged tools/ci.sh --base origin/main PASSED (20m21s, logs /root/ngfw-wt/logs/ci/TEST-traffic-B-20261005-20261005-154239-3873918; includes35Turbo tasks, Go race/lint/build, all topology modules vet/unit,149 startup checks). Tree evolved during execution: final coherent source requires another gate before merge.

Actual BGP TestP12TopologyOnHost PASSED92.83s with200VPP learned routes, bidirectional learned-prefix ICMP and exact TCP echo, source-owned peer return /32 route, scoped capture readiness, idempotence/withdrawal/rollback, SHARED_VPP_UNCHANGED. Actual WG TestWireguardHandshakeOnHost PASSED6.75s including ping/UDP capture, handshake state and rollback; private fixture uses slotloopback instance2760 because upstream WG limits instances<16384 (27060 invalid). Neither of these fixture tests establishes slot REST candidate commit acceptance for their features; record that distinction.

Built deterministic primary dispatcher using existing native production PSK/certificate campaigns plus reviewed protocol hooks, no arbitrary adapters. Enforced observed private mount/network namespaces before private child mounts; logs/capture0600, process-owned command group deadline and cleanup; extracted native peer binaries mounted read-only within private namespace. Native source built, runtime validation still owed. Tunnel REST baseline currently failsHTTP400before agent apply; investigating exact validator pointers. OSPF executing. DHCP live and final review still owed.
