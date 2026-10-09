# FRR slot lock cleanup — independent T1 hosted verification

Date: 2026-10-08. Frozen published PR209 head: `089c09c43d28001b199679c76e79b2992d137167`. No product, workflow or gate edits by tester. No local Go/race/full quick execution: Go unavailable.

Latest exact-head hosted [CI gate run37807567473](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37807567473), job `113416081131`, is in progress. The unchanged `.github/workflows/ci.yml` executes `tools/ci.sh quick --base origin/main`; agent Makefile test target runs `go test -race -count=1 ./...`, which includes the new offline FRR regressions. Source inclusion alone is not executed test evidence.

Required regressions: `TestFailedBaseResetRetainsSlotCleanup`, `TestRootFRRContendedLockClosesDescriptors`, `TestRootFRRCleanupWithoutSlotPreservesHolderFiles`. Startup predicates/deadlines remain outside this change. Completed latest-run gate and agent log evidence are outstanding; older source-equivalent runs cannot provide final PASS.

| Scenario | Expected | Observed | Verdict |
|---|---|---|---|
| Mandatory unchanged quick | Complete gate success | Latest exact-head hosted run in progress | PENDING |
| Race-enabled FRR regressions | Required cases execute and pass | Not yet available from completed latest-run logs | PENDING |
| Native FRR startup/200-route acceptance | Actual native evidence | Not run in this task | NOT RUN |

Verdict: FAIL for frozen head089c09c4 — actual quick lint failure, merge blocked; compiled/race evidence still outstanding. This file is source-safe tester evidence only, uncommitted pending manager authorization.

Final handoff: job113416081131 completed toolchain/setup successfully and remains in Run repository gate, no conclusion, at final observation. Manager requested observer stop for owner handoff; no workflow cancellation, persistent observer or automation created. Resume with latest exact-head jobs/logs and agent artifact; require actual race-enabled regression output and successful complete `CI GATE PASSED` before final PASS.

## Resumed completed-run diagnosis

Decoded latest exact-head hosted job113416081131 confirms actual lint failure:

```text
2026-10-08T16:30:18.9212604Z CI GATE FAILED — apps/agent lint/test/build failed
2026-10-08T16:30:18.9246236Z ##[error]internal/agent/ospf_topology_integration_test.go:245:21: G304: Potential file inclusion via variable (gosec)
2026-10-08T16:30:18.9256851Z ##[error]internal/renderers/frr/frrtest/harness_test.go:19:15: G304: Potential file inclusion via variable (gosec)
2026-10-08T16:30:18.9259189Z ##[error]internal/renderers/frr/frrtest/harness_test.go:37:20: G304: Potential file inclusion via variable (gosec)
2026-10-08T16:30:18.9261024Z * gosec: 3
```

Root handed findings to developer for source correction without weakening gate. No independent second execution yet; this is an observed hosted FAIL, not a reproduced twice claim. Require complete unchanged quick and race regression evidence on replacement head and current integration tree. Historical pending rows above are superseded by this completed failure.

Replacement PR209 head `9e4c78fe29b716cbe8a81d1205e1de759f501d81` is one commit above TD19-merged main `417e8fcd0c82fae7906da0fba1c0622a8de017f7`. Fresh unchanged mandatory quick run `37812533974` / job `113432780425` is in progress. Prior head FAIL does not establish final replacement verdict: replacement T1 PENDING until actual complete quick and race evidence.

Owner frequency override: stop CI observation and do not trigger/rerun complete CI until all tasks complete, then once. Existing runs were started before this steering. Last observation: replacement quick37812533974/job113432780425 remains repository-gate in progress without conclusion. Final replacement compiled/race verdict remains PENDING. No workflow was triggered or rerun by T1; no report was committed/published.
