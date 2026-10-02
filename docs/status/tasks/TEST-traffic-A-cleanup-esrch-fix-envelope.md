# TEST-traffic-A cleanup ESRCH test closure

Own isolated NGFW-traffic-cleanup-esrch-fix /
`task/TEST-traffic-A-cleanup-esrch-fix-20261002`, base failedf717677c unchanged;
cherry-picked1edf2c8b binding ruling as2fab569b. Existing history/reports preserved.
Actual prior targeted9PASS1ERROR is a failed run, and later47PASS did not close it.

Only source edit: postcleanup assert_verified_child_stopped accepts explicit
ProcessLookupError ESRCH beside FileNotFoundError ENOENT as verified process gone.
No generic OSError catch. Readiness, positive PID/birth S/R checks, timeouts/caps,
original method names/counts and product behavior untouched. Live issuer absent.

Separate controls file outside test discovery exercises post ESRCH accepted;
EACCES/EPERM/EIO/EINVAL remain errors; readiness ESRCH/EACCES/EPERM/EIO fail;
wrong readinessPID/birth/Z fail, positive comm-with-spaces parsing succeeds.
Controls are2 tests, never part of original47 counts. Actual controls2PASS0.007s.
Affected5 repeats each with direct numericPID signals forbidden, then strict47.
Independent closure, integration/main and unchanged hosted gates remain required.

Actual corrected evidence: separate controls2PASS0.007s; affected5 repetitions
each10PASS11.228s with os.kill numericPID signals patched to fail; full strict
47PASS4.020s with zero failures/errors/skips/expected failures/unexpected success.
Unchanged sourced check against fresh origin/main14dc5f82 EXIT0 in2s; gitleaks
307733bytes/no leaks; diff check clean. Prior failed evidence remains historical
and binding ruling closure requires fresh independent verification, not author
PASS alone. No new functionality, live issuer or activation introduced.
