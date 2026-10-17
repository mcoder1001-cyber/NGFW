# Task envelope: deploy-hosts-minors-20261017

Role: developer. Branch: `fix/deploy-hosts-minors-20261017`, cut from
`origin/fix/deploy-hosts-lineage-routes-20261017` (PR #240 head `d50d4fd2c`, not yet merged
when this task started).

Task (from the manager session, 2026-10-17):

Cut a branch from `origin/main` after PR #240 is merged, or from that branch if it is not
yet merged, and close the five minor items the 2026-10-17 review listed for
`deploy/deploy-hosts.sh` (`docs/status/2026-10-17-unmerged-audit-product-review.md`,
section 2-الف.4, at `b0cab4260` on `claude/review-unmerged-code-product-0351a9`):

1. the remote scripts are fed to `bash -s` on stdin, so any child that reads stdin would
   consume the rest of the script; either redirect every remote child's stdin from
   `/dev/null` or send the script another way, and add a fixture whose fake child reads stdin;
2. nginx is not reloaded after the upgrade; decide with the packaging docs whether `ngfw-web`
   changes need `systemctl reload nginx.service` and implement or document the decision;
3. a host without any default route is refused ("no management default route"); decide
   whether a management-only appliance on a link-local network is a valid target and either
   allow it with an explicit flag or document the refusal;
4. document the Python >= 3.11 requirement (`hashlib.file_digest`) for both the operator
   machine and the appliance, and fail with a clear message on older Python;
5. the host pattern rejects underscores; allow SSH configuration aliases with underscores
   while keeping DNS names strict, with fixtures.

Keep remote command construction strictly validated and do not loosen ShellCheck or any CI
gate. Update `docs/install/deploy-hosts.md`, keep this WIP file, run the four gates, push,
open a PR and request the panel review per `prompts/REVIEW-PROMPT.md` (R1, R2, R7, R8).

Done means: PR open with all gates green locally and the five items each fixed or
explicitly decided in the docs.

Gates: `bash -n deploy/deploy-hosts.sh`; `shellcheck --external-sources deploy/deploy-hosts.sh`;
`python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v`;
`python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_prepare.py -v`.
