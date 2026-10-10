# Hardware installation WIP — 2026-10-10

Branch: codex/hardware-manager-20261010.
Base/local initial SHA: d2d55984d. Published/read-back manager checkpoint:
cc80be66edcfa68521d85db2ad8dd1fa47c6820d; successful CLI push, local/remote matched.
Owned files: hardware-manager-20261010 envelope/WIP and external build output.

Completed: fetched GitHub, inspected main/branches/open PRs/CI and board. No open
PRs. Board validates 212 tasks, 205 merged, 7 parked, zero running/ready. Reviewed
main source has prior complete hosted quick PASS37903333143 plus three fixture
workflows; this does not establish target appliance acceptance. Three workers are
observably live in this task (two preflights and independent reviewer).

Actual read-only tests: BatchMode strict-known-host SSH to both targets PASS;
management IP/default routes identified. No NGFW/VPP packages or services present.
Both have ext4 /dev/sda2 corruption; boot fsck says invalid extent inode259596 and
UNEXPECTED INCONSISTENCY, active checksum/journal I/O errors. Independent reviewer
confirmed the same failures. No target packages/configuration/services changed.

Data inventory: .37 seven nonmanagement Intel igc ports; .211 seventeen ports,
including existing bridge memberships that require careful topology handling.
Physical af_packet import is rejected by current product (D-105); DPDK must use
actual IOMMU isolation and explicit management exclusions after recovery.

Current failure: both root filesystems require offline repair; console/rescue
access requested asynchronously, no answer yet. Reboot could stop at emergency mode
and lose SSH. Neither target has been changed.

Published host handoffs: .37 86a2f06eafc6406e3e5395769d923a09bbfa60aa;
.211 a4059c73b4a2e3346b7cf1cf705c0a5b908c317d. Both workers finished and are
awaiting resume, not active. Independent review d2f3473d94101945daf86d4f84beee8d6df2ea30
blocks installation/reboot and approves unchanged-host artifact preparation. Reviewer
independently passed 25 Python fixtures. Its Go subprocess printed success but stalled
afterward and was stopped; that command is not recorded as a completed PASS.

Build completed from clean cc80be66edcf (product files identical to current main and
hosted green candidate 30de26ee; GitHub run37903333143 independently read back success).
VPP full verifier passed all72 tests, manifest/control fields/all11 hashes and install
gate. pnpm frozen install604 packages passed. Fresh API/web14/14 build tasks passed,
including bundle budget and absence of development routes. All prepared Go helpers
built and API production deploy completed. Source payload prepared successfully at
/root/Documents/Codex/2026-10-10/hardware/package-source, version0.1.0~dev+cc80be66edcf.
This is prepared source, not yet built/inspected .deb artifacts or target acceptance.
Logs: same directory's pnpm-install.log and package-prepare.log.

Proposed dataplane-{37,211}.json documents in external hardware output accepted by
current DataplaneSchema and independently reviewed: exactly7/17 data devices,
matching whitelists, unique names, correct management exclusions and singleton groups.
Unapplied; interfaces physical rows and authenticated commit/live evidence remain.

Additional build limitation: root free space around1GiB, tmpfs2.6GiB. Use task-owned
tmpfs staging for dpkg duplication. No other agent's artifacts will be deleted.

Remaining: .deb package build and independent inspection; offline disk recovery;
reviewed guarded install, persist all data NICs, real acceptance and reboot persistence.
Target installation/firstboot/API/dataplane/packet/reboot acceptance all NOT RUN.
Initial dpkg-buildpackage -us -uc -b completed exit0 with all41 unchanged fixtures
passing. Four .deb artifacts preserved in external runtime/ with buildinfo/changes
and SHA256SUMS. Independent archive review found a real metadata failure: API native
argon2 computes libc6>=2.34 and libgcc-s1>=4.2 in shlibs:Depends, but API Depends
omitted that generated field. Initial artifacts are BLOCKED, not ready for install.
Corrected the API control field to consume shlibs:Depends; no compiler sources or
tests changed. Root owns this one control-file correction to unblock the requested
installation preparation. Required independent source/archive review and unchanged
hosted quick must pass before any merge. Targets still unchanged and disk-blocked.
Next: commit/publish correction, rebuild clean corrected source into a new task-owned
output, inspect final native control and helper receipts, publish reviewable PR and
verify unchanged hosted quick. Offline recovery still requires owner console input.
