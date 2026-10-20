# Package upgrades

P10 is not yet a tested appliance release. A/B image updates are outside this task.
Use the signed package repository only after release-builder and deferred appliance
acceptance. Never point a running device at WIP/checkpoint packages.

Before upgrade, export a verified configuration/backup, record current package
versions and confirm console access/recovery. Preserve `/var/lib/ngfw`, PostgreSQL
state, `/etc/ngfw/api.env`, the secret master key and operator TLS certificates.
Losing the secret key makes existing encrypted secrets unreadable.

Upgrade the matching-version `ngfw-meta` package and its management packages
through APT. The meta dependency pins the exact verified product VPP version.
Maintainer scripts preserve configuration/data and do not start/restart VPP.
Since D-253 they also neither stop nor start any `ngfw` unit on upgrade
(`dh_installsystemd --no-start --no-stop-on-upgrade`; the packages ship no preinst,
and prerm stops the shipped units only on `apt-get remove`): the running agent and
API keep executing the previous version until the operator restarts them, the
sockets `apply-executor.socket`, `ngfw-ra-openfile.socket` and
`ngfw-ra-namespace-broker.socket` keep listening, and the completed provisioning
oneshots `ngfw-firstboot.service` and `ngfw-firewall-bootstrap.service` keep their
`active (exited)` state, so `vpp.service` and `nftables.service`, which require
them, are not touched and `firstboot.sh` does not run again. Restart agent and API
after the upgrade (`systemctl restart ngfw-agent.service ngfw-api.service`, which
`deploy/deploy-hosts.sh` does); until then the previous version keeps serving.
Packages built before D-253 stopped every unit they ship in their preinst, which
stopped VPP for about nine seconds through `Requires=ngfw-firstboot.service` and
left the three sockets stopped until a reboot (`deploy-hosts.sh` restarts only
agent and API). That stop was in the incoming package's preinst, so the first
upgrade onto D-253 packages is already free of it; on an appliance upgraded
earlier, check `systemctl is-active apply-executor.socket ngfw-ra-openfile.socket
ngfw-ra-namespace-broker.socket` and start them (or reboot) if they are inactive.
Since D-255 `ngfw-firstboot.service` pulls `postgresql.service`,
`valkey-server.service` and `nftables.service` in with `Wants=` instead of
`Requires=` (the A/B snapshot unit `ngfw-upgrade-prepare.service` likewise wants
PostgreSQL), so an explicit restart of one of them, whether by an operator
(`systemctl restart postgresql.service`) or by the daemon's own package upgrade
(the Ubuntu `valkey-server` postinst restarts and the `nftables` postinst
try-restarts their units), no longer restarts firstboot and, through
`Requires=ngfw-firstboot.service`, VPP, the agent and the API. A restart of
`nftables.service` still restarts the agent, which requires the base firewall, and
with it the API, which requires the agent; apply a changed base policy with
`systemctl reload nftables.service`.
The new unit text is read at the next `systemctl daemon-reload`
(`deploy/deploy-hosts.sh` runs one after APT) or reboot; until then the running
manager keeps the previous `Requires=`. On a fresh appliance the first boot still
needs all three active and a PostgreSQL cluster that accepts connections
(`pg_isready`, because Debian's `postgresql.service` is only an `ExecStart=/bin/true`
wrapper that stays active when the `postgresql@<version>-main.service` cluster
fails): `firstboot.sh` refuses to provision otherwise, keeps the bootstrap
credentials and VPP, agent and API stay blocked until the retry succeeds. On a
provisioned appliance VPP and the agent are still ordered after PostgreSQL, Valkey
and nftables through firstboot's `After=`, but are no longer withheld when
PostgreSQL or Valkey fail to start (the API fails and retries on its own until the
database is up), and VPP is no longer withheld when nftables fails (firstboot's
`Requires=nftables.service` did that before; the agent still is, through its own
`Requires=`, so VPP then runs without configuration).
`ngfw-web` has no maintainer script: after upgrading it, test the configuration
(`nginx -t`) and reload `nginx.service`, otherwise a changed site file
(`/etc/ngfw/nginx.conf`) stays unapplied until the next restart;
`deploy/deploy-hosts.sh` does this after APT.
Firstboot completion remains durable; upgrades never reseed users or regenerate
valid keys/certificates. A pending bootstrap cleanup may still run after interruption.

Restart/maintenance-window coordination belongs to the manager/operator. Check
service health, API login, configuration revision, agent reconciliation and rollback
before closing the window. The first API start after an upgrade to a release with
boot-time NIC registration (D-252) adds one `system` revision ("register missing
dataplane NICs from the host inventory (F-default-vpp-nics)", events `COMMIT_APPLIED` (or
`COMMIT_NOT_ENFORCED`) and `system.seed-defaults`) when the host has data NICs the saved
configuration does not carry; an unchanged inventory adds nothing. It is deferred while a
candidate has edits or a commit awaits confirmation. A failed commit is recorded once per
reason as `system.seed-defaults-failed`; a validator rejection is not retried before the
next API start, other failures are retried with increasing delays. Before the next
start-up configuration apply, review the Interfaces screen and release to the host every
newly registered NIC the host still needs (storage, HA heartbeat): registration marks them
engine-owned. The pre-upgrade backup imports only if the box's physical rows are kept (see
[restoring a backup](../user/interfaces/default-dataplane-nics.md#restoring-a-backup-replacement-hardware-removed-nics)),
and a rollback to a pre-upgrade revision drops the registered rows until the next API
start. A site opt-out (`NGFW_SEED_DEFAULT_NICS=0` in `/etc/ngfw/api.env`) is accepted only
after firstboot has completed and must be in place before that first API start. Database migrations require compatibility review; a
package downgrade alone is not a database rollback. If a migration is incompatible,
restore the tested backup through the approved recovery process.

Package install/remove/purge/reinstall, migration upgrades and full boot verification
are NOT RUN in the current environment; record their results in the central deferred
acceptance campaign when the lab is available.
