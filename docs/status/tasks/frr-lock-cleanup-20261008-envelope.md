# FRR slot lock cleanup

Branch: `codex/frr-lock-cleanup-20261008`.
Base: `4d4723f78ab053d005015b2a7e41ba840fec06c5`.

Owned: `apps/agent/internal/renderers/frr/frrtest/**`, the FRR slot acquisition section of `apps/agent/internal/agent/ospf_topology_integration_test.go`, and this task's envelope/WIP.

Scope: preserve the acquired cleanup handle after failed base reset; close the descriptor when nonblocking slot acquisition fails; add offline lock release and descriptor regressions. Startup deadlines and daemon identity predicates stay unchanged. Independent source review and the unchanged hosted quick gate are required before merge.
