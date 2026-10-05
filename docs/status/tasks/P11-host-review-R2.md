# P11-host — independent R2 security review

Reviewed source: `f59c3ea7b9cf0d6ad52a0f1c4db82bb0f6bddd1b`.

Review performed independently as R2 security; no product files edited. Diff reviewed against the merge base with origin/main. Focused tests used disposable fixtures with TMPDIR=/root/.cache/review-r2; no shared service, package, boot configuration or VPP changes. Full CI and appliance runtime acceptance belong to separate gates.

Secret scan: `gitleaks detect --no-git --source docs/status/tasks --config .github/gitleaks.toml --redact --no-banner` in this task worktree: exit 0, no leaks found. A second scan copied exactly changed existing files into disposable scratch and used the same configuration: exit 0, no leaks found. Neither scan establishes that a runtime release builder cannot export a key.

No BLOCKER, MAJOR or MINOR findings. Reviewed opt-in/root guard, collision refusal and fixed fixture locking, lab environment parsing as data rather than shell evaluation, explicit reviewed binaries, fresh agent build, namespace isolation, private mode0700 work directory, mode0600 logs, mode0600 captures written inside private evidence, safe public summary carrying capture hashes rather than SA/API/peer diagnostics. Exceptions do not print raw diagnostic log contents. Added rollback assertions do not add a product privilege boundary.

Command executed in /root/ngfw-wt/ready-p11-host-20261005: `TMPDIR=/root/.cache/review-r2 PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s test/topology/ipsec -p test_host_acceptance.py -v`

```text
test_refuses_owned_resource_collisions ... ok
test_slot_exports_parsed_as_data ... ok
Ran 2 tests in 0.000s
OK
```
Packet campaign was not rerun by R2; separate tester owns that evidence.

Verdict: APPROVE.
