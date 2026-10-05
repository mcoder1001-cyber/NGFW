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
