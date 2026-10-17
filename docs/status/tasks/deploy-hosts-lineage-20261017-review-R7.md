# R7 review — deploy-hosts-lineage-20261017 (PR #240), docs, status evidence and scope

Reviewer: independent R7 panel agent (read-only session; report saved verbatim by the developer).
Diff reviewed: `git diff 99a774734...HEAD` (HEAD 1649dd100). All commands ran in the worktree
`/root/NGFW/.claude/worktrees/recursing-lichterman-91e003`. The PR #240 body could not be read
(network off); the claims in commit 455023193's message and the WIP file were checked instead.

## MAJOR

**1. The amendment breaks decision-policy conventions.**
- Where: `docs/decisions/DEC-248-development-package-ordering.md:20-43`; `docs/decisions/LOG.md`, which has no D-248 line and ends at D-247 (:285).
- Why: `decision-policy.md` says an agent decision gets "one line in LOG.md including the options considered", with fields for options, why, reversal cost and tasks affected.
- The log's own convention for changing a decision is a new number with "Amends D-nnn" (LOG.md:56 D-047, :122 D-113, :167 D-167, :215 D-192).
- This amendment is a new agent decision: an operator override that turns on `--allow-downgrades`, which D-248 itself had kept closed. It reuses D-248, adds no LOG line, and gives no reversal cost or tasks affected.
- Failure scenario: the owner reviews and overturns decisions through LOG.md. This decision is invisible there.
- Fix: add a line such as `| 2026-10-17 | D-249 | Amends D-248: operator-side ancestry check; --allow-divergent ... See DEC-248 amendment | options… | why… | low (script-only) | deploy-hosts-lineage-20261017 |` and retitle the section. The manager should confirm the number. If D-248's "not in the aggregate log" reason (DEC-248:17-18) is meant to cover the amendment too, the manager should record that exception explicitly.

**2. The docs and the decision record leave out the main real-world case.**
- Where: `docs/install/deploy-hosts.md:83-93`; DEC-248:28-43, including the options considered.
- D-112 (LOG.md:121) and GitHub squash merges mean a build from any PR branch is never an ancestor of the next `main` build. So once an appliance has run a branch build, every later `main` build needs `--allow-divergent`.
- `git fetch origin` (`deploy/deploy-hosts.sh:146`) fetches only `refs/heads/*`. A deleted, squash-merged branch stays unresolvable unless `refs/pull/N/head` or `refs/archive/*` is fetched explicitly. The docs say "refused until it is fetched" without saying how.
- Failure scenario: the owner reads the override as rare, for stale or unrelated branches. In practice it becomes routine, with no documented review step.
- Fix: in both the docs and the DEC, state the squash-merge consequence and give a review procedure (for example `git fetch origin pull/<N>/head` and `git diff --stat <installed> <new>`). Recommend one host per override run. [other: R1] Whether tree containment should be accepted automatically.

## MINOR

**3. The override's scope is not documented.**
- `--allow-divergent` applies to every listed host (sh:393). On each host, the remote side then accepts any lower development version (sh:264).
- docs:86-88 and the script header (sh:7-8) describe it as accepting "the divergent development build", and the header leaves out the unverifiable-lineage case.
- Fix: say it applies to the whole fleet, recommend one host per run, and mention the unresolvable case.

**4. One DEC sentence is false for older hash-only versions.**
- DEC-248:32-33 says "Ancestry implies the count ordering, so the remote dpkg guard still applies unchanged."
- That fails when the new package is an older hash-only build delivered with `-p` and no `build-info.json` (which `verify_packages` allows). The lineage check logs "is an ancestor", then the remote refuses it as a downgrade. The only way through is `--allow-divergent`, for a build that is not divergent.
- Fix: qualify the sentence to "when the new version carries a commit count".

**5. The crash-loop claim is too broad.**
- docs:115-120 says a crash-looping daemon that is momentarily active "is reported as a failure".
- Restarts are only counted between the first counter read right after the restart (sh:330) and the second read (sh:341). That window is the health loop plus 5 s.
- A daemon that fails later is missed: for example, a 10 s VPP timeout plus `RestartSec=3` (`deploy/systemd/ngfw-agent.service:16-17`). nginx and VPP get no restart-counter check at all.
- Fix: state the window and its limit, or add a tech-debt row.

**6. "Fetching origin once" is inaccurate, and the fetch is new scope.**
- docs:81 and DEC:31 say origin is fetched once. It is actually fetched once per host (sh:145-146).
- The fetch has no timeout and updates the operator's remote-tracking refs and FETCH_HEAD.
- It means `-p` deploys now contact origin. The envelope did not ask for that.
- Fix: correct the wording and document the side effect. [other: R8] The missing timeout.

**7. The WIP file is incomplete under AGENTS.md** (`wip.md:5, :49-51`).
- It records only checkpoint 1 (455023193). It does not record head 1649dd100 or PR #240.
- It has no hosted CI result.
- "None known" has no exact next command.
- It has no out of scope, open questions or decisions sections.
- The source review's 2-الف.4 minor findings are not listed as out of scope or as tech-debt rows: stdin `bash -s`, no nginx reload, refusal of hosts with no default route, Python ≥ 3.11 not documented, underscores rejected in host names.

**8. The envelope's source reference is fragile** (`envelope.md:3-4`). It cites the source review by path on an unmerged side branch only. Pin SHA `b0cab4260f9214b988508258b493f209ffa2f789` so it survives the branch cleanup that the same review recommends.

**9. "Before copying anything" (docs:79) hides a leftover.**
- The statement is true, but the preflight has already created `/root/ngfw-deploy/run.XXXXXXXX` (sh:234) before `check_lineage` runs (sh:357).
- A lineage refusal leaves an empty recovery directory on the host, and the error message does not name it.
- [other: R8]

## NIT

10. The comment at sh:268-269 says hop limits change between two readings. Nothing shows that, and the fixture keeps `hoplimit 64` constant (test:144).
11. The warning at sh:393 says builds "will be installed" even when every host passes the lineage check.
12. docs:120-122 should mention that route lines are sorted and that `src`/`proto` are ignored.
13. docs:91-92 should say that an unresolvable *new* source is also refused (sh:147-148).
14. The override and `--allow-downgrades` are not recorded in the recovery directory, only on stderr. [other: R8]
15. The WIP pastes only the test summary tail. Paste the per-test `-v` lines so the 25 test names can be matched.
16. `plan/tasks.yaml` has no row for this task (manager).

## [other: R1]

- The `build-info.json` branch (sh:141-144) is never exercised: fixture deliveries have no `build-info.json`.
- The unresolvable *new* source path has no test.
- `restart_counts` reads `systemctl show` once with no retry. This is the same pattern as the TD-7 finding (`docs/tech-debt.md:31`): a D-Bus timeout becomes a failure after the packages are installed.
- The packaging job has a 15-minute budget (`.github/workflows/packaging-fixtures.yml:51`), and this suite alone takes 324.557 s locally. Confirm the hosted duration.

## Checked and accurate

- **Order:** preflight, then `check_lineage`, then `mkdir`/`scp`. Nothing is transferred on refusal.
- **Stable releases:** downgrades stay refused with or without the flag.
- **Mixed versions:** refused in the preflight, before `mktemp`.
- **Routes:** each default route is reduced to `default`/`via`/`dev`/`metric` and sorted. Raw `ip` output goes to `default-route-v{4,6}.txt`.
- **Stability window:** NRestarts is read after the manual restart, then health check, then `sleep 5`, then `is-active` on all four services, then the counters must match.
- **Fixture paragraph** (docs:143-154): accurate against the test file. The shared fake `git` intercepts every call; the CI runner `.github/scripts/packaging-fixtures.py:24-25` discovers `test_*.py`.
- **Commit-message claims:** "Remote command construction is unchanged apart from the 0/1 literal" and "no CI gate changed" are both true.
- **Scope:** `prepare.sh` changed in comments only. Nothing beyond the three findings except the mixed-version refusal and the origin fetch, and both are documented.
- **Envelope:** stored beside the WIP file. Markdown links resolve.

## Evidence

```
$ git diff 99a774734...HEAD -- .github tools/ci.sh | wc -c
0
$ bash -n deploy/deploy-hosts.sh deploy/debian/ngfw/prepare.sh; echo "bash -n exit=$?"
bash -n exit=0
$ shellcheck --version | sed -n 2p; shellcheck --external-sources deploy/deploy-hosts.sh; echo "shellcheck exit=$?"
version: 0.11.0
shellcheck exit=0
$ shellcheck-disable count: base=3 head=3
$ grep -c '    def test_' deploy/debian/ngfw/tests/test_deploy_hosts.py   -> HEAD 25, base 99a774734 20
$ grep -c '    def test_' deploy/debian/ngfw/tests/test_prepare.py         -> 4
$ python3 -m unittest -v ...DeployHosts.test_stale_branch_with_higher_commit_count_is_refused_before_transfer ...DeployHosts.test_unknown_installed_source_is_fetched_once_then_refused_without_override
... ok
... ok
Ran 2 tests in 19.580s
OK
$ dpkg --compare-versions '0.1.0~dev+111111111111' lt '0.1.0~dev+ffffffffffff' && echo ...
legacy hash order is not chronological: new 1111 < installed ffff
$ bash deploy/deploy-hosts.sh -h   -> prints header lines 2-9 including --allow-divergent; exit=0
$ grep -n "248" docs/decisions/LOG.md   -> (no output)
$ git rev-parse origin/fix/deploy-hosts-lineage-routes-20261017 -> 1649dd10095b43038da0a24e923e152ae5a2283a (local tracking ref)
$ git status --short -> (clean)
```

**Verdict: APPROVE WITH CHANGES** (0 BLOCKER, 2 MAJOR, 7 MINOR, 7 NIT; both MAJORs are decision-record and docs fixes).
