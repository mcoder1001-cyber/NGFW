# TEST-traffic-A producer CI gate — independent R1 / R2 / R7

2026-10-02. **APPROVE R1/R2/R7** only new gate checkpoint
`b09b1c4038ecc38b76f2b5e8c4d16dba193a17a4`. Own isolated worktree
NGFW-traffic-producer-gate-review, branch task/traffic-producer-gate-review.
Only this report authored; no source/test/CI authorship or arbiter role.
No findings within the bounded new producer gate scope.

R1: explicit four modules each nonzero integer, loaded total40 and actual
completed40 required. Frozen accepted() rejects failures/errors/skips/expected
failures/unexpected successes and zero execution. Import failures become failing
loader cases, not silent skips. Policy checks separately verify count/outcome
contracts, never counted as product source or live traffic acceptance.
R2: pinned checkout, contents:read, persist-credentials:false, Ubuntu24 and
five-minute bound. Workflow executes only policy and source Python runner;
no package installation, secrets, hardware adapter or new privilege introduced.
R7: e169 traffic source diff is empty; previous independent ebdd identity closure
and historical400b BLOCK are retained. WIP explicitly attributes actual failure
closure to independent review, not author-only tests. Gate/source-only counts,
false proof flags and source gaps vs lab NOT RUN remain accurate. No whole task
DONE or final merge-ready claim follows from this stacked checkpoint.

Actual independent execution:
- `python3 .github/scripts/traffic-producer-fixtures.py --check-policy`:13 PASS.
- `python3 .github/scripts/traffic-producer-fixtures.py`:40 tests in3.994s PASS;
 failures=0 errors=0 skipped=0 expectedFailures=0 unexpectedSuccesses=0.
- Six genuinely executed controlled unittest suites of40 cases: pass accepted;
 actual failure, error, SkipTest, expectedFailure and unexpectedSuccess rejected.
 All six guard assertions PASS, separately from40 source tests.
- `git diff e1697d99788b7d3771eced0e2135a1e5893d9a24 HEAD -- test/topology/traffic-a`:
 empty. Inspected actual runner/workflow/envelope/WIP and preserved closure.

Known integration dependency is explicit, not a lab waiver: old foundation16
module-loader would load22 expanded foundation/evidence cases. This branch
neither fixes nor approves that separate compatibility delta. Independent fix
review and foundation→correlation→producer serial landing, final current-main
preservation and unchanged full hosted quick/all phase gates on exact final
head remain required. Hosted CI not queried/run here. No live SSH/network,
ip/tcpdump/VPP/nft/capture/lease or host mutation executed. Source-only approval
never grants live namespace/device/manager ownership or packet provenance.
