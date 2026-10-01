# CI-remote-codex — R7 docs, evidence and scope review

Reviewed branch HEAD `d84c86ea` (workflow implementation `131c3aaf88a6b56603ba9c7cb48916877b14a830`) against `origin/main`. Scope: workflow, CI report, manager checkpoint, board/progress checkpoint and existing CI contributor documentation. Read shared context, REVIEW-PROMPT and R7 prompt. The manager envelope authorizes the actual worktree and hosted CI; older local-only conventions do not require owner reconfirmation.

## Findings

1. **BLOCKER — docs/status/tasks/CI-remote-codex.md:25–33 — validation claims lack pasted evidence.** The task report says actionlint, check and baseline generation passed, and describes a minimal pinned-Node UDS reproduction, but carries no command/output blocks, source commit or execution details. R1 does provide independent actionlint/check output tied to the implementation commit, so those two assertions are supported elsewhere; baseline generation and UDS reproduction remain prose-only in committed evidence. Scratch logs are not an auditable substitute. Fix: paste actual commands/output with versions, worktree and source SHA for each claimed phase; label baseline evidence explicitly as baseline rather than workflow success. Record the quick failure and relevant EPERM output, without promoting it to PASS.

2. **MAJOR — docs/decisions/LOG.md:170 / .github/workflows/ci.yml:3–18 — hosted CI decision not logged.** The branch introduces hosted execution with push/PR/manual triggers and cancels superseded runs, while the decision log ends at D-161. Implementing the existing thin-wrapper design is in scope, but selecting hosted execution rather than keeping local-only checks is a non-trivial operational choice. Fix: add an ordinary decision row with options (hosted quick/local-only/full lab), rationale, reversal cost and affected tasks. No new PENDING requirement is inferred: the owner authorized this work.

3. **MAJOR — docs/contributing.md:202–203; docs/status/tasks/CI-remote-codex.md:34–39 — hosted execution state is stale.** Contributor documentation says the repository has no remote and nothing runs the workflow. The task report says it has not run and is pending publication. The manager envelope identifies draft PR #57 and run 36923948061 as in progress. Fix: update the touched CI documentation with the actual publication/run state and link to the hosted run; include tested commit SHA and eventual conclusion when available. In-progress is not green and the original quick gate must still succeed before merge eligibility.

## Scope and checkpoint observations

- Workflow only invokes the unchanged quick gate. No product/generated/host configuration changes appear in this implementation commit.
- Board/progress changes belong to separately committed manager checkpoints and are explicitly authorized orchestration work. Five tasks are marked running; merged remains 105/156 and 70.4%, with no inflated completion claim.
- Manager checkpoint explicitly leaves live VPP acceptance pending. No premature full-quick or hosted-green claim was found.
- Existing R1/R2 reports review the workflow, not product readiness; hosted tester evidence remains pending.

## Independent evidence

Commands run in `/workspace/scratch/96b8b6fbc8a7/NGFW-ci`; PATH includes the task toolchain. No full gate rerun or product edits were performed.

```text
$ PATH=/workspace/scratch/96b8b6fbc8a7/toolchain/bin:$PATH actionlint .github/workflows/ci.yml
[no output; exit 0]
$ python3 tools/board.py --help
board ok: 156 tasks; progress 70.4% by hours, 105/156 merged; ready=10 running=8 parked=2
```

The board command validates the current board even when passed --help. Baseline scratch log inspection finds `clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated`, but that file does not associate the evidence with a source SHA and does not contain a final quick PASS; this review does not claim it independently reproduced generation or EPERM.

Initial verdict: **BLOCK** (1 BLOCKER, 2 MAJOR). Superseded by the verification below.


## Verification of original findings — d6b38ca3

Reviewed only the fixes to the original three findings at `d6b38ca3` (HEAD also includes independent R8 report `b32bbe28`). No product code edits or full gate rerun.

1. **Resolved:** Task report now pastes actual actionlint/check outputs, installed runtime versions, baseline gate header with source SHA `01bbee3d`, clean generated-output result and API EPERM excerpts. It explicitly distinguishes baseline Node 24.19.0 from installed Node 22.23.2 and the separate minimal Unix-listener reproduction, which is recorded with exit 1. Baseline generation success is no longer presented as exact pinned full-gate success. R1 independently supplies the actionlint/check invocation evidence.
2. **Resolved:** D-162 records hosted quick execution, alternatives, rationale, low reversal cost and affected tasks. This is an ordinary logged choice within the owner's authorization.
3. **Resolved:** Contributor documentation states hosted runs execute the existing quick gate and live integration requires the lab. Task report identifies draft PR #57, in-progress run 36923948061 and remote source SHA `46d95cb61cd8c9027620e955356723d5cc12ae8a`. It expressly requires original `CI GATE PASSED` before claiming green. No successful hosted run is claimed.

Optional documentation improvement: replace the descriptive `$ node Unix-listener reproduction (v22.23.2)` label with the exact minimal reproduction source/command, and add a clickable hosted run URL. Neither affects the recorded failure state or the independent hosted gate requirement.

Final R7 verdict: **APPROVE**. Original BLOCKER and MAJOR findings are resolved. Approval covers evidence accuracy and scope only; hosted quick must actually finish green before merge eligibility, and live appliance acceptance is still outstanding where applicable.

## Narrow capture-cleanup evidence recheck — 98a24642fc1476d9f35903cf9420640a393bc70d

Reviewed only the new capture cleanup hunk and appended hosted-run evidence. Task report accurately marks run 36925855433 / source d12adc535e35404ce011f55fe3b9dae1e80d3d98 as failed: 35/35 Turbo succeeded, then agent capture TempDir cleanup failed. Local artifact `/tmp/ci-remote-agent-failure.log` lines 159–162 independently matches the pasted `directory not empty` failure and package result. The patch joins canceled fixture workers with a bounded 5-second cleanup wait before TempDir deletion; existing behavior assertions are retained. Focused manager 100-count race output is labelled focused evidence, and the report explicitly requires independent review and a further complete hosted run. No complete-green claim is made.

**R7 documentation verdict: APPROVE** for this narrowly reviewed addition. Hosted complete quick remains pending/red until another actual run succeeds; this report does not infer full-gate approval from the focused race test. No product edits or full gate run by R7.
