# Prepared cleanup-strength recovery WIP — targeted test ERROR

Own branch `task/traffic-cleanup-prepared-20261002`; isolated
`NGFW-traffic-cleanup-prepared`; base
`0392eaaef2b623af907399c4a17f44b7bfd53cf9`. Remote checkpoint awaiting manager.
Imported exacta526 assertion helpers + own approved docs/diagnosis/report only;
product source unchanged. Prior original16/33/40/47 workflows and policies,
arbitration, shared WIP and all current production paths byte-identical to base.
No new arblog or source activation. Foundation497 reported landed; other phases
and actual-current-main final integration remain pending.

Personally executed affected timeout-descendant/noisy-byte-cap cases five times
each with `os.kill` patched to raise on any numeric PID signalling. Real result:

```
Ran 10 tests in 11.222s
FAILED (errors=1)
targeted cleanup controls: tests=10 failures=0 errors=1 skipped=0 expectedFailures=0 unexpectedSuccesses=0
ERROR test_timeout_stops_descendant_that_ignores_term
  test_foundation.py:163 self.assert_verified_child_stopped(observed[0])
  test_foundation.py:137 Path('/proc/<verified-pid>/stat').read_text()
ProcessLookupError: [Errno 3] No such process
```

The helper positively verified same mounted PID/birth/readiness before cleanup;
this error occurs during postcleanup read when process disappears. No surviving
product descendant is inferred. However the assertion handles FileNotFoundError
only, so actual ESRCH becomes a real test error and CI reliability gap. Do not
catch all OSError, waive permissions/other I/O errors, relax readiness or signal
a child-reported PID. Narrow FileNotFoundError/ProcessLookupError postcleanup
handling needs source-author correction and independent scope review.

Subsequent once-only whole-suite run:

```
python3 .github/scripts/traffic-transaction-fixtures.py
Ran 47 tests in 4.023s — OK
transaction source fixtures: tests=47 failures=0 errors=0 skipped=0 expectedFailures=0 unexpectedSuccesses=0
```

This47 PASS does NOT erase targeted10 ERROR or close this new finding. Earlier
independent7c cleanup review remains historical, not approval of an unmade fix.
Current prepared phase is not ready for final integration. Source helper is exact
approveda526; all product/gate/production prefixes and whitespace checks PASS.

Next: manager assign separate narrow helper correction/independent test closure,
then rerun affected repetitions (numeric signals prohibited) and strict47 on
corrected head. Existing frozen checkpoint is durable evidence, not a final PR.
After predecessor phases land, preserve actual main/current production paths,
independently review composition, make one final exact-head commit and obtain
unchanged full hosted quick and applicable source gates before expected-head merge.
All flags false; live issuer/observer/callback deadlines/locks/causal stages remain
NOTIMPLEMENTED. No actual packet/netns/SSH/VPP/nft/capture/lab/host operation.
