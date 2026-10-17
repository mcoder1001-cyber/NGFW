# Review R1 (correctness & tests): deploy-hosts-lineage-20261017, PR #240

Reviewer: independent R1 panel agent (read-only session; report saved verbatim by the developer).
Diff reviewed: `git diff 99a774734...HEAD` (HEAD `1649dd100`), worktree
`/root/NGFW/.claude/worktrees/recursing-lichterman-91e003`. The PR body was not fetched (no network
allowed); the claims in the WIP file were compared instead and match.

**Verdict: BLOCK.** Three MAJOR findings and no tech-debt row covers them. No BLOCKERs. The claimed
test results reproduce, and once MAJORs 1–3 are fixed in this branch APPROVE is expected after a
verify round.

## MAJOR

**1. The ancestry decision is not tied to the version actually replaced under the lock.** `deploy/deploy-hosts.sh:217-227,357` vs `:239-267,371`
- **Problem:** the preflight reads the installed version without the lock, and `check_lineage` judges that value. `REMOTE_INSTALL` takes the lock, reads the installed versions again and only runs the `dpkg ge` check. It never compares them with what the preflight saw.
- **Failure:** a concurrent run (the case the lock exists for) can install a different build between preflight and lock. This run then installs over it with no ancestry check, which is exactly the stale/unrelated-branch case the PR says it fixes, and no flag is needed. With `--allow-divergent`, the remote even accepts a lower development version over whatever is installed at that moment.
- **Reproduced:** a fixture subclass where the preflight sees `git40.c` (an ancestor of the new build) and the locked install sees `git41.b` (unrelated history):
  ```
  [appliance.example] installed source cccccccccccc is an ancestor of aaaaaaaaaaaa
  [appliance.example] OK            (returncode 0, installed over git41.b)
  ```
- **Fix:** pass the preflight version as a 5th validated argument. Under the lock, require every package to equal it ("installed version changed since preflight"). Pass `1` to the remote only for hosts where `check_lineage` actually accepted a divergent or unresolvable lineage. Add a fixture where the version changes between the two phases.

**2. `--allow-divergent` also allows a plain rollback on the same history, and it applies to every host in the run.** `deploy/deploy-hosts.sh:153-158,264-266,371,393`
- **Problem:** when the new source is a strict ancestor of the installed one (a rollback, not a divergence), the flag accepts it. The remote then logs "divergent development build accepted" and adds `--allow-downgrades`. Base docs said "Downgrades … are refused", and the envelope asks only to allow "divergent or unresolvable" histories.
- **Why it matters:** squash merges (D112) will make the flag routine. One stale `-p` directory passed with it then rolls back every host that tracks main, and migrations cannot be undone.
- **Reproduced:** fixture with `e` descending from `a`, installed `git50.e`, new `git42.a`:
  ```
  WARNING: aaaaaaaaaaaa does not descend from installed eeeeeeeeeeee; --allow-divergent accepted by operator
  divergent development build accepted over ngfw-agent 0.1.0~dev+git50.eeeeeeeeeeee
  [first.example] OK   ...   allow-downgrades in apt calls: 2
  ```
- **Fix:** refuse when `merge-base --is-ancestor <new> <installed>` succeeds and the commits differ (or require a separate explicit rollback flag). Scope the flag per host as in 1. Add a fixture.

**3. The normal production lineage path (`build-info.json`, full 40-character commit) has no test.** `deploy/deploy-hosts.sh:141-144`; fixtures at `test_deploy_hosts.py:72`
- **Problem:** every deploy fixture copies a delivery that has no `build-info.json`. Only the hand-assembled 12-character path is tested, while real `-b`/`-w` builds always carry `build-info.json`.
- **Proof:** a mutant that replaces this block with `exit 99` passes the whole suite (`Ran 25 tests … OK`).
- **Current behaviour is correct:** I ran it once by adding `build-info.json`. `unrelated-lower` gives rc 1 with `merge-base … bbbb…(40) aaaa…(40)`, and `ancestor` gives rc 0.
- **Fix:** write `build-info.json` (source `a`*40, count 42) into the delivery in at least the stale-branch and ancestor tests, then recompute `SHA256SUMS`.

## MINOR

4. **Only the `via` value of a route is actually tested.** `:278` with the fixture at `test_deploy_hosts.py:137-144`. A mutant that keeps only `via|`… → `via)` passes all 25 tests. Add fixtures that change only `dev` and only `metric`.
5. **Two remote safety conditions are never tested on their own.** `:264`
   - Removing `$ALLOW_DIVERGENT == 1` (mutant M6) passes all 25: the local check always refuses first.
   - Removing `$VERSION =~ $development` (mutant M7) passes all 25. The doc claim at `deploy-hosts.md:90` ("never applies to a stable release on either side") is tested only for the installed side.
   - Fix: the race fixture from 1 covers M6; add a stable-new-version-with-flag case for M7.
6. **The stability window is short and only partly tested.** `:330-341`
   - The window is a fixed 5 s after the API health check, with `RestartSec=3` (`deploy/systemd/ngfw-agent.service:17`). An agent crash later than about 5 s after the API answers is not caught.
   - Only the agent's counter mismatch is tested; the API's is not. An unreadable counter is reported as "restarted during the stability window".
   - Consider a window of at least 2×`RestartSec` plus agent startup, an API-counter fixture and a clearer message.
7. **Unrecognised development versions silently skip the ancestry check.** `:126,137-140`. Example: `--short=12` can return more than 12 characters when ambiguous. The run falls back to the count-only guard, i.e. the original bug. Fail closed for any `~dev` version that does not parse, unless the flag is given.
8. **Some route changes are not detected.** `:273-282`. Sorting each line on its own loses multipath grouping: swapping nexthops between two default routes compares as SAME. `via inet6 X` keeps the word `inet6` and drops the address, so a nexthop change compares as SAME. I reproduced both with a stub `ip`.

## NIT

9. **Fetch is per host, not once.** `:146` runs `git fetch origin` for every host. The docs (`deploy-hosts.md:81`, DEC-248:31) say "once".
10. **Test name and module docstring are out of date.** `test_override_never_accepts_release_downgrade_or_shell_syntax` (`test_deploy_hosts.py:388`) tests no shell syntax. The module docstring (`:4`) does not mention that `git` is now faked.

## [other: R8]
- On a lineage refusal, the empty run directory `/root/ngfw-deploy/run.XXXXXXXX` created by the preflight (`:234`) stays on the host (refusal happens at `:357`).
- `git fetch origin` (`:146`) has no timeout and can hang a fleet run.

## Checked and correct
- Option parsing: `getopts '-:'` rejects `--allow-divergent=1`, `--allow`, `-b--allow-divergent` (rc 2) and the flag placed after a host (rejected by host validation).
- `(( ALLOW_DIVERGENT )) &&`/`||` under errexit behaves correctly.
- `local` is declared separately from the command-substitution assignments, so failures are not masked.
- `restart_counts` exiting inside `$(...)` fails closed: an unreadable counter gives a mismatch, or the assignment fails under errexit.
- `stable_routes` keeps its exit status through the pipeline (pipefail is inherited by the command substitution).
- The two-line preflight check and the `wc -l` here-string count work on Linux (GNU `wc`).
- The `development` regex used from a variable matches as intended.
- Fixture markers are reset per subtest by `reset_host`; I found no cross-test coupling that hides a failure.

**Do the tests catch a revert of each fix?**
- Lineage removed: killed by 5 tests (stale-branch, newer-main, descendant ×2, unknown).
- Raw route text restored: killed by 8 tests, but only success-path ones. The `route`/`route6` subtests still pass with the fix reverted, so they do not prove the fix themselves.
- Restart comparison removed: killed by `crashloop` and the success test.
- `mixed`: fails if the preflight check is reverted.

## Evidence (all run in `/root/NGFW/.claude/worktrees/recursing-lichterman-91e003`)
```
bash -n deploy/deploy-hosts.sh deploy/debian/ngfw/prepare.sh -> exit=0
shellcheck --external-sources deploy/deploy-hosts.sh -> exit=0 (0.11.0); extracted REMOTE_PREPARE/REMOTE_INSTALL -> exit=0
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v
  Ran 25 tests in 333.642s  OK  (real 5m33.909s)   [WIP claims 25 tests, 324.557s, OK: reproduced]
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_prepare.py -v
  Ran 4 tests in 3.606s  OK   [WIP claims 4 tests OK: reproduced]
NGFW_CI_LOG_DIR=<scratch> tools/ci.sh check --base origin/main -> check PASSED (0m10s)
  (origin/main = 99a774734 = PR base; full `tools/ci.sh quick` not run: it needs pnpm/network; hosted quick CI remains required)
Mutants (full suite each): M1 no-lineage FAILED(5) | M2 raw-routes FAILED(8) | M3 via-only OK(25) |
  M4 no-restart-compare FAILED(2) | M5 build-info->exit 99 OK(25) | M6 remote-no-flag OK(25) | M7 remote-no-new-dev OK(25)
```

Verdict: **BLOCK**. Fix MAJORs 1–3 in this branch (or the manager accepts debt rows); the MINORs are optional.
