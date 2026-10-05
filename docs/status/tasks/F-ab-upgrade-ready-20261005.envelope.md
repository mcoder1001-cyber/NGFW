# F-ab-upgrade execution envelope
Branch: codex/ready-ab-20261005
Worktree: /root/ngfw-wt/ready-ab-20261005
Base: origin/main e5dba658
Slot: 15, prefix w15. No host boot, packages, services, partitions or VPP changes.
Owned: deploy/upgrade/**, docs/install/ab-upgrade.md, test/topology/ab-upgrade/**, F-ab-upgrade status.
Shared: minimal package preparation/install hooks only; manager owns board and CI hooks.
Implement signed verified ext4 staging and lifecycle. Run unchanged quick gate; independent review and merge owned by manager. VM boot acceptance may defer.
