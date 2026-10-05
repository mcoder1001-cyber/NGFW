# Board publication repair

Observed main aebfc46f01a3cceab6750e2e0a8fbf333e796bef contains malformed
plan/tasks.yaml introduced by parent bdf6c9baa139a9afe49183e100d569c3ac22897a:
three literal output-warning header lines and a mid-file truncation marker
replace 516 lines. The published blob is 44a34006748bf3c4c430427631ce495cb49fb2ed.
Python YAML loading actually failed at the first warning header.

Restore the complete board from d314f0728f5f000f62f297d7030629d2fe1c6aa1,
then retain the exact legitimate F-backup-restore and TEST-traffic-B rows
from the external claim commit, including running state, branch, worktree,
owner, start time and verified-worker metadata. Before generation, semantic
comparison verified the same ordered set of all 211 tasks; only those two
rows differed, and only those six fields changed. No feature is premarked
merged; the existing HA status reconciliation remains a separate step.

Actual tools/board.py succeeded: 211 tasks, 192 merged, four running,
nine parked, zero ready and six todo. Full generator duplicate/dependency/cycle
validation passed; generated progress preserves both external worker claims.
No product code changed. Independent R2/R7 review and unchanged hosted quick
CI are required before expected-head merge. Publication checks compare every
blob SHA and the complete local/remote Git tree before moving the branch.
