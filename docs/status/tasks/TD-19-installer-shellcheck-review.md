# TD-19 installer ShellCheck/source-style review

**R1/R2/R7/R8: APPROVE bounded source/check delta** exact `cb186b45bbdf6e3dda34368d6122438bb410b96e`, independently reviewed 2026-10-02 in an isolated sparse worktree. Scope is the mandatory ShellCheck companion and changes atop `413d4265`; inherited repository-key changes from unmerged PR71 are not newly approved or authorized for alternate-path merge by this report.

## Source semantics and gate

Workflow now requires ShellCheck availability, prints its actual version and runs unsuppressed `shellcheck -x -P SCRIPTDIR` on the four intended source targets before strict fixtures. Existing full quick is unchanged. No new lint suppressions or exclusion gates were added. Actual first hosted failure is retained in WIP rather than described as passed; the latest hosted rerun remains separate required evidence. Runner-installed ShellCheck version is observed, not falsely described as a separately pinned download.

Quoted heredocs preserve deferred Go login HOME/PATH, remote Bash positional/OS variables and Python `${Version}` query literals as data. Command substitution removes terminal newlines only; reviewed commands receive the same effective bodies and separate quoted arguments. No local expansion of remote command variables or source interpolation was introduced. The transfer fixture extracts the actual new heredoc body, retaining original hash/control/policy/version assertions.

Explicit `if` conditions retain the original left-associative AND/OR fallback semantics, including success-command/logging failure triggering fallback. Slot refusal, rig teardown warning/ignore behavior and local verification success/failure paths retain their prior observable behavior; no new VPP commands, lifecycle or privilege changes. Two Bash newline substitutions preserve per-line leftover output. No original key/parser source or package format was changed by the style fixes.

## Independent actual execution

`python3 docs/status/tasks/TD-19-run-fixtures.py`: **36 PASS**, 8.873 s; zero failures/errors/skips/expected failures/unexpected successes; EXIT 0. The four new test methods exercise deferred heredoc output with hostile caller variables, literal profile output, actual extracted teardown function with harmless fake commands including success-log failure fallback and exact delete argv, and multi-line formatting. Existing fixtures remain meaningful; no assertion was skipped or lowered to accept failure. They are source/redirected fixture evidence, not target installation proof.

`bash -n scripts/00-add-repos.sh scripts/20-install-build.sh scripts/40-install-lab.sh tools/lab`: PASS. `git diff --check`: PASS. Local ShellCheck absent, therefore **NOT RUN**; no local lint PASS asserted. Hosted first-run diagnostic evidence is parent-provided; fresh final-head hosted ShellCheck and full quick are mandatory before merge. No actual remote SSH, APT, rig, service, VPP command or host mutation was executed. No full TD-19 completion or new approval of inherited PR71/security changes follows from this bounded review.
