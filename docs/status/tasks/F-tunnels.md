# F-tunnels — GRE, IPIP, VXLAN tunnels end to end (status)

Branch `claude/modest-keller-upaw4m` (cloud session modest-keller). Gap-only on top of DF-6's descriptors. **No
contract change**: the existing `tunnels.gre|vxlan|ipip` schema/proto keys are projected; the new kinds
(VXLAN-GPE, GTP-U, L2TPv3, PPPoE, 6RD) and a `TunnelState` RPC need schema + proto commits and are **not built**.

## What was built

| layer | files | what |
|---|---|---|
| agent | `internal/desired/tunnels.go` | builder `Tunnels`: gre/ipip/vxlan → `gre.tunnel` / `ipip.tunnel` / `vxlan.tunnel` (canonical addresses, outer table from `underlayVrf`, VXLAN default port → 0, IPIP no `dscp` → COPY_DSCP flag), `tunnels.meta/<vpp name>` (name, description, VXLAN decap), and with `interfaces` in the transaction `interface/<vpp name>` (creator = tunnel key), admin state, MTU, VRF binding, addresses, `l2.bridge-domain-member` (L2 tunnels only). Rules `tunnels.instance-required`, `tunnels.instance-range` (TD-8b), `tunnels.instance-unique`, `tunnels.vrf-exists`, `tunnels.value`, warning `tunnels.interfaces-domain`. Assembler `AssembleTunnels`: Retrieve → `tunnels.*` named from `tunnels.meta`, removes `interfaces.<vpp name>` (unless the stored document names it) and hides tunnel bridge memberships from the bridge-l2 assembler. `tunnels.meta` descriptor (agent-local, `CheckPersistent`) |
| agent | `internal/subsystems/tunnels.go` (+ 1 call under the `wave-BC: F-tunnels` register anchor, anchor comment in `Domains`) | registers the three tunnel descriptors (tolerant Retrieve when the plugin is absent, as F-lisp) and `tunnels.meta` over `<state dir>/tunnels-meta-<owner>.json`; appends the names to `Domains["tunnels"]`; `TunnelsIDSpan()` from the same id scope as `Env.IDs` (fail closed) |
| agent | `internal/agent/projection.go` (2 lines under the anchors) | `desired.Tunnels` in project(), `desired.AssembleTunnels` in assemble() |
| agent (DF-6 gaps) | `descriptors/{gre,ipip,vxlan}/{tunnel,sixrd,register}.go` | TD-11b: `RecordsNoOwnership()` on gre/ipip tunnel, ipip 6rd, vxlan tunnel (tag ownership); TD-11c: `iface.RegisterKind` for "GRE tunnel device", "IPIP tunnel device", "ip6ip-6rd", "VXLAN" |
| agent (test model) | `descriptors/core/coretest/tunnels.go` | gre/ipip/vxlan add/del/dump with device classes, duplicate instance refused, L2 VXLAN has a MAC |
| agent tests | `internal/agent/rpc_tunnels_test.go` | apply → Retrieve == desired; idempotent; agent restart (names kept); lost tunnel re-created; rollback deletes attributes before each tunnel; foreign `gre7001` never taken over; TD-8b / instance / VRF / bridge-domain checks; L2 VXLAN in a bridge domain round-trips |
| board tests | `subsystems/reachability_test.go` (gre, ipip, vxlan → wired, `maxPending` 23 → 20), `subsystems/creators_guard_test.go` (4 gaps removed), `agent/rpc_lisp_test.go` (unsupported-field example now `routing.bfd`), `agent/projection_test.go` (examples projected with `VRX_VPP_ID_RANGE=all`) |
| schema example | `packages/schema/examples/tunnels-gre-vxlan-ipip.json` | instances added to `mirror`, `vx-100`, `vx-mcast` (the agent needs one) |
| web | `apps/web/src/domains/vpn/tunnels/**`, `locales/{en,fa}/tunnels.json`; router/nav/i18n lines under the anchors | VPN › Tunnels (`/vpn/tunnels`): tabs GRE / VXLAN / IPIP, each the WEB-2 `CollectionView` (list + drawer SchemaForm, pending state), engine-interface column, link status from `/state/interfaces`; `tunnels` in `BUILT_DOMAINS` |
| docs | `docs/user/vpn/tunnels.md`, see-also in `docs/user/interfaces/basics.md`, F-tunnels notes in `docs/agent/descriptors/{gre,ipip,vxlan}.md` |

## Shared hunks
- `apps/agent/internal/subsystems/subsystems.go`: register call under `// wave-BC: F-tunnels`; anchor comment in `Domains`.
- `apps/agent/internal/agent/projection.go`: one line under each `// wave-BC: F-tunnels`.
- `apps/agent/internal/desired/lisp.go`: F-lisp's placeholder `agent.unsupported-field` warning for gre/vxlan/ipip removed (it said "until F-tunnels wires …").
- `apps/web/src/{router.tsx,nav/nav.ts,nav/nav.test.ts,i18n.ts}`: one line per anchor (nav.test lists `tunnels` after `vpn`, schema order).

## Checks run here (cloud sandbox: no VPP, no PostgreSQL, no lab slot)
- agent: `go build ./...`, `go vet ./...` clean; `go test ./...` all ok except `internal/contracttest`
  `TestSchemaProtoDrift` / `TestSchemaProtoDriftDetectsBreakage`, known-failing on main — output identical with and without
  this change (compared against a stash). `golangci-lint` (scratchpad binary) on the touched packages: 0 issues.
- web: `tsc --noEmit`, eslint, logical-CSS check ok; `vitest run` 79 files / 486 tests passed (new: page, model, locale parity).
- schema: `vitest run` 59 files / 1497 tests passed (example change).
- API not touched.

## Not tested / not built
- **No host run**: the lab integration check (gre + ipip + vxlan on the shared VPP, `NRestarts`, `vppctl show …`),
  the agent-restart timing and the screenshots against a real endpoint were not run (no VPP / lab here). The fake
  VPP model covers apply / Retrieve / restart / rollback.
- **Not built (contract needed):** `tunnels.vxlanGpe|gtpu|l2tpv3|pppoe`, IPIP 6RD, `GET /api/v1/state/tunnels`
  (`TunnelState` RPC), the "advanced" toggle for GTP-U/L2TPv3/PPPoE. Their descriptors stay pending on F-tunnels in
  the reachability table (vxlan_gpe, gtpu, l2tp, pppoe) and in the TD-11c gap list.
- The duplicate (src, dst, vni) → 400 acceptance is the schema's semantic rule (P02c); not re-verified end to end.

## Open questions
- `instance` is optional in the schema but required by the agent (the DF-6 descriptors key tunnels by VPP name).
  Options: keep it required (this change), or have the agent allocate instances from its id range and persist them in
  `tunnels.meta`. Default kept: required.
- GTP-U forwarding entries (VPP-wide): default drop, unchanged.
