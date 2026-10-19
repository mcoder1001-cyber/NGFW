# repair-211-lcp-20261019 WIP

- Branch: `codex/repair-211-lcp-20261019`.
- Base: installed appliance source `cb148aafb`.
- Product source / verified remote checkpoint: `98e5525c0b183586984557e673c95de2649f0f58`; CLI push succeeded and configured branch tracking.
- Local HEAD before this evidence checkpoint: `98e5525c0b183586984557e673c95de2649f0f58`; this receipt changes documentation only.
- Draft PR: https://github.com/mcoder1001-cyber/NGFW/pull/250 (created; parent attached it successfully).
- Owned files: `apps/agent/internal/subsystems/base_policy.go`, `base_policy_namespace.go`, `base_policy_namespace_test.go` and this task's envelope/WIP documents.
- Completed code: exact port of PR 249's three files from `44714075332aa75408924c524ae4a5d0f9e90aa5`; valid binary namespace reads retain existing behavior. Only invalid namespace bytes invoke fixed read-only `show lcp`; root is accepted only after one exact unset status with bounded UTF-8/control-character validation. Transport, malformed, named and ambiguous corroboration remain errors.
- Independent review: `routing_review` APPROVE on product source `98e5525c0b183586984557e673c95de2649f0f58`; no actionable source findings. Reviewer checked fixed command/context, preserved valid binary path, exact single unset confirmation, UTF-8/control/size validation and generated retval handling.

## Actual validation and artifact

- `GOMAXPROCS=2 GOFLAGS=-p=2 tools/ci.sh check --base cb148aafb`: passed in 13 seconds, including contract/pattern/secret/trace/slot checks.
- `GOMAXPROCS=2 go test -p=2 -count=1 ./internal/subsystems -run 'Test(BasePolicy|ProjectBasePolicy)'`: passed (`ok ngfw/agent/internal/subsystems 0.120s`). Repeated after Go generation completed: passed (`0.113s`).
- `GOMAXPROCS=2 go vet -p=2 ./internal/subsystems`: passed with exit 0, including repeat after Go generation completed.
- From `apps/agent/internal/subsystems`, `GOMAXPROCS=2 go test -p=2 -race -count=1 -v base_policy_namespace.go base_policy_namespace_test.go`: all 19 cases passed (`ok command-line-arguments 1.099s`). `go vet -p=2` on the same two files passed.
- Initial whole-package focused race run overlapped the quick gate's protobuf regeneration and failed because `gen/ngfw/v1/dataplane.pb.go` was temporarily absent. This was a scheduling error; rerun after generation remains required, not claimed passing.
- Stripped static build passed using Go 1.26.0: `GOMAXPROCS=2 CGO_ENABLED=0 go build -p=2 -trimpath -ldflags '-s -w -X main.version=repair-211-lcp-98e5525c0' -o /root/ngfw-wt/repair-211-lcp-20261019/artifacts/ngfw-agent-98e5525c0 ./cmd/ngfw-agent`.
- Artifact: `/root/ngfw-wt/repair-211-lcp-20261019/artifacts/ngfw-agent-98e5525c0`.
- SHA256: `86a65b51dfbd5da268533ebbaae3c5b2b2143549547a7ce9f1176c083b499f5c`.
- `file` confirms x86-64, statically linked, stripped; `go version -m` confirms go1.26.0, CGO_ENABLED=0. Generated Go, go.mod and go.sum matched product source after regeneration.
- Complete unchanged quick gate started with `GOMAXPROCS=2 GOFLAGS=-p=2 NGFW_CI_APPLY_SHARDS=2 NGFW_CI_TASK_CONCURRENCY=2 tools/ci.sh quick --base cb148aafb`; still running, not claimed passed. Logs: `/tmp/ngfw-ci/repair-211-lcp-20261019-20261019-112011-3012212`.
- No live integration or appliance mutation by this worker. Parent owns appliance deployment/verification and preserves rollback evidence; no merge requested.

## Remaining work and recovery

- Remaining: finish complete quick gate, rerun affected whole-package race after generation completes, publish final actual test receipt.
- Current failure: initial race run hit temporary generated-file absence; no product code failure identified by focused verification.
- Exact next command after generation completes: `GOMAXPROCS=2 go test -p=2 -race -count=1 ./internal/subsystems -run 'Test(BasePolicy|ProjectBasePolicy)'` from `apps/agent`.
