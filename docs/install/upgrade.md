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
Firstboot completion remains durable; upgrades never reseed users or regenerate
valid keys/certificates. A pending bootstrap cleanup may still run after interruption.

Restart/maintenance-window coordination belongs to the manager/operator. Check
service health, API login, configuration revision, agent reconciliation and rollback
before closing the window. The first API start after an upgrade to a release with
boot-time NIC registration (D-249) adds one `system` revision ("register missing
dataplane NICs from the host inventory", events `COMMIT_APPLIED` and
`system.seed-defaults`) when the host has data NICs the saved configuration does not
carry; an unchanged inventory adds nothing. It is deferred while a candidate has edits
or a commit awaits confirmation, and a rejected commit is recorded once as
`system.seed-defaults-failed` instead of being retried. The pre-upgrade backup imports
only if the box's physical rows are kept (see the user guide on restoring a backup), and a
rollback to a pre-upgrade revision drops the registered rows until the next API start. A
site opt-out (`NGFW_SEED_DEFAULT_NICS=0` in `/etc/ngfw/api.env`) is accepted only after
firstboot has completed and must be in place before that first API start. Database migrations require compatibility review; a
package downgrade alone is not a database rollback. If a migration is incompatible,
restore the tested backup through the approved recovery process.

Package install/remove/purge/reinstall, migration upgrades and full boot verification
are NOT RUN in the current environment; record their results in the central deferred
acceptance campaign when the lab is available.
