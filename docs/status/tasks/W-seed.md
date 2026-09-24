# W-seed: wave-A hotspot anchors and shared seams (no behaviour change)

Branch `task/W-seed` (slot 3), base `task/P08@74ec04e` (speculative, D-114), then `task/P08@e1587c9` merged (see "P08 merge").
Commits: `5c6e1f8 contract(wave-A): anchors` (schema + proto), `64fc0e9 chore(wave-A): hotspot anchors, seams and shells`
(the rest), plus the P08 merge and this status file. Questions and rule conflicts: `W-seed-questions.md`.

## What
**Anchors.** One `// wave-A: <task-id>` line (`#`/`<!-- -->` where the language needs it) per touching task at each insertion point
of `docs/status/wave-A-hotspots.md` §1. The touching tasks come from §3 and from the five wave-B envelopes (P11, F-wireguard, P12,
F-kea-dhcp-relay, F-unbound-chrony-syslog), and the anchors are in board order (`plan/tasks.yaml`). There are two exceptions to board order:
proto message anchors follow the §2 field numbers, and the `nav.test.ts` list follows navigation order (Q4). Every anchor block starts with
a one-line comment that says what a feature adds under its anchor.

| id | file | anchor blocks (touching tasks) |
|---|---|---|
| A1 | `apps/agent/internal/subsystems/subsystems.go` | new domain consts (10) · `Domains` Interfaces (6), VRFs (2), Routing (4), new domain entries (10) · end of `Register()` (16) = 48 |
| A2 | `apps/agent/internal/agent/projection.go` | end of `project()` (16) · end of `assemble()`, after `desired.Assemble` and the routes (16) = 32 |
| A4 | `apps/agent/internal/agent/server.go` | `Action` is a type switch with `default:` Unimplemented (same message); case anchors F-vrf-static-ecmp (ping), F-neighbors-ra (arp-flush), F-nat44-ed-sessions (session-kill), F-unbound-chrony-syslog (dns lookup, from its envelope) = 4 |
| C1 | `packages/schema/src/domains/interfaces.ts` | `InterfaceSchema` (6) · `SubinterfaceSchema` (3) = 9 |
| C1 | `packages/schema/src/domains/vrfs.ts` | `VrfSchema` (2) |
| C1 | `packages/schema/src/domains/routing.ts` | `NextHopSchema` (1) · `StaticRouteSchema` (2) · `RoutingSchema` root (2) = 5 |
| C1 | `packages/schema/src/domains/services.ts` | `ServicesSchema` root (2) |
| C2 | `packages/schema/src/semantic/index.ts` | imports (14) · `SEMANTIC_VALIDATORS` spreads (14) = 28 |
| C3 | `packages/schema/src/index.ts` | `export * from './domains/ext/<slug>.js'` (11) |
| C5 | `packages/proto/vrx/v1/dataplane.proto` | end of `service Dataplane` (16) · `Interface` (6) · `Subinterface` (3) · `Vrf` (2) · `StaticRoute` (2) · `RoutingConfig` (2) · `ServicesConfig` (2) · `AclConfig` (2) · `ActionRequest.action` (3) · `EventKind` (5) = 43; plus 16 `// ----- <task-id> -----` section stubs at the end of the file |
| C6 | `docs/contracts/proto.md` | new heading "## 11. Feature RPCs" (16) |
| P1 | `apps/api/src/app.module.ts` | import (16) · `controllers` (16) · `providers` (16) = 48 |
| P4 | `apps/api/src/agent/agent.client.ts` | `@ngfw/proto` type imports (16) · methods (16) = 32 |
| P5 | `apps/api/src/testing/fake-agent.ts` | `impl()` handler object (16) |
| P6 | `apps/api/src/infra/bus.ts` `TOPICS` (6) · `apps/api/src/telemetry/relay.service.ts` `eventTopic()` cases (6) = 12 |
| W1 | `apps/web/src/router.tsx` | above `system/users` (15; EI uses ED's natTabs) |
| W2 | `apps/web/src/nav/nav.ts` | `BUILT_DOMAINS` (8) · non-domain NavItems in `buildNav` (7) = 15 |
| W2 | `apps/web/src/nav/nav.test.ts` | `available` list (15, in navigation order) |
| W3 | `apps/web/src/i18n.ts` | en+fa imports (17) · `NAMESPACES` (17) · `en` (17) · `fa` (17) = 68 |
| shells | `apps/web/src/domains/{vpn,services}/tabs.ts` | P11, F-wireguard · F-kea-dhcp-relay, F-unbound-chrony-syslog = 4 |

**One entry per line.** `Domains` slices (A1), `BUILT_DOMAINS` and the test's `available` list (W2), and the i18n imports,
`NAMESPACES` and `en`/`fa` literals (W3). Their content is unchanged; the only additions are the shell namespaces `services` and `vpn`.

**Seams** (launch plan §5.2):
- `subsystems.Env.Publish func(*vrxv1.Event)` and `Env.Resync func()` are reached through `Wiring.Publish(ev)` and
  `Wiring.RequestResync()` (`subsystems/seams.go`). Both are nil by default, and nil means no-op. `TestEventAndResyncHooksDefaultInert`
  checks this. `agent.go` does not set the hooks yet, because it is outside this envelope. Q1 has the proposed wiring.
- `subsystems.SlotIDRange() (*IDRange, error)` reads `VRX_VPP_TABLE_BASE` and returns base..base+999. It returns nil when the variable
  is unset (the product agent owns every id) and an error for a malformed value, 0, or a range that overflows. `TestSlotIDRange` covers these cases.
- The vpn and services page shells are `apps/web/src/domains/DomainTabsPage.tsx` plus `{vpn/VpnPage,services/ServicesPage}.tsx`
  and their `tabs.ts` registries. With an empty registry the page renders exactly `DomainPlaceholderPage`, so there is no
  user-visible change. The shell routes replace the placeholder routes for `vpn` and `services`, so each path is registered once. The nav
  is unchanged: both domains stay out of `BUILT_DOMAINS`. The i18n keys `vpn:{title,tabs,loading}` and `services:{…}` exist in en
  and fa. `DomainTabsPage.test.tsx` covers the placeholder when empty, a tab per registration with `?tab=`, the fallback for an unknown
  tab, and a single route per path.

## How verified
<!-- filled from the runs below -->

## Out of scope
Feature logic, schema/proto fields, RPCs, reachable new screens, P08 behaviour, `tools/ci.sh`, anchor removal, host tests.
`agent.go` wiring of the A5 hooks (Q1).
