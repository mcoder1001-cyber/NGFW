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
NOT PASSED: pinned buf executable provisioning incomplete (exit126 Permission denied at proto generation).
Generated deletions from the failed generator were restored; no generated-source changes intended.
```

NOT RUN: actual API e2e (no PostgreSQL/Valkey services), actual VPP recovery/rig packets (no VPP socket), BPF globals and actual UI T4 screenshot. These require the provisioned dedicated-VPP lab described in `test/topology/capture-trace/README.md`. No packet, host or UI acceptance claimed; no board change, deployment or merge.

Publication: initial CLI push automatically rejected unverified destination/source export. Manager subsequently verified the exact GitHub repository `mcoder1001-cyber/NGFW` is public and has admin/push permissions, and local origin matches the user-selected repository. Branch-history gitleaks scan clean. Connector publication is pending these verified same-repository checks, not a private-source export to a new destination.
