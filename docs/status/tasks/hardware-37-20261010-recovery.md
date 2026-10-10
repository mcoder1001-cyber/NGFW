# Hardware .37 root-filesystem recovery investigation

Date: 2026-10-10. Owner `/root/host_37`, branch `codex/hardware-37-20261010`, worktree `/root/ngfw-wt/hardware-37-20261010`. User now explicitly requests disk/root-filesystem repair; the original installation/acceptance objective remains. Manager coordinates one host at a time and independently reviewed transition/repair steps. This checkpoint contains **read-only diagnostics and a candidate design**, not a staged or executed repair.

## Actual fresh evidence

All target commands used strict-known-host BatchMode SSH to `root@172.30.126.37`. Commands ran through remote Python subprocesses; output and diagnostics were streamed to protected controller files. No target file, mount, network, service, bootloader, driver or package changed. No credential content or full environment was read.

| Observation | Actual result |
|---|---|
| Root | `/dev/sda2`, ext4, `rw,relatime`, major:minor `8:2` |
| Filesystem | `clean with errors`, errors behavior Continue, error count1319; last error `ext4_find_extent`, inode259596, EFSCORRUPTED |
| Root fsck service | Exit3 status query; failed automatic fsck, invalid extent node at block15505493, manual fsck required |
| Disk | Samsung SSD 860 PRO 256GB, SATA, disk256060514304bytes; root63510503424bytes and EFI511705088bytes only |
| Disk health | No existing smartctl/nvme tool; sysfs state running, I/O-error count `0x6`; physical health is **not established** |
| Hardware/BMC | Physical `systemd-detect-virt=none` (exit1), INTEL BOTU-C612-8L boardV1.5; no `/dev/ipmi*`, IPMI class path or existing ipmitool |
| Memory | Total16645222400bytes, available15971741696bytes, no swap |
| PID1 | `/usr/lib/systemd/systemd`, version259.5-0ubuntu3, root/cwd `/`; systemctl help includes soft-reboot |
| RAM mounts | `/run` tmpfs is **noexec**, size1625512k; `/tmp` executable tmpfs size8127552k but tmp.mount conflicts with umount.target; `/dev/shm` tmpfs |
| Existing rescue | `/run/nextroot` absent; `/run/initramfs` and `/run/systemd/shutdown` empty; no separate rescue disk or loop device |
| Tools | e2fsck1.47.2 and OpenSSH10.2p1 present; target busybox/dropbear/kexec/pivot_root/switch_root binaries absent |
| Current boot artifacts | Kernel7.0.0-22 and initrd76384759bytes; initrd inventory has busybox/ip/ipconfig/mount/switch_root and igc module, no SSH/e2fsck in selected paths |
| Dynamic runtime | ldd resolves e2fsck, sshd, sshd-session/auth, systemd/systemd-shutdown, bash/python3 and ip dependencies; complete NSS/PAM/runtime behavior is still unverified |
| Root holders | Fresh executable-process scan excludes kernel threads:17 userspace processes all referencing old root through root/cwd/executable, across4 mount namespaces; no claim based on the broader444-entry scan |
| Management | enp12s0/igc routable/configured,172.30.126.37, gateway172.30.126.1; controller route stays on enp12s0 |
| Network stop boundary | networkd active but conflicts with shutdown.target and initrd-switch-root.target; generated management .network has no explicit KeepConfiguration directive |
| Mounted negative guard | `os.open('/dev/sda2', os.O_RDONLY|os.O_EXCL|os.O_CLOEXEC)` fails errno16 EBUSY; no data read or write performed |

Exact command selectors included `findmnt -J`, `lsblk -J -b`, `tune2fs -l /dev/sda2`, `systemctl status systemd-fsck-root.service --no-pager --full`, `dmesg --level err,warn`, `systemd-detect-virt`, `systemctl --version/--help`, `free -b`, `ip -j`, `losetup --list --json`, `lsinitramfs` current initrd, `ldd` named runtime binaries, `networkctl status enp12s0 --no-pager`, and `systemctl show` selected mount/network/SSH unit properties. `/proc` scans read comm/exe/root/cwd/mount-namespace metadata, not process command arguments or environment.

Protected directory `/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37` is0700; every diagnostics file is0600. Actual SSH collection exits0 and stderr0bytes:

| Private diagnostic file | Bytes | SHA256 |
|---|---:|---|
| readonly-recovery-diagnostics-20261010.json | 163077 | 9cf70a7449ab141f0e630a7d8b1c12b4dd4b8fec8a8fb7b86331b0b49049c6bd |
| readonly-recovery-capabilities-20261010.json | 15059 | 944e971d39489dee2851961d6003ffa8d92d2195c71936e0d5e60c2ba47ae8f2 |
| readonly-recovery-boundaries-20261010.json | 7172 | 102f2d97ac8386f196fd1169b2ba06cd86119daec8ee80f6cdc11a8fd4629bf9 |
| readonly-recovery-namespaces-20261010.json | 8938 | a53d249503f4b585436aa94daee3924081e3ba96cbf4f763c402ac55f21a933a |

Existing private network/configuration and compatibility-firewall snapshots were reverified by metadata/hash/size only. Both manifests remain unchanged and valid, directory0700/files0600. Native nft ruleset remains uncaptured because nft is unavailable; these backups are **not a full disk/data image**.

## Candidate RAM recovery path and required proof

Inference from observed version/capabilities: a dedicated executable tmpfs staged at `/run/ngfwrescue` may support a minimal userspace-only recovery root using systemd soft-reboot. This is **not certified safe or ready to execute**. The upstream [matching v259.5 soft-reboot documentation](https://github.com/systemd/systemd/blob/v259.5/man/systemd-soft-reboot.service.xml) describes replacing userspace while retaining the kernel and switching to a supplied new root; it also explains that surviving processes can keep the previous filesystem pinned. Direct chroot is insufficient to release PID1 and other old-root holders.

The [matching-family switch-root implementation](https://github.com/systemd/systemd/blob/v259.5/src/shared/switch-root.c) uses lazy detach in its root-switch paths. Therefore disappearance from one findmnt view alone cannot establish that the block device is safe to repair. The [Linux open manual](https://www.man7.org/linux/man-pages/man2/open.2.html) documents exclusive block-device opening and EBUSY for a device in use; the actual mounted negative probe above confirms that guard works on this host.

Required engineering work before transition, all subject to manager's reviewed staging procedure:

1. Compare root filesystem/sector geometry to partition boundaries; inventory existing e2image/undo capabilities and metadata-backup limits. Neither compressed metadata nor an undo log is a complete user-data backup.
2. Prepare a genuinely RAM-resident root with executable binaries, correct loaders/libs, systemd re-exec dependencies, e2fsck, SSH session/auth helpers, required NSS/runtime files, isolated temporary authentication and control scripts. No old-root bind mounts, inherited working directories, library symlinks or file descriptors may be used to keep it working. Existing `/run` noexec and `/tmp` shutdown behavior must not be ignored.
3. Validate a separately reachable key-only rescue SSH listener restricted to the management address/port2222, with preserved host identity and controller authentication, before ending the current service/session. Private authentication material must never enter receipts or bundle logs. The final design must specify survival/boot ordering, network retention and failure behavior; networkd shutdown cannot be assumed harmless.
4. Independently review the exact executable staged tree, transition, bounded recovery behavior and return to the original system. Confirm the RAM environment stays reachable if repair refuses an inode or return boot fails; never use an automatic blind hardware reboot as a fallback.
5. After the reviewed transition, require PID1 and all userspace roots/executables/maps to be independent of old root, audit every mount namespace and device holder, and successfully open the root block device exclusively read-only. If EBUSY or any old-root mount/reference remains, **do not fsck**. A readonly remount, chroot, failed unmount or hidden lazy mount does not meet this gate.
6. Only then perform the manager-approved offline repair with actual output/exit evidence and controlled handling of destructive inode/data decisions. No mounted repair and no blindly forced all-yes repair are authorized by this diagnostic phase. Validate a subsequent clean offline check before restoring original root/userspace and checking management reachability, routing and disk errors.

No target staging, SSH listener start, runtime unit/network configuration, mount change, pivot, soft-reboot, kexec, fsck repair or reboot has occurred. No existing verified RAM rescue or console path is claimed. User repair authorization is present; the current constraint is proving the technical path and preserving access.

## Reviewed-source RAM staging checkpoint

Manager now authorizes reversible RAM staging on this host, with exact source independently reviewed before execution. The owned credential-free `hardware-37-20261010-stage.py` source is the executable candidate. Its controller, embedded remote staging, cgroup audit and owned-stop/restart snippets each pass Python AST parsing. **Target staging and functional tests remain NOT RUN at this checkpoint.**

Additional actual readonly evidence:

| Private file | Bytes | SHA256 |
|---|---:|---|
| readonly-recovery-geometry-ssh-20261010.json | 12108 | 507a338072540a684c59f11a54daeeef318b2cb3113e16e0d25e4039d14a85e4 |
| readonly-recovery-integrity-apt-20261010.json | 23664 | 9f4faddd0ac832caf3ee6800409c7b4624e2415e667112a7c6414f95283954d6 |
| auth-return-private-20261010.tar | 655360 | 2a57558ae6a6c7c2694b2034249e301d209bb0b5b8bc69076e77710725cd45c9 |

Root partition is15505494 blocks of4096 bytes, exactly63510503424 bytes. Invalid extent block15505493 is the **last legal filesystem block**, not outside the partition. Existing e2image/e2undo tools are available. Effective original SSH22 uses PAM and permits root/password/public-key access; staging changes none of those original settings. Kernel command line has no systemd.unit/rd.unit/init/rdinit/mask overrides. `dpkg -V e2fsprogs systemd openssh-server libc6 libsystemd-shared` exit0 reports346 missing documentation/man/lintian paths and no other entries; it is not a blanket package-integrity PASS. Selected copied binary/library bytes will be checked against local installed package MD5 metadata when available, and actual RAM loader/version/function tests remain required.

The fresh minimal private return-authentication archive was manager-authorized, streamed directly off-host, exit0/stderr0,27 members fully readable. It includes original SSH configuration/host keys, root authorized-key directory, passwd/group metadata, required SSH/common PAM files and root-only shadow/gshadow records. Controller archive0600/directory0700; no contents, individual key fingerprints, hashes or password records are exposed in Git. It is a configuration/authentication backup only and no original root file has been restored. An identical protected copy will reside inside the actual RAM root. Original network/configuration snapshots remain protected and unchanged.

Candidate mechanics encoded in the source:

- Dedicated executable tmpfs `/run/ngfwrescue`, cap6GiB allocated on demand. Runtime mount unit has DefaultDependencies=no and no umount/shutdown conflict; `/run/nextroot` must remain absent throughout staging.
- Matching systemd259.5, systemd-executor, SSH/session/auth executables, flattened executable library closures, NSS, shell/core/FS tools and Python runtime are copied to RAM. Standalone dependency libraries are not re-ldd-scanned in a way that loses executable RUNPATH. No original-root bind is permitted; only virtual /dev (including devpts), /proc and /sys trees are recursively/private bound where appropriate.
- Bootstrap service execs chroot then RAM sshd, in PID1's mount namespace, with no RootDirectory, WorkingDirectory or explicit mount Requires/After. Service [Unit] SurviveFinalKillSignal=yes, IgnoreOnIsolate=yes, DefaultDependencies=no, After=basic.target; ordinary reboot/rescue conflicts follow upstream guidance. Stdout/stderr append to protected RAM files. The RAM-root same-name service uses direct `/usr/sbin/sshd` paths for later launch/restart; minimal default.target requires only this service plus a reviewed oneshot that recreates /run/sshd after runtime transfer, with no network daemon, udev, firewall, fstab or boot-disk services/generators.
- Rescue SSH listens only on management172.30.126.37:2222, root public-key authentication only, PAM/password/keyboard-interactive disabled. Original SSH22 is preserved. Copied authentication material stays private.
- Parser validation, unit properties/drop-ins, all loader checks, mounted-negative approved static block guard exit3, MainPID RAM root/cwd/executable/maps/file-and-directory FDs and unchanged complete IPv4/IPv6 addresses/routes/rules are required actual evidence before stage success.
- Separate `--test` performs authenticated PTY connections before/after stopping and restarting only ngfw-rescue.service. It proves all live authenticated SSH descendants remain in that service cgroup, share PID1 mount namespace, and have RAM root/cwd/executable/maps/fds; old22 must still listen and network state must stay identical. No transition or repair is part of either mode.

Approved manager/R7 static guard controller path is `recovery-private/shared/block-check`,860432 bytes,SHA256 `02a2270ad8e0f480efc7464b26d2216fec8a915aa12755da6339015abed0f41c`; it will be transferred only into RAM and independently hash-checked. Six guard refusal tests/rebuild were reviewed separately by R7; this worker does not claim to have run those tests.

Native nft inventory remains unavailable. Full disk-image capacity is unavailable on controller; metadata image/undo capacity and off-host preservation must be measured before any later corrective write. Neither a metadata image nor undo protects arbitrary user data or power/hardware crashes. Physical SMART health remains pending verified RAM-only download from signed cached official package metadata, without apt update/install.

## Next action

Publish this exact candidate source/receipt and obtain manager/R7 artifact review before execution. Proposed next command, **only after that review**, is `python3 docs/status/tasks/hardware-37-20261010-stage.py --stage`, followed by `--test` only if stage succeeds. No nextroot marker, userspace transition, offline repair, network/service changes outside the owned RAM rescue or ordinary reboot is authorized at this staging checkpoint. A healthy-storage result is still required before NGFW installation/acceptance resumes.

Exact candidate source SHA256 at this checkpoint: `b9da683baf03bcbe0ece64a4e1822d578d8af6f097f79bf2dd7a69425b3417c8`. The RAM preparation oneshot uses verified copied /usr/bin/mkdir, chmod and chown paths, runs before rescue SSH, and does not pull disk or network services. Matching systemd-shutdown and dependencies are included for a separately reviewed future normal reboot.

Before-repair boot baseline: protected readonly-boot-baseline-20261010.json,3120 bytes,SHA256 `c76678897876dcd36b5d5254a9af0d399b6c9efaf80451e1e5ebbb09656a141d`, SSH exit0/stderr0. All19 selected kernel/initrd/EFI/grub/fstab/boot-config files read/hash successfully. Readonly debugfs ncheck inode259596 exit0; path output remains private. The runtime-preparation oneshot now directs stdout/stderr into protected RAM files, avoiding default journal dependencies.

Both RAM rescue/runtime log files are explicitly precreated0600 within0700 var/log before starting any service.

Post-publication history scan detected a false positive on the public archive digest constant name, not credential bytes. Prior source checkpoint02329d51e is preserved at remote codex/archive-hardware37-stage-02329d51e; renamed RETURN_ARCHIVE_DIGEST without changing CI detector policy. This own checkpoint is replaced with expected-head lease only after an actual passing post-commit check.
