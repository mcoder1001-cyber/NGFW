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
