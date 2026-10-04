# M-prompts — questions / owner actions

1. **Refused cleanup of `/tmp/g-w4` (owner/manager action).** The envelope asks to delete the CI `TMPDIR=/tmp/g-w4` before finishing.
   The command that did it (`rm -rf /tmp/g-w4`, chained after a status-file update) was refused by the permission layer on
   2026-10-01 ~18:00 UTC. Per the envelope a refusal is final: no other route was tried. The status-file update was then
   written with the editor tool instead. The leftover is small (`du -sh /tmp/g-w4` → 1.6M, tmpfs) and holds only that one
   gate run's temp files; nothing of this row depends on it. Please remove it, or add a permission rule for the worker
   cleanup the envelope prescribes.
