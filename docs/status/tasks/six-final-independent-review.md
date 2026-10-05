# Independent source/process review — six-task freeze campaign

Reviewer role: read-only product source; report written only in reviewer docs
branch. Reviewed root 2d10e7be6d01abb79e57786e757c0a87013f30de plus current
working freeze runner, and security 46470888af73ad0e1b8fdf23ce186b6fefee0750.
Generated-file deletions visible while another generator runs are not considered
committed product changes; final merge must verify generated tree cleanliness.

## Freeze runner: BLOCK pending P2 correction

P2, test/acceptance/freeze/run.py:33-70: existing summary.json remains untouched
until all cases finish. Reuse an output directory after a successful run, then
make Popen raise OSError (missing executable, permissions/resource error): old
source_sha and offline_passed=true survive. SIGINT also exits without replacing
old summary or cleaning its child session. This creates stale PASS evidence and
can strand the heavy-step lock/workload. Publish a current not-passed manifest
before spawning; persist terminal/progress results atomically; handle spawn
errors and interruption with owned-process-group TERM/KILL cleanup. Add
regressions using an existing successful manifest and interrupted child.

Actual pure verification:

- python3 -m unittest discover -s test/acceptance/freeze -p test_runner.py -v:
  3 tests PASS, 0.009 s. Dry-run NOT RUN, retained failures and offline/live split
  behave correctly.
- Mock timeout experiment: first wait TimeoutExpired, TERM wait TimeoutExpired,
  then final -9. PASS: owned group received TERM then KILL, exit_code124 retained,
  later cases executed, offline_passed=false.
- Mock spawn-error experiment with preseeded previous PASS summary: reproduced
  stale offline_passed=true after current run raises OSError.

P11-pkg closeout: APPROVE. DEC-ipsec-route-based explicitly excludes that old
build; report states superseded rather than implemented and keeps native runtime
packaging and live acceptance separate. No obsolete host installation required.

## Security dependency/guard delta: APPROVE source scope

Fastify pinned 5.12.5 in app and workspace override; lockfile also resolves Nest's
runtime to 5.12.5. Static10.1.2 satisfies installed plugin fastify5.x metadata,
Nest platform peer ^10.1.2 and Swagger peer ^8 || ^9 || ^10. Scoped js-yaml5
upgrade preserves4.x consumers and resolves Swagger5.4.1. No framework swap or
security-boundary expansion. Test policy fixture isolates missing AAA datastore
while retaining actual token and anonymous path checks; authenticated CSS200
prevents proving denial solely by breaking serving. No guard implementation
changed. Malformed/traversal variants and protected docs surfaces are covered.

Upstream primary advisory ranges independently checked:
[Fastify](https://github.com/fastify/fastify/security/advisories/GHSA-p68q-wchp-6fh7),
[static](https://github.com/fastify/fastify-static/security/advisories/GHSA-83w8-p2f5-377r),
[js-yaml](https://github.com/nodeca/js-yaml/security/advisories/GHSA-r3ph-w7gj-g6xm).
The selected versions address those ranges. This review does not claim absence
of unknown vulnerabilities or duplicate the whole-tree inspection.

Mandatory complete local/hosted quick gate and exact current integration-tree
verification remain merge requirements; reported focused tests/audit evidence
are not substitutes. No full CI was run concurrently by this reviewer.

## Follow-up after root correction

Root now writes an atomic current RUNNING/not-passed manifest before git/spawn,
records failures/progress and catches KeyboardInterrupt with group cleanup.
Independent pure experiments PASS: preseeded PASS + spawn OSError -> FAIL/rc1;
git OSError -> FAIL/rc1; wait KeyboardInterrupt -> INTERRUPTED/rc130 and owned
group SIGTERM. Every resulting manifest has offline_passed=false. Stale PASS
finding is fixed in the inspected working tree.

Remaining P2 process cleanup: SIGTERM has no handler, so manager termination
leaves the separately sessioned child alive. stop_owned also only escalates when
leader wait times out; a promptly exiting leader with a TERM-ignoring grandchild
can retain the inherited heavy lock. Route SIGTERM through interruption cleanup
and make final group cleanup cover descendants after leader exit. Requested
regressions are sent to owner; reviewer has not modified product/tests.

## Final follow-up: APPROVE root source scope

Reviewed corrected root 98de54ad89000493ec7c685027bf7d9410226ce7 and current
runner: script SIGTERM handler raises KeyboardInterrupt; owned cleanup always
sends final group SIGKILL after the grace/reap stage, including when the leader
has exited. P2 stale-report/interruption/descendant findings are resolved.
Independent rerun: 5 pure runner tests PASS in0.031s. Additional mocked
leader-exits-gracefully experiment PASS: group TERM followed by group KILL.
No product/test code edited by reviewer. Root freeze runner and P11-pkg:
APPROVE source scope; security dependency/guard delta remains APPROVE source
scope. Complete quick/local/hosted and exact integration tree remain mandatory.
Root's reported packet/offline campaign is owner evidence, not independently
re-executed by this reviewer and not a release certificate.

## Board recovery and CI preflight follow-up: APPROVE source scope

Independently compared recovered task map to e9c3c9afd: 211 IDs before/after,
zero removals/additions. Only F-backup-restore and TEST-traffic-B rows differ,
preserving newer owner/state/branch/worktree/started/worker_status assignments.
Observed origin/main literal tool truncation prefix; recovered board begins
valid YAML and passes read-only validation. No WBS removal or false completion
transition introduced by recovery.

--check bypasses auto-ready, validates duplicate IDs, dangling dependencies,
cycles and unknown states, and exits before board/PROGRESS writes. CI invokes
it in the existing mandatory slot preflight and fails on invalid board; no
checks removed, renamed, weakened or silently bypassed. Current wrapper needs
the same existing python3-yaml dependency as board management.

Independent pure run: python3 -B -m unittest discover -s test/acceptance/freeze
-v: 9 tests PASS in0.845s. tools/board.py --check: board valid211/read-only.
Generator --check: 58 guides/63 controllers/113 schema sources, all links resolve.
Baseline diff whitespace check PASS. Product source union checked: API manifest,
new security test, lockfile, workspace override and security prompt exactly
match reviewed security branch. Other changes remain the approved runner,
documentation/status and recovered board/preflight tests. No additional
unintended product source edits observed. No full CI run concurrently; complete
mandatory gate and tested current integration tree remain merge requirements.
