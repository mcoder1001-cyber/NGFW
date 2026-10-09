# pppoe-lifecycle-20261008 — R7 docs, evidence and scope review

Reviewed committed source: `48a8502a66ae414c14ed1abfb89f6ff8f41a14d4`; tree `8c564118c0716ecb91ab82bf6a8f2c0dc5b8baf5`. Base: `4d4723f`. No source changes or commits made by R7. Source/checkpoint inspection only; no product, native acceptance or test-execution pass is claimed.

**BLOCKER — task evidence/report is incomplete.** `docs/status/tasks/pppoe-lifecycle-20261008-wip.md:18-23` reports source/syntax/check passes and a fixture failure without pasted commands/output or durable evidence links; no task report exists. Add source-safe actual output with current source association. Preserve the observed fixture failure and unavailable Go/gofmt evidence; do not relabel them as deferred acceptance or successful lifecycle execution.

**MAJOR — recovery record omits exact current source and the latest repair.** `docs/status/tasks/pppoe-lifecycle-20261008-wip.md:4,10-16,25-28` uses a git-lookup placeholder and predates the durable incomplete-transition retry change. Record the current source SHA/tree above, enumerate that repair and its pending R1 recheck, and state the exact next command. Publication rejection is correctly recorded; the next publication step must explicitly wait for owner permission for the exact patch and must not route through another transport to bypass rejection.

[other: R1] The older R1 report's identical-retry convergence blocker must be rechecked on the current commit; R7 makes no correctness verdict on that repair.

Discovery/LAN encapsulation and real lifecycle acceptance are explicitly unfinished. No native or complete PPPoE readiness is claimed. The repair remains within lifecycle/transition scope.

## Reviewer verification

Read shared context, contribution/decision policies, REVIEW-PROMPT and R7 rules; inspected envelope, WIP/report, changed paths and available independent review reports. `git rev-parse HEAD HEAD^{tree}` produced the source/tree recorded above. No board file changed, so board validation was not required for this diff. No contract reshape, framework replacement or newly changed trust boundary was found in this documentation/scope review.

## Final evidence correction recheck

Frozen final product source: `d08aae29b535671e37e5caeae6d1abc590d80101`, tree `e17623c58d191d9689f18111e4c34ae19fa5a58a`. Evidence/report checkpoint inspected: `0b3ae76f864aa526f31602b6daede6dc98266fa4`, tree `020ba56ec5010f6387a382a8f9c637df00360e2e`. These are local unpublished checkpoints; no remote source SHA or publication success is claimed.

Task WIP/report now records exact frozen source, the additional retry/removal/late-hook scope, reproducible owned control script, actual source-check output and explicit publication authorization boundary. The initial R7 BLOCKER/MAJOR are resolved. The final report keeps Go/race/golden, complete quick and real lifecycle acceptance unexecuted. Native lifecycle failure is retained as a failed result, not a passing skip or deferred success.

Independent R7 command/output recheck from this worktree:

```text
$ python3 docs/status/tasks/pppoe-lifecycle-20261008-check.py
rendered Python AST: PASS
retired and blocked admission: PASS
immutable parent pidfd despite numeric identity reuse: PASS
Full Go/race/golden/lifecycle execution: NOTRUN; hosted gate still required
[exit 0]

$ python3 docs/status/tasks/pppoe-lifecycle-20261008-check.py --process-identity
rendered Python AST: PASS
private live-parent identity fixture: FAIL: ipv6-parent-unavailable
[exit 1]
```

The evidence matches the final task report. The passing controls establish only the explicitly bounded checks; the process-identity failure remains unresolved execution evidence for the developer/R1 to assess. R7 does not approve correctness or waive that failure. Original R1 and all applicable source rechecks must cite the final frozen product source before any merge decision.

Verdict: **APPROVE** (R7 documentation, evidence honesty and scope only). This result grants no publication, full-gate, native-acceptance or merge approval.
