# Management persistence WIP

Branch: codex/test-host-management-20261007; base f6ae6e555ecb1edf5397ff1a8115076ebfc11d38.
Local/remote checkpoint: see git HEAD and origin/codex/test-host-management-20261007 (this checkpoint is published immediately).
Root SSH authenticated on both machines. Both report Ubuntu 26.04. Existing copied Netplan config specifies unrelated enp1s0 / 172.30.110.223 and does not persist current management addresses. networkd enabled-runtime; SSH service disabled. DNS currently 172.30.18.4, search Amnafzar.local. Target clocks incorrectly report April 2026.
Desired preserved management: 172.30.110.211/24 on enp4s0 via 172.30.110.1; 172.30.126.37/24 on enp12s0 via 172.30.126.1. SSH client 172.30.126.195.
Completed: read-only inventory. No target changes yet.
Remaining: backup, additive management Netplan config, generate, rollback-protected apply, persist networkd/SSH, independent reconnect, sequential reboot acceptance, installer preflight.
Next command: copy owned management YAML to targets after backing up /etc/netplan and /etc/resolv.conf; netplan generate, then bounded netplan try with independent SSH validation before confirmation.
