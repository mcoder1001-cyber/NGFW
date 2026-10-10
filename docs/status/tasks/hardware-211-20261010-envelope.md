# Hardware installation and acceptance: 172.30.110.211

- Owner: worker `/root/host_211`; manager `/root`.
- Branch: `codex/hardware-211-20261010`.
- Worktree: `/root/ngfw-wt/hardware-211-20261010`.
- Product base: `d2d55984d74fa1d06c32e8271886f11f16375407` (`origin/main`).
- Owned files: `docs/status/tasks/hardware-211-20261010*`; task-specific deployment scripts only if later required and approved by manager.
- Owned remote target: `root@172.30.110.211`; no changes to the other target or shared development host.
- Authorization: owner requests package installation, tests, all interfaces except management/routing, and reboot if necessary. Management access and routing must remain functional.
- Constraints: exact manager-provided product payload; independent installation review; no replacing OS, mounted filesystem repair, blind nftables baseline/flush, management PCI rebinding, unsafe VFIO/no-IOMMU, secrets in evidence, or developer-host VPP changes.
- Current phase: read-only preflight and recovery/install/test planning. Manager explicitly forbids installation, reboot and filesystem repair pending console/recovery information after confirmed filesystem corruption.
- Runtime publication: commit coherent evidence and publish immediately to own branch; report actual remote SHA to manager.
- Acceptance: SSH and exact management routes preserved; packages installed from verified payload; firstboot/services/HTTPS/auth; 17 data interfaces persisted through API and present in VPP; routing/NAT/ACL/FRR and recovery/reboot tests with actual evidence where hardware links allow.
