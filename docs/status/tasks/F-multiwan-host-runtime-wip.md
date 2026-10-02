# Multi-WAN runtime checkpoint, 2026-10-02

Branch: `codex/network-manager-20261002`. Worktree: `NGFW-network-manager`. Developer: network_delivery_manager (reassigned from executive manager to development).

Owned files: new `apps/agent/internal/multiwan/runtime.go` and `runtime_test.go`; planned new `internal/multiwan/probe_linux.go`, `internal/agent/rpc_wan.go`, `rpc_wan_test.go` and small coordinated lifecycle integration in `internal/agent/agent.go`. Identity worker confirmed no overlapping agent lifecycle edits.

Implemented foundation: schema-bounded 2048 workers, one probe at a time per logical monitor, independent monitor timing, full cancellation/drain on configuration replacement, stale generation rejection, preserved hysteresis for unchanged configuration, immutable sorted snapshots, AND health across monitors, unobserved links not healthy. `Active` remains empty until real route installation is confirmed; no fabricated installed route.

Actual test: `go test -race ./internal/multiwan -count=5` PASS (1.751s), including generation replacement held while deliberately late old probe finishes, monitor isolation, malformed/oversized config preservation and snapshot isolation. Tests are unit/race tests, not laboratory acceptance.

Remaining code: actual interface-bound Linux probes, committed-config lifecycle wiring, owner-scoped WanState RPC, static IPv4/default VRF route source and NAT cleanup. Dynamic gateway/VRF/ABF remain explicit follow-ups. Nothing in this checkpoint completes the entire F-multiwan-host task. Lab cases remain NOT RUN in DEFERRED-ACCEPTANCE.

Next command: implement Linux bound probe; then production lifecycle/RPC tests; independent review and complete hosted quick CI before any merge.
