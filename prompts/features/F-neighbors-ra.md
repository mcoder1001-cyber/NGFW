# Task: F-neighbors-ra — ARP/ND table, static neighbours, proxy-ARP/ND, IPv6 RA, DAD   (prepend 00-CONTEXT.md)

## Goal
Implement the neighbour database end to end in FAST MODE: live ARP/ND table (paged) with flush, static neighbours, neighbour-DB
limits, proxy-ARP (ranges + interfaces), proxy-ND, IPv6 router advertisements (per-interface config + advertised prefixes) and IPv6 DAD
(new in 26.06). Reference: TNSR "ARP / NDP / IPv6 RA"; VPP `ip-neighbor`, `ip6-nd`, `arp`, `ip6_dad` (WBS D2.3 in `plan/wbs.csv`).

## Inputs to read first
- DF-2 descriptors (merged) — reuse, name for name: `apps/agent/internal/descriptors/ip_neighbor/` (`ip-neighbor.neighbor/<if>/<ip>`,
  `ip-neighbor.config/<ipv4|ipv6>` — **global**, globals owner only), `ip6_nd/` (`ip6-nd.ra-config/<if>`, `ip6-nd.ra-prefix/<if>/<prefix>`,
  `ip6-nd.proxy/<if>/<ip6>` — **opt-in only**, `ip6-nd.dad/global` — **global**, `RegisterGlobals` only), `arp/` (`arp.proxy-range/<table>/
  <low>-<high>`, `arp.proxy-interface/<if>`); docs in `docs/agent/descriptors/{ip_neighbor,ip6_nd,arp}.md`; shared helper `descriptors/df2`
  is read-only for you (changes → questions file)
- `apps/agent/binapi/ip_neighbor/` (`ip_neighbor_dump`, `ip_neighbor_flush`, `want_ip_neighbor_events_v2`), `ip6_nd/`, `arp/`, `ip6_dad/`
- `packages/schema/src/domains/interfaces.ts` — **no neighbour/RA/proxy fields exist**
- `apps/api/src/state/state.controller.ts` (P06, merged; reworked by P08): `GET /api/v1/state/neighbors` still answers **501** ("needs an agent state RPC") — you add it additively and make it real
- `docs/vpp-code-track.md` **V12** (VPP aborted after `ip6nd_proxy_add_del` → proxy-ND stays opt-in behind `VRX_DF2_PROXY_ND=1`, D-064; the
  product may expose it only as "experimental, off by default"); host-vrx-a: plugin `ip6_dad_autoremove` **not loaded** (no auto-remove)
- LOG D-071 (neighbour-DB config and DAD are VPP globals: only the globals owner sets them; slots may only require), D-082 (globals lock)
- Registration facts (checked 2026-09-24, prep-waveA, against DF-2 + P08): `ipneighbor.Register` / `ip6nd.Register` / `arp.Register(r, c,
  owner, tables *df2.IDRange, …)` take `df2.WithClaims(<P08 Wiring.KeyedClaims("acl")>)` — the persisted claim store, never the in-memory
  default; `arp.Register`'s table range is the slot range `N000–N999` on the shared host and nil (= all) only for the product agent;
  `RegisterGlobals` (neighbour config, DAD) only when `Env.GlobalsOwner`. `ip_neighbor_flush` / `ip_neighbor_dump` /
  `want_ip_neighbor_events_v2` default to `sw_if_index = ~0` = **every interface of every slot**: flush only interfaces this agent may
  name (own tag or untagged, never a foreign tag — resolve with DF-1 `iface.ResolveName`), never send `~0` on the shared host; filter
  events to nameable interfaces and re-subscribe from P08's reconnect hook (`Wiring.Connected`, like DF-8's `Reconnected`)

## Contract changes
Additive on `contract/F-neighbors-ra`: `interfaces.<if>.ipv6Ra?{suppress, managed, other, lifetimeSec, minIntervalSec, maxIntervalSec,
prefixes{<prefix>:{validSec, preferredSec, offLink, noAutoconfig}}}`, `interfaces.<if>.proxyArp?`, `interfaces.<if>.proxyNd?[]`,
`vrfs.<name>.proxyArpRanges?[]`, `routing.neighbors?{static[{interface, ip, mac, noFibEntry}], ipv4Limits?, ipv6Limits?, dad?{transmits,
delayMs}}` + a paged neighbour state message and an `ArpFlushAction` in `ActionRequest` (proto).

## Scope — build exactly this
1. **Schema**: static neighbour IP in a connected subnet of that interface; MAC unicast; RA intervals min < max ≤ 1800; RA prefixes are
   /64 for autoconfig; proxy-ARP range low ≤ high in one family.
2. **Agent**: projection onto the DF-2 descriptors; globals (limits, DAD) emitted only when the agent is the globals owner (D-071); neighbour
   lister with agent-side paging/filter (`ip_neighbor_dump` per AF, vrf/interface filter) and change events from
   `want_ip_neighbor_events_v2`; flush action in `apps/agent/internal/actions/neighbors-ra/`. Fake-client unit tests; ONE host check
   (prefixed taps): Retrieve == desired, `vppctl show ip neighbors` / `show ip6 interface <if>` contain it, rollback clears, restart simulation.
3. **API**: config via pointer routes; replace the 501 in `/api/v1/state/neighbors` (paged, filter vrf/interface/state);
   `POST /api/v1/actions/arp-flush {interface?}` served by a controller in your own feature module (a static route wins over the
   generic `:action` route; `apps/api/src/actions/**` belongs to F-vrf-static-ecmp in this wave); the flush is audited.
4. **UI**: Neighbours screen (ServerDataGrid, live via WS events, flush button with confirm), static entries form, per-interface IPv6 RA tab; en+fa.
   P08's interface drawer renders every `interfaces.<if>` leaf through SchemaForm grouped by the `withUi` `group` hint, so the new
   per-interface fields appear there without a drawer rewrite; give them a `group` and make absent/default mean "off" (drawer saves
   write defaults back; P08's `dropPhantomOptionals` drops only unchanged optional objects).
5. **Docs**: `docs/user/routing/neighbors-ra.md` (static ARP, RA with SLAAC prefix, proxy-ARP; CLI equivalent).

Files you own: `apps/agent/internal/descriptors/{ip_neighbor,ip6_nd,arp}/**` (DF-2's, merged — gap fixes only),
`docs/agent/descriptors/{ip_neighbor,ip6_nd,arp}.md`, `apps/agent/internal/actions/neighbors-ra/**`, `apps/agent/internal/agent/project_neighbors_ra*.go`,
`apps/agent/internal/desired/neighbors_ra*.go` (P08 puts builders in `internal/desired/`), `apps/api/src/features/neighbors-ra/**`,
`apps/web/src/domains/routing/neighbors-ra/**`, `apps/web/src/locales/*/neighbors-ra.json`, `docs/user/routing/neighbors-ra.md`,
`test/topology/neighbors-ra/**`.
Shared files (P08 layout; protocol and anchors in `docs/status/wave-A-hotspots.md`; list each hunk in your PR):
`apps/agent/internal/subsystems/subsystems.go` (register, `Domains`, one line in the reconnect hook), `apps/agent/internal/agent/projection.go`
(one call in `project()`, one in `assemble()` after `desired.Assemble` — `desired/interfaces.go` stays P08's), `server.go` (the Action case;
the neighbour RPC method goes in an owned `rpc_neighbors_ra.go`), `state.controller.ts` (move the neighbors handler into your
controller), `apps/api/src/agent/agent.client.ts`, `apps/api/src/{infra/bus.ts,telemetry/relay.service.ts}` (event topic), `app.module.ts`,
web router/nav/`i18n.ts`; `packages/api-client` regenerated.

## Acceptance (paste the evidence)
- [ ] `vppctl show ip neighbors` lists the static entry; `vppctl show ip6 interface <if>` shows RA config + prefix (pasted); Retrieve == desired
- [ ] Agent-restart simulation → static neighbours + RA back within 30 s (log excerpt)
- [ ] Rollback removes static neighbours, RA config, proxy entries (Retrieve)
- [ ] Static neighbour outside every connected subnet → 400 problem+json with `pointer`
- [ ] UI screenshot of the live neighbour table against the real endpoint; flush shown working
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
Enabling proxy-ND by default (V12); DAD auto-remove (plugin not loaded — needs F-startup-gen + handover); DHCPv6/prefix delegation
(F-kea-dhcp-relay); static routes, FIB browser, ping (F-vrf-static-ecmp); ARP termination in bridge domains (F-bridge-l2); linux-cp
neighbour sync (P12); ND-based uRPF (F-rpf-adl-pbr).

## Open questions to surface, not to decide silently
Scale of the neighbour table events stream (rate-limit?) — pick 1 Hz coalescing, document.
