# Package installation on a fresh appliance

P10 is under development. These steps describe the intended product installation;
clean resolute install/boot and real signed repository acceptance are NOT RUN.
Do not run them on the shared lab/development host. The signing trust anchor must
be provisioned through an independently authenticated channel; never enable
unsigned/insecure APT options. Use Ubuntu 26.04 amd64 and product VPP packages
verified from deploy/vpp, not FD.io/upstream unsuffixed packages.

1. Provision the reviewed public archive keyring as
   `/usr/share/keyrings/ngfw-archive-keyring.gpg` and configure the product repository
   with `Signed-By` pointing to that file. Install only a tested release with
   `apt-get install ngfw-meta`; the installer must not start VPP before firstboot.
2. Create `/etc/ngfw/bootstrap.env` as root, mode 0600, using a local secure editor.
   It contains `NGFW_BOOTSTRAP_ADMIN_USER` and `NGFW_BOOTSTRAP_ADMIN_PASSWORD` in
   systemd EnvironmentFile syntax. Also specify `NGFW_BOOTSTRAP_MGMT_IF` as the
   explicit existing management interface and optional comma-separated
   `NGFW_BOOTSTRAP_PUNT_IFS` as existing VPP/Linux-CP interfaces (no wildcard).
   Early firewall generation persists these nonsecret interface inputs separately
   before the database/bootstrap phase. Passwords must meet the current API policy.
   Do not put credentials in command arguments, repository files or logs.
3. Provision hugepages and management networking independently before boot.
   Startup generation deliberately refuses missing host facts instead of claiming
   a safe configuration. The default dataplane document has no PCI devices.
4. Firstboot creates the fixed local `ngfw` database/role, runs application Drizzle
   migrations and existing admin seeding, verifies the persisted expected admin
   password, and generates stable secret/JWT keys. It creates a self-signed TLS
   pair, renders startup.conf with ngfw-startupgen and validates nginx.
5. The durable completion marker is published before bootstrap credentials are
   deleted. If interrupted at that boundary, the next unit execution completes
   credential cleanup. Failures preserve the bootstrap file; inspect the fixed,
   sanitized error and remedy the prerequisite, then retry the unit.
6. On a fresh device only, reboot and verify firstboot, VPP, agent, API, PostgreSQL,
   Valkey and nginx unit states. VPP/API/agent require successful firstboot.
   Trust the self-signed certificate through a local verified fingerprint workflow,
   then replace it with the appliance management certificate.

Initial API env is canonical and accepts only the three generated database, secret-key
and JWT-key fields. Configure optional runtime overrides after provisioning, preserving
the effective key precedence documented by the API.

The shipped initial dataplane document explicitly enables `linux_cp_plugin.so`,
`linux_nl_plugin.so` and `npt66_plugin.so` for appliance policy and routing support
(D-060). These plugins are disabled by default upstream even when their files are
installed. Firstboot copies this fixed document with mode0600 and renders it using
the normal host-validated generator. No physical devices are configured: startup
retains `no-pci` and the management PCI blacklist. A missing required plugin stops
generation before the firstboot completion marker or credential deletion.
Provisioning an existing firstboot-complete appliance does not rerun this step;
correct its startup through the separately guarded product apply procedure below.

The API package provisions `/data`, `/data/backups`, `/data/updates` and
`/data/support` as `ngfw:ngfw`, mode 0750. Package reconfiguration reapplies these
directory permissions without changing the ownership, contents or permissions of
existing files inside them. These paths reserve storage for backup, update and
support operations; provisioning them does not imply those features are complete.
Provisioning rejects symlinks in storage paths. Ownership and modes are applied
through opened directory descriptors, so concurrent link replacement cannot redirect
root's permission changes outside the selected directory. If configuration refuses
an unexpected link, inspect and repair it through the appliance console; it is not
silently removed or followed. Existing ancestor directories retain their ownership.

Remaining release prerequisites: dynamic LCP/punt-set synchronization, package
licensing metadata and reviewed agent capability/daemon-file ownership compatibility.
No installation is declared accepted until the deferred appliance campaign records
actual command outputs and results. Later VPP startup changes use the reviewed
`apply-startup.sh --mode product` approval/recovery procedure, not firstboot.

The product agent unit sets `NGFW_VPP_ID_RANGE=all` for its dedicated appliance.
Never start this unit on the shared development host: workers there must use their
allocated `NGFW_VPP_TABLE_BASE`. Do not set both variables in `agent.env`; the agent
refuses ambiguous ID ownership instead of starting.

The runtime-dependency installer temporarily denies package-driven service starts and
restores the original policy-rc.d after completion/error. It does not make an incomplete
installation boot-ready: provision the full product packages, explicit bootstrap/interface
inputs and reviewed units before reboot. If forcibly terminated without EXIT cleanup,
restore the retained original start-policy backup through the local console.


Dynamic punt admission is enabled explicitly by the product agent unit with
`NGFW_BASE_POLICY=1`, only for the `ngfw` globals owner. The agent loads root-owned
regular 0600 `/etc/ngfw/base-policy.env` without shell evaluation. Bootstrap
permanent interfaces stay in `punt_interfaces`; the separate typed
`dynamic_punt_interfaces` starts empty and is reconstructed by agent resync from
owned root-namespace LCP pairs. Combined permanent/dynamic limit is 64;
management and ambiguous namespace state are rejected. nftables must load the
base table before the agent starts. Existing foundation installations require
the new base layout before activating this follow-up; absence of its dynamic
set fails closed. These boot/upgrade instructions have not been exercised on a
fresh appliance; use the centralized deferred acceptance campaign.
