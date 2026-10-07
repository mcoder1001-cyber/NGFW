# Independent T3 F-hardening-lite

Exact SHA 726d92141402459505ee27fdc6fafc7aeef94295; 2026-10-05; branch codex/review-r4-seven-20261005. Own reports only; reviewer-owned git archive snapshot; no product edits or shared host/VPP mutation. Task envelope: independent R4/T3, filesystem fixtures only; no real kills or shared services/config changes.

Actual reviewer command: `tools/heavy.sh .review-fixtures/hardening/deploy/hardening/tests/run.sh`
```
Ran 10 tests in 1.560s
OK
```

Slot none: filesystem-only/private loop fixture; no shared dataplane objects.

| Scenario | Expected | Observed | Result |
|---|---|---|---|
| Ten offline controls/signature/root-boundary cases | successful safe fixture run | output above, exit 0 | PASS |
| Actual appliance runtime/firmware | live acceptance | unavailable, not executed | DEFERRED |

Verdict: PASS for executed host-independent/disposable-disk scope; deferred appliance execution is not a runtime PASS.
