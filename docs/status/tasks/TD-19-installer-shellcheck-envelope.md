# TD-19 installer ShellCheck companion

Base: `038ee06840e51f823bc69d080f454d5a070948e2` (frozen PR 71).
Branch: `task/TD19-installer-shellcheck-20261002`.
Worktree: `NGFW-installer-lint`.
Owned: `.github/workflows/provisioning-fixtures.yml`, this envelope and matching WIP; genuine installer diagnostics only after observed hosted findings and manager agreement for legacy scope.

Check the four edited shell files using `shellcheck -x -P SCRIPTDIR` on the Ubuntu 24.04 hosted fixture runner. Verify presence and print actual ShellCheck version; missing tool fails. No local installation, network provisioning, services, VPP, SSH, package mutation or laboratory execution. No suppression, exclusion, or weakening of the unchanged quick gate. The existing provisioning fixture suite still runs with zero skips required. Ubuntu runner validation is source lint, not Ubuntu 26.04 appliance acceptance.

Independent reviewer and current-main integration, unchanged hosted quick and companion gate are manager responsibilities before merge. Full TD-19 repository key provenance and laboratory acceptance remain outside this narrow scope.
