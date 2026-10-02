# NGFW agent execution and recovery

Owner instruction, 2026-10-02: keep development moving, give every agent its own branch, commit frequently, test and independently review, then merge ready work sequentially. Remove Telegram notifications. Laboratory-only acceptance may be deferred into `docs/status/DEFERRED-ACCEPTANCE.md`; real code failures and mandatory quick CI must pass before merge.

Read `prompts/00-CONTEXT.md`, `docs/contributing.md`, `docs/decisions/decision-policy.md` and the task prompt. Managers also read `prompts/MANAGER-PROMPT.md`. Owner instructions supersede historical lab-only blockers, not security or host privilege boundaries.

## Durable work

- One development task, one agent, one named branch and isolated worktree. Declare owned files; never edit another agent's worktree or main. Independent reviewers do not write product code.
- Commit each coherent change immediately; checkpoint unfinished work at least every 15 minutes and before handoff/interruption. Commit contracts before consumers. Do not commit secrets or falsely label unfinished work complete.
- Publish each checkpoint to that agent's GitHub branch immediately. A local commit is not a durable checkpoint. If CLI push lacks credentials, use the authorized GitHub connector; report the remote SHA and actual publication result to the manager. Do not expose credentials or claim publication before success.
- Maintain `docs/status/tasks/<task>-wip.md`: branch, remote/local SHAs, owned files, completed code, actual test results, remaining code, current failure and exact next command. Store the task envelope in the same directory. No knowledge essential for recovery may exist only in chat.
- Preserve reviewed history in archive refs/remote checkpoint branches before squashing. Never rewrite main. Follow D112 for final single-commit integration and expected-head merge checks.

## Manager recovery and merge queue

- At every start, inspect GitHub main/branches/PRs/CI and the board. Re-clone missing workspace files from remote checkpoints. Verify whether agents are alive; do not treat board `running` as proof of a live worker. Resume existing work instead of silently rebuilding it.
- Record live worker roles and evidence separately from task states. A departed worker is `awaiting resume` operationally until reassigned; never report it as active. Keep unfinished functionality explicit.
- Use available parallel agent slots for independent work/review; start the next ready task when a slot opens. An unavailable lab parks only actual lab execution, not host-independent development.
- Publish small PRs as soon as their coherent scope is reviewable; run the unchanged complete hosted quick gate. Fix real failures, obtain independent applicable reviews, and merge approved green PRs sequentially. If main changes, verify the new integration tree; do not merge a stale untested product tree.
- After each merge, check main CI and update the board/status in GitHub. Distinguish new product merges from correction of stale rows whose code was already integrated.
- Cloud chat agents are not a persistent service. Scheduled reports do not keep developers alive. The existing supervisor requires an explicitly provisioned persistent runner; do not claim one is running without observed process and checkpoint evidence.

Hourly reports: fresh task-state counts, verified live developers/testers/reviewers, PRs/new merges, CI, blockers and next action. Say `unverifiable` when a live-agent inventory is unavailable.
