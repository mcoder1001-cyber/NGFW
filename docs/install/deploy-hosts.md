# Upgrade initialized appliances over SSH

`deploy/deploy-hosts.sh` builds the four NGFW management packages from a chosen
origin commit, or upgrades an explicitly named, already initialized appliance
using a locally reviewed package directory. Target dedicated appliances only;
never target the shared development/CI host while its VPP handover is pending.
An initialized-package check is not authorization to modify a shared host.
It is an operator maintenance tool,
not a release certification or a fresh-machine installer. Dependencies and the
pinned VPP runtime must already be installed. See [package upgrades](upgrade.md)
for release and recovery prerequisites and [bare metal](bare-metal.md) for initial
installation.

Build from an origin ref using independently verified VPP build output:

```sh
deploy/deploy-hosts.sh -b -r main -v /path/to/verified-vpp -w /var/tmp/ngfw-build-reviewed
```

The work directory must be new, or contain an intact cache from the exact fetched
source commit and VPP manifest. The script runs the canonical VPP verification
and Debian preparation/build steps. `prepare.sh` builds this checkout's API and
web assets and emits `0.1.0~dev+git<reachable-commit-count>.<12-character-source-commit>`
package versions. Full Git history is required; shallow checkouts are refused.
Within one source history, descendant commits have increasing counts, and the
`git` prefix sorts above the previous hash-only development versions.
The wrapper preserves the source SHA and VPP manifest digest in `build-info.json`.
The count is also recorded in the build metadata and checked against package
versions. Source refs predating this version format need their older reviewed
build workflow; their hash-only package versions cannot satisfy the new build
provenance contract. It never deletes an existing work directory to rebuild a mismatched cache; choose
a new `-w` path. Without `-w` it creates a private directory under `/var/tmp`.
A failed build retains its checkout and intermediates for diagnosis.

Validate a prepared directory without connecting to any host:

```sh
deploy/deploy-hosts.sh -b -p /path/to/reviewed-packages
```

The directory must contain exactly `ngfw-agent`, `ngfw-api`, `ngfw-meta` and
`ngfw-web` packages at one version, `SHA256SUMS`, and optionally `build-info.json`.
The agent/API must be `amd64`; meta/web must be `all`. The manifest must cover
every package and the metadata, with no duplicate or out-of-directory paths.
Package filenames must match their actual Debian metadata. Missing manifests,
links, extra archives, mixed versions and checksum failures are rejected.

**Trust:** hashes protect integrity, not publisher authenticity. `-p` means the
operator has independently reviewed and authenticated these local archives and
their checksum manifest; this script does not turn an untrusted directory into
a signed release. Build provenance is a cache consistency record, not a signed
attestation. Maintainer scripts execute as root. Use the signed release workflow
for production delivery and do not treat WIP builds as production releases.

With a maintenance window and recovery console ready:

```sh
deploy/deploy-hosts.sh -p /path/to/reviewed-packages appliance-a.example appliance-b.example
```

`-u USER` selects an SSH identity which must already have UID 0 on the target;
the script does not configure credentials or elevate privileges. SSH uses batch
mode and strict existing host-key verification. Hosts must be DNS names, IPv4
addresses or SSH configuration aliases. Use an alias for IPv6 and custom ports.
The first failed host stops the fleet. `-b` never connects to the supplied hosts.

Each host gets a distinct root-private recovery directory at
`/root/ngfw-deploy/run.XXXXXXXX`. A lock in that root-owned, mode-0700 parent directory serializes this tool's
install phase; symbolic or incorrectly owned/mode lock files are refused.
A private local snapshot is copied there, and its manifest digest, archive
hashes, package metadata and installed package versions are rechecked remotely.
The target must have a completed firstboot marker, all four packages installed
and agent/API/nginx/VPP services individually active. Stopped appliances are
refused before installation. Downgrades, including stable-release to development
build downgrades, are refused. New development builds use the source history
count for ordering; unrelated source histories still need explicit review.
A forced package downgrade cannot undo database migrations.

Before installation the script records IPv4/IPv6 default routes and installed
versions, then simulates APT. Any proposed dependency/VPP change or removal is
rejected. The real APT call forbids downloads and removals and preserves existing
configuration files. It makes no VPP restart request. Dependencies requiring
changes must be reviewed separately through the appropriate package workflow.

Backups are mandatory: the script archives `/etc/ngfw`, `/etc/vpp/startup.conf`
and `/var/lib/ngfw`, and makes and inspects a PostgreSQL custom-format dump.
Tar/read errors, failed or empty dumps, and unreadable dump archives stop before
installation. The database must match the canonical firstboot peer-authenticated
`ngfw` configuration and the master key must use its canonical non-symlink path;
customized database or master-key settings require manual backup review.
`api.env` is checked as data and never executed. Existing authentication and keys
are preserved; no initial users or new credentials are created.

Keep configuration writers quiescent throughout the maintenance window. The
filesystem archive and database dump are sequential backups, not an atomic
cross-store snapshot. Backup restore and database migration compatibility still
require appliance acceptance; fixtures do not prove those properties.

After APT, the script restarts agent/API, checks the local loopback HTTP health
endpoint through the SSH session, checks each agent/API/nginx/VPP service individually,
checks installed versions and unchanged IPv4/IPv6 default routes, then opens a
fresh SSH connection. Only that final connection writes `upgrade-complete`.
This check does not certify externally served TLS, login, dataplane forwarding,
agent reconciliation or application rollback; run those acceptance checks before
closing the maintenance window.

If a step fails, stop and inspect the reported recovery directory, APT plan,
previous versions, backups and system logs through the console or surviving SSH
session. The tool reports failure and retains evidence. It does not automatically
reinstall old packages or restore the database: either could destroy state after
a migration. Follow the reviewed recovery process. Backups contain secrets; keep
them private and manage retention explicitly. A successful run leaves them intact.

Host-independent verification:

```sh
bash -n deploy/deploy-hosts.sh
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v
```

Fixtures create real minimal Debian archives and run the actual wrapper and remote
shell through recording SSH/APT/service seams and redirected temporary host paths.
They test success, repeated backup preservation, failed preflight/transfers/
backups/install/health/SSH, downgrade/dependency rejection, invalid package sets,
cache provenance, development version ordering, shallow-history refusal, mixed
service states, and the private deployment lock. They do not access a live host, a real
database, VPP, credentials or the network. The unchanged strict hosted packaging
fixture runner discovers this suite; the complete quick gate remains required.
