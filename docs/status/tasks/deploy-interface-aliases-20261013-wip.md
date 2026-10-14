# Interface aliases and dual-host deployment

Branch: `codex/deploy-interface-names-ngfw`, based on fetched main `2955461ea`.
Original work is retained at `codex/interface-names-ngfw-cli` (`2f54c8b7b`).
Latest-main checkpoint: `37d4885db`; follow-up includes conflict-resolution repairs,
inventory-only NIC aliases and packaged `/usr/bin/ngfw`.

Authorized scope: commit the requested Ethernet display names and CLI naming;
install the latest integrated product on `root@172.30.110.211` and `root@172.30.126.37`.
Owned files: interface display-name helper/tests, interface page/drawer/tests,
dashboard and VLAN display consumers, interface documentation, Debian CLI staging/install.

Independent read-only review approved the follow-up. ESLint, shell syntax,
`tools/ci.sh check --base origin/main`, API/web build and Debian package fixture
tests passed. The first quick run found one old physical-port test expecting
`ens161`; expectation-only correction `735835adb` is committed and published.
The final quick run passed 662 web tests, 783 API tests, agent/CLI lint,
race tests/builds, all unit-mode topology modules and the VPP deployment-script
harness (all four shards passed). It exited 0 with `CI GATE PASSED`, wall time
19m50s. Full logs: `/root/ngfw-wt/logs/ci/NGFW-20261013-121346-70774`.
The gate started at test checkpoint `735835adb`; subsequent changes only record
deployment evidence and do not change compiled files or test assertions.

Both hosts are reachable by SSH. Host 211 is a fresh Ubuntu 26.04.1 installation;
its changed SSH host key was explicitly confirmed by the user. Its root partition
was safely grown into unused disk space after backing up the partition table and
fstab; root now has 224 GiB free. Host 37 runs package build `97ae88ee5b6a` with
API, agent, nginx and VPP active. Preserve its database, secrets, management network
and VPP startup configuration during upgrade.

Verified VPP runtime artifacts are `26.06-release+ngfw3`; install-gate verification
passed. Both hosts now have `ngfw-agent`, `ngfw-api`, `ngfw-web` and `ngfw-meta`
`0.1.0~dev+48ad9f3e9990`. Compiled product files are unchanged by the subsequent
expectation-only test commit. Both development branches were published to GitHub;
remote product checkpoint is `48ad9f3e9990`, remote test checkpoint is `735835adb`.

Host 211 required normalizing its existing relative Asia/Tehran localtime link
to the same absolute zone path for the strict identity provisioner. Its management
interface `enp4s0` was set explicitly for firewall bootstrap. Firstboot completed
and deleted bootstrap credentials. HTTPS `/` returns 200, public API health is ok,
authenticated agent health is reachable, VPP-connected and not degraded;
18 interfaces are observed with host inventory/data plane available and no
observation errors. No failed units remain.

Host 37 upgrade preserved its database, secrets and management configuration;
the VPP startup configuration matches its preupgrade checksum. Package/systemd
upgrade restarted the services, including VPP; subsequent reconcile succeeded.
HTTPS `/` returns 200, API health is ok, direct agent health reports VPP connected
with no degraded flag, and `/usr/bin/ngfw --version` reports `48ad9f3e9990`.
The unrelated network wait-online failure predates this deployment (2026-10-10).

On both hosts the installed CLI matches the packaged CLI bytes, SHA256
`e28ea92f5304669707ccfb5af38f49f5c6f6a3c0701193ae676bfaa4d621077c`.
The served interface display-name JS matches the new production build, SHA256
`73c8b8d01dee6c6b324b27bb0e5c440bb5a0a763e42b9c88a00f3aeee1944538`.
Both package transfers passed SHA256 verification and `dpkg --audit` is empty.
Packages/backups/check logs are retained in `/root/ngfw-deploy` on each host;
local build and gate logs are in this worktree's `.scratch/deploy`.

Completed: packages installed and verified on both hosts, original code/test
checkpoints published, complete quick gate passed, and completion receipt committed
and published. Web URLs: `https://172.30.110.211/`, `https://172.30.126.37/`.
