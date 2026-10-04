# NAT46 live closeout — 2026-10-04

Reviewed product/source checkpoint:3d001ee60de023ef32bc18df2c48db570f84ef0d on codex/closeout-nat46-driver (basef41491d06). Independent reviewer management_acceptance approved the historical fallback restoration, shared IPv6 key guard and private-only hard acceptance gates. Publication/integration remain root-owned; none claimed by this worker.

## Change and supported boundary

The legacy driver let tools/lab rig precreate unowned VPP ports, so the real agent refused its Create with InterfaceAlreadyExists(-79). The driver now quiesces both Linux peers/veths, uses generated binapi bindings to remove only the two exact untagged rig ports, preserves the Linux rig, lets the agent create owned ports, restores Linux routes/endpoints and runs V19preflight before packets. Configurable NGFW_NAT46_BIN_DIR selects current binaries; ctl uses an argv array. Driver and binapi helper require NGFW_DISPOSABLE_VPP1 plus the root-owned regular/no-symlink isolated-vpp.py startup marker and a different mount namespace from PID1 before mutation. A flag alone is refused.

The first usable packet attempt exposed source recovery loss: current nat46.go had discarded the reviewed50f2ceca return-path fallback although the board recorded that row merged. Restored configuration-only fallback: serverIPv6 embeds the serviceIPv4 in its low32bits, bits64..95zero, and uses a routed unique /64 MAP domain. Assemble returns the exact configured server; one embedded server per /64 is enforced. New family checks avoid interpreting IPv6 service/IPv4-mapped server shapes as NAT46. Desired projection rejects the exact parsed/masked IPv6 LPM key already held by ordinary nat.map, for MAP-E and MAP-T even when IPv4 prefixes differ, preventing overwrite/deletion of the shared VPP key.

Arbitrary/nonembedded IPv6 addresses retain the historical /128 forward-only shape. The pinned MAP LPM cannot match source prefixes longer than64 and reverse translation derives IPv4source from serverIPv6low32bits. General SIIT/EAM for arbitrary servers requires separate implementation/VPP-code-track work. No C, schema or unrelated product changes were made. This acceptance certifies the bounded embedded/routed fallback; it does not certify arbitrary-address bidirectional NAT46 or REST/browser acceptance.

## Fresh validation

- Race tests nat46PASS1.115s, mapnatPASS1.144s, desiredPASS35.963s; includes masked cross-family MAP-E/T IPv6 collision and noncollision controls.
- bash -n passed; unisolated driver flag0/flag1 bothEXIT64 before mutation. tools/ci.sh check --basef41491d06 PASS11s. Complete quick/full gate belongs to root final-integration campaign, not claimed here.
- Own corrected ngfw-agent rebuilt from reviewed source, SHA256614073dc6a4c792167e2d456cecf72a35f55da5b4563187101f9a5de7e6f5ec5. Current root-built ctl/preflight reused.

Exact finite run: load canonical tools/lab env8, set NGFW_NAT46_BIN_DIR=/tmp/g-w8/closeout-bin, execute tools/heavy.sh python3 test/topology/hardware-smoke/isolated-vpp.py docs/status/tasks/F-nat46-host-evidence/host.sh run docs/status/tasks/closeout-nat46-evidence.

Final guarded run18:48:49–18:49:13 Tehran time EXIT0; [raw transcript](closeout-nat46-evidence/review-final-live.txt):

| Case | Fresh outcome |
|---|---|
| Descriptor host test | Named TestNat46OnHost executed PASS, no SKIP. This alone is descriptor evidence. |
| Agent apply/idempotence | APPLY_STATUS_APPLIED with no errors; Retrieve==canonical; reapplyAPPLIED and empty plan. Routed IPv6server prefix installed through desired routing.static. |
| Packets | Strict3transmitted/3received ICMP. TCP exact ngfw-nat46-ok payload verified; WAN tcpdump records clientfd00:8:4646::a08:102↔serverfd00:8:46::a08:2e0a. MAP TX8/552bytes and RX7/342bytes. No throughput claim. |
| Actual agent restart | Own MAPdomain deleted with agent stopped; agent restarted and recreated it. Retrievecanonical within1.00s≤30s; exactTCPpayload works after restart. Shared VPP never restarted. |
| Invalid mapping | Duplicate serviceIPv4 DryRun refused with /nat/nat46/mappings/1/ipv4; ApplystatusFAILED; Retrieve unchanged. |
| Rollback/cleanup | Explicitnat:{} and routing.static:[] applied; Retrieve nat{}, domain0, both interfaceMAP-T features absent, ownedserverroute absent. Agent stopped; tools/lab rig down removed namespace/veth/ports. DisposableVPP PID3687457 stopped; sharedNRestarts0 unchanged. Slot8 released. |

## Preserved failures and recovery

Original root artifact artifacts/test-closeout/nat46.log preserves InterfaceAlreadyExists. handoff-first-run.txt preserves successful handoff/config followed by missing Linuxroute after veth quiesce. routes-rerun.txt preserves successful forward translation but returnTCPtimeout using lost /128 projection. focused-tests.txt preserves an incorrect new test expectation that Assemble returned an error for ignored foreign shapes; corrected test follows the existing ignore/no-mapping contract and guards no panic. descriptor-rerun.txt and review-fixed-tests.txt contain actual green reruns. embedded-product-run.txt/strict-final-run.txt preserve subsequent bounded fallback passes before final review guards. No earlier failure was relabelled PASS.

All essential source, commands, raw results and functional boundaries are durable here. Remaining wider NAT46 row acceptance: API/browser screenshots, arbitrary-server support decision/implementation, full final-tree campaign and any independent-review follow-up. Manager reconciles the board with these actual scoped outcomes.
