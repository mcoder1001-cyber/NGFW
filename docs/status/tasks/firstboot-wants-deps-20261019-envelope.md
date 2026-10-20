# Task envelope: firstboot-wants-deps-20261019

Role: developer. Branch: `claude/trusting-jackson-71c8da` (worktree
`/root/NGFW/.claude/worktrees/trusting-jackson-71c8da`), cut from PR #246's head
`9cabd7f47` (`claude/musing-ramanujan-bd8fe0`, D-253), itself cut from `origin/main` `cb148aafb`.
Started by the manager session on 2026-10-19 as a side session. Stacked on PR #246: once #246 is
merged, `git rebase --onto origin/main 9cabd7f47` leaves only this task's commits.

Background (from the manager): D-253 (PR #246) stopped our own package upgrades from stopping the
provisioning oneshots, but kept the first-boot gate as `Requires=` + `After=`. The same propagation
path stays open from the other side: `deploy/systemd/ngfw-firstboot.service` carries
`Requires=postgresql.service valkey-server.service nftables.service`, so per systemd.unit(5) an
explicit stop or restart of PostgreSQL or Valkey (their own package upgrades, an operator's
`systemctl restart`) restarts `ngfw-firstboot.service` and therefore `vpp.service`, the agent and
the API. Recorded as follow-up 1 of D-253, not decided.

Task: decide and implement how `ngfw-firstboot.service` depends on `postgresql.service`,
`valkey-server.service` and `nftables.service` so that an explicit restart of PostgreSQL or Valkey
no longer propagates to `vpp.service`, `ngfw-agent.service` and `ngfw-api.service`, while keeping
first-boot semantics (fresh appliance: firstboot.sh runs after PostgreSQL/Valkey/nftables and before
VPP, agent, API and nginx; fails closed when they are unavailable; never re-runs on upgrade).
Confirm the propagation claim against systemd.unit(5); observe `systemctl restart
valkey-server.service` with and without the change on a disposable systemd instance or lab
appliance if available; never touch the shared dev host's VPP or a production appliance. Record the
decision as the next free D-number, extend the tests (`stop_propagation_closure('vpp.service')`
asserts the intended set, first-boot ordering preserved), update `docs/install/upgrade.md` and
`docs/status/DEFERRED-ACCEPTANCE.md`, keep a WIP note with an envelope, run
`python3 -m unittest discover -s deploy/debian/ngfw/tests -v` (pass, no skips), build the package
set with `deploy/debian/ngfw/prepare.sh` (verified VPP output
`/root/Documents/Codex/2026-10-04/debian-latest/outputs/NGFW-Debian-latest-development/VPP-26.06-release+ngfw3-amd64`),
commit and push each checkpoint, open a PR against `main`. Do not merge, do not upgrade any host.

Owned files: `deploy/systemd/ngfw-firstboot.service`, `deploy/upgrade/ngfw-upgrade-prepare.service`,
`deploy/debian/ngfw/assets/firstboot.sh`, `deploy/debian/ngfw/tests/maintainer_scripts.py`,
`deploy/debian/ngfw/tests/test_firstboot.py`, `deploy/debian/ngfw/tests/test_packaging.py`,
`deploy/debian/ngfw/tests/lab/firstboot-propagation.sh` (new), `deploy/debian/README.md`,
`docs/install/upgrade.md`, `docs/install/deploy-hosts.md` (one sentence),
`docs/status/DEFERRED-ACCEPTANCE.md` (one section), `docs/decisions/LOG.md` (one appended line,
D-255), `docs/status/tasks/firstboot-wants-deps-20261019-*.md`.
