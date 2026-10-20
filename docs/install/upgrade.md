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
Two things an upgrade no longer does since D-253, because the stop that did them
was the one restarting VPP: (a) `apply-executor.service`, the socket-activated
root daemon behind the dataplane start-up apply, keeps running the previous
`apply-executor.py`. When the upgrade changed that file, stop the service once no
start-up apply is running: `systemctl stop apply-executor.service` (the socket
stays active and the API's next request starts the new code; a running apply
would be killed with it, `KillMode=control-group`). `deploy-hosts.sh` does not do
this. (b) The base management firewall policy (`/etc/nftables.d/ngfw-base.nft`,
rendered by `render-base-policy.py` through `ngfw-firewall-bootstrap.service`) is
not re-rendered: a changed renderer applies at the next reboot. Do not restart
`nftables.service` or `ngfw-firewall-bootstrap.service` to force it: `ngfw-firstboot.service`
requires `nftables.service`, so that restarts VPP, the agent and the API.
Packages built before D-253 stopped every unit they ship in their preinst, which
stopped VPP for about nine seconds through `Requires=ngfw-firstboot.service` (on
A/B appliances also through `ngfw-upgrade-prepare.service`, required by firstboot)
and left the three sockets stopped until a reboot (`deploy-hosts.sh` restarts only
agent and API). That stop was in the incoming package's preinst, so the first
upgrade onto D-253 packages is already free of it where the installed prerm stops
only on removal; check before the window with
`grep -n stop /var/lib/dpkg/info/ngfw-*.prerm` (every stop line must carry
`[ "$1" = remove ]`; packages built with debhelper 13.14 carry none). A rollback to a
pre-D-253 package runs that package's preinst with `upgrade` and stops the units
once more. On an appliance upgraded with pre-D-253 packages, check
`systemctl is-active apply-executor.socket ngfw-ra-openfile.socket
ngfw-ra-namespace-broker.socket` and start them (or reboot) if they are inactive.
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
