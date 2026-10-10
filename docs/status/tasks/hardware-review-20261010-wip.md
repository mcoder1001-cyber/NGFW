# Hardware independent review WIP

- Branch: `codex/hardware-review-20261010`; worktree `/root/ngfw-wt/hardware-review-20261010`.
- Base/local starting SHA: `d2d55984d74fa1d06c32e8271886f11f16375407`. Receipt checkpoint SHA is the commit containing this file; exact publication SHA will be reported after successful push.
- Remote: reviewer branch not yet published at this checkpoint; do not infer publication from a local commit.
- Owned files: `docs/status/tasks/hardware-review-20261010*` only.
- Completed: independent source review; read-only inspection of both hosts' root filesystem, PCI management exclusions, network/routing state, OS and cached dependency candidates; recovery/deployment prerequisite review written to `hardware-review-20261010-review-R8.md`.
- Actual test results: packaging 16 PASS, firstboot 6 PASS, base policy 3 PASS; selected vppstartup management/host NIC Go test subprocess printed `ok` (`0.138s`), but the parent Go tool remains in post-test processing with final exit unverified. `tools/ci.sh check --base origin/main` exited zero, `check PASSED (0m21s)`. No target installation, activation, handover, repair, reboot, forwarding or full acceptance test ran.
- Current failure: both `/dev/sda2` root filesystems are mounted ext4 read/write with invalid extents, directory checksum faults and journal I/O errors; boot fsck explicitly requests manual repair. Existing SSH/default routes remain intact.
- Remaining: exact artifact check and post-recovery final safety/configuration review when manager provides evidence; target install/test remains dependent on console/rescue access and clean offline repair. No product code is assigned to this reviewer.
- Exact next command: commit the reviewed receipts and `git push -u origin codex/hardware-review-20261010`; final artifact/configuration review when requested by manager. Check final status of own Go PID 3445343, and stop that exact process if still stuck without a test child.
- Live role evidence: reviewer executing independent SSH reads and local tests during this session. This status is a checkpoint, not evidence of a persistent worker service.
