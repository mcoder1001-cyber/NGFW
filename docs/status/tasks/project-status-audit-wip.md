# Project status audit

Branch: `codex/project-status-audit-20261003`
Base/local starting SHA: `19aa88a5cdbe35954079c563fe5143dc18fa5dba`.
Remote checkpoint: not published yet; manager review precedes publication for this task envelope.
Owned files: `tools/project-status.py`, `tools/tests/test_project_status.py`, this status file.

Completed: read-only YAML task counts, progress by estimated hours and task count, unknown-state disclosure, local HEAD/branch and origin/main divergence, historical contributors deduplicated by case-insensitive author email. Live developer count is explicitly unverifiable; board running rows are not evidence of alive workers. No board/status regeneration, fetch, or shell execution is performed by the tool.

Validation: `python3 -m unittest discover -s tools/tests -v`: six tests passed. Covers weighted progress, unknown states, invalid estimates and duplicate task IDs, email aliases, missing Git/remote data and divergence direction. Text CLI and JSON CLI executed successfully; JSON parsed with `python3 -m json.tool`; `git diff --check` passed.

Observed snapshot: 156 tasks, 114 merged, 13 running, 10 ready, 2 parked, 17 todo; estimated hours 990 / 1342.5 (73.7%). Three historical author emails. These are local board/history observations, not verified live developers or remote freshness.

Full quick gate: `tools/ci.sh --base main` in progress; local main is divergent from the starting origin/main and includes unrelated contract changes in this comparison; no passing gate claim is made; log `/tmp/ngfw-project-status-quick.log`.
Remaining: independent manager/reviewer review, publication and hosted gate. No remaining product code identified.
Next command: `tail -n 40 /tmp/ngfw-project-status-quick.log`.
