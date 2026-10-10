# Independent proposed return-path review

Design verdict: the manager's normal reboot return from RAM is supportable after
actual offline repair and validation. Execution remains pending those receipts.
This is not a claim that a repaired system has booted or management survived.
The owner authorized repair/reboot; the unresolved gates concern technical proof.

Matching systemd259.5 single `systemctl reboot --force` uses the manager's Reboot
method, bypassing normal reboot-unit dependencies. PID1 selects MANAGER_REBOOT,
then execs systemd-shutdown. That binary synchronizes, terminates processes,
unmounts/remounts filesystems read-only and requests a kernel reboot. It must be
copied with its complete matching dependency closure and executable in RAM;
verify PID1's manager connection before the final command. Double --force skips
cleanup and is not part of this plan.
[Matching systemctl manual](https://raw.githubusercontent.com/systemd/systemd/v259.5/man/systemctl.xml),
[Single-force dispatch](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/systemctl/systemctl-start-special.c),
[Manager reboot method](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/core/dbus-manager.c),
[PID1 shutdown dispatch](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/core/main.c),
[Final shutdown implementation](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/shutdown/shutdown.c).

Before considering return, independently verify actual clean offline e2fsck and
drive-health/error evidence, and fully copy/hash-check the budgeted metadata image,
undo, private config/auth snapshots and logs offhost. Metadata/undo do not recover
every regular-file byte or protect a power failure; unresolved media faults require
a different preservation/replacement decision before writes/return.

Mount the repaired root read-only/noload only for integrity inspection. Compare
original selected kernel/initrd and EFI/boot files, boot entry/current command line,
root UUID/fstab, selected management configs, SSH runtime and private auth
fingerprints to recorded pretransition evidence. Keep backup contents private.
Verify original boot does not select the RAM target; check unchanged interface
identity, management addresses/default routes and policy rules that normal network
startup will use. Unmount the inspection root, confirm it remains offline, and sync.

Remove only the reviewer/manager-identified owned /run/nextroot symlink after
checking its exact lstat/readlink identity; never recursively remove an unexpected
directory. Proposed final command, not executed by this reviewer:

```text
SYSTEMCTL_SKIP_AUTO_SOFT_REBOOT=1 SYSTEMCTL_SKIP_AUTO_KEXEC=1 systemctl reboot --force
```

The environment guards prevent automatic selection of another root/kernel; an
ordinary kernel reboot releases the RAM rescue. Previous successful original SSH
boot plus unchanged verified boot/config/auth and a clean repaired filesystem can
provide a concrete basis for the return decision. It does not prove the next boot
will succeed, so no such guarantee or mandatory missing-console claim is made.
If actual integrity/health/boot checks fail, remain in the tested RAM rescue and
report the precise missing artifact/recovery capability. After the final reboot,
acceptance requires actual authenticated original SSH, unchanged management and
routing, clean filesystem/kernel evidence; package installation/test follows that.

Current reviewer evidence is primary matching-source research and sanitized worker
reports only. No target root mount, transition, repair, reboot or firmware operation
was performed by this reviewer. A second userspace return is a separate proposal
requiring its own mount/runtime/SSH lifetime review.

## Actual .211 normal-return readiness verdict

APPROVE the exact reviewed single-force normal reboot under the parent's release.
This verdict concerns return readiness, not an observed successful original boot.
No target operation was performed by this reviewer.

Private post-repair-selected-integrity.json62147 B/0600
SHA25614af90cc4bc922c3e607dea8d190619bcc3af410e60870cbfa1c198d5e4078e1
independently verified:52 current metadata/hash/link comparisons pass. Reviewer
reconstructed the9 expected boot records directly from original-boot-config-baseline
and43 auth/config records from the preserved original-auth.tar.gz (unchanged
SHA256bf00f6dbd3ddcf7177322f8b3d9df3d2bd971221e2cf7e58b9613d59f9e0fccc),
then parsed raw NGFW_RECORD stat/hash/link outputs:46files,5directories,1symlink
all match. Original baseline octal mode strings were normalized to integers;
there is no permission mismatch. Credentials and backup contents were not printed.
All22 commands exit0/stderr empty; originalroot mount actually ro,norecovery,
nosuid,nodev,noexec and EFI mountro; both ordinary unmounts exit0. Actual fstab,
GRUB, kernel/initrd, commandline/root/EFI identities and firmware boot variables
are recorded privately, not claimed as exhaustive firmware validation.

Reviewer independently parses current six network JSON command outputs and the
immediate pre-transition-baseline.json: all six compare exactly equal, including
28addresses/28links/7IPv4routes/4IPv6routes/3IPv4rules/2IPv6rules. This is the actual
maintenance comparison baseline, distinct from older archival capture filenames.

Private post-repair-return-readiness.json29501 B/0600
SHA2569569441fa385f370b4f65414497324590d35c3a6a23e2234ff62289fc88b37e5
independently parsed: all8 commands exit0/stderr empty; root/PID1 root/executable,
systemd/executor/shutdown allRAMdev46. Reviewer independently matches all three
runtime binary hashes to the116-entry staged manifest:

```text
systemd          e547e7b09809e63fdaaef2a6a703f4db97d4a1f8c790d67c4d2438e6c7227228
systemd-executor d254a2e199cec12e63188f78bd073bafe89dc29fbe3c860c2d626fce2563bbf5
systemd-shutdown 8c02ee0ac53416b5e42f5cee603db1efd648961fb9449d759cbe448eaf5d52ac
Version=259.5-0ubuntu3
SystemState=degraded
rescue ActiveState=active SubState=running MainPID=6421 Result=success
runtime helper ActiveState=active SubState=exited Result=success
```

Actual loader-list resolves eight shutdown library/loader paths with no missing
dependency; subsequent stat confirms every pathRAMdev46. No destructive shutdown
binary test was executed. The responsive matching manager and successful minimal
rescue/helper support the reviewed manager Reboot dispatch despite the minimal
RAM system's degraded aggregate state; no concrete reboot failure was observed.
Originalroot/EFI ordinary-unmount and exact nextroot absence are confirmed, sync0.
Final audit259processes/93FDs/nsfs0/races0/failures0, finalguard0; additional guard0
after all helper and loader-path stat processes finish. Scoped completed repair,
five-pass readonly clean0, stable18 counter/exact kernel sample and independently
verified offhost metadata/undo/transcript are in the preservation report.

Operator/root received the actual verdict before publication so the prepared
target need not wait for another review cycle. Exact approved return command:

```text
SYSTEMCTL_SKIP_AUTO_SOFT_REBOOT=1 SYSTEMCTL_SKIP_AUTO_KEXEC=1 systemctl reboot --force
```

No double-force or second userspace return is approved here. Required next actual
check is authenticated original22 SSH, normal disk root/boot ID, management and
routing, filesystem/kernel health and private backup durability. Package
installation/activation/NIC-binding/hardware acceptance remain pending.

## Actual original .211 boot and management confirmation

Manager independently authenticated fresh original22 after normal reboot.
Reviewer independently reads manager-postboot-211.json1197 B/0600
SHA256e30e3b595bb12cdd6adf2f2b1e6d0b2c9d24348cba4f4522dedaae925c314cc6:
SSH exit0/stderr empty. Its actual script uses set-eu and explicit absent rescue/
nextroot tests, so the absence assertions are enforced. Selected output:

```text
boot_id 3a609803-be4e-46eb-bfc6-7dcfe385cf50
/dev/sda2 ext4
management enp4s0 source172.30.110.211 gateway172.30.110.1
PCI0000:04:00.0 driverigc IOMMUgroup28
Filesystem state: clean
ssh.service active / Result=success / ExecMainStatus=0
systemd-fsck-root.service active / Result=success / ExecMainStatus=0
```

Actual normal return and management confirmation PASS. This is stronger than the
earlier readiness verdict: original disk boot/SSH have now been observed. Worker
full postboot L3/DNS/current-kernel receipt and later NGFW installation/binding/
forwarding acceptance remain pending. The new boot resets kernel/device counters;
do not equate a counter after kernel reboot with the old-kernel18 baseline.

## .37 read-only return-proof source and actual selector failure

Full final return-proof.py SHA256
8772baa382446ccb4180efe8cc20185a6323a92c1934bf751d8dbe24de98fba1
independently read and hashed, outer/remote AST2 PASS. Operator branch remote
07aa11a114273f428073e5a86b3c1f37566f27f0 independently read back before approval.
This was source applicability for existing authorized read-only capture only.

Source pins own19-file boot baseline, auth/network archives and47-file RAM
baseline. Archive-derived regular/directory kind, UID/GID/mode and regular
bytes/hash compare, with final symlink no-follow metadata and target while
parents resolve inside original root. Available boot modes compare prior values.
Root ro,noload,nosuid,nodev,noexec and EFI ro,nosuid,nodev,noexec are measured;
ordinary unmount only and final full audit/exclusive guard0 remain required.
Current EFI hashes/MZ headers are explicitly current evidence without invented
historical equality. No reboot is performed by this source.

Actual capture subsequently refused a synthetic archive selector treated as a
real original filesystem path: return/root-shadow.record. Operator reports SSH1,
stdout0 B and stderr323 B, with both original mounts ordinary-unmounted in finally;
no normal return occurred. Reviewer/root initial all-tar-regular classification
missed this synthetic-artifact distinction. Exact source correction must exclude
generated artifacts from path/metadata comparisons and compare original passwd/
shadow selected root-record digests explicitly. Synthetic backup0600 mode cannot
prove original /etc/shadow metadata. Failure remains retained; authorized read-only
retry follows exact focused correction publication/applicability. No new owner
or manager permission loop is required. Actual successful capture and separate
normal-return verdict remain pending.
