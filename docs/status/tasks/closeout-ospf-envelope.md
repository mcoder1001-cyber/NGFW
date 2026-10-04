# OSPF topology MTU closeout envelope

- Branch `codex/closeout-ospf`, base `f41491d06`; isolated worktree `/root/ngfw-wt/codex-closeout-ospf`.
- Own `apps/agent/internal/agent/ospf_topology_integration_test.go` and `docs/status/tasks/closeout-ospf*` only.
- Slot 7, disposable VPP, namespaced FRR. No root-FRR mode, shared VPP restart or installed configuration changes.
- Investigate actual Exchange adjacency failure; modify fixture only when demonstrated. Keep Full, route count, restart and rollback assertions unchanged.
