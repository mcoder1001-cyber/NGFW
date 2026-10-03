# Go host-services fixture correction envelope

Developer: reassigned P10 developer; branch `codex/go-hostservices-fixture-20261003`.
Isolated worktree: NGFW-go-hostservices; base `a237827811a4abd2293157d5e8ea0cebf61196fc` current remote main.
Owned: `apps/agent/internal/agent/rpc_dns_test.go` and `docs/status/tasks/Go-hostservices-*` only.
Slot 1; no daemon ownership or integration assigned. No production code, test helper redesign, original/main worktree, board, schema/generated or other developers' files.

Real local complete quick gate fails TestHostServicesApplyRetrieveRollback: rpc_dns_test.go61 expects unbound/unbound.conf in one temp root, but newSvc unconditionally selects hostDirOf(t,stateDir). Reproduced independently by developer and diagnosed by root. Correct the fixture to use the same isolated stateDir/hostDirOf pair, preserving all apply, retrieve, idempotence, state and rollback assertions. Never skip the case, hide binaries or weaken checks.

Read AGENTS.md/shared context and review/test policy. Reproduce targeted test before correction, then run corrected targeted race test twice and relevant host service package checks; unchanged complete quick remains required. No host daemon starts/restarts, live VPP, installs or protected path changes. Existing real config-check binaries only run read-only on private staged files. Commit coherent changes immediately, unique WIP records real outputs and remote/local SHAs. If CLI push lacks credentials root publishes exact tree via connector. Developer never self-reviews or merges.
