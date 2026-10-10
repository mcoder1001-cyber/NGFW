# Hardware installation WIP — 2026-10-10

Branch: codex/hardware-manager-20261010.
Base/local initial SHA: d2d55984d; remote checkpoint: publication pending.
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
access requested asynchronously. Reboot could stop at emergency mode and lose SSH.
Additional build limitation: development host root has only1.7GiB free and tmpfs
2.6GiB free; no other agent's artifacts will be deleted. Verifying existing patched
VPP artifacts against current source before planning any build.

Remaining: offline disk recovery, current package payload, reviewed guarded install,
persist all data NICs, real acceptance and reboot persistence. All NOT RUN.
Next command: deploy/vpp/verify.sh --require-files
/root/Documents/Codex/2026-10-04/debian-latest/outputs/NGFW-Debian-latest-development/VPP-26.06-release+ngfw3-amd64
--install-gate --no-tests; then decide valid package preparation based on evidence.
