# Project status integration

Owned branch: codex/project-status-merge-20261003. Worktree: .scratch/status-integration. Speculative base: reviewed PR135 head08adbb696; actual merge remains required before board completion. Original published PR102 source checkpoint02efcf0c is preserved, its six unit tests and read-only helper are ported unchanged. Three independent source reviews approve the helper.

Owned files: tools/project-status.py, tools/tests/test_project_status.py, this report and envelope; after actual PR135 merge, scoped plan/tasks.yaml and hardware integration result/status records. No product code or CI configuration changes.

The helper reports board state and estimated-hours progress, local Git refs/contributors and explicitly unverifiable live workers. It never fetches or treats running rows as a worker inventory. Historical156-row figures from the original report are superseded by fresh current-board observations. Current board has211 rows; scoped completion states will change only after actual merge. Full unchanged hosted current-main gate still required for PR102 final integration.

Next: run six unit tests and text/JSON helper; publish checkpoint, wait PR135 merge, integrate latest main and record actual7closures, independently review final delta, run unchanged gate and merge with expected head.
