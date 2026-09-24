# Task P14 — Unattended installer ISO   (prepend 00-CONTEXT.md)

## Goal
A bootable ISO that installs **Ubuntu 26.04** + VRX packages (including our source-built VPP 26.06 debs) unattended on a blank
machine or VM and boots into a working appliance with a bootstrap admin password shown on the console.

## Build exactly this
1. `deploy/image/iso/`: Ubuntu Server 26.04 autoinstall (`user-data`/`meta-data`, cloud-init
   nocloud) with the partition layout from `docs/09-os-packages.md` §7 (EFI, rootA, rootB
   reserved, /var/log, /var/lib/postgresql, /data), LUKS optional flag, no default user,
   SSH keys optional, our APT repo added from an embedded pool so **installation works offline**
   (all `.deb`s from P10 and their dependencies in the ISO pool).
2. Late-commands: install `vrx-meta`, purge NetworkManager/snapd/unattended-upgrades/ufw,
   disable irqbalance, write `/etc/vrx/bootstrap.env` with a random admin password, apply the
   kernel cmdline defaults from `scripts/30-tune-dataplane.sh` **but with `dpdk { disable }`**
   until the hardware wizard in the UI enables it.
3. First-boot console banner (`/etc/issue` + a systemd unit) prints management IP, HTTPS URL,
   and the bootstrap password once; the password file is deleted after first login.
4. Build script `deploy/image/build-iso.sh` (xorriso, grub-efi) producing `vrx-<version>.iso` + SHA256 + GPG signature; a `tools/ci.sh iso`
   long target (manual).
5. Test: (a) reproducible build (same inputs → same package manifest), (b) `xorriso -indev` listing shows pool + autoinstall files,
   (c) `cloud-init schema --config-file user-data` validates. The QEMU boot test **cannot run on this host (no KVM)** — it is deferred to
   a VM the product owner provides; say so in `docs/status/tasks/P14.md`.
6. `docs/install/iso.md` with BIOS/UEFI notes, supported-VM notes, and the offline-install story.

## Acceptance
- [ ] Zero prompts during install; works with the network cable unplugged
- [ ] Re-running the build with the same inputs produces an identical package set (record the manifest)

## Out of scope
A/B upgrade mechanism (uses rootB later), cloud images, secure boot signing.
