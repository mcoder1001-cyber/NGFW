# TASK ENVELOPE — F-ab-upgrade
id: F-ab-upgrade   branch: task/F-ab-upgrade   worktree: /root/ngfw-wt/F-ab-upgrade   base: main@<BASE>   started: <STARTED>
title: S5 system (day 16-18): A/B image upgrade with automatic rollback
prompt: prompts/features/F-ab-upgrade.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main@11a175b: host tools, host boot safety, `/api/v1/health`, CLI contract for F-backup-restore)   wbs: D0.5
scope: signed update bundle format, the `vrx-upgrade` CLI (status/stage/activate/confirm/rollback), `vrx-upgrade-health.service` boot integration, tests on sparse loop-device disk images with P14's §7 layout; the real two-slot VM boot is deferred (no KVM). The UI is F-backup-restore's
merged deps you can rely on: P10, P14 (+ F-vpp-debs)
  - P10: packages, units, firstboot, `/etc/vrx`, `/var/lib/vrx`, `/data/updates`, the APT signing key location (`~/.config/ngfw/apt-signing/`), the SY6 install-list anchor `# wave-BC: F-ab-upgrade`
  - P14: the §7 partition layout and labels (`VRX-EFI`, `vrx-rootA`, `vrx-rootB`, `vrx-log`, `vrx-pg`, `vrx-data`), the appliance marker `/etc/vrx/appliance`, `deploy/image/common/`
  - F-vpp-debs: `manifest.json` of the VPP debs (bundle manifest records it)
read first: prompts/features/F-ab-upgrade.md · docs/status/wave-BC-numbers.md (section "S5 system": SY6, SY7, SY9 + "P14", "F-ab-upgrade") · docs/09-os-packages.md §7 · docs/install/iso.md + docs/install/bare-metal.md · prompts/features/F-backup-restore.md (your consumer) · docs/decisions/LOG.md D-001, D-002, D-012, D-059, D-092
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - disk images, mounts, the debootstrap root and bundles live under /root/ngfw-wt/F-ab-upgrade/.scratch/ (names carry w<SLOT>); loop devices only on your own `.scratch/` files; the health-check test talks to your slot API port / slot agent socket (or fakes), never to the product stack
  - slots 1–11 only; 12 is CI
daemon-owner: none (nothing runs as a daemon on the host; `vrx-upgrade-health.service` is only ever enabled inside an image)
HOST BOOT SAFETY (this host is vrx-a; D-012):
  - every GRUB call takes an explicit path into the image: `grub-editenv <mnt>/boot/grub/grubenv …`, `grub-reboot --boot-directory=<mnt>/boot …`, `grub-install --boot-directory=<mnt>/boot --efi-directory=<mnt>/boot/efi --no-nvram …`; never `update-grub`, never a bare `grub-reboot`
  - the CLI refuses to act on the running system's root/boot unless the appliance marker exists; this host never has it and tests never create it outside `.scratch/`
  - paste `lsblk`, `sha256sum /boot/grub/grubenv` and `losetup -a` (filtered to `.scratch/`) before and after every root test run
obligations:
  - verify before write: signature + per-member sha256 + min-from-version are checked before anything touches a slot; unsigned, tampered (one byte) or mismatched → refuse, nothing written (pasted)
  - no user input interpolated into commands (rule 9): fixed argv, the bundle path is an argument validated to be a regular file under the configured updates dir
  - the CLI's argv, exit codes and `status --json` are F-backup-restore's contract (it runs `vrx-upgrade` through the agent Action). Document them in docs/install/ab-upgrade.md and keep them stable; decide with P10's units how the agent triggers it (recommended: fixed-name `vrx-upgrade@<op>.service` units outside the agent sandbox, shipped by you) and log the choice with options
  - persistent data (`/var/lib/postgresql`, `/data`, `/var/lib/vrx`) is shared, not copied; the DB migration runs on first boot of the new slot after a pre-migration dump to `/data`
  - signing key: default a dedicated Ed25519 key in `~/.config/ngfw/upgrade-signing/` (0700; `openssl` is present, `age` is not). Never committed, never printed; tests use throwaway keys under `.scratch/`
  - D-059: an air-gapped update bundle as a product and secure boot are have-nots — list them
files you own exclusively:
  - deploy/upgrade/** (CLI, health unit + script, bundle builder/verifier, `tests/run.sh` unit-style tests without root)
  - docs/install/ab-upgrade.md
  - test/topology/ab-upgrade/** (Go module driving the CLI on loop images; root/loop parts behind `VRX_INTEGRATION=1`, skipped in unit mode as ci.sh runs `test/**` modules)
  - docs/status/tasks/F-ab-upgrade*
shared hotspots (append-only, conflicts resolved by the manager at merge; ids from docs/status/wave-BC-numbers.md "S5 system"):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below `# wave-BC: F-ab-upgrade` (seeded by P10). List every hunk under "Shared hunks" in docs/status/tasks/F-ab-upgrade.md
  - SY6 deploy/debian/vrx/** (P10's install list): one line each for `vrx-upgrade` and `vrx-upgrade-health.service` (+ `vrx-upgrade@.service` if you choose it)
  - SY7 root .gitignore `/.scratch/`: seeded by the manager; if absent, never `git add -A`
  - SY9 tools/ci.sh: ask for a `deploy/upgrade` shellcheck + `tests/run.sh` step (like TD-6's deploy/vpp step); never edit it
contract numbers: none (docs/status/wave-BC-numbers.md "F-ab-upgrade": names only)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc, /boot, /data and the disks of this host, /root/vpp, host packages or services
  - deploy/image/** (P14/F-images — read-only), deploy/debian/** beyond your anchor lines, deploy/systemd/** + deploy/firstboot/** (P10), deploy/vpp/**
  - apps/** (the agent Action is F-backup-restore's), packages/**, tools/*, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md
host facts (verified 2026-09-24; no host package installs):
  - present: sfdisk, losetup, mkfs.ext4, qemu-img, zstd, grub-editenv, grub-reboot, grub-install (x86_64-efi only), openssl, gpg, debootstrap (`resolute`), shellcheck, systemd-nspawn
  - absent: sgdisk (use sfdisk), mksquashfs (ext4 slots are the default), mkfs.vfat/mtools (format the EFI partition from a build chroot, or leave it unformatted in the loop test), bats (Go tests or `tests/run.sh`), `age`
  - debootstrap needs the mirror `repo.amnafzar.ir`; if it is unreachable, questions file + a tiny hand-made root for the tests
  - no KVM (D-002): the real two-slot boot is deferred with an exact VM test plan in F-ab-upgrade.md
coordination:
  - F-backup-restore (depends on you): the CLI contract and the trigger mechanism; tell it in your status file what is stable
  - P10 / F-hardening-lite: which unit runs the CLI and with which sandbox · P14: layout, labels, marker · F-images: same layout for its disk images
evidence: loop-image test (stage → grubenv one-shot entry for slot B → simulated health OK → default = B; simulated failure → default stays A), tampered/unsigned refusals, host-unchanged listings, `shellcheck` of deploy/upgrade/*
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-ab-upgrade.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-ab-upgrade-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-ab-upgrade.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: every mount under `.scratch/` unmounted (`findmnt | grep /root/ngfw-wt/F-ab-upgrade` empty), every loop device backed by your files detached, images and the debootstrap root deleted, throwaway keys deleted, processes stopped by PID
questions: docs/status/tasks/F-ab-upgrade-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own · touch this host's boot configuration, partitions or packages
