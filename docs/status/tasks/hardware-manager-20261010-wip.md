# Resumed disk recovery — 2026-10-10 08:59 UTC

Owner now explicitly asks to fix the disk problem. Root resumed existing host37,
host211, and evidence_review workers; actual live diagnostic messages received.
This supersedes the earlier awaiting-resume operational snapshot below. Source
main remains4908716b; no product code or CI change is required for this recovery.
Previous resumed checkpoint published/read back14594feca3d7cdc00dddc3ae3581f99923c525d2;
at this edit local/remote manager HEADb78f87c58a25c92d6720f4eb9f6e84aa8ab93793;
this coherent checkpoint is immediately committed and published/read back.
Owned files and private destinations remain as in the task envelope.

Fresh read-only findings: both root /dev/sda2 ext4 mountedrw, clean-with-errors,
failed fsck at inode259596 invalid extent block15505493. Counts .37=1319,
.211=1317. No repair, target package staging/install, reboot or network change.
No usable second filesystem or IPMI device; disk models/controller errors do not
establish physical hardware health without SMART. Both PID1/systemd259.5 and
multiple other processes pin the old root. RAM is sufficient; /run is noexec,
/tmp executable but its mount conflicts with shutdown. Stock initrds lack SSH/fsck.

Current work: independently review a dedicated executable RAM /run/nextroot with
matching minimal systemd/sshd/e2fsck runtime, authenticated SSH before transition,
and exact network retention. Matching upstream v259.5 supports soft-reboot, but
switch_root uses lazy detach, so successful transition/empty mountinfo is not
sufficient. .37 mounted-negative block-device O_RDONLY|O_EXCL correctly returns
EBUSY; require positive exclusive probe and all-namespace/reference audit after
transition before any fsck. Prepare metadata backup/undo and an explicit safe
return path. Full system image has not been taken; config backups are not one.

Reversible RAM-only staging is now authorized to both exclusive host workers,
independently supported by R7. No transition or repair is authorized yet. Stage
at /run/ngfwrescue (dedicated exec tmpfs, correct survival mount unit); keep
/run/nextroot absent until final reviewed readiness. Candidate SSH survivor uses
regular runtime unit invoking chroot then RAM sshd in the SAME mount namespace
as PID1, with RAM-only root/cwd/maps/fds, key-only/PAM-off authentication. All
SSH children remain in the protected service cgroup. RAM /etc unit of same name
uses direct valid ExecStart and overrides persisted runtime /run bootstrap.
Copy matching systemd-executor and dependencies as well as PID1; official v259.5
closes old pinned executor on reexec and reopens from RAM. No blind PID1 fd closure.
Networkctl reload reconfigures links, so no live KeepConfiguration reload is
permitted as passive staging. Review exact stop behavior and all network snapshots.
SMART readonly via signed cached Ubuntu package is being investigated in RAM.

Task-only static offline guard compiled with -static -O2 -Wall -Wextra -Werror;
three refusal tests PASS (missing args/nonblock/invalid identity). Source is the
owned block-check.c task document, not product code. Binary860432bytes SHA256
02a2270ad8e0f480efc7464b26d2216fec8a915aa12755da6339015abed0f41c,
private shared controller path. It validates direct block identity then
O_RDONLY|O_EXCL|O_CLOEXEC|O_NOFOLLOW, writes no device data; BUSY exit3 prevents
repair. Independent R7 source/binary review APPROVE; independently rebuilt same static
binary/hash and six refusal tests PASS. R7 isolated controller Linux behavioral
5/5 PASS: unmounted exclusive success, mounted BUSY, lazy-detached fd-pinned BUSY,
released final reference success, ordinary-unmount success. Only tiny owned RAM
ext4/owned loop/private namespace used, then exact-identity cleanup verified.
Target mounted-negative helper validation remains required before transition. Namespace/reference audit and controlled no-remount
environment remain additional mandatory checks; positive exclusive open alone
is not a complete recovery workflow.

Actual initial staging status (historical): .211 reviewed RAM stage517d789e/c4027e96 stopped
safely after dedicated8GiB RAM mount/initial binaries: standalone library ldd
lost systemd application RUNPATH. Original22 preserved; no credentials, virtual
mounts, rescue SSH or nextroot at failure. A flattened executable closure fix
743272ac is under focused independent review before exact owned partial-RAM
cleanup/rerun. No mounted-root filesystem repair. .37 script still in review.
Root caught ignored SurviveFinalKillSignal in initial211 [Service] section;
corrected [Unit] and ordinary shutdown ordering before execution. Removed
bootstrap WorkingDirectory dependency; .37 explicit Requires mount likewise
must not stop protected SSH after path reconciliation. PTY needs devpts binding.
Current remaining transition proof: authenticated command AND PTY, all service
properties and same global namespace/RAM-only refs, transferred /run/sshd lifetime,
real positive exclusive/all-namespace checks after pivot, private metadata capacity
and off-host verification before corrective writes, independently reviewed return.

.211 root geometry matches exactly15505494*4096=63510503424bytes. Invalid extent
block15505493 is last legal block and zeros; no partition enlargement justified.
Read-only debugfs identifies invalid inode as user journal, not root home. Original
auth/config archive captured off-host privately35240bytes/43readablemembers/
0600 tar exit0; SHA bf00f6dbd3ddcf7177322f8b3d9df3d2bd971221e2cf7e58b9613d59f9e0fccc.
Original selected binary/library dpkg verification reports no executable/library
mismatch; missing docs/man assets only. Kernel cmdline has no default-unit override.
Signed official Ubuntu cached InRelease/Packages index validation PASS .211;
SMART package RAM extraction/read-only health query awaits stage, no hardware
health result yet. Root controller245MiB and /dev/shm1.9GiB free at08:47; compressed
metadata actual sizes not yet measured, no full-system image claim.

Latest08:59: R7 static/behavior receipt ccd6581d, corrected source-review receipt
31872b92, return-design receipt0da3aefd published/read back. .37 final source
b9da683b at remote5b9a8e6523e9d072e25a65147a32f9004e55ad42 APPROVE RAM-only
stage/test; prior public digest-naming detector false-positive checkpoint02329
remotely archived before expected-head lease replacement, no secret or rule change.
Auth and boot backups independently metadata/hash/readability verified on both;
.37 auth archive655360bytes/27members, .21135240bytes/43members, private0600/
0700 parents, no unsafe paths or content exposure. These are scoped backups.
.211 retry safely found assumed /usr/sbin/chroot absent; discovered /usr/bin/chroot
fix approved. Next stop correctly caught host-side exists() following candidate
BusyBox absolute symlink outside chroot; replace with lexists plus actual chroot
helper execution. Original22 remained available; RAM virtual mounts/keys remain
private, no rescue listener/transition/repair yet at that failure. Runtime helper
oneshot explicitly recreates /run/sshd after switch_root overlays staged /run,
logs RAM-only; serialized active service coldplug does not create RuntimeDirectory.
Minimal default target requires helper and SSH; no journald/network/disk boot units.

Independent normal-return design is supportable after actual clean repair and
verified offhost metadata/undo/logs plus readonly/noload auth/network/boot checks.
Single systemctl reboot --force uses PID1/systemd-shutdown sync/kill/unmount/reboot;
double force rejected. Remove only exact owned nextroot symlink and set documented
skip-auto-soft-reboot and skip-auto-kexec flags. User already authorized necessary
reboot; no invented universal console requirement. Actual transition/repair/return
approval remains held pending real staged runtime receipts and offline proof.
Fresh remote main4908716b unchanged, all three main hosted gates completedSUCCESS,
no openPR; remote board212=205merged+7parked. Actual live roles verified4: root
manager, two host operators/testers, independent recovery reviewer. External live
inventory remains unverifiable; no persistent runner claimed. No new product merge
in resumed disk-recovery phase. Root~241MiB/devshm1.9GiB free.

Current limitation: no completed authenticated RAM rescue proof yet. Exact next
commands are exclusive workers' reviewed RAM-only --stage/--test, SMART signed
package read-only query, then independent artifact/runtime review. Do not transition
or fsck yet. Do not wait for prior console question before useful safe
preparation. Do not perform transition or corrective writes before review.
Root controller filesystem recovered to~175MiB free and /dev/shm1.9GiB free; only redundant task-owned build trees
were removed to recover staging/backup capacity: own completed package-source and old
ngfw-hardware-package-20261010 tmpfs build only; preserve immutable packages,
source archive refs, checkpoints, all private evidence and other tasks' work.

Previous durable handoff follows; its awaiting-recovery roles are historical:

# Hardware installation WIP — 2026-10-10 08:06 UTC

Branch: codex/hardware-manager-20261010.
Worktree: /root/ngfw-wt/hardware-manager-20261010.
At this status edit localHEAD/remote ownbranch:0f3ab280f3d3941b21e2580d584bdaddc614822c;
origin/main:4908716b4501312102382e6979b8fc1ded6f9311.
This status checkpoint will be committed/pushed immediately; discover its current
local/remote SHA with `git rev-parse HEAD` and
`git ls-remote origin refs/heads/codex/hardware-manager-20261010`. Publication is
reported on PR217 only after successful push/readback. Owned files are task docs,
API native Depends field in deploy/debian/ngfw/debian/control, external build output.
No edits to another worktree or local main. User workspace remains untouched.

Compiled source:2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c, remotely preserved in
codex/archive-hardware-manager-20261010. Prior integrationbde83bc preserved in
codex/archive-hardware-manager-20261010-integration. Final D112 one-commit source
5bd7e8b has identical product content. PR217 merged expected head without bypass;
remote main4908716b has exact expected parents/current main d2d55984 plus5bd7e8b
and fully tested treea0d7b7cbc7f37dc2fadd9a93d41ce486dcda50d2.

Completed host-independent code: API package now consumes existing shlibs:Depends,
enforcing actual native libc6>=2.34/libgcc-s1>=4.2 plus Node22 bounds. No other
product/test/build/CI change. Corrected clean prepare completed14TS tasks,
seven Go helpers, production API deployment. Unchanged dpkg-buildpackage exited0,
all41 mandatory fixtures PASS. Current VPP verifier72tests/manifests/hashes/install
gate PASS; four corrected application archives and seven selected VPP packages
preserved with hashes. Old runtime/archives remain explicitly BLOCKED.
Final hosted unchanged mandatory quick38033766837 SUCCESS: literal CI GATE PASSED
read independently, actual checkout8a15d644/fully tested treea0d7b7. Both additional
hosted fixture gates success. All R1/R2/R7/R8 APPROVE, T1PASS; immutable receipts in
[combined review](hardware-manager-20261010-review.md) and
[actual evidence](hardware-manager-20261010-evidence.md).
Premerge default full gitleaks found10 preexisting sanctioned test placeholders;
independent R2 triage confirmed no real secret. Unchanged canonical-config full
no-git scans both exit0/noleaks. No allowlist/test was modified to hide findings.
Main bare quick38035583209/job114165198295 COMPLETED SUCCESS on exact4908716b.
ActualAPI completedAt2026-10-10T08:07:08Z; independent T1full58932-byte log shows
checkout4908716b,35/35tasks,149harnesschecks and literal CI GATE PASSED.
LogSHA2569c957c9fe1c469f1acbde3f434b2b021ba21e6a09a6c666c0d10f3503df2fb12.
Final postmergeT1PASS published/readback4a8a82205d490068d865a1344d86afcaf93b8502.
R7recoverydocs/metadataAPPROVE published/readback0eb7036450d410b7540e05a786205c72bd814acf.
Fresh main/head/tree and board counts readback08:03 confirms exact490/treea0;
noopenPR. Source metadata correction and all mandatory code gates are complete.
Fresh08:03readonlySSH bothPASS; protected addresses/default routes unchanged,
bootfsckunitsstillfailed. No targetinstall/repair/reboot or live acceptance.
Main packaging38035583240 and provisioning38035583210 completedSUCCESS on4908716b.
Public925703c checkpoint checkPASS13s; scanned15040bytes/no leaks; clean afterpush.
Private network/config snapshots completed on both hosts; configtar/allroute-rules/
addresses-links/PCI maps captured with actual exits/hash receipts. Native nft tool
unavailable(exit127); existing iptables-save/ip6tables-save exports both exit0/empty
on each host, but do not prove native nft rules empty. Configurationbackuponly, NOT
fullsystem/data backup. All files0600/dirs0700, nevercommitted or included in candidate
archive. Independent R7 metadata/readability/exclusion checksPASS; no new blocker.
Backup receipts .37=0b96a5ff51aca239e2b1492456c37e2052f139ed;
.211=269d455f6aee29cc89007bbac4aa93d00c0fad7f. See
[recovery runbook](hardware-manager-20261010-recovery.md).

Hardware task: BLOCKED, installation and hardware acceptance NOT RUN. Both ext4
/dev/sda2 roots have structural checksum/extent/journal errors and failed boot fsck:
UNEXPECTED INCONSISTENCY at inode259596. Fresh strict-known-host SSH07:38UTC PASS;
.37enp12s0UP172.30.126.37/24/default172.30.126.1/group58/PCI0000:0c:00.0;
.211enp4s0UP172.30.110.211/24/default172.30.110.1/group28/PCI0000:04:00.0.
Both roots mountedrw, clean-with-errors; counts1258/.37,1248/.211.
No package transfer/install, firstboot/service change, NIC binding, route/config
change, filesystem repair or reboot performed. Management PCI/groups stay excluded.
Seven/seventeen data NICs have independently verified original names/PCI/group
maps; proposed configs are schema-validated and NOT APPLIED. .211 data bridges need
reviewed membership changes only after clean storage. Physical af_packet declarative
import unsupportedD105; guarded DPDK/true IOMMU, no no-IOMMU bypass.

Required recovery input: verified console/rescue (physical/serial/IPMI/KVM/LiveISO)
and off-host backup/offline repair path, or evidence owner already repaired root
unmounted and management returned. Asynchronous owner question remains unanswered.
Do not fsck mounted root, force unattended repair/reboot, or delete corrupt dirs.
After recovery: repeat root/fsck/device-health/SSH/route preflight; fix trusted time;
inspect package/service dependencies and preserve existing routing/firewall/netplan.
Complete canonical firstboot with VPPno-pci/managementblacklist, start agent/API,
seed builtin physical rows BEFORE any data PCI binding and before operator revisions.
Verify exact7/17 original names/markers and management exclusions; export authoritative
seeded dataplane, guarded apply-startup uses same names, reconverge and verify stored
and live inventory. Ordinary API import cannot fabricate physical markers. Then real
API/TLS/forwarding/restart/reboot persistence tests. No live forwarding/throughput,
offline dependency closure or release acceptance claimed.

Remote preflight .37=86a2f06eafc6406e3e5395769d923a09bbfa60aa;
.211=a4059c73b4a2e3346b7cf1cf705c0a5b908c317d. Resume existing owned host branches,
never silently rebuild. Live roles before08:06handoff:rootmanager finishes status/artifact publication;
all host/reviewer/tester workers finished awaitingresume. No live installer/developer/
tester/reviewer is claimed; root also awaits required recovery input after handoff; no active installers/persistent supervisor.
Board212tasks:205merged,7parked,0running/ready/review. Adhoc hardware request has no
invented WBS state. This cycle has one new product merge(PR217), no stale-row fixes.
Root disk~500MiB available; do not duplicate broad local builds or delete others' work.
Outputs/logs:/root/Documents/Codex/2026-10-10/hardware/.

Current failure: active target root-filesystem errors; verified rescue input missing.
Remaining hardware work: verified console/rescue/full-data backup or agreed recovery
plan, offline root diagnosis/repair and clean preflight; guarded package installation,
physical rows/import and actual API/TLS/forwarding/restart/reboot tests. No code/gate
failure remains in the packaging correction. This hardware task is not complete.
Exact next read-only command AFTER owner reports offline recovery complete:
`ssh -o BatchMode=yes -o StrictHostKeyChecking=yes root@172.30.126.37 'findmnt -no SOURCE,FSTYPE,OPTIONS /; systemctl status systemd-fsck-root.service --no-pager; ip -br addr; ip route show table all'`
Repeat for172.30.110.211; compare private backed-up state. Read recovery runbook first.
Checkpointpublication/actual remote SHA is reported onPR217 after successful push
and readback. Preserve current source/history/artifacts; resume existing task branches.
