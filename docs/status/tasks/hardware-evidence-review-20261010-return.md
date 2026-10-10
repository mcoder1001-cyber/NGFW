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
