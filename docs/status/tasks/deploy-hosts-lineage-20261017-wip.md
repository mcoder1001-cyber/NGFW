# WIP: deploy-hosts-lineage-20261017

Branch: `fix/deploy-hosts-lineage-routes-20261017` (cut from `origin/main` 99a774734).
Envelope: [deploy-hosts-lineage-20261017-envelope.md](deploy-hosts-lineage-20261017-envelope.md).
Checkpoint 1: local = remote `4550231934f28ea793ae7f488f7761ee10732520` (pushed 2026-10-17).

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

## Actual test results (checkpoint 1, this worktree, disposable seams only)

```text
bash -n deploy/deploy-hosts.sh deploy/debian/ngfw/prepare.sh      -> exit 0
shellcheck --external-sources deploy/deploy-hosts.sh               -> no findings (0.11.0)
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v
Ran 25 tests in 324.557s
OK
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_prepare.py -v
Ran 4 tests in 3.335s
OK
```

No live host, database, VPP, credentials or network were used; the fake `git`
never touches the real repository. Hosted quick CI and the packaging fixture
gate are unchanged and remain required.

## Remaining code / current failure / next command

None known at the time of the PR; reviewer findings go here.
