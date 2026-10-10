# Hardware installation task envelope

Owner request (2026-10-10): install NGFW packages on root@172.30.126.37 and
root@172.30.110.211, test them, preserve management and routing, add the remaining
physical interfaces, reboot if necessary.

Base: origin/main d2d55984d. Branch: codex/hardware-manager-20261010.
Worktree: /root/ngfw-wt/hardware-manager-20261010.
Owned files: docs/status/tasks/hardware-manager-20261010*,
deploy/debian/ngfw/debian/control (native API dependency correction), and external build output.
Do not edit main, another agent's worktree, shared development host VPP or secrets.

Workers: host_37 owns its isolated hardware-37-20261010 branch/worktree and .37;
host_211 owns hardware-211-20261010 and .211; install_review owns an independent
review branch and reports only. One worker per target; reviewer makes no host changes.

Recovery gate: both targets have independently confirmed preexisting ext4 root
corruption and failed boot fsck. No package installation or reboot until an offline
recovery path is available and root filesystem is clean. Console/recovery access
requested from owner. Do not repair a mounted root filesystem.

Management exclusions: .37 enp12s0, 172.30.126.37/24, default172.30.126.1;
.211 enp4s0, 172.30.110.211/24, default172.30.110.1. Management PCI devices and
their IOMMU groups must stay outside DPDK. Import data ports through persistent
product configuration. Existing physical af_packet import is unsupported by D-105.

Acceptance: package install/configuration; protected management/default routes and
fresh SSH; service/API/TLS health; persistent data-interface inventory and real
forwarding tests; restart/reboot only with recovery available. Record actual failures
and unexecuted tests accurately. No throughput or aggregate release claim.
