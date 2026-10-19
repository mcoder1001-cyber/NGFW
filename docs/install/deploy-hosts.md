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

**Python:** the package checks use `hashlib.file_digest`, so the operator machine
and the appliance both need Python 3.11 or newer. The packages depend on an
unversioned `python3`; the Ubuntu 26.04 appliance image satisfies this, so the
check guards operator machines and foreign or older installations. The script
refuses an older interpreter with a clear message: on the operator machine
before it connects anywhere, on the appliance in the preflight before any
staging directory is made.

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
`git` prefix sorts above the previous hash-only development versions. Counts do
not order unrelated histories; the upgrade step verifies ancestry (below).
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
Upgrading one development build to another also needs this script to run from
a complete (non-shallow) Git checkout of the source repository that contains
both source commits; see source lineage below.
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
DNS names and addresses may contain only letters, digits, dots and hyphens. An
alias may also contain underscores, which DNS names never do: such a host is
accepted only when `ssh -G` (the effective client configuration, read without
connecting) shows that its `Host` block sets a `HostName` other than the alias
itself; otherwise it is refused before any connection, and an error from the
operator's ssh configuration is shown as is. `Host` patterns match the name as
typed and `ssh -G` echoes an unmatched name lowercased, which the check
recognises: a differently cased spelling of an alias is refused like any other
unconfigured name. `ssh -G` evaluates the operator's own client configuration,
including any `Match exec` it contains, exactly as the connection would. The first failed host stops the fleet. `-b` never
connects to the supplied hosts. Options must precede hosts. The long options are
`--allow-divergent` (exactly one host; see source lineage below) and
`--allow-no-default-route` (exactly one host; see default routes below).

The remote preflight and install scripts are fixed code sent over the SSH
session's standard input. The remote launcher moves that input to a private
descriptor, gives the shell and every remote child `/dev/null` as standard
input and lets bash read the script from its own copy of the pipe, so a child
that reads standard input (an APT or maintainer-script prompt, for example)
gets end-of-file instead of the rest of the script. Plain remote commands are
sent with an empty standard input (`/dev/null`). This relies on the SSH
session's standard input being a pipe, which OpenSSH provides for a command
without a terminal; remote shell errors are reported as `/dev/fd/3: line N`.

Each host gets a distinct root-private recovery directory at
`/root/ngfw-deploy/run.XXXXXXXX`. A lock in that root-owned, mode-0700 parent directory serializes this tool's
install phase; symbolic or incorrectly owned/mode lock files are refused.
A private local snapshot is copied there, and its manifest digest, archive
hashes, package metadata and installed package versions are rechecked remotely.
The target must have a completed firstboot marker, all four packages installed
at one version, agent/API/nginx/VPP services individually active and an nginx
configuration that passes `nginx -t`; a missing tool is named. Stopped
appliances, mixed installed versions and a site that already fails its test
are refused before anything is transferred.

**Source lineage (D-249):** development versions embed the source commit, and
their commit counts order only descendant commits. Before copying anything the
helper reads the installed version, resolves the installed and new source
commits in this repository and requires the new source to be a descendant of
the installed one, via `git merge-base --is-ancestor`. If either commit is
missing locally it fetches `origin` branches once per run (no tags, no
credential prompts, 120 second limit, `FETCH_HEAD` untouched) and retries. A
refusal names its cause, removes the empty staging directory it had created,
transfers nothing and stops the fleet:

- *rollback*: the new source is an ancestor of the installed one. Always
  refused, with or without the flag; a package downgrade cannot undo database
  migrations.
- *divergent*: neither commit descends from the other. The message prints the
  merge-base, how many commits exist only on each side, and the review
  commands (`git log --oneline <installed>..<new>` and
  `git diff --stat <installed> <new>`).
- *unresolvable*: the installed or the new source commit is not in this
  repository, for example a deleted branch. Fetch it explicitly
  (`git fetch origin pull/<N>/head:refs/remotes/origin/pr/<N>` keeps a
  squash-merged branch reachable and referenced) and rerun. Git being unable
  to decide the ancestry is refused as well, never read as divergence. A `~dev` version that carries no recognisable commit is treated the
  same way rather than falling back to count ordering.

Under D-112 every task branch is squashed into one new commit before merge, so
a build from any branch is never an ancestor of the next `main` build. Once an
appliance runs a branch build, the next `main` build is reported as divergent;
this is expected, not an error in the build. Review the two histories with the
printed commands, then rerun with `--allow-divergent` for that single host. The
flag accepts a divergent or unresolvable development lineage on that host
only; when the remote `dpkg` ordering is lower the install additionally passes
`--allow-downgrades` to APT, and `override.txt` in the recovery directory
records this. The flag never accepts a rollback, a stable-release downgrade
(including stable-release to development), a stable release on either side, or
mixed installed package versions, which the preflight prints and which must be
repaired by hand first. The install step rechecks, under its lock, that every
package still carries the version the verdict was taken on and refuses if a
concurrent run changed it. Install only `main` builds outside explicit review.

Before installation the script records the raw IPv4/IPv6 default routes and
installed versions, then simulates APT. Any proposed dependency/VPP change or removal is
rejected. The real APT call forbids removals, preserves existing configuration
files and cannot download: both calls hide every configured repository
(`Dir::Etc::SourceList` and `Dir::Etc::SourceParts` set to `/dev/null`), so APT
sees only the staged archives and the installed packages. apt 3 and newer refuse
`--no-download` for local archives ("Pathname to install is not absolute"),
which is why the repositories are hidden instead. It makes no VPP restart request. With packages built from
D-253 on (`dh_installsystemd --no-stop-on-upgrade`) the APT step leaves every
unit as it was: VPP, nftables, the sockets and the completed provisioning
oneshots keep running, and only the explicit agent/API restart below changes
service state. Packages built before D-253 stopped every unit they ship in their
preinst, which stopped `vpp.service` for about nine seconds through
`Requires=ngfw-firstboot.service` (observed on appliance 37 on 2026-10-13 and
2026-10-19) and left `apply-executor.socket`, `ngfw-ra-openfile.socket` and
`ngfw-ra-namespace-broker.socket` stopped, which this script does not restart.
The script also does not stop `apply-executor.service` (the running executor keeps
the previous `apply-executor.py` until an operator stops it while no start-up
apply runs) and does not re-render the base firewall policy (applies at the next
reboot; never restart nftables or firewall-bootstrap for it); see
[package upgrades](upgrade.md). Dependencies requiring
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

After APT, the script restarts agent/API, validates the nginx configuration
(`nginx -t`) and reloads `nginx.service`: `ngfw-web` ships the site
`/etc/ngfw/nginx.conf` (linked from `sites-enabled` by firstboot) and the static
web assets without a maintainer script, and a changed site file takes effect
only on reload. The reload is graceful. The explicit test, the same step
firstboot runs before enabling the site, writes its output to `nginx-test.txt`
in the recovery directory. When it fails the run stops: the running nginx still
serves the previous configuration, but the new site file is already on disk
and the unit's own start-time test would fail on the next restart or reboot,
and the agent/API health check has not run. Restore or fix
`/etc/ngfw/nginx.conf` (the previous copy is member `etc/ngfw/nginx.conf` of
`preupgrade-config.tar.gz` in the recovery directory:
`tar -C / -xzf <recovery>/preupgrade-config.tar.gz etc/ngfw/nginx.conf`), then
run `nginx -t && systemctl reload nginx.service`. Two limits: `systemctl reload`
only signals nginx, so a configuration that passes the test but is rejected at
runtime (a port held by another process, a path the sandboxed unit cannot open)
leaves nginx on its previous configuration without failing the run; check
nginx's error log when the web UI does not reflect the upgrade. And APT keeps a
locally modified site file (`--force-confold`), in which case the shipped one
lands as `/etc/ngfw/nginx.conf.dpkg-dist` to be merged by hand. Before the
nginx steps the script records the agent/API automatic restart counters
(`NRestarts`, read after the manual restart with retries); it then checks the
local loopback HTTP health endpoint through the SSH session, waits a ten-second
stability window, reads the counters again, then checks each
agent/API/nginx/VPP service individually and requires the counters to be
unchanged. A daemon that crashes between the restart and the end of that
window is reported with its before/after counters even if it is momentarily
active again; a crash later than the window is outside this check, and nginx
and VPP are only required to be active. On any failure the service status of
agent/API is saved to `service-status.txt` in the recovery directory. A host
left crash-looping may still pass the next preflight's single service check
while the unit is momentarily active; repair the unit by hand before
redeploying rather than relying on the tool to refuse it.

Default routes are compared on routing intent only: for each default route the
type, `via` gateway (also after `inet6`), `dev`, `metric`, `src` and grouped
multipath `nexthop` entries, sorted by route. `proto`, `expires`, `hoplimit`,
`pref`, `mtu`, `onlink` and `nhid` are ignored, because RA-learned IPv6 routes
print changing lifetimes. The raw route text before the upgrade stays in the
recovery directory; on a mismatch the normalised before/after forms are printed
and the raw after-state is saved as `default-route-v{4,6}-after.txt`. Installed
versions are rechecked, then a fresh SSH connection is opened. Only that final
connection writes `upgrade-complete`.
This check does not certify externally served TLS, login, dataplane forwarding,
agent reconciliation or application rollback; run those acceptance checks before
closing the maintenance window.

A host with neither an IPv4 nor an IPv6 default route is refused
("no management default route") unless `--allow-no-default-route` is given. A
management-only appliance on a directly connected (for example link-local)
network is a valid target, but the operator must declare it, because an empty
route table is also what a broken host looks like; like `--allow-divergent`,
the flag is accepted with exactly one host per run and prints a local warning.
The preflight refuses such a host before anything is transferred; the install
step rechecks the routes under its lock. With the flag the empty before-state is
accepted and recorded in `no-default-route.txt` in the recovery directory; the
before/after comparison still applies, so a default route that appears during
the upgrade is reported as a change. The upgrade itself needs no default route:
APT runs without any repository, so it cannot download.

If a step fails, stop and inspect the reported recovery directory, APT plan,
previous versions, backups and system logs through the console or surviving SSH
session. The tool reports failure and retains evidence. It does not automatically
reinstall old packages or restore the database: either could destroy state after
a migration. Follow the reviewed recovery process. Backups contain secrets; keep
them private and manage retention explicitly. A successful run leaves them intact.

Host-independent verification:

```sh
bash -n deploy/deploy-hosts.sh
shellcheck --external-sources deploy/deploy-hosts.sh
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v
```

Fixtures create real minimal Debian archives and run the actual wrapper and remote
shell through recording SSH/APT/service seams and redirected temporary host paths.
They test success, repeated backup preservation, failed preflight/transfers/
backups/install/health/SSH, downgrade/dependency rejection, invalid package sets,
cache provenance, development version ordering, shallow-history refusal, mixed
service states, source lineage (stale branch, divergent `main`, legacy hash
versions, unknown installed source, `--allow-divergent`), an expiring IPv6 RA
default route, a changed IPv4/IPv6 gateway, a crash-looping agent, mixed
installed versions, a rollback under the override, an installed version that
changes between preflight and the locked install, hostile preflight output,
multipath next-hop swaps, device/metric-only route changes, a crash-looping
API, the `build-info.json` provenance path and the private deployment lock,
remote children that drain their standard input (with a negative control for the
former `bash -s` launcher), nginx validation and reload failures, a host without
a default route with and without the flag (and the flag with two hosts),
interpreters older than 3.11 on either side, and underscore hosts in either case
that are or are not configured SSH aliases. A
private fake `git` supplies the source history; the real repository is never
read or fetched. They do not access a live host, a real database, VPP,
credentials or the network. The unchanged strict hosted packaging
fixture runner discovers this suite; the complete quick gate remains required.
