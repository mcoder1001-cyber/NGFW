# F-pim-frrsync — R4 dataplane/shared-host review

Reviewed exact local source SHA `1025cae41053c5b207754b0496798fcd1f32b514` against base `06e4368c`. Independent reviewer; no product edits.

No BLOCKER, MAJOR or MINOR source findings.

Verified named `mfib.route.pim` keys and durable boot records isolate static and dynamic families; existing-prefix Create refuses takeover, Delete requires the current-boot family record, and Retrieve cannot adopt static routes. Messages remain generated `apps/agent/binapi/ip` APIs, unchanged. Poller uses S1 exclusively, translates the configured LCP map, suppresses removed/ambiguous dependencies and disabled PIM, preserves snapshots on read failure, withdraws successful empty observations, and retries transient sync failures. Default-table programming requires an explicit all-ID product range; numbered shared-host slots register no source. Child integration uses frrtest-owned prefixed namespace, processes and cleanup, with integration opt-in and lab lock; no system unit/VPP restart/global mutation introduced.

Commands run in this worktree's `apps/agent`:

```text
/workspace/scratch/e4f791ef53f7/go/bin/go test -race -count=1 ./internal/descriptors/mfib ./internal/frrsync/pim ./internal/renderers/frr/pim
ok ngfw/agent/internal/descriptors/mfib 1.081s
ok ngfw/agent/internal/frrsync/pim 1.045s
ok ngfw/agent/internal/renderers/frr/pim 1.132s
/workspace/scratch/e4f791ef53f7/go/bin/go test -race -count=1 ./internal/subsystems -run Pim
ok ngfw/agent/internal/subsystems 1.328s
```

Earlier whole-subsystems run failed existing `TestRpfAdlPbr*` ACL dependency tests; this is not a whole gate pass. No real FRR/VPP packet, daemon or restart test ran here. Those lab-only acceptance items remain deferred by the task/owner policy, not represented as passing.

Verdict: **APPROVE** (source review; lab acceptance deferred).

Final delta review: `997d4fa4b585608ee6c76767cd0cee1b3aed8017` retains ownership/API/host boundaries. The 256-record cap rejects overflow before cache replacement; the new status reporter emits safe failure/recovery events only on transitions and honors cancellation. Independent final `go test -race -count=1 ./internal/frrsync/pim` PASS 1.157s and `go test -race -count=1 ./internal/subsystems -run Pim` PASS 1.085s. Verdict remains **APPROVE**.
