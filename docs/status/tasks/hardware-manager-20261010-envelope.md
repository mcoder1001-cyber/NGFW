Current authorized scope, 2026-10-10 16:08 UTC: ROOT remains sole host mutation owner on both targets. Existing host211 worker resumes source-only clock correction and R7 independently reviews; no unrelated task. Clock/RTC persistence and repeat normal reboot are necessary acceptance within the original installation/testing/reboot authorization. Preserve management/routing, TLS verification, desired timezone, existing chrony configuration and all original recovery records. ROOT also owns the narrow same-campaign addition to docs/status/DEFERRED-ACCEPTANCE.md for physical packet/throughput tests lacking carrier/peers; real clock/native failures cannot be deferred.

# Hardware installation task envelope

Owner resume, 2026-10-10 12:34 UTC (16:04 Asia/Tehran): complete this one
hardware task and mark it Done only after its required acceptance. Do not start
unrelated board work. The pause below is superseded. Root owns the necessary
setup documentation in docs/install/bare-metal.md. A small final integration
branch codex/hardware-firstboot-fix-20261010 is prepared by the same root manager
in an isolated own tmpfs worktree /dev/shm/ngfw-hardware-firstboot-integration-20261010,
because controller root has under0.5GiB free. This is D112 integration of the
same hardware correction, not another development task. Operational checkpoints
remain on the existing manager branch and worktree; no main history rewrite.
The old root-owned prepared build tree may be removed only after all four
immutable native archives and their exact hashes are verified as retained.
It is reproducible build cache, not recovery/private data or Git history.
firstboot fix files assets/firstboot.sh, assets/initial-dataplane.json and
debian/ngfw-meta.install under deploy/debian/ngfw, its tests/test_firstboot.py,
and apps/agent/cmd/ngfw-startupgen/main_test.go. Existing host211 operator and R7
reviewer resume within this same task; host37 remains paused until ordered phase.
No unrelated task is marked Done without actual reviewed complete evidence.

Owner pause, 2026-10-10 12:28 UTC: "تسکهایی که تا الان دان کردی رو اعلام کن و فعلا تسکت رو متوقف کن".
Task is explicitly PAUSED. All historical phase releases and next execution
commands are suspended until explicit owner resume. Workers acknowledged no
in-flight host mutation; only durable evidence checkpoints are being published.
Both logical repairs and all11 installed/configured package audits PASS.
.211 initial service/TLS start PASS but native seed remains revision0/FAIL;
missing loaded LCP/plugin dependency is observed and no correction applied.
.37 firstboot is NOT executed. Neither7 nor17 data ports are bound/imported.
See current paused wip/status for actual state, evidence and resume command.

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

Same hardware runtime correction ownership (2026-10-10 12:58 UTC): root also owns
apps/agent/internal/agent/rpc_autoblock.go and rpc_autoblock_test.go.
Actual agent restart fails on an ownerless empty persisted cache. Normalize only
accepted snapshots to the effective service owner; keep foreign-owner rejection.
Independent review and restart regression precede native fixed artifact deployment.

Root owns docs/decisions/LOG.md narrow D245 in own final integration worktree.
Operational root also owns hardware-manager-20261010-normalize-empty-cache-211.py
manager-only reviewed known-empty legacy cache recovery; no target execution yet.

2026-10-10 13:51 UTC: departed host37 worker cannot resume (agent thread limit).
Root assumes later .37 operational phase in OWN existing manager worktree,
owned hardware-manager-20261010-upgrade-four-37.py and later manager37-prefixed
source/evidence. Writes only task-private ROOT parent, never departed worker
worktree/private child. Existing37 evidence is read-only consumed by exact SHA.
Prepare adapted same four-archive upgrade preserving original policy+mask/all21
inactive units; no .37 package/firstboot/NIC changes until reviewed actual gates.

2026-10-10 same-campaign root additionally owns hardware-manager-20261010-data-preflight-37.py and its contract. Read-only fixednative generator/protected7 verification, outputs only ROOTprivate parent; no overlapworkerupgrade/retry211.

ROOT same-campaign owns physical-apply-37.py and its contract; worker owns separate211retry. Read originalworker8d4 as source reference only, neverexecuteits obsolete211tuple.

ROOTsamecampaign additionally owns physical-native-37.py+contract fortrue7native1physicalacceptance; worker257native211 remainsREADonlysource reference.

ROOTsamecampaign owns boot.py+contract for3ownedunitenablement(no--now), ownerauthorizedreboot andpostbootreadonlyacceptance, plusnarrowpostboot-nativeproof selectorsinROOTphysical-native37/workerphysical-native211 afteractualbootproof. No foreignenablement orsharedcontrollerVPP change.
