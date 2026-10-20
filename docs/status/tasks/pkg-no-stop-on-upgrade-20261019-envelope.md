# Task envelope: pkg-no-stop-on-upgrade-20261019

Role: developer. Branch: `claude/musing-ramanujan-bd8fe0` (worktree
`/root/NGFW/.claude/worktrees/musing-ramanujan-bd8fe0`, cut from `origin/main` `cb148aafb`).
Started by the manager session on 2026-10-19 as a side session.

Background (from the manager): on 2026-10-19 appliance 172.30.126.37 was upgraded with
`deploy/deploy-hosts.sh` (receipt `docs/status/tasks/deploy-host-37-20261019-wip.md` on PR #244).
The journal showed that during the package upgrade dpkg ran a generated `ngfw-meta` maintainer
script executing `deb-systemd-invoke stop 'ngfw-firewall-bootstrap.service' 'ngfw-firstboot.service'`;
`vpp.service` has `Requires=ngfw-firstboot.service`, so systemd stopped VPP and, through
`ngfw-agent.service`'s `Requires=vpp.service`, the agent; the script's later restart brought them
back: about nine seconds of data-plane outage on every package upgrade, although `deploy-hosts.sh`
never asks for a VPP restart and `docs/install/deploy-hosts.md` says so. The same was recorded for
the 2026-10-13 upgrade.

Task: decide and implement how a package upgrade stops restarting VPP. Investigate first: where
`vpp.service` gets `Requires=ngfw-firstboot.service`, which units `dh_installsystemd` lists in each
package's maintainer scripts, and whether `--no-stop-on-upgrade` for the provisioning oneshots or
replacing `Requires=` with `Wants=`/`After=` for already-completed provisioning units is the safer
contract; keep first-boot semantics (firstboot must still run on a fresh appliance and must not
re-run on upgrade). Record the decision in `docs/decisions/LOG.md`, update
`docs/install/deploy-hosts.md` and `docs/install/upgrade.md` if their claims change, and extend
`deploy/debian/ngfw/tests` so the generated maintainer scripts are asserted not to stop the units
that `vpp.service` requires.

Done means: packaging tests pass (`python3 -m unittest discover -s deploy/debian/ngfw/tests -v`),
a built package set from `deploy/debian/ngfw/prepare.sh` (verified VPP output
`/root/Documents/Codex/2026-10-04/debian-latest/outputs/NGFW-Debian-latest-development/VPP-26.06-release+ngfw3-amd64`)
shows the intended maintainer-script contents, commits pushed, a WIP note under
`docs/status/tasks/`, and a PR against `main`. Do not merge and do not upgrade any host.

Owned files: `deploy/debian/ngfw/debian/rules`, `deploy/debian/ngfw/tests/maintainer_scripts.py`,
`deploy/debian/ngfw/tests/test_packaging.py`, `deploy/debian/ngfw/tests/test_firstboot.py`,
`.github/workflows/packaging-fixtures.yml`, `deploy/debian/README.md`, `docs/install/upgrade.md`,
`docs/install/deploy-hosts.md`, `docs/status/DEFERRED-ACCEPTANCE.md` (one section),
`docs/decisions/LOG.md` (one appended line, D-253),
`docs/status/tasks/pkg-no-stop-on-upgrade-20261019-*.md`.
