# Hardware .37 root-filesystem recovery investigation

Date: 2026-10-10. Owner `/root/host_37`, branch `codex/hardware-37-20261010`, worktree `/root/ngfw-wt/hardware-37-20261010`. User now explicitly requests disk/root-filesystem repair; the original installation/acceptance objective remains. Manager coordinates one host at a time and independently reviewed transition/repair steps. Current state: reviewed RAM rescue staging/authenticated tests PASS; corruption remains unresolved and no transition/repair has occurred. Earlier read-only diagnosis and candidate checkpoints are retained below with actual later evidence.

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

## Actual RAM stage and authenticated tests

The manager/R7-reviewed executed source was SHA256 `b9da683baf03bcbe0ece64a4e1822d578d8af6f097f79bf2dd7a69425b3417c8`, published/read back at5b9a8e6523e9d072e25a65147a32f9004e55ad42. Target --stage exited1 after starting the rescue unit because Type=simple returned before the chroot/sshd exec completed, and the immediate RAM-root assertion raced that startup. This failure is preserved rather than relabeled exit0. RAM tmpfs/binaries/private auth/units/listener had been created; original22 stayed reachable and nextroot remained absent.

Fresh read-only inspection showed the same MainPID3669 subsequently had root/cwd/executable on RAM device0:51. The owned credential-free stage-validate.py continuation (SHA256 `82e3822e304d3486ec3a6a949da059892565b2c7df0848cedcc35c3269e9b6f8`) then completed exit0: bounded same-PID readiness, exact published unit/config bytes, all matching systemd/executor/shutdown/SSH/e2-tools/Python/shell loader closures, parser/no stderr, SurviveFinalKillSignal/IgnoreOnIsolate recognized yes, no drop-ins/rootdir/WorkingDirectory/mount dependencies, same PID1 mount namespace, and no oldroot maps or file/directory FDs. Approved guard again returned mounted BUSY exit3. Current complete IPv4/IPv6 routes/rules and every original address configuration compare equal to the protected original snapshots; address lifetime counters alone were excluded and that exclusion is recorded explicitly.

The approved original source --test then exited0. Root public-key authenticated SSH with actual PTY passed before and after stopping/restarting only ngfw-rescue.service. Four live cgroup processes in each held connection passed RAM root/cwd/executable/maps/file-directory FD and global mount-namespace proof. Exact owned MainPID3669 stopped; new MainPID3866 started and authenticated successfully. Original22 continued listening, complete before/after network state was identical, nextroot absent. Tests ended their held sessions cleanly; the owned rescue service remains active for review, with SSH22 and management2222 available. No userspace transition, root repair, reboot or networkd configuration/reload has occurred. Filesystem corruption is still unresolved; NGFW installation/acceptance remains NOT RUN.

| Protected actual receipt | Bytes | SHA256 |
|---|---:|---|
| ram-stage-result-20261010.stderr (original failed startup assertion) | 90 | 37d7fdb1b4b2e4962f8775af85f993b24f96096e50341d685f7505f078066063 |
| ram-stage-validation-20261010.json | 54959 | 17c370c94118a2635554ca80151a47fde40654a783b7015b6581955b4745c0d3 |
| ram-functional-tests-20261010.json | 3241 | aec1a0f1d6fe8f6bd924a96f57cfa133e119e4a7b17d891ec0c49dbac625ae59 |
| ram-cgroup-before-restart.json | 876 | e67b666d125d5b9124743046e7b24efb27e1169029e4ca71e49558a619347b98 |
| ram-cgroup-after-restart.json | 876 | 5ec89c24624d9b5934a7ee81e62486ec2e391b6f6c3da0d83047da8f989b2822 |
| readonly-smart-trust-20261010.json | 768 | f32720101586c165b76ab58eacbfdddca51240cee9e716ffefd7a4949e269aee |

All receipt files0600 in private0700 host37 directory; no authentication contents were printed/committed. Cached Ubuntu InRelease verified with installed Ubuntu archive keyring via gpgv exit0; signed uncompressed Packages SHA/size match. Trusted matching smartmontools7.5-2 amd64 expected664482 bytes/SHA256 ab211f171a9595f6b9686caaec44ca07a623c4605923caae756e2446e055e0ff is established; RAM-only download/read-only SMART query remains next.

Future fresh staging source now includes the same bounded readiness and stable MainPID gate to avoid the startup race. Its new source SHA256 is `aafb62c9ff15a9812e45d364e0c91c4e7072345993d5e45a6ab53539a8d910c2`; no runtime artifact/unit/network behavior changed. Do not rerun --stage on the existing RAM root; it intentionally refuses overwrite. Executed source and actual follow-up evidence remain distinct above.

Exact next actions: publish/read back this receipt; finish trusted RAM-only SMART health/integrity/capacity checks; obtain independent final staged-tree, kernel-network retention, bounded metadata/undo, namespace/block-guard and return procedure review. Only manager's later explicit instruction may create nextroot/transition or repair an actually unmounted root. Maintain a held authenticated RAM PTY across any reviewed transition and reauthenticate after the transferred-run helper completes.

## RAM health diagnostic and held management session

Manager-authorized trusted SMART diagnostic completed without installation: gpgv verified cached Ubuntu InRelease, authenticated signed Packages SHA/size previously matched; `apt-get download smartmontools=7.5-2` ran with RAM cwd/cache/log overrides and empty pkgcache/srcpkgcache, no update/install. Downloaded664482-byte archive actual SHA256 matched ab211f171a9595f6b9686caaec44ca07a623c4605923caae756e2446e055e0ff. `dpkg-deb --extract` ran only into owned RAM; selected smartctl plus seven dependencies are RAM-copied, all seven dependency installed-package MD5 checks pass, existing RAM libraries were checked byte-identical before reuse. Actual `chroot /run/ngfwrescue /usr/sbin/smartctl -x /dev/sda` exit0; no self-test, control change or SMART enable command ran. Private receipt ram-smart-diagnostic-20261010.json16921 bytes/SHA256728665ba546f6d244cbc5def704b2c0fa38120c1ef04e3e419ddab82cb306560; stderr0/controller exit0, file0600. Disk identifying/serial output stays private.

Actual selected health results: aggregate SMART self-assessment PASSED; Percentage Used Endurance Indicator126, Wear_Leveling_Count normalized001/raw2648, Reallocated_Sector_Ct2 and Runtime_Bad_Block2, power-on26762 hours. Program/erase/uncorrectable/interface CRC counts0; ATA error log reports no errors. These are observed device counters, **not a declaration of healthy storage**; aggregate PASS does not erase the wear indications or filesystem corruption. Parent/R7 have been informed before any corrective write.

Fresh authenticated RAM SSH PTY remains deliberately held for any later reviewed transition: controller exec session30565, RAM shell PID3940, rescue MainPID3866, cgroup /system.slice/ngfw-rescue.service. Actual four live authenticated SSH descendants again pass RAM root/cwd/executable/maps/fds and shared PID1 mount namespace. Private held-session receipt ram-held-pty-session-20261010.json1215 bytes/SHA256b77e86555d4d6179ac7809eb2b8028e9a9e440daae7fdd012dbd86260c0fe33d,0600. This observed held session is live at capture; revalidate it rather than assuming liveness from a task label. Exact next session check: read-only global `/proc/3940` and service cgroup reference audit, or send an innocuous command through controller session30565. Do not exit/stop the owned rescue while the manager reviews transition.

Manager will handle .211 first after independent review. .37 remains RAM-ready and held, with nextroot absent; no ordinary reboot, soft-reboot, pivot or root filesystem check/repair is permitted until subsequent explicit manager instruction. No mounted `e2fsck -n` result is used as acceptance.

## Owner wear acceptance and readonly preservation preparation

Owner explicitly replied via manager: «تعمیر کن فرسودگی مهم نیست. این مشکل روی کدام ماشین است؟» Manager answered wear is .37 and filesystem corruption affects both hosts. This authorizes logical filesystem repair despite the reported wear; do not ask again about replacement or park solely for endurance126. Offline/unmounted, namespace/reference/exclusive-device guards, private metadata/undo/evidence preservation, actual media-read errors and independently reviewed transition/return remain required. No corrective write or transition has been released for .37 yet; manager handles .211 first. New uncorrectable/read/reset failure is an actual failure to assess, separate from accepted wear.

Readonly recovery sizing used `du -x -B1 --apparent-size --max-depth=1 /root /home /var/lib /etc /boot`; actual du exit1 and three corruption diagnostics, so totals are **lower bounds, not complete recoverable-data inventory**. Apparent /root23739066949 bytes,/home5308,/var/lib145660748,/etc1028290,/boot113496533. Seven root regular files total23739061129 apparent bytes and14552125440 allocated bytes, dominated by one file23739026220 apparent/14552076288 allocated bytes. Filename/content remains private and no user-data contents were read by sizing. Full recoverable user-data backup cannot fit current controller free disk/RAM or6GiB rescue capacity; no full backup or verified compressibility is claimed.

Private readonly-preservation-sizing-20261010.json4458 bytes/SHA2560c56f7e4ebccb1a665af3405d683e5c84c181a5464bcd0b466eb373d1d9e2213, SSH0/stderr0. Private readonly-userfiles-inode-20261010.json1591 bytes/SHA256df6cd46d856979b87b9f1c6bb7a8de8dfe896fab0a34bb5e44d46a3b7cbe7333, SSH0/stderr0. Readonly debugfs stat/ncheck exits0 show inode259596 regular8MiB/group999 beneath /var/log/journal; machine-specific journal pathname is private. Earlier inference that this inode might be root/authentication data is not supported by actual ncheck.

Actual selective staged integrity and capacity: ram-selected-integrity-capacity-20261010.json13094 bytes/SHA256ef0459d164ebd1d83a91787d85617767ef50688fd05f725afcf375bea58c0bc5, SSH0/stderr0. All47 selected systemd/executor/shutdown/SSH/session/auth/e2-tools/shell/Python/helper binaries and executable-closure/NSS libraries match installed package MD5 and exact staged RAM bytes; static guard SHA matches approved artifact. Rescue filesystem cap6442450944 bytes, available6247342080 bytes at capture. The receipt contains the complete virtual mount tree for reviewer inspection. These selected checks do not turn missing package documentation or the full root filesystem into an integrity PASS.

Preservation sequence proposal, not execution: after later reviewed transition and all old-root/device-holder guards pass, take bounded readonly media observations and a private metadata image only while root is actually unmounted; measure allocated/compressed bytes and transfer it off-host with verified hash/readability before corrective writes. Preserve existing private boot/auth/network baselines and subsequent fsck logs/undo off-host. Inode-table bytes alone are3845088*256=984342528, with bitmaps/group descriptors/journal/additional metadata still required; this is a geometry estimate, not measured image size. Raw e2image may have filesystem-sized sparse apparent length, so actual allocated bytes/quota and streaming/compressed representation must be measured. Do not use -f on mounted root or infer full-data rollback from metadata/undo. Full user-file recovery requires additional capacity if complete backup is required; unreadable paths must be recorded rather than silently omitted. No image or undo has been created yet.

## Bounded readonly direct-media sample

Manager requested the same bounded read preparation as .211. Operator supplied credential-free sampler source SHA2565784de633c8d0e797728c61da2df437ea74be6f9bb783ebf47f6c91d0d92c6d4, copied exactly to owned hardware-37-20261010-bounded-direct-read.py and protected controller source. Additional actual remote preflight verified current root st_dev8:2 and /dev/sda2 is a block device st_rdev8:2. Strict-known-host BatchMode SSH ran python3 -B with source on stdin. Device was opened O_RDONLY|O_DIRECT|O_CLOEXEC; anonymous mmap provides page-aligned1MiB buffers to os.preadv.256 reads at offsets0..267386880 completed exactly268435456 bytes, exit0. No raw device bytes were printed or stored.

Actual sysfs ioerr before0x9 and after0x9; no new matching physical storage-error/reset/uncorrectable/timeout kernel messages during the sample. Earlier boundary receipt had0x6, so an earlier increase preceded this sample and no cause is inferred. The sample reads only the first256MiB and **does not establish whole-disk health or filesystem consistency**. Root is still mounted and no mounted filesystem check is claimed. Private bounded-direct-read-20261010.json513 bytes/SHA25686057e4f6d3572ef6b7565a30b8e5c9fcae888b42b8c98c8d9d10ee68b2d0cbe,0600, SSH0/stderr0. Duration is the sampler's single monotonic-clock observation only; no cross-clock elapsed duration is inferred.

Original22 and rescue management2222 remain available, held shell3940/session30565 was revalidated by an innocuous terminal command, route to controller remains enp12s0/source172.30.126.37. .37 remains waiting for manager's phase release after .211; nextroot absent, no additional service restart, transition, deletion or corrective write was performed.

## Narrow readonly offline-audit preparation while .211 recovers

Manager reports .211 actual RAM soft transition succeeded and sequences its return before releasing .37. .37 has not transitioned: fresh original22 readonly identity checks show mountedroot8:2, disk8:0, RAM0:51, rescue active, nextroot absent; held authenticatedPTY30565/shell3940 responds. All original management/network/services remain unchanged.

Own copied C hardware-37-20261010-nsfs-check.c is exact reviewed .211 source SHA25636e12485aa4e1ca3a85e0a1760fff89604c0d4b3f9b5b190aeee873bc45a66ee. Own Bash hardware-37-20261010-offline-audit.sh is exact final reviewed .211 source SHA256e5608afb712bd86251395800a4e27d9d7f1d8df75d75d0f2c41d827ada89affc; both hosts have same verified root8:2/disk8:0, so no identity substitution is needed. Independently rebuilt static helper SHA2562907bb0345fb9dad199e48b2539ce9038b24fbafa00fa9672b4074239a6ac681 (1014992bytes) was copied/read-only verified into own private controller host37/nsfs-check-static (0600), not another target. Only that owned private file is consumed by the final wrapper. Exact approved guard SHA25602a2270ad8e0f480efc7464b26d2216fec8a915aa12755da6339015abed0f41c is already inside own RAM tree.

Own narrow staging candidate hardware-37-20261010-audit-stage.py SHA2569b9ba20bf71eefd39d1cc5c05647e13f4e8ef695d9745b3aefa03bef37651ebf has controller/embedded Python and Bash syntax PASS. It verifies existing host/block/RAM/held-session identity and exact source/binary digests; tests RAM runtime command resolution; copies only RAM usr/bin nsfs-check/offline-audit.sh/block-check (existing usr/local/sbin guard alias); checks Bash syntax, non-nsfs refusal, same-mountnamespace selector and mounted-exclusive negative. It never mounts, transitions, checks or repairs ext4, changes networking, restarts services, or performs full process audit on the running original OS. Receipts stay controller0600. Execution awaits exact candidate independent applicability review; no actual staging/test PASS claimed yet.

Important preserved limitation: nsfs helper enters only a short-lived child; its /proc/self/mountinfo may be filtered by the RAM chroot's root. A selector exit0 alone does not prove oldroot unmounted. Later full offline audit must inspect every live userspace root/cwd/exe/maps/fds and namespace references, reject unreadable/racing userspace, reject oldroot and unresolved nsfs mounts/block holders, and finish with exact block-check O_RDONLY|O_EXCL exit0 after helper children exit. Mounted negative BUSY3 is expected now, never acceptance for repair. A reviewed .211 artifact is not itself .37 transition approval.

Last published/read-back checkpoint before this audit preparation:55e62fd31d168e0dfb8f7d4eadd188f956033512 (installation dependency/suppression/firstboot plan; pre/post-commit check PASS). Exact next command after R7 candidate approval/publication: python3 docs/status/tasks/hardware-37-20261010-audit-stage.py. .37 transition remains held until manager releases a reviewed phase after .211 returns.

Initial audit-source checkpoint0886fee64f7e66f7b755e7e8103e945fb8712859 published/read back successfully with pre/post-commit lightweight check PASS. Final follow-up changes only the controller static-artifact path to its own verified private copy; target behavior unchanged. Candidate remains unexecuted pending exact final applicability review.

### Actual mounted audit-helper staging and negative validation

Final own-path source9b9ba20bf71eefd39d1cc5c05647e13f4e8ef695d9745b3aefa03bef37651ebf independently approved/published/read back at a32e51885df0beaa16c211a78de32be6fc711e41, pre/post-commit check PASS, then executed. Actual SSH exit1 is preserved: all three RAM helpers were copied/hash verified and runtime/Bash syntax/non-nsfs refusal checks passed, then the wrapper incorrectly expected the mounted namespace selector to return0. Actual correct selector result is3 because originalroot remains mounted. This is an overly strict test expectation, not an ext4 check or offline PASS. No target service/network/mount/transition change occurred.

Private initial ram-audit-stage-20261010.json is0bytes/SHAe3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855; ram-audit-stage-20261010.stderr is160bytes/SHA5fcc7875b4c82a5e27bb1f53c7afd24788fba16b7bf305ab3fed9ee7e72638ef, actual selector assertion traceback. Both0600. Readonly continuation ram-audit-selector-continuation-20261010.json796bytes/SHAefa2601c92ee055eee63d50ac88f6d7fa2b51579a42e75fdbc4ef55b7c4f604a, SSH0/stderr0, independently records literal:

```text
namespace inode=4026531832 type=0x20000
mount_entries=47 oldroot_mounts=1 unresolved_nsfs_mounts=0 malformed=0
selector exit=3
BUSY: block device still has an exclusive holder; no repair permitted
block-check exit=3
ngfw-rescue.service MainPID=3866
```

New exact wrapper source546f1efb0625094ea0ef24adf9709ef467ded8968a3b61b4d1242151bbe299af changes only mounted-selector expectation to3 plus explicit oldroot1/malformed0 and separate validated receipt filenames. Original failed receipt retained. All helper source/static/guard hashes and target scope unchanged. Candidate awaiting independent exact correction review before rerun; no success is assigned to failed invocation. No full process audit/transition/metadata/undo/repair/install has run; original22/held3940 stay live, nextroot absent.
