# Independent recovery safety review — 2026-10-10

Scope: read-only feasibility review after the owner explicitly requested disk/root
filesystem repair. This is authorization to pursue repair; the outstanding issues
below concern verified access, filesystem isolation and data preservation. The
reviewer made no target, product, network, service, boot or repair changes.
Original source R7 APPROVE on5bd7 and public-backup evidence approval0f3ab280
remain separate from this review.

Current verdict: **BLOCK executing a root transition or repair on either host**;
there is a concrete RAM preparation candidate to validate. This is not a claim
that a console is the only technically possible recovery method. Neither a
surviving recovery connection nor an offline root has been demonstrated yet.

## Actual evidence available at 08:26–08:39 UTC

The manager's published recovery runbook at0f3ab280 was read locally, along with
this reviewer's prior published private-metadata addendum0eb70364. Current remote
readback during this review:

```text
git ls-remote origin refs/heads/codex/hardware-evidence-review-20261010 \
  refs/heads/codex/hardware-manager-20261010 refs/heads/main
0eb7036450d410b7540e05a786205c72bd814acf reviewer
4f8cc7f40ff23581058643b13458b3b8624299a9 manager
4908716b4501312102382e6979b8fc1ded6f9311 main
```

Fresh target diagnostics below are sanitized reports from the assigned host
workers, not commands executed by this reviewer. Immutable worker diagnostic
receipts are pending at this checkpoint; previous configuration backup receipts
remain published. Full diagnostic or credential contents were not printed.

| Observation | .37 | .211 |
|---|---|---|
| Installed PID1 | systemd259.5 | systemd259.5-0ubuntu3 |
| Root | /dev/sda2,ext4,rw,63.5GB decimal | /dev/sda2,ext4,rw,59.1GiB |
| Error state | clean-with-errors,count1319 | clean-with-errors,count1317,journal I/O error |
| Boot fsck | invalid extent inode259596,manual required | same inode/manual requirement |
| Physical drive | Samsung860PRO256GB,SATA | Samsung870,SATA; SCSI ioerr counter0x6 |
| RAM/swap | 16.65GB/~15.97GB available/no swap | 32GiB/~30GiB available/no swap |
| Other boot media | only EFI/root partitions,none verified | only EFI/root partitions,none verified |
| Rescue/BMC | no usable channel established | no usable channel established |
| Installed rescue tools | e2fsck,sshd; hidden busybox to confirm | e2fsck,sshd; hidden dynamically linked busybox |
| Existing initrd | ~76.38MB; busybox,ip,switch_root,igc; no matched SSH/e2fsck | ~76.38MB; busybox,ip,pivot_root,switch_root; no matched SSH/e2fsck |
| Old-root users | PID1 executable/root/cwd; earlier444 scan included kernel threads | PID1 has1 fd/105 mappings;12 processes reported |

The error counts are snapshots, not proof that identical inode failures have the
same cause. Neither worker has a SMART health result. Absence of an ipmitool binary
or local /dev/ipmi is not proof that an external BMC cannot exist.
Management remains protected onenp12s0/.37 and enp4s0/.211 with the previously
verified original addresses and routing. No install, reboot or filesystem repair
is reported by either worker.

Follow-up .37 selectors reported by its assigned worker:

```text
findmnt -T /run: tmpfs,rw,nosuid,nodev,noexec,size1625512k
findmnt -T /tmp: tmpfs,rw,nosuid,nodev,size8127552k
systemctl show tmp.mount: active,DefaultDependencies=no,Conflicts=umount.target
/proc userspace executable scan:17 processes,4 mount namespaces; root device8:2
systemd-networkd:active,Conflicts=shutdown.target/initrd-switch-root.target
generated management .network: no explicit KeepConfiguration
os.open('/dev/sda2', O_RDONLY|O_EXCL): errno16 EBUSY
```

The mounted-negative exclusivity result is useful but does not test the proposed
positive/offline case or deliberately pinned lazy detach. Current controller
capacity was directly checked by this reviewer:

```text
df -B1 /root/Documents/Codex/2026-10-10/hardware
Filesystem       1B-blocks        Used Available Use% Mounted on
/dev/sda2     210812596224 202165678080  31129600 100% /
```

No capacity for an off-host preservation artifact is certified by this snapshot.

## Supported candidate and the limits of the mechanism

Both hosts have an installed systemd version for which the matching upstream
v259.5 documentation describes userspace soft reboot into /run/nextroot, with
the existing kernel retained. The documented survivor service requires
DefaultDependencies=no,SurviveFinalKillSignal=yes,IgnoreOnIsolate=yes and correct
ordering/conflicts; retained mount units must avoid umount.target conflicts.
Surviving processes must not pin the old OS. These are requirements to validate,
not evidence that a new recovery SSH service currently exists.
[Matching systemd manual](https://raw.githubusercontent.com/systemd/systemd/v259.5/man/systemd-soft-reboot.service.xml).

Direct systemctl switch-root is unsuitable here: upstream v259.5 explicitly
refuses it outside initrd. Presence in help does not override that restriction.
[Matching manager implementation](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/core/dbus-manager.c).
Do not bypass the check by forging initrd markers. An ad-hoc pivot from an SSH
shell also leaves PID1/executable/library/open-descriptor references unresolved;
the kernel documentation requires removing all old-root accesses before unmount.
[Kernel initrd documentation](https://docs.kernel.org/admin-guide/initrd.html).

**R7-RAM-1:** stock systemd soft reboot is not an offline-filesystem certificate.
Its v259.5 implementation calls switch_root with old_root_after=NULL, and that
path uses MNT_DETACH. An absent /dev/sda2 mount in one namespace can coexist with
live references to the detached filesystem. A reviewed recovery plan must prove
the device is no longer in use, rather than infer this from a new / root or SSH.
[PID1 transition implementation](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/core/main.c),
[Root-switch implementation](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/shared/switch-root.c).

The Linux block-device O_EXCL check is a concrete additional validation candidate:
O_RDONLY|O_EXCL without O_CREAT fails EBUSY while the device is in system use.
A vetted read-only helper must first fail on the currently mounted root, then
must also fail in a disposable test with a lazy-detached mount deliberately pinned
by an open reference. Only after those tests could a successful check contribute
to an offline certificate, together with namespace/reference scans and disabled
automounts. Do not hold its exclusive fd while launching an independent fsck.
This proposed check has **not** run on either target and is not yet a reviewed
executable. [Linux open manual](https://man7.org/linux/man-pages/man2/open.2.html).

## Preparation and validation before risking current SSH

The following is a proposed operator plan, not executed commands or PASS receipts.
Preparation can use RAM without writing corrupted ext4 or modifying NIC ownership.
The assigned installer must independently review the exact files/units/helper
before target preparation; this reviewer owns documentation only.

1. Confirm disk identity/UUID/major:minor, all mount namespaces and process refs,
   active network/firewall services and shutdown hooks, executable RAM capacity,
   target binary provenance and every dynamic dependency. Set a strict RAM budget
   that leaves ample headroom; do not try to fit a whole59GiB image into16/32GiB RAM.
2. Assemble a dedicated executable tmpfs root, separate from /run/nextroot until
   ready. .211 /run is reported noexec; /tmp is executable tmpfs, but default /tmp
   shutdown behavior must not destroy the candidate. Validate an explicitly
   retained RAM mount and its dependencies. It needs a matching usable PID1,
   loader/libraries, minimal OS tree, shell/ip/mount/unmount/findmnt, e2fsck and
   its libraries, OpenSSH daemon plus sshd-session/auth helpers and dependencies,
   virtual /dev,/proc,/sys,/run, and minimal reviewed units. No old /usr or /etc
   bind mounts, service generators that mount sda2, or old-root-backed logging.
3. Keep host keys/authorized controller public key in private RAM files only;
   preserve fingerprint verification. Use key-only authentication and a complete
   minimal account/NSS/auth setup. Check sshd configuration and actual login,
   session/helper execution and repair-tool versions inside the candidate root.
   Do not print keys, credentials, private config or existing SSH command secrets.
4. Start a separate RAM-rooted recovery SSH service on an available management-only
   port while current port22 remains reachable. A controller must authenticate
   independently and verify its shell/executables/maps/cwd/fds reside in RAM or
   virtual filesystems. Validate an available firewall path without blindly
   changing current rules. Do not bind the survivor to a data port.
5. Rehearse the exact units and transition in a disposable compatible environment:
   survivor connection continuity, reconnect, new PID1 execution, retained RAM
   mount, unchanged routes/rules/addresses, no old block-device references, busy
   probe behavior, and failure handling. Syntax-only/chroot success does not test
   shutdown survival. Preflight should verify the live unit dependency graph and
   cgroup survival; no automatic reboot, old-root remount or disk repair on failure.
6. Before activating a next-root marker or scheduling a transition, preserve a
   vetted RAM recovery shell independent of the old root and a tested way to
   restart SSH from RAM if the new PID1 fails. Confirm all current network service
   stop hooks preserve management state. Avoid starting a network manager that
   may reconfigure links. Any proposed networkd configuration must explicitly
   preserve existing state and be tested. An unverified second SSH listener alone
   does not replace a recovery channel after PID1 transition.
7. Only after reviewed preflight, usable independent recovery and the backup gate
   below: handle one host first; transfer control through the supported mechanism.
   Independently verify RAM PID1 and authenticated recovery SSH. Stop if any
   management comparison fails or any old-root reference remains. Establish
   device-offline proof, including all namespaces and a validated kernel busy
   probe; a normal unmount is preferred when the old root remains addressable.
   Lazy detach alone never authorizes repair.
8. Capture the chosen offline preservation artifacts, then run the non-writing diagnostic
   e2fsck -fn on the identified unmounted device and review the full exit/output.
   Repair interactively only after diagnosis and preservation readiness, retaining
   output off-host. Repeat the offline check until clean. Return to normal boot
   only with the recovery route still available; freshly check both management
   SSH and route/NIC/firewall/storage evidence before resuming NGFW installation.

A /run/nextroot tree changes how ordinary systemctl reboot can behave, so creating
that marker early is an operational change, not harmless staging. Keep it absent
until the reviewed transition checkpoint.
[Matching systemctl manual](https://raw.githubusercontent.com/systemd/systemd/v259.5/man/systemctl.xml).
Networkd's KeepConfiguration defaults differ by environment; infer no route
preservation from the installed version alone.
[Matching network configuration manual](https://raw.githubusercontent.com/systemd/systemd/v259.5/man/systemd.network.xml).

## Data preservation and exact remaining input

**R7-RAM-2:** preservation must have a concrete scope, sufficient measured space
and verified artifacts. Existing configuration snapshots protect neither ordinary
file data nor filesystem metadata. A full root image is one option, not a mandatory
59GiB allocation for every fsck. The manager proposes limited metadata preservation
plus repair undo, with explicit limits. This can be prepared within available RAM
if its measured size/budget is safe, but does not certify full data preservation.
Do not repartition unknown free space or assume compression will meet a budget.

For a limited plan, use verified offline e2image metadata capture, partition/UUID
identity records, private hashes/readback, and an e2fsck undo artifact on another
filesystem. Raw-r or QCOW2-Q preserve directory/indirect metadata, whereas default
e2image does not include those blocks; ordinary file data requires-a. A QCOW2 file
needs random access; only raw-r supports a stdout compression pipeline. Thus-Q
into a budgeted RAM file can be a practical metadata candidate, with an independently
verified private off-host copy if capacity exists. Inspect actual exit, allocated
size, readability and integrity; never treat a partial artifact as a completed
snapshot. Directory names in metadata can be sensitive. Do not use-I to restore
metadata blindly, or-f to bypass a mounted-source check.
[Upstream e2image manual](https://raw.githubusercontent.com/tytso/e2fsprogs/master/misc/e2image.8.in).

Record separate budget/headroom for the RAM OS, SSH, e2image, fsck and undo. An undo
file can grow with every overwritten block, and low RAM/ENOSPC during repair is a
failure path, not an acceptable size-control technique. Retain logs/artifacts
privately off-host wherever measured capacity permits. At the measured31MB free
controller snapshot there is no capacity PASS. A metadata/undo-only plan leaves
ordinary file contents and power/crash recovery unprotected; do not claim otherwise.
Further physical read failures or failed preservation capture require revisiting
the recovery strategy, potentially imaging first onto suitable external storage.

e2fsck's undo file cannot recover a power/system crash, and directory rebuild
option-D applies more broadly than one corrupt inode. Neither is a substitute
for preservation readiness. Mounted -n results cannot certify filesystem health;
repair must occur offline with actual diagnostic-based choices and exit handling.
[e2fsck manual](https://man7.org/linux/man-pages/man8/e2fsck.8.html).

The next useful work is candidate assembly/review and isolated rehearsal, not a
forced boot into the existing fsck failure. If RAM survival cannot be demonstrated,
the exact external input is a usable physical/serial/KVM/BMC or rescue-boot channel
with demonstrated shell access; its mere name is insufficient. No repeated owner
permission for the already-authorized repair is requested by this review.

Pending evidence: immutable fresh host diagnostic receipts; executable RAM mount
and shutdown graph verification; exact survivor SSH/helper implementation and
rehearsal; device busy-check validation; measured preservation budget/destination. No
repair, reboot, hardware acceptance or final recovery safety PASS is claimed.

Reviewer-only validation: git diff --check exited0; tools/ci.sh check --base
origin/main exited0 and printed check PASSED (0m13s) at this preparation checkpoint.
No duplicate full quick or target test was run by this reviewer.
