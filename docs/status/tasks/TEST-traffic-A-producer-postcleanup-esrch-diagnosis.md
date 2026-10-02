# Producer postcleanup ESRCH diagnosis — source-test path only

2026-10-02. Snapshot e1697d99788b7d3771eced0e2135a1e5893d9a24.
Own isolated task/traffic-producer-esrch-diagnosis branch/worktree. Report only;
no product/test changes, no extension of previous A3 ruling or merge verdict.

Concrete MINOR test-robustness finding: test_producer.py verify_stopped_identity
catches only FileNotFoundError. Deterministic Path.read_text injection with
ProcessLookupError(errno.ESRCH) propagates instead of accepting verified process
absence. This is the uncovered analogous gone-process error path, not a newly
observed intermittent real process/test failure. Actual prior independent40 and
hosted results remain valid for their executed runs. No producer group cleanup
leak or false PASS inferred.

Actual bounded controls performed against exacte169 helper, no subprocess/signals:
ENOENT accepted; ESRCH propagates ProcessLookupError; EACCES/EPERM propagate
PermissionError; EIO/EINVAL propagate OSError. Checked both callsites: they first
verify actual mountedPID/birth S/R before produce() and only call stopped helper
after timeout/owned-group cleanup. Positive readiness must keep treating ESRCH
and permission failures as errors. Existing correct before/after identity contract
is preserved, unlike prior unrelated-PID proof gap.

If manager schedules hardening, use a NEW separate producer-proof test phase:
only explicit POSTcleanup ProcessLookupError beside FileNotFoundError, never
broad OSError; separate controls preserving permissions/readiness rejection,
original names/counts/timings/product identity and fresh affected repeats/full
source gate. Preserve old frozen PR84/e169 and observed results, do not silently
reuse foundation1016 arbitration or edit its already-reviewed feature head.
No host/process/network/VPP/nft/SSH operation, actual unittest failure, heavy
build, false provenance or whole-task conclusion asserted by this diagnosis.
