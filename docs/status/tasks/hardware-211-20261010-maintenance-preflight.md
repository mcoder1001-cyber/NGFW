# Host 211 root-repair maintenance preflight — read-only

The owner explicitly requests disk/root-filesystem repair, with installation and testing remaining the original objective after recovery. This worker's current envelope authorizes fresh read-only diagnostics and recovery-path assessment only. No target files were staged, credentials read, packages installed, services stopped/restarted, networking/mounts/bootloader altered, or reboot/kexec/pivot/repair performed. Manager coordinates any transition or repair one host at a time after concrete independent safety review.

## Actual root/disk and backup evidence

Fresh strict-known-host BatchMode SSH succeeded. Observed root:

```text
/      /dev/sda2 ext4   rw,relatime
Filesystem state:         clean with errors
FS Error count:           1317
/dev/sda2: Inode 259596 has an invalid extent node (blk 15505493, lblk 0)
/dev/sda2: UNEXPECTED INCONSISTENCY; RUN fsck MANUALLY.
systemd-fsck-root.service loaded failed failed
```

The journal reader still reports Input/output error, and kernel ext4 directory-checksum warnings persist. The target clock remains April 18 while the task date is October 10; no clock change was attempted. These observed errors are not repaired by the current read-only phase.

Only disk `sda` is visible: 488397168 sectors (232.9 GiB), Samsung SSD 870 EVO 250 GB. EFI `sda1` starts at sector 2048, size 999424 sectors (488 MiB); root `sda2` starts at 1001472, size 124043952 sectors (59.1 GiB). Root identity remains UUID `343f6a24-1c79-4177-b7d0-52c8582914fb`; EFI UUID `9804-C768`. No additional block device, independent rescue filesystem or swap was observed. Existing mounted root has about 40 GiB free; this is not a safe independent backup destination for its own repair.

Existing SMART/health tools `smartctl` and `hdparm` are absent. SCSI sysfs device state is `running` with I/O error counter `0x6`; filtered warning/error kernel log shows ATA model/quirk/DRM notices, no captured link-reset or medium-error line. This does not establish drive health or explain the counter. SMART-level health remains unverified; no diagnostic package or device-write operation was used.

Private configuration backup was independently rechecked using metadata/hashes only: directory 0700, manifest 0600, all 24 recorded stdout/stderr artifacts 0600 with matching hashes, allowed archive scope and gzip exit0. Manifest SHA256 remains `3efddb28c12ba8388366329d5c020de60809ca92620d77124957090962f347f6`. This is configuration/network preservation only, **not a full data/system backup**. The existing nftables snapshot gap remains explicit. No private payload or credential is committed.

## Existing maintenance capabilities

| Item | Actual observation |
|---|---|
| Platform | Bare metal (`systemd-detect-virt` reports none), INTEL Q670 |
| PID 1 | systemd 259, package `259.5-0ubuntu3` |
| Advertised commands | Installed `systemctl --help` includes soft-reboot and switch-root; no action was invoked |
| Memory | MemAvailable 31444956 KiB at detailed probe; no swap |
| RAM mounts | `/run` about 3.2 GiB, noexec; `/tmp` about 16 GiB, executable tmpfs; `/dev/shm` tmpfs |
| Repair tools | Existing `/usr/sbin/e2fsck` and `/usr/sbin/fsck.ext4`; dynamic dependencies resolve |
| SSH runtime | `/usr/sbin/sshd` and separate `/usr/lib/openssh/sshd-session`, `sshd-auth`; dynamic dependencies resolve |
| Hidden BusyBox | `/usr/lib/initramfs-tools/bin/busybox`; dynamically linked to libc and ELF loader, not self-contained static rescue |
| BusyBox applets | sh, ash, mount, umount, switch_root, ip, chroot, sync; no pivot_root or e2fsck in the queried applets |
| Initrd | `initrd.img-7.0.0-22-generic`, 76384759 bytes, listing exit0; tool entries busybox, ip, pivot_root, switch_root; no SSH/dropbear/e2fsck tool entry matched |
| Missing standalone tools | busybox in PATH, dropbear, kexec, pivot_root, switch_root, smartctl, hdparm and ipmitool not found in PATH |
| BMC | No `/dev/ipmi*` or `/sys/class/ipmi` device observed; no ipmitool, so no BMC query possible |
| Kernel controls | kexec_load_disabled=0, lockdown none; kexec executable absent; no kernel transition performed |
| Privileges | Broad root effective/permitted/bounding capabilities observed, including SYS_ADMIN; authority is still constrained by the reviewed recovery envelope |

BusyBox requires `/usr/lib/x86_64-linux-gnu/libc.so.6` plus the dynamic loader. Existing e2fsck requires libext2fs, libcom_err, libblkid, libuuid, libe2p, libc and loader. Resolved `ldd` output is runtime linkage evidence, not proof of a complete RAM root: SSH authentication/NSS/PAM/key files and helper execution require separately reviewed preservation. No credential files were read or copied in this phase.

## Old-root holders and recovery constraints

Read-only `/proc` metadata inspection found 12 processes holding the existing root through root/cwd/executable, open file descriptors or mappings. PID 1 alone has root/cwd/executable on the old ext4 filesystem, 1 old-root file descriptor and 105 mapped regions. No environment values, command arguments or credential contents were collected.

```text
pid1: systemd
old_root_holder_processes: 12
pid1 root_on_old_fs/cwd_on_old_fs/exe_on_old_fs: true/true/true
pid1 fd_on_old_fs: 1
pid1 maps_on_old_fs: 105
```

A chroot or auxiliary bind mount does not release these references. A read-only remount or lazy detach does not prove that root is offline for repair. The advertised systemd transition commands and available RAM are capability facts only; no safe transition has been demonstrated. A usable RAM-maintenance path must have independently reviewed PID 1 transition, complete runtime/authentication dependencies, preserved management address/routes and working fresh SSH, then actually release old-root holders and successfully unmount the old root in every relevant namespace before e2fsck. It also needs a verified failure/recovery path and appropriate data protection beyond the configuration-only backup. None of these gates has been executed or declared passing.

Management was rechecked through the existing controller route; the protected interface/address/default gateway are unchanged from the private network capture and task envelope. No management port, NIC ownership, route, address or network-daemon action was altered. No secondary rescue or BMC path is currently verified.

Independent reviewer requested an additional read-only namespace/mount/network-manager probe. Root device major:minor is `8:2`. Five visible mount namespaces were found; four contain the old root device (counts 1, 1, 1 and 4), one kernel namespace has no such mount. The main PID 1 namespace still mounts old root once. This confirms that an unmount check limited to the calling shell's namespace would be insufficient.

Selected actual `systemctl show run.mount tmp.mount` output:

```text
Id=run.mount
ActiveState=inactive
SubState=dead
FragmentPath=
Where=
What=
Options=

Id=tmp.mount
ActiveState=active
SubState=mounted
FragmentPath=/usr/lib/systemd/system/tmp.mount
SourcePath=/proc/self/mountinfo
Where=/tmp
What=tmpfs
Options=rw,nosuid,nodev,size=16313080k,nr_inodes=1048576,inode64,usrquota
```

The live `/run` tmpfs is observed by findmnt despite its inactive/nonconfigured mount-unit object; its actual flags include noexec. `systemd-networkd.service` is active/running, Restart=on-failure, KillMode=control-group, PrivateMounts=no, no RootDirectory. No explicit `KeepConfiguration=` declaration was found in current `/run/systemd/network/*.network` or `/etc/systemd/network/*.network`. This is not proof that a transition preserves addresses/routes; no stop or transition was tested. Current networkctl output is retained privately without exposing configuration contents.

## Durable command/output records

Actual read-only probes used fixed SSH options `BatchMode=yes`, `StrictHostKeyChecking=yes`, `ConnectTimeout=10`, with existing target tools: findmnt/lsblk/systemctl/journalctl/dumpe2fs/dmesg/date/IP route, sysfs/proc metadata, systemd-detect-virt, command availability, capsh, ldd, lsinitramfs and existing BusyBox `--list`. No service action or package installer was invoked. The detailed fixed-stdin Python collectors stream directly to private controller files:

| Private diagnostic artifact | SSH exit | Bytes | SHA256 | Capture stderr |
|---|---:|---:|---|---|
| maintenance-preflight.json | 0 | 17310 | `572740c23c8b46d9b1887d3bf23ee30d4f941bbab933ee05af7153a3cf6f0f66` | empty |
| maintenance-tool-preflight.json | 0 | 1924 | `fdb313e180ffcfe2846ecbe1083a65e5177272abc4524b5364bd559acd551e2a` | empty |
| maintenance-ns-network.json | 0 | 11113 | `f742327b2b860f84126fe2ca692d358e31c32916abc134299c87d0d851b2b9a0` | empty |

All diagnostic files and their error outputs are 0600 under the existing private host-211 controller directory. Public evidence is limited to nonsecret capability/disk facts and selected diagnostic outputs above. Independent recovery-safety reviewer received sanitized facts and private filenames for review. Current outcome: **RAM maintenance not verified; repair/install/reboot NOT RUN**. Preserve branch/worktree for the manager's reviewed next step.


## Additional read-only geometry/auth/runtime check

Mounted root exclusive-open safety probe `os.open("/dev/sda2", O_RDONLY|O_EXCL)` failed with errno16/EBUSY as expected. It did not open exclusively or write any disk bytes. Filesystem geometry read from its superblock is 15505494 blocks × 4096 bytes = 63510503424 bytes, exactly the existing partition size. Reported invalid extent block15505493 is the final valid filesystem block. A read-only 4096-byte read succeeded; the entire block is zero, SHA256 `ad7facb2586fc6e966c004d7d1d16b024f5805ff7cb47c7a85dabd8b48892ca7`. This supports a missing/invalid extent node observation, not a diagnosis of physical-drive health.

Existing `e2image` and `e2undo` executables and their linkage are available. Selected effective `sshd -T -C user=root,...,lport=22` exited0: current host uses standard `/etc/ssh/ssh_host_{rsa,ecdsa,ed25519}_key` paths, `.ssh/authorized_keys` and `.ssh/authorized_keys2`, no AuthorizedKeysCommand. No credential content was exported. Original SSH allows password authentication; a separate RAM rescue configuration must explicitly require publickey and disable password, keyboard-interactive and PAM without changing the original configuration.

Configured APT offers smartmontools7.5-2 via an Ubuntu mirror. Trusted provenance still requires cached InRelease verification using the Ubuntu archive keyring and exact Packages-index/package hash validation before a RAM-only download/extract. No apt update/install or disk-health control command has run.

Private `maintenance-geometry-auth.json`: SSH exit0, 55247 bytes, SHA256 `b1f9f70487bbdeebc8d3cbd527c834b38d73883cd3a6ba313d08078013bd8547`, empty capture stderr, mode0600. This contains only detailed diagnostics and selected effective SSH paths, not credential contents.

## Explicitly authorized reversible RAM staging — pending execution

Manager now authorizes a dedicated executable tmpfs at `/run/ngfwrescue` with an explicit mount unit `DefaultDependencies=no`, no umount.target relationship. `/run/nextroot` must remain absent. Prepare matching systemd259.5, SSH daemon/session/auth helpers, minimal local NSS/shell/repair/metadata tools and dependencies; copy credentials only on-target into RAM privately, with a separate key-only listener bound exclusively to management IP172.30.110.211:2222. Preserve original port22 and all network/disk/boot settings. The candidate default target activates only rescue SSH, with no generators, networkd, udev, firewall, fstab or disk boot units. Test authenticated chroot SSH and runtime root/exe/maps/fd metadata, unit verification and byte budget; stop exactly the test daemon/processes before any reviewed transition. Chroot testing proves staging only, not PID1 transition or old-root release. Soft reboot, root handover, KeepConfiguration alteration, reboot and fsck remain unauthorized pending concrete independent review.
