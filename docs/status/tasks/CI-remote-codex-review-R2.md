# CI-remote-codex — R2 security review

Scope: `.github/workflows/ci.yml` only; independent review of the current file. Read `prompts/00-CONTEXT.md`, `prompts/REVIEW-PROMPT.md`, and `prompts/reviewers/R2-security.md`. The manager's task envelope authorizes remote CI and the actual worktree location, overriding the older local-only convention.

## Findings

No BLOCKER, MAJOR, or MINOR findings in this scope.

- Lines 3–9: ordinary `pull_request`, `push`, and manual triggers; no `pull_request_target`. The workflow token has only `contents: read` and no explicit repository secrets are injected. Pull-request code executes on an ephemeral GitHub-hosted runner (line 18), not the appliance host.
- Lines 28–40 and 80: action dependencies use complete commit SHA pins. Checkout disables persisted credentials. No shared dependency cache is enabled for Go and no cache-save action is present.
- Lines 65–77: event/ref expressions enter environment variables, rather than interpolated shell source. Quoted Bash array arguments preserve a malicious-looking ref as one argument; they do not evaluate its contents as commands.
- Lines 44–62: package versions are explicit. Buf downloads use fixed release URLs; checksum filtering accepts only the expected asset name. `pipefail` plus `sha256sum --check --strict` prevents installation on missing, malformed, or mismatched checksum entries. Release checksums fetched from the same publisher protect integrity but do not independently authenticate a compromised publisher; immutable action pins and ecosystem package verification remain separate controls.
- Lines 78–85: upload is restricted to the runner's gate-log directory rather than checkout contents or the home directory, runs even after failure, and expires after 14 days. Adjacent `tools/ci.sh` inspection confirms its gitleaks invocation uses `--redact` for console output and the JSON report. Arbitrary PR code can write arbitrary logs, but this workflow provides it no explicit repository secrets or write-capable token.

## Verification evidence

Commands ran in `/workspace/scratch/96b8b6fbc8a7/NGFW-ci`, not the historical `/root/ngfw-wt` path. No full quick gate or appliance tests ran; the UDS baseline restriction is outside this security review.

Inspected numbered workflow, token settings, and log/tool-install call sites using `nl -ba .github/workflows/ci.yml` and `rg -n 'install-tools|curl|sha256|VRX_CI_LOG_DIR|tee|gitleaks' tools/ci.sh`.

Executed the exact workflow checksum-filter pipeline on local synthetic files:

```text
valid: exit=0 buf-Linux-x86_64: OK
mismatch: exit=1 buf-Linux-x86_64: FAILED
sha256sum: WARNING: 1 computed checksum did NOT match
missing: exit=1 sha256sum: 'standard input': no properly formatted checksum lines found
malformed: exit=1 sha256sum: 'standard input': no properly formatted checksum lines found
```

Executed a Bash argv probe with `BASE_REF='main; printf injected'` and the workflow's quoted array construction:

```text
argv probe: <quick> <--base> <origin/main; printf injected>
```

The ref remained one literal argument. No injected command ran.

`rg -n 'BEGIN (RSA|EC|OPENSSH) PRIVATE|password\s*[:=]\s*[^<]|secrets\.|pull_request_target|write-all' .github/workflows/ci.yml` returned no matches (exit 1).

This establishes local security properties, not a successful hosted GitHub run or independently verified release availability.

Verdict: **APPROVE**.
