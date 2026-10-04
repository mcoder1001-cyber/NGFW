# R1 correctness and tests

Reviewed final branch head: `62535f7e18be3e41fcf0784aeecda7ef932bd785`; final product change `997d4fa4` (the difference is review documentation only).

No unresolved product correctness findings. Renderer, source failure preservation, successful empty withdrawal, source-specific/wildcard conversion, current-config mapping removal/remap/ambiguity, retry after transient sync failure, scoped descriptor isolation and global registration guards have regression coverage. Poller uses the scheduler seam, propagates context and does not hold its cache lock across synchronization. Live topology, daemon and VPP restart acceptance remain explicitly NOT RUN.

The preliminary retry-test coverage finding was resolved by testing an initial failed synchronization followed by success on the identical observation, without sleeps. Mapping-remap and duplicate-mapping tests were added.

Independent command at the reviewed head:

```text
go test -race -count=1 ./internal/frrsync/pim ./internal/renderers/frr/pim ./internal/descriptors/mfib ./internal/subsystems -run 'Test(Translate|FailedRead|Run|NamedDescriptor|Pim|Render|Interface|Source)'
ok ngfw/agent/internal/frrsync/pim 1.046s
ok ngfw/agent/internal/renderers/frr/pim 1.042s
ok ngfw/agent/internal/descriptors/mfib 1.028s
ok ngfw/agent/internal/subsystems 1.309s
```

An independent broader subsystem run also reproduced unrelated PBR failures: missing mandatory `acl.acl/lan-b` in `TestPBRPolicyNamesFACLList` and `TestRpfAdlPbrWithoutFAcl`. This is not a claim that the full suite passes.

Required `tools/ci.sh --base main`: BLOCKED-ENV. Installation, generation, forbidden-pattern, secret and slot guards passed. Turbo API tests reported 11 failed files/76 passed; 8 failed tests/507 passed/65 skipped. Failures include `listen EPERM` on temporary Unix gRPC sockets and `chown EINVAL` in JWT ring tests. The manager independently reproduced these failures on baseline. The gate was interrupted with Ctrl-C (exit 130) after the failed test process left lingering handles. No complete quick pass is claimed. Logs: `/tmp/ngfw-ci/pim-20261004-045443-2/08-turbo.log`. Hosted quick remains required before merge.

Verdict: APPROVE for product correctness at the stated head; integration remains conditional on the mandatory complete quick gate.

Final delta verified independently: runtime snapshots above 256 routes fail before cache replacement; the exact supported boundary and overflow retention are tested. Read/synchronization outcomes now produce transition-based outage/recovery events and warning logs without raw error payloads. Repeated failures do not spam diagnostics, and recovery requires a complete successful observation and synchronization. Context cancellation exits without a spurious outage event.

```text
go test -race -count=1 ./internal/frrsync/pim ./internal/subsystems -run 'Test(Pim|Run|SupportedSnapshot|Translate|FailedRead|Mapping)'
ok ngfw/agent/internal/frrsync/pim 1.146s
ok ngfw/agent/internal/subsystems 1.322s
```
