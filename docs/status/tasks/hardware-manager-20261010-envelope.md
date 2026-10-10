# Hardware installation task envelope

Owner request (2026-10-10): install NGFW packages on root@172.30.126.37 and
root@172.30.110.211, test them, preserve management and routing, add the remaining
physical interfaces, reboot if necessary.

Base: origin/main d2d55984d. Branch: codex/hardware-manager-20261010.
Worktree: /root/ngfw-wt/hardware-manager-20261010.
Owned files: docs/status/tasks/hardware-manager-20261010*,
deploy/debian/ngfw/debian/control (native API dependency correction), and external build output/status plus task-private recovery backup parent.
Host workers exclusively own recovery-private/host-37 and recovery-private/host-211.
Configuration snapshots remain0600/private and outside Git/downloadable artifacts.
Do not edit main, another agent's worktree, shared development host VPP or secrets.

Workers: host_37 owns its isolated hardware-37-20261010 branch/worktree and .37;
host_211 owns hardware-211-20261010 and .211; install_review owns an independent
review branch and reports only. One worker per target; reviewer makes no host changes.

Recovery gate: both targets have independently confirmed preexisting ext4 root
corruption and failed boot fsck. No package installation or reboot until an offline
recovery path is available and root filesystem is clean. Console/recovery access
requested from owner. Do not repair a mounted root filesystem.

Management exclusions: .37 enp12s0, 172.30.126.37/24, default172.30.126.1;
.211 enp4s0, 172.30.110.211/24, default172.30.110.1. Management PCI devices and
their IOMMU groups must stay outside DPDK. Import data ports through persistent
product configuration. Existing physical af_packet import is unsupported by D-105.

Acceptance: package install/configuration; protected management/default routes and
fresh SSH; service/API/TLS health; persistent data-interface inventory and real
forwarding tests; restart/reboot only with recovery available. Record actual failures
and unexecuted tests accurately. No throughput or aggregate release claim.

Final preparation: PR217 merged4908716b/treea0d7b7, fullunchanged mandatory source
and baremain gates PASS; all applicable independent reviews APPROVE. FinalT1remote
4a8a82205d490068d865a1344d86afcaf93b8502; R7recovery0eb7036450d410b7540e05a786205c72bd814acf.
Owner follow-up (2026-10-10): fix the disk problem. This authorizes corrective
filesystem recovery within the original management/routing protection constraint.
Active roles after verified resume: root recovery manager; host_37 and host_211
exclusive host investigators/operators; evidence_review independent recovery reviewer.
No persistent installer is claimed. Investigate a verified RAM rescue path before
requiring physical console: independently reviewed reversible staging, authenticated
SSH test, unchanged network, executable nextroot, and block-device-exclusive plus
all-namespace checks proving the old root is no longer mounted. Chroot or lazy detach
alone is insufficient. No automatic destructive repair or mounted-root fsck.
Configuration copies are not full data backup. A metadata snapshot/undo strategy
must disclose its limits and preserve available evidence before corrective writes.
Hardware goal remains unfinished; installation follows successful root recovery.

Owner follow-up after actual SMART results (2026-10-10):
"تعمیر کن فرسودگی مهم نیست. این مشکل روی کدام ماشین است؟"
Meaning: repair it; SSD wear is not a blocker. Root clarified: wear126%/two
remapped blocks pertains to172.30.126.37; filesystem corruption affects BOTH.
The owner explicitly accepts that wear for logical filesystem recovery. Do not
ask again for replacement/repair permission or park .37 for wear alone. Preserve
scoped backups/metadata/undo and verify offline root as planned. New actual media
read/uncorrectable/reset failures still require diagnosis before corrective writes.
Original package installation/testing objective remains active after both repairs.

Current coordination update (2026-10-10 12:01 UTC): actual offline logical repairs/completeclean
checks and durable rawundos bothPASS, both kernels returned withoriginal22. Whole
37normalacceptance pending;211packages configuredPASS. Root manager exclusively
owns real physicaldriverbinding plus guarded async startup --apply, coordinated
with the host worker and reviewed exact whitelist/rendering/rollback evidence.
Workers retain ownhost recovery/install/runtime/seed/readiness/acceptance work and
ownprivate receipt directories. Rootmanager writes only ownprivate parent receipts
and ownedtaskfiles; never edits foreign worktrees. Historical initial console
request above was superseded by observed/reviewed matchingRAM recovery; owner
authorizes repair/install/test/reboot and accepts37wear without repeated approval.
Active chat workers:host37+host211; R7evidence_review independent reviewer.
Departed install_review is not counted live; no persistent supervisor claimed.
