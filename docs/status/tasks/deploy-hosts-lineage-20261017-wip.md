# WIP: deploy-hosts-lineage-20261017

Branch: `fix/deploy-hosts-lineage-routes-20261017` (cut from `origin/main` 99a774734).
Envelope: [deploy-hosts-lineage-20261017-envelope.md](deploy-hosts-lineage-20261017-envelope.md).
Remote/local SHAs: see the PR; updated at each checkpoint below.

## Owned files

- `deploy/deploy-hosts.sh`
- `deploy/debian/ngfw/prepare.sh` (comment only)
- `deploy/debian/ngfw/tests/test_deploy_hosts.py`
- `docs/decisions/DEC-248-development-package-ordering.md`
- `docs/install/deploy-hosts.md`
- `docs/status/tasks/deploy-hosts-lineage-20261017-*.md`

## Completed code

1. Source lineage: the preflight prints the single installed version (mixed versions are
   refused); `check_lineage` resolves the installed and new source commits in the operator's
   repository (one `git fetch origin` retry) and requires `git merge-base --is-ancestor`.
   `--allow-divergent` (the only long option, parsed by `getopts '-:'`) accepts a divergent or
   unresolvable development lineage; the remote then accepts a lower development version and
   passes `--allow-downgrades` to APT. Stable-release downgrades stay refused.
2. Default-route comparison keeps only `default`, `via X`, `dev Y`, `metric Z` per route line,
   sorted; raw `ip` output is still written to the recovery directory.
3. After the manual restart the helper records `NRestarts` of agent/API, waits 5 s after the
   health check, rechecks every service and requires unchanged counters.
4. Fixtures: shared fake `git` with a private lineage (a ← c; b unrelated; d unknown), installed
   version table per failure mode, IPv6 RA route with changing `expires`, `route6`, `crashloop`,
   `mixed`, `--allow-divergent` paths, bad long options.

## Actual test results

Recorded at the checkpoint commit; see the PR description for the pasted output.

## Remaining code / current failure / next command

None known at the time of the PR; reviewer findings go here.
