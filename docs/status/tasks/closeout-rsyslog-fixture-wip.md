# Periodic test impstats after restart

Branch codex/closeout-rsyslog-fixture; base9619ff99. Own rsyslog_test.go and this evidence. No production changes.
Full unchanged WAN quick failed TestHostImpstats: fake single batch had no qualifying fresh action after restart. Fake pre-stamped now+1100ms before returning; scheduling delay beyond this stamp makes the only batch permanently stale. Production intentionally verifies only post-restart records and remains unchanged.

Fixture now emits actual-time periodic batches after Restart returns, matching a running daemon. Snapshot actions/path per generation; stop and join the previous producer before restarting; stop/join cleanup before tempdir removal; finite10s limit and parent cancellation. Existing stale-action, rollback, restart-count, drift, host-scan and timestamp parsing assertions remain. HostImpstats simulates1500ms restart delay beyond the old batch margin.

Repeated initial focused test started before the final delay regression was added and is not final-source evidence. Exact current-source race/lint plus deterministic old-fixture reproduction and unchanged complete integration quick are required. No gate failure waived. Parent owns publication/review/integration.

Exact source0ae10c7b validation completed:30 focused race executions PASS91.290s including explicit1500ms delay regression; unchanged old9619 fixture with only1500ms forced descheduling overlay reproduces identical HostImpstats convergence FAIL6.503s. Full rsyslog race package PASS11.570s and pinned lint0issues. All rollback/stale record/host loading tests retained. Independent source review097120d6 approves conditional checks; exact validation now fulfilled. Full joined integration quick still required before merge.
