# Task envelope: deploy-hosts-vpp-restart-check-20261019

Role: developer. Branch: `claude/gifted-cartwright-6b6380` (worktree
`/root/NGFW/.claude/worktrees/gifted-cartwright-6b6380`, cut from `origin/main` `cb148aafb`).
Started by the manager session on 2026-10-19 as a side session.

Background (from the manager): `deploy/deploy-hosts.sh` upgrades an initialized appliance over
SSH: preflight, backups, `apt-get install ./ngfw-*.deb`, then
`systemctl restart ngfw-agent.service ngfw-api.service`, nginx test and reload, API health, a
10 s stability window with an NRestarts comparison for agent/API, and a default-route comparison;
`docs/install/deploy-hosts.md` says the script makes no VPP restart request. On 2026-10-13 and
2026-10-19 the upgrade of appliance 172.30.126.37 nevertheless restarted VPP for about 9 s,
because the packages' generated preinst stopped `ngfw-firstboot.service`, which `vpp.service`
requires (receipt `docs/status/tasks/deploy-host-37-20261019-wip.md` on PR #244). D-253 (PR #246,
task `pkg-no-stop-on-upgrade-20261019`) fixed the packaging with
`dh_installsystemd --no-stop-on-upgrade` and tests the generated maintainer scripts; the first
upgrade onto the fixed packages is expected to be outage-free. The script itself never noticed
the restart: it only requires `vpp.service` to be active at the end (follow-up 3 of the D-253
WIP note).

Task: make `deploy/deploy-hosts.sh` detect a VPP restart during the upgrade instead of only
requiring `vpp.service` to be active afterwards. In the remote install script record
`systemctl show` state (`ActiveEnterTimestampMonotonic`, `NRestarts`) of `vpp.service`, and
consider `nftables.service` and the three sockets `apply-executor.socket`,
`ngfw-ra-openfile.socket`, `ngfw-ra-namespace-broker.socket` (which pre-D-253 packages left
stopped), before `apt-get install` and again after the agent/API restart; write both to the
recovery directory; fail the run with a message naming the before/after values when VPP's
activation timestamp changed or NRestarts rose; decide and document whether a stopped socket
after the upgrade is a failure or a recorded warning. Keep the remote command construction
strictly validated, do not loosen ShellCheck or any gate, keep the fake-systemctl seam of the
fixtures consistent. Extend `deploy/debian/ngfw/tests/test_deploy_hosts.py` with fixtures for an
unchanged VPP (pass), a changed activation timestamp (fail with the recovery directory named) and
the socket case; update `docs/install/deploy-hosts.md` (the APT-step paragraph and the health
checks) and `docs/status/DEFERRED-ACCEPTANCE.md`.

Gates: `bash -n deploy/deploy-hosts.sh`; `shellcheck --external-sources deploy/deploy-hosts.sh`;
`python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v` (slow);
`python3 -m unittest discover -s deploy/debian/ngfw/tests -v`.

Done means: WIP note and this envelope under `docs/status/tasks/`, each checkpoint committed and
pushed, a PR against `main`. Do not merge and do not run the script against any host.

Owned files: `deploy/deploy-hosts.sh`, `deploy/debian/ngfw/tests/test_deploy_hosts.py`,
`docs/install/deploy-hosts.md`, `docs/status/DEFERRED-ACCEPTANCE.md` (one paragraph),
`docs/decisions/LOG.md` (one appended line, D-254),
`docs/status/tasks/deploy-hosts-vpp-restart-check-20261019-*.md`.
