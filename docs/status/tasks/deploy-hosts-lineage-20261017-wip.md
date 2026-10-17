# WIP: deploy-hosts-lineage-20261017

Branch: `fix/deploy-hosts-lineage-routes-20261017` (cut from `origin/main` 99a774734).
Envelope: [deploy-hosts-lineage-20261017-envelope.md](deploy-hosts-lineage-20261017-envelope.md).
PR: https://github.com/mcoder1001-cyber/NGFW/pull/240 (open, base `main` = `99a774734`).
Checkpoint 1: `455023193` + status `1649dd100` (pushed 2026-10-17; reviewed tree).
Checkpoint 2: `f1679314c` (all panel MAJORs fixed; R2/R7/R8 verify rounds APPROVE on it).
Checkpoint 3: `ce7c2ead5` + fixture guard `4f1cba348` (verify-round NITs; R1 verify round APPROVE).
Checkpoint 4: fixtures isolated from the runner's `GIT_SSH_COMMAND`/`GIT_TERMINAL_PROMPT`
(R1 verify MINOR), subtest coupling removed; product script unchanged since `4f1cba348`.
Combined review: [deploy-hosts-lineage-20261017-review.md](deploy-hosts-lineage-20261017-review.md)
(BLOCK on `1649dd100`; all MAJORs fixed in checkpoint 2; verify rounds: R1, R2, R7, R8 APPROVE).

## Owned files

- `deploy/deploy-hosts.sh`
- `deploy/debian/ngfw/prepare.sh` (comment only)
- `deploy/debian/ngfw/tests/test_deploy_hosts.py`
- `docs/decisions/DEC-248-development-package-ordering.md`
- `docs/install/deploy-hosts.md`
- `docs/status/tasks/deploy-hosts-lineage-20261017-*.md`
- `docs/decisions/LOG.md` (one appended line, D-249)

## Completed code

1. Source lineage (D-249): the preflight prints the single installed version (mixed versions
   are refused with their versions); `check_lineage` resolves the installed and new source
   commits in the operator's complete checkout (one tag-less, prompt-less, 120 s-limited
   `git fetch origin` per run; abbreviation prefix check) and requires
   `git merge-base --is-ancestor`. Rollbacks within one history, undecidable ancestry,
   unparseable `~dev` versions and stable-release downgrades are refused regardless of the flag.
   `--allow-divergent` (the only long option, `getopts '-:'`, exactly one host) accepts a divergent
   or unresolvable development lineage; the remote receives the override literal only for that
   host, rechecks under its lock that every package still carries the preflight version, and
   only then passes `--allow-downgrades` to APT (recorded in `override.txt`). Refusals remove
   the empty staging directory and print merge-base, per-side counts and review commands.
2. Default-route comparison keeps route type, `via` (also after `inet6`), `dev`, `metric`,
   `src`, `weight` and grouped multipath `nexthop` entries per route, sorted; raw `ip` output
   before (and, on mismatch, after) the upgrade is written to the recovery directory, both
   normalised forms are printed, and an `ip` failure aborts under errexit.
3. After the manual restart the helper records `NRestarts` of agent/API (with retries), waits
   10 s after the health check, reads the counters again before the service checks and prints
   before/after on mismatch; the exit trap saves `service-status.txt`.
4. Fixtures (20 → 28 tests): shared fake `git` with a private lineage (c < a < e; b branched
   from c; d unknown until fetched), installed version table per failure mode, `build-info.json`
   provenance, IPv6 RA route with changing `expires` plus a multipath route, gateway/device/
   metric/next-hop changes, crash-looping agent and API, mixed versions, rollback, preflight-to-
   lock race, hostile preflight output, unparseable `~dev`, unresolvable installed/new source,
   fetch-resolved source, undecidable ancestry, `--allow-divergent` paths, bad long options.

## Actual test results (head `4f1cba348`, this worktree, disposable seams only)

```text
bash -n deploy/deploy-hosts.sh deploy/debian/ngfw/prepare.sh      -> exit 0
shellcheck --external-sources deploy/deploy-hosts.sh               -> no findings (0.11.0)
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v
test_build_and_cache_reuse_are_bound_to_source_and_vpp ... ok
test_corrupt_package_fails_before_ssh ... ok
test_descendant_sources_install_including_legacy_hash_versions ... ok
test_development_versions_increase_for_descendants_and_preserve_release_guard ... ok
test_existing_deployment_lock_prevents_install ... ok
test_external_secret_key_requires_manual_backup_review ... ok
test_help_and_build_only_have_no_network_or_host_mutation ... ok
test_hostile_preflight_output_is_rejected_before_git_or_transfer ... ok
test_inactive_preflight_service_prevents_install ... ok
test_install_and_postinstall_failures_are_reported_and_stop_fleet ... ok
test_installed_version_changed_between_preflight_and_lock_is_refused ... ok
test_metadata_identity_version_and_architecture_fail_closed ... ok
test_missing_or_incomplete_or_traversing_manifest_fails_before_ssh ... ok
test_newer_main_build_over_divergent_branch_needs_explicit_override ... ok
test_noncanonical_database_fails_before_backup_or_install ... ok
test_old_shared_lock_symlink_cannot_truncate_target ... ok
test_override_never_accepts_release_downgrade_or_mixed_versions ... ok
test_preinstall_failures_do_not_install_or_continue_to_second_host ... ok
test_remote_lock_symlink_is_refused_without_truncation ... ok
test_repeated_upgrade_keeps_previous_backup ... ok
test_rollback_within_one_history_is_refused_even_with_override ... ok
test_shallow_build_refuses_incomplete_version_history ... ok
test_stale_branch_with_higher_commit_count_is_refused_before_transfer ... ok
test_stale_source_cache_and_preexisting_work_are_not_reused_or_deleted ... ok
test_success_uses_private_backup_and_verified_installs ... ok
test_symlink_and_extra_archive_fail_before_ssh ... ok
test_unknown_source_is_fetched_once_then_refused_without_override ... ok
test_unsafe_arguments_fail_before_ssh ... ok
----------------------------------------------------------------------
Ran 28 tests in 431.701s

OK
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_prepare.py -v
Ran 4 tests in 3.615s

OK
git diff 99a774734...HEAD -- .github tools/ci.sh                    -> empty
```

Checkpoint 4 (`e992710e9`, fixtures only; product script unchanged): the same discover command
`Ran 28 tests in 455.237s` / `OK`. Earlier full runs: checkpoint 1 (`1649dd100`) 25 tests OK in
324.557 s (reproduced by R1 in 333.642 s); checkpoint 2 (`f1679314c`) 28 tests OK in 433.522 s;
`4f1cba348` 28 tests OK in 431.701 s (R1: 435.873 s in a clean environment). No live host, database, VPP,
credentials or network were used; the fake `git` never touches the real repository. Hosted
quick CI and the packaging fixture gate are unchanged and remain required; their result on
PR #240 is recorded by the manager at merge time.

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
- Hosted packaging job duration with 28 deploy-hosts tests (15-minute budget) to be read from
  the PR run.
