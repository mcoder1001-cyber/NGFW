# Images repaired exact-head R2 review
Reviewed head: 6bd58ecbe10efb45a486139777655c6c826093cd. Delta against prior approved bf9fa4b25a62c4009229ab23c9d0a4480e507953 reviewed independently; prior approval preserved for unchanged scope.
No BLOCKER, MAJOR or MINOR security findings. Common mutation_root refuses /, aliased root or parent, absolute/traversing mutation paths and existing symlink components. grub_config preflights boot and boot/grub/grub.cfg before kernel enumeration and mutation. put refuses nonregular output. configure retains all-parent preflight. Static guard assumes an administrator-controlled build tree without hostile concurrent rename/mount writers; it is not a sandbox for attacker-controlled live filesystem races. Trusted signed pool and administrator build inputs remain the boundary.
Actual commands in reviewer worktree:
```sh
mkdir -p /root/.cache/review-r2-final/{images,tmp}
git archive 6bd58ec deploy/image test/topology/images deploy/debian/ngfw/assets/firstboot.sh | tar -x -C /root/.cache/review-r2-final/images
TMPDIR=/root/.cache/review-r2-final/tmp PYTHONDONTWRITEBYTECODE=1 tools/heavy.sh python3 -m unittest discover -s /root/.cache/review-r2-final/images/test/topology/images -v
```
Actual output: Ran 13 tests in 1.197s; OK, no skips. Includes real small-format conversion/comparison and external boot sentinels.
Additional python3 stdin fixture command loaded this exact image.py via importlib and exercised put absolute path, ../ escape, dangling ancestor link, directory output, host-root put, and management-interface newline. Each required ValueError; asserted external sentinel and directory contents unchanged. Actual output: 6 additional negative guards PASS; external sentinel preserved. Disposable paths only; no shared boot/config mutation.
Real release artifact construction, signed pool install, appliance boot, cloud provisioning, runtime identity/secret removal acceptance: NOT RUN. Full quick not independently run by R2. [other: R4/R8] these remain separate acceptance gates.
Verdict: APPROVE (security, exact head above).
