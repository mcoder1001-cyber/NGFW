# Task envelope: deploy-hosts-lineage-20261017

Source: independent product review 2026-10-17 (`docs/status/2026-10-17-unmerged-audit-product-review.md`
at commit `b0cab4260f9214b988508258b493f209ffa2f789` on `claude/review-unmerged-code-product-0351a9`,
section 2-الف) of PR #237 (`deploy/deploy-hosts.sh`).

Fix on a branch cut from `origin/main`, with fixtures, and open a PR:

1. Downgrade guard trusts `dpkg --compare-versions` on `0.1.0~dev+git<count>.<sha>` versions, but
   commit counts order only descendant commits. Verify ancestry of the installed and new source
   commits before transfer; refuse divergent or unresolvable histories unless `--allow-divergent`;
   update DEC-248 and `docs/install/deploy-hosts.md`.
2. The raw text of `ip -6 route show default` is compared before/after; RA routes carry
   `expires NNNsec`, so a successful install reports a changed route. Compare via/dev/metric only
   and add an expiring-route fixture.
3. The post-restart agent check is a single `systemctl is-active`; add a stability window and a
   restart-counter comparison so a crash-looping agent is not reported healthy.

Constraints: remote command construction stays strictly validated; no ShellCheck or CI gate is
loosened. Gates: `bash -n`, `shellcheck --external-sources deploy/deploy-hosts.sh`,
`test_deploy_hosts.py`, `test_prepare.py`. Independent review per `prompts/REVIEW-PROMPT.md`.
