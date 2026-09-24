# Task: F-lb — VPP load-balancer plugin (GRE / NAT / L3DSR / Maglev)   (prepend 00-CONTEXT.md)

## Goal
Expose VPP's `lb` plugin: VIPs with GRE4/GRE6, L3DSR or NAT4/NAT6 encapsulation, application servers (AS), Maglev-style
consistent hashing (new-flows table), per-interface NAT for the NAT encap. Tier T3 (WBS D7.9: "solved outside the product by
HAProxy/nginx") — build the minimal, honest version. Reference: VPP plugin `lb`.

## Inputs to read first
- DF-7 (branch `task/DF-7`, not yet merged — read with `git show task/DF-7:<path>`, build on it once merged):
  `apps/agent/internal/descriptors/lb/`, `docs/agent/descriptors/lb.md`, `docs/agent/descriptors/policer.md` (DF-7 conventions),
  `descriptors/df7/**` (shared helper, read-only). Objects: `lb.conf/global` (globals owner only), `lb.vip/<prefix>/<proto>/<port>`,
  `lb.as/<vip prefix>/<proto>/<port>/<as addr>`, `lb.intf-nat/<if>/<ip4|ip6>`; helpers `lb.DumpVIPs`, `lb.FlushVIP`.
- `apps/agent/binapi/lb/` (`lb_conf`, `lb_add_del_vip_v2`, `lb_add_del_as`, `lb_add_del_intf_nat4/6`, `lb_vip_dump`, `lb_as_dump`, `lb_flush_vip`)
- `docs/vpp-code-track.md` **V20**: removed VIPs/ASes are freed only by CLI-triggered GC (fallback: globals owner runs the lb cleanup;
  AS /32 tracking entries linger in table 0), lb enum/type fields not byte-swapped by VPP (fallback: the descriptor swaps, little-endian
  only), broken `lb_vip_details` → all lb objects are **write-only** (D-063) with D-076 applied-once records for `intf-nat` (D-080 identity)
- D-052 (QoS precedent: new service features live under `services`), D-071 (`lb.conf` is a global), D-082, D-064
- `packages/schema/src/domains/services.ts` — **no lb model exists** (`nat.loadBalancedMappings` is NAT44-ED static LB, not this plugin)

## Contract changes
First, on `contract/F-lb` (additive; `contract(schema): services.lb` + proto + drift guard + `docs/status/tasks/F-lb-contract.md`):
`services.lb{ settings{ ip4Source?, ip6Source?, flowBuckets?, flowTimeoutSec? }, vips{<name>: { prefix, protocol: any|tcp|udp, port?,
encap: gre4|gre6|l3dsr|nat4|nat6, dscp? (l3dsr), targetPort? + nodePort? + srvType? (nat), newFlowsTableLength (power of 2), srcIpSticky,
servers[]{address, flushOnDelete}}}, natInterfaces[]{interface, family} }`. Tell the manager and continue.

## Scope — build exactly this
Files you own: `apps/agent/internal/descriptors/lb/**`, `docs/agent/descriptors/lb.md`, `apps/agent/internal/agent/project_lb*.go`,
`apps/api/src/features/lb/**`, `apps/web/src/domains/services/lb/**`, `apps/web/src/locales/*/lb.json`, `docs/user/services/lb.md`,
`test/topology/lb/**`. Shared: one-line appends only.
1. **Schema semantics** (on the contract branch): encap ↔ VIP family consistent; AS family = VIP family for GRE/NAT; port requires tcp/udp;
   table length power of two; unique (prefix, protocol, port); nat encaps require a matching `natInterfaces` entry.
2. **Agent**: projection `services.lb` → `lb.conf` (globals owner only; slots *require* it), `lb.vip`, `lb.as`, `lb.intf-nat`; read-only state
   from `lb.DumpVIPs` (what VPP reports correctly: prefix, encap/type, AS list with weights/"removed" flag) — never used as Retrieve.
3. **API**: config via pointer routes; `GET /api/v1/state/lb/vips` (VIPs + ASes + in-use/removed state); `POST /api/v1/actions/lb/vips/{name}/flush`.
4. **UI**: Load balancer page: VIP list + AS sub-table (SchemaForm), state column, flush action, a visible "write-only / GC" notice; en + fa.
5. **Docs**: `docs/user/services/lb.md` — GRE and L3DSR examples, what the plugin does NOT do (no health checks, no L7), V20 caveats, CLI equivalent.

## Acceptance (paste the evidence)
- [ ] After commit `vppctl show lb vips verbose` lists the VIP with its ASes and encap (pasted); a flow from ns-lan to the VIP is encapsulated
      (GRE seen by `tcpdump` in ns-wan) — optional evidence, not a NAT-class packet test
- [ ] Agent-restart simulation → VIP/AS re-applied without duplicates within 30 s (log excerpt; `intf-nat` applied once per VPP identity)
- [ ] Rollback removes VIPs/ASes (API delete messages sent; `show lb vips` shows them "removed" until GC — documented, V20)
- [ ] GRE4 VIP with an IPv6 AS → 400 problem+json with `pointer`
- [ ] `tools/ci.sh --base main` green; UI screenshot against the real endpoint

## Out of scope (do not build)
Health checks / L7 proxying (HAProxy/nginx, not planned); NAT44-ED load-balanced static mappings (F-nat44-ed-sessions); CNAT VIPs
(F-det44-map-dslite-cnat); VRRP/HA of VIPs (F-vrrp-config-sync); host-stack HTTP proxy (F-host-stack); fixing V20 in C (vpp-code-track only).

## Open questions to surface, not to decide silently
Is an in-product LB worth shipping given V20 (deleted VIPs linger until a CLI GC)? Default: ship behind the T3 label with the notice. Should the
globals owner run the `lb` GC CLI path at all (it needs vppctl, which the agent never uses)? Default: no — document the leak (V20).
