# Review R7 (docs, status-file evidence and scope): agent-allifs-unbound-nics

Reviewed head `552b9bcce` (product commit `9a63c09ce`, status commits `8d34740d4` and `552b9bcce`), base `origin/main` `99a774734`, PR #241. Reviewer: panel R7. This was a read-only run: I did not create, modify or delete any file in the worktree, and `git status` stayed clean throughout. The developer saves this report.

## Verdict: BLOCK. 0 BLOCKER, 2 MAJOR (#1, #2), 7 MINOR (#3–#9), 6 NIT (#10–#15)

Both MAJORs are status and decision-log fixes that take minutes in this branch. Once both are fixed and confirmed in a verify round, the verdict becomes APPROVE WITH CHANGES, or APPROVE if the MINORs are also fixed or answered.

**Summary.** The task report has every required section: what was built, pasted red/green output, the agent gate output, out of scope, open questions, decisions and review. Its claims match the code and the developer's own log:
- Test names, failing line numbers (`global_blocking_test.go:262`, `physical_nics_acl_test.go:54,86`) and the `Unavailable … acl.interface-binding/ens161` symptom match.
- The developer's gate log shows 143 `ok`, 0 `FAIL`, golangci-lint `0 issues.` and `AGENT_GATE_EXIT=0`.
- The 143 matches the 143 test-bearing directories in `apps/agent`.
- `9a63c09ce..552b9bcce` adds docs only, so the gate on `9a63c09ce` covers the product code at the head.
- `projection.go:411` is the only production caller of `desired.ACL`.
- `autoblock/runtime.go:123` confirms the overlay always sets `AllInterfaces: true`.
- I re-ran the green targeted tests and they pass.

There is no scope creep. Every changed file is inside the fence and the envelope's owned list, except `docs/decisions/LOG.md` (NIT #13). The extra fake-VPP end-to-end test stays within "unit tests only". The quick gate is still running, as the files say; nothing claims it passed.

Not touching `docs/tech-debt.md` on `main` is the right call: `main` has no such row, the defect is fixed here, and adding a row would collide with #238's row. The handover is incomplete, though (#2, #3).

## Findings

### MAJOR

1. **MAJOR: D-250 is already taken on another published branch.**
   - Location: `docs/decisions/LOG.md:287`, `docs/status/tasks/agent-allifs-unbound-nics.md:64`, the PR body, and the subject of `8d34740d4`.
   - `origin/fix/deploy-hosts-minors-20261017` carries a different D-250 at its `LOG.md:287` ("`deploy/deploy-hosts.sh` closes the five minor items …"). It was committed at 11:15:08 and pushed at 11:15:11 (local reflog). This branch committed its D-250 at 11:23:26 and pushed at 11:23:28, eight minutes later.
   - The developer avoided D-248/D-249 but did not check D-250.
   - Scenario: whichever branch merges second puts two different D-250 decisions on `main`. Every "D-250" in this report, the PR and that branch then becomes ambiguous for the owner, who would have to apply the retroactive alias rule at `LOG.md:182`.
   - Fix: renumber to **D-251**, which no `origin/*` ref uses (`git grep -E 'D-25[1-9]'` over every remote ref returned nothing). Change `LOG.md:287`, report line 64 and the PR body, and correct the `8d34740d4` subject in the final D112 squash message. I did not check that branch's PR state, because `gh pr list` is outside this run's allowed commands. The clash exists either way, since the branch is published.

2. **MAJOR: the #238 follow-up covers only one merge order.**
   - Location: `docs/status/tasks/agent-allifs-unbound-nics.md:53`, `:60` ("Open questions: None"), and `agent-allifs-unbound-nics-envelope.md:8`.
   - The report says the #238 developer will update the row "once this PR is merged". That only works if #241 merges first.
   - #238's combined review (`boot-nic-registration-port-review.md:17`) is at APPROVE WITH CHANGES, waiting only for the manager to accept the debt row, so #238 merging first is realistic. In that case `main` would get three pieces of text that this PR makes false:
     - (a) `docs/tech-debt.md` row "2026-10-17 (PR #238 review R4, MAJOR …)" describing the defect as open;
     - (b) the user-doc paragraph in `docs/user/interfaces/default-dataplane-nics.md` (after the "awaiting engine" sentence): "Exception (known agent defect …) … every commit touching interfaces or security — registration included — is refused … select the block list's interfaces explicitly instead of *all interfaces*", plus the parenthetical "(except through an all-interfaces block list or automatic blocking …)" in the released-row bullet;
     - (c) `docs/status/DEFERRED-ACCEPTANCE.md:81` case (e) "(known agent defect, tech-debt)".
   - Once #238 is merged, its branch and developer are gone, and nothing in the repo assigns the correction. Users would be told to avoid *All interfaces* for a defect that no longer exists.
   - Fix (status text in this branch):
     - (i) In the report and WIP, recommend merging #241 before #238 (R4's own option (a)). #238 then drops the row, the exception and the parenthetical, and rewords case (e) before it merges.
     - (ii) State the other order as well: "if #238 merges first, rebase this branch and in it close the row with a pointer to PR #241, remove the exception paragraph and the parenthetical, and reword case (e)". Note that this adds `docs/user/**`, which makes R6 mandatory.
     - (iii) Raise the merge-order choice under Open questions for the manager.

### MINOR

3. **MINOR: the suggested replacement row lives only in the PR body and still has a placeholder.**
   - Location: PR #241 body, "Follow-up on the #238 branch"; report line 53 points to it.
   - The text reads "fixed by PR #@@N@@". The number was never filled in, and the text is not in the repo. The owner reads only the repo, and AGENTS.md says no knowledge essential for recovery may live outside it.
   - Fix: replace `#@@N@@` with `#241` and paste the row text verbatim into the report's "Out of scope" section.

4. **MINOR: the WIP file is stale at the head.**
   - Location: `docs/status/tasks/agent-allifs-unbound-nics-wip.md:2,8`.
   - Line 2 says "Published checkpoint: 9a63c09ce", but `origin/claude/agent-allifs-unbound-nics` has been `552b9bcce` since 11:25:18. There is no local SHA field, which AGENTS.md requires.
   - Line 8 still lists "PR" under Remaining, while the report (line 3) names PR #241 as open. The two files disagree.
   - Fix: when the quick gate ends, update the remote and local SHAs, the PR number, the gate result and the next command.

5. **MINOR: the pasted outputs are edited without marking the edits.**
   - Location: report lines 16–25 and 34–47.
   - Line 38 appends "(golangci-lint run --allow-serial-runners ./... — no findings)" to a timing line; the real log line is `0 issues.`.
   - Line 44, "…  143 packages ok, 0 FAIL", is a summary, not output.
   - The red block drops `go test`'s bare `FAIL` lines.
   - I checked the developer's log (`scratchpad/agent-gate.log`): every claim is true (143 `ok`, 0 `FAIL`, `0 issues.`, exit 0). The problem is that annotations sit inside "pasted output".
   - Fix: paste the real `0 issues.` line. Mark elisions as comments, for example `# 141 further "ok" lines elided; grep -c '^ok' agent-gate.log → 143; grep -c FAIL → 0`.

6. **MINOR: the quick-gate result will not reach the report.**
   - Location: report line 49.
   - It says the result goes "to the WIP file and the PR description". The report is the owner-facing record, so it would keep saying "running at the time of writing" for good.
   - Fix: also paste the gate's tail and exit code into the report's "How verified" section and remove "running". This is about where the result goes; the gate not having finished yet is not a finding.

7. **MINOR: the PR body points to the wrong file for the gate outputs.**
   - The "Gates" heading says "(pasted outputs in `docs/status/tasks/agent-allifs-unbound-nics-wip.md`)". The WIP file has prose only; the pasted outputs are in `agent-allifs-unbound-nics.md`.
   - Fix: correct the file name in the PR body.

8. **MINOR: the user doc on `main` describes *All interfaces* more broadly than the new behaviour.**
   - Location: `docs/user/security/global-blocking.md:27`, which says "*All interfaces* also covers interfaces added later."
   - After this PR, *All interfaces* means the interfaces the data plane has:
     - a physical NIC awaiting engine is covered only once it is bound (at the next projection, per report line 55);
     - a released NIC is never covered by VPP ACLs (host-bound traffic is covered only by "Protect the box").
   - Fix (optional): add one sentence there. This brings in R6, so the developer may instead answer "won't fix, covered by the #238 doc work".
   - `main`'s `docs/user/interfaces/default-dataplane-nics.md:72-73` ("the commit succeeds") and `:109-110` ("does not make commits fail") already describe the fixed behaviour; they were false on `main` and become true with this PR, so they need no change. The report could say that `main`'s docs were checked.

9. **MINOR: the lab acceptance for this fix is not traced.**
   - Location: report line 54, "Not run: … lab or appliance acceptance", with no pointer to a record.
   - The only record is case (e) of the #238 branch's DEFERRED-ACCEPTANCE row. Nothing on `main` tracks it if #238 stalls.
   - Fix: name that row and case in the report. Include the rewording of case (e) in the follow-up text from #2 and #3.

### NIT

10. The `9a63c09ce` message ("All three fail on main") and the PR heading "Tests (fail on `main`, pass here)" are imprecise. On `main` the desired test file does not compile (`ACL` takes 3 arguments there). The red evidence comes from reverting the `global_blocking.go` hunk on this branch, which the report states correctly. Fix in the D112 squash message: "fail without the `global_blocking.go` change".
11. The `9a63c09ce` message cites "docs/tech-debt.md 2026-10-17 row" without saying the row is on #238's branch; `main` has none. Fix in the squash message.
12. The two new tests use opposite names for the same roles, which makes the pasted red output harder to follow. `global_blocking_test.go:246-248` has ens161 = unbound and ens192 = released; `physical_nics_acl_test.go:27-29` has ens192 = unbound and ens161 = released. Optional: align the names.
13. The envelope's owned list (`agent-allifs-unbound-nics-envelope.md:6`) leaves out `docs/decisions/LOG.md`, which the branch edits. Add it.
14. WIP line 6 gives the test pattern as `'TestGlobalBlocking|...PhysicalNic|...AutoBlock...'`, which cannot be run. Replace it with the full pattern from report line 27.
15. Report lines 10 and 53 and envelope line 2 refer to the #238 files only as code spans "on the #238 branch". Once #238 merges and its branch is deleted, those references point nowhere. Use links pinned to `c2f898d2b` (`https://github.com/mcoder1001-cyber/NGFW/blob/c2f898d2b/docs/tech-debt.md`, `…/docs/status/tasks/boot-nic-registration-port-review-R4.md`) and link PR #238 in the report header.

### Answers to the dispatch questions

- **D-250's content and format are justified.** The line has the table's 7 cells (8 pipes, same as header line 7), lists the options, the reason, the reversal cost and the task, and follows the existing blank-line row layout. Logging rather than writing a PENDING file is correct: it is an internal Go signature, on no always-PENDING item, and costs far less than 2× the task to reverse. Only the number is wrong (#1).
- **Decision-log note for the manager (outside this branch):** D-249 already means two different decisions on published branches. One is in `origin/claude/boot-nic-registration-port` (amends HIST-NATIVE-D-164); the other is in `origin/fix/deploy-hosts-lineage-routes-20261017` and `-minors-20261017` (amends D-248). One of them must be renumbered before the second merges. A number-reservation step would prevent repeats of #1.
- **Board:** `plan/tasks.yaml` has no row for this task (or for #238). The developer was right not to touch it: `prompts/00-CONTEXT.md:124` says workers do not edit the board and the manager does. The manager should add or close a row when merging.
- **Markdown links:** the three new files have no `[..](..)` links. The one URL (PR #241) is correct, and every repo path the report cites exists at the head.

### `[other: …]`

- Nothing serious outside my aspect. The R1-style re-run of the green tests passes; the red run was not reproduced, because `git apply` is forbidden in this run.

## Evidence (commands run in `/root/NGFW/.claude/worktrees/funny-meninsky-c9ef19`, read-only)

```
$ git log --oneline -6 ; git rev-parse origin/main HEAD
552b9bcce docs(status): task report for agent-allifs-unbound-nics (PR #241)
8d34740d4 docs(status): envelope, WIP and decision D-250 for agent-allifs-unbound-nics
9a63c09ce fix(agent): all-interfaces block lists target only the rows the projection keeps
99a774734 fix(deploy): complete package upgrades and audit unfinished branches
99a774734e2cf455f71563ff0dba5afb58051685
552b9bcce6e33847cd61ba245b83556966cca8e4

$ git diff --stat 99a774734...552b9bcce
 .../agent/internal/agent/physical_nics_acl_test.go | 122 +++++
 apps/agent/internal/agent/projection.go            |   5 +-
 apps/agent/internal/desired/acl.go                 |   9 +-
 apps/agent/internal/desired/acl_test.go            |  10 +-
 apps/agent/internal/desired/global_blocking.go     |  18 ++-
 .../agent/internal/desired/global_blocking_test.go |  58 +++-
 apps/agent/internal/desired/rule_expiry_test.go    |   6 +-
 apps/agent/internal/subsystems/acl_pbr_test.go     |   2 +-
 docs/decisions/LOG.md                              |   2 +
 .../tasks/agent-allifs-unbound-nics-envelope.md    |   9 ++
 docs/status/tasks/agent-allifs-unbound-nics-wip.md |  10 ++
 docs/status/tasks/agent-allifs-unbound-nics.md     |  68 ++++
 12 files changed, 297 insertions(+), 22 deletions(-)

$ git diff --stat 9a63c09ce 552b9bcce        # head adds docs only over the gated commit
 docs/decisions/LOG.md | 2 + ; ...-envelope.md | 9 + ; ...-wip.md | 10 + ; agent-allifs-unbound-nics.md | 68 +
 4 files changed, 89 insertions(+)

# D-number scan over every origin ref (D-248+ only)
$ for r in $(git for-each-ref --format='%(refname:short)' refs/remotes/origin/); do git show "$r:docs/decisions/LOG.md" | grep -o '| D-2[4-9][0-9] |' ...; done
origin/claude/agent-allifs-unbound-nics: ... | D-247 | | D-250 |
origin/claude/boot-nic-registration-port: ... | D-247 | | D-249 |
origin/fix/deploy-hosts-lineage-routes-20261017: ... | D-247 | | D-249 |
origin/fix/deploy-hosts-minors-20261017: ... | D-247 | | D-249 | | D-250 |

$ git show origin/fix/deploy-hosts-minors-20261017:docs/decisions/LOG.md | grep -n 'D-250'
287:| 2026-10-17 | D-250 | `deploy/deploy-hosts.sh` closes the five minor items of the 2026-10-17 review (2-الف.4): ...
$ git log -S'| D-250 |' --format='%h %ad %s' --date=iso origin/fix/deploy-hosts-minors-20261017 -- docs/decisions/LOG.md
1a42cb3a0 2026-10-17 11:15:08 +0000 fix(deploy): close the five minor review items for deploy-hosts.sh
$ git reflog show --date=iso refs/remotes/origin/fix/deploy-hosts-minors-20261017 | head -2
e1157fa6e ...@{2026-10-17 11:25:10 +0000}: update by push
1a42cb3a0 ...@{2026-10-17 11:15:11 +0000}: update by push
$ git reflog show --date=iso refs/remotes/origin/claude/agent-allifs-unbound-nics
552b9bcce ...@{2026-10-17 11:25:18 +0000}: update by push
8d34740d4 ...@{2026-10-17 11:23:28 +0000}: update by push
9a63c09ce ...@{2026-10-17 11:20:00 +0000}: update by push
$ for r in <all origin refs>; do git grep -l -E 'D-25[1-9]' $r -- docs; done
(no output — D-251 is free)

$ awk 'NR==7||NR==287{n=gsub(/\|/,"|"); print NR": pipes="n}' docs/decisions/LOG.md
7: pipes=8
287: pipes=8

# Green targeted tests (allowed command), from apps/agent
$ env -u NGFW_INTEGRATION go test -count=1 -run 'TestGlobalBlocking|TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs|TestAutoBlockWithUnboundAndReleasedNICsOnFake|TestProjectPhysicalNic' ./internal/desired/ ./internal/agent/
ok  	ngfw/agent/internal/desired	5.173s
ok  	ngfw/agent/internal/agent	0.267s
$ (same, -v, grep '^--- \|^ok')
--- PASS: TestGlobalBlockingProjection / DriftReconstructs / AllInterfacesAndDisabled / BucketsAndScale (4.50s) / Limits
--- PASS: TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs (0.00s)
ok  	ngfw/agent/internal/desired	4.991s
--- PASS: TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs (0.01s)
--- PASS: TestAutoBlockWithUnboundAndReleasedNICsOnFake (0.06s)
--- PASS: TestProjectPhysicalNic{AwaitingDataplane,BoundProjectsNormally,NoLookupProjectsNormally,LcpTapIsNotUnbound,UnboundHasNoLcpPair,ReleasedToHost}
--- PASS: TestGlobalBlockingOnFake (0.06s)
ok  	ngfw/agent/internal/agent	0.272s

# Cited failure lines exist
$ sed -n '262p' .../desired/global_blocking_test.go ; sed -n '54p;86p' .../agent/physical_nics_acl_test.go
		t.Fatal("a released NIC is not an all-interfaces target")
			t.Fatalf("no binding for %s: its interface alias is never projected", name)
		t.Fatalf("auto-block set with an unbound and a released NIC: %v %v", resp, err)

# Developer's gate log (read only) vs. the paste
$ grep -E 'NGFW_TIMING|AGENT_GATE_EXIT' agent-gate.log ; grep -c '^ok' ; grep -c FAIL ; non-ok lines
(timestamps identical to report lines 35-45) AGENT_GATE_EXIT=0
ok lines: 143   FAIL lines: 0   no test files: 166
ok  	ngfw/agent/internal/agent	58.744s
ok  	ngfw/agent/internal/desired	30.261s
0 issues.
$ git ls-files 'apps/agent/*_test.go' | xargs -n1 dirname | sort -u | wc -l
143

# Overlay always selects all interfaces; only one production caller of desired.ACL
$ grep -n AllInterfaces apps/agent/internal/autoblock/runtime.go
123:	out.Acl.GlobalBlocking.Lists[ListName] = &ngfwv1.GlobalBlockingList{Enabled: proto.Bool(true), AllInterfaces: proto.Bool(true), ...
$ grep -rn 'desired.ACL(\|^\s*ACL(' apps/agent --include=*.go | grep -v _test
apps/agent/internal/agent/projection.go:411:		desired.ACL(p, ds, in, netdev) ...

# Docs on main
$ git show origin/main:docs/user/interfaces/default-dataplane-nics.md | grep -n 'commit succeeds\|does not make'
72-73: ... `agent.nic-not-bound`), not an error — the commit succeeds and nothing is taken over early.
109: ... a released row is never configured on the data plane, so the missing NIC does not make [commits fail]
$ grep -rn -i 'all interfaces' docs/user/security/
docs/user/security/global-blocking.md:27:| All interfaces / Interfaces | Where the list is enforced. *All interfaces* also covers interfaces added later. ...
$ git grep -n -E '#241|pull/241|agent-allifs' origin/claude/boot-nic-registration-port -- docs plan
(no output)
$ git show origin/claude/boot-nic-registration-port:docs/status/tasks/boot-nic-registration-port-review.md | grep -n 'Combined verdict'
17:## Combined verdict: APPROVE WITH CHANGES — one condition open for the manager, merge only on the owner's go-ahead
$ git show origin/claude/boot-nic-registration-port:docs/status/DEFERRED-ACCEPTANCE.md | sed -n 81p | grep -o '(e) .*tech-debt)'
(e) with auto-block enabled and a live entry, or an `allInterfaces` block list, plus an unbound and a released row: commit and enforcement succeed (known agent defect, tech-debt)

# PR bodies
$ gh pr view 241 --repo mcoder1001-cyber/NGFW --json state,headRefOid,body
OPEN 552b9bcce...  body contains: "Gates (pasted outputs in `docs/status/tasks/agent-allifs-unbound-nics-wip.md`)" and "fixed by PR #@@N@@"
$ gh pr view 238 --repo mcoder1001-cyber/NGFW --json state,headRefOid,title,comments
OPEN c2f898d2b fix(interfaces): register missing data NICs at every API boot   (no comment mentions 241)

# Board, markdown links, cleanliness
$ git diff --name-only 99a774734...552b9bcce -- plan ; grep -n -i 'allifs\|boot-nic-registration' plan/tasks.yaml
(no output)
$ grep -n -o '\[[^]]*\]([^)]*)' docs/status/tasks/agent-allifs-unbound-nics*.md
(no output)
$ git status --short | wc -l
0
```

Verdict: **BLOCK** (0 BLOCKER, 2 MAJOR, 7 MINOR, 6 NIT). It becomes APPROVE WITH CHANGES once #1 (renumber to D-251) and #2 (merge order recorded and both orders covered in the report and WIP) are fixed in this branch and confirmed in a verify round.

## Verify round (head `873f1d946`)

Verified head `873f1d946` (product tree = `856bf743a`; gated head `0b57dc070`), base `99a774734`, PR #241. Reviewer: panel R7, verify round only. This was a read-only run. Temporary files went only to `scratchpad/review-R7-verify/`, and `git status --short` at the end is 0 lines.

### Verdict: APPROVE WITH CHANGES. New findings: 0 BLOCKER, 1 MAJOR (N1), 3 MINOR (N2–N4), 3 NIT (N5–N7)

Round-1 status: 12 fixed, 2 partially fixed (#1, #3), 1 not fixed (#7). All three leftovers are in the GitHub PR description and are folded into N1.

**Merge condition:** N1, which means publishing the PR description and checking it. Checking it is mechanical: the manager can confirm it with the `gh` command given in N1. I do not need another R7 round for it; a third round would have to go to the arbiter. N2–N7 are optional.

### Round-1 findings

1. **MAJOR #1 (D-250 clash): partially fixed.**
   - Fixed in the repo:
     - `docs/decisions/LOG.md:287` is now D-251, with 8 pipes like header line 7. Its text now says "kept rows from the projection" (chosen after R1 #1 / R4 #1), and it explains the renumbering.
     - The report (`:166`), the WIP and the envelope say D-251.
     - The WIP's D112 squash message (`wip.md:13`) is "… (PR #241, D-251)" and notes the `8d34740d4` subject correction.
     - Scan over all 17 `origin/*` refs: `origin/main` = D-249 (#240), #242 = D-250, this branch = D-251, #238 = D-252 (`009f9a423`). No clash.
   - Not fixed: the PR description still says "Options considered (decision log D-250)". See N1.
2. **MAJOR #2 (merge order / #238 follow-up): fixed.**
   - The manager's order #240 → #242 → #241 → #238 is in the report (`:152`, `:162`) and the WIP (`:10`).
   - It is independently recorded on the other branches:
     - #242: `deploy-hosts-minors-20261017-wip.md:9`, its review file `:25`, and the PR #242 body ("per the manager's merge order #240 → #242 → #241").
     - #238: `009f9a423` `boot-nic-registration-port-wip.md:14-17` ("Merge order: #240 → #242 → #241 → #238; … the R4 debt row is closed with a pointer to #241 …").
   - So the #238 session has the assignment in its own recovery file, and the replacement texts are in this repo at report `:153-154`.
   - Given that decision, I do not require the reverse order to be documented. If the order is ever changed, part (ii) of round 1 applies again.
   - One residual mismatch between the two records is N4 (MINOR).
3. **MINOR #3: partially fixed.** The row text with "#241" is in the report (`:153`). The live PR body still has "fixed by PR #@@N@@" (N1).
4. **MINOR #4: fixed.** `wip.md:2` gives the published checkpoint `0b57dc070` with local = remote, the status commit on top, and PR #241. It also has both gate results (`:7-8`), the next command (`:14`) and "Current failure: none".
5. **MINOR #5: fixed.**
   - The red block (report `:19-80`) and the green block (`:87-89`) are byte-identical to `red.log` and `green.log`; only the `$ …` command line is added.
   - Every pasted agent-gate line is verbatim in `agent-gate-2.log`, and the elision comment's counts are right (143 `ok`, 0 FAIL).
   - The quick-gate summary (`:119-145`) is identical to `ci-quick-2.log:88-114`.
   - Small unmarked elision: N6.
6. **MINOR #6: fixed.** Both gate results are pasted in the report (`:94-146`), and "running" is gone.
7. **MINOR #7: not fixed.** The live PR body still says "Gates (pasted outputs in `docs/status/tasks/agent-allifs-unbound-nics-wip.md`)" (N1). The report's review table (`:177`) nevertheless lists "#7 PR body" as handled.
8. **MINOR #8: fixed.** A sentence was added at `docs/user/security/global-blocking.md:27`. Its "every interface the data plane has" is too broad: N3.
9. **MINOR #9: fixed.**
   - `DEFERRED-ACCEPTANCE.md` has a new section "All-interfaces block lists and auto-block with unbound and released NICs — 2026-10-17" with cases (a)–(e).
   - Its format matches the neighbouring 2026-10-07 section: heading, paragraph, closing quick-gate sentence.
   - Both paths it cites exist.
   - The report names it (`:156`).
10. **NIT #10: fixed.** The squash message no longer says "fail on main", and the PR-draft heading is "fail with the pre-fix line".
11. **NIT #11: fixed.** `wip.md:13` notes that the cited row is on the #238 branch.
12. **NIT #12: fixed.** ens192 is unbound and ens161 released in both `global_blocking_test.go:249-250,296-297` and `physical_nics_acl_test.go:28-32`.
13. **NIT #13: fixed.** The envelope's owned list (`:6`) now includes LOG.md (D-251), the user-doc sentence, and the DEFERRED and tech-debt rows. All 19 changed files are inside it.
14. **NIT #14: fixed.** `wip.md:6` has the full six-test pattern. Its "six tests fail" claim is wrong: N2.
15. **NIT #15: fixed.**
    - Report `:3` and envelope `:2` now use links pinned to `c2f898d2b`.
    - `c2f898d2b` is an ancestor of #238's head `009f9a423`, so it stays reachable.
    - At that commit, `tech-debt.md:230` is the "PR #238 review R4, MAJOR" row and the R4 review file exists.

### New findings

**N1. MAJOR: the PR #241 description was never updated. It contradicts the head, and the status report says it was fixed.**
- **Where:** PR #241 body; report `:177` ("#7 PR body"); also the hand-off claims for #3 and #7.
- **Evidence:** the live body (`gh pr view 241`, headRefOid `873f1d946`) is byte-identical to the developer's first draft `scratchpad/pr-body-1.md` (11:23). None of the later drafts (`pr-body-2..5.md`, the last at 12:03) reached GitHub.
- **What the live body still says:**
  - "decision log D-250". That number belongs to #242 on `main` once #242 merges: the exact ambiguity of round-1 #1.
  - The rejected first design as the chosen one ("`desired.ACL` takes the Linux netdev lookup … (chosen …)").
  - "Tests (fail on `main` …)" with three tests.
  - The gate on `9a63c09ce`, "`tools/ci.sh quick` on the head: **running**", and the outputs "in … `-wip.md`".
  - The panel "In progress" without R6.
  - "fixed by PR #@@N@@", and "The user-doc exception paragraph can then be dropped". The report (`:152`) says the opposite: "replaced (not dropped)".
- **Scenario:** the manager merges from the PR page and reads a design, decision number and gate state that are not the branch's. If GitHub squash is used, D-250 and the stale text can also reach `main`'s history.
- **Fix:**
  1. Publish the corrected description. `pr-body-5.md` in the scratchpad already matches the head. Use `gh pr edit 241 --repo mcoder1001-cyber/NGFW --body-file <file>`, or, if that errors, `gh api -X PATCH repos/mcoder1001-cyber/NGFW/pulls/241 -F body=@<file>`.
  2. Check it: `gh pr view 241 --repo mcoder1001-cyber/NGFW --json body --jq '.body|[test("@@N@@"),test("unbound-nics-wip.md"),test("decision log D-250"),test("D-251")]'` must print `[false,false,false,true]`.
  3. Record the actual result in the WIP. Until then, change report `:177` so it no longer says "#7 PR body" is handled.

**N2. MINOR: the WIP says six tests fail on red, but the pasted red output shows five.**
- **Where:** `docs/status/tasks/agent-allifs-unbound-nics-wip.md:6` ("red with the pre-fix line put back — the six tests fail").
- **Evidence:** `red.log` and report `:19-80` have five `--- FAIL`. `TestProjectAutoBlockOverlayAddsNoNetdevLookups` passes in the red run. That is expected: it guards the lookup count, which the one-line revert does not change. Still, the claim does not match the evidence.
- **Fix:** "five of the six fail; `…AddsNoNetdevLookups` guards the lookup count and passes either way". Add the same note under the red block in the report.

**N3. MINOR [shared with R6]: the new user-doc sentence says *All interfaces* covers more than the code does.**
- **Where:** `docs/user/security/global-blocking.md:27`: "*All interfaces* means every interface the data plane has …".
- **What the code does:** the target set is `sortedKeys(kept)` (`apps/agent/internal/desired/global_blocking.go:117`), which is top-level `interfaces` rows only. VLAN sub-interfaces (`desired/interfaces.go:183`) are separate VPP interfaces and are never targeted. R2 MINOR-4, R4 O-2 and this PR's own row at `docs/tech-debt.md:228` say so. That row even offers "state the limitation in … `global-blocking.md`" as its fix.
- **Scenario:** an operator relies on *All interfaces* or auto-block on a routed VLAN sub-interface, and listed sources are forwarded there.
- **Fix:** "every top-level interface the data plane has (VLAN sub-interfaces are not covered yet)".

**N4. MINOR: the two records of the #238 hand-over disagree about the user-doc paragraph.**
- **Where:** report `:152-154` says the exception paragraph is "replaced (not dropped …)" with R4's text, the parenthetical goes, and case (e) is reworded. `009f9a423` `boot-nic-registration-port-wip.md:16-17` (#238's own recovery file) says "the user-doc exception removed" and does not mention case (e) or the parenthetical.
- **Scenario:** the #238 session follows its own WIP and deletes the paragraph. `main` then loses the only user text saying that explicit references to an awaiting-engine or released NIC refuse commits. R2 MINOR-3 said "Do not simply delete the #238 paragraph".
- **Fix:** this branch cannot edit #238's files. The developer posts one comment on PR #238 pointing to report `:152-154` (link pinned to the final SHA), or the manager relays it. Record which was done in the WIP.

**N5. NIT: the envelope is stale.** `agent-allifs-unbound-nics-envelope.md:8` still says the #238 texts are updated "once this PR has a number (replacement text in the PR description)". Point it to the decided order and to the report's Out-of-scope texts.

**N6. NIT: the agent-gate paste has an unmarked elision.** Report `:94` says "Pasted unedited; the `ok` lines … elided as marked", but the 166 `?  … [no test files]` lines and the final `make: Leaving directory …` line are also left out without a mark. Extend the comment at `:109`.

**N7. NIT: the five new tech-debt rows have no heading.** `docs/tech-debt.md:226-230` follow the "## App lazy-route navigation test occurrence — 2026-10-07" section with no heading of their own, so they render as part of it. #238 appends at the same spot the same way. Also, `:226` cites "the PR #238 row …", which is not on `main` until #238 merges. Optionally add a heading such as "## Block lists and auto-block with unbound and released NICs (PR #241) — 2026-10-17", and add "(on PR #238's branch until it merges)".

**Observation (not a finding).** `gh pr checks 241` reports no checks, and the PR is `CONFLICTING`: `main` moved to `3f099b530` (#240, which adds D-249 at the same `LOG.md` spot). The WIP's plan to rebase after #242 and re-run the hosted gate covers this. D-251 stays free.

### Report claims checked against the logs

- **Timeline from the reflog:** `0b57dc070` was committed at 11:52:50 and pushed at 11:52:52. The red run was at 11:53:14, the agent gate ran 11:54:27–11:56:32Z, and the quick gate started 11:57:04 (wall time 5m09s). `873f1d946` was committed at 12:03:06. So nothing was committed while the gates ran, as the report says.
- **CI log directory** `/root/ngfw-wt/logs/ci/funny-meninsky-c9ef19-20261017-115704-1958055`: `ci-quick-2.log:3` shows branch `@ 0b57dc070`. `09-agent.log` has 143 `ok`, 0 FAIL, and builds with `-X main.version=0b57dc070`.
- **Gated tree vs head:** `git diff --stat 0b57dc070 873f1d946` shows two status files only, so the product tree at the head is the gated one.
- **Cited failure lines:** `global_blocking_test.go:266`, `:303` and `physical_nics_acl_test.go:108`, `:165`, `:226` are the failing assertions shown in the red output.

### Commands run (in `/root/NGFW/.claude/worktrees/funny-meninsky-c9ef19`, read-only)

```
$ git rev-parse HEAD
873f1d946a0c8e65f08603db4891025560bcaa66
$ for r in <17 origin refs>; do git show "$r:docs/decisions/LOG.md" | grep -o '| D-2[5-9][0-9] |'; done
origin/archive/deploy-hosts-minors-20261017-stacked: | D-250 |
origin/claude/agent-allifs-unbound-nics: | D-251 |
origin/claude/boot-nic-registration-port: | D-252 |
origin/fix/deploy-hosts-minors-20261017: | D-250 |
(origin/main: D-249 only, from #240)
$ awk 'NR==7||NR==287{n=gsub(/\|/,"|"); print NR": pipes="n}' docs/decisions/LOG.md
7: pipes=8
287: pipes=8
$ git show 009f9a423 -- docs/status/tasks/boot-nic-registration-port-wip.md | grep 'Merge order'
+this branch; PR #240 keeps D-249, #242 D-250, #241 D-251). Merge order: #240 → #242 → #241 → #238; the R4 agent fix lands as
+PR #241 before this branch, so the R4 debt row is closed with a pointer to #241 and the user-doc exception removed once #241 is
$ git show origin/fix/deploy-hosts-minors-20261017:docs/status/tasks/deploy-hosts-minors-20261017-wip.md | sed -n 9p
Decision: D-250 … (PR #241 also added a D-250; the manager ruled on 2026-10-17 that the merge order is #240 → #242 → #241, so this branch keeps D-250 and PR #241 renumbers).
$ git merge-base --is-ancestor c2f898d2b origin/claude/boot-nic-registration-port && echo ANCESTOR-OF-238-HEAD
ANCESTOR-OF-238-HEAD

# PR description vs drafts
$ gh pr view 241 --repo mcoder1001-cyber/NGFW --json updatedAt,headRefOid,body --jq '.updatedAt,.headRefOid,(.body|test("@@N@@")),(.body|test("D-250")),(.body|test("D-251")),(.body|test("agent-allifs-unbound-nics-wip.md")),(.body|test("\\*\\*running\\*\\*"))'
2026-10-17T12:09:43Z
873f1d946a0c8e65f08603db4891025560bcaa66
true
true
false
true
true
$ python3 -I -c '<compare live body with scratchpad pr-body*.md>'
pr-body.md False / pr-body-1.md True / pr-body-2.md False / pr-body-3.md False / pr-body-4.md False / pr-body-5.md False
$ gh pr view 241 --json comments → (none);  gh pr checks 241 → no checks reported on the 'claude/agent-allifs-unbound-nics' branch
$ gh pr view 241 --json mergeable → CONFLICTING;  gh pr view 238 → OPEN 009f9a423;  gh pr view 242 → OPEN 083a7ce87 MERGEABLE

# Pastes vs logs (scratchpad, read-only)
$ diff red.log <report :18-80>      → only "0a1 > $ env -u NGFW_INTEGRATION go test …" (the command line)
$ diff green.log <report :86-89>    → only the command line
$ grep -E '^--- (FAIL|PASS)' red.log | wc -l → 5 FAIL (…SkipsUnboundAndReleasedNICs, …NothingBound, …OverlaySkipsUnboundAndReleasedNICs, TestAutoBlockWith…OnFake, TestAllInterfacesBlockList…OnFake)
$ <every pasted gate line> grep -qxF agent-gate-2.log → all present
$ awk '<lines between go test -race and END>' agent-gate-2.log | awk '{print $1}' | sort | uniq -c
    166 ?
    143 ok
$ grep -E 'NGFW_TIMING|AGENT_GATE_EXIT|issues' agent-gate-2.log → 11:54:27.898 … 11:56:32.851, "0 issues.", AGENT_GATE_EXIT=0
$ diff <(sed -n '88,114p' ci-quick-2.log) <(sed -n '119,145p' report) && echo SUMMARY-IDENTICAL
SUMMARY-IDENTICAL
$ grep -c '^ok' 09-agent.log ; grep -c -E '^(FAIL|--- FAIL)' 09-agent.log ; tail -3 09-agent.log
143 / 0 / go build … -X main.version=0b57dc070 …
$ git reflog (HEAD, origin branch): 0b57dc070 commit 11:52:50, push 11:52:52; 873f1d946 commit 12:03:06, push 12:03:09
$ git diff --stat 0b57dc070 873f1d946
 docs/status/tasks/agent-allifs-unbound-nics-wip.md |  19 ++-
 docs/status/tasks/agent-allifs-unbound-nics.md     | 179 +++++++++++++++------
$ git diff --name-only 99a774734...873f1d946 | grep -v <owned paths> → (no output)

# Code facts behind N3
$ grep -n 'allIfs = sortedKeys(kept)' apps/agent/internal/desired/global_blocking.go
117:			allIfs = sortedKeys(kept)
$ grep -n 'GetSubinterfaces' apps/agent/internal/desired/interfaces.go | head -1
183:		for _, id := range sortedKeys(itf.GetSubinterfaces()) {

# Targeted tests (allowed), from apps/agent
$ env -u NGFW_INTEGRATION go test -count=1 -run '<six new tests>|TestGlobalBlocking|TestProjectPhysicalNic' ./internal/desired/ ./internal/agent/
ok  	ngfw/agent/internal/desired	5.231s
ok  	ngfw/agent/internal/agent	0.379s
exit=0
$ (six new tests, -v)
--- PASS: TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs (0.01s)
--- PASS: TestGlobalBlockingAllInterfacesNothingBound (0.00s)
ok  	ngfw/agent/internal/desired	0.185s
--- PASS: TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs (0.01s)
--- PASS: TestProjectAutoBlockOverlayAddsNoNetdevLookups (0.00s)
--- PASS: TestAutoBlockWithUnboundAndReleasedNICsOnFake (0.08s)
--- PASS: TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake (0.05s)
ok  	ngfw/agent/internal/agent	0.311s
$ git status --short | wc -l
0
```

Verdict: **APPROVE WITH CHANGES** (new: 0 BLOCKER, 1 MAJOR, 3 MINOR, 3 NIT; round 1: 12 fixed, 2 partially fixed, 1 not fixed, with all three leftovers folded into N1). The merge condition is N1: the PR #241 description is published to match the head, and the `gh` check above prints `[false,false,false,true]`. The manager may confirm that directly. N2–N7 are optional.
