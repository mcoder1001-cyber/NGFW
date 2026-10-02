# TD-19 installer companion PR76 — independent R4

2026-10-02. **APPROVE R4** for source composition
`a17d1bc6356b91836e7cd3167ee30095b439b74c`, tree
`d370d8dc74135bdf625da8c56a10c9620714d4da`, in isolated worktree
NGFW-installer-pr76-r4-review, branch task/installer-pr76-r4-review.
No product authorship. Report is the only owned output.
Manager identifies this as the local composition matching PR76; remote abbreviated
3d886c66 was not present in this object database and was not independently fetched
by this reviewer. Approval binds the exact local source/tree above; manager must
verify remote tree identity before attaching it as remote-head approval.

No R4 BLOCKER, MAJOR or MINOR found. Inspected tools/lab delta against actual
main base db15515d and source checkpoint cb186b45bbdf6e3dda34368d6122438bb410b96e.
Quoted heredoc command substitutions produce the same literal remote bash/Python
source without caller expansion. ${Version}, remote positional $1/$@ and loop
substitution remain deferred. VPP restart/kill remain under the same exclusive
VPP_LOCK then LAB_LOCK, including readiness wait; no new global operation or
host privilege is introduced. Provision package selection still travels as stdin
data and staging path as positional argument, not interpolated remote source.

AND/OR-to-if changes retain both command and success-log in the condition, hence
preserve fallback when the actual operation OR its log fails. Slot numeric/range
refusal, prefix resolution, exact anchored own-object matchers, root requirement,
pending-install refusal and command dispatch are unchanged. Deletion still uses
only the same owned names; quiesce delay and tolerant netns/veth races remain.
New newline substitution changes only leftover diagnostic formatting; leftover
failure/exit still occurs. No VPP API, Go/YANG descriptor, transaction ownership,
shared-host allocation or global privilege semantic change in this bounded delta.

Actual independent verification: `python3 docs/status/tasks/TD-19-test-shell-source.py`
ran 4 tests, PASS in 0.023s. Actual changed fragments run with harmless stubs,
including hostile caller variables, deferred login profile, three teardown
operation/log failure combinations and two-line leftover output. `bash -n tools/lab`
PASS. No redundant broad suite run. No SSH, provisioning, sysctl, apt, VPP, nft,
netns or global host execution occurred. Other reviewer broad fixture results and
main preservation are attributed to them, not claimed as this execution.

This narrow panel approval does not claim whole TD19 DONE or real host acceptance.
Merge still requires actual remote-tree identity, mandatory panel approvals and
unchanged complete hosted quick on the exact final head; preserve main key-parser
security and deferred acceptance boundaries.
