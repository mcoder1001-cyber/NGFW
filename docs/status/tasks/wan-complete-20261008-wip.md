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

## Fixed carrier probe executor checkpoint

Added `cmd/ngfw-wan-probe` and Debian staging/checksum/install entries. The fixed
executor binds only `ppp0`, accepts only literal unicast IPv4 endpoints and the
three existing ICMP echo / HTTP HEAD port 80 / DNS root-NS UDP port 53 modes,
with a maximum three-second deadline. There is no device/namespace selector,
DNS-name resolution permission, alternate port or arbitrary command execution.
The carrier helper owns namespace validation, temporary destination-limited
policy and cleanup. IPv6 and hostname carrier monitors report unavailable.

Focused race executor tests PASS (1.018s); Debian preparation fixture initially
failed its fixed binary count (6 versus 7), then PASS after extending its existing
build/checksum/install assertions to cover the new binary (3 tests, 0.621s).
Carrier runtime gateway/probe dispatch integration awaits its verified API.
