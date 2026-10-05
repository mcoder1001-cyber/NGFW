# Historical pre-readiness task report

Superseded by the canonical task report; failures and incomplete gates below are source-specific history.

# TEST-traffic-B — REST traffic acceptance

Status: blocked awaiting fresh DHCP guard repair; independent R1 BLOCK remains open. Every primary phase must configure through the slot REST candidate/commit API and reach the real VPP-backed agent before actual packet assertions. Earlier gRPC-only native/WG/BGP/OSPF proof is preserved but does not close this requirement.

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

## Frozen3e source-specific result and repair handoff

The unchanged composed command `NGFW_INTEGRATION=1 GOMAXPROCS=2 GOFLAGS=-p=2 python3 test/topology/traffic-b/run.py --slot 27 --output .scratch/traffic-b-rest-all-fixed` completed exit0, 18:54:27–19:05:23UTC on local3e94010253e78fc68fb742e3b98132cfb5a870ad (published5339c006b11fada1df71a4bb62941810e5de2fd7, tree73aa210d374e421edfeda822d4a7af33d50a07af). All seven drivers returned zero: PSK responder and initiator, certificate, WireGuard, GRE/VXLAN, BGP, OSPF and DHCP relay. [Aggregate](TEST-traffic-B-rest-evidence/attempt-3e940-summary.json) and [public metadata](TEST-traffic-B-rest-evidence/attempt-3e940-public-result.txt) preserve exact source and hashes. Optional eight smokes remain explicitly not run.

This is not completed acceptance. Independent R1 found that the reused DHCP commit helper accepts HTTP200/status-applied without enforcing empty notApplied and absence of changed-path agent.unsupported-field warnings. A status-applied response can contain those failures. A fresh repair agent must add strict guards and meaningful negative tests, pin a new coherent source, repeat all seven and complete unchanged quick plus independent reviews. Under the A3 arbitration the current author hands off rather than beginning another correction round. Independent quick on3e was still pending at handoff; no PASS is claimed.

All owned commands ended. Slot27 namespaces, listeners12700/12780/16700, stack roles/databases and runtime processes were absent after cleanup; shared VPP remained MainPID1014/NRestarts0. Earlier3c failed aggregate and certificate FLAKY remain preserved. Production WireGuard secret transport remains pending; this envelope uses the authorized tagged fixture.
