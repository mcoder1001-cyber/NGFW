# F-multiwan — Multi-WAN failover and load balancing (cloud session charming-johnson, 2026-09-27)

WAN groups with link-health monitors: failover (best healthy member carries the default route) or weighted balance,
per-member NAT with sticky sessions, alarms on link transitions, and a live WAN page. User page:
`docs/user/network/multi-wan.md`.

## Decisions
- **RoutingConfig.wan_groups = 12** — self-allocated (F-multiwan was promoted from backlog BL-NET after
  wave-BC-numbers.md was written; 12 is the lowest free RoutingConfig field).
- **Next hop** is a member field (`gateway|dhcp|pppoe` + a `gateway` address for the static case) rather than a
  separate object; it composes with F-pppoe-client (a member can be a PPPoE WAN).
- **Failover selection**: lowest `priority` among healthy members (ties by interface name); **balance**: weighted
  ECMP over healthy members. Both are pure functions (`internal/multiwan`), unit-tested.

## Built (all runs in this container)
- **Contract** (05af3c6e): schema `routing.wanGroups[]` (members, monitors, mode, stickySessions); semantic
  `routing.wan-groups` (unique names, member interfaces exist); proto `RoutingConfig.wan_groups=12` +
  WanGroup/WanMember/WanMonitor. Drift guard + buf breaking clean.
- **Agent** (6abe04de): `internal/multiwan` — `MonitorConfig.Healthy` (loss%/latency), `State.Observe`
  (downAfter/upAfter hysteresis, no flap), `FailoverActive` (lowest-priority healthy), `BalancePaths` (healthy
  weighted). Unit-tested. The probing (ICMP/HTTP/DNS per link) and routing/NAT reaction are host-side.
- **API** (e94d271b): `WanState` RPC + `GET /api/v1/state/wan` (per-group member up/down, loss, latency, active
  member); fake agent reports member health. e2e: commit a group → live state, unknown member rejected.
- **Web** (f9af89b4): `/routing/wan` page (members with health, loss/latency, active member); groups edited via the
  generic Config → Routing editor; en + fa; jsdom tests.

## Evidence (this session)
- Agent: `go test ./internal/multiwan/` (Healthy thresholds, hysteresis no-flap, failover selection, balance paths);
  go vet + golangci-lint clean; schema-proto drift guard + `buf breaking` clean.
- API: unit 306/306; e2e 2/2 (live state, semantic rejection).
- Web: 497/497, lint, `check-logical-css`.
- No VPP in the container, so the failover/balance decision is unit-tested and the live view is verified against a
  fake agent; the routing/NAT application and topology are the host row.

## Not done here → `F-multiwan-host` (lab: VPP + two WAN netns)
- Agent host-side wiring: run the monitors (probes sourced per member link/VRF), install the default route
  (failover) / weighted ECMP (balance) via VPP, per-member source NAT with sticky sessions and clearing the dead
  link's NAT sessions on failover, ABF pinning to a group/member, and the `WanState` RPC handler (501 until then).
- Topology on the af_packet rig with two WAN netns: cut link 1 → traffic moves within `downAfter × interval + 2 s`;
  restore → back (failover); balance splits ~1000 flows by weight within ±10 %.
