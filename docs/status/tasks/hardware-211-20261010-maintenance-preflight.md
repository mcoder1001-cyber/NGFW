# Host 211 root-repair maintenance preflight — read-only

The owner explicitly requests disk/root-filesystem repair, with installation and testing remaining the original objective after recovery. The initial phase below authorized only read-only diagnostics. Subsequent explicit manager authorization permits reversible RAM staging and isolated rescue SSH; actual scoped target changes and runtime tests are recorded in the final receipt below. No disk repair, root transition, reboot, original service/network change or package installation has run. Manager coordinates any transition or repair one host at a time after concrete independent safety review.

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


Manager's source review caught `SurviveFinalKillSignal=` in the wrong section before any target staging. Candidate is corrected to [Unit], with explicit normal-reboot/rescue conflicts and Before=shutdown.target/rescue.target/emergency.target. Both service variants have After=basic.target; candidate supplies an inert DefaultDependencies=no basic.target, with no disk/network pull-in. RAM maximum is increased from1GiB to8GiB for bounded metadata/undo; allocation is measured, not assumed. Source review and actual property/parser/FD checks are required before transition.

Additional read-only integrity probe `dpkg -V e2fsprogs systemd openssh-server libc6 libsystemd-shared` exited0, reporting346 missing documentation/manual/lintian assets only; no executable/library checksum difference or missing path was reported. Kernel command line has no systemd.unit/rd.systemd.unit override. Repeated mounted-negative O_RDONLY|O_EXCL|O_CLOEXEC open fails EBUSY16. Private maintenance-integrity.json20391bytes SHA256 `649b32242ad8b1e0f5a36d6006f78e88b1670aed86da80bca46b99bb0ef0a036`, empty stderr/mode0600.

Ubuntu package-cache provenance read-only verification: gpgv exited0 with the Ubuntu Archive Automatic Signing Key2018; cached InRelease Origin/Label Ubuntu, Suite/Codename resolute. Decompressed cached Packages7581722bytes SHA256 `92eb335c0ed27983d41a5a0efdf0edce3e67bfa34e8e987af9df260ca72d9bf9` matches the signed release's exact index. Private smart-provenance.json1358bytes SHA256 `fb4e53c5a825b483f438e06387eb897f87f2a6f943ee5179ef8b618c75091426`, empty stderr/mode0600. Package itself not downloaded/extracted yet; SMART not run.


Independent R7 also blocked the initial source and required removing bootstrap WorkingDirectory= to avoid implicit mount Requires/After; chroot itself changes cwd to RAM and runtime audit must prove it. Candidate adds a private virtual-only devpts bind, alongside devtmpfs/proc, and controller authenticated PTY test. Address lifetime counters are excluded from semantic network hash; any remaining mismatch is investigated, never corrected by changing networking. This second source correction also precedes target staging.


## First actual reversible RAM stage attempt

R7 approved corrected source c4027e964/script517d789e for staging only. Exact script ran and exited1 early: standalone recursive `ldd` of private libsystemd-core loses the executable's RUNPATH and reported libsystemd-shared unresolved. Dedicated8GiB executable tmpfs exists at `/run/ngfwrescue` (rw,nosuid,nodev,relatime; no noexec); only initial binary copies exist. No credentials, virtual-device binds or rescue daemon were staged yet. This is a staging collector failure, not demonstrated source binary corruption. Correct collector uses each executable's flattened complete ldd closure, checks unresolved dependencies once at that executable, and copies every resolved library directly. Independent applicability review precedes cleanup/retry. Original22/network remain intact, nextroot absent; no transition/repair.

Private first attempt `ram-stage.json`:2419bytes SHA256 `2001172ff24e6bace825e527db2d52013da751139c7a4daedb1a142bd60d8d66`, script SSH exit1/empty stderr/0600. Preserve this actual failure; do not overwrite it or claim stage PASS.

Owner/manager explicitly extended backup scope to original authentication before repair. Read-only off-host `tar --acls --xattrs --numeric-owner` of `/etc/ssh`, `/root/.ssh`, account/group/shadow/NSS files and `/etc/pam.d` exited0. Private archive35240bytes SHA256 `bf00f6dbd3ddcf7177322f8b3d9df3d2bd971221e2cf7e58b9613d59f9e0fccc`;43 allowed members verified readable, empty stderr,0600, outside Git/candidate bundle. Credentials are never printed or committed. This supplements network preservation, not a full data backup; restoration is conditional on actual auth damage and separate reviewed permission.

Read-only debugfs ncheck exited0: invalid extent inode259596 is a user journal file, inode259599 is `/root/.cache`; full filenames retained privately. Header has3845088×256=984342528bytes of inode tables,474 groups,64MiB internal journal. Rough metadata budget is around1GiB before directory data; actual metadata/compression/undo sizes remain unmeasured.


Focused R7 raised a possible literal backslash-n separator concern before retry. Independent local AST inspection does not reproduce it: both join separators are one character ordinal10; shadow/NSS/hosts constants contain real ordinal10 newlines and zero literal backslash-n; SMART stanza separator is two characters ordinals10,10. Source hashes are unchanged by the attempted normalization because the actual files already contain correct escape constants. Reviewer receives this concrete evidence for reconciliation before retry; no target NSS files were created by the first early failure.


Second attempt: reviewed owned-partial cleanup exited0. Corrected collector completed tree/dependencies/private keys/virtual mounts, but failed before daemon/unit activation because the implementation used `/usr/sbin/chroot`, which is absent on this target. Source now discovers and validates `/usr/bin/chroot` or `/usr/sbin/chroot` before mutation and uses that exact path for checks/bootstrap and SMART execution. Rescue session PATH is explicitly local sbin/bin. Second private ram-stage-retry.json3496bytes SHA256 `d1d86702ff95fe9c9851d4888e9c5ff09a5b647f6777ae71dc9f8d269976ee60`, SSH exit1/empty stderr. No rescue listener passed yet; no transition. Virtual mounts/private tree cleanup and exact corrected-source retry require focused reviewer applicability.


Reviewed design adds an inert RAM-only runtime preparation oneshot to the new root: mkdir/chmod/chown `/run/sshd` with explicitly validated RAM tool paths, DefaultDependencies=no, Before=rescueSSH and RemainAfterExit=yes. Candidate default target Requires/After both preparation and SSH; candidate SSH After preparation. It addresses the later coldplug case where transferred old `/run` hides staged `/run` and original ssh.service shutdown removes its RuntimeDirectory. No original ssh unit override or network change is applied. Staging unit verification includes this new helper; transition remains separately gated.


Read-only original boot/config baseline captured9 existing files successfully, no read failure; private4925bytes SHA256 `2e3af5ef6c75edbbadcbb4467e798da93adff516ca9a4e3257d3cddb70a38958`, empty stderr. Expected paths absent from glob are not automatically a complete baseline; EFI/GRUB scope needs explicit presence assessment before return-boot claims. Original ssh.service has RuntimeDirectory=sshd, RuntimeDirectoryPreserve=no, KillMode=process, no ExecStop/ExecStopPost reported, confirming runtime-directory lifetime risk. Kernel command line has no default-unit override.

R7 focused runtime-helper review approves the RAM staging design, with explicit private append log for oneshot added to avoid a default journald socket dependency. Both logs are0600 inside RAM. Actual helper commands will be exercised in the chroot during runtime tests; no transition follows staging approval.


Third stage attempt stopped on helper-path validation before SSH activation: host Path.exists resolves the candidate's absolute BusyBox applet symlink against the host, where `/usr/bin/busybox` is absent. Correct validation uses lexists followed by actual chroot execution of mkdir/chmod/chown on RAM `/run/sshd`; it never creates the path in original root. Private third attempt3438bytes SHA256 `6dc81d156092ced825bac60598b7c13827466d98e4786fc3a92b91d6e475a726`, exit1/empty stderr. Actual failed attempts remain recorded and no stage PASS or transition claimed.


Additional nonmutating chroot probe proves staged sshd -t and matching systemd --version each exit0; helper combined executable check fails because the trimmed initramfs BusyBox lacks chown. Exact path metadata confirms missing chown while mkdir/chmod/tty/stty links exist. Source now explicitly copies genuine host mkdir/chmod/chown executables and flattened dependencies rather than assuming these critical helper applets. Locale C is explicit in staging/rescue session/service. Private runtime-check1072B SHA256 `245c5803c4205b5678b0196780a5bece470440dcea10e256bacb679777a1dc87`; applet-path metadata SHA256 `3a2551303d4442fa995af3e712de43992b29b7b630b8f27cfff3997270b7202d`. No additional target modification or daemon activation from these probes.


## Actual RAM staging, authentication and health receipt

Exact candidate source SHA256 `e33b95eacf965361d2dc3ba05866e3ee80d9eb1cd6259679fa7d3582a34166d6` published/read back at local/remote `2ee35a45fa9ca67fb4dd556f4fc5a724d70a125d`; independent R7 approves scoped RAM-only cleanup/staging. Last stage exit0, original SSH22 authenticated exit0, separate key-only management2222 authenticated command exit0/UID0. Authenticated persistent PTY `/dev/pts/0`, shellPID6459, controller session71683 remains open under manager's survivor plan. Earlier historical stop-test-before-transition instruction is superseded by the reviewed survivor plan: preserve this tested same-cgroup RAM session for any separately authorized transition.

R7 independently confirmed correct newline ordinals10/zero literal backslashes and withdrew its escaped-output interpretation at reviewer checkpoint31872b92; no source newline correction was needed.

| Actual runtime property | Evidence |
|---|---|
| Rescue daemon | PID6421 active/running, regular runtime unit, no transient/control/drop-in override |
| Unit survival | SurviveFinalKillSignal=yes, IgnoreOnIsolate=yes, DefaultDependencies=no |
| Dependency closure | Requires=system.slice only; no Wants; After=basic.target/system.slice; ordinary reboot/halt/kexec/poweroff/rescue/emergency conflicts; Before=shutdown/rescue/emergency |
| Namespace/sandbox | daemon/session helpers/bash share PID1 mnt:[4026531832]; RootDirectory empty, PrivateMounts/PrivateTmp/ProtectSystem=no |
| Process references | Four persistent cgroup processes6421/6456/6458/6459: root/cwd/exe and all file maps RAM; old-root file descriptors/maps zero; unreadable FD count zero |
| Logs/terminal | daemon FD1/2 point to private RAM rescue-ssh.log; PTY FDs are virtual devpts; other FDs sockets/virtual devices |
| Runtime helper | RAM-only mkdir/chmod/chown each actual chroot exit0; helper/default target verification exit0 with empty stdout/stderr |
| Mount | Dedicated8GiB executable tmpfs; private virtual dev/devpts/proc binds, no old-root bind |
| Capacity after SMART/guard/auth backup |124469248 bytes allocated;8465465344 bytes available; MemAvailable31325548KiB |
| Network | Semantic addresses/links/all-table routes/rules hash unchanged before/after: d287533c93d6c62e3c54610ec9f1547b07365754918b1f7d8eeac0e12e013836 |
| Next root marker | `/run/nextroot` ABSENT |

Actual nonsecret final tree manifest after SMART tools/new libraries, static guard and private auth preservation SHA256 `53af8f3dfd5637efa1fb1664ceeb6e839f48521a4733f72f9d8747044feefaba`; private credentials/backups/logs are excluded. Original source libs are never overwritten by the SMART stage. All private command/audit/error artifacts remain0600 under0700 host-211; no backup/auth/SMART serial contents are committed or printed.

Approved static read-only guard copied into RAM and SHA256 verified `02a2270ad8e0f480efc7464b26d2216fec8a915aa12755da6339015abed0f41c`; actual `/usr/bin/block-check /dev/sda2 8 2` mounted-negative exits3: `BUSY: block device still has an exclusive holder; no repair permitted`. No positive-offline claim is made. Original auth archive is also privately copied to RAM,0600/hash matches controller baseline `bf00f6dbd3ddcf7177322f8b3d9df3d2bd971221e2cf7e58b9613d59f9e0fccc`; original files/configuration are unchanged.

Signed cached Ubuntu smartmontools7.5-2 amd64 package downloaded/extracted solely in RAM,664482bytes SHA256 `ab211f171a9595f6b9686caaec44ca07a623c4605923caae756e2446e055e0ff`, full signature/index/deb chain validated; no apt update/install. RAM smartctl binary SHA256 `58afd627df25e1ccfbce81503052bfe2b921f2a083878c53d480706e12c4c5c9`. Only `smartctl -x -j /dev/sda` information query ran, exitbitmask0. Samsung870EVO250GB, firmwareSVT02B6Q: SMART statuspassed, reallocated/reserved/programfail/erasefail/runtimebad/uncorrectable/ECC counters0. CRC attribute199=1003, powerhours3953, poweron-recovery252. Extended ATA error/self-test logs count0; SATA PHY reset=false, CRC-error counters0, PhyRdy→NRdy392 and COMRESET6. These values do not establish filesystem integrity or explain existing corruption.

Fresh SCSI I/O-error count is0x9 versus early baseline0x6. No filtered current-boot ATA/SCSI reset/I/O/UNC line is observed; whether this increment reflects information-query rejection or actual device errors is unverified. Manager notified immediately; **corrective writes remain held**. Repeat read-only SMART/PHY/SCSI/kernel comparison after any separately authorized offline metadata read; any new media/interface/I/O error blocks correction pending review. No counters are reset and no SMART enable/self-test command runs.

Controller actual capacity:249257984 disk bytes free and1977929728 `/dev/shm` bytes available. Metadata estimate around1GiB fits dedicated RAM budget, but actual compressed off-host image size is not measured. Compressed image must be measured against current controller capacity and verified off-host before corrective writes. This is configuration/auth preservation plus planned metadata/undo, **not full system/data backup**; no image or undo exists yet.

| Private actual receipt | Bytes | SHA256 |
|---|---:|---|
|ram-stage-complete.json|29866|`648753664f64800496978dff2b6b96886277d8e216d385e46ed37aca25a4edc6`|
|ram-runtime-audit.json|11076|`184223796a22822088d8f0122c458f20d89e6c337dae5ac73e88b0ae7478e981`|
|ram-final-baseline.json|20105|`ab3293c9e2403b1b659bf06b632663e7f553e3852587d087316a83a76fbe8092`|
|smart-result.json|78124|`dbb9a1dec811c0b24d13056c55743d4a770da48f415671c6ea52a252f7ba07b9`|
|block-check-transfer.json|320|`d9739d4ab82c60bac055c1ca8e388407b2357742a8ffdcc7fd0f58468cf51b97`|
|original-auth-ram-transfer.json|192|`803596e7d24a88107de56790fa694f0c416985bc83c581d79f80afdd52c683e3`|
|original-boot-config-baseline.json|4925|`2e3af5ef6c75edbbadcbb4467e798da93adff516ca9a4e3257d3cddb70a38958`|

Current outcome: reversible RAM staging/authentication/runtime tests PASS; formal TRANSITION-ONLY review pending. Root remains mounted; positive offline proof, fsck/image/repair/reboot/install and hardware acceptance NOT RUN. Parent coordinates one host at a time and any exact marker/soft-reboot action; staging approval does not authorize transition or correction.

Actual units from private command receipt (no credentials):

mount:

```ini
[Unit]
Description=NGFW reviewed RAM rescue staging
DefaultDependencies=no

[Mount]
What=tmpfs
Where=/run/ngfwrescue
Type=tmpfs
Options=size=8G,mode=0755,nosuid,nodev,exec
```

runtime:

```ini
[Unit]
Description=NGFW RAM key-only management rescue SSH
DefaultDependencies=no
IgnoreOnIsolate=yes
SurviveFinalKillSignal=yes
After=basic.target
Before=shutdown.target rescue.target emergency.target
Conflicts=reboot.target kexec.target poweroff.target halt.target rescue.target emergency.target

[Service]
Type=exec
KillMode=control-group
Restart=on-failure
RestartSec=1s
Environment=LC_ALL=C
ExecStart=/usr/bin/chroot /run/ngfwrescue /usr/sbin/sshd -D -e -f /etc/ssh/sshd_config
StandardOutput=append:/run/ngfwrescue/var/log/rescue-ssh.log
StandardError=inherit
```

candidate:

```ini
[Unit]
Description=NGFW RAM key-only management rescue SSH
DefaultDependencies=no
IgnoreOnIsolate=yes
SurviveFinalKillSignal=yes
After=basic.target ngfw-runtime-prepare.service
Before=shutdown.target rescue.target emergency.target
Conflicts=reboot.target kexec.target poweroff.target halt.target rescue.target emergency.target

[Service]
Type=exec
KillMode=control-group
Restart=on-failure
RestartSec=1s
Environment=LC_ALL=C
WorkingDirectory=/
RuntimeDirectory=sshd
RuntimeDirectoryMode=0755
ExecStart=/usr/sbin/sshd -D -e -f /etc/ssh/sshd_config
StandardOutput=append:/var/log/rescue-ssh.log
StandardError=inherit
```

runtime_prepare:

```ini
[Unit]
Description=Create OpenSSH runtime after original run transfer
DefaultDependencies=no
Before=ngfw-rescue.service

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/bin/mkdir -p /run/sshd
ExecStart=/usr/bin/chmod 0755 /run/sshd
ExecStart=/usr/bin/chown 0:0 /run/sshd
StandardOutput=append:/var/log/runtime-prepare.log
StandardError=inherit
```

ngfw-rescue.target:

```ini
[Unit]
Description=NGFW RAM maintenance target
DefaultDependencies=no
Requires=ngfw-runtime-prepare.service ngfw-rescue.service
After=ngfw-runtime-prepare.service ngfw-rescue.service
AllowIsolate=yes
```

basic.target:

```ini
[Unit]
Description=Minimal inert RAM basic target
DefaultDependencies=no
```


## Bounded read and counter classification follow-up

Manager authorized bounded raw read and exact read-only SMART repeat. Existing `/usr/bin/dd` is uutils-coreutils0.8.0. Exact `dd if=/dev/sda2 of=/dev/null bs=1M count=256 iflag=direct status=none` exits1 immediately0.003s with Invalid input; I/O count9 remains9 and no new storage kernel error. This is a failed diagnostic invocation, not successful raw-read evidence. Read-only SMART-x-j repeat then exits0 and reproduces exactly+3 ioerr9→12; CRC1003/media0/extended errorlog0 and PHY baseline remain unchanged, no new kernel storage line. This supports a query-associated counter increment; the responsible optional query is not identified and physical health is not guaranteed.

To test actual direct reads with an aligned buffer, existing Python opens `/dev/sda2` O_RDONLY|O_DIRECT|O_CLOEXEC and uses preadv into anonymous page-aligned RAM.256×1MiB=268435456 bytes read successfully in0.599s, exit0, ioerr12→12, no new ATA/SCSI reset/I/O/UNC/failed/timeout line. No raw bytes are printed or saved and no disk write occurs. Private bounded-direct-read.json513bytes SHA256 `844194cbfb98cc19e73d1eceeafe1e11ede7b54de9979487f570ac2c6af08b18`; source1428bytes SHA256 `5784de633c8d0e797728c61da2df437ea74be6f9bb783ebf47f6c91d0d92c6d4`; empty capture stderr. Private ioerr-classification.json682196bytes SHA256 `37844f3873bc07b07a7a09a997cb59dc6107cdc09247924fab9d6078da6d6eba`, with full SMART/kernel logs kept confidential.

This scoped read supports the manager/reviewer's TRANSITION-ONLY → positive offline guard/all-namespace/holder proof → read-only fsck/metadata-image assessment. It does not authorize correction. After actual metadata read, repeat counters/CRC/media/new kernel events and compare raw-read baseline separately from SMART-query increments. Any genuine new media/interface/read/reset failure holds writes for review. Existing root corruption and original package-install objective remain unfinished.

## Network stop premise and RAM audit tooling checkpoint

Read-only network-stop-preflight.json93954B SHA256 `5803a360e3aa124a54b3aeb7e0fe52d18ecaa88a6dec2470c85424cb25bd1c2f` records effective management network metadata, detailed addresses/all-table routes/rules and actual networkd service properties privately. All four observed addresses have infinite lifetimes and no dynamic flag; IPv4 routes are kernel6/static1, IPv6 kernel4, no DHCP/RA route or lease file. Management remains enp4s0/172.30.110.211/24 with static IPv4 configuration. networkd is active; no ExecStop/ExecStopPost or service drop-in override, KillMode=control-group, KillSignal=15, SendSIGKILL=yes, no private mount namespace. No KeepConfiguration edits or network reload performed.

ACD follow-up network-acd-preflight.json13208B SHA256 `84a51a1ee4239981d1af06c8499a42328398ae29ea7e210d0295354d43ebe54c`: matching, generic and prefix .network.d directories absent under /etc,/run,/usr/lib,/usr/local/lib; actual NetworkFileDropins is empty. Effective matching file has no DuplicateAddressDetection/DHCP/IPv6AcceptRA/KeepConfiguration override. Actual networkctl IPv4 source/state is static/configured. R7 independently checked matching systemd259.5 IPv4 DAD default and stop behavior, closing static-address ACD removal exception. Journal query exits0 with no ACD/duplicate/conflict line; missing debug logs alone do not establish the premise. `networkctl cat enp4s0` exits1 because this syntax expects a configuration name; actual JSON and explicit inventory provide effective metadata instead. Config contents remain private.

Minimal static namespace helper source hardware-211-20261010-nsfs-check.c SHA256 `36e12485aa4e1ca3a85e0a1760fff89604c0d4b3f9b5b190aeee873bc45a66ee` compiles with staticgcc/O2/Wall/Wextra/Werror without warnings; binary1014992B SHA256 `2907bb0345fb9dad199e48b2539ce9038b24fbafa00fa9672b4074239a6ac681`. Independent R7 rebuild is byte-identical. It inspects nsfs type, changes only a short-lived child's mount namespace, and refuses old-root mounts, empty/unreadable mountinfo or unresolved namespace-file mounts. Its mountinfo is filtered by the RAM process root and is reference inspection, not an exhaustive view of detached old mounts. Final exact O_RDONLY|O_EXCL blockguard is independent and decisive; it must run only after namespace helpers exit.

Actual RAM-held mount-namespace FD reports fstype nsfs/type0x20000, oldroot mount count1, namespace helper exit3; non-nsfs input refuses2. Whole audit while original root is mounted exits2 as expected:271 processes/291 FDs/nsfs0/races0,1310 refusal observations include known old-root references and absent staged sysfs. This negative audit is not offline proof. Private audit-tool-validation.json88600B SHA256 `395effcb2564445ffaafe202de946f8b1e43671f2c6882f7ba92ec8ab044e3b8`, no capture stderr. Selected RAM stat/readlink/cat and Bash associative arrays execute successfully. Final checked mount-namespace selector/syntax validation also exits0/empty: audit-final-source.json729B SHA256 `97d73bc53b2181c7fef21ea08a589610312db0d096f9a5518665cbe149a8d76f`.

Exact final RAM Bash audit source SHA256 `e5608afb712bd86251395800a4e27d9d7f1d8df75d75d0f2c41d827ada89affc`. It records process root/cwd/exe, mapping devices, FD stat device/rdev/inode and fdinfo mount IDs; checks readable nonempty userspace mountinfo and mount namespace IDs; distinguishes kernel threads; inspects/deduplicates nsfs FD references; refuses unknown bound nsfs, unreadable entries/races or missing sysfs holder directories. A bounded fresh retry is permissible only for inconclusive process disappearance. It does not mount, change network/services, repair or open original disk for writes. Positive offline proof remains NOT RUN.

Updated nonsecret tree manifest `8eb22416d7c0e3ce80d7e2cce705f20bcdce0178365c4fba641ff7630defa458`; RAM125493248 allocated/8464441344 free of8589934592. Private ram-audit-final-baseline.json20284B SHA256 `5f477e33366167baee53d30fa1ea7341ae7aaa894dd5c68e96322b9d1a53efd6`. ioerr remains0xc. Current controller actual free6857019392B (unprivileged available2555531264B), shm1907728384B. All10 new private diagnostic source/output files copied from own0700 tmpfs into own0700 controller disk directory,0600, fsync and SHA readback verified. No backup/log contents enter Git. Fresh .37 original22/management route and held RAM PTY verified by its owner at09:30UTC; no .37 transition.

Manager phase release86c0ba96f1ee809de93a5fab5dc4538bf297d351 authorizes exact owned nextroot marker plus ordinary soft-reboot only after final source applicability, publication, live session and capacity gates. Then require RAM PID1/executor, runtime helper, fresh key-auth2222, unchanged management/routes/rules, reference audit and guard0 before e2fsck-f-n and measured seekable RAM metadata image/private durable compressed offhost copy. Keep RAM PTY71683 alive. No corrections, return reboot, package activation or original-file deletion in this release. Transition/offline diagnosis/image/repair/install/hardware acceptance remain NOT RUN at this checkpoint.

## Actual transition and read-only diagnosis

Audit-tool checkpoint local/remote matching `f5018b5710dcc78a602e478c19565f81d441c83b`, clean worktree/readback, R7 applicable approval obtained before crossing. Original22 ordinary soft-reboot command exits0/empty at09:34UTC. Held RAM PTY71683/bash6459 survives. Actual PID1 root and executable are tmpfs device46; matching executor FD9 also device46, hash `d254a2e199cec12e63188f78bd073bafe89dc29fbe3c860c2d626fce2563bbf5`. Sysfs mounted; six network sections (addresses/topology/all IPv4+IPv6 routes/rules) compare equal to pre-transition baseline. Exact owned nextroot link removed after actual RAM proof.

First fresh2222 auth returns255 because /run/sshd is initially absent during startup. Diagnosis stopped at this failed gate. Held PTY read-only inspection then finds reviewed helper activating and directory created; later helper active/exited success0 and owner0:0/mode0755. Fresh2222 auth subsequently exits0/empty. No manual directory repair or surviving SSH restart needed. Failure is retained, not silently relabelled PASS. Private post-transition-fresh-auth.json577B SHA256 `8d8935aab15f80eadaf75926fae80bde34d6e7de24d63abb9807f4495d8142f7`; network/runtime receipt22250B SHA256 `1dffaa1a0bf0da26871aa3a128306c381c4e6a0a684407dd6694db521791142c`.

Actual first stable offline audit passes:267 processes/93FDs/nsfs0/races0/failures0, then final exact blockguard0. Private post-transition-offline-audit.json25017B SHA256 `b74d8312d66acfcf7483a83a05cada8da896a5ed9ccd99e73a720b5c27cf61d6`, SSH0/no stderr. No arbitrary descriptor close, forced unmount or hidden retry. This permits the already released read-only diagnosis only.

`e2fsck -f -n /dev/sda2` exits12 (4|8: errors uncorrected plus aborted): invalid extent inodes259596/259597/259598 at blocks15505493/15503361/15503362; directory259599 corrupt at block0/offset0 and salvage declined under-n. Pass2 aborts. Incidental inode259816 extent optimization is outside targeted recovery. No change made. Private offline-fsck-readonly.json1878B SHA256 `03190e6c7c7994f01a6897105ec83aa4fed68ebc1fbe56aa32674db053167f3a`; ioerr0xc remains0xc. Initial health command failures (trimmed BusyBox lacks dmesg; df-B unsupported) are recorded explicitly.

Recovered kernel snapshot uses only non-clearing klogctl10(size, max64MiB) and3(read-all) in a minimal static helper. Source hardware-211-20261010-kernel-read.c SHA256 `78f74c244897cf6009f63570e11fa0c6e91b3da6bcc14dc7b89917b1ff3add4b`; binary856288B SHA256 `3ebe9a3e308b2d05485035582c961eb272f826fa5d3c059080f82a85d13bdd2d`, independent R7 rebuild identical. Actual RAM kernel read0/no stderr. Standard df-P-k reports8265208KiB free; no clear/consume/loglevel action. Private kernel-reader-runtime.json230252B SHA256 `4bf012eaeab19822c9d3e1fdb008e00da848099db818b219989f91193213b02b`.

Before-image SMART-x-j0/healthpassed, CRC1003/media0 unchanged; query-associated ioerr0xc→0xf. Read-only `e2image -Q /dev/sda2 /root/recovery-private/root-before-repair.qcow2` then fails1 immediately: corrupt extent header while iterating inode259596. Resulting private RAM file is0B/mode0600 and is NOT a metadata backup. ioerr0xf stays0xf and no new storage kernel event. Capture offline-metadata-capture.json230869B SHA256 `bb678aab9e4aef0f03eaa93f9a83606f1e789fdc047669db381717be0b5287e7`. No valid metadata image, offhost image or undo file exists yet; corrections remain held. Evaluate a documented read-only native-metadata fallback rather than force or bypass this failure.

Optional affected inode-name/type diagnosis requires matching read-only debugfs. Strict full controller closure comparison refused before copying because generic loader/libc/blkid/uuid bytes differ; exact ext2fs/e2p/com_err match. Parent's independently verified two-file missing-only debugfs/libss bundle and actual RAM loader/binding/version validation are approved by R7; no existing PID1/SSH library may be overwritten. Actual inode stat/ncheck still pending; no-w/content dump permitted. Repair/normal return reboot/package installation/hardware acceptance remain NOT RUN.

## Valid scoped preservation and capped interactive repair readiness

The [matching upstream native-format dispatch](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/misc/e2image.c) calls the super/group descriptor, inode-table and bitmap writers without raw/Q inode-extent traversal. The [inode-table writer](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/lib/ext2fs/imager.c) reads group table blocks directly. Parent and R7 approved plain `e2image /dev/sda2 /root/recovery-private/root-before-repair.e2i` after guard/capacity/health gates. Actual capture0,986808320 nominal bytes,19720 allocated512B blocks, mode0600, SHA256 `b324a9c0ebefd45e2336b8a5bc946adc1a942ccd177eb940c709decee37c7a8d`. ioerr0xf→0xf/no new storage event. Private offline-native-metadata.json230817B SHA256 `40aa67834bf9dd7e92551a217e22778f2c77f031ec699aa0842ac4961c341698`.

Actual offhost gzip1727953B/mode0600, SHA256 `55ede2b7720d1e8a1e0d3e319857566824c24305d9106cef85ca54d554ec495e`; stream decompression986808320B exactly matches source SHA. File and private directory fsync verified. Receipt native-metadata-transfer.json576B SHA256 `3d5fa0ed7cf06a342e3f8c4326a7d384cc868d2268bc169c4a20a6801f5d2a8c`. Native format excludes directory/extent/EA/journal blocks and file data: this is scoped preservation, not a full system/data image.

Minimal debugfs/libss missing-only package bundle source/hash verified; actual existing RAM loader--list and LD_BIND_NOW=1 debugfs-V0/EXT2FS1.47.2, no existing runtime overwrite. Private debugfs-minimal-runtime.json1474B SHA256 `519686c329e98e72de9a438662827857740711756ee1fe4ac755eb141b09fdce`. Read-only stat/ncheck0 maps three corrupt extent regular0640 files259596/7/8 to journals; directory0700 inodes259599 and259602 are /root/.cache and /root/.config, with metadata blocks15503363/15503874. Full names/stats kept private: affected-inode-readonly.json4448B SHA256 `70964aa20cc7a214d9eb40207cb74384f444beee677c51599a2a41ab9a2c8164`.

BusyBox also lacks dd; first supplement attempt stopped before any raw artifact. A reviewed minimal static reader hardcodes O_RDONLY device8:2/63510503424B, permits only five selected blocks and emits exactly4096B per successful read. Source hardware-211-20261010-raw-block-read.c SHA256 `1a6e5a23215376f7daa60c090c5b6ea26709db8d73ec58df5f55aa337c83535c`; static binary821424B SHA256 `3256798046a2f81d1b06b3a1b5e93837c7da35a1bcc472dcc095c8330f6a1ea2`, independent R7 rebuild identical. Actual fresh guard0, helper/source hashes, five4096B mode0600 RAM and persisted offhost files all match SHA256 `ad7facb2586fc6e966c004d7d1d16b024f5805ff7cb47c7a85dabd8b48892ca7` (all-zero blocks). Receipt raw-metadata-supplement.json2141B SHA256 `859d8ac2a83a4f12dfc47cd9ece75e449ca59633483ce70ceef1106944713b0f`. Raw payloads are never printed/committed.

Post-image SMART0/healthpassed/CRC1003/media0, query-associated ioerr0xf→0x12 occurs before pure five-block capture. Capture finishes0x12, later kernel read0/no new ATA/SCSI/reset/I/O/UNC events and0x12 still unchanged. File chronology and actual pre/post health receipts establish the distinction. Private post-image-health.json267681B SHA256 `775c972f99530617214f1c91996aa95679ad609302c7b1a5ca061ba70926b80e`; post-supplement-health.json230138B SHA256 `7fde03b91343c41f11c88b1cbc892b24f74cb5fd07a3198d863713336b33a803`.

Exact RAM repair wrapper SHA256 `61e40215461de05f5a4c3db4d20e54b122551d3af53fe1e136b5abd15fe89422`: new absent/non-symlink `/root/recovery-private/root-repair.undo`, RAM device46, parent0700/uid0, umask077. Actual1MiB write/file-sync/remove test passes/mode0600; RLIMIT_FSIZE soft/hard1073741824B (1GiB). Previous4GiB RAM test also passed, then controller capacity changed and cap narrowed to1GiB. RAM8254240KiB free; controller2707152896B actual free, enough for capped undo plus512MiB margin. Actual preflight0/empty and finalguard0; no undo file or corrective write yet. Private undo-preflight-1g.json692B SHA256 `18059dfd050ab8bfe15fd1f8f3e243beeb0197bd31cabf5f35d941fbd654cefe`.

Exact proposed interactive command: `LC_ALL=C e2fsck -f -E fixes_only,nodiscard -z /root/recovery-private/root-repair.undo /dev/sda2` under the measured1GiB hard/soft cap. Controller fsck-session.py SHA256 `0186e5abf5862bb6f3d7689d8aa2c6fc11d40412a3e39a0aa358588fd03df752` keeps full transcript private0600/fsync and emits prompt notifications only, forwards one explicit y/n or stop; never all/default answers. Parent conditional correction release covers classified journal extent/accounting and cache/config salvage/checksum prompts only. Newly exposed inode/data/deletion or health/undo failure holds that prompt for root/R7 classification. Fresh complete offline audit and actual applicable R7 verdict precede execution; no-y/no-D, no generic optimization. Actual correction/clean validation/offhost undo/normal return/install/hardware acceptance remain pending.


## Active corrective session and single-byte answer correction

Previous coherent checkpoint local/verified remote `74a0db35428e380ab0323ee503aa41c6d89110ee`. R7 formal correction APPROVE and manager conditional release became executable after fresh complete offline audit:262 processes/93FDs/nsfs0/races0/failures0/finalguard0; private pre-correction-offline-audit.json24506B SHA256 `cbfebefb4c3c1f6dd6f5a0974ae81a84c8f391bc64390fd40012109d06fecd20`. Interactive capped fsck started, controller session66248, persistent RAMPTY71683 remains intact. Full transcript private0600/fsync; actual first three journal invalid extent questions answered y.

Actual driver defect discovered while still in known scope: previous driver0186e5ab wrote y plus newline. Matching upstream [ask_yn](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/e2fsck/util.c) disables ICANON and reads one byte; the newline accepted the following default. Consequent journal accounting and known cache/config missing dot reconstruction received implicit yes. All actual affected operations remain classified by parent/R7, but these were not all separate explicit answers. The previous claim of one prompt per answer is corrected. Hold at known directory259602 missing dot Fix; no unknown prompt accepted. Preserve live fsck/undo rather than restart.

Future driver now sends exactly answer.encode(), SHA256 `dc3219514cc7be54a07d83120b704811dfe3d8c6a0566a06c158ac545ab939d7`. Focused controller-only fsck-single-byte.py SHA256 `e6b6d00e27f6c86ba53679c33b0f708c4d02fa133458738dc0afce6584de34e0` identifies unique exact owned driver/cwd/executable/uid/transcript FD and exact SSH child/PPID/executable/starttime/uid; proves its writable FIFO equals SSH stdin by device/inode and flags. Opens only that pipe WRONLY|NONBLOCK|CLOEXEC, rechecks identities/fstat/current child input, writes exactly one selected y/n byte and records a private fsynced ledger. It closes only its own opened FD. Actual probe writes zero bytes and passes; private repair-single-byte-answers.jsonl722B/0600 SHA256 `162de6b45b9c177fa254baa32e189e86668836e7ea51d85fab314a6aeaca8e4d`. Python AST passes for both sources. Focused independent applicability requested before the first helper answer. No target process restart/signals/descriptor closure or unknown repair.

Current active worker, not awaiting-resume: existing fsck held at classified prompt while this coherent source correction is published. Exact next command after focused applicable review: `python3 docs/status/tasks/hardware-211-20261010-fsck-single-byte.py y`, then inspect the new private prompt before another single byte. Parent/R7 will not write concurrently. Continue known individual repairs; new inode/data/deletion/health/undo issue holds for classification. Normal return reboot, product installation and hardware acceptance remain NOT RUN.


## New directory259603 prompt held; narrow raw preservation prepared

Single-byte controller helper applicability approved by R7 on published87cc97e7. Actual independent y bytes repaired known config259602 self/parent entries; next NEW directory259603 block0 offset0 salvage question is held. Read-only debugfs stat/ncheck0 gives directory0700/root,4096B/links2/data block15503875; no recoverable pathname from corrupt parents. Private new-inode-259603-readonly.json1120B/0600 SHA256 `4a2409d4de11c01a88109c9526b9c9802444dfcc433823656d9499e7def6fd04`. This new directory was not included in prior salvage approval.

Parent releases only read-only preservation of actual new metadata block15503875 while existing fsck remains paused; expected device BUSY from this own writer is not a new offline-positive claim. Rawreader allowlist extended ONLY15503875, source SHA256 `c5c0649f6150b869042e8890977bc84f5d2e8f18f736451cfe833cd0e5355216`; static binary821424B SHA256 `050316c810573137bd76d313415d9e5877d5dd92f841cdbcbbc09d671905d4e5`, static-Werror compile and six nonallowlisted refusal tests2/empty pass. Existing five-block reader history preserved in previous commits. R7 focused applicability precedes v2RAM stage/read/private durable4096B preservation and actual health comparison. No new salvage answer, fsck restart or other disk writer authorized. Verify own paused fsck identity/deviceFD/current prompt, no other new writer. Continue only after actual captured block/type evidence and specific salvage classification.


## Actual new directory metadata preservation

Raw v2 checkpoint local/verified remote `8d09a644c62def73d6d6f4fb89d7295b31a72db6`; R7 independently rebuilds identical static binary and approves capture only. Existing paused fsckPID13880 has exact fixes_only,nodiscard/-z command and sole observed root block-device writable FD3/access2; current259603 salvage prompt unchanged. Initial holder/health capture exits1 because a guessed SCSI counter path is absent, retained privately; holder output itself identifies only own fsck. Narrow actual /sys/block/sda/device/ioerr_cnt retry0 gives0x12, no target changes. Private new-directory-259603-capture-preflight.json230372B SHA256 `f4443c20522535b33466e8c7efd7fdbe64c1b92f74b12e2ea7c0f1c8c86a5692`; health-path-retry.json380B SHA256 `180863d8aa73a91bde3c56cb3403b200b652f02b04d0409679d2388f954460d0`.

Actual source/binary hash-pinned RAM-v2 read creates new private4096B metadata block15503875, then durable controller0600 file/fsync/directory-sync; target/source/offhost SHA256 all `ad7facb2586fc6e966c004d7d1d16b024f5805ff7cb47c7a85dabd8b48892ca7`, all zero. Private new-directory-259603-supplement.json230597B SHA256 `e8ab1ad9b9ad517105e9a6671f8712b32968eddf147199b1fbafc547373eab83`. Read exits0, no stderr; kernel snapshot exactly equals before, counter0x12 remains0x12. Original pathname remains unknown; inode259603 is root-owned0700 directory/links2, no valid entries survive in its only data block. Prior native image preserves inode metadata; new supplemental block closes this directory-block gap. This is scoped evidence, not full user-data recovery.

Await specific parent/R7 salvage/dot reconstruction classification for259603; no new answer sent. Controller fresh actual rootfree2244972544B still exceeds capped1GiB undo+512MiB reserve. Keep live repair driver capture-only and persistent RAMPTY alive. Exact next command only after concrete verdict: reviewed single-byte helper y for current259603 salvage, then independently inspect next prompt. Unknown objects and destructive clears remain held; normal return/install/hardware tests pending.


## Classified new directory259603 repair and next user-cache prompt

Previous actual preservation checkpoint local/verified remote81d23a87f709f1f46f8df9a0d8bb571600d57c53. Parent/R7 individually approve259603 all-zero directory salvage/self-dot and temporary missing-dotdot placeholder, with mandatory actual parent/lost+found connection verification in pass3; original pathname remains unknown. Matching pass2.c creates root-inode placeholder before pass3 determines parent, not an invented original path. Explicit single-byte answers recorded, no queued newlines. Next NEW directory259594 salvage prompt held.

Readonly stat/ncheck0 for259594 identifies UID/GID1000 directory0700/4096B/links2, only block15505492, actual private home-user .cache path. Metadata receipt new-inode-259594-readonly.json862B SHA256 `7e4dde9cf610e5991da264a3cc9ab4e6ecbb307972d979d5fb58013ec968b96d`. Parent conditionally releases only exact extra block preservation under same paused-fsck identity/prompt and health gates; specific cache salvage awaits actual all-zero block and R7 verdict. Rawreader v3 adds ONLY15505492 to previous allowlist, source SHA256 `b76411e2c3279fb3b7dc0fdfdc4f64c510491fe585e7a0ad00c0eb3d07facda9`, static821424B SHA256 `d8d8aa8316e7fff7bc568d44979e70e66f12742d1a826c6111d6f109fad1c80e`; static-Werror and six outside-allowlist refusal tests2/empty PASS. No new target-stage/read/repair for259594 yet; publish source and obtain focused reviewer applicability before copying v3 to a new RAM path. Current fsck/undo and persistentPTY stay running; no normal return/install/hardware tests.


## Corrective repair complete; clean offline validation and durable undo

Previous published/readback sourcecheckpoint203ea2a84bf410117ef5a2b354388c0be98d4dd0. R7 independently approved actual259594 all-zero cache block preservation: new-directory-259594-supplement.json460615B SHA256 `a281c91e2aeda18797a5b218f117fae791004520a426e1266f0cf13f5bec0d2d`, source/offhost4096B/0600/ad7facb2, sole paused fsck13880/deviceFD3, counter18/kernel unchanged. Specific salvage/dot and actual pass3 parent repair executed with one-byte answers. Parent links validated:259594→31,259599/259602→152;259603 original path remains unknown and is preserved in lost+found.

Unknown zero-length inode259595 Clear declined explicitly n. Actual readonly type regular0644 UID1000/size0/blocks0; receipt813B SHA256 `c06cbc16a58fe547a55814894570d590feccd79d646d3fd9c3589b714456193f`; parent/R7 preserving Connect/count approval executed. Next259600 Clear also declined n: readonly current and native original inode stat byte-identical, regular0644/root/size0/blocks0; all three queries0, receipt1496B SHA256 `de64762841163fa25daed98938aaa99a3068234d53e6eff22e9f92c75aa1b6c3`. Reviewed regular preserving class applied, Connect/count executed. No unknown inode deleted. Known root/parent/directory reference counts and exact pass5 reconstructed block bitmap/free counts fixed after explicit parent/R7 source-backed metadata approval, nodiscard active.

Corrective fsck completes exit1 (filesystem modified), all classified questions resolved. Fresh complete offline audit260processes/93FDs/nsfs0/races0/failures0/finalguard0: post-repair-offline-audit.json24401B SHA256 `2b3aa09b0ce690601581be10aff8cd66c001972e92e0d614c6c6e647bcaa4b0c`. Complete `e2fsck -f -n /dev/sda2` now exits0/all five passes; optional259597/259816 optimizations declined, not acceptance failures. Kernel snapshots before/after and mid-repair exactly equal; counter0x12→0x12. Private post-repair-clean-health.json461056B SHA256 `bb09796a8d02333d5f1f3f9859e6a3f38186bf4c93dc977cfc064ec94dc16906`.

Actual undo749568B/0600/RAMdev46 SHA256 `92df3461b98f30de5f52d308f99e152bba598476cda02d10d055726c2a92b477`, transferred read-only and durably fsynced offhost0600, identical full hash. Full transcript3270B/0600/fsync SHA256 `790f88f2c37f4dad36612ca8912da4ef45a006c92b1b0e4dc2c0f99fac415799`; answer ledger20099B SHA256 `716dc358b4f824379290ae5c3d2e2a2ba507fe67564418e334f9a9d252cd70c9`. Actual37 answered prompts:35yes/2no. Initial5 old-driver y+newline inputs caused5 implicit known-scope defaultyes; corrected helper then sends27 exact bytes (25y/2n), no newline. History explicitly retained. Final preservation receipt1052B SHA256 `7f15a9f0fe29b1735b6d0d4253fe20382d263fe47cb191d23349242fbe071c99`; controller actualfree1926938624B, actual undo below1MiB rather than cap1GiB.

Worker remains active for released readonly integrity: ordinary root mount ro,noload and separateEFIro if needed, selected boot/kernel/initrd/GRUB/fstab/netplan/SSH and original auth metadata/hash comparison, actual original boot path; ordinary unmount then refreshed audit/guard0. Never execute old-root programs or modify EFI variables/config. Return hardware reboot remains held for actual independent integrity verdict; planned single-force normal reboot later, never doubleforce. Product installation and hardware acceptance remain NOT RUN. Publish this clean milestone immediately before integrity mounts.
