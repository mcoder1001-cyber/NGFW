# Independent P10 broad subsystem validation recheck

Exact head `88cc40ade655f0a08801f1822dd1401ffc382263`, isolated task/P10-punt-final-recheck. This preserves earlier00cb review and its initially unclassified broad failure; no historical failure converted to PASS.

Personally reran00cb full subsystem race count1 with JSON captured to a file rather than truncated terminal output. Exactly two failed tests: TestLinuxNetdevKindOnThisHost (`netdev_test.go:43`: `netlink RTM_GETLINK: operation not permitted`) and TestReachabilityTable (`reachability_test.go:166`: basepolicy missing TD-11a row). The second was a REAL repository failure, not lab deferral.

Inspected88cc delta: adds basepolicy wired/P10 reachability entry, matching actual provider registration already reviewed; no assertion or guard weakened. Reran exact88 frozen head:

```
go test -json -race -count=1 ./internal/subsystems > /tmp/p10-final-subsystems-88-review.jsonl
exit1
```

Parsed EVERY fail JSON record: exactly TestLinuxNetdevKindOnThisHost, first error `netdev_test.go:43: lo: kind "" exists false err netlink RTM_GETLINK: operation not permitted`; package fail follows. TestReachabilityTable now PASS. Remaining local subsystem result is therefore FAIL due to this observed restricted-environment netlink error; it is not a full local PASS. No other failed test records occurred.

Previous exact00cb focused basepolicy race5, BasePolicy subsystem race5 and Python base policy3 passed as recorded in separate final-layout report.88 code delta is reachability metadata only, plus documentation. No live nft/systemd/VPP acceptance executed. Full unchanged hosted quick gate must still pass on final integration tree before merge. Independent source verdict for corrected metadata/provider/layout scope: APPROVE; this does not waive CAP_CHOWN/global /etc decisions or claim whole P10 acceptance.
