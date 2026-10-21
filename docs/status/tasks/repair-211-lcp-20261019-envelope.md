# repair-211-lcp-20261019 task envelope

- Date: 2026-10-19.
- Manager: parent appliance configuration agent; worker: lcp_repair.
- Branch: `codex/repair-211-lcp-20261019`.
- Worktree: `/root/ngfw-wt/repair-211-lcp-20261019`.
- Base: installed appliance source `cb148aafb` (main).
- Ownership: `apps/agent/internal/subsystems/base_policy.go`, `base_policy_namespace.go`, `base_policy_namespace_test.go` and this task's envelope/WIP files only.
- Task: port the reviewed strict LCP namespace corroboration from `44714075332aa75408924c524ae4a5d0f9e90aa5` (PR 249) onto the newer installed source, preserving current main behavior. This prerequisite unblocks persistent configuration validation for the user's initial port 9/11 layer-2 test.
- Constraints: no VPP, host privilege, sandbox, auth or schema changes; no appliance mutations; no main merge. Manager owns independent review and any deployment.
- Verification: focused namespace/base-policy tests, race tests, vet, unchanged complete quick gate, and stripped static agent build using Go 1.26.0. Record actual failures; do not weaken checks or fix unrelated scope silently.
- Publication: checkpoint coherent changes immediately to the named GitHub branch; record publication outcome and source/artifact identity for the manager.
- Host slot: none; unit tests and build only; no live lab objects or daemons.
