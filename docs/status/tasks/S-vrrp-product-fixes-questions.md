# S-vrrp-product-fixes — remaining owner decisions

F3: whether the product agent may start keepalived remains open under `docs/decisions/PENDING-agent-privileges.md`. The implemented scope is non-privileged readiness checking and a clear DryRun/Apply finding; slot harnesses own daemon lifetime. No daemon spawning was added.

The task prompt's `core.go` location predates the interface-address descriptor split. The necessary plugin-owned VIP filtering hook is implemented in `core/ifaddr.go`, with its constructor registration adjustment in `core/core.go`. Tests cover the actual product registry hook.

Review, finishing CI and actual merge of this shared-workspace implementation remain outstanding. The task must not be recorded as merged based on local tests alone.
