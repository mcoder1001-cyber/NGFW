# F-multiwan-wiring recovery envelope
Branch: codex/F-multiwan-wiring-20261004; worktree /root/ngfw-wt/F-multiwan-wiring-20261004; base origin/main d5557440c.
Owned files: apps/agent/internal/multiwan/pbr.go and routes_test.go; docs/status/tasks/F-multiwan-wiring*. Existing runtime, routing, NAT, dead-link cleanup and group ABF integration are on main (#154). Scope is recovery verification and fixing cross-family group path selection. No host mutation.
Laboratory-only packet and restart acceptance remains in DEFERRED-ACCEPTANCE.md. Independent review and mandatory quick gate required before merge. Manager owns board.
