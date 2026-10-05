# TEST-traffic-B — REST traffic acceptance

Status: implementation in progress; independent R1 BLOCK remains open. Every primary phase must configure through the slot REST candidate/commit API and reach the real VPP-backed agent before actual packet assertions. Earlier gRPC-only native/WG/BGP/OSPF proof is preserved but does not close this requirement.

D-237 records test-only REST adapter, private fixture and baseline warning choices. Current runtime acceptance for the REST adapters is not yet established. The final table and actual command outputs will be recorded here after the composed REST run and complete unchanged quick, followed by independent applicable review. No missing orchestrator code is lab-deferred.

Prior evidence and certificate FLAKY remain in TEST-traffic-B-evidence and docs/tech-debt.md. Original gRPC primary source727775ae7 and narrowed WGd3 proof are historical only.

## Actual REST follow-up, 2026-10-05

The sequential WireGuard/BGP/OSPF command completed exit0 at18:23:03UTC; all three
actual drivers passed. [Full outputs and scoped evidence](TEST-traffic-B-rest-evidence/three-phase-summary.json)
are committed. This is a three-phase subset, not all-primary acceptance.
The run records source34e545fc6 and subsequently committed OSPF default hunk0c95;
the final all-seven campaign must repeat on one frozen integrated tree.

| Phase | Actual result | Evidence |
|---|---|---|
| WireGuard REST | PASS: applied commit, established API peer, positive kernel handshake, inner ping, captured UDP, tagged agent restart, REST rollback, shared invariance | [Output](TEST-traffic-B-rest-evidence/wireguard-full-output.txt) |
| BGP REST | PASS132.44s:200 learned FIB routes, ICMP/TCP, policy and link changes, restart/recovery, REST rollback, shared invariance | [Output](TEST-traffic-B-rest-evidence/bgp-full-output.txt) |
| OSPF REST | PASS85.97s:100 learned FIB routes, ICMP/TCP, route loss/recovery, restart/recovery, REST rollback, shared invariance | [Output](TEST-traffic-B-rest-evidence/ospf-full-output.txt) |
| Native PSK REST | FAIL: strict changed-path warning at /vpn/ipsec | [Prerequisite](TEST-traffic-B-questions.md) |
| Native certificate REST | Pending native prerequisite; no REST pass claimed | [Prerequisite](TEST-traffic-B-questions.md) |
| GRE/VXLAN/DHCP REST | Earlier independent combined-source PASS; final integrated seven-phase rerun pending | Prior validation records |

WireGuard uses the envelope-approved tagged test-secret resolver and signed
private licence, preserving real REST configuration/licensing guards. Production
secret delivery remains pending; actual HTTP403/422 failures are retained.
Agent event timestamp was observed null, while the actual kernel handshake stamp
is positive. API-default fixture input materialization preserves exact Retrieve
comparisons. Host-independent safety/recovery suite15/15 passed, including actual
SIGTERM, surviving-descendant cleanup, ownership/namespace refusals, partial DB
create cleanup and rollback-failure rig teardown. Current-source full unchanged
quick, fresh-main integration and independent review closures remain pending.
Task status remains running; no Done claim.
