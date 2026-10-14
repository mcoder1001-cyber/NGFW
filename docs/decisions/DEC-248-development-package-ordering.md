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
