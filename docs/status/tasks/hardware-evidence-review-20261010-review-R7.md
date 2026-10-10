# R7 docs/evidence review — PR217

Independent reviewer `/root/evidence_review`, own worktree
`/root/ngfw-wt/hardware-evidence-review-20261010`, branch
`codex/hardware-evidence-review-20261010`. Reviewed PR217 candidate
`bde83bc87ae817a61bbc70e4029f76109ae77c35` against main
`d2d55984d74fa1d06c32e8271886f11f16375407`. No product, other worktree, board,
shared VPP or target was modified by this reviewer.

## Findings

**BLOCKER R7-1 — completed check claims lack committed command/output evidence.**
Location: `docs/status/tasks/hardware-manager-20261010-wip.md:34`, continuing
through corrected-build evidence at lines43–47. The WIP asserts completed VPP72
tests, frozen install, 14 build tasks, seven helpers, API deployment and 41 package
fixtures using prose and an external log-directory path. It contains no command/
output selection or link to a committed evidence appendix. A reader recovering
solely from this PR cannot inspect those results in the repository status.

Fix: commit an owned evidence appendix with exact commands, observed exit status
and selected real output for completed checks; link it from the WIP and link the
immutable independent receipts. Keep native archive inspection and final-head
quick pending until completed. A PR comment/body may supplement the required
committed status/evidence receipt. Manager was notified and agreed to add evidence.
This is the explicit R7 status requirement, not an allegation of failing tests or
false completion of pending work. Recheck only the documentation delta/new SHA.

No other R7 BLOCKER/MAJOR/MINOR/NIT findings. The narrow metadata correction is
necessary to installation and declared in the envelope. No always-PENDING change
was silently decided, board/user API/UI/hardware rules were not changed, and no
new user documentation is needed for this one-field correction. Proposed NIC
configurations are clearly unapplied; hardware/install/reboot/release acceptance
is correctly NOT RUN. The interrupted Go parent exit143 is correctly excluded
from completed PASS results.

## Actual reviewer commands and output

Commands ran in this reviewer's own worktree. Published receipt/log inspection is
not represented as independent execution of the original tests.

```text
gh pr view 217 --json headRefOid
{"headRefOid":"bde83bc87ae817a61bbc70e4029f76109ae77c35"}
git diff --stat origin/main...bde83bc87ae817a61bbc70e4029f76109ae77c35
4 files changed, 121 insertions(+), 1 deletion(-)
git log --oneline origin/main..bde83bc87ae817a61bbc70e4029f76109ae77c35
bde83bc87 fix(packaging): declare native API library dependencies
git diff --check d2d55984d74fa1d06c32e8271886f11f16375407 bde83bc87ae817a61bbc70e4029f76109ae77c35
[no output; exit0]
git ls-remote origin refs/heads/codex/archive-hardware-manager-20261010 refs/heads/main
2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c refs/heads/codex/archive-hardware-manager-20261010
d2d55984d74fa1d06c32e8271886f11f16375407 refs/heads/main
git rev-parse 2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c:deploy/debian/ngfw/debian/control bde83bc87ae817a61bbc70e4029f76109ae77c35:deploy/debian/ngfw/debian/control
dbf2c0d12f880da0c5b4ffde19711c7f1d4fca9e
dbf2c0d12f880da0c5b4ffde19711c7f1d4fca9e
gh run view 37903333143 --json status,conclusion,headSha
{"conclusion":"success","headSha":"30de26ee6a5697a3713fe4375399b86c5588a472","status":"completed"}
gh run view 38033113750 --json status,conclusion,headSha,jobs
headSha=bde83bc87ae817a61bbc70e4029f76109ae77c35; status=in_progress; conclusion=""
python3 tools/board.py --help
board ok: 212 tasks; progress 98.4% by hours, 205/212 merged; ready=0 running=0 parked=7
tools/ci.sh check --base origin/main
check PASSED (0m14s)
```

`git diff --name-only` shows only control plus task envelope/review-plan/WIP; only
WIP/review-plan differ from archived2045ab8. The sole product line consumes existing
`${shlibs:Depends}`, preserving Node22 bounds. Rules/tests/CI/compiler code are
unchanged. Remote archive proves reviewed history survived the final one-commit
squash. Historical green is not final-head green. Board validates and is unchanged.

Actual selected log output independently read using `cat`/`sed`/`rg`:

```text
package-prepare-fixed.log:
ok   tests/run.sh: 72 passed, 0 failed
verify.sh: OK
Tasks:    14 successful, 14 total
Prepared source package: /dev/shm/ngfw-hardware-package-fixed-20261010
package-build-fixed.log:
source version 0.1.0~dev+2045ab8b3d2f
test_packaging.py: Ran 16 tests in 1.144s; OK
test_system_identity.py: Ran 12 tests in 0.431s; OK
test_firstboot.py: Ran 6 tests in 20.381s; OK
test_base_policy.py: Ran 3 tests in 0.001s; OK
test_pppoe_assets.py: Ran 4 tests in 0.319s; OK
dh_installsystemd --no-enable --no-start
```

This corroborates41 fixture passes during the running build, not final native
archive completion. Old runtime manifest explicitly says archive_review BLOCK for
missing dependencies and target_acceptance NOT RUN. README agrees. Original
commands were not independently rerun by R7. Exact worker receipts inspected:

- [R1/T1 pending source/fixture receipt](https://github.com/mcoder1001-cyber/NGFW/blob/4c2ac762f316a4147c008f4fc79c775dc1c1776d/docs/status/tasks/hardware-37-20261010-review-R1.md).
- [R2 exact final-head approval](https://github.com/mcoder1001-cyber/NGFW/blob/da9426156320e5a95fd1b26351af43bb102a5f1a/docs/status/tasks/hardware-211-20261010-review-R2.md).
- [R8 old-artifact BLOCK/source-only APPROVE](https://github.com/mcoder1001-cyber/NGFW/blob/3009a40ead3d73e0c1ed277d334cfc739f2d67a7/docs/status/tasks/hardware-review-20261010-artifact-review.md).

Both host receipts corroborate ext4 corruption/failed boot fsck, protected ports
enp12s0/enp4s0 and exact7/17 data NIC inventory. These are published observations,
not fresh target operations by R7. Offline console/recovery and clean filesystem
remain required. Full install/API/TLS, physical forwarding and reboot are NOT RUN.

Operational collaboration inventory observed root, host_37 R1/T1, install_review
R8 and this R7 reviewer running; host_211 had ended its R2 handoff. Those roles
are separate from board counts and hardware installers awaiting offline recovery.
Transient chat-agent presence proves no persistent runner/service.

**Verdict: BLOCK** for candidate `bde83bc87ae817a61bbc70e4029f76109ae77c35`, solely
for R7-1. Final quick/native archive reviews remain separate merge prerequisites;
their pending status is honestly described, not an additional R7 defect.
