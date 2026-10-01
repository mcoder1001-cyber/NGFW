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
- Draft PR #57 publishes this workflow. Hosted run 36923948061 is in progress. A hosted run must end with the
  original `CI GATE PASSED` before any production merge claims a green gate.

No changes were made to `tools/ci.sh`, its checks, application code, generated
outputs, or the integration environment. GitHub-hosted execution is pending
a green hosted run and completion of independent review.

## Pasted verification evidence (2026-10-01)

Reviewed workflow product commit: `131c3aaf`; uploaded tree includes unchanged workflow at remote SHA `46d95cb61cd8c9027620e955356723d5cc12ae8a`.

```text
$ actionlint .github/workflows/ci.yml
(no output; exit status 0)
$ node --version
v22.23.2
$ pnpm --version
12.5.1
$ go version
go version go1.26.0 linux/amd64
```

```text

== contract guard: HEAD vs origin/main ==
no contract files changed in the 6 commit(s) of HEAD since origin/main (5561d041)

== forbidden patterns (+ gitleaks) ==
WARN control-plane lines exempted with 'ALLOW:' — reviewer, check each justification:
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:1:import { execFileSync } from 'node:child_process'; // ALLOW: test-only openssl cert
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:13:    execFileSync('openssl', ['version'], { stdio: 'ignore' }); // ALLOW: test-only openssl cert
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:20:const openssl = (args: string[]) => execFileSync('openssl', args, { stdio: 'ignore' }); // ALLOW: test cert
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: vrxtestsecrets only in test code
ok: gitleaks — scanned ~25412 bytes (25.41 KB) in 230ms no leaks found 

== packet-trace ban on the shared VPP (D-128) ==
ok: no packet trace (trace add / show trace / clear trace / tracedump API) outside docs and the generated bindings

== slot resource scheme (1..32, no collisions) ==
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m03s)
```

Baseline source SHA: `01bbee3d` (documentation-only changes over `5561d041`). The first baseline used Node 24.19.0, not the later installed pinned Node 22.23.2; it is not asserted as an exact pinned full-gate pass. Actual baseline output:

```text
== VRX CI gate: quick ==
worktree  /workspace/scratch/96b8b6fbc8a7/NGFW
branch    main @ 01bbee3d   (base: origin/main)
tools     node v24.19.0 · pnpm 12.5.1 · go1.26.0 · buf 1.73.0 · golangci-lint 2.13.2 (pinned) · gitleaks 8.30.1 (pinned)
caches    pnpm store /root/.local/share/pnpm/store/v11 · turbo /root/.cache/vrx-turbo · go /root/.cache/go-build
logs      /tmp/ngfw-baseline-ci/NGFW-20261001-153507-6

== contract guard: HEAD vs origin/main ==
no contract files changed in the 1 commit(s) of HEAD since origin/main (5561d041)

== tools (golangci-lint, gitleaks) ==
golangci-lint 2.13.2
gitleaks 8.30.1

== install (pnpm --frozen-lockfile --prefer-offline) ==
Lockfile is up to date, resolution step is skipped Done in 38ms using pnpm v12.5.1 

== generate + generated-output gate ==
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated

```

Pinned runtime Unix-socket reproduction:

```text
@ngfw/api:test:      → No address added out of total 1 resolved errors: [listen EPERM: operation not permitted /tmp/vrx-td10a-h1-Zn48dL/agent.sock]
@ngfw/api:test:      → No address added out of total 1 resolved errors: [listen EPERM: operation not permitted /tmp/vrx-td23-CmvJir/agent.sock]
@ngfw/api:test:      → No address added out of total 1 resolved errors: [listen EPERM: operation not permitted /tmp/vrx-td23-CmvJir/agent.sock]
@ngfw/api:test:      → No address added out of total 1 resolved errors: [listen EPERM: operation not permitted /tmp/vrx-td23-CmvJir/agent.sock]
@ngfw/api:test:      → No address added out of total 1 resolved errors: [listen EPERM: operation not permitted /tmp/vrx-td23-CmvJir/agent.sock]
```

The preceding lines are the baseline API failure; no quick-gate success is claimed. A separate minimal Node 22.23.2 Unix listener also fails as recorded below.

```text
$ node Unix-listener reproduction (v22.23.2)
(node:299) [UNDICI-EHPA] Warning: EnvHttpProxyAgent is experimental, expect them to change at any time.
(Use `node --trace-warnings ...` to show where the warning was created)
EPERM: listen EPERM: operation not permitted /tmp/ngfw-ci-evidence.sock
exit status: 1
```
