# CI-remote-codex — independent R4 capture-cleanup review

Reviewed manager-authored `98a24642fc1476d9f35903cf9420640a393bc70d`. Read R4 prompt, shared-host rules and handover facts. Reviewer made no product edits or commits.

No R4 findings. Changed code is unit-fixture cleanup only. No real VPP trace/capture command was added or executed, no generated binapi modified, no C/plugin code, system service, global lock, port or range changed. Test FakeVPP and files reside exclusively under t.TempDir and use existing w5/w6 ownership fixtures. Handover remains pending and untouched.

Cancellation and completion join are registered after the fixture TempDir creation and therefore execute before directory removal. The done channel closes after Manager.Run finishes cancellation, capture deletion and file finalization. Cleanup timeout fails the test. The agent-restart simulation's interrupted-state, packet-count and stopped-capture assertions are unchanged. This is safer unit fixture ownership, not real restart/packet acceptance evidence.

Independent pinned toolchain, GOMAXPROCS=2 GOFLAGS=-p=2:

```text
go test -race -run 'TestAgentRestartDuringCapture|TestBusyAndGlobals' -count=100 ./internal/actions/capture-trace
ok ngfw/agent/internal/actions/capture-trace 2.572s
```

Verdict: **APPROVE** (R4 test-only change). Full hosted gate remains a separate merge prerequisite.
