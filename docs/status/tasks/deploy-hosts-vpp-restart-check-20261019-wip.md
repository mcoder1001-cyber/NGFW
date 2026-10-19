# WIP: deploy-hosts-vpp-restart-check-20261019 — detect a VPP restart during the upgrade

Branch: `claude/gifted-cartwright-6b6380` (cut from `origin/main` `cb148aafb`), worktree
`/root/NGFW/.claude/worktrees/gifted-cartwright-6b6380`.
Envelope: [deploy-hosts-vpp-restart-check-20261019-envelope.md](deploy-hosts-vpp-restart-check-20261019-envelope.md).
Origin: follow-up 3 of `pkg-no-stop-on-upgrade-20261019-wip.md` (PR #246, D-253); receipt
`deploy-host-37-20261019-wip.md` (PR #244).
PR: https://github.com/mcoder1001-cyber/NGFW/pull/247 (open against `main`; hosted gates and independent review pending; not to be merged by this task).

## Checkpoints

Each checkpoint is pushed the moment it is committed (local SHA = remote SHA).

| checkpoint | SHA | content |
|---|---|---|
| 0 | `72c6a7e21` | envelope and this file |
| 1 | `8340771f6` | remote install script unit-state records and comparison, fixtures, docs, D-254, this file |
| 2 | `57ba7dea7` | PR link, runs B and C verbatim/summary; product tree unchanged |
| 3 | (this commit) | hosted receipts for the product tree; product tree unchanged |

## Owned files

- `deploy/deploy-hosts.sh`
- `deploy/debian/ngfw/tests/test_deploy_hosts.py`
- `docs/install/deploy-hosts.md`
- `docs/status/DEFERRED-ACCEPTANCE.md` (one paragraph in the operator upgrade tool section)
- `docs/decisions/LOG.md` (one appended line, D-254)
- `docs/status/tasks/deploy-hosts-vpp-restart-check-20261019-*.md`

## Decision (D-254, `docs/decisions/LOG.md`)

Every watched unit (`vpp.service`, `nftables.service`, `apply-executor.socket`,
`ngfw-ra-openfile.socket`, `ngfw-ra-namespace-broker.socket`) must be unchanged between the record
taken immediately before `apt-get install` and the one taken after the stability window and the
agent/API counter comparison; any difference fails the run. A socket that is already inactive
before the upgrade is a recorded warning (both files, stderr before and after, `systemctl start
<socket>` named), never a preflight refusal or a failure: the upgrade did not cause it, the
comparison attributes a stop correctly, and the script repairs nothing. Options weighed in the LOG
line: preflight refusal (would block appliances 37/211, whose sockets pre-D-253 upgrades probably
left stopped, for no safety gain), warning-only for sockets (would certify an unreachable apply
executor), the old active-only VPP check (blind to the restart), VPP alone (same preinst, same
evidence).

## Completed code (checkpoint 1)

- `deploy/deploy-hosts.sh`, remote install script only (59 added lines; no option, argument,
  launcher or preflight change; the six validated install arguments are unchanged; no new
  `# shellcheck disable`):
  - `WATCHED_UNITS` and `unit_states`: one `systemctl show -p ActiveEnterTimestampMonotonic,NRestarts,ActiveState,LoadState <unit>`
    per unit, keyed output read by name (the `--value` form prints the properties in systemd's own
    order, verified on systemd 259: `ActiveState`, then the timestamp, then `NRestarts`; a socket
    unit prints no `NRestarts` at all, recorded as `-`); every value validated
    (`[a-z-]+`, `[0-9]+`, `[0-9]+|-`), a read failure or unexpected output fails the run.
  - `warn_stopped_sockets`: a socket line without `ActiveState=active` prints
    `WARNING: <socket> is not active before|after the upgrade (<line>); this script does not start it; start it by hand: systemctl start <socket>`.
  - Before: after the staged-bytes recheck and immediately before `echo 'installing packages'`:
    `unit-state-before.txt` (mode 0600 via `umask 077`) and the before-warnings.
  - After: right after the agent/API `NRestarts` comparison (so after the window and the
    `is-active` loop) and before the route comparison: `unit-state-after.txt`; on any difference
    the message `a unit this script does not restart changed during the upgrade; the packages must leave it as it was (D-253); records: <BACKUP>/unit-state-before.txt and unit-state-after.txt`,
    then `before: <line>` / `after:  <line>` for each differing unit, plus a hint for
    `vpp.service` (data plane interrupted; `journalctl -u vpp.service -u ngfw-firstboot.service`),
    `nftables.service` (ruleset reloaded) and a socket that is no longer active
    (`systemctl start <socket>`); `exit 1` (the EXIT trap writes `service-status.txt`, the local
    side dies with `inspect <backup> on the host`). On equality the after-warnings are printed.
- `deploy/debian/ngfw/tests/test_deploy_hosts.py`: the fake `systemctl show` keeps the exact
  `-p NRestarts --value <unit>` form for the agent/API counters and adds the keyed form (asserting
  its exact argument list), printing in systemd's order with no `NRestarts` for sockets; modes
  `vpp-restart` (timestamp 100 → 9100 once installed), `vpp-crash` (`NRestarts` 0 → 1 only),
  `socket-stopped` (`apply-executor.socket` inactive once installed), `socket-preinactive`
  (inactive throughout). New test `test_watched_units_must_be_unchanged_by_the_upgrade`: unchanged
  record (both files equal `UNIT_STATE`, mode 0600, two keyed reads per unit, ordered after the
  simulation/backups and before the real `apt-get`, and after the last counter read and
  `is-active` loop and before the route after-state), the two VPP failures (message, regex on the
  redirected record paths, exact before/after lines, hint, `inspect /root/ngfw-deploy/run.`, fleet
  stopped, `service-status.txt`, no `upgrade-complete`), the stopped socket (exact lines and start
  command, no VPP text) and the pre-existing stopped socket (two warnings, equal records with
  `ActiveState=inactive`, `upgrade-complete`).
- Docs: `docs/install/deploy-hosts.md` (APT-step paragraph, health-check paragraph, fixture list),
  `docs/status/DEFERRED-ACCEPTANCE.md` (one paragraph in the operator upgrade tool section),
  `docs/decisions/LOG.md` (D-254).

Extra local check (not a gate): the remote install script extracted from its quoted heredoc
(`sed -n "/^read -r -d '' REMOTE_INSTALL/,/^REMOTE$/p"` with a `verify_packages` stub) passes
`bash -n` and `shellcheck -s bash` clean; neither gate parses the heredoc content.

## Remaining code

None; the full gates and the PR remain (below).

## Merge note

PR #244 (`apt_args`: repositories hidden instead of `--no-download`) and PR #246 (D-253 docs)
touch the same `docs/install/deploy-hosts.md` paragraph and the same `DEFERRED-ACCEPTANCE.md`
section; the script hunks do not overlap (#244 changes lines ~411-413, this branch inserts before
`echo 'installing packages'` and after the counter comparison). Whichever merges later needs the
two prose paragraphs re-merged by hand; nothing in the code conflicts.

## Actual test results

- `bash -n deploy/deploy-hosts.sh`: OK. `shellcheck --external-sources deploy/deploy-hosts.sh`:
  clean (0.11.0).
- Targeted run A (`python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v -k watched_units -k test_success_uses -k nginx_is_validated -k postinstall_failures`, tree of checkpoint 1): `Ran 4 tests in 307.644s` / `OK` (the new test and the three existing tests that read the fake `systemctl show`).
- Run B, full `python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v` (tree of checkpoint 1, `8340771f6`): `Ran 34 tests in 746.437s` / `OK` (34 = 33 on `origin/main` + 1 new); verbatim below.
- Run C, whole-suite `python3 -m unittest discover -s deploy/debian/ngfw/tests -v` (same tree, run after B, never concurrently): `Ran 126 tests in 800.287s` / `OK`, no skips (126 = 125 on `origin/main` + 1 new; the D-253 maintainer-script tests live on PR #246 and are not on this branch). The wrapper that ran B and C exited 1 only because its final `tail -4` is rejected by this host's `tail`; both logs end in `OK` / `rc=0`.

Run B, verbatim:

```text
test_build_and_cache_reuse_are_bound_to_source_and_vpp (test_deploy_hosts.DeployHosts.test_build_and_cache_reuse_are_bound_to_source_and_vpp) ... ok
test_corrupt_package_fails_before_ssh (test_deploy_hosts.DeployHosts.test_corrupt_package_fails_before_ssh) ... ok
test_descendant_sources_install_including_legacy_hash_versions (test_deploy_hosts.DeployHosts.test_descendant_sources_install_including_legacy_hash_versions) ... ok
test_development_versions_increase_for_descendants_and_preserve_release_guard (test_deploy_hosts.DeployHosts.test_development_versions_increase_for_descendants_and_preserve_release_guard) ... ok
test_existing_deployment_lock_prevents_install (test_deploy_hosts.DeployHosts.test_existing_deployment_lock_prevents_install) ... ok
test_external_secret_key_requires_manual_backup_review (test_deploy_hosts.DeployHosts.test_external_secret_key_requires_manual_backup_review) ... ok
test_help_and_build_only_have_no_network_or_host_mutation (test_deploy_hosts.DeployHosts.test_help_and_build_only_have_no_network_or_host_mutation) ... ok
test_hostile_preflight_output_is_rejected_before_git_or_transfer (test_deploy_hosts.DeployHosts.test_hostile_preflight_output_is_rejected_before_git_or_transfer) ... ok
test_inactive_preflight_service_prevents_install (test_deploy_hosts.DeployHosts.test_inactive_preflight_service_prevents_install) ... ok
test_install_and_postinstall_failures_are_reported_and_stop_fleet (test_deploy_hosts.DeployHosts.test_install_and_postinstall_failures_are_reported_and_stop_fleet) ... ok
test_installed_version_changed_between_preflight_and_lock_is_refused (test_deploy_hosts.DeployHosts.test_installed_version_changed_between_preflight_and_lock_is_refused) ... ok
test_metadata_identity_version_and_architecture_fail_closed (test_deploy_hosts.DeployHosts.test_metadata_identity_version_and_architecture_fail_closed) ... ok
test_missing_or_incomplete_or_traversing_manifest_fails_before_ssh (test_deploy_hosts.DeployHosts.test_missing_or_incomplete_or_traversing_manifest_fails_before_ssh) ... ok
test_newer_main_build_over_divergent_branch_needs_explicit_override (test_deploy_hosts.DeployHosts.test_newer_main_build_over_divergent_branch_needs_explicit_override) ... ok
test_nginx_is_validated_and_reloaded_after_install (test_deploy_hosts.DeployHosts.test_nginx_is_validated_and_reloaded_after_install) ... ok
test_no_default_route_needs_explicit_flag_and_is_recorded (test_deploy_hosts.DeployHosts.test_no_default_route_needs_explicit_flag_and_is_recorded) ... ok
test_noncanonical_database_fails_before_backup_or_install (test_deploy_hosts.DeployHosts.test_noncanonical_database_fails_before_backup_or_install) ... ok
test_old_python_is_refused_on_this_machine_and_on_the_target (test_deploy_hosts.DeployHosts.test_old_python_is_refused_on_this_machine_and_on_the_target) ... ok
test_old_shared_lock_symlink_cannot_truncate_target (test_deploy_hosts.DeployHosts.test_old_shared_lock_symlink_cannot_truncate_target) ... ok
test_override_never_accepts_release_downgrade_or_mixed_versions (test_deploy_hosts.DeployHosts.test_override_never_accepts_release_downgrade_or_mixed_versions) ... ok
test_preinstall_failures_do_not_install_or_continue_to_second_host (test_deploy_hosts.DeployHosts.test_preinstall_failures_do_not_install_or_continue_to_second_host) ... ok
test_remote_children_cannot_consume_the_script (test_deploy_hosts.DeployHosts.test_remote_children_cannot_consume_the_script) ... ok
test_remote_lock_symlink_is_refused_without_truncation (test_deploy_hosts.DeployHosts.test_remote_lock_symlink_is_refused_without_truncation) ... ok
test_repeated_upgrade_keeps_previous_backup (test_deploy_hosts.DeployHosts.test_repeated_upgrade_keeps_previous_backup) ... ok
test_rollback_within_one_history_is_refused_even_with_override (test_deploy_hosts.DeployHosts.test_rollback_within_one_history_is_refused_even_with_override) ... ok
test_shallow_build_refuses_incomplete_version_history (test_deploy_hosts.DeployHosts.test_shallow_build_refuses_incomplete_version_history) ... ok
test_stale_branch_with_higher_commit_count_is_refused_before_transfer (test_deploy_hosts.DeployHosts.test_stale_branch_with_higher_commit_count_is_refused_before_transfer) ... ok
test_stale_source_cache_and_preexisting_work_are_not_reused_or_deleted (test_deploy_hosts.DeployHosts.test_stale_source_cache_and_preexisting_work_are_not_reused_or_deleted) ... ok
test_success_uses_private_backup_and_verified_installs (test_deploy_hosts.DeployHosts.test_success_uses_private_backup_and_verified_installs) ... ok
test_symlink_and_extra_archive_fail_before_ssh (test_deploy_hosts.DeployHosts.test_symlink_and_extra_archive_fail_before_ssh) ... ok
test_underscore_hosts_need_a_configured_ssh_alias (test_deploy_hosts.DeployHosts.test_underscore_hosts_need_a_configured_ssh_alias) ... ok
test_unknown_source_is_fetched_once_then_refused_without_override (test_deploy_hosts.DeployHosts.test_unknown_source_is_fetched_once_then_refused_without_override) ... ok
test_unsafe_arguments_fail_before_ssh (test_deploy_hosts.DeployHosts.test_unsafe_arguments_fail_before_ssh) ... ok
test_watched_units_must_be_unchanged_by_the_upgrade (test_deploy_hosts.DeployHosts.test_watched_units_must_be_unchanged_by_the_upgrade) ... ok
----------------------------------------------------------------------
Ran 34 tests in 746.437s
OK
```

Hosted, on the product tree `8340771f6` (PR #247 head at opening): mandatory quick gate [39233622449](https://github.com/mcoder1001-cyber/NGFW/actions/runs/39233622449) success; packaging fixture gate [39233622470](https://github.com/mcoder1001-cyber/NGFW/actions/runs/39233622470) and [39233511351](https://github.com/mcoder1001-cyber/NGFW/actions/runs/39233511351) success. The same two workflows re-run on the docs-only `57ba7dea7` (in progress when this was written).

## Current failure

None.

## Exact next command

None for the developer: all four gates are green locally and PR #247 is open. For the manager: read `gh pr checks 247` (mandatory quick gate + packaging fixtures), request the panel review (R1, R2, R7, R8 per `prompts/REVIEW-PROMPT.md`), and when merging after #244/#246 re-merge the two prose paragraphs named under *Merge note*.
