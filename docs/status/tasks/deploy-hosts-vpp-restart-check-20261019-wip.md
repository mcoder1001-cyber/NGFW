# WIP: deploy-hosts-vpp-restart-check-20261019 — detect a VPP restart during the upgrade

Branch: `claude/gifted-cartwright-6b6380` (cut from `origin/main` `cb148aafb`), worktree
`/root/NGFW/.claude/worktrees/gifted-cartwright-6b6380`.
Envelope: [deploy-hosts-vpp-restart-check-20261019-envelope.md](deploy-hosts-vpp-restart-check-20261019-envelope.md).
Origin: follow-up 3 of `pkg-no-stop-on-upgrade-20261019-wip.md` (PR #246, D-253); receipt
`deploy-host-37-20261019-wip.md` (PR #244).
PR: not yet opened.

## Checkpoints

Each checkpoint is pushed the moment it is committed (local SHA = remote SHA).

| checkpoint | SHA | content |
|---|---|---|
| 0 | `72c6a7e21` | envelope and this file |
| 1 | (this commit) | remote install script unit-state records and comparison, fixtures, docs, D-254, this file |

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
- Full `-p test_deploy_hosts.py` and whole-suite discover: pending.

## Current failure

None.

## Exact next command

`python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v` (then the
whole-suite discover), record the summary lines here, open the PR.
