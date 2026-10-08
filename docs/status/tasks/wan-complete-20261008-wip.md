# Dynamic WAN gateway completion checkpoint

Branch: `codex/wan-complete-20261008`, base `417e8fcd`.

Owned: `internal/multiwan/gateways*`, WAN route/monitor wiring and the two PBR
projection hooks in `internal/agent/service.go`. No schema or stored-state change.

Implemented: complete owned DHCP lease polling, generation-bound expiring runtime
snapshots, BOUND-only admission, withdrawal on missing/error observations and retry
on each watch tick. Detached projection converges WAN routes, PBR and NAT pool
addresses without persisting leases. Lease renumber/removal queues old NAT session
cleanup. Unbound dynamic groups reserve IPv4 default-route ownership.

PPPoE remains explicitly unavailable in gateway reader: negotiated peer state is
not evidence of a usable VPP egress. The carrier worker must supply verified
transit-interface readiness; using raw physical WAN would forward unencapsulated
IP. This is remaining source work, not laboratory-only acceptance.

Validation: focused multiwan race tests PASS (1.041s); existing agent WAN/generation tests PASS (3.237s); owned DHCP dump renewal/release/error and runtime configuration-isolation tests PASS (1.097s), all with race detector.
CI deferred per owner instruction until all source tasks finish. No lab run.

Next: finish focused agent tests, independent review, publish checkpoint. Local
and remote commit identifiers are recorded by the manager after publication.

R4 review repair: retired-address cleanup now cancels when the address is rebound,
including before health hysteresis recovers. Ordinary dead-member retries cancel
when the member recovers. Cleanup cursors reset when protected addresses leave the
queue. A failed-delete/rebound control preserves the replacement session while
still removing an unrelated dead-member session. Focused race tests PASS:
`internal/multiwan` 1.045s, `internal/agent` 1.091s.
