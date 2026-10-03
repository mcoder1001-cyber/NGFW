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
before closing the window. Database migrations require compatibility review; a
package downgrade alone is not a database rollback. If a migration is incompatible,
restore the tested backup through the approved recovery process.

Package install/remove/purge/reinstall, migration upgrades and full boot verification
are NOT RUN in the current environment; record their results in the central deferred
acceptance campaign when the lab is available.
