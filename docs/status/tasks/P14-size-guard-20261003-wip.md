# P14 strict disk size inventory

Branch `codex/p14-size-guard-20261003`, isolated `developers/P14-size`,
base `95ed63b5` final console successor. Root may cherry-pick only this new commit
after PR122; existing console fixes are inherited, not reimplemented.

Actual harmless pre-fix fragment reproducer: successful synthetic SIZE `1e300`
or 39-digit decimal caused awk scientific output; Bash conditional arithmetic
reported an error but continued (exit0) instead of minimum-size refusal.
No real inventory/early script/poweroff was run.

Production early guard now runs a read-only Python helper, embedded by the builder.
Checked lsblk completion precedes parsing; each row has strict unsigned64 byte count,
type and 0/1 removable flag. Python integer comparison requires at least one fixed
disk >=96GiB without scientific conversion or Bash overflow. Unsigned64 max is
representable; above-max, nonfinite/fractional/exponential and malformed rows fail.
No storage selector changes, host operations or installation are introduced.

Four regression methods execute the production helper with temporary stub lsblk:
boundary/fixed-vs-removable, failed partial inventory, malformed/overflow rows even
after a valid disk, and unsigned64 maximum. Aggregate adds one check.

Worker validation is source inspection and diff-check; post-fix tests NOT RUN.
Root scheduled commands:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 deploy/image/iso/tests/test_size_guard.py
bash deploy/image/iso/tests/run.sh
```

Independent review and root gates required. This is resilience P2 closure, not
actual signed ISO or VM acceptance. Python is already an installer dependency.
