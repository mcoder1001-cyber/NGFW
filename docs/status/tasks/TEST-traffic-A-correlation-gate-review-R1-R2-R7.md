# TEST-traffic-A strict correlation gate — independent R1 / R2 / R7

2026-10-02. **APPROVE R1 / R2 / R7** exact checkpoint
`7c5e605247e4f0b66a59248a0bf0e2805cebffac`. Own worktree
NGFW-traffic-correlation-gate-review, branch task/traffic-correlation-gate-review.
No production/test/CI authorship; only this report is owned. No findings.

R1: loader explicitly names test_foundation/test_evidence/test_correlation;
requires three nonempty integer counts and total33. A missing module produces
loader error, not a silent successful empty suite. Completion additionally
requires testsRun==33 and frozen accepted() rejects failure/error/skip/expected
failure/unexpected success. Therefore reduced/zero/partially executed suites
cannot return successful gate status. Policy checks are explicitly separate
from source test count and lab acceptance. Workflow invokes both paths.

R2: contents:read, pinned checkout and persist-credentials:false, Ubuntu24 and
five-minute timeout retained. Only repository Python source support executes;
no package setup, credentials, hardware adapter or host/network mutation added.
No unsafe shell interpolation or broader privileges introduced by new gate.

R7: source identity diff against1427 is empty. Envelope/WIP identify this as
stacked gate preparation, not yet-current-main mergeable product. Foundation
must land first, retaining its CI and arbitration row; correlation source and
both ruling histories stay preserved. Exact final current-main composition,
applicable review, unchanged full hosted quick and strict33 are required before
merge, with postmerge main verification. Seven executors and producer/lifecycle
remain source NOTIMPLEMENTED; lab NOT RUN is distinct. No whole-task or live
provenance proof is claimed. New gate-policy13 checks are not miscounted as
source33 or forwarding acceptance. Board/product source are untouched.

## Actual independent execution

- `python3 .github/scripts/traffic-correlation-fixtures.py --check-policy`:
 13 PASS, explicitly not source/lab acceptance.
- `python3 .github/scripts/traffic-correlation-fixtures.py`: 33 tests in2.465s,
 PASS, failures=0 errors=0 skipped=0 expectedFailures=0 unexpectedSuccesses=0.
- Supplemented synthetic result policy with six genuinely executed unittest
 suites of33 cases each: pass accepted; actual failure, exception, SkipTest,
 expectedFailure and unexpectedSuccess each rejected by complete() — all six
 guard assertions PASS. These controlled checks are not product source cases.
- `git diff 1427ab2775eb43a597224f12f8135d22d9d2fecb HEAD -- test/topology/traffic-a`:
 empty, confirming no source expansion under the gate review.

No real SSH, capture, ip/netns, VPP/nft, lease or host operation executed. Hosted
CI was not run/queried by this reviewer. This approval does not certify a later
main composition, producer phase, live traffic acceptance or whole TEST-A DONE.
