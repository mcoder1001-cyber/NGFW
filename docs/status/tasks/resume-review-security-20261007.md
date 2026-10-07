# Independent R2 review; scoped R1/R4 source-equivalence audit

Task envelope: independent reviewer, not an author of the proposed changes. Own only this file in `/root/ngfw-wt/resume-review-security-20261007`, branch `codex/resume-review-security-20261007`, base `0ec397e327123cadfd5d278a9a1cda37532fdc2c`. No product, board, main, shared-host or other worker edits. Read AGENTS, context, contributing, decision policy, REVIEW-PROMPT, R2/R4 prompts and shared-host/handover rules. User's exact one-file envelope supersedes generic review filename/WIP requirements.

## Exact reviewed revisions

- Current candidate: `c0922dcd9da509c7ca5b14f9121fb0ed9b6d75aa`, `origin/codex/resume-manager-20261007` against main `0ec397e327123cadfd5d278a9a1cda37532fdc2c` (main tree `943c5149b6daa4122d5fc7a345ec4505ab454916`). Six documentation/board files, 52 additions/15 deletions; no source or raw evidence changes.
- Original PR179: `883978a24df19f6d9d18854955372937dadafab7`, OPEN. This review approves the adjusted candidate above, not a stale PR179 tree.
- PR181: `4d85327789db65669e4de63a16f1616c2ca114ed`, OPEN. PR195 head `e45e0064c51852a1f742ed3c81bda7781c3f604a`, merged as main `0ec397e327123cadfd5d278a9a1cda37532fdc2c` at 2026-10-07T06:48:39Z.
- PR180: `661cbc21bf4bead10abb93333f493406f4142db1`, OPEN; current PR body retains failed mgmtd startup at unchanged 30-second deadline before 200-route criteria. P12 acceptance is outstanding.

## Verdict: APPROVE (current candidate, scoped R2 + AutoBlock R1/R4 equivalence)

No BLOCKER/MAJOR in the reviewed security/equivalence scope. Candidate closes bounded original MPLS/SRv6 H1 and corrects LAB-vpp-per-slot source status already integrated by PR195. It explicitly retains mixed-component limitations, earlier campaign HTTP504 failures, all four later failed attempts (1/2/7 health connection, 4 browser evaluation), and separate P12/OSPF cleanup/acceptance work. It does not assert all 13 attempts passed, whole-appliance certification, or universal reliability. Broader acceptance remains open. No security boundary, socket privilege, daemon ownership or shared-host mutation is authorized by these docs.

Necessary adjustments from PR179 are present: identify actual PR195 integration/current main quick; replace obsolete running-gate/old-main text; retain failures with bounded zero504 reporting; close per-slot source only; leave P12/OSPF and TD19 target installation outstanding. No additional R2 edit is required in candidate c0922dcd. This is not permission to merge without required current-candidate hosted quick, applicable R7 review, D112 history preservation and expected-head sequential checks. Main's green run does not validate the candidate.

[other: R7, advisory] `docs/status/tasks/resume-manager-20261007-wip.md:3,11` still contains initial checkpoint wording and old board counts; PROGRESS also has older baseline inconsistencies. Manager should reconcile publication/task-state reporting for final checkpoint. Worker liveness inventory is unverifiable from this reviewer; spawning records are not an independently observed persistent runner. No liveness or aggregate-progress certification here.

## AutoBlock equivalence and dependency separation

Executed `git rev-parse origin/main:<path> origin/review-pr181:<path>` for each path; identical Git blobs:

| Path | Both blob hashes |
|---|---|
| apps/agent/internal/agent/rpc_autoblock.go | bfec514824e7d3adc2ae833ce3773434aaa62a49 |
| apps/agent/internal/agent/rpc_autoblock_test.go | 8ae3d1833e41f319efe65435279c318265c4a74b |

`git diff origin/main origin/review-pr181 --quiet --` those two paths: exit 0. Byte identity establishes these AutoBlock changes are already merged, not merely planned or deferred. Source inspection confirms owner and entry validation precede the transaction lock/no-op; no-op requires clean state, successful nonempty fingerprint, empty retained/input entries and disabled protection. First publication, dirty/cache replay, retained entries and enabled/nonempty cases keep enforcement. No new privilege boundary follows from the equality audit.

PR181 also differs in `apps/agent/internal/agent/p12_topology_integration_test.go` and P12 evidence. The complete branch is not equivalent to main and must not be integrated merely because AutoBlock is superseded. Preserve reviewed PR181 history/evidence when retiring duplicate source; PR180/P12 still needs corrected proof, applicable review and current gates. No P12 product correctness verdict or live cleanup reapproval here.

## Evidence boundaries and receipts

`gh run view 37583587580 --json conclusion,headSha,status,url` returned conclusion `success`, status `completed`, head `0ec397e327123cadfd5d278a9a1cda37532fdc2c`: actual complete hosted main quick, not a lab PASS. PR195 integration stat includes AutoBlock and per-slot source plus original evidence files.

Read committed `claude-autoblock-noop-wip.md` (SHA256 bb8704a29bd80b728a0c71ea9f2dc57481933185c24edb432144986e3e65447b): reports 13 slot14 attempts, zero HTTP504 and four failures. Warm-up changes launcher cache state from attempt11; fixture deadlines/assertions remain reported unchanged. Raw per-attempt replay logs are not in that committed evidence directory (it contains four helper scripts). Therefore this review verifies the candidate accurately attributes the committed report; it does not independently rederive all replay counts from absent raw logs or establish causal elimination of every historical failure. Campaign11/native13 historical receipts remain main evidence; no copying, rewriting or fresh appliance attestation.

Read original slot20 up/down receipts in main. `01-up.txt` shows owned ngfw-vpp-w20, own runtime/sockets, successful rc0; `06-down.txt` shows unit not-found/inactive, runtime removed, zero w20 shared-memory objects, no slot instances, and shared MainPID1014/NRestarts0/ActiveEnterTimestamp unchanged. Its process listing includes its own observation shell; do not treat that shell as a leaked VPP. HugePages_Free differs across the whole historical window (3999 before, 4020 after); do not claim exact whole-window equality. Bounded source/slot lifecycle closure does not certify all migrated harnesses or full opt-in CI-slot VPP execution. Root transient-unit capability is existing lab scope, not a new appliance privilege approval.

SHA256 of intact main slot20 receipts:

```text
78cd985b74f83f941bd88ba7f5b18e933a2a4381c89def86992b289072ce0f5a  01-up.txt
65adc1c4feb0d37a71b1d500617667e2833a7a9efa7ae3cc9bc1f77852d89b0e  02-running.txt
3d7d7104fa5a9b24b5794d9858b2b1d7a20276f756043f36fb4eec5cbf230b80  03-rig.txt
c47e5c608f5cf5dda446bb8118eeee6b3a57b666523f6ad6613a6ef41ce1485c  04-idle-cpu.txt
e894a730f4387aba71c2ee66a058d417073816800e8b12a52cd12855225ab4c4  05-core-integration.txt
0e661942665793b371e6be0a6cec5d7c5130e5b4cb18b9b68f585f80dfa1cd14  06-down.txt
```

TD19 trust decision/source integration does not establish target provisioning/install/boot or Ubuntu/NodeSource runtime acceptance; committed `claude-td19-wip.md` says NOT RUN. Candidate does not close TD19. Historical stale pending labels can be reconciled separately without converting NOTRUN to PASS.

## Actual lightweight checks and durable handoff

Commands executed in this worktree: candidate/original diff inspection; `git diff --check` on both proposals (exit0); gitleaks redacted history scan PR179 (1 commit, no leaks) and candidate (2 commits, no leaks); gitleaks directory scans committed AutoBlock helper evidence and slot20 evidence (no leaks). No heavy test, service/network namespace change or shared host mutation performed.

`tools/ci.sh check --base origin/main` output: contract guard no contract changes; no secret-shaped strings; gitleaks no leaks; trace/classify guards passed; board valid 212 tasks read-only; slot scheme 30 developer slots + CI12, 964 ports/32 ranges, no collision; `check PASSED (0m16s)`. This is check-only, not complete quick. Completed code: none (review only). Remaining reviewer product work: none. Publication: commit this report and push the assigned branch, then verify remote SHA; exact report SHA is resolved externally to avoid a self-referential commit hash. Next command: `git push origin HEAD:refs/heads/codex/resume-review-security-20261007`, then `git ls-remote origin refs/heads/codex/resume-review-security-20261007`. Manager next: current-candidate required gate/reviews and expected-head merge; re-review changed scope if candidate changes.
