# Hardware installation and acceptance: 172.30.110.211

- Owner: worker `/root/host_211`; manager `/root`.
- Branch: `codex/hardware-211-20261010`.
- Worktree: `/root/ngfw-wt/hardware-211-20261010`.
- Product base: `d2d55984d74fa1d06c32e8271886f11f16375407` (`origin/main`).
- Owned files: `docs/status/tasks/hardware-211-20261010*`; task-specific deployment scripts only if later required and approved by manager.
- Owned remote target: `root@172.30.110.211`; no changes to the other target or shared development host.
- Authorization: owner requests package installation, tests, all interfaces except management/routing, and reboot if necessary. Management access and routing must remain functional.
- Constraints: exact manager-provided product payload; independent installation review; no replacing OS, mounted filesystem repair, blind nftables baseline/flush, management PCI rebinding, unsafe VFIO/no-IOMMU, secrets in evidence, or developer-host VPP changes.
- Current phase: actual RAM soft-reboot and positive offline guard/audit completed; persistentPTY and freshSSH2222/network preserved. Read-only fsck aborts12 on corruption; metadata-Q fails1 on invalid extent, so valid preservation/correction gate remains unmet. No repair, normal return reboot or installation yet.
- Runtime publication: commit coherent evidence and publish immediately to own branch; report actual remote SHA to manager.
- Acceptance: SSH and exact management routes preserved; packages installed from verified payload; firstboot/services/HTTPS/auth; 17 data interfaces persisted through API and present in VPP; routing/NAT/ACL/FRR and recovery/reboot tests with actual evidence where hardware links allow.

## Independent review assignment — 2026-10-10

Manager resumed this worker as R2 security reviewer of metadata candidate `2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c` on `codex/hardware-manager-20261010`. Own only `hardware-211-20261010-review-R2.md` and own envelope/WIP. No product edits or target operations. Review exact source diff, secrets and security boundaries, confirm unchanged policy/CI, then compare supplied final squash/PR tree and publish applicability. Narrow history scans only; no heavy `node_modules` scan or broad tests.

## Read-only private recovery capture — 2026-10-10

Manager resumed original host task solely for off-host configuration/network-state preservation. Own controller subdirectory `/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211` (0700), its private files (0600), and public task recovery receipt/WIP/envelope. Only selected `/etc/netplan`, `/etc/systemd/network`, `/etc/resolv.conf` are archived; live addresses/links/all-table routes/rules and PCI/name/IOMMU maps captured read-only. No target backup files, configuration/service/package/FS changes. Existing firewall save binaries may be read as fallback; no installation. Never commit private backup bytes or disclose contents; retain capture failures privately. Configuration backup only, not complete system/data backup or authorization to repair mounted root. Independent R7 checks metadata/completeness; offline recovery remains gated.

## Owner-requested root repair assessment — 2026-10-10

Owner now explicitly requests fixing disk/root filesystem; original installation/testing objective remains after recovery. Worker resumes same host/branch/worktree for read-only fresh filesystem/disk identity/health, platform/rescue/BMC/tool/capability checks and private-backup verification. Assess RAM maintenance preserving SSH and truly unmounting old root. No credential reads or diagnostic package installs; do not stage target files, change services/network/bootloader/mounts, repair mounted root, stop services, reboot, kexec or pivot without a concrete independently reviewed path. Manager coordinates any transition/repair one host at a time. Own public maintenance-preflight evidence plus existing task docs and private host-211 diagnostics only.


## Reversible RAM staging authorization

Manager authorizes only dedicated executable RAM tmpfs `/run/ngfwrescue`, isolated key-only management listener2222 and complete matching minimal rescue tree/runtime tests. Keep `/run/nextroot` absent. Original SSH/network/boot/disk configuration remains unchanged. SMART package may be downloaded/extracted exclusively into RAM only after official cached signature/index/package hash verification, with read-only smartctl -x only. No apt update/install, SMART test/enable/write control, soft-reboot, KeepConfiguration application, transition or fsck is authorized. Independent reviewer receives concrete tree/unit/procedure hashes before manager coordinates next action.

## Released phase — transition and read-only diagnosis only

Manager durably releases exact owned `/run/nextroot -> /run/ngfwrescue` and ordinary original22 soft-reboot after final independently applicable RAM audit helper, durable checkpoint, persistentPTY/otherhostSSH/controllercapacity gates. Keep PTY71683. After actual newRAMPID1/runtimehelper/SSH/network proof, enumerate process/device/namespace references and require exact guard0; only then read-only `e2fsck -f -n` and measured privateRAM e2image-Q plus verified compressed durable offhost metadata. No corrections/returnreboot/packageactivation/original-filedeletion. Namespace helper child-only setns reference inspection does not substitute for exclusive guard. Never commit confidential archives/logs.
