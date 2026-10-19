# repair-211-lcp-20261019 WIP

- Branch: `codex/repair-211-lcp-20261019`.
- Base: installed appliance source `cb148aafb`.
- Product source / verified remote checkpoint: `98e5525c0b183586984557e673c95de2649f0f58`; CLI push succeeded and configured branch tracking.
- Published PR head: `cc1656787d3c9a8b6d851ced2dca8501209e1aff` (CLI push succeeded); product source remains `98e5525c0b183586984557e673c95de2649f0f58`. Subsequent documentation checkpoints use agent-owned remote checkpoint ref `codex/repair-211-lcp-20261019-checkpoints` so the unchanged mandatory hosted run can finish without every status update cancelling it. The development worktree remains on its assigned branch; no product changes or history rewriting are involved.
- Draft PR: https://github.com/mcoder1001-cyber/NGFW/pull/250 (created; parent attached it successfully).
- Owned files: `apps/agent/internal/subsystems/base_policy.go`, `base_policy_namespace.go`, `base_policy_namespace_test.go` and this task's envelope/WIP documents.
- Completed code: exact port of PR 249's three files from `44714075332aa75408924c524ae4a5d0f9e90aa5`; valid binary namespace reads retain existing behavior. Only invalid namespace bytes invoke fixed read-only `show lcp`; root is accepted only after one exact unset status with bounded UTF-8/control-character validation. Transport, malformed, named and ambiguous corroboration remain errors.
- Independent review: `routing_review` APPROVE on product source `98e5525c0b183586984557e673c95de2649f0f58`; no actionable source findings. Reviewer checked fixed command/context, preserved valid binary path, exact single unset confirmation, UTF-8/control/size validation and generated retval handling.

## Actual validation and artifact

- `GOMAXPROCS=2 GOFLAGS=-p=2 tools/ci.sh check --base cb148aafb`: passed in 13 seconds, including contract/pattern/secret/trace/slot checks.
- `GOMAXPROCS=2 go test -p=2 -count=1 ./internal/subsystems -run 'Test(BasePolicy|ProjectBasePolicy)'`: passed (`ok ngfw/agent/internal/subsystems 0.120s`). Repeated after Go generation completed: passed (`0.113s`).
- `GOMAXPROCS=2 go vet -p=2 ./internal/subsystems`: passed with exit 0, including repeat after Go generation completed.
- From `apps/agent/internal/subsystems`, `GOMAXPROCS=2 go test -p=2 -race -count=1 -v base_policy_namespace.go base_policy_namespace_test.go`: all 19 cases passed (`ok command-line-arguments 1.099s`). `go vet -p=2` on the same two files passed.
- Initial whole-package focused race run overlapped the quick gate's protobuf regeneration and failed because `gen/ngfw/v1/dataplane.pb.go` was temporarily absent. This was a scheduling error. After both generation passes completed, the unchanged whole-package race command was rerun and PASSED: `ok ngfw/agent/internal/subsystems 1.311s`.
- Stripped static build passed using Go 1.26.0: `GOMAXPROCS=2 CGO_ENABLED=0 go build -p=2 -trimpath -ldflags '-s -w -X main.version=repair-211-lcp-98e5525c0' -o /root/ngfw-wt/repair-211-lcp-20261019/artifacts/ngfw-agent-98e5525c0 ./cmd/ngfw-agent`.
- Artifact: `/root/ngfw-wt/repair-211-lcp-20261019/artifacts/ngfw-agent-98e5525c0`.
- SHA256: `86a65b51dfbd5da268533ebbaae3c5b2b2143549547a7ce9f1176c083b499f5c`.
- `file` confirms x86-64, statically linked, stripped; `go version -m` confirms go1.26.0, CGO_ENABLED=0. Generated Go, go.mod and go.sum matched product source after regeneration.
- Complete unchanged quick gate started with `GOMAXPROCS=2 GOFLAGS=-p=2 NGFW_CI_APPLY_SHARDS=2 NGFW_CI_TASK_CONCURRENCY=2 tools/ci.sh quick --base cb148aafb`; still running, not claimed passed. All 35 Turbo lint/typecheck/test/build tasks passed, including API 113 files / 823 tests and web 117 files / 694 tests. The complete agent vet, golangci-lint (`0 issues`), race suite (including full subsystem package: `33.716s`) and build passed. CLI lint/race/build and all 27 test modules in unit mode passed. Deployment shellcheck passed; the final apply-startup fake-host harness is running. Logs: `/tmp/ngfw-ci/repair-211-lcp-20261019-20261019-112011-3012212`.
- No live integration or appliance mutation by this worker. Parent owns appliance deployment/verification and preserves rollback evidence; no merge requested.

## Appliance receipt reported by the manager

The parent reports the reviewed artifact (`86a65b51dfbd5da268533ebbaae3c5b2b2143549547a7ce9f1176c083b499f5c`, source `98e5525c0`) installed on appliance 211, with old binary and state backed up at `/root/ngfw-route9-11-repair-20261019`. VPP PID remained `167929`, NRestarts remained `0`. Restarting the agent stopped its dependent API; the parent started the API, after which VPP, agent and API were active.

Post-repair validation returned HTTP 200 and exactly four cross-connect creates. Commit applied four operations successfully and was confirmed as revision 4 (transaction `9bec043f-d1a9-4808-8b24-ac0bfd15e04f`). The parent independently verified live 9↔11 and 5↔6 cross-connects, stored running configuration, empty diff, no pending confirmation and no observation errors. This is the parent's actual deployment receipt; this worker made no appliance changes. No policy, auth, environment or privilege edits were made. The parent later observed initial forwarded frame counts 5→6=1, 6→5=2, 9→11=2 and 11→9=2, with historical drop counters unchanged. These were sparse 70-byte frames, not sustained tester acceptance; a tester stream had not arrived. A subsequent parent follow-up measured no ingress over 39/44 seconds: physical ports 9/11 were DOWN while 5/6 were UP, with all four admin states UP and the cross-connect configuration preserved. Historical tx-errors (5=4, 11=3) were interface-down errors and did not increase during the idle interval. The parent is following up on tester/cable changes; no configuration changes were made. The earlier sparse-frame receipt is historical only. Sustained packet-level acceptance is still pending and is not attributed to this namespace repair.

## Remaining work and recovery

- Remaining: finish complete quick gate and publish final actual test receipt. Focused unit/race/vet/build are complete.
- Current failure: none in focused verification; the initial race scheduling failure was resolved by a successful repeat after generation.
- Exact next command: inspect `/tmp/ngfw-ci/repair-211-lcp-20261019-20261019-112011-3012212/09-turbo.log` and the running quick gate process; then publish its actual result. Hosted mandatory run `39246565687` on PR head `cc1656787` is in progress. Earlier runs were cancelled by normal branch concurrency after documentation pushes. Keep the PR head stable while this run completes; publish status-only recovery updates to the agent-owned checkpoint ref and link their immutable source in the PR description.
