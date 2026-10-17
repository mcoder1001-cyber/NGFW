# Review plan: deploy-hosts-minors-20261017

Dispatcher output per `prompts/REVIEW-PROMPT.md`. Branch `fix/deploy-hosts-minors-20261017`,
diff against its base `origin/fix/deploy-hosts-lineage-routes-20261017` (PR #240 head `d50d4fd2c`).

## Changed paths by area

| area | paths |
|---|---|
| deploy tooling (product) | `deploy/deploy-hosts.sh` |
| deploy fixtures | `deploy/debian/ngfw/tests/test_deploy_hosts.py` |
| operator docs | `docs/install/deploy-hosts.md` |
| decisions / status | `docs/decisions/LOG.md` (D-250), `docs/status/tasks/deploy-hosts-minors-20261017-*.md` |

## Reviewers to spawn

| id | reason |
|---|---|
| R1 correctness & tests | mandatory on every branch; five acceptance items each need a failing-without-it fixture |
| R2 security | mandatory; remote command construction changed (launcher literal, sixth argument, `ssh -G`, Python probes) |
| R7 docs & evidence | mandatory; D-250 decision line, operator guide, WIP evidence, scope fence (five items only) |
| R8 operability & packaging | `deploy/**`: nginx validate/reload after APT, no-default-route flag, Python requirement, ShellCheck |

Not added: R3 (no contract/API change), R4 (no VPP/dataplane/shared-host change; VPP is never
restarted), R5 (no hot path), R6 (no web/i18n change).

## Testers

None beyond the host-independent fixtures (`test_deploy_hosts.py`, `test_prepare.py`); there is no
lab acceptance for this tool in scope. Appliance acceptance of a real upgrade stays deferred per
`docs/status/DEFERRED-ACCEPTANCE.md`.
