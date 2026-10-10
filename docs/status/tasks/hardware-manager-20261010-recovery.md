# Hardware recovery and installation resume

Current phase, 2026-10-10 10:43UTC: both /dev/sda2 roots initially had structural
ext4 errors and failed boot fsck. .211 is in matching RAM rescue, correction
completed exit1 and subsequent full offline-f-n exits0 through all5passes. Native
metadata image, seven all-zero affected external blocks, actual749568B undo and
complete transcript/answer ledger are durably preserved offhost. Full post-audit
260processes/93FDs/nsfs0/races0/failures0/finalguard0 passed, kernel exactlyequal
and ioerr18 stable. Selected originalroot/EFI readonly integrity52/52 comparisons
passed and both ordinaryunmounts0; separate normal-return release awaits finalRAM
shutdown/manager/freshguard/sync/nextroot and applicable R7 actual verdict.
.37 remains on original22 plus held RAM rescue PTY;
its transition waits for .211 normal return. Package installation and hardware
acceptance remain unexecuted. The owner explicitly requests repair despite .37 SSD wear; wear
alone is not a replacement or permission blocker. Recover .211 first, then .37.
The earlier console-only plan is superseded by the reviewed matching-systemd RAM
rescue design and actual readiness evidence, not by an unverified rescue assumption.

## Preserve management and existing data

Protected .37:172.30.126.37/24 onenp12s0,default172.30.126.1,
PCI0000:0c:00.0,IOMMUgroup58. Protected .211:172.30.110.211/24 onenp4s0,
default172.30.110.1,PCI0000:04:00.0,IOMMUgroup28.
These interfaces/devices/groups must remain in the kernel; preserve addresses,
all routing tables/rules, DNS and netplan, and current firewall allowances.

Read-only configuration snapshots were successfully collected into controller-private
/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-{37,211}.
They contain network configuration and snapshots, not full root/data backup. Keep
0700 directories/0600 files; no public content, Git commit or candidate-tar inclusion.
Actual command exits/readability/hashes and gaps are preserved in published receipts:
[.37](https://github.com/mcoder1001-cyber/NGFW/blob/0b96a5ff51aca239e2b1492456c37e2052f139ed/docs/status/tasks/hardware-37-20261010-wip.md),
[.211](https://github.com/mcoder1001-cyber/NGFW/blob/269d455f6aee29cc89007bbac4aa93d00c0fad7f/docs/status/tasks/hardware-211-20261010-recovery-receipt.md).
Independent R7 inspected permissions/manifests/archive readability without publishing
contents: parent/host directories0700, all24/.37 and26/.211 regular files private,
both configuration archives5members with management netplan included; all regular
payload lengths readable. The23-member candidate tar contains0private-backup members.
Native nft capture is unavailable on both hosts (toolmissing,exit127); successful
empty IPv4/IPv6 iptables-save exports do not establish empty native nft state.
Inspect actual native firewall after clean recovery and before activation.
Do not infer a complete backup from a tar file merely existing.
Additional original SSH/account and selected boot/config backups are privately
captured, readable and hash-verified both offhost and in RAM. These remain scoped
backups. Full root/userdata images have not been taken; .37 apparent /root usage
is at least23.739GB and exceeds available backup capacity. Do not remove that
data or equate filesystem metadata/undo with file-byte or power-loss protection.
Before correction, capture verified offline filesystem metadata offhost, inspect
the actual affected inode names/types, and retain a bounded undo log. A concrete
new read/media failure or unexpectedly affected user file requires diagnosis.

## Offline diagnosis and repair

1. The exclusive host operator publishes actual readiness receipts and readback,
   keeps the authenticated RAM PTY live, and verifies original SSH to the other
   host. Exact .211 staged source e33b95eacf965361d2dc3ba05866e3ee80d9eb1cd6259679fa7d3582a34166d6
   is reviewed at2ee35a45; actual readiness is published ate65e4f86. R7 transition
   and read-only phase approval is45b08eab, counter addendum5af876c0. Review effective
   management static/dynamic state and matching networkd exit behavior; do not
   modify KeepConfiguration or use networkctl reload as passive preparation.
2. Only on the reviewed phase release, create the exact owned symlink
   /run/nextroot -> /run/ngfwrescue and invoke ordinary systemctl soft-reboot through
   original SSH. Do not force this transition. Matching259.5 RAM PID1, executor,
   shutdown and SSH helpers are copied and validated. The protected SSH unit has
   RAM-only process references in PID1's global mount namespace and surviving
   children; a new-root helper recreates /run/sshd after transferred /run overlays it.
3. Verify actual RAM PID1/executor, helper completion, /run/sshd ownership/mode,
   persistent PTY and fresh authenticated2222 SSH, and exact management addresses,
   all-table routes/rules. Remove only the exact owned nextroot symlink after the
   transition is confirmed. Audit every process root/cwd/exe/maps/fd, every mount
   namespace including namespace fds without a representative process, and block
   users/automounts. switch_root lazy-detaches the old root; empty mountinfo or
   successful SSH alone is insufficient. The independently tested static guard
   SHA02a2270ad8e0f480efc7464b26d2216fec8a915aa12755da6339015abed0f41c
   must positively open the actual /dev/sda2 identity8:2 O_RDONLY|O_EXCL (exit0).
   If BUSY, identify the holder; never close arbitrary descriptors or force fsck.
4. Only after that offline proof, run e2fsck -f -n with full private output/exit.
   Actual .211 diagnostic exited12 (uncorrected4 plus aborted8). Its e2image -Q
   failed on corrupt extent traversal and retained0B; do not repeat or claim it
   as preservation. Reviewed plain e2image /dev/sda2 NEW_PRIVATE_RAM_FILE succeeds
   without extent traversal. It saves super/group descriptors/inode tables/bitmaps,
   omitting external directory/extent/EA/journal blocks and user data. Supplement
   affected blocks15505493/15503361/15503362/15503363/15503874 using reviewed
   static read-only allowlisted helper; .211 actual five4096B blocks are all zero.
   Measure actual capacity and image size, compress privately
   offhost, verify compressed and decompressed source hashes, and keep both pipeline
   exits. Record block-read/SCSI/CRC/kernel baselines before and after reads. Do not
   use a nonseekable image output or modify the mounted original root. Both filesystem and
   partition are exactly63510503424bytes; no partition enlargement is justified.
5. Review actual inode/path findings and prompts plus verified metadata and undo
   capacity before interactive LC_ALL=C e2fsck -f -E fixes_only,nodiscard -z
   NEW_PRIVATE_CAPPED_RAM_UNDO /dev/sda2 on the still-unmounted identified root.
   Actual .211 wrapper inherits verified1GiB soft/hard RLIMIT_FSIZE, measured RAM
   and controller capacity plus512MiB margin, private1MiB write/sync preflight.
   Controller must write exactlyONE y/n byte without newline: matching e2fsck
   noncanonical ask_yn consumes onebyte and queued newline accepts the nextdefault.
   Initial .211 driver violated this;5 implicit known-scope defaults are honestly
   retained, controller-only verified pipe helper corrected continuation. Future
   driver is fixed. Explicit individual y/n only: known three journal extent clears/accounting and
   known cache/config salvage/checksum; new objects/prompt effects require concrete
   classification before acceptance. Directory salvage may discard malformed entries.
   No unattended -y, arbitrary directory deletion, format, repartition, SMART reset
   or generic -D repair. Known inode259596 is a damaged regular journal file, not
   /root; do not infer all reported corruption is confined to that file. Undo is
   not a crash-safe full backup. Record every actual correction and its exit code.
6. Require a subsequent clean offline e2fsck -f -n, stable actual block-read health,
   offhost verified image/undo/logs, and a temporary read-only/noload original-root
   check of selected original boot/auth/network files. Ordinary unmount afterward.
   Reconfirm kernel/initrd/EFI/fstab/management/SSH baseline and the exact owned
   nextroot marker's removal before return. Then set SYSTEMCTL_SKIP_AUTO_SOFT_REBOOT=1
   and SYSTEMCTL_SKIP_AUTO_KEXEC=1 and use single systemctl reboot --force: the RAM
   PID1 still performs systemd-shutdown cleanup. Double force bypasses that cleanup
   and is prohibited. Actual return is separately released after these gates;
   the owner already authorizes a necessary reboot, so no repeated permission ask.
7. After normal boot, prove fresh original22 SSH, exact protected PCI/NIC and
   addresses/routes/rules/DNS, clean root/boot-fsck status and no new storage errors.
   Historical ext4 error counts are not alone a post-repair failure. Repeat the
   reviewed sequence for .37 while .211 stays reachable; the accepted endurance126%
   and two remaps remain physical facts, not something software repair removes.

No correction or normal return approval is implied by a read-only phase release.
Current actual findings and next commands belong in each published worker WIP and
the manager WIP; do not treat this runbook as execution evidence.

## Resume the existing installation tasks

Resume codex/hardware-37-20261010 and codex/hardware-211-20261010 in their existing
isolated worktrees; read durable WIP/envelope. Use only corrected runtime-fixed/
archives and the seven selected VPP26.06+ngfw3 packages; old runtime/ is BLOCKED.
Check archive hashes and independently reviewed provenance before transfer/install.
Package metadata source2045ab8 equals merged tested product tree; no offline
system-dependency closure or release certification is claimed.

After clean-storage preflight: synchronize trusted time; inspect dependency service
start behavior and temporarily suppress automatic starts for controlled installation.
Protect SSH/routing/firewall before starting the application. Complete canonical
firstboot (only its allowed overrides), preserve management blacklist and VPPno-pci,
connect agent, then enable hardware seeding only after provisioning and BEFORE any
data PCI binding/operator revision or candidate edit. Verify authoritative original
names and physical markers for exact7/.37 and17/.211 nonmanagement NICs, then export
seeded dataplane and use guarded startup with those same names. Physical af_packet
import is unsupportedD105. True IOMMU is required; never bypass with no-IOMMU or
fabricated DB/API import markers. Remove obsolete .211 data bridge membership only
as a reviewed data-port topology change; preserve management/routing state.

Actual installation/service/API/TLS/storage/live inventory/forwarding tests must
then pass. Check route/SSH preservation after each activation. Perform required
restart/reboot persistence only with verified recovery available; reconnect and
verify exact interface inventory and routing again. Record unexecuted tests as
NOT RUN, and measured throughput only if an actual traffic test runs.
