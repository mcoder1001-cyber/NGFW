# Task: F-images — VM and cloud image builds   (prepend 00-CONTEXT.md)

## Goal
Produce VRX appliance disk images for hypervisors and clouds (WBS D11.2 T1: KVM/VMware/Hyper-V/Proxmox images + SR-IOV/PCI-passthrough notes;
D11.1 T2: AWS/Azure/GCP images, cloud-init, ENA/Hyper-V drivers) from the same package set as the installer ISO. FAST MODE: **build and
validate the artefacts offline**; booting them needs KVM or cloud accounts, which this host does not have (D-002) — that part is deferred.

## Inputs to read first
- `prompts/P14-iso-installer.md` — autoinstall `user-data`, partition layout, first-boot banner, offline pool; reuse its package list/pool, do not fork it
- `prompts/P10-packaging-deb.md` — `vrx-meta` and the local APT repo; `prompts/features/F-vpp-debs.md` — our VPP debs + `manifest.json`
- `docs/09-os-packages.md` §4 (kernel cmdline, hugepages) and §7 (partition layout incl. rootA/rootB for F-ab-upgrade)
- `docs/decisions/LOG.md` D-001 (Ubuntu 26.04, our VPP debs), D-002 (no Docker, no libvirt/nested KVM), D-059
- `prompts/features/F-startup-gen.md` — VPP must boot with `dpdk { no-pci }` / hardware wizard later, as in P14

## Scope — build exactly this
1. **Builder** `deploy/image/vm/build.sh`: debootstrap (or `mmdebstrap`) Ubuntu 26.04 into a raw sparse image with the §7 layout (GPT, EFI, rootA,
   rootB reserved, /var/log, /var/lib/postgresql, /data) on a loop device under `/root/ngfw-wt/F-images/.scratch/` (git-ignored, cleaned up),
   install `vrx-meta` from the local repo, install GRUB for UEFI + BIOS, cloud-init with the NoCloud + ConfigDrive datasources, serial console
   enabled, no default password (bootstrap password printed once as in P14), machine-id emptied, SSH host keys regenerated on first boot.
2. **Formats**: `qemu-img convert` → `qcow2` (KVM/Proxmox), `vmdk` streamOptimized + `.ovf/.ova` (VMware, vmxnet3), `vhdx` (Hyper-V); each with
   SHA256 + manifest (package versions incl. VPP). Long builds (> 8 min) via `nohup … > /root/ngfw-wt/logs/F-images-build.log 2>&1 &`, polled.
3. **Cloud profiles** `deploy/image/cloud/`: per-cloud overlays (AWS: ENA module + `ec2` datasource, raw → VHD/`vmdk` for import; Azure: Hyper-V
   drivers + fixed-size VHD aligned to 1 MiB; GCP: `gce` datasource, `disk.raw` tar.gz) and the documented import command lines
   (`aws ec2 import-image`, `az image create`, `gcloud compute images create`) — **not executed**; marketplace listing text is not written.
4. **Validation without booting**: `virt-inspector`/`guestfish` only if installed (never apt install), else `losetup -P` + read-only mount:
   check fstab, packages (`dpkg-query --admindir`), cloud-init datasource list, grub.cfg, nftables/systemd units present, no secrets or build-host
   keys inside the image; `qemu-img check`/`info` on every output.
5. **Docs**: `docs/install/images.md` — per-hypervisor import steps, NIC notes (vmxnet3, virtio, ENA, SR-IOV/PCI passthrough requirements for DPDK),
   minimum sizing (16 GB RAM per §7), and the deferred boot test plan.

Files you own: `deploy/image/vm/**`, `deploy/image/cloud/**`, `docs/install/images.md`, `test/topology/images/**`. Shared files: none — P14's
`deploy/image/iso/**` is read-only for you (propose a shared `deploy/image/common/` in the questions file if duplication is large).

## Acceptance (paste the evidence)
- [ ] One full build producing qcow2 + ova + vhdx with SHA256SUMS and manifest (pasted `ls -l`, `qemu-img info`)
- [ ] Offline inspection output: partition table, fstab, `vrx-meta` + VPP 26.06 versions, cloud-init datasources, no `/etc/ssh/ssh_host_*`, empty machine-id
- [ ] This host unchanged: `losetup -a` empty after the build, no mounts left (`findmnt | grep F-images` empty)
- [ ] Boot on KVM/VMware/Hyper-V and cloud import **deferred** with an exact test plan in `docs/status/tasks/F-images.md`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
The installer ISO (P14); A/B upgrade logic (F-ab-upgrade — only leave rootB in the layout); cloud routing integration / HA with cloud route tables
(D11.1 remainder — have-not); marketplace publishing; image signing and secure boot (F-hardening-lite / out); Docker or libvirt anything (D-002);
VPP package build (F-vpp-debs); SR-IOV configuration automation.

## Open questions to surface, not to decide silently
Which formats are release-blocking (proposal: qcow2 + ova T1, cloud T2). Whether cloud images ship with DPDK enabled for ENA/netvsc by default (P14 ships `dpdk { disable }`).
