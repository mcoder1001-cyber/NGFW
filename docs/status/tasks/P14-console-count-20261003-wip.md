# P14 decimal database count overflow

Isolated branch `codex/p14-console-count-20261003`, worktree `developers/P14-count`,
base `0a94b0c5` (separate from frozen PR122 and its lint repair).
Owned banner count comparison, console regression and this report.

Actual pre-fix offline reproducer: real banner against a temporary image/query hook
returned successful `18446744073709551616` (2^64) unused administrators; console
credential file was removed (`positive overflow credential retained: False`).
No host root/database/service/build was used and no secret printed.

Fix compares validated decimal text with all-zero pattern rather than Bash
arithmetic. Any positive count stays retained regardless of integer range or
leading zeros. Existing query failure/malformed-response guard is unchanged.
Regressions add >signed64, 2^64, leading-zero positive and leading-zero zero cases,
with successful ordinary-zero recovery after retention.

Root owns scheduled tests and publication. Exact focused command:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 deploy/image/iso/tests/test_console_recovery.py
```

Post-fix tests not launched by worker. Source inspection and diff-check only;
independent review/aggregate/CI remain required. Actual installed login acceptance
is still NOT RUN. Root may cherry-pick only this successor after PR122 lint repair.
