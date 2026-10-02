# Agent timing prepared composition — held for cleanup merge

Own branch `task/ci-agent-timing-prepared-20261002`; isolated
`NGFW-ci-agent-timing-prepared`. Actual main base checked
`0098d93f114d0f6eed56fdc1ef82d2cdf753f793`. CleanupPR86 is still pending; this
checkpoint must be independently composed/reviewed against actual main only
AFTER cleanup merges. Manager explicitly requests no publication or final PR now.

Owned source only `apps/agent/Makefile`: exact independently approvedb10/50b
addition of eight constant UTC BEGIN/END markers around existing vet/lint dispatch/
race-test/build recipes. Removing those eight lines recovers actual0098 original
Makefile byte-for-byte. Flags/order/failure propagation/targets/fallback unchanged.
No product, test/assertion, broad gate, concurrency, cache, dependency, timeout,
board, current PR86 or main changes. Source review/prototype docs retained verbatim.
Optional corrected4c8 read-only latency diagnosis and independent review retained
verbatim as measured historical baseline; not a new current-run measurement.

This is timing instrumentation, not a CI optimization/speedup or test acceptance.
Markers are captured normally in successful08-agent artifact; END indicates
ordinary return, not gate approval. Prior independent harmless fake-command
failure controls are attributed to their preserved review, not rerun here.
After cleanup, manager preserves actual-main changes, independently reviews final
composition, schedules unchanged complete hosted quick at exact published head
and reads marker evidence. No hosted timings/causal attribution/time savings are
claimed until actually observed. No heavy local Go or new automation.
