# D-248: chronological development package versions

Date: 2026-10-14. Scope: Debian preparation and the installed-appliance upgrade helper.

Use `0.1.0~dev+git<reachable-commit-count>.<sha12>` from complete Git history.
Refuse shallow builds and keep the existing package downgrade guard. Cache
provenance binds the source SHA, reachable count and verified VPP manifest.

Options considered: retain hash-only versions (hash ordering is not chronological),
use the build timestamp (rebuild-dependent), or use reachable count plus SHA
(selected). Descendant commits have increasing counts, and `git` sorts above the
previous hexadecimal-only development versions. Stable-release downgrades remain
refused; unrelated source histories require operator review.

Reversal cost is low for the build-format and cache metadata. A package downgrade
cannot undo database migrations. This does not certify a release or authorize
an appliance operation. The decision is recorded separately here so this source
change does not republish historical appliance details in the aggregate log.

## Amendment 2026-10-17: counts order descendants only

Scope: `deploy/deploy-hosts.sh` installed-appliance upgrades. A reachable commit
count is strictly greater than the count of every ancestor, but says nothing
about unrelated histories: a stale branch with many checkpoints sorts above a
newer `main` build, so `dpkg --compare-versions ge` alone accepted an older tree
and then refused later `main` builds as downgrades with no way forward.

The embedded 12-character source commit is the installed build's recorded
identity (package metadata, no separate host state). Before transfer the helper
reads the installed version from every package, resolves both commits in the
operator's repository (fetching `origin` once if needed) and requires
`git merge-base --is-ancestor <installed> <new>`. Ancestry implies the count
ordering, so the remote `dpkg` guard still applies unchanged. A non-descendant
or unresolvable source is refused unless the operator passes
`--allow-divergent` after review; only then does the remote accept a
development-to-development version that `dpkg` orders lower and pass
`--allow-downgrades` to APT. Stable-release downgrades and mixed installed
package versions are refused regardless of the flag. A package downgrade still
cannot undo database migrations.

Options considered: a monotonic build number from CI (no CI release pipeline
exists yet), a merge-base-aware version string (dpkg cannot express it), or
operator-side ancestry verification (selected; the host has no Git history).
