# Task: F-ab-upgrade — A/B image upgrade with automatic rollback   (prepend 00-CONTEXT.md)

## Goal
Atomic appliance upgrades (WBS D0.5 in `plan/wbs.csv`, T1, "required for GA-1"): install a new image into the inactive root slot
(rootA/rootB), boot it once with a watchdog, commit it when health checks pass, otherwise fall back to the previous slot automatically.
Delivered as a small, testable CLI `vrx-upgrade` + boot integration; the UI is F-backup-restore's (D8.7). **This host's boot configuration,
partitions and VPP are never touched** — everything is proven on loop-device disk images.

## Inputs to read first
- `docs/09-os-packages.md` §7 — partition layout: EFI 1 GB, root A 20 GB, root B 20 GB (reserved for D0.5), `/var/log`, `/var/lib/postgresql`, `/data`
  (update bundles live on `/data`)
- `prompts/P14-iso-installer.md` (creates the layout, rootB reserved) and `prompts/P10-packaging-deb.md` (packages, `vrx-firstboot`, `/etc/vrx`);
  `prompts/features/F-vpp-debs.md` (`manifest.json` of our VPP debs), D-001 (Ubuntu 26.04, our VPP debs), D-012 (no VPP restarts), D-059
- GRUB `grub-reboot`/`GRUB_DEFAULT=saved` + `grubenv` semantics; systemd `boot-complete.target` / `systemd-bless-boot` (Ubuntu 26.04 man pages)
- Host facts (verified 2026-09-24; no host package installs): present — sfdisk, losetup, mkfs.ext4, qemu-img, zstd, grub-editenv,
  grub-reboot, grub-install (x86_64-efi only), openssl, gpg, debootstrap (`resolute`), shellcheck; **absent** — sgdisk (use `sfdisk`),
  mksquashfs (ext4 is the default anyway), mkfs.vfat/mtools (format the EFI partition from a build chroot or leave it unformatted in the
  loop test), bats (use Go tests in `test/topology/ab-upgrade/` or a `deploy/upgrade/tests/run.sh`), `age`. debootstrap needs the Ubuntu
  mirror (`repo.amnafzar.ir`); if it is unreachable, write it in the questions file and test against a tiny hand-made root instead.
- **Host boot safety:** a bare `grub-reboot`/`grub-editenv`/`grub-install`/`update-grub` acts on THIS host's `/boot`. Every GRUB call
  takes an explicit path into the image (`grub-editenv <mnt>/boot/grub/grubenv …`, `grub-reboot --boot-directory=<mnt>/boot …`,
  `grub-install --boot-directory=<mnt>/boot --efi-directory=<mnt>/boot/efi --no-nvram …`); the CLI refuses to act on the running system's
  root/boot unless an appliance marker exists (e.g. `/etc/vrx/appliance`, written by the installer — design it, document it; tests and this
  host never have it)

## Scope — build exactly this
1. **Bundle format** `deploy/upgrade/`: `vrx-update-<version>.tar` = rootfs image (squashfs or tar.zst of a P14-style chroot) + `manifest.json`
   (version, sha256 of every member, min-from-version) + detached signature (the APT signing key from P10 or a dedicated Ed25519 key — key
   path outside the repo, never committed). Verification before anything is written; unsigned/mismatched bundle → refuse.
2. **CLI** `deploy/upgrade/vrx-upgrade` (Go or POSIX shell with fixed argv — no user input interpolated into commands): `status` (active slot,
   other slot version, pending/confirmed), `stage <bundle>` (verify → mkfs+extract into the inactive slot → copy `/etc/vrx` + machine identity →
   update fstab of the new slot), `activate` (one-shot boot into the other slot: `grub-reboot`), `confirm` (make it default), `rollback`.
   Persistent config (`/var/lib/postgresql`, `/data`, `/var/lib/vrx`) is shared, not copied; DB schema migration runs on first boot of the new
   slot with a pre-migration dump to `/data` for rollback.
3. **Boot integration**: `vrx-upgrade-health.service` in the new slot — waits for vrx-agent/vrx-api/VPP healthy (agent `Health` RPC on
   `/run/vrx/agent.sock`, API `GET /api/v1/health`)
   within N minutes, then `confirm`; a failure or a reboot before confirm leaves GRUB on the old default → automatic rollback. Document the
   watchdog path when the kernel itself hangs (hardware watchdog optional).
4. **Tests** (no KVM, no reboot here): build a sparse disk image with the §7 partition table on a loop device (`losetup`, `sgdisk`, only under
   `/root/ngfw-wt/F-ab-upgrade/.scratch/`, cleaned up), install a minimal debootstrap root into slot A, `stage` a bundle into slot B, assert
   grubenv/fstab/manifest results, simulate "health failed" → rollback leaves slot A default; bats or Go tests driving the CLI.
5. **Docs**: `docs/install/ab-upgrade.md` (bundle format, CLI, boot flow diagram, rollback, what the UI calls). The CLI's argv, exit codes
   and `status --json` output are a **contract** for F-backup-restore, which runs `vrx-upgrade` through the agent Action with fixed argv
   (op as an enum, bundle path under `/data/updates/`) — document them there and keep them stable.

Files you own: `deploy/upgrade/**`, `docs/install/ab-upgrade.md`, `test/topology/ab-upgrade/**`. Shared files: P10's install list (one line to
ship `vrx-upgrade` + `vrx-upgrade-health.service` in a vrx package, under your anchor); P14's autoinstall is read-only; any other change there
→ `docs/status/tasks/F-ab-upgrade-questions.md`. Loop-device tests run as root only behind `VRX_INTEGRATION=1` (ci.sh runs `test/**` Go
modules in unit mode, where they must skip).

## Acceptance (paste the evidence)
- [ ] Loop-image test: stage → grubenv shows one-shot entry for slot B → simulated health OK → default = B; simulated failure → default stays A (pasted)
- [ ] Tampered bundle (one byte changed) and unsigned bundle are refused before any write (pasted)
- [ ] `lsblk`, `/boot/grub/grubenv` (sha256) of **this host** unchanged before/after (pasted); `losetup -a` shows none of yours left
      (filter by your `.scratch/` backing files — P14/F-images may hold their own loop devices at the same time)
- [ ] Real boot of both slots on a VM **deferred** (no KVM on this host, D-002) — listed in `docs/status/tasks/F-ab-upgrade.md` with the exact VM test plan
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
The upgrade UI and staged-rollout screens (F-backup-restore, D8.7); ISO/autoinstall changes (P14); VM/cloud images (F-images); package signing policy
and secure boot (F-hardening-lite; secure boot is out entirely); air-gapped update bundle as a product (D12.1, list as have-not); VPP package build
(F-vpp-debs); upgrading this host.

## Open questions to surface, not to decide silently
Squashfs (read-only root + overlay) vs a writable ext4 slot — default ext4 for FAST MODE, flag the trade-off. Which key signs bundles (reuse P10's
APT key vs dedicated key) — default dedicated Ed25519, stored like P10's key outside the repo.
