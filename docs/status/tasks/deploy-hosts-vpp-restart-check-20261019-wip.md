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
| 0 | (this commit) | envelope and this file |

## Owned files

- `deploy/deploy-hosts.sh`
- `deploy/debian/ngfw/tests/test_deploy_hosts.py`
- `docs/install/deploy-hosts.md`
- `docs/status/DEFERRED-ACCEPTANCE.md` (one paragraph in the operator upgrade tool section)
- `docs/decisions/LOG.md` (one appended line, D-254)
- `docs/status/tasks/deploy-hosts-vpp-restart-check-20261019-*.md`

## Completed code

Nothing yet.

## Remaining code

1. Remote install script: record the five watched units before `apt-get install` and after the
   agent/API restart-counter check; write `unit-state-before.txt` / `unit-state-after.txt`; fail on
   any difference with before/after lines and a unit-specific hint; warn on an already-stopped socket.
2. Fixtures: fake `systemctl show` keyed form; cases unchanged / `vpp-restart` / `vpp-crash` /
   `socket-stopped` / `socket-preinactive`.
3. Docs: `deploy-hosts.md` (APT step, health checks, fixture list), `DEFERRED-ACCEPTANCE.md`, D-254.

## Actual test results

None yet.

## Current failure

None.

## Exact next command

Edit `deploy/deploy-hosts.sh` (remote install script, before `echo 'installing packages'` and after
the agent/API NRestarts comparison), then `bash -n deploy/deploy-hosts.sh && shellcheck --external-sources deploy/deploy-hosts.sh`.
