# F-dashboard-prom-alarms-host — R2 independent security review

Reviewed HEAD `35c4533e723352c918bb8e842a9155e59c4d6bed` against `origin/main`; REVIEW-PROMPT and R2-security instructions read.

## Findings
No confirmed R2 security defect in reviewed changes. Listener allowlist uses socket RemoteAddr, not spoofable forwarded headers; malformed CIDRs reject; direct gRPC listen/port input validated. Empty allowlist intentionally permits any reachable peer under existing management contract, so deployment must explicitly configure suitable restrictions. Same-address update replaces handler; stop times out then closes active requests; source closes before listeners. Metrics contain counters, not secrets. No new API auth route.

Targeted command in apps/agent, toolchain PATH, GOMAXPROCS=2 GOFLAGS=-p=2:
```text
go test ./internal/promexport -run 'Test.*(Allow|Handler)' -count=1
ok ngfw/agent/internal/promexport 0.020s
```

Verdict: **APPROVE** (code security only).

## Limits and evidence
Status-file secret scan: `toolchain/bin/gitleaks detect --no-git --redact -s <worktree>/docs/status/tasks --no-banner` returned `no leaks found` for this worktree. This is limited to status records; source secret handling assessed statically. No whole quick/full gate or live integration rerun. Full appliance/VPP/daemon/browser acceptance remains pending; this verdict assesses code security and must not mark the feature completed.
