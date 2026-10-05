# F-hardening-lite — independent R2 security review

Reviewed source: `73b6c32fb19087652691b95c38f8b1ea8e2ec60b`.

Review performed independently as R2 security; no product files edited. Diff reviewed against the merge base with origin/main. Focused tests used disposable fixtures with TMPDIR=/root/.cache/review-r2; no shared service, package, boot configuration or VPP changes. Full CI and appliance runtime acceptance belong to separate gates.

Secret scan: `gitleaks detect --no-git --source docs/status/tasks --config .github/gitleaks.toml --redact --no-banner` in this task worktree: exit 0, no leaks found. A second scan copied exactly changed existing files into disposable scratch and used the same configuration: exit 0, no leaks found. Neither scan establishes that a runtime release builder cannot export a key.

No BLOCKER, MAJOR or MINOR findings. Reviewed offline canonical target restrictions, symlink ancestor preflight before any mutation, repeat-stage opt-in preservation, valid SSH key precondition, explicit management interface checks, least-privilege profiles and config-only compliance claims. Signed metadata is copied once into a private directory; gpgv verifies those exact bytes and the parser consumes that immutable release snapshot. Key/signature/release reads are bounded; each member checksum covers the same byte snapshot whose size is checked. The earlier pathname verify/parse race is fixed. No package installation or runtime sandbox activation occurs.

Command executed in /root/ngfw-wt/ready-hardening-20261005: `TMPDIR=/root/.cache/review-r2 PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s deploy/hardening/tests -v`

```text
test_metadata_replaced_after_gpgv_cannot_change_authenticated_members ... ok
Ran 9 tests in 2.189s
OK
```

Verdict: APPROVE.

Security addendum: inspected source 837842ecec5c1b8c358b7a242220a2cd9a176224. Its only addition is test/hardening-lite/go.mod and a Go wrapper invoking fixed repository deploy/hardening/tests/run.sh via exec.Command with no shell or external input. Security product files unchanged; APPROVE remains. Wrapper not rerun by R2.
