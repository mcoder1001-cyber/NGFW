# Task: F-loopback-bvi-gso-lldp-span — loopback/BVI, GSO & offload flags, LLDP, SPAN/ERSPAN, nsim   (prepend 00-CONTEXT.md)

## Goal
Finish the remaining interface features end to end in FAST MODE: loopback interfaces (and their use as bridge BVI), per-interface GSO
and jumbo MTU, LLDP (global + per interface, neighbour table), port mirroring (SPAN, ERSPAN via a GRE destination) and the network
delay simulator (test tool). Reference: TNSR "Loopback, LLDP, SPAN"; VPP `gso`, `lldp`, `span`, `nsim` plugins
(WBS D1.11, D1.8, D1.7, D1.10, D1.9 in `plan/wbs.csv`; D1.9 is T3 "test tool, not a product feature").

## Inputs to read first
- Loopbacks: `apps/agent/internal/descriptors/core/loopback.go` (P05, `interface.loopback/loop<N>`; projection already maps
  `interfaces.<loopN>`) — reuse, do not edit (manager-owned); BVI membership = `l2.bridge-domain-member` port_type bvi owned by
  **F-bridge-l2** (consume only; if it is not merged yet, stub the BVI part behind a skip and say so)
- LLDP + SPAN: read-only on `task/DF-7` until merged — `git show task/DF-7:docs/agent/descriptors/lldp.md` / `span.md`:
  `lldp.global` (global, `lldp.RegisterGlobals` only, D-071, write-only), `lldp.interface/<if>` (write-only, Update = disable+enable),
  `span.mirror/<src>/<dst>/<device|l2>` (`sw_interface_span_enable_disable`, Retrieve `sw_interface_span_dump`)
- `apps/agent/binapi/gso/` (`feature_gso_enable_disable` — no dump) and `apps/agent/binapi/nsim/` (`nsim_configure2`,
  `nsim_cross_connect_enable_disable`, `nsim_output_feature_enable_disable`) — new descriptors you write
- `packages/schema/src/domains/services.ts` — `services.lldp{enabled, systemName, txHold, txIntervalSec, interfaces[]}` **exists**;
  `interfaces.ts` has `mtu` (jumbo = MTU ≤ 9216, existing `interface.mtu` descriptor) but no gso/span/nsim fields
- `docs/vpp-code-track.md` **V20** (lldp passes the sw_if_index as a hw index → fallback: Create verifies with `lldp_dump`, documented) and
  **V19/V21** (per-interface feature state survives interface deletion → clear GSO/SPAN before deleting an interface)

## Contract changes
Additive on `contract/F-loopback-bvi-gso-lldp-span`: `interfaces.<if>.gso?: boolean`, `interfaces.<if>.mirror?[]{destination, direction:
rx|tx|both, level: device|l2}`, `services.nsim?{delayMs, bandwidthMbps, packetSize, dropFraction, crossConnect?{a,b}, outputInterfaces[]}`.
LLDP needs none. Proto to match.

## Scope — build exactly this
1. **Schema**: mirror destination ≠ source, exists, not itself mirrored (no loops); GSO only on interface types that support it (not
   sub-interfaces); nsim values in VPP ranges; LLDP interface exists (rule exists — test it).
2. **Agent**: new `gso.interface/<if>` (write-only, D-063/D-076: idempotent Create or a boot-identity claim record per D-080 keyed on
   sw_if_index **and** logical name); `nsim.config` (**global** — only via the globals owner, D-071; tests hold the globals lock
   exclusively and restore previous values, D-082) + `nsim.cross-connect`/`nsim.output`; wire DF-7 `lldp`/`span` into the projection.
   Unit tests with the fake client (model duplicate-add behaviour); ONE host integration check (prefixed loopbacks/taps): Retrieve ==
   desired for readable types, `vppctl show span` / `show lldp` / `show interface` contain it, rollback clears it, restart simulation.
3. **API**: config via pointer routes; `GET /api/v1/state/lldp/neighbors` (from `lldp` state; 501 with reason if the API has no neighbour
   dump — check binapi); mirror sessions appear in `/state/interfaces` items.
4. **UI**: loopback create in the interfaces list; per-interface "Advanced" group (GSO, MTU/jumbo); LLDP page (global form + neighbour
   table); Mirroring page; nsim only under the Tools group; en+fa.
5. **Docs**: `docs/user/interfaces/loopback-bvi-gso-lldp-span.md` (loopback as BVI, ERSPAN to a GRE tunnel, LLDP).

Files you own: `apps/agent/internal/descriptors/{lldp,span,gso,nsim}/**`, `docs/agent/descriptors/{lldp,span,gso,nsim}.md`,
`apps/agent/internal/agent/project_loopback_bvi_gso_lldp_span*.go`, `apps/api/src/features/loopback-bvi-gso-lldp-span/**`,
`apps/web/src/domains/interfaces/loopback-bvi-gso-lldp-span/**`, `apps/web/src/locales/*/loopback-bvi-gso-lldp-span.json`,
`docs/user/interfaces/loopback-bvi-gso-lldp-span.md`, `test/topology/loopback-bvi-gso-lldp-span/**`.
Shared files: one-line appends only (agent registry/projection hook, `app.module.ts`, web router/nav); `packages/api-client` regenerated.

## Acceptance (paste the evidence)
- [ ] `vppctl show span`, `show lldp`, `show interface <loopN>` and `show bridge-domain <id> detail` (BVI) reflect the commit (pasted)
- [ ] Agent-restart simulation → loopback, mirror, LLDP, GSO back within 30 s (log excerpt; write-only types re-applied exactly once)
- [ ] Rollback removes mirror/LLDP/GSO/nsim and the loopback (Retrieve for readable types, `vppctl` for write-only ones)
- [ ] Mirror destination equal to its source → 400 problem+json with `pointer`
- [ ] UI screenshot against the real endpoint in `docs/status/tasks/F-loopback-bvi-gso-lldp-span.md`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
Bridge domains and BVI membership descriptors (F-bridge-l2); GRE tunnel creation for ERSPAN (F-tunnels — use an existing tunnel);
bonds (F-bonding); sub-interfaces (F-vlan-qinq); checksum/TSO offload flags of DPDK devices (startup.conf, F-startup-gen);
SNMP LLDP-MIB (F-snmp); packet capture (F-capture-trace).

## Open questions to surface, not to decide silently
Whether VPP exposes an LLDP neighbour dump (if not: neighbours are CLI-only → park as a V-item, show "not available").
nsim is a test tool — confirm it belongs in the product UI at all.
