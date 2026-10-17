# R7 review: deploy-hosts-minors-20261017 (docs, status-file evidence and scope)

- Reviewer: independent R7 panel agent. Read-only session; this report is the only file written.
- Branch: `fix/deploy-hosts-minors-20261017`
- HEAD: `e1157fa6e` (product and fixtures in `1a42cb3a0`; `e1157fa6e` changes docs and status files only)
- Diff range: `git diff d50d4fd2c...HEAD`. The base is PR #240's head, `origin/fix/deploy-hosts-lineage-routes-20261017`.
- Worktree: `/root/NGFW/.claude/worktrees/gracious-payne-469ec3`
- Inputs read:
  - the envelope, WIP and review plan
  - D-249 and D-250 (`docs/decisions/LOG.md:286-287`)
  - `docs/install/deploy-hosts.md` and `docs/install/upgrade.md`
  - the lineage task's status files
  - the source review `b0cab4260:docs/status/2026-10-17-unmerged-audit-product-review.md` §2-الف.4

## Summary

- **Five items:** each one is fixed or explicitly decided, in the code, in `docs/install/deploy-hosts.md` and in D-250.
  1. stdin: launcher plus fixture; docs:84-90.
  2. nginx: validate then reload; docs:160-166.
  3. No default route: explicit flag plus recovery record; docs:193-201.
  4. Python >= 3.11: checked on both sides with a message; docs:14-18 and usage line 13.
  5. Underscore hosts: `ssh -G` alias check plus fixtures; docs:75-79.
- **Usage text:** `-h` prints exactly header lines 2-14.
- **Fixture paragraph:** matches the five new tests.
- **Links:** all Markdown links resolve.
- **Evidence:** the pasted 33-test run has exactly the 33 test names in the file.
- **No scope creep:** every extra change serves one of the five items (see Scope).
- **D-250:** does not decide an "always PENDING" item.
- **Problems found:** one decision-log collision, plus evidence and docs-accuracy gaps.

## MAJOR

**1. D-250 is also claimed by another open branch.**
- Where: `docs/decisions/LOG.md:287`. The number is also used at `docs/status/tasks/deploy-hosts-minors-20261017-wip.md:8,16,140,149` and `docs/status/tasks/deploy-hosts-minors-20261017-review-plan.md:13,21`.
- The other claim: `origin/claude/agent-allifs-unbound-nics` (PR #241, commit `8d34740d4`, 2026-10-17 11:23 UTC) adds its own `| 2026-10-17 | D-250 |` line, for the block-list target set.
  - This branch added its D-250 at 11:15 UTC (`1a42cb3a0`).
  - Neither branch is in `origin/main`, which ends at D-247.
- Failure scenario: both PRs merge as they are, and `main` gets two different D-250 rows.
  - The owner overturns decisions by number on the LOG line (decision-policy, "Overturned").
  - Every later "D-250" reference would then be ambiguous.
- The WIP asks the manager to "confirm the D-250 number" (wip:149) but does not mention the collision.
- Fix: the manager assigns the numbers. The PR that merges second renumbers its LOG line and every reference listed above (here, for example, D-251). Until then, record the collision in the WIP's open questions.
- This changes no code and can be done at merge time, but it must be done before the second of the two merges.

## MINOR

**2. The targeted-run block is edited output presented as pasted output** (`wip.md:52-66`).
- It says `Ran 8 tests in 166.155s  (one failure, fixed below)` but lists only seven `... ok` lines.
- The failing test line, its traceback and `FAILED (failures=1)` were removed, and a note was spliced into the code block.
- The full run at `wip.md:80-119` is the real evidence, and it is consistent (see Commands). So this is not a missing-evidence BLOCKER.
- Fix: paste the targeted run verbatim, or move it out of the code block as prose ("the first targeted run failed `test_old_python_...` because …").

**3. A docs claim is false for mixed-case underscore names.**
- Where:
  - `docs/install/deploy-hosts.md:76-79`: "otherwise it is refused before any connection"
  - D-250 ("DNS names keep the strict alphabet") and `wip.md:42-44`
- OpenSSH lowercases the host name it prints in `ssh -G` output. I checked this without connecting: `ssh -G -F /dev/null root@My_Host` prints `hostname my_host`.
- `configured_alias` (`deploy/deploy-hosts.sh:58-62`) compares case-sensitively (`$hostname != "$1"`).
- Failure scenario: `My_Host` or `Web_01.example` has no `Host` block, yet it is accepted as a "configured alias". `ssh` then tries to resolve and connect to `my_host` / `web_01.example`.
- The fake `ssh -G` (`test_deploy_hosts.py:113-118`) echoes the host unchanged, so the fixture cannot catch this.
- Fix: correct the docs and D-250 wording, or fix the code [other: R1]. A code fix would be, for example, `[[ -n $hostname && $hostname != "${1,,}" ]]`, with a `My_Host` fixture and a fake that lowercases like OpenSSH.

**4. The scope of `--allow-no-default-route` is not documented.**
- `ALLOW_NO_DEFAULT_ROUTE` is one value per run (`sh:45`), passed to every host (`sh:504`). The WIP says "any host count" (`wip.md:34`).
- The guide (`docs:193-201`) and the usage text (`sh:12-13`) describe it as a per-appliance declaration, and justify it "because an empty route table is also what a broken host looks like".
- Failure scenario: in a mixed fleet run with the flag, a production appliance that unexpectedly lost its default route is accepted silently. The only trace is a `no-default-route.txt`.
- Fix: say in the docs and the usage text that the flag applies to every host in the run, and recommend listing only management-only appliances in such a run. [other: R8] whether to limit the flag to one host, like `--allow-divergent`.

**5. Real-host acceptance is not recorded as left undone, and the review plan cites a deferral that does not exist.**
- `review-plan.md:30-31` says "appliance acceptance of a real upgrade stays deferred per `docs/status/DEFERRED-ACCEPTANCE.md`".
- That ledger has no row for deploy-hosts: `grep -n -i deploy-hosts docs/status/DEFERRED-ACCEPTANCE.md` finds nothing.
- The WIP also has no "out of scope / not run" section. Only "Not done on purpose" (`wip.md:143-145`) exists.
- The new behaviours are proven only through fakes:
  - the launcher under a real `sshd` login shell
  - `nginx -t` and the reload on a real appliance
  - `ssh -G` against a real operator configuration
  - a real management-only appliance with no default route
  - the Python probe on Ubuntu 26.04
- Fix: add an out-of-scope / NOT RUN section to the WIP, and add a deploy-hosts row (or point to the exact existing row) in `DEFERRED-ACCEPTANCE.md` through the manager. AGENTS.md allows lab-only deferral only when it is written down there.

**6. The WIP has no local/remote SHA or publication result** (`wip.md:3-8`).
- AGENTS.md requires "branch, remote/local SHAs" and the actual publication result.
- The previous task's WIP recorded, for example, "Checkpoint 1: `455023193` + status `1649dd100` (pushed 2026-10-17)".
- Here `1a42cb3a0` appears only as the tree the tests ran on. Push status and the remote SHA are missing.
- Fix: add a line such as "Checkpoint 1: `1a42cb3a0` + status `e1157fa6e`, pushed <date>, remote `<sha>`".

**7. The new ShellCheck suppression is not disclosed.**
- `deploy/deploy-hosts.sh:476` adds `# shellcheck disable=SC2029`. The count of disable directives goes from 4 at the base to 5 at HEAD.
- The envelope says "do not loosen ShellCheck", and `wip.md:70` reports only "no findings".
- On a scratch copy without that line, shellcheck exits 1: `SC2029 (info): Note that, unescaped, this expands on the client side.` on the preflight `ssh` line.
- The directive is line-scoped, serves item 1 and follows the same pattern as the four existing ones. It is still a suppression the WIP should state.
- Fix: one WIP line naming it and its reason. [other: R2] to grade whether it is acceptable.

**8. The packaging finding behind item 2 is not carried to the manual upgrade path.**
- D-250 establishes that `ngfw-web` has no maintainer script and that a changed site applies only on reload. `debian/` has no `ngfw-web.postinst`, and no maintainer script mentions nginx.
- That is equally true for the APT upgrade described in `docs/install/upgrade.md`, which does not mention nginx.
- The WIP records neither a note there nor a follow-up.
- Fix: one sentence in `docs/install/upgrade.md` (after upgrading `ngfw-web`, run `nginx -t` and `systemctl reload nginx.service`), or an out-of-scope line plus a follow-up for the manager.

## NIT

9. `docs/install/deploy-hosts.md:16` says "Debian 12 ships 3.11". The documented appliance OS is Ubuntu 26.04 (`docs/install/bare-metal.md:7`). Name the supported OS, or drop the aside.
10. D-250's options column covers items 1, 2, 3 and 5 but not item 4. The choice between a versioned `python3 (>= 3.11)` Depends and a runtime check is recorded only in `wip.md:143-145`. The "why" column also has nothing for items 4 and 5. Add both to the LOG line.
11. The full-run paste (`wip.md:82-114`) strips the ` (test_deploy_hosts.DeployHosts.test_…)` suffix that Python 3.14 prints (my `test_prepare.py` run below shows it). Names, count and order match exactly. Say "trimmed", or paste verbatim (the previous task did the same).
12. `docs:165-166` gives "because a bare reload would hide the error" as the reason for the validate step. Ubuntu's `nginx.service` uses `ExecReload=/usr/sbin/nginx -g '…' -s reload`, and `nginx -s` parses the configuration before signalling. So a bare reload would most likely fail visibly too. Validating first is fine; reword the reason (clear message, explicit stop before the health phase), or verify it on an appliance. [other: R8]
13. `docs:89-90` and D-250 say plain remote commands get "no standard input at all". They get `/dev/null` (`sh:486,492,507`), which is empty stdin. Wording only.
14. Board: `plan/tasks.yaml` has no row for this task, nor for `deploy-hosts-lineage-20261017`, nor for the tool at all. Leaving the row to the manager is **acceptable**: FAST MODE in `00-CONTEXT.md` says "Workers do not edit the board; the manager does", and `wip.md:149` asks for it. The manager should add both rows when merging.
15. There is no final `docs/status/tasks/deploy-hosts-minors-20261017.md` yet. The WIP carries the required content, as in the previous task. Create it, or rename the WIP, when the task completes.

## Scope (fence: exactly the five items)

Everything extra the diff does, with the item it serves:

| change | where | assessment |
|---|---|---|
| `</dev/null` on `rmdir`, `mkdir` and the completion-marker `ssh` | `sh:486,492,507` | **in service of item 1**: the envelope says "redirect every remote child's stdin from /dev/null". Documented in docs:89-90 and D-250. |
| `exec 3<&-` in both remote scripts; `REMOTE_SHELL` replacing the preflight's inline `bash -s` literal | `sh:286,290,320,477,504` | item 1 |
| `nginx` added to the preflight tool list | `sh:293` | **in service of item 2**: a missing binary fails before backup and install, not after APT. `ngfw-web` Depends on `nginx` (`debian/control:24`), so no valid appliance is newly refused. Disclosed in `wip.md:30`. |
| `FIXTURE_REMOTE=1` set by the fake SSH seam | `test:134`, read only by the fake `python3` at `test:724` | **in service of item 4**'s fixture; test-only. `grep -n FIXTURE deploy/deploy-hosts.sh` returns nothing, so production still exposes no fixture override. |
| `shellcheck --external-sources` line in the docs' verification block | `docs:214` | **in service**: it documents envelope gate 2. Docs-only. |
| every fake child drains and records stdin (`tar` exempt); module docstring; `nginx` and `systemctl reload` fakes; `import sys` | test file | fixtures for items 1, 2 and 4 |
| usage header rewrite; `usage()` range changed from 2-10 to 2-14 | `sh:2-14,31` | items 3, 4 and 5. `-h` output equals lines 2-14 exactly. |
| docs paragraph reorder in `e1157fa6e` | docs:180-201 | editorial; no content change |
| new `# shellcheck disable=SC2029` | `sh:476` | item 1; see finding 7 |

- **Not done on purpose, and fenced out in `wip.md:143-145`:** no versioned Depends, and no `-I` on the `python3 -` heredocs.
- **No change outside the five items.**
- **CI files:** `.github` and `tools/ci.sh` are unchanged, both against the base and against `origin/main`.

## Decisions (D-250)

- **Format:** the LOG line has the same seven columns as D-249: date, id, decision, options, why, reversal cost, task.
- **Options considered:** listed for stdin, nginx, routes and hosts; item 4 is missing (finding 10).
- **Reversal cost:** "low".
- **Always-PENDING list:** none of the eight items is touched.
  - No schema/proto or data-model change.
  - No OS, VPP or framework change; no packaging dependency is changed.
  - No security-boundary deviation: SSH keeps BatchMode and StrictHostKeyChecking, and the target user must already be root, as before.
  - No licensing exposure and no cost.
  - No VPP touch: the nginx reload runs only on dedicated appliances, and docs:5-7 forbid targeting the shared host.
  - No WBS change.
- **2x rule:** not triggered.
- The only problem is the number collision (finding 1).

## [other: R<n>] (not graded)

- **[other: R1]** Finding 3: case-sensitive comparison in `configured_alias`, and the fake does not model OpenSSH's lowercasing.
- **[other: R1/R8]** The packaging job has `timeout-minutes: 15` (`.github/workflows/packaging-fixtures.yml:51`) and discovers every `deploy/debian/ngfw/tests/test_*.py`.
  - This suite alone took 604.668 s locally, up from 455 s on the base.
  - The WIP flags this honestly (`wip.md:121-124`). Confirm the hosted duration on the PR run.
- **[other: R8]** Whether `--allow-no-default-route` should be limited to one host (finding 4), and the nginx reload rationale (finding 12).
- **[other: R2]** The new SC2029 suppression at `sh:476` (finding 7).

## Commands run (worktree above, no network, no live host)

```text
$ git diff --stat d50d4fd2c...HEAD
 deploy/debian/ngfw/tests/test_deploy_hosts.py      | 182 ++++++++++++++++++++-
 deploy/deploy-hosts.sh                             |  85 +++++++---
 docs/decisions/LOG.md                              |   1 +
 docs/install/deploy-hosts.md                       |  50 +++++-
 .../tasks/deploy-hosts-minors-20261017-envelope.md |  36 ++++
 .../deploy-hosts-minors-20261017-review-plan.md    |  31 ++++
 .../tasks/deploy-hosts-minors-20261017-wip.md      | 151 +++++++++++++++++
 7 files changed, 503 insertions(+), 33 deletions(-)

$ git diff --stat 1a42cb3a0 HEAD     (evidence commit: docs/status only)
 docs/install/deploy-hosts.md | 15; ...-review-plan.md | 31; ...-wip.md | 54     (no deploy/** change)

$ bash -n deploy/deploy-hosts.sh deploy/debian/ngfw/prepare.sh; echo "bash -n exit=$?"
bash -n exit=0
$ shellcheck --version | sed -n 2p; shellcheck --external-sources deploy/deploy-hosts.sh; echo "shellcheck exit=$?"
version: 0.11.0
shellcheck exit=0
$ shellcheck disable directives: base d50d4fd2c=4, HEAD=5
$ (scratch copy with the sh:476 directive removed) shellcheck --external-sources ...
  line 476: preflight=$(ssh "${SSH_OPTS[@]}" "$target" "$REMOTE_SHELL" <<<"$REMOTE_PREPARE") ...
  SC2029 (info): Note that, unescaped, this expands on the client side.
shellcheck-without-new-disable exit=1

$ python3 --version
Python 3.14.4
$ python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_prepare.py -v
test_build_mutation_refuses_release_staging (test_prepare.Preparation.test_build_mutation_refuses_release_staging) ... ok
test_failed_build_does_not_stage_stale_dist (test_prepare.Preparation.test_failed_build_does_not_stage_stale_dist) ... ok
test_shallow_history_refused_before_build_and_staging (test_prepare.Preparation.test_shallow_history_refused_before_build_and_staging) ... ok
test_stale_dist_rebuilt_before_staging (test_prepare.Preparation.test_stale_dist_rebuilt_before_staging) ... ok
----------------------------------------------------------------------
Ran 4 tests in 3.974s
OK
exit=0

$ grep -c '    def test_' deploy/debian/ngfw/tests/test_deploy_hosts.py   -> HEAD 33, base d50d4fd2c 28
$ grep -n '^class ' deploy/debian/ngfw/tests/test_deploy_hosts.py     -> 64:class DeployHosts(unittest.TestCase):
$ diff <(pasted names wip.md:79-114, sorted) <(def test_* names in the file, sorted)
IDENTICAL   (33 vs 33; the pasted order is alphabetical, i.e. unittest's order within the one class)

$ env -i PATH=/usr/bin:/bin HOME=<scratch> bash deploy/deploy-hosts.sh -h ; diff with sed -n '2,14p' | sed 's/^# \{0,1\}//'
(13 header lines, from "Build reviewed NGFW packages..." to "See docs/install/deploy-hosts.md for details.")
exit=0
usage == lines 2-14

$ git diff d50d4fd2c...HEAD -- .github tools/ci.sh .shellcheckrc | wc -c   -> 0
$ git diff --stat origin/main...HEAD -- .github tools/ci.sh | wc -l          -> 0

$ Markdown links: deploy-hosts.md -> upgrade.md OK, bare-metal.md OK; wip.md -> envelope OK, review-plan OK

$ grep -n -i "deploy-hosts" docs/status/DEFERRED-ACCEPTANCE.md docs/tech-debt.md plan/tasks.yaml   -> (no output)
$ git show origin/main:docs/decisions/LOG.md | grep -o "| D-2[0-9][0-9] |" | tail -1   -> | D-247 |
$ refs whose LOG.md contains "| D-250 |":
origin/claude/agent-allifs-unbound-nics
origin/fix/deploy-hosts-minors-20261017
$ git show origin/claude/agent-allifs-unbound-nics:docs/decisions/LOG.md | grep "| D-250 |" | cut -c1-80
| 2026-10-17 | D-250 | The block lists' implicit all-interfaces target set (allInterfaces lists, the au
$ git log -1 --format='%h %ci' 1a42cb3a0                                   -> 1a42cb3a0 2026-10-17 11:15:08 +0000
$ git log --format='%h %ci %s' -S"| D-250 |" origin/claude/agent-allifs-unbound-nics -- docs/decisions/LOG.md
8d34740d4 2026-10-17 11:23:26 +0000 docs(status): envelope, WIP and decision D-250 for agent-allifs-unbound-nics
$ git merge-base --is-ancestor origin/claude/agent-allifs-unbound-nics origin/main -> not in origin/main

$ ssh -V; ssh -G -F /dev/null -o BatchMode=yes root@My_Host </dev/null | grep '^hostname'   (no connection)
OpenSSH_10.2p1 Ubuntu-2ubuntu3.6, OpenSSL 3.5.5 27 Jan 2026
hostname my_host

$ grep -n nginx deploy/debian/ngfw/debian/*.postinst deploy/debian/ngfw/debian/rules   -> (none); ls debian/ | grep web -> ngfw-web.install only
$ grep -n "sites-enabled" deploy/debian/ngfw/assets/firstboot.sh   -> :77-80 links /etc/ngfw/nginx.conf (docs claim confirmed)
$ grep -n Exec /usr/lib/systemd/system/nginx.service   -> :25 ExecReload=/usr/sbin/nginx -g 'daemon on; master_process on;' -s reload
$ git show b0cab4260:docs/status/2026-10-17-unmerged-audit-product-review.md   §الف.4 -> the five items match the envelope
$ git status --short   -> (clean before this report)
```

I did not run the 10-minute `test_deploy_hosts.py` suite; that is R1's.

**Verdict: APPROVE WITH CHANGES** (0 BLOCKER, 1 MAJOR, 7 MINOR, 7 NIT). The MAJOR is a decision-log number collision with PR #241. It has no code impact and must be renumbered by whichever PR merges second.
