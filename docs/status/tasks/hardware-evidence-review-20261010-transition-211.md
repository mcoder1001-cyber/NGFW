# Independent .211 transition and read-only offline phase review

Verdict: **APPROVE transition to the prepared RAM root and read-only offline
diagnosis/preservation only**. This does not approve filesystem corrections,
normal reboot return, hardware acceptance or any .37 transition. The owner has
authorized logical repair on both hosts, including .37 despite disclosed wear;
later correction decisions require actual offline findings, not another wear
permission request or a fabricated universal full-image prerequisite.

Exact .211 source branch readback:
2ee35a45fa9ca67fb4dd556f4fc5a724d70a125d,
[reviewed staging source](https://github.com/mcoder1001-cyber/NGFW/blob/2ee35a45fa9ca67fb4dd556f4fc5a724d70a125d/docs/status/tasks/hardware-211-20261010-ram-stage.py),
SHA256e33b95eacf965361d2dc3ba05866e3ee80d9eb1cd6259679fa7d3582a34166d6.
This final source explicitly copies genuine helper executables instead of assuming
BusyBox chown; its chroot helper execution, daemon activation and parser checks
actually completed. Earlier failed attempts and withdrawn reviewer escaping error
remain documented; no failed attempt is relabeled PASS.

Reviewer independently parsed private actual receipts, selected safe command outputs
and metadata without printing keys/config contents. All receipts below are under
the controller's private host-211 directory0700, regular files0600:

| Receipt | Actual SHA256 |
| --- | --- |
| ram-stage-complete.json | 648753664f64800496978dff2b6b96886277d8e216d385e46ed37aca25a4edc6 |
| ram-runtime-audit.json | 184223796a22822088d8f0122c458f20d89e6c337dae5ac73e88b0ae7478e981 |
| smart-result.json | dbb9a1dec811c0b24d13056c55743d4a770da48f415671c6ea52a252f7ba07b9 |
| block-check-transfer.json | d9739d4ab82c60bac055c1ca8e388407b2357742a8ffdcc7fd0f58468cf51b97 |
| original-auth-ram-transfer.json | 803596e7d24a88107de56790fa694f0c416985bc83c581d79f80afdd52c683e3 |
| original-boot-config-baseline.json | 2e3af5ef6c75edbbadcbb4467e798da93adff516ca9a4e3257d3cddb70a38958 |
| ram-final-baseline.json | ab3293c9e2403b1b659bf06b632663e7f553e3852587d087316a83a76fbe8092 |

Actual reviewer commands: git rev-parse HEAD, git ls-remote origin the host branch,
sha256sum the source, Python json.loads/stat/hashlib of named ready receipts, and
field-by-field comparison of actual units against staged source. Selected results:

```text
remote source2ee35a45 readback matches local: True
stage command exits: 23 commands, all exit0
stage allocation116760576B; final allocation124469248B
network before/after hashes equal: True; nextroot absent: True
actual candidate SSH/preparation/bootstrap/mount source equality: all True
unit verifier: exit0, stdout0bytes, stderr0bytes
PID6421 sshd;6456/6458 sshd-session;6459 persistent bash:
 all root/cwd/exe RAM0:46; same PID1 mnt namespace and rescue cgroup
 all old-root maps0; all old-root file/directory fds0; unreadable fds0
daemon stdout/stderr: RAM regular log file; shell PTY: /dev/pts/0
SurviveFinalKillSignal/IgnoreOnIsolate=yes; DefaultDependencies=no
PrivateMounts/PrivateTmp/ProtectSystem=no; RootDirectory/DropInPaths empty
Transient=no; no Requires on the staging mount
static guard860432B SHA02a2270a...d0f41c, target mode0700
mounted-negative: exit3 BUSY; stderr0bytes
RAM8GiB: available8465465344B; MemAvailable31325548KiB
controller df-B1: shm available1977929728B; disk available249184256B
offhost original-auth35240B/mode0600; RAM copy hash equality: True
selected boot/config baseline9 files: all read; default-unit overrides0
```

Source/tree inspection confirms matching systemd, executor, shutdown, SSH helpers,
e2fsck/e2image/e2undo and helper binaries are present. Candidate default target
requires only the rescue/preparation services, with an inert basic target and no
network/disk generators. Actual preparation/SSH units match source and address the
transferred /run/sshd lifetime issue. Worker separately proved original22 and
fresh key-only2222 command authentication, plus a persistent authenticated PTY;
controller session71683/shell6459 must remain open across transition. These results
prove preparation, not PID1 boot or original-root release.

Signed provenance checks all exited0 for Ubuntu smartmontools7.5-2; exact package
SHA256ab211f171a9595f6b9686caaec44ca07a623c4605923caae756e2446e055e0ff.
Actual smartctl information query exit0/aggregate passed; media reallocation,
program/erase/runtime-bad/uncorrectable counts0 and ATA error log count0. Historical
CRC count1003 remains material. SCSI ioerr increased0x6 to0x9 across preparation;
cause is not proven, even if optional SMART command handling is a possible
explanation. The final selected recent storage-kernel-error list is empty.
Do not claim physically healthy or stable counters from these observations.

Before crossing, publish/read back the worker's actual coherent public receipt,
reconfirm the persistent RAM session and original .37 SSH access, and change only
the owned RAM nextroot linkage/transition. Use the supported matching userspace
soft reboot. Its built-in lazy detach means mount visibility is insufficient;
the exact source-based design and strong offline guard remain required.
[Matching soft reboot documentation](https://raw.githubusercontent.com/systemd/systemd/v259.5/man/systemd-soft-reboot.service.xml),
[Matching root switch implementation](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/shared/switch-root.c).

After transition, first verify PID1/executor runtime belongs to RAM, actual helper
completion and /run/sshd mode/owner, persistent PTY and fresh authenticated SSH,
and management addresses/routes/rules. Audit all process root/cwd/exe/maps/fds,
mount namespaces including namespace fds without a live representative process,
and other block users/automounts. Require the exact static guard positive exit0 on
8:2 while root remains truly unmounted before any offline e2fsck/image operation.
If guard stays busy, identify the holder; never force fsck or close arbitrary PID1
descriptors. Any failed management/runtime/offline check stops the phase.

Then perform read-only filesystem diagnosis and measured metadata capture in the
private RAM root. The existing 8GiB capacity and controller budgets permit beginning
this bounded read-only phase; compressed image/undo sizes are not yet measured.
Keep both pipeline exit codes, image hashes and actual space consumption; a size
failure stops corrections until a concrete destination is available. Copy/hash-check
required preservation offhost before corrections or destroying RAM. Metadata/undo
do not cover every file byte or power failure; private auth/config backups are
selected recovery inputs, not a full image. Record CRC/SCSI/kernel baselines and
monitor across the approximately1GiB metadata/block read. Any new read/reset/media
error or unexplained counter increase requires review before writes. Filesystem
repair prompts, undo destination and final boot return remain separately reviewed.

Reviewer performed no target operation; all target execution above is attributed
to the host worker's actual evidence. Own documentation check on the preceding
checkpoint exited0: `tools/ci.sh check --base origin/main`, `check PASSED (0m14s)`.
No duplicate full quick was run.
