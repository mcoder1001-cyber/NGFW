# Go host-services fixture — WIP

Branch `codex/go-hostservices-fixture-20261003`; isolated worktree `NGFW-go-hostservices`; base `a237827811a4abd2293157d5e8ea0cebf61196fc`.
Owned only `apps/agent/internal/agent/rpc_dns_test.go` and `docs/status/tasks/Go-hostservices-*`.
Root-provided envelope read first; AGENTS/shared policy read. No production changes, daemon ownership, live VPP, service operation, original/main or P10 worktree edits.

Diagnosis: TestHostServicesApplyRetrieveRollback selects `base` via t.Setenv, then newSvc unconditionally selects hostDirOf(t,stateDir), so files render into another private path and assertions look in the wrong directory. Fix will keep one stateDir/hostDirOf pair and every existing apply/retrieve/state/idempotence/rollback assertion. No tools hidden, assertions weakened or skips introduced.

Completed: policy/envelope and code-path read. Standard own-worktree pnpm dependency preparation/generation running, log `/tmp/go-hostservices-prepare.log`.
Tests: previous independent P10-worktree reproduction failed as expected; this worktree's pre-fix reproduction still required. No test pass claimed.
Remaining: fresh targeted pre-fix race failure; fixture correction; two corrected targeted race runs; relevant host-services tests; unchanged complete quick; publication/PR and independent review.
Remote publication: pending; initial local SHA obtained with git rev-parse HEAD after checkpoint commit.
Next command: after standard generation completes, `cd apps/agent && go test -race -count=1 ./internal/agent -run '^TestHostServicesApplyRetrieveRollback$'`.
