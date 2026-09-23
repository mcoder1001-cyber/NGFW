# Task: F-bridge-l2 — bridge domains, L2XC/L3XC, split-horizon, MAC aging, time-range MAC filter   (prepend 00-CONTEXT.md)

## Goal
Implement L2 switching end to end in FAST MODE: bridge domains with members (incl. one BVI), split-horizon groups, flags and MAC
aging, static L2 FIB entries, L2 cross-connects, L3 cross-connects, and the `mactime` time-range MAC filter.
Reference: TNSR "Bridge domains / L2 cross-connect"; VPP `l2`, `l3xc`, `mactime` (WBS D1.6 in `plan/wbs.csv`).

## Inputs to read first
- `apps/agent/internal/descriptors/l2/` + `docs/agent/descriptors/l2.md` (DF-1, merged) — reuse `l2.bridge-domain` (`bd_tag` ownership,
  mac-age inside the object), `l2.bridge-domain-member` (port_type normal/bvi/uu-fwd, `shg`; one BVI per BD), `l2.xconnect` (one object
  per direction), `l2.fib-entry`, `l2.flags`, `l2.vlan-tag-rewrite`
- `apps/agent/internal/descriptors/l3xc/` + `docs/agent/descriptors/l3xc.md` — `l3xc.l3xc/<rx if>/<ip4|ip6>` (`l3xc_update`/`l3xc_del`)
- `apps/agent/binapi/mactime/` — `mactime_enable_disable`, `mactime_add_del_range`, `mactime_dump` (new descriptor, you write it)
- `docs/agent/descriptors/interface.md` — `interface/<name>` alias (D-065/D-069), ClaimStore for physical NICs (D-075)
- `packages/schema/src/domains/interfaces.ts` — **no L2 model exists** (only L3 fields)
- `docs/lab/shared-host-rules.md` — bridge-domain ids from your slot's table range (`N000–N999`); VPP 26.06 docs → "L2 bridge domains"

## Contract changes
Additive on `contract/F-bridge-l2` first (`contract(schema): l2`): new `interfaces`-adjacent keyed collections —
`l2.bridgeDomains{<name>:{id, flood, uuFlood, forward, learn, arpTerm, macAgeMin, members{<if>:{shg?, bvi?, uuFwd?}}, staticMacs[]}}`,
`l2.xconnects{<rxIf>: {tx}}`, `l3xc{<rxIf>:{ipv4Paths[], ipv6Paths[]}}`, `l2.macFilters{<name>:{mac, ranges[{days,start,end}], action}}`.
A new root key is a reshape → put it under `interfaces.<if>.l2` / a keyed record inside an existing domain unless the manager answers
otherwise (write the question, pick the in-domain variant, continue). Records keyed by name (D-045/D-053).

## Scope — build exactly this
1. **Schema**: member interface in at most one BD or xconnect; a member has no L3 addresses/VRF while bridged (except the BVI);
   one BVI per BD and the BVI must be a loopback; shg 0–255; mac-age 0–255 min; xconnect rx ≠ tx; mactime ranges well-formed.
2. **Agent**: projection onto the DF-1 descriptors above + a new `mactime` descriptor package (`mactime.range/<name>`, enable per
   interface; Retrieve from `mactime_dump` — if the dump cannot round-trip a field make it write-only per D-063/D-076 with the D-080
   boot-identity record). Retrieve covers every object; fake-client unit tests; ONE host integration check (`VRX_INTEGRATION=1`,
   prefixed taps/loopbacks, lab lock shared): Retrieve == desired, `vppctl show bridge-domain <id> detail` / `show l2patch`/`show l3xc`
   contain it, rollback leaves nothing, agent-restart simulation recreates it.
3. **API**: config via pointer routes; `GET /api/v1/state/l2/bridge-domains` (members, BVI, learned-MAC count),
   `GET /api/v1/state/l2/bridge-domains/{id}/macs?page` (server-side paged `l2_fib_table_dump`).
4. **UI**: "Bridging" screen: BD list + SchemaForm, member table, MAC table (ServerDataGrid), cross-connect list; en+fa.
5. **Docs**: `docs/user/interfaces/bridge-l2.md` (BD with BVI, xconnect, time-range filter; CLI equivalent).

Files you own: `apps/agent/internal/descriptors/{l2,l3xc,mactime}/**`, `docs/agent/descriptors/{l2,l3xc,mactime}.md`,
`apps/agent/internal/agent/project_bridge_l2*.go`, `apps/api/src/features/bridge-l2/**`, `apps/web/src/domains/interfaces/bridge-l2/**`,
`apps/web/src/locales/*/bridge-l2.json`, `docs/user/interfaces/bridge-l2.md`, `test/topology/bridge-l2/**`.
Shared files: one-line appends only (agent registry/projection hook, `app.module.ts`, web router/nav); `packages/api-client` regenerated.

## Acceptance (paste the evidence)
- [ ] `vppctl show bridge-domain <id> detail` shows members, shg, BVI and mac-age as committed; Retrieve == desired (pasted)
- [ ] Agent-restart simulation → BD, members, xconnect, l3xc back within 30 s (log excerpt)
- [ ] Rollback returns members to L3 and deletes the BD (Retrieve)
- [ ] Interface in two bridge domains → 400 problem+json with `pointer` to the second membership
- [ ] UI screenshot against the real endpoint in `docs/status/tasks/F-bridge-l2.md`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
Loopback creation and BVI UX on the loopback screen (F-loopback-bvi-gso-lldp-span consumes your BVI member support); SPAN/ERSPAN
(F-loopback-bvi-gso-lldp-span); VLAN sub-interfaces and tag rewrite UX (F-vlan-qinq); bonding (F-bonding); ARP termination tables
beyond the BD flag (F-neighbors-ra); VXLAN/GRE-L2 tunnels as BD members beyond "any `interface/<name>` works" (F-tunnels); EVPN.

## Open questions to surface, not to decide silently
Where the L2 model lives (in-domain record vs. new root key) — pick in-domain, ask. Whether mactime (a "device allow-list by time") is
worth shipping in the UI or config-only.
