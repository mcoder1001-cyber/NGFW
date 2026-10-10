# Hardware .37 installation and acceptance envelope

- Owner: agent `/root/host_37`; manager `/root`.
- Branch: `codex/hardware-37-20261010`.
- Worktree: `/root/ngfw-wt/hardware-37-20261010`.
- Base: `d2d55984d74fa1d06c32e8271886f11f16375407` (`origin/main`).
- Owned remote host: `root@172.30.126.37` only.
- Owned repository files: `docs/status/tasks/hardware-37-20261010*`; any additional task-specific deployment script requires declaration to the manager.
- User authorization: install NGFW packages and test; add all nonmanagement physical data interfaces; reboot if necessary while preserving management ports and routing.
- Exclusions: other agents' worktrees, main, development-host `/etc/vpp`, `/root/vpp`, VPP services, host SSH/security boundary changes, unsafe VFIO/no-IOMMU, operating-system replacement.
- Dependency: manager supplies reviewed current package payload; independent reviewer approves concrete management-preservation installation plan.
- Current stop condition: root filesystem `/dev/sda2` has real corruption. Do not install, reboot, or repair a mounted filesystem. Manager has requested console/recovery availability from the owner.
- Durable evidence must contain no credentials, secret files, authentication tokens or password hashes.
