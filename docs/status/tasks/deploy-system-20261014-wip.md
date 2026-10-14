# SSH deployment completion checkpoint

- Branch: `codex/finish-deploy-20261014`.
- Source checkpoint: `f9f24c52` from `claude/status-check-report-aa44ab`; its original deployment history remains intact.
- Current local/remote SHA: recorded by the manager after checkpoint publication; no remote publication claimed by this file.
- Owned files: `deploy/deploy-hosts.sh`, `deploy/debian/ngfw/tests/test_deploy_hosts.py`, `docs/install/deploy-hosts.md`, this record and the deployment envelope.
- Scope: finish the existing initialized-appliance upgrade wrapper. Fresh OS/runtime provisioning, firstboot setup and real-host deployment are not implemented by this wrapper.

Implemented strict complete checksum/package metadata/version/architecture validation;
source commit + verified VPP manifest cache binding; safe argument and remote path
validation; private local staging and unique per-run remote recovery directories;
strict SSH host-key verification; serialized target upgrade; downgrade and dependency
change refusal; mandatory config/peer database backups with archive checks;
no VPP restart, forced downgrade, checksum auto-generation or ignored backup errors;
service/loopback API/default route/version checks and a final fresh SSH connection.
The operator guide explains package trust, non-monotonic development versions,
manual recovery and the limits of sequential backups and fixture acceptance.

Executed before final checkpoint:

```text
bash -n deploy/deploy-hosts.sh
exit 0
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v
Initial coherent fixture run: Ran 13 tests in 24.716s; OK.
Expanded fixture run: Ran 14 tests in 29.880s; OK.
bash tools/ci.sh check --base origin/main
check PASSED (0m03s); gitleaks unavailable, built-in secret scan only.
```

No real host, credentials, package installation or live services were accessed.
Fixtures execute the actual wrapper and remote control flow using real minimal
Debian archives and disposable command seams. Full hosted quick and packaging
fixture gates, independent reviews, expected-head integration and appliance
acceptance remain manager/operator work; none are represented as passed here.

Current review findings: multi-unit is-active must check each unit separately;
move deployment lock into the private root directory; replace non-monotonic
source-hash-only development package versions with reviewed source-derived ordering.
These remain open in this checkpoint; no merge readiness is claimed. Next command:
`python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v`.
