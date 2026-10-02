# Dashboard recovery — independent integration review

Reviewed head: `9baa4512e02530bd4b487ac789e4ae32ba3bbaec` in NGFW-dashboard, based on main `471c61fa`. Reviewer is not the recovered feature author. Source/recovery inspection only; focused test runner and hosted full quick gate are separate evidence.

## Verdict

**APPROVE code/recovery scope.** No new BLOCKER/MAJOR found. Hosted full quick gate must pass before merge. Real VPP/FRR/appliance/browser acceptance is NOT RUN and explicitly deferred by the product owner; historical reports' lab-only BLOCK does not prohibit this authorized merge, and does not become PASS.

## Recovery evidence and applicable aspects

Compared recovered head with `origin/codex/dashboard-host-20261001`. All dashboard product paths are byte-identical: promexport collector/source/listener/test files, desired Prometheus projection/tests, subsystem descriptor/wiring/tests, agent projection and seam tests. Other code changes are already-main CI infrastructure and capture test cleanup; remaining changes are recovery/status/docs/board. `git diff --exit-code` on dashboard product paths returned zero. Original independent R1–R8 reports survive in the branch, including the resolved terminal source-stop race and failed-bind retrieval, D164 decision ledger and corrected tech debt.

- R1 correctness: inspected bind-first endpoint replacement, same-address handler update, context checks, clone-only Retrieve and successful-start-only applied value; earlier cancellation fix survives unchanged.
- R2 security: allowlist is direct socket peer CIDR, no forwarded-header trust; collector is read-only; no new privileged operation or secret flow. Empty allowlist semantics pre-exist in the management contract and are not silently changed.
- R3 contracts: existing management.prometheus contract only; no new schema/proto/REST or hand-edited generated paths.
- R4 agent: management projection and assembly are symmetric; listener has no persisted fake ownership, reconstructed by reconciliation; closers execute source terminal Stop before HTTP listener close. Pending live host lifecycle proof remains explicit.
- R5 performance: stats connection reused and serialized, no binapi per interface; top node errors bounded; output reflects real available aggregate counters instead of invented directional/link stats. Synchronous govpp reads cannot be interrupted mid-call; this documented limitation survives and is not falsely claimed bounded.
- R6 UI: recovered host slice changes no UI; historical UI/browser acceptance remains NOT RUN.
- R7 decisions/docs: D164 and corrected debt survive; no silent pending privilege/handover decision. Recovery reports distinguish code from acceptance.
- R8 operations: scrape failure returns error, bad bind preserves old listener/value, shutdown forces close after grace deadline; full hosted quick and real restart/traffic/alarm proof still required/deferred respectively.

No product edits or tests were performed by this reviewer; do not attribute parallel developer/tester output to this report. Manager should link exact hosted gate run and centralized deferred campaign in PR before final merge.
