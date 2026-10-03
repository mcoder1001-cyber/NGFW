# Go host-services fixture — frozen correction, complete local quick PASS

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

Unchanged COMPLETE local quick gate PASSED on frozen source b4ef518f with documentation checkpoint df29183f, exit 0:

```text
tools/ci.sh --base a237827811a4abd2293157d5e8ea0cebf61196fc
35 turbo tasks successful (28 cached, 56.606s)
ngfw/agent/internal/agent 46.385s — PASS
agent lint/race/build, CLI lint/test/build and every test-module unit/compile/vet stage — PASS
deploy/vpp shellcheck + preserved fake-host apply-startup shards — 33/29/26/61 assertions PASS
mode quick · wall time 10m23s
CI GATE PASSED
```

Raw quick log `/tmp/go-hostservices-quick.log`; detailed logs `/root/ngfw-wt/logs/ci/NGFW-go-hostservices-20261003-133014-396580`. The VPP counters belong to fake-host fixtures; no host VPP/service restart occurred. NGFW_INTEGRATION remained unset, so real lab integration is NOT claimed. All spawned test/gate processes ended; worktree clean before this documentation-only checkpoint.

Current code failure: none. Remaining: exact-tree remote publication/PR acknowledgement, unchanged hosted gate results and independent review. If main changes, root must validate the final integration tree; developer does not self-review or merge. No production/test-helper redesign, new skip, relaxed nonempty configuration acceptance or CI change was introduced.
Next command: `git rev-parse HEAD && git rev-parse HEAD^{tree}` after final documentation commit; root connector-publishes that exact tree and creates the reviewable PR from Go-hostservices-pr.md, then checks final published-head CI and independent review before guarded integration.
