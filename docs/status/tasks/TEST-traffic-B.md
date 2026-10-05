# TEST-traffic-B — REST traffic acceptance

Status: implementation in progress; independent R1 BLOCK remains open. Every primary phase must configure through the slot REST candidate/commit API and reach the real VPP-backed agent before actual packet assertions. Earlier gRPC-only native/WG/BGP/OSPF proof is preserved but does not close this requirement.

D-237 records test-only REST adapter, private fixture and baseline warning choices. Current runtime acceptance for the REST adapters is not yet established. The final table and actual command outputs will be recorded here after the composed REST run and complete unchanged quick, followed by independent applicable review. No missing orchestrator code is lab-deferred.

Prior evidence and certificate FLAKY remain in TEST-traffic-B-evidence and docs/tech-debt.md. Original gRPC primary source727775ae7 and narrowed WGd3 proof are historical only.
