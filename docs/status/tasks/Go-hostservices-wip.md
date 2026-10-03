# Go host-services fixture — coherent correction, full gate running

Branch `codex/go-hostservices-fixture-20261003`; isolated `NGFW-go-hostservices`; base `a237827811a4abd2293157d5e8ea0cebf61196fc`.
Owned only `apps/agent/internal/agent/rpc_dns_test.go` and `docs/status/tasks/Go-hostservices-*`.
Local frozen source `b4ef518f1f5093b1c4eb0ed1046a7403704c4771`; initial envelope `237df828`, first path fix `1490b9d0`. Connector publication requested from root; remote acknowledgement pending. No production changes, live VPP, daemon starts/restarts, package installs, original/main/P10 or other worktree edits.

## Actual staged failure evidence

Fresh main-base targeted race run before any correction:
`go test -race -count=1 ./internal/agent -run '^TestHostServicesApplyRetrieveRollback$'`
FAIL (0.482s), rpc_dns_test.go:61 missing unbound/unbound.conf. Raw `/tmp/go-hostservices-before.log`.

Cause: fixture sets EnvHostServicesDir to `base`, then newSvc overrides it to hostDirOf(t,stateDir). First fix keeps one stateDir/hostDirOf pair and passes that same stateDir to newSvc; every apply/retrieve/file/state/idempotence/rollback assertion remains.

That path correction reached a second preexisting assertion failure: rollback retrieval returns `services:{dhcp:{} qos:{}}`, while fixture expected ENTIRE services=nil. FAIL (0.649s), rpc_dns_test.go:103. Raw `/tmp/go-hostservices-after-1.log`; second sequential attempt correctly did not run after failure. Related agent tests also failed that same assertion (`/tmp/go-hostservices-related.log`); its selected subsystem regex matched no tests, so no subsystem-test pass is claimed for that selection.

Root independently confirmed existing contracts: `desired/kea_test.go` requires empty services.dhcp present; `desired/qos_test.go` requires empty services.qos. Root explicitly authorized strict canonical expectation. The corrected rollback check uses proto.Equal to exactly `ServicesConfig{Dhcp:&DhcpService{},Qos:&QosService{}}`: ALL configured services, including DNS/NTP, must be absent; unexpected/nonempty configuration fails. Management=nil and idle Unbound render assertions are preserved. No production behavior or test skips changed.

## Actual corrected results

```text
tools/ci.sh check --base a237827811a4abd2293157d5e8ea0cebf61196fc
check PASSED (0m09s)
go test -race -count=1 ./internal/agent -run '^TestHostServicesApplyRetrieveRollback$' (attempt 1)
ok ngfw/agent/internal/agent 1.727s
go test -race -count=1 ./internal/agent -run '^TestHostServicesApplyRetrieveRollback$' (attempt 2)
ok ngfw/agent/internal/agent 1.902s
go test -race -count=1 ./internal/agent -run 'Test(HostServices|Syslog|DNS|Dns)'
ok ngfw/agent/internal/agent 1.883s
go test -race -count=1 ./internal/subsystems
ok ngfw/agent/internal/subsystems 22.550s
```

Corrected logs: `/tmp/go-hostservices-after-final-{1,2}.log`, `/tmp/go-hostservices-related-final.log`, `/tmp/go-hostservices-subsystems.log`. Existing real Unbound/chrony/rsyslog config-check tools ran read-only on private staged fixture files; no host service was started or signalled.

Unchanged full quick gate is NOW RUNNING on frozen source b4ef518f, log `/tmp/go-hostservices-quick.log`. No full-gate pass claimed before completion. Remaining: full local quick, exact-tree publication/PR and unchanged hosted gates, independent review; developer does not self-review or merge.
Next command: inspect `/tmp/go-hostservices-quick.log`, commit final raw result, ask root to publish exact final tree and create the reviewable PR from Go-hostservices-pr.md.
