# SSH deployment completion checkpoint

- Branch: `codex/finish-deploy-20261014`.
- Source checkpoint: `f9f24c52` from `claude/status-check-report-aa44ab`; original deployment history remains intact.
- Published checkpoint: local `a0aae79d44870481e84633be91fb80197eb68f41`, remote `4dca909d4dbf9950180ad97094b21920c731cf5e`, identical tree `2cd5a3dc5bd5b99fbbb25892d16cd6fa06e1bdc1` (parent confirmed connector publication).
- This follow-up commit closes review findings; parent records its remote SHA after publication.
- Owned files: `deploy/deploy-hosts.sh`, `deploy/debian/ngfw/prepare.sh`, `deploy/debian/ngfw/tests/test_deploy_hosts.py`, `deploy/debian/ngfw/tests/test_prepare.py`, `docs/install/deploy-hosts.md`, this record and the deployment envelope.
- Scope: finish the existing dedicated initialized-appliance upgrade wrapper. Fresh OS/runtime provisioning, firstboot setup and real-host deployment are outside this wrapper.

Implemented strict complete checksum/package metadata/version/architecture validation;
source commit, full-history commit count and verified VPP manifest cache binding;
safe argument and remote path validation; private local staging and unique remote
recovery directories; strict SSH host-key verification and disabled connection multiplexing; serialized target upgrade;
downgrade and dependency change refusal; mandatory config/peer database backups
with archive checks and canonical master-key path verification; no forced downgrade,
checksum auto-generation or ignored backup errors; individual service/loopback API/
default route/version checks and a final fresh SSH connection.

Review findings addressed:

- `systemctl is-active` checks each required service separately before installation
  and afterwards. Mixed service-state fixtures reproduce the real multi-unit
  ANY-active behavior and prove failure when agent/nginx/VPP is inactive.
- The lock is append-only under root-owned mode-0700 `/root/ngfw-deploy`, never the
  shared writable `/run/lock` path. Existing lock links or incorrect ownership/mode
  are refused. Fixtures prove old shared-lock links cannot truncate files and
  private-lock links are rejected without target mutation.
- Canonical preparation now emits `0.1.0~dev+git<count>.<sha12>` from complete Git
  history. Counts increase for descendant commits; `git` sorts above legacy
  hexadecimal-only development versions. Shallow histories are refused before
  build/staging. The stable-release downgrade guard remains unchanged. Cache
  metadata and package versions are bound to the count and source SHA.

Actual verification:

```text
bash -n deploy/deploy-hosts.sh deploy/debian/ngfw/prepare.sh
exit 0
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v
Ran 20 tests in 44.344s; OK.
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_prepare.py -v
Ran 4 tests in 0.797s; OK.
bash tools/ci.sh check --base origin/main
check PASSED (0m03s); gitleaks unavailable, built-in secret scan only.
git diff --check
exit 0
```

No real host, credentials, package installation or live services were accessed.
Fixtures execute the actual wrapper and remote control flow using real minimal
Debian archives and disposable command seams. The operator guide explains package
trust, dedicated-host scope, version ordering, manual recovery, quiescent writers
and limits of sequential backups and fixture acceptance.

Remaining: final independent review, complete hosted quick and strict packaging
fixture gates, expected-head integration and appliance acceptance. These remain
manager/operator work; none are represented as passed here. No worker code blocker
remains. Next command for narrow replay:
`python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v`.
