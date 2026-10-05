# Independent R4 F-hardening-lite

Exact SHA 726d92141402459505ee27fdc6fafc7aeef94295; 2026-10-05; branch codex/review-r4-seven-20261005. Own reports only; reviewer-owned git archive snapshot; no product edits or shared host/VPP mutation. Task envelope: independent R4/T3, filesystem fixtures only; no real kills or shared services/config changes.

Actual reviewer command: `tools/heavy.sh .review-fixtures/hardening/deploy/hardening/tests/run.sh`
```
Ran 10 tests in 1.560s
OK
```

Reviewed deploy/hardening/stage.py canonical non-/ root refusal, whole mutation path preflight, explicit management rp_filter (no all/default/lo), opt-in profiles and no runtime service activation. Refusal exercised against / and symlink fixture, with no writes. Check/signing tools do not mutate shared VPP. No R4 blocker found. Appliance daemon compatibility/runtime activation remains explicitly deferred.

Findings: none within R4 scope. Verdict: APPROVE.
