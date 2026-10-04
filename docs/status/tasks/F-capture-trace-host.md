# F-capture-trace-host — acceptance source, 2026-10-03

Implemented real HTTP authentication/authorization, audit persistence, binary download and lifecycle e2e using the repository's fake-agent harness. Added real-VPP persisted-boot recovery test and a rig packet acceptance runner. The runner demands a dedicated per-slot VPP/socket/systemd unit (shared dispatch capture is banned), TD-3 preflight and slot ownership; it checks busy/validation, stop, real packet count, file hash/size, single-link 0600, tcpdump, temporary-file removal, DELETE/404 and both shared/dedicated NRestarts. Nine runner tests cover error transport and fail-closed safety gates.

Actual verification:

```text
python3 -m unittest discover -s test/topology/capture-trace -p 'test_*.py'
Ran 9 tests in 0.003s — OK

go -C apps/agent test ./internal/agent -run TestCapture -count=1
ok ngfw/agent/internal/agent 0.025s
(host acceptance test skipped in unit mode; not real VPP evidence)

pnpm --filter @ngfw/api exec eslint test/e2e/capture-trace.e2e.test.ts
exit 0 (existing root package module-type warning)

tools/ci.sh check --base main
check PASSED (0m03s)

tools/ci.sh --base main
Initial attempts: pinned buf executable provisioning incomplete (exit126 Permission denied at proto generation).
Retried with SHA-verified pinned buf: generation/check passed; full Turbo encountered
11 API test-suite failures from Unix-socket listen EPERM and dependent cleanup errors
(75 API suites passed, 502 tests passed). Gate is NOT PASSED; remaining Turbo was still
running at this checkpoint. No checks disabled or source weakened.
Generated outputs unchanged after successful generation.

pnpm --filter @ngfw/api typecheck
exit 0

NGFW_TEST_PREFIX=w5 NGFW_VALKEY_DB=5 pnpm --filter @ngfw/api exec vitest run -c vitest.e2e.config.ts test/e2e/capture-trace.e2e.test.ts
Global setup blocked: pg-test cannot reach PostgreSQL as admin. No e2e assertions executed.

NGFW_INTEGRATION=1 go -C apps/agent test -v ./internal/agent -run '^TestCaptureInterruptedRecoveryOnHost$' -count=1
SKIP: dedicated per-slot VPP not provisioned: capture host acceptance NOT RUN
ok ngfw/agent/internal/agent 0.063s (skip is not host acceptance)
```

NOT RUN: actual API e2e (no PostgreSQL/Valkey services), actual VPP recovery/rig packets (no VPP socket), BPF globals and actual UI T4 screenshot. These require the provisioned dedicated-VPP lab described in `test/topology/capture-trace/README.md`. No packet, host or UI acceptance claimed; no board change, deployment or merge.

Publication: initial CLI push automatically rejected unverified destination/source export. Manager subsequently verified the exact GitHub repository `mcoder1001-cyber/NGFW` is public and has admin/push permissions, and local origin matches the user-selected repository. Branch-history gitleaks scan clean. Connector publication succeeded to that verified same repository: PR137, checkpoint `881c1b5f0293bfc2bb5868283f6baab58b7d2ff8`, source tree matching local `552f7201`. No private-source export to a new destination. Independent source review approved; mandatory complete green gate remains required before integration.
