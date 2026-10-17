# WIP: deploy-hosts-minors-20261017

Branch: `fix/deploy-hosts-minors-20261017`, cut from `origin/fix/deploy-hosts-lineage-routes-20261017`
(PR #240 head `d50d4fd2c`; PR #240 was still open, so this branch stacks on it and rebases onto
`main` once #240 merges).
Envelope: [deploy-hosts-minors-20261017-envelope.md](deploy-hosts-minors-20261017-envelope.md).
Originating review: `docs/status/2026-10-17-unmerged-audit-product-review.md` §2-الف.4 (`b0cab4260`).
PR: not yet opened. Decision: D-250 in `docs/decisions/LOG.md`.

## Owned files

- `deploy/deploy-hosts.sh`
- `deploy/debian/ngfw/tests/test_deploy_hosts.py`
- `docs/install/deploy-hosts.md`
- `docs/status/tasks/deploy-hosts-minors-20261017-*.md`
- `docs/decisions/LOG.md` (one appended line, D-250)

## Completed code (the five items)

1. **stdin.** The remote scripts still travel on SSH stdin, but through a launcher
   (`REMOTE_SHELL`): `bash -c 'exec 3<&0 </dev/null && exec bash /dev/fd/3 "$@"' ngfw-deploy ARGS`.
   The pipe moves to descriptor 3, the shell and every remote child get `/dev/null` as stdin and
   bash reads the script through its own copy of the pipe (`/dev/fd/3`, byte-wise, close-on-exec);
   both scripts close the inherited descriptor 3 first. Plain remote commands (`rmdir`, `mkdir`,
   the completion marker) are sent with `</dev/null`. Every fake remote child in the fixtures now
   drains stdin and records what it swallowed (`tar` excepted: `dpkg-deb` feeds it archive members);
   the success path asserts nothing was consumed and a negative control shows the former `bash -s`
   launcher losing the rest of the script to the first stdin-reading child.
2. **nginx.** After `apt-get install` and the agent/API restart the install script runs
   `nginx -t -q` and `systemctl reload nginx.service`; `nginx` joins the preflight tool list.
   Reason (packaging): `ngfw-web` ships `/etc/ngfw/nginx.conf` (linked from `sites-enabled` by
   firstboot) and the web assets with no maintainer script, so a changed site applies only on
   reload; a bare reload would hide a broken configuration, hence the test first.
3. **No default route.** Still refused by default; `--allow-no-default-route` (second long
   option, any host count) declares a management-only appliance. The literal `0/1` is the sixth
   validated remote argument; the acceptance is recorded in `no-default-route.txt` in the
   recovery directory and the before/after route comparison still applies.
4. **Python >= 3.11.** Checked on the operator machine before any connection
   (`python3 -I -c 'import sys; ...; sys.exit(sys.version_info < (3, 11))'`, message names the
   found version) and in the remote preflight (message includes `python3 --version`), documented in
   the usage header and the operator guide.
5. **Underscore hosts.** Host alphabet is `[A-Za-z0-9][A-Za-z0-9._-]*` without `..`; a host with
   an underscore must be an SSH configuration alias whose `Host` block sets `HostName`, verified
   with `ssh -G` (no connection) before anything else; DNS names and addresses keep the old rule.

Remote command construction is unchanged in kind: the launcher is a fixed literal, the six
arguments are validated locally (path prefix + 8 alphanumerics, 64 hex, `verify_packages`
version, 0/1 literals, preflight-restricted installed version) and rechecked remotely.

## Actual test results (this worktree, disposable seams only)

Targeted run after the last code change (`cd deploy/debian/ngfw/tests && python3 -m unittest -v ...`):

```text
test_remote_children_cannot_consume_the_script ... ok
test_success_uses_private_backup_and_verified_installs ... ok
test_nginx_is_validated_and_reloaded_after_install ... ok
test_no_default_route_needs_explicit_flag_and_is_recorded ... ok
test_underscore_hosts_need_a_configured_ssh_alias ... ok
test_newer_main_build_over_divergent_branch_needs_explicit_override ... ok
test_override_never_accepts_release_downgrade_or_mixed_versions ... ok
Ran 8 tests in 166.155s  (one failure, fixed below)
test_old_python_is_refused_on_this_machine_and_on_the_target ... ok
Ran 1 test in 15.090s
OK
```

```text
bash -n deploy/deploy-hosts.sh deploy/debian/ngfw/prepare.sh      -> exit 0
shellcheck --external-sources deploy/deploy-hosts.sh               -> no findings (0.11.0)
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_prepare.py -v
Ran 4 tests in 3.879s
OK
git diff --stat origin/main...HEAD -- .github tools/ci.sh           -> empty
```

Full `test_deploy_hosts.py` discover run (33 tests): in progress at this checkpoint; result is
recorded below when it finishes.

## Remaining code / current failure / next command

- Remaining code: none known.
- Current failure: none in the targeted run. Two fixture-only iterations were needed: the
  stdin-draining recorder broke `dpkg-deb -f` (it pipes archive members into `tar`; `tar` is now
  exempt) and the fake old interpreter must answer only the version probes (the recorders are
  Python scripts themselves).
- Exact next command (in the worktree):
  `python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v`

## Decisions

- D-250 (one LOG line): launcher with private descriptor; validate-then-reload nginx; explicit
  `--allow-no-default-route` with a recovery record; Python >= 3.11 on both sides; underscores
  only in configured SSH aliases.
- Not done on purpose: no versioned `python3 (>= 3.11)` dependency in `debian/control` (packaging
  change outside the five items; the tool checks the interpreter instead), no `-I` for the
  `python3 -` heredocs (not asked; unchanged behaviour).

## Open questions

- Manager: confirm the D-250 number and add the `plan/tasks.yaml` row.
- PR #240 merge order: this branch stacks on `fix/deploy-hosts-lineage-routes-20261017`; after #240
  lands, rebase onto `main` (expected: clean, same files but disjoint hunks) before merging.
