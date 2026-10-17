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

## D-249 (2026-10-17, amends D-248): counts order descendants only

Scope: `deploy/deploy-hosts.sh` installed-appliance upgrades; logged as D-249 in
`LOG.md`. A reachable commit count is strictly greater than the count of every
ancestor, but says nothing about unrelated histories: a stale branch with many
checkpoints sorts above a newer `main` build, so `dpkg --compare-versions ge`
alone accepted an older tree and then refused later `main` builds as downgrades
with no way forward.

The embedded 12-character source commit is the installed build's recorded
identity (package metadata, no separate host state). Before any transfer the
helper reads the single installed version from every package (mixed versions
are refused), resolves both commits in the operator's complete Git checkout
(one tag-less, prompt-less, 120 s-limited `git fetch origin` per run if either
is missing; a branch or tag named like the abbreviation never shadows the
commit) and requires `git merge-base --is-ancestor <installed> <new>`. When the
new version carries a commit count, ancestry implies the count ordering, so the
remote `dpkg` guard still applies unchanged; the install step additionally
requires, under its lock, that every package still carries the version the
verdict was taken on. Three refusals are distinguished: a rollback (the new
source is an ancestor of the installed one) is always refused because
migrations cannot be undone; divergent histories are refused with the
merge-base, the commit counts on each side and the review commands; an
unresolvable commit is refused naming the missing side. The operator may pass
`--allow-divergent` for exactly one reviewed host per run; only when that host's
lineage actually needed it does the remote accept a development-to-development
version that `dpkg` orders lower and pass `--allow-downgrades` to APT, recording
the override in the recovery directory. Stable-release downgrades and mixed
installed package versions are refused regardless of the flag.

Consequence of D-112: task branches are squashed into one new commit before
merge, so a build from any branch is never an ancestor of the next `main`
build. After installing a branch build on an appliance, the next `main` build
needs the reviewed override; `docs/install/deploy-hosts.md` gives the review
procedure. Install only `main` builds outside explicit review.

Options considered: a monotonic build number from CI (no CI release pipeline
exists yet), a merge-base-aware version string (dpkg cannot express it), or
operator-side ancestry verification with an explicit override (selected; the
host has no Git history). Reversal cost: low, script and docs only. Tasks
affected: deploy-hosts-lineage-20261017.
