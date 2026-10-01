# CI-remote-codex — GitHub-hosted quick gate

## Scope

Add the thin GitHub Actions wrapper already described in `docs/contributing.md`.
It calls the existing mandatory `tools/ci.sh quick` without replacing, skipping,
or weakening any repository checks. This is CI infrastructure, not a completed
WBS product feature.

The workflow runs on pull requests, pushes and manual dispatch. Permissions are
read-only, checkout credentials are not persisted, full history is available to
the contract guard and gitleaks, and no application secrets or private lab access
are required. Concurrent runs of the same ref cancel older runs. There is no
schedule and no full integration gate or VPP restart.

Node 22.23.2, pnpm 12.5.1, Go 1.26.0, buf 1.73.0, golangci-lint 2.13.2 and
gitleaks 8.30.1 match the documented toolchain. Protobuf generators match the
committed generated headers (protoc-gen-go 1.36.12, protoc-gen-go-grpc 1.6.2).
Release checksums verify buf and the CI script's pinned tools. Go parallelism and
the fake-host harness shard count are both bounded to two. Action references
are immutable SHAs obtained from the official repositories' version tags.

## Validation

- `actionlint 1.7.11 .github/workflows/ci.yml`: passed, including ShellCheck of
  the embedded Bash.
- `tools/ci.sh check --base origin/main`: passed (contract, forbidden patterns,
  gitleaks, packet-trace ban, and slot scheme).
- Existing cloud baseline generation gate: passed with aligned generators.
- Existing cloud full quick gate: blocked by the execution environment. Unix
  socket creation/listening returns `EPERM`; the API fake-agent suites therefore
  fail before exercising product behavior. A minimal UDS listener under pinned
  Node 22.23.2 confirms the same restriction. No assertions were removed.
- The new workflow has not yet run on GitHub. A hosted run must end with the
  original `CI GATE PASSED` before any production merge claims a green gate.

No changes were made to `tools/ci.sh`, its checks, application code, generated
outputs, or the integration environment. GitHub-hosted execution is pending
publication and independent review.
