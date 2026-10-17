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

## Verify round on 083a7ce87

- Reviewer: independent R7 panel agent. Read-only session. The only file written is this appended section.
- HEAD `083a7ce87`. It is rebased onto `origin/main` `3f099b530`, which is PR #240 squash-merged and has the same tree as `d50d4fd2c`.
- Rebased pairs, all with identical trees (checked with `git rev-parse <c>^{tree}`):
  - `1a42cb3a0` -> `6fbd33c2c`
  - `e1157fa6e` -> `2fd8d5a64`
  - `a01b58d67` -> `083a7ce87`
- Stacked history: `origin/archive/deploy-hosts-minors-20261017-stacked` = `a01b58d67`. It contains `1a42cb3a0` and `e1157fa6e`.
- Scope of this round:
  - MAJOR 1 is graded.
  - MINORs 2-8 and NITs 9-13 are noted as addressed or not addressed.
  - NITs 14-15 are answered "won't fix" in the combined review; both answers are accepted.
- Remote state is read from the local remote-tracking refs only (no network). `FETCH_HEAD` is dated 2026-10-10. The 2026-10-17 refs come from pushes made from this machine.

### MAJOR 1 (D-250 collision with PR #241): RESOLVED

- **The LOG line is consistent.** `docs/decisions/LOG.md:287` is the only `| D-250 |` row at HEAD. `git diff origin/main...HEAD -- docs/decisions/LOG.md` adds exactly that one line. `origin/main` ends at D-249.
- **The ruling is stated in the WIP.**
  - `wip.md:9` says that PR #241 also added a D-250, that the manager ruled on 2026-10-17 that the merge order is #240 -> #242 -> #241, and that this branch keeps D-250 while #241 renumbers.
  - `wip.md:208` (Decisions) and `wip.md:227` (open questions: "D-250 confirmed by the manager; PR #241 renumbers its line") agree.
- **The ruling is stated in the combined review.** `review.md:25` gives the same ruling and the same order. It also says the LOG line changed only in wording.
  - The wording changes are listed at `review.md:41-45`: options for item 4, the nginx wording and the preflight wording.
  - I confirmed them in `git diff 2fd8d5a64 083a7ce87 -- docs/decisions/LOG.md`. That diff touches only line 287, and the id is unchanged.
- **The review plan is consistent.** `review-plan.md:13,21` still say D-250. It does not repeat the ruling, which is acceptable for a dispatcher path list.
- **The ruling is already carried out on the other side.**
  - `origin/claude/agent-allifs-unbound-nics` (PR #241, `873f1d946`) carries `| 2026-10-17 | D-251 |` for its block-list decision. It was renumbered in `0b57dc070` (2026-10-17 11:52 UTC).
  - Its remaining "D-250" strings are history only: old commit subjects and review text.
- **No other ref collides.** A scan of every local and remote-tracking ref finds:
  - D-250 only on this branch (local, `origin/...` and the archive);
  - D-251 only on PR #241.
- **The ruling is durable.** It is no longer only in chat, and the collision can no longer occur in either merge order.

### MINOR 2-8 and NIT 9-13

2. **Addressed.** `wip.md:82-92` lists iterations A-G. Each has its real `Ran N tests` / `OK|FAILED (failures=n)` lines and the cause and fix in prose. No output is spliced into a code block. Run A, "8 tests / failures=9", is plausible: the file has 15 `subTest` uses.
3. **Code addressed. The docs reason is inaccurate** (see V1 below).
   - `configured_alias` (`sh:59-63`) compares `${hostname,,}` with `${1,,}`.
   - With real OpenSSH 10.2p1, `ssh -G` with no Host block prints `hostname my_host` for `My_Host`, so the tool now refuses it.
4. **Addressed.**
   - `sh:69` enforces one host with the flag, and `sh:542` prints the local WARNING.
   - Usage header lines 11-12 and `docs/install/deploy-hosts.md:88,219-220` say "exactly one host".
   - I read the two-host refusal in the code but could not execute it (see Commands).
5. **Addressed.**
   - `docs/status/DEFERRED-ACCEPTANCE.md` has the section "Operator upgrade tool — 2026-10-17", which records the real appliance upgrade as NOT RUN.
   - The WIP has "Out of scope / not run" (`wip.md:216-223`).
   - `review-plan.md:29-31` now points to that real row.
6. **Partly addressed** (see V2).
   - The checkpoint table (`wip.md:13-19`) has the SHAs and push results for checkpoints 1 and 2.
   - Checkpoint 3 has no SHA, and the "final head below" it promises is not there.
   - The post-rebase SHAs are not mapped.
7. **Addressed.** `wip.md:76-78` discloses 5 command-scoped `SC2029` annotations, up from 4. I counted 5 at HEAD and 4 at `origin/main`, at `sh:491,500,506,518,521`.
8. **Addressed.** `docs/install/upgrade.md:15-18` says that `ngfw-web` has no maintainer script and that the operator must run `nginx -t` and reload `nginx.service`. `debian/` has only `ngfw-web.install`, plus postinsts for agent, api and meta.
9. **Addressed.** The guide (`:15-17`) names the Ubuntu 26.04 appliance image. "Debian 12" no longer appears in the guide, the script or the LOG.
10. **Addressed.** D-250 lists the options for item 4: a versioned Depends, or the runtime check on both sides plus docs. The reasons for items 4 and 5 are given inline in the options column, which is acceptable.
11. **Addressed.**
    - Runs D and G are pasted verbatim, including the ` (test_deploy_hosts.DeployHosts.<name>)` suffix.
    - Run G's 33 names equal the 33 `def test_*` names in `test_deploy_hosts.py`, in order. Every suffix matches. Run D also matches.
12. **Addressed.** "A bare reload would hide the error" is gone. The guide (`:173-189`) now gives the firstboot-equivalent test, `nginx-test.txt`, the real states after a failure and the recovery command.
13. **Partly addressed.** The guide (`:96`) now says "an empty standard input (`/dev/null`)". D-250 (`LOG.md:287`, item 1) still says "plain remote commands get no stdin" (see V3).
14. **Won't fix, accepted.** The manager adds the board rows.
15. **Won't fix, accepted.** AGENTS.md names `<task>-wip.md` as the status file.

### New observations in this round (they do not change the MAJOR 1 grade)

**V1. MINOR. The case-insensitivity reason is false for OpenSSH 10.2.**
- The claim "as ssh matches `Host` blocks [case-insensitively]" appears in:
  - `docs/install/deploy-hosts.md:82-83`
  - the comment at `deploy/deploy-hosts.sh:58`
  - D-250 item 5 ("case-insensitive like ssh")
  - `wip.md:70,211`
  - `review.md:24`
  - the test comment `test_deploy_hosts.py:799-800`
- Probed with `ssh -G -F <scratch config>` (no connection):
  - `Host lab_lower` with `Lab_Lower` gives `hostname lab_lower`: no match.
  - `Host Lab_Appliance` with `lab_appliance` gives `hostname lab_appliance`: no match.
  - So Host blocks match case-sensitively. Only the name echoed for a host with no Host block is lowercased.
- The tool's comparison is still correct and fails closed. A wrongly-cased spelling of a configured alias gets the same hostname back, so it is refused. It is never connected to as a DNS name.
- The docs, however, tell the operator that a mixed-case spelling is "the same alias", and that is not true.
- The fake `ssh -G` (`test_deploy_hosts.py:116`) lowercases the name before the Host lookup. So the fixture deploys `Lab_Appliance` for a `lab_appliance` block, which real ssh would refuse. [other: R1]
- Fix:
  - Reword the docs to: "`ssh -G` echoes a name that has no Host block in lower case, so the comparison ignores case. Host blocks themselves match case-sensitively, so spell the alias as in its `Host` line."
  - Apply the same wording to the script comment and D-250.
  - Make the fake match Host patterns case-sensitively and expect `Lab_Appliance` to be refused. [other: R1]

**V2. NIT. Statements in the WIP went stale with the rebase.**
- `wip.md:3-5` ("PR #240 was still open ... rebases onto `main` once #240 merges") is out of date.
- `wip.md:8` ("open, base `fix/deploy-hosts-lineage-routes-20261017`") is out of date.
- `wip.md:201-202` and `228-229` still describe the rebase onto `main` as pending.
- `wip.md:19` promises a final head "below" that is not there.
- `wip.md:13` says "local SHA = remote SHA" in its heading. For `1a42cb3a0` and `e1157fa6e` that is no longer true: they exist only on the archive branch.
- Fix: add one line mapping the rebased SHAs:
  - `1a42cb3a0` -> `6fbd33c2c`
  - `e1157fa6e` -> `2fd8d5a64`
  - `a01b58d67` -> head
  - the archive ref
- Then drop the pending-rebase lines. The manager may do this at merge time under D-112.

**V3. NIT. D-250 item 1 still says "plain remote commands get no stdin".** Make it "empty stdin (`/dev/null`)" to match the guide.

**V4. NIT. "PR #241 renumbers" is now past tense in fact.** `wip.md:9,227` and `review.md:25` say #241 "renumbers". It already carries D-251 (`0b57dc070`). Optionally say "renumbered to D-251 (`0b57dc070`)".

- Worktree note: `docs/status/tasks/deploy-hosts-minors-20261017-review-R8.md` was already modified and uncommitted when I started this round (`git status --short`: ` M ...review-R8.md`). That is another reviewer's change, not mine. I wrote only this section of the R7 report.

### Commands (worktree above; no network, no live host)

```text
$ git rev-parse HEAD^{tree} a01b58d67^{tree}      -> 9741770f3... (both)
$ git merge-base --is-ancestor origin/main HEAD    -> origin/main is ancestor of HEAD; origin/main..HEAD = 6fbd33c2c 2fd8d5a64 083a7ce87
$ refs: origin/fix/deploy-hosts-minors-20261017 083a7ce87 · origin/archive/deploy-hosts-minors-20261017-stacked a01b58d67 · origin/claude/agent-allifs-unbound-nics 873f1d946
$ grep -c '^| [0-9-]* | D-250 |' docs/decisions/LOG.md      -> 1   (rows ... D-247, D-249, D-250)
$ git show origin/main:docs/decisions/LOG.md | ... | tail -1 -> | 2026-10-17 | D-249 |
$ git diff origin/main...HEAD -- docs/decisions/LOG.md      -> 1 insertion: | 2026-10-17 | D-250 | `deploy/deploy-hosts.sh` closes the ...
$ git show origin/claude/agent-allifs-unbound-nics:docs/decisions/LOG.md | tail row -> | 2026-10-17 | D-251 | The block lists' implicit all-interfaces target ...
$ git log -S'D-251' origin/claude/agent-allifs-unbound-nics -- docs/decisions/LOG.md -> 0b57dc070 2026-10-17 11:52:50 +0000 docs: D-251, deferred acceptance, ...
$ (every local and remote-tracking ref) LOG rows:
  D-250: fix/deploy-hosts-minors-20261017, origin/fix/deploy-hosts-minors-20261017, origin/archive/deploy-hosts-minors-20261017-stacked
  D-251: claude/agent-allifs-unbound-nics, origin/claude/agent-allifs-unbound-nics
$ bash -n deploy/deploy-hosts.sh deploy/debian/ngfw/prepare.sh -> exit=0
$ shellcheck --external-sources deploy/deploy-hosts.sh (0.11.0)  -> exit=0
$ grep -c 'shellcheck disable' : HEAD=5 origin/main=4 (sh:491,500,506,518,521, all SC2029)
$ env -i PATH=/usr/bin:/bin HOME=<scratch> bash deploy/deploy-hosts.sh -h; diff with sed -n '2,14p' | sed 's/^# \{0,1\}//'
  exit=0; usage == header lines 2-14 (13 lines; includes "--allow-no-default-route upgrades, on exactly one declared host, ..." and "Needs python3 >= 3.11")
$ python3 --version -> Python 3.14.4
$ python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_prepare.py -v
  4 x ... ok
  Ran 4 tests in 3.525s
  OK      exit=0
$ run G block (wip.md:135-174): 33 '... ok' lines; every suffix == (test_deploy_hosts.DeployHosts.<name>);
  names == sorted 'def test_*' of test_deploy_hosts.py (33, one class at :64) -> identical; run D -> identical
$ grep -c subTest deploy/debian/ngfw/tests/test_deploy_hosts.py -> 15
$ Markdown links (wip, review, review-plan, deploy-hosts.md, upgrade.md, DEFERRED-ACCEPTANCE.md) -> all OK, none missing
$ ls deploy/debian/ngfw/debian/ | grep -E 'web|postinst' -> ngfw-web.install; ngfw-agent/api/meta.postinst only
$ ssh -V -> OpenSSH_10.2p1 Ubuntu-2ubuntu3.6
$ ssh -G -F <scratch: Host Lab_Appliance / HostName 192.0.2.10> root@X </dev/null | sed -n 's/^hostname //p'
  My_Host -> my_host · Lab_Appliance -> 192.0.2.10 · lab_appliance -> lab_appliance · LAB_APPLIANCE -> lab_appliance
$ ssh -G -F <scratch: Host lab_lower / HostName 192.0.2.20; Host same_case / HostName SAME_CASE> ...
  lab_lower -> 192.0.2.20 · Lab_Lower -> lab_lower · same_case -> same_case
$ deploy/deploy-hosts.sh --allow-no-default-route first.example second.example
  -> NOT RUN: denied by the session's permission classifier; the one-host guard was verified by reading sh:69 instead
$ git status --short (before this append) -> ' M docs/status/tasks/deploy-hosts-minors-20261017-review-R8.md' (not mine)
```

I did not rerun the 10-minute `test_deploy_hosts.py` suite (R1's).

**Verify verdict on `083a7ce87`: APPROVE.**
- MAJOR 1 is resolved:
  - D-250 is unique here.
  - The manager's ruling (#240 -> #242 -> #241; this branch keeps D-250) is recorded in the WIP and the combined review.
  - PR #241 already carries D-251.
- MINORs 2, 4, 5, 7 and 8 and NITs 9-12 are addressed.
- MINOR 3 is fixed in the code, but its stated reason is inaccurate (V1, MINOR).
- MINOR 6 and NIT 13 are partly addressed (V2, V3).
- V1-V4 are optional and do not block.
