# Current testing closeout audit — 2026-10-04

Source: base 664544349, audit branch codex/closeout-traffic-audit at 4c8d1b247.
Owned files: docs/status/tasks/closeout-traffic*. No product, board or host mutation.
The prior bidirectional195/250 AF_PACKET link test is supplied session evidence; this audit did not rerun it. Physical connectivity does not certify individual feature acceptance.

## Fresh offline outcomes

Commands: python3 .github/scripts/traffic-<name>-fixtures.py --check-policy, then the same command without arguments for foundation/correlation/producer/transaction. Cleanup proof uses --controls and --repetitions; unchanged transaction47 was already run once, avoiding duplication.

| Gate | Named source cases | Policy cases | Result |
|---|---:|---:|---|
| foundation |16 (0.820s)|11|EXIT0, no skips|
| correlation |33 (3.111s)|13|EXIT0, no skips|
| producer |40 (6.102s)|13|EXIT0, no skips|
| transaction |47 (5.561s)|13|EXIT0, no skips|
| cleanup proof |2 controls (0.013s),10 repetitions (12.076s)|17|EXIT0, no skips|

Counts overlap; they must not be summed as unique tests. Source fixture PASS does not mean packet PASS. No SSH, VPP, rig or capture activity occurred.
`python3 test/topology/traffic-a/run.py plan --slot 14` returned status NOTIMPLEMENTED, whole_chain_proven=false, all seven stages NOTIMPLEMENTED. `bash -n` passed for NAT46 host.sh, OSPF run.sh and api400.sh.

## Closeability matrix and exact next commands

All commands below need canonical allocated slot env from tools/lab env, NGFW_INTEGRATION=1 and tools/heavy.sh for Go compilation/testing. Manager runs hosts sequentially with documented shared lab and exclusive globals windows. Parse actual test names and SKIP, not just exit0. Existing historical reports are not current acceptance.

| Task | Supported source and next test | Remaining acceptance / blocker |
|---|---|---|
| F-lb-host | apps/agent: go test -v -count=1 -run '^TestLbOnHost$' ./internal/agent with NGFW_LB_HOST=1; GC test '^TestLbGarbageCollectOnHost$' with NGFW_LB_GLOBALS=1 | Agent config/readback/replay and cleanup evidence; real API/UI screenshot. VIP residue needs GC manager window. Descriptor TestLBOnHost uses separate NGFW_DF7_LB flag; avoid write-only lb.conf unless dedicated disposable VPP. |
| F-srv6-host | go test -v -count=1 -run '^TestSrv6OnHost$' ./internal/agent; '^TestSrv6GlobalsOnHost$' with NGFW_FSRV6_GLOBALS=1 | Global restore window, API/real-agent screen, relevant encapsulated packets separately. |
| F-mpls-srmpls-host | go test -v -count=1 -run '^TestMplsOnHost$' ./internal/agent; globals half NGFW_DF7_GLOBALS=1 | Table0/label bind idempotence and no residue, SR-MPLS/readback/replay/API screen. Descriptor TestMPLSOnHost and sr_mpls TestPolicySteeringOnHost exist; TestEndpointColorOnHost unconditionally SKIP because write-only irreversible effects. |
| F-rule-expiry-host | Existing agent TestACLRuleRemovedAtExpiryWithoutACommit and API e2e rule-expiry are fake/source evidence | No recovered live topology driver. Must prove traffic initially permitted then drops at expiresAt without commit, expired rule absent after agent restart, rollback and real counters. |
| F-global-blocking-host | Agent TestGlobalBlockingOnFake, API e2e global-blocking, live host ACL topology building blocks | No recovered composed live driver. Listed source blocked both directions and local-in on selected interface, unselected/unlisted pass; counters, list refresh/rollback/replay and UI. Historical200k measurement requirements are separate from normal functional acceptance; do not claim performance. |
| F-pppoe-client-host | Descriptor TestCpOnHost needs NGFW_DF6_PPPOE_CP_HOST=1 on dedicated VPP; TestSessionOnHost may SKIP if discovery MAC absent | Descriptors are AC/decap, not dialer. Need kernel PPP, extracted/test peer server, real secret delivery, pppd/session/NAT/reconnect/wrong-password/MSS/address-route mirror. Setup wizard still explicitly disables PPPoE. |
| F-multiwan-host | internal/multiwan runtime/probe fixtures and agent rpc_wan*_test.go, API multiwan e2e | No recovered two-WAN topology driver. Static IPv4/default VRF source supports testing failover/restoration, weighted flows/NAT cleanup/retry/replay/browser. Dynamic gateway, VRF and ABF explicitly remain code follow-ups; cannot close broader historic scope. |
| F-nat46-host | docs/status/tasks/F-nat46-host-evidence/host.sh run after canonical slot17 env; bounded descriptor: go test -v -count=1 -run '^TestNat46OnHost$' ./internal/descriptors/nat46 | Full IPv4-to-IPv6 TCP payload, restart/replay, rollback/API/browser still owed. Current source acceptance previously passed descriptor only. Driver internally invokes bare go test and timeout60 vppctl: wrap its finite invocation with heavy and assess against current timeout policy before execution. |
| F-ospf-host | test/topology/ospf/run.sh; NGFW_OSPF_FIB=root for full FIB; api400.sh; TestOSPFLive ./internal/renderers/frr/ospf | Root mode refuses active system FRR, existing LCP pairs/default namespace or FRR-protocol kernel routes. Full VPP FIB/withdrawal/restart/rollback/API/v3/browser owed; MD5 product secret delivery PENDING. Driver internally bare go test: invoke finite run under tools/heavy.sh. |
| P11-host | Current test/topology/ipsec/README.md native production-agent isolated proof: build-native-plugin.py, isolated-vpp.py, TestIKEv2NativePackets ./internal/desired with NGFW_NATIVE_AGENT_BIN, NGFW_NATIVE_INITIATOR=1 and NGFW_NATIVE_PEER_LOSS=1 | Requires disposable patched0002 plugin and extracted stock peer; no shared plugin replacement/restart. This current executable supersedes old policy-based run.sh (still exits2). Verify native initiator/responder, real production sealed-secret agent path, ESP/no plaintext, exact1MiB/TCP, rekey, peer loss, actual agent restart, withdrawal/rollback; API/UI remains separate. |
| TEST-traffic-A | Offline helpers exist; run.py run explicitly refuses | Genuine source gaps: seven composed stage executors, candidate/commit/rollback lifecycle, shared lock/slot ownership lease renewal, rig binding, live capture provenance and causal config/counter-to-packet linkage. Source pass cannot close. |
| TEST-traffic-B | test/topology/traffic-b directory absent; constituent WG/IPsec/tunnels/FRR/Kea drivers exist | Orchestrator source absent; update historical prompt to current route-based DEC. DS-Lite unsafe deletion remains banned, unsupported secret/runtime paths remain explicit. |
| TEST-traffic-C | test/topology/traffic-c directory absent; constituents MPLS/SRv6/VRRP/QoS/IPFIX/IGMP exist | Orchestrator source absent; composed packet captures/drop/ARP/failover counters and API transaction rollback required; dedicated product capture only. |
| INTEGRATE-E2E | prompts/INTEGRATOR-PROMPT.md plus tools/ci.sh full and tri suite | TEST A/B/C dependencies not complete; complete installed product/API/browser campaign and permitted chaos prerequisite. Two connected hosts do not establish tri-VM or full stack integration. |

## Recovery

No source gaps were implemented and no task was marked done. This is source audit/evidence only. Root manager owns hosts, board, publication and integrations. Publication is blocked by root-observed GitHub403; no publication claimed. Next command: manager selects a fully provisioned host and executes the focused supported test with JSON/raw evidence, recording SHA/slot/windows/cleanup and all SKIP cases, then resolves missing composed drivers separately.
