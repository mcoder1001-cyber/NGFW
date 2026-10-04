# F-pim-frrsync — R8 operability review

Exact final SHA `997d4fa4b585608ee6c76767cd0cee1b3aed8017`, base `06e4368c`. Independent review; no product edits.

Initial MAJOR resolved: observation/parser/scale/scheduler failures previously appeared only in Debug logs. Final code emits one safe, readable existing ERROR event and structured Warn per degraded transition; complete successful read plus sync emits a recovery event/Info. Repeated failures do not flood diagnostics, raw daemon/error payloads are withheld, and intentional cancellation emits no outage. Regression covers failure/recovery/duplicate transitions and cancel behavior. No remaining BLOCKER/MAJOR/MINOR findings.

Production registration uses the established FRR runtime and S1 source lifecycle. Poll context has a deadline, ticker closes on cancellation, successful reads retry failed sync, failed/overflow reads preserve supported state, configuration suppression withdraws dependencies, and durable MFIB retrieval/reconnect supports restart recovery. The opt-in child harness uses prefixed frrtest namespace/PIDs/cleanup and no system unit. Operator docs give fixed FRR/VPP diagnostics, default-VRF/global-owner limits, 256-record cap, scale debt, restart/packet lab acceptance and actual gate/environment limitations. No new package dependency/migration/installer/service/CI mutation.

Independent exact-final commands in `pim/apps/agent`:

```text
/workspace/scratch/e4f791ef53f7/go/bin/go test -race -count=1 ./internal/frrsync/pim
ok ngfw/agent/internal/frrsync/pim 1.157s
/workspace/scratch/e4f791ef53f7/go/bin/go test -race -count=1 ./internal/subsystems -run Pim
ok ngfw/agent/internal/subsystems 1.085s
```

Real FRR/VPP forwarding/restart acceptance NOTRUN and lab-deferred; no whole CI gate pass claimed.

Verdict: **APPROVE**.
