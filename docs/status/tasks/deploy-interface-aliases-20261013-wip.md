# Interface aliases and dual-host deployment

Branch: `codex/deploy-interface-names-ngfw`, based on fetched main `2955461ea`.
Original work is retained at `codex/interface-names-ngfw-cli` (`2f54c8b7b`).
Latest-main checkpoint: `37d4885db`; follow-up includes conflict-resolution repairs,
inventory-only NIC aliases and packaged `/usr/bin/ngfw`.

Authorized scope: commit the requested Ethernet display names and CLI naming;
install the latest integrated product on `root@172.30.110.211` and `root@172.30.126.37`.
Owned files: interface display-name helper/tests, interface page/drawer/tests,
dashboard and VLAN display consumers, interface documentation, Debian CLI staging/install.

Independent read-only review approved the follow-up. ESLint, shell syntax and
`tools/ci.sh check --base origin/main` passed. Latest-main API/web build is running.
Full quick gate and package verification remain pending.

Both hosts are reachable by SSH. Host 211 is a fresh Ubuntu 26.04.1 installation;
its changed SSH host key was explicitly confirmed by the user. Its root partition
was safely grown into unused disk space after backing up the partition table and
fstab; root now has 224 GiB free. Host 37 runs package build `97ae88ee5b6a` with
API, agent, nginx and VPP active. Preserve its database, secrets, management network
and VPP startup configuration during upgrade.

Verified VPP runtime artifacts are `26.06-release+ngfw3`; install-gate verification
passed. Transfer to host 211 is running. No product package has yet been installed.

Next: finish API/web build, run quick gate, commit/publish this checkpoint,
prepare Debian packages with `deploy/debian/ngfw/prepare.sh`, validate package contents,
install runtime dependencies and firstboot on 211, upgrade packages on 37, and
verify installed version, CLI, HTTP and service health on both hosts.
