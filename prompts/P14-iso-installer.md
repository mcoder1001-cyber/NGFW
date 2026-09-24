# Task P14 — Unattended installer ISO   (prepend 00-CONTEXT.md)

## Goal
A bootable ISO that installs **Ubuntu 26.04** + VRX packages (including our source-built VPP 26.06 debs) unattended on a blank
machine or VM and boots into a working appliance with a bootstrap admin password shown on the console.

## Read first
`docs/09-os-packages.md` §3/§4/§7, `prompts/P10-packaging-deb.md` + P10's merged `deploy/debian/vrx/`, `deploy/apt/` and
`docs/install/bare-metal.md` (firstboot, `/etc/vrx/bootstrap.env` = `VRX_BOOTSTRAP_ADMIN_USER/PASSWORD`, the published repo under
`/srv/vrx-artifacts/apt/`), `deploy/vpp/README.md` (P14 bullet: copy the same VPP files, record `upstream.commit`, `patches[].sha256`,
`build.inputs`), `docs/agent/renderers/vppstartup.md`, `docs/decisions/PENDING-vpp-host-hardening.md` option B (hugepages on the kernel
cmdline, `net.core.rmem_max` for linux_nl), `docs/decisions/LOG.md` D-001, D-002, D-058, D-081, D-089, D-092.

## Host facts (verified 2026-09-24; no host package installs)
No Ubuntu ISO is on this host: the 26.04 live-server ISO (subiquity autoinstall base, ~3 GB) must be downloaded once and verified against
Ubuntu's `SHA256SUMS` + its GPG signature — if unreachable, write it in the questions file (PENDING-network) and build everything else.
Present: xorriso, grub-mkrescue/grub-mkstandalone (**x86_64-efi only**), genisoimage, gpg, zstd, debootstrap (`resolute`), systemd-nspawn.
Absent: mtools/dosfstools (the EFI image), GRUB i386-pc (BIOS), isolinux, cloud-init (schema check), squashfs-tools → use them inside a
build chroot under your worktree's `.scratch/` (installed there from the mirror `repo.amnafzar.ir`) or `apt-get download` + `dpkg -x`
under `.scratch/` — never on the host. No KVM (D-002).

## Build exactly this
1. `deploy/image/iso/`: Ubuntu Server 26.04 autoinstall (`user-data`/`meta-data`, cloud-init
   nocloud) with the partition layout from `docs/09-os-packages.md` §7 (EFI, rootA, rootB
   reserved, /var/log, /var/lib/postgresql, /data), LUKS optional flag, no default user,
   SSH keys optional, our APT repo added from an embedded pool so **installation works offline**
   (all `.deb`s from P10's published repo and their dependency closure in the ISO pool). Pieces F-images will reuse (package list,
   first-boot banner unit, bootstrap-password writer) go in `deploy/image/common/` (yours; F-images reads it).
2. Late-commands: install `vrx-meta`, purge NetworkManager/snapd/unattended-upgrades/ufw,
   disable irqbalance, write `/etc/vrx/bootstrap.env` with a random admin password, apply the
   kernel cmdline defaults from `scripts/30-tune-dataplane.sh` / docs/09 §4 (flag 1G vs 2M hugepages against PENDING-vpp-host-hardening
   option B in the questions file). VPP stays off the NICs until the hardware wizard in the UI binds them: P10's firstboot renders
   startup.conf with `vrx-startupgen` and no devices → `dpdk { no-pci }` (there is no `dpdk { disable }` stanza in VPP).
3. First-boot console banner (`/etc/issue` + a systemd unit) prints management IP, HTTPS URL,
   and the bootstrap password once; the password file is deleted after first login.
4. Build script `deploy/image/build-iso.sh` (xorriso, grub-efi) producing `vrx-<version>.iso` + SHA256 + GPG signature (key outside the
   repo, never printed; reuse P10's APT key or a dedicated one — questions file). A `tools/ci.sh iso` long target is the manager's
   (tools/ci.sh is manager-only): ask for it in the questions file. Outputs and the downloaded base ISO live under `.scratch/` (never in git).
5. Test: (a) reproducible build (same inputs → same package manifest), (b) `xorriso -indev` listing shows pool + autoinstall files,
   (c) `cloud-init schema --config-file user-data` validates (cloud-init from the build chroot). The QEMU boot test **cannot run on this
   host (no KVM)** — it is deferred to a VM the product owner provides; say so in `docs/status/tasks/P14.md`.
6. `docs/install/iso.md` with BIOS/UEFI notes, supported-VM notes, and the offline-install story.

## Acceptance
- [ ] Zero prompts during install; works with the network cable unplugged — proven statically (autoinstall has no interactive sections,
      the pool resolves every dependency of `vrx-meta` offline: `apt-get install --simulate` against the pool only, pasted); the real
      install is **deferred** to a product-owner VM
- [ ] Re-running the build with the same inputs produces an identical package set (record the manifest)
- [ ] This host unchanged: no mounts or loop devices of yours left (`findmnt`, `losetup -a` filtered to your `.scratch/`), `/boot` untouched

## Out of scope
A/B upgrade mechanism (uses rootB later), cloud images, secure boot signing, any change to this host's packages or boot configuration.
