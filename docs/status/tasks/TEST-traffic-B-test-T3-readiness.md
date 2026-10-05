# Independent T3: readiness-fixed full composed REST packet acceptance

Verdict: **PASS**, all seven primary REST packet phases on exact product/test source `43cfd1c9217163745246fb8816399756e70ad71b`. Tested frozen own HEAD `ccfde07d2c1c50cfaedbd83b63ceea6fe89de9a8`; `git diff 43cfd1c HEAD -- apps packages deploy tools test` was empty before and after execution. Independent reviewer authored no product changes. This closes the fresh T3 gate; full T1, hosted/main gates and canonical final documentation remain the manager's separate acceptance requirements.

Own branch/worktree: `codex/traffic-final-r4-t3-20261005`, `/root/ngfw-wt/traffic-final-r4-t3-20261005`. Explicit manager release followed its successful same-product all-seven run and cleanup. Physical slot28: HTTP12800, WEB16800, private Valkey12880 DB0. Fixed logical native/routing names live inside independently observed private network/mount namespaces. Source remained frozen throughout.

Actual unchanged command, session18850, exit0; summary window **2026-10-05T20:45:43Z–20:54:38Z**:

    NGFW_INTEGRATION=1 GOMAXPROCS=2 GOFLAGS=-p=2 python3 test/topology/traffic-b/run.py --slot 28 --output /root/ngfw-wt/traffic-final-r4-t3-20261005/.scratch/t3-all7-43c-readiness-independent

Private log: `/root/ngfw-wt/logs/traffic-independent-t3-all7-43c.log`. Output:

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

| Scenario | Required observation | Independently observed | Result |
|---|---|---|---|
| Native PSK responder and initiator | Separate strict applied REST proofs; ICMP/TCP/ESP, rekey, rollback | Both roles revision2, distinct proof SHA256s and actual candidate owner/hash; packet tests59.41s/44.89s, ESP present/plaintext IPIP absent, empty production-owned rollback | PASS |
| Native certificate IPsec | Production agent and stock RFC7427 peer; REST, packets, rollback |45.89s PASS, revision2/candidate owner/hash/applied state and baseline rollback; private VPP stopped | PASS |
| WireGuard | Applied REST candidate, kernel handshake, ICMP/UDP, recovery/rollback | Candidate owner/hash/revision, positive kernel handshake1791233339, inner ping/scoped UDP capture, owned restart/empty state/rig teardown | PASS, authorized tagged fixture |
| GRE/VXLAN | REST persistence and ownership; kernel ICMP/encapsulation, rollback | Both revision2 and persisted hashes, real ICMP and GRE/VXLAN captures, empty state/namespace cleanup | PASS |
| BGP | Exact REST/Retrieve, learned FIB, ICMP/TCP, recovery/rollback |117.965s PASS, learned-prefix FIB/peer route/packet evidence, idempotence, restart and applied baseline rollback | PASS |
| OSPF | Exact REST/Retrieve, learned FIB, ICMP/TCP, recovery/rollback |85.815s PASS, withdrawal/reannounce and FIB/peer/packet evidence, applied baseline rollback | PASS |
| DHCP relay | Two strict commits, real lease/DISCOVER giaddr, agent restart, single applied rollback |30.862s PASS; owner1, revisions1/2 and exact hashes; DISCOVER giaddr10.28.1.1; real API-agent Retrieve ready +1.50s within original30s budget; ONE rollback POST applied revision3/baseline hash | PASS |

Actual DHCP excerpt:

    read-only product API agent Retrieve applied relay at +1.50s (same 30 s recovery budget; no config mutation)
    --- PASS: TestKeaDhcpRelay (30.84s)
        --- PASS: TestKeaDhcpRelay/restart-safety (4.71s)
        --- PASS: TestKeaDhcpRelay/rollback-and-validation (0.85s)
        --- PASS: TestKeaDhcpRelay/cleanup (0.91s)
    SHARED_VPP_UNCHANGED

Retained rollback proof is applied revision3, parent2, hash `54c12e2e583cd387094b1835ee53e719e80c8371443e84d4800d6bb2893840a2`, matching first baseline candidate. Afterwards `/state/dhcp/relays` returned empty items. No POST retry, timeout increase, warning/assertion weakening or live source mutation was used. The readiness fix uses authenticated real agent Retrieve, not a cached process/listening probe. Existing strict numeric ownership, candidate digest/readback, partial/unsupported failure guards and exact D237 inactive schema-default warning inventory remain unchanged.

Curated independent text/JSON evidence: [TEST-traffic-B-independent-T3-readiness-evidence](TEST-traffic-B-independent-T3-readiness-evidence/composed-summary.json), 36 files/151556 bytes. Includes both native role proofs, certificate/BGP/OSPF/WG REST metadata, GRE/VXLAN states and scoped captures/pings, learned FIB/peer routes/TCP captures, DHCP two commits/rollback/DISCOVER, actual output excerpts and artifact provenance. GRE/VXLAN driver asserts owner continuity but retains digest/revision rather than separately recording the numeric owner. Raw pcaps, credentials, private keys and daemon logs remain ignored scratch. Gitleaks scanned151.56KB in176ms: **no leaks found**.

Artifacts: own production and authorized tagged agents previously compiled under capped heavy runner; tag independently inspected. API/package/native-plugin artifacts copied read-only from equal source with1553 file hashes verified and actual schema/proto/yang runtime imports passing. Relevant production inputs are byte-identical to43c. No fresh API/plugin compilation claim. Fresh DHCP helper executed from this43c test source. No duplicate full CI.

Summary `primary_passed:true`, seven phases passed, `whole_wave_passed:false`. Eight optional smoke phases remain **NOT RUN**, individual reasons preserved in the summary: det44/CNAT private globals acceptance, DS-Lite pool deletion banned D211, IS-IS private OSI window, BFD dedicated export, unbound DNS export, chrony source export, syslog receiver export. The [owner route-based decision](../../decisions/DEC-ipsec-route-based.md) supersedes obsolete kernel-vpp phase1a/P11 build requirements: native VPP IKEv2 route-based IPsec is current primary scope; strongSwan is the interoperability peer. NGFW-side kernel-vpp/socket-vpp/strongSwan and old P11-pkg scope are excluded and not tested, without implying a missing required primary variant. Historical P11 board rows remain unchanged. Tagged WG fixture does not establish production secret delivery.

After actual exit0, independently verified no12800/12880/16800 listeners, no w28 namespaces, no ngfw_w28 databases or roles, no owned API/agent/Valkey/VPP/wrapper processes beyond inspection shell/rg. Exclusive nonblocking `/run/lock/ngfw-globals.lock` acquisition succeeded and released. Shared VPP stayed **MainPID1014/NRestarts0**; every wrapper reported unchanged identity. Explicit slot/global release was sent to manager before report curation. All owned commands finished. No system service restart, shared VPP write/trace, host package install or management-interface mutation.

Historical evidence is preserved: two unchanged old8e root full runs reproduced DHCP post-restart rollback503, a real reproducible BLOCK, never FLAKY. [Old independent8e source-specific PASS](TEST-traffic-B-test-T3-final.md) did not waive those failures. Fresh developer43c readiness correction is now independently positive in this whole run; root's same-product whole run also passed (DHCP29.154s, restart4.28s, single rollback0.81s). Earlier3c collision,3e weak guard,eb numeric failure,8f cancelled143 and root parallel PSK ENV-EAGAIN remain historical nonacceptance observations. No debt waiver replaces this correction or mandatory new-source gates.
