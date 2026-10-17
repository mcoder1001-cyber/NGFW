# WIP: deploy-hosts-minors-20261017

Branch: `fix/deploy-hosts-minors-20261017`, cut from `origin/fix/deploy-hosts-lineage-routes-20261017`
(PR #240 head `d50d4fd2c`) while PR #240 was still open; after #240 was squash-merged as `3f099b530`
(2026-10-17 11:59 UTC, tree identical to `d50d4fd2c`) the branch was rebased onto `origin/main`
(`git rebase --onto origin/main d50d4fd2c`; the tree hash did not change) and the stacked history
was kept at `refs/heads/archive/deploy-hosts-minors-20261017-stacked` (= `a01b58d67`).
Envelope: [deploy-hosts-minors-20261017-envelope.md](deploy-hosts-minors-20261017-envelope.md).
Originating review: `docs/status/2026-10-17-unmerged-audit-product-review.md` §2-الف.4 (`b0cab4260`).
PR: https://github.com/mcoder1001-cyber/NGFW/pull/242 (open; base retargeted to `main` by the manager after #240 merged).
Decision: D-250 in `docs/decisions/LOG.md` (PR #241 had also added a D-250; the manager ruled on 2026-10-17 that the merge order is #240 → #242 → #241, so this branch keeps D-250, and PR #241 has already renumbered its line to D-251 in `0b57dc070`).
Review plan: [deploy-hosts-minors-20261017-review-plan.md](deploy-hosts-minors-20261017-review-plan.md);
combined review: [deploy-hosts-minors-20261017-review.md](deploy-hosts-minors-20261017-review.md).

## Checkpoints

Each checkpoint was pushed the moment it was committed (local SHA = remote SHA verified). The
stacked SHAs live on the archive ref; the rebased SHAs are the branch `origin/fix/deploy-hosts-minors-20261017`.

| checkpoint | stacked SHA (archive ref) | rebased SHA (branch) | content | reviews |
|---|---|---|---|---|
| 1 | `1a42cb3a0` | `6fbd33c2c` | the five items, fixtures, guide, D-250 line, envelope, this file | — |
| 2 | `e1157fa6e` | `2fd8d5a64` | full-run evidence, review plan; product tree unchanged | panel R1/R2/R7/R8 reviewed this tree |
| 3 | `a01b58d67` | `083a7ce87` (tree `9741770f3`, identical to `a01b58d67`) | panel MAJORs and minors (see combined review), docs, the four reports verbatim, this file | verify rounds R1/R7/R8 on `083a7ce87` |
| 4 | — | `3d4debffd` | verify-round minors (ssh case model, `-T`, preflight `nginx -t`, exact restore command, status files) | superseded: pushed before run J was read (one fixture failed, see run J); no product change versus checkpoint 5 |
| 5 | — | the commit carrying this version of this file; final head recorded in PR #242 and the combined review | fake `nginx` decides by its arguments (repeated upgrades), run L evidence, this file | — |

## Owned files

- `deploy/deploy-hosts.sh`
- `deploy/debian/ngfw/tests/test_deploy_hosts.py`
- `docs/install/deploy-hosts.md`
- `docs/install/upgrade.md` (one paragraph: `ngfw-web` has no maintainer script, reload nginx)
- `docs/status/DEFERRED-ACCEPTANCE.md` (one section: operator upgrade tool NOT RUN on a real appliance)
- `docs/status/tasks/deploy-hosts-minors-20261017-*.md` (envelope, this file, review plan, review-R1/R2/R7/R8 saved verbatim, combined review)
- `docs/decisions/LOG.md` (one appended line, D-250)

## Completed code (the five items, state after checkpoint 3)

1. **stdin.** The remote scripts still travel on SSH stdin, but through a fixed launcher
   (`REMOTE_SHELL`): `env -i … bash -c 'exec 3<&0 </dev/null && exec bash /dev/fd/3 "$@"' ngfw-deploy ARGS`.
   The pipe moves to descriptor 3, the shell and every remote child get `/dev/null` as stdin and
   bash reads the script through its own copy of the pipe (`/dev/fd/3`, byte-wise, close-on-exec);
   both scripts close the inherited descriptor 3 first. Plain remote commands (`rmdir`, `mkdir`,
   the completion marker) are sent with `</dev/null`. Fixtures: every fake remote child drains stdin
   and records what it swallowed (`tar` excepted: `dpkg-deb` feeds it archive members) and records an
   inherited FIFO on descriptor 3; the tool itself runs with a line of operator input on stdin and the
   fake `ssh` records any stdin a plain command receives; negative controls show the former `bash -s`
   launcher losing the rest of the script to the first stdin-reading child and a script without
   `exec 3<&-` leaking the pipe to its children.
2. **nginx.** After `apt-get install`, the agent/API restart and the restart-counter baseline, the
   install script runs `nginx -t` (output kept in `nginx-test.txt` in the recovery directory) and
   `systemctl reload nginx.service`; `nginx` joins the preflight tool list. Reason (packaging):
   `ngfw-web` ships `/etc/ngfw/nginx.conf` (linked from `sites-enabled` by firstboot) and the web
   assets with no maintainer script, so a changed site applies only on reload. On a failed test the run
   stops and the message says what is true: the running nginx keeps the previous configuration, the new
   site file is on disk and nginx would not start again until fixed, agent/API health was not checked,
   restore or fix `/etc/ngfw/nginx.conf` (member `etc/ngfw/nginx.conf` of `preupgrade-config.tar.gz`)
   then `nginx -t && systemctl reload nginx.service`. Documented limits: `systemctl reload` only
   signals nginx (runtime rejection is not detected), `--force-confold` keeps a locally modified site
   file (`.dpkg-dist`).
3. **No default route.** Still refused by default, now in the preflight before anything is
   transferred ("…needs --allow-no-default-route); nothing was transferred"). `--allow-no-default-route`
   (second long option, exactly one host per run like `--allow-divergent`, local WARNING) declares a
   management-only appliance; its `0/1` literal is the preflight's only argument and the sixth
   validated install argument; the install step rechecks the routes under its lock, records the
   acceptance in `no-default-route.txt` and still compares before/after.
4. **Python >= 3.11.** Checked on the operator machine before any connection
   (`python3 -I -c 'import sys; …; sys.exit(sys.version_info < (3, 11))'`, message names the found
   version) and in the remote preflight (message includes `python3 --version`); documented in the usage
   header and the guide (Ubuntu 26.04 appliance image satisfies it; `python3` stays unversioned in
   `control`). The fixture runs the probes with `sys.version_info` spoofed to 3.10, so the comparison
   itself is exercised.
5. **Underscore hosts.** Host alphabet is `[A-Za-z0-9][A-Za-z0-9._-]*` without `..`; a host with an
   underscore must be an SSH configuration alias whose `Host` block sets a `HostName` other than the
   alias itself, verified with `ssh -G -T` (no connection; errors from the operator's configuration are
   shown). OpenSSH matches `Host` patterns on the name as typed and echoes an unmatched name
   lowercased; the comparison ignores case to recognise that echo, so a differently cased spelling of
   an alias is refused like any unconfigured name (verified against OpenSSH 10.2 in the verify round).
   DNS names and addresses keep the old rule.

Remote command construction is unchanged in kind: the launcher is a fixed literal; the preflight
argument and the six install arguments are validated locally (0/1 literals, path prefix + 8
alphanumerics, 64 hex, `verify_packages` version, preflight-restricted installed version) and
rechecked remotely. Command-scoped `# shellcheck disable=SC2029` annotations: 5 (was 4; the new one
is the preflight command, composed of the launcher literal and the 0/1 literal). No ShellCheck or CI
gate changed.

## Actual test results (this worktree, disposable seams only; no live host, database, VPP, nginx, credentials or network)

Iterations, in order, with the real summary lines (nothing below is edited output):

| run | command | result |
|---|---|---|
| A (checkpoint 1 draft) | 8 targeted tests | `Ran 8 tests in 4.950s` / `FAILED (failures=9)`: every run failed in `verify_packages` with `tar: This does not look like a tar archive` because the new stdin-draining fake `tar` swallowed the archive member `dpkg-deb -f` pipes into it; fix: `tar` exempt from the drain |
| B | the same 8 tests | `Ran 8 tests in 166.155s` / `FAILED (failures=1)`: 7 ok; `test_old_python_is_refused_on_this_machine_and_on_the_target` failed with `'target python3 must be >= 3.11 …' not found in '… target SSH user must already be root …'` because the fake old interpreter also answered the recorders (Python scripts); fix: the fake answers only the version probes |
| C | that 1 test | `Ran 1 test in 15.090s` / `OK` |
| D (checkpoint 1) | full discover | `Ran 33 tests in 604.668s` / `OK` (verbatim lines below) |
| E (checkpoint 3 draft) | 9 targeted tests | `Ran 9 tests in 182.306s` / `FAILED (failures=2)`: `ip -6` count `4 != 3` (the preflight's new route probe) and the nginx recovery message carries the fixture's redirected host prefix on remote stderr; fixes: expected count 4 with a comment, regex that allows the prefix |
| F | those 2 tests | `Ran 2 tests in 44.632s` / `OK` |
| G (checkpoint 3) | full discover | `Ran 33 tests in 625.916s` / `OK` (verbatim lines below); R1's verify-round rerun of the same tree: `Ran 33 tests in 643.642s` / `OK` |
| H (checkpoint 4 draft) | 5 targeted tests | `Ran 5 tests in 91.039s` / `FAILED (failures=1)`: the nginx-failure regex did not allow the new "restore with" clause; fix: regex ends at `tar.gz;` |
| I | the nginx test | `Ran 1 test in 37.163s` / `OK` |
| J (checkpoint 4, `3d4debffd`) | full discover | `Ran 33 tests in 619.241s` / `FAILED (failures=1)`: `test_repeated_upgrade_keeps_previous_backup`. The fake `nginx` decided preflight-vs-install by the `installed` marker, which a second upgrade of the same host already has, so its argument assertion failed in the preflight; fix: decide by the arguments. The product script is unaffected. The developer committed and pushed `3d4debffd` before this result was read (the command chain did not stop on the failed guard), so that head carries the failing fixture model and unfilled run markers; superseded by checkpoint 5 |
| K | the repeated-upgrade and nginx tests | `Ran 2 tests in 61.932s` / `OK` |
| L (checkpoint 5) | full discover | `Ran 33 tests in 630.637s` / `OK` (verbatim lines below) |

Run D, verbatim (`python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v`, tree `1a42cb3a0`):

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
----------------------------------------------------------------------
Ran 33 tests in 604.668s
OK
```

Run G, verbatim (tree of checkpoint 3):

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
----------------------------------------------------------------------
Ran 33 tests in 625.916s
OK
```

Run L, verbatim (tree of checkpoint 5):

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
----------------------------------------------------------------------
Ran 33 tests in 630.637s
OK
```

Other gates on the checkpoint-5 tree (identical results on checkpoints 3 and 4):

```text
bash -n deploy/deploy-hosts.sh deploy/debian/ngfw/prepare.sh      -> exit 0
shellcheck --external-sources deploy/deploy-hosts.sh               -> no findings (0.11.0)
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_prepare.py -v
Ran 4 tests in 3.959s
OK
git diff --stat origin/main...HEAD -- .github tools/ci.sh           -> empty
```

Hosted CI on PR #242:

| head | Mandatory quick gate | Offline packaging and temporary-key signing fixtures |
|---|---|---|
| `e1157fa6e` (checkpoint 2, stacked) | run 39029184249 (superseded by the rebase) | PASS 2 min 42 s, run 39029184371 ("Run all fixtures without skips": `Ran 124 tests in 150.154s` / `OK`, the 33 deploy-hosts tests included) |
| `083a7ce87` (checkpoint 3, rebased onto `main`) | PASS 21 min 22 s, run 39031611498 | PASS 2 min 42 s (run 39031609208) and PASS 1 min 54 s (run 39031611496) |
| checkpoint 4 | recorded in PR #242 by the manager at merge time | idem |

The hosted packaging job stays well inside its 15-minute budget (the whole packaging suite runs in
about 2.5 minutes there; the deploy-hosts suite alone takes 10.5 to 10.7 minutes on this machine).

Independent panel reviewers reproduced the evidence on `e1157fa6e`: R1 `Ran 33 tests in 607.350s`
/ `OK`, `tools/ci.sh check --base d50d4fd2c` PASSED; R2, R7, R8 `bash -n`, `shellcheck` and single
fixtures clean (see their reports).

## Remaining code / current failure / next command

- Remaining code: none known after checkpoint 3.
- Current failure: none.
- Open: hosted quick gate and packaging fixtures on the final head (recorded in PR #242 and the
  combined review when they finish); the manager merges.
- Exact next command (verify round, in the worktree):
  `python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v`

## Decisions

- D-250 (one LOG line; options per item): launcher with private descriptor; test-then-reload nginx
  with the recovery named; explicit one-host `--allow-no-default-route` refused in the preflight
  before transfer and recorded; Python >= 3.11 checked on both sides (no versioned package dependency
  in this task); underscores only in configured SSH aliases (Host patterns match the name as typed; the lowercased echo of an unmatched name is recognised).
- Not done on purpose: no versioned `python3 (>= 3.11)` dependency in `debian/control` (packaging
  change outside the five items; R8 agrees the preflight check plus docs suffice), no worker-PID check
  after the nginx reload (limit documented; not modellable without a real nginx).

## Out of scope / not run

- A real upgrade of an initialized appliance with this tool (nginx reload taking effect, the
  no-default-route path, recovery from a failed step) is NOT RUN; recorded in
  `docs/status/DEFERRED-ACCEPTANCE.md` ("Operator upgrade tool — 2026-10-17"). Fixtures run the real
  shell through recording seams only.
- Follow-up (not in this task, pre-existing): the two `python3 -` heredocs could use `-I` so the
  operator's working directory is not first on the import path (R2-N2).

## Open questions

- Manager: add the `plan/tasks.yaml` row (D-250 confirmed by the manager; PR #241 already carries D-251).
