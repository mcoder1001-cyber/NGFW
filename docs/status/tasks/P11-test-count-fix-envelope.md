# P11 current VPP test-count consistency — bounded fixture correction

Branch `codex/p11-test-count-fix-20261003`; isolated `NGFW-p11-test-count-fix`; base `4909418bfc0f2a53700833982b16fc175d9892c6`.
Owned ONLY `deploy/strongswan/test_verify_inputs.py` and unique `P11-test-count-fix-{envelope,wip}.md` status files.

Root's actual integration failed a hardcoded 66-count assertion after the reviewed CPU-affinity fix added six real VPP tests. The real script runs all 72 successfully. Preserve fresh RED evidence, then validate rigorous current-script success-summary/numbered-ok-record consistency and absence of failed records, with positive count. Never weaken VPP scripts/gates, stub the positive static gate, hide/remove new cases, skip tests or use >=66 alone as success.

Run real current unchanged script/static verifier under actual clean HOME context and the complete 11 intake/23 stage suites after correction; disclose existing synthetic provenance stubs. Use PYTHONDONTWRITEBYTECODE=1 to prevent untracked caches. Tests only, no product/gate/builder/frozen-root-integration/original/P10/main edits. Commit coherent checkpoints immediately; root publishes via connector if shared Git/CLI credentials deny publication. Fresh independent R1 review follows; developer never self-reviews or merges.
