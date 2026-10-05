# Independent T3 final composed REST packet acceptance

Tested HEAD ffeaa3b283ae0a31c4d7b4377963f0a52444a801, exact product/test source8e1c41d59ef177e81dfec3ed095c317c20b36dd5. Own isolated branch/worktree codex/traffic-final-r4-t3-20261005, /root/ngfw-wt/traffic-final-r4-t3-20261005. Scoped git diff8e..testedHEAD for apps/packages/deploy/tools/test was empty before start. Later report/evidence commits change docs only. Physical slot28, HTTP12800/privateValkey12880 DB0; fixed logical native/routing fixtures operate inside independently observed private namespaces. Production and approved tagged agent compiled in this worktree; API/package/native-plugin artifacts copied read-only from identical source and1553 hashes verified, runtime schema/proto/yang imports PASS. No fresh API/plugin compilation claim. Artifact provenance committed with evidence.

Actual unchanged command, session45416:

    NGFW_INTEGRATION=1 GOMAXPROCS=2 GOFLAGS=-p=2 python3 test/topology/traffic-b/run.py --slot 28 --output /root/ngfw-wt/traffic-final-r4-t3-20261005/.scratch/t3-all7-8e-independent

Log /root/ngfw-wt/logs/traffic-independent-t3-all7-8e.log; actual window2026-10-05T20:04:06Z–20:15:47Z, exit0. Source frozen throughout. No assertion retry, timing change, warning suppression or live source edit.

    START ipsec
    END ipsec passed
    START ipsec-cert
    END ipsec-cert passed
    START wireguard
    END wireguard passed
    START tunnels
    END tunnels passed
    START bgp
    END bgp passed
    START ospf
    END ospf passed
    START dhcp-relay
    END dhcp-relay passed

| Scenario | Expected | Independently observed | Verdict |
|---|---|---|---|
| Native IPsec PSK responder and initiator | Separate applied REST candidate/revision, ICMP/TCP, ESP/no plaintext, rekey/recovery, rollback | Both roles revision2 and own proof hashes;86.95s/46.46s tests pass, scoped capture digests retained, stored profiles/routes/protection empty at rollback | PASS |
| Native certificate IPsec | Real production agent/stock RFC7427 peer, REST candidate, ICMP/TCP and cleanup | Production certificate test49.920s, applied revision2/hash/owner and baseline rollback proof, private VPP stopped | PASS |
| WireGuard | Real REST commit, kernel handshake, inner ping/UDP, restart, rollback | Applied candidate/hash/owner/revision, kernel positive timestamp1791230974, captured UDP and inner ping, owned restart recovery/empty state/rig cleanup | PASS (authorized tagged secret fixture) |
| GRE and VXLAN | REST candidate ownership/hash/persistence, kernel peer ICMP and encapsulation, rollback | Both revision2 with persisted candidate hashes, successful ICMP and scoped GRE/VXLAN captures, tunnel/namespace cleanup | PASS |
| BGP learned FIB | REST candidate/revision and exact Retrieve, learned-prefix ICMP/TCP, restart/rollback |122.921s PASS;100 learned routes recover5.3s after agent restart/pair loss, packet/FIB/TCP evidence and baseline rollback | PASS |
| OSPF learned FIB | REST candidate/revision/exact defaults, learned-prefix ICMP/TCP, restart/rollback |83.459s PASS; route withdrawal/reannounce, exact FIB/packet/TCP evidence and applied baseline rollback | PASS |
| DHCP relay | Two strict REST commits, actual lease/DISCOVER giaddr, restart, applied baseline rollback |29.420s PASS, owner1/revisions1,2 and hashes; captured DISCOVER giaddr10.28.1.1, lease/restart; rollback applied revision3 and restored baseline hash | PASS |

Actual retained excerpts:

    production owned rollback removed profiles, protection, routes, tunnels and interfaces; Retrieve empty before disposable VPP shutdown
    underlay capture: ESP present; plaintext IPIP absent before negotiation, during ICMP/TCP/rekey, and after SA deletion
    --- PASS: TestIKEv2NativePackets (86.95s)
    --- PASS: TestIKEv2NativePackets (46.46s)
    PRODUCTION_CERTIFICATE_PEER_PACKETS=PASS
    REST_WIREGUARD_PACKETS=PASS
    --- PASS: TestP12TopologyOnHost (122.78s)
    --- PASS: TestOSPFTopologyOnHost (83.35s)
    --- PASS: TestKeaDhcpRelay (29.39s)
    --- PASS: TestKeaDhcpRelay/restart-safety (3.63s)
    --- PASS: TestKeaDhcpRelay/rollback-and-validation (0.85s)
    --- PASS: TestKeaDhcpRelay/cleanup (0.84s)
    SHARED_VPP_UNCHANGED

Committed reviewed text/JSON under TEST-traffic-B-independent-T3-evidence/: composed-summary.json, both PSK REST proofs, certificate/BGP/OSPF/WG REST proofs, GRE/VXLAN state/capture/ping, BGP/OSPF FIB/peer/ping/TCP capture, DHCP strict marker/probe/capture, actual excerpts and artifact hashes. Controller proofs retain candidate owner/digest/revision and applied rollback; GRE/VXLAN driver asserts owner continuity and retains digest/revision rather than recording numeric owner separately. Raw pcaps/private keys/credentials/daemon logs remain ignored scratch, not committed. Separate proof SHA256s enforce both native roles. Dispatcher requires DHCP two commits plus rollback, strictly numerical safe owner and exact candidate hashes. Only unchanged inactive schema-default baseline warning inventory is accepted under D237; changed forwarding warnings/new warning membership refuse.

Actual summary: primary_passed:true; all seven driver phases passed; whole_wave_passed:false. Optional eight smoke items explicitly NOT RUN with individual reasons retained in composed-summary.json: det44/CNAT need private globals acceptance, DS-Lite pool deletes bannedD211, IS-IS private OSI window, BFD dedicated export, unbound DNS export, chrony source export, syslog receiver export. The owner-approved [route-based IPsec decision](../../decisions/DEC-ipsec-route-based.md) supersedes the obsolete kernel-vpp phase1a/P11 build requirements: native VPP IKEv2 route-based IPsec is the current primary scope, and strongSwan serves only as its interoperability peer. NGFW-side strongSwan/kernel-vpp/socket-vpp and the old P11-pkg build are scope-excluded and not tested; no missing required primary variant or pending primary acceptance is implied. Historical P11-pkg/P11-host board rows remain historical and are not changed by this report. Authorized tagged WG fixture is not production secret-delivery proof. No deferred item represented as PASS.

Cleanup independently checked after session exit0: ss found no12800/12880/16800 listener; ip netns found no w28 namespace; pg-test list no ngfw_w28 DB and pg_roles query no ngfw_w28% role; process list no owned agent/API/Valkey/VPP/wrapper beyond inspection shell/rg. Nonblocking exclusive globals lock acquisition succeeded and released. Shared systemctl observation MainPID1014/NRestarts0. Manager explicitly notified slot/global release before its next campaign. All per-phase wrappers reported shared identity unchanged. No system service started/restarted, shared VPP written, packet trace, host package installed or management interface altered.

Actual leak scan:

    gitleaks dir --config .github/gitleaks.toml --redact docs/status/tasks/TEST-traffic-B-independent-T3-evidence
    scanned ~152977 bytes (152.98 KB) in135ms
    no leaks found

Historical root8e first composed run failed DHCP immediate post-restart rollback503 (line245), and the final unchanged sequential root full run reproduced the SAME rollback503 (line243). This is **REPRODUCIBLE and BLOCKS task acceptance**, not FLAKY. Intermediate root DHCP29.080s and this independent full DHCP29.420s passed, but those positive source-specific observations do not waive two observed failures. Preserve both actual aggregate FAIL diagnostics/hashes and this independent PASS. No assertion/reconnect timing or source was changed during these runs. Root concurrent second PSK ENV-EAGAIN/global-window refusal was not source PASS. Earlier3c collision/3e weak-guard/eb numeric FAIL and8f cancellation remain historical evidence. Mandatory fresh corrected-source all7, independent T3 and full quick/hosted/main gates are required after reassignment.

Verdict: **PASS for this independently executed source-specific run only**, unchanged all-seven primary REST packet T3 on product8e. **Overall task acceptance BLOCKED by twice-reproduced external DHCP rollback503**. Optional smoke remains NOT RUN. All owned commands/fixtures finished. No flake debt waiver applies.

The earlier report's provisional once-only FLAKY classification and queued debt link are superseded by the second actual reproduction. No DHCP reconnect-flake debt row can substitute for fixing and independently verifying this real failure. Fresh fixture developer assignment will investigate an actual read-only API-to-agent readiness predicate within the existing recovery budget before the unchanged single rollback POST; no rollback retry, warning/assertion weakening or timeout increase is authorized by this report.
