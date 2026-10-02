# Multi-WAN runtime checkpoint, 2026-10-02

Branch: `codex/network-manager-20261002`. Worktree: `NGFW-network-manager`. Developer: network_delivery_manager (reassigned from executive manager to development).

Owned files: new `apps/agent/internal/multiwan/runtime.go` and `runtime_test.go`; planned new `internal/multiwan/probe_linux.go`, `internal/agent/rpc_wan.go`, `rpc_wan_test.go` and small coordinated lifecycle integration in `internal/agent/agent.go`. Identity worker confirmed no overlapping agent lifecycle edits.

Implemented foundation: schema-bounded 2048 workers, one probe at a time per logical monitor, independent monitor timing, full cancellation/drain on configuration replacement, stale generation rejection, preserved hysteresis for unchanged configuration, immutable sorted snapshots, AND health across monitors, unobserved links not healthy. `Active` remains empty until real route installation is confirmed; no fabricated installed route.

Actual test: `go test -race ./internal/multiwan -count=5` PASS (1.751s), including generation replacement held while deliberately late old probe finishes, monitor isolation, malformed/oversized config preservation and snapshot isolation. Tests are unit/race tests, not laboratory acceptance.

Remaining code: actual interface-bound Linux probes, committed-config lifecycle wiring, owner-scoped WanState RPC, static IPv4/default VRF route source and NAT cleanup. Dynamic gateway/VRF/ABF remain explicit follow-ups. Nothing in this checkpoint completes the entire F-multiwan-host task. Lab cases remain NOT RUN in DEFERRED-ACCEPTANCE.

Next command: implement Linux bound probe; then production lifecycle/RPC tests; independent review and complete hosted quick CI before any merge.

## Second implementation checkpoint

- Real Linux probes now bind every network connection and name resolution to the member's LCP device with `SO_BINDTODEVICE`; no unbound fallback, proxy, redirect follow or shell. HTTP HEAD, actual DNS-server root-NS exchange (including literal server IP), and IPv4 ICMP reply identity/payload checks. Cancellation interrupts reads and every probe has a configured deadline.
- Agent lifecycle starts a watcher of durably committed desired state. Configuration removal/rollback stops probes. Changes to interface identity/VRF invalidate old health even with identical WAN groups. Draining/stopped runtimes report unavailable, not stale health.
- Owner-scoped `WanState` RPC returns sorted observed health, selected-group filtering, errors for unknown groups and unwired runtime. `Active` stays empty: no installed route is fabricated.
- Production projection explicitly warns that WAN route/NAT/ABF forwarding is not yet implemented. This PR's coherent scope is operational monitor observations, not full multi-WAN host completion. No board row is marked complete.
- Added protocol regressions using `net.Pipe` (HTTP no redirect, DNS transaction/response validation, ICMP echo and read cancellation), owner/filter errors, default-VRF/LCP fail closed, real Service Apply/removal watcher and interface replacement invalidation. These are unit/race tests using fake VPP/protocol peers, not laboratory evidence.
- Recent evidence: focused monitor/protocol/RPC race suite x5 PASS (multiwan 1.906s, agent 1.212s); durable Apply/removal watcher race x2 PASS (5.079s); `go vet ./internal/multiwan ./internal/agent` PASS; full `golangci-lint run --timeout=5m` 0 issues. Final post-readiness-change checks rerun before publication.
- Remaining: independently review this production monitor slice and hosted full quick CI; implement actual static IPv4/default-VRF owned route + generation-safe NAT cleanup in subsequent slice; dynamic gateways/VRF/ABF remain code follow-ups. All real VPP/network namespace/browser acceptance NOT RUN, deferred centrally.
