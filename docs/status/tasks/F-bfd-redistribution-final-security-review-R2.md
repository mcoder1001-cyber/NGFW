# BFD final independent R2 security review

Exact local source: `2a5abf9e527ebdecf67c82c0f45ba54260bfcc9f` (published source `a0dd57d1fd183d43a7dd26850f76bc7e9780bfc1`, manager-provided). Reviewer owns reports only on `codex/review-r2-final-security-20261005`, private RAM worktree `/dev/shm/r2-final-security`. No product edits.

Grade provenance: whole-feature multihop/compensation R2 APPROVE on `0912488609545fec12a329f6f801d00a7cd77231`, published review `1c71a8959640d5ace6d42acf5bbca8e6ffe34f6d`; independently approved arithmetic closure on `8f3d2f0789daa63a48d8254e6ee5df3dadea521f`, review `5a8c01bb49013566db1746911330ad95fa7e1dce`. Exact git comparisons show descriptor subtree unchanged 091→8f3, and desired projection, FRR BFD/redistribution renderers and BFD API unchanged 8f3→2a5. Other-main FRR secret-generation files match pinned main `492c0156` byte-for-byte and are excluded from this feature grade. Current changed descriptor/lifecycle, generic durable-store mutations, endpoint index, event history and observation UI were independently read in full.

Endpoint Lookup and Claim capture current complete VPP identity, lock the durable record map, rebuild the index once and preserve that lock through boot filtering and ambiguity/conflict refusal. Direct store mutations update buckets under the same lock. Malformed suffixes remain indexed and fail closed; foreign-boot records cannot confer ownership. Batch journal append precedes map/index mutation; nonbatch snapshot flush precedes map/index mutation. Failed writes therefore do not create successful claims. Prune/replacement rebuilds the optional index. No change weakens exact interface/index or successful-add recovery proof: EEXIST remains nonadoption and compensating deletion remains restricted to recorded successful adds.

Observation callbacks are installed for the specific owner and invoked after owned successful Create/Delete or owned Retrieve. Only live owner/key membership accepts events; deletion, snapshot pruning and disconnect remove stale history. Subscription loss invalidates history. Deleted/recreated keys start with unknown flap time; another owner's identical key remains separate. Maps are bounded by currently owned sessions, rather than historical churn. UI adds translated states and empty observations guarded against fetch/agent failures; it grants no new mutation permission. Existing sealed-key resolution, wire bounds, redaction and owner/admin boundaries retain cited reviewed bytes.

Independent affected race command:

```
env TMPDIR=/dev/shm/r2f-t GOTMPDIR=/dev/shm/r2f-t GOCACHE=/dev/shm/bf-cache GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C apps/agent test -race ./internal/subsystems ./internal/descriptors/bfd -run 'TestBfd|TestBFD|TestOwnedSession|Test.*Claim' -count=1
```

Actual result: PASS, subsystems 13.059s; descriptors/bfd 1.089s; exit 0. This includes 1024-tuple restart/index/ambiguity/foreign-boot/prune cases, 2048 historical churn/late-event/foreign-owner negatives, successful owned descriptor lifecycle and existing durable journal/claim recovery regressions.

No shared service, database or VPP change, live native multihop packet test, full quick gate or appliance execution was performed. Complete exact integration quick and remaining aspects stay manager prerequisites. No BLOCKER, MAJOR or MINOR security findings in reviewed feature scope.

Verdict: **APPROVE**, whole-feature R2 grade by explicit unchanged-path provenance plus independent affected-security review.
