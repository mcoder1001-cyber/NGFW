# CI-remote-codex — R8 operability and packaging review

Scope: workflow implementation `131c3aaf88a6b56603ba9c7cb48916877b14a830`, inspected at branch HEAD `d6b38ca33527c71b23de3a701bbf838a3cdcc3bf`. Read shared context, REVIEW-PROMPT and R8 prompt. The manager envelope authorizes this worktree and hosted CI, superseding historical local-only instructions. No product edits or full local quick execution were performed.

## Findings

No BLOCKER or MAJOR findings in the workflow's operability scope. Hosted execution remains a separate merge prerequisite.

1. **MINOR — .github/workflows/ci.yml:44–62, 80–85 — bootstrap failures do not produce the uploaded gate artifact.** `install-tools` dispatches without `init_logs`, and npm/apt/Buf/plugin installation runs directly. Failure before the gate leaves `runner.temp/ngfw-ci` absent, so artifact upload warns instead of preserving bootstrap output. GitHub step console output still records the failure, making it diagnosable. Optional fix: tee bootstrap output into a dedicated file under the upload directory with pipefail preserved.

## Operability checks

- Ubuntu 24.04 hosted x86_64 selection matches Buf Linux-x86_64 and the install-tools amd64 selection. Quick requires ordinary shell/build utilities, Python 3 for fake UNIX sockets, shellcheck, Node, pnpm, Go, Buf and the two Go protobuf plugins. The workflow explicitly installs the non-image tool dependencies. No appliance VPP, PostgreSQL, Valkey or real systemd service execution is requested by quick. The startup harness injects fake systemctl/ip/sysfs and temporary paths; it still needs ordinary UNIX socket capability, which this local restricted runtime lacks.
- Go 1.26.0 matches `apps/agent/go.mod`'s Go 1.26 requirement. GOTOOLCHAIN=local prevents unnoticed fallback toolchain downloads. Node 22.23.2 meets the root >=22 engine; pnpm 12.5.1 matches packageManager. Go protobuf plugin 1.36.12 matches the protobuf module. Buf configuration uses local protoc-gen-go, protoc-gen-go-grpc and workspace-installed ts-proto; both Go plugins are placed on the persisted job PATH. Buf handles .proto parsing without a separately invoked protoc executable. Frozen-lockfile installation precedes pnpm gen.
- install-tools pins golangci-lint 2.13.2 and gitleaks 8.30.1 and validates selected release archive checksums before extraction/install. The workflow additionally validates the exact Buf asset; missing/mismatched checksum lines fail under pipefail. Release availability and linter/toolchain compatibility require actual hosted execution; static version matching does not prove them.
- Full checkout history supports origin/base comparison and contract checks; PR base refs enter shell through quoted array arguments. The workflow runs the unchanged quick dispatch, including lint/typecheck/test/build, generation dirty check, Go race tests and fake-host startup harness. No missing-tool bypass is set.
- GOMAXPROCS=2, GOFLAGS=-p=2 and two startup shards limit Go build/test and harness pressure. Turbo retains its repository default concurrency, so peak memory and cold-run duration must be observed in hosted execution rather than inferred green. The 45-minute job limit bounds execution and download stalls; it is deliberately larger than the historical warm-cache quick budget. No integration/lab lock or live host restart is triggered.
- Per-ref cancellation prevents obsolete runs continuing indefinitely; PR and push refs are separate groups and can both execute. Gate failure output includes the last 60 log lines; complete step logs upload with always(), 14-day retention and run-attempt-specific names. Cancellation or whole-job timeout may prevent final upload; GitHub console output remains the primary evidence in that case.

## Independent evidence

Commands ran in `/workspace/scratch/96b8b6fbc8a7/NGFW-ci`, not the historical host worktree. Ran individually:

```text
$ PATH=/workspace/scratch/96b8b6fbc8a7/toolchain/bin:$PATH actionlint .github/workflows/ci.yml
[no output; exit 0]
$ bash -n tools/ci.sh
[no output; exit 0]
```

Inspected workflow diff against origin/main, tools/ci.sh installation/dispatch/logging/generation paths, both Go Makefiles, agent go.mod and proto generator/config/package definitions. These are syntax/static checks only. Hosted run `36923948061` was reported in progress by the manager; this review neither fetched its conclusion nor claims hosted PASS, independent full quick PASS or appliance acceptance.

## Minor disposition verification

The manager accepts the bootstrap-artifact improvement as optional; existing GitHub console output preserves bootstrap failure diagnostics. No MAJOR fix was requested or agreed, and this MINOR does not block merge. This verification changes only the verdict disposition; no full gate was rerun.

Verdict: **APPROVE** (one accepted optional MINOR; no merge-blocking R8 finding). Merge still requires successful hosted tester evidence and resolution of other mandatory reviewers' findings.


## Scheduling-change verification — b2534a63

Reviewed `b2534a63bc807942970d713e95d33427c17eaad6`: the workflow adds VRX_CI_TASK_CONCURRENCY=2; do_turbo validates an optional integer 1..64 and supplies a quoted `--concurrency` argument pair. Unset or empty retains the original command. The diff changes no requested tasks, test assertions, test timeout, continue behavior, integration policy or job timeout. No new R8 findings.

The task report records original hosted run 36923948061 as failed (34/35 Turbo tasks succeeded; UI-kit 89/90 tests passed, one 30-second timeout). This supports investigating scheduling contention; it does not prove CPU contention is the sole cause. Two simultaneous Turbo tasks align better with the two-core resource budget and complement the existing Go/harness limits, but individual tasks may still create workers. Adequate memory, runtime and resolution of the UI timeout need a successful hosted rerun. R8 approves the scheduling control, not an unobserved green outcome.

Independent verification in the same worktree: extracted the actual do_turbo function into a Bash harness with mock step/run/note/fail functions, to inspect argv without executing Turbo or the gate. The full lint/typecheck/test/build task list remains present:

```text
UNSET exit=0 <turbo><pnpm><turbo><run><lint><typecheck><test><build><--continue><--output-logs=errors-only>
1 exit=0 [same argv plus] <--concurrency><1>
2 exit=0 [same argv plus] <--concurrency><2>
64 exit=0 [same argv plus] <--concurrency><64>
0 exit=7 VRX_CI_TASK_CONCURRENCY must be an integer from 1 to 64
65 exit=7 VRX_CI_TASK_CONCURRENCY must be an integer from 1 to 64
02 exit=7 VRX_CI_TASK_CONCURRENCY must be an integer from 1 to 64
2; echo injected exit=7 VRX_CI_TASK_CONCURRENCY must be an integer from 1 to 64
$ PATH=/workspace/scratch/96b8b6fbc8a7/toolchain/bin:$PATH actionlint .github/workflows/ci.yml
[no output; exit 0]
$ bash -n tools/ci.sh
[no output; exit 0]
```

Scheduling-change verdict: **APPROVE**. Existing optional bootstrap-log MINOR remains accepted. No full local gate ran; hosted rerun evidence remains required before merge.
