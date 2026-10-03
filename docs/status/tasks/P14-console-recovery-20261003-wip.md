# P14 uncertain database credential lifecycle regression

Branch `codex/p14-console-recovery-20261003`, base `54324568`, isolated
worktree `developers/P14-console`. Owned only new ISO test file and this report.

The existing suite covers failed queries without output and valid counts, but not
partial zero output with failed exit or malformed successful counts. New standalone
regressions run the actual bootstrap/banner scripts against temporary image roots
with the existing non-live-only database query hook. Uncertain responses must keep
credential and issue bytes intact, issue mode 0600 and logs free of the password.
Successful zero then removes the console copy, sanitizes the issue, changes mode to
0644, preserves HTTPS URL and permits idempotent subsequent checks.

Fixtures cover partial `0`/exit2, blank, negative, nonnumeric and multirow responses.
No database, host root, mounts, services or network are used. Production scripts and
existing renderer/manifest/aggregate files are untouched to avoid active scope overlap.

Actual worker validation: source inspection and `git diff --check` only. No tests
launched. Coordinator next scheduled command:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 deploy/image/iso/tests/test_console_recovery.py
```

Independent review/automated results remain pending; these regressions do not prove
real appliance first-login cleanup or signed ISO/VM acceptance.
