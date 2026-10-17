# WIP: deploy-hosts-lineage-20261017

Branch: `fix/deploy-hosts-lineage-routes-20261017` (cut from `origin/main` 99a774734).
Envelope: [deploy-hosts-lineage-20261017-envelope.md](deploy-hosts-lineage-20261017-envelope.md).
PR: https://github.com/mcoder1001-cyber/NGFW/pull/240 (open, base `main` = `99a774734`).
Checkpoint 1: `455023193` + status `1649dd100` (pushed 2026-10-17; reviewed tree).
Checkpoint 2: review fixes; SHA recorded in the PR after push (see "Actual test results").
Combined review: [deploy-hosts-lineage-20261017-review.md](deploy-hosts-lineage-20261017-review.md)
(BLOCK on `1649dd100`; all MAJORs fixed in checkpoint 2; verify round requested from R1/R2).

## Owned files

- `deploy/deploy-hosts.sh`
- `deploy/debian/ngfw/prepare.sh` (comment only)
- `deploy/debian/ngfw/tests/test_deploy_hosts.py`
- `docs/decisions/DEC-248-development-package-ordering.md`
- `docs/install/deploy-hosts.md`
- `docs/status/tasks/deploy-hosts-lineage-20261017-*.md`
- `docs/decisions/LOG.md` (one appended line, D-249)

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

- Remaining code: none known after checkpoint 2. Open: R1/R2 verify round; hosted CI result on
  the PR (quick gate + packaging fixture gate), to be recorded here.
- Current failure: none locally.
- Exact next command (verify round, in the worktree):
  `python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v`

## Decisions

- D-249 (amends D-248): operator-side ancestry verification with a one-host `--allow-divergent`
  override; rollbacks, stable-release downgrades and mixed versions always refused.
- The installed build's identity is the source commit embedded in the package version; no new
  host state file. The override and the service status are recorded in the recovery directory.
- 10 s stability window after API health (RestartSec=3); no agent-socket anchor (path is
  overridable and may persist across restarts).

## Out of scope (from the originating review, 2-الف.4, unchanged here)

- remote scripts are fed to `bash -s` on stdin (a child reading stdin would consume the script;
  none found);
- nginx is not reloaded after the upgrade;
- a host without any default route is refused;
- Python ≥ 3.11 (`hashlib.file_digest`) is not documented as a target requirement;
- the host pattern rejects underscores.

## Open questions

- Manager: confirm the D-249 number and add the `plan/tasks.yaml` row.
- Hosted packaging job duration with 33 fixtures (15-minute budget) to be read from the PR run.
