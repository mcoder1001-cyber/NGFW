# Task: F-tunnels — GRE, IPIP/6RD, VXLAN(-GPE), GTP-U, L2TPv3, PPPoE   (prepend 00-CONTEXT.md)

## Goal
Implement the **tunnel interfaces** end to end in FAST MODE: GRE (L3/TEB/ERSPAN, p2p/p2mp), IPIP (+6RD), VXLAN, VXLAN-GPE first
(T1), then GTP-U, L2TPv3 and PPPoE sessions (T3 value — schedule last, drop first if the time box runs out). Tunnels are interfaces:
addresses, MTU, VRF and bridge membership reuse the interface paths. Reference: TNSR "GRE / IPIP / VXLAN tunnels"; VPP plugins
`gre`, `ipip`, `vxlan`, `vxlan-gpe`, `gtpu`, `l2tp`, `pppoe` (WBS D6.6 in `plan/wbs.csv`).

## Inputs to read first
- `packages/schema/src/domains/tunnels.ts` — **only** `tunnels.gre`, `tunnels.vxlan`, `tunnels.ipip` exist (records keyed by name,
  `TUNNEL_KINDS`). Add `tunnels.vxlanGpe`, `tunnels.gtpu`, `tunnels.l2tpv3`, `tunnels.pppoe` (+ 6RD under ipip) **first** as separate
  `contract(schema|proto): …` commits on your task branch (no `contract/` branch; schema + proto + drift guard, additive after contracts-v1;
  numbers from `docs/status/wave-BC-numbers.md`); `TUNNEL_KINDS` gets the new prefixes. F-lisp adds `tunnels.lisp` under its own anchor
- Merged DF-6 packages `apps/agent/internal/descriptors/{gre,ipip,vxlan,vxlan_gpe,gtpu,l2tp,pppoe}/` + shared `df6/` (read-only) and
  `docs/agent/descriptors/{gre,ipip,vxlan,vxlan_gpe,gtpu,l2tp,pppoe,df6}.md` — keys `gre.tunnel/gre<instance>`, `ipip.tunnel/ipip<instance>`,
  `ipip.sixrd/<name>` (write-only), `vxlan.tunnel/vxlan_tunnel<instance>`, `vxlan-gpe.tunnel/<name>`, `gtpu.tunnel/<name>`, `l2tp.tunnel/<name>`,
  `pppoe.session/<mac>/<id>`, bypass toggles (write-only)
- `apps/agent/binapi/{gre,ipip,vxlan,vxlan_gpe,gtpu,l2tp,pppoe}/` — the only source of names
- **P08 (merged)** patterns you extend: builders + assemblers in `apps/agent/internal/desired/`, registration + `Domains` in
  `apps/agent/internal/subsystems/subsystems.go` (`Wiring.IfaceClaims()` = df6's claim store, `Wiring.BootStore()`), the hook in
  `apps/agent/internal/agent/projection.go`; tunnels already appear in `/state/interfaces`. `docs/status/vertical-slice.md`
- `docs/vpp-code-track.md`: **V8** gtpu crashes VPP on any failed add/delete (fallback: dump-first guard, host test opt-in
  `VRX_DF6_GTPU_HOST=1`); **V14** L2TPv3 has no delete message (fallback: `df6.ErrNoDelete`, create only with `VRX_DF6_L2TP_CREATE=1`);
  **V21** vxlan bypass flag survives interface delete (fallback: disable before enable); V19 (per-interface state inherited by index reuse)
- `docs/decisions/LOG.md` D-064 (crashing tests opt-in, check `NRestarts`), D-065/D-069 (logical names, `interface/<name>`), D-071
  (`l2tp.lookup-key`, `pppoe.cp` are globals → globals owner only), D-074 (check existence before every delete), D-076/D-080, D-082

## Scope — build exactly this
1. **Schema**: semantic rules — src/dst same family and src ≠ dst; src configured on an interface in the underlay VRF; (src, dst, key/vni,
   instance) unique per kind; VXLAN multicast dst requires `mcastInterface`; VNI ≤ 16777215; GRE ERSPAN requires `sessionId`; GTP-U
   `qfi` needs `pduExtension`; PPPoE sessionId 1–65535. Several of these already exist in P02c's `tunnels.ts` / `semantic/tunnels.ts`
   (test them, do not re-add). Addresses, MTU and both VRFs live on the tunnel record itself (`tunnelCommon`); other domains reference a
   tunnel by its `instance`-fixed VPP name.
2. **Agent**: projection per kind → the DF-6 descriptors + `interface/<name>` alias + address/MTU descriptors; TEB/VXLAN bridging only via
   DF-1's `l2.bridge-domain-member` in F-bridge-l2's bridge domains (`tunnels.*.bridgeDomain` = the numeric BD id; F-bridge-l2 is merged —
   do not build BDs). Retrieve covers every kind; write-only kinds follow D-063/D-076.
   ONE integration check on the host VPP (`VRX_INTEGRATION=1`, shared lock, prefixed objects) for gre + ipip + vxlan + vxlan-gpe;
   gtpu/l2tp/pppoe host steps stay opt-in (V8/V14/pppoe needs a learnt client MAC — unit tests with the fake cover them).
3. **API**: config via pointer routes; `/api/v1/state/interfaces` already lists tunnels (P08) — add `GET /api/v1/state/tunnels` (kind,
   endpoints, VRF, oper state, counters). OpenAPI; regenerate `packages/api-client`.
4. **UI**: Tunnels page with one tab per kind (GTP-U/L2TPv3/PPPoE behind an "advanced" toggle), SchemaForm create/edit, status column;
   en + fa; screenshot against the real endpoint.
5. **Docs**: `docs/user/vpn/tunnels.md` — GRE-over-IPsec pointer to P11, VXLAN L2 extension example, CLI equivalents, per-kind limits.
Files you own (the envelope's list wins): DF-6's `apps/agent/internal/descriptors/{gre,ipip,vxlan,vxlan_gpe,gtpu,l2tp,pppoe}/**` gap-only,
`docs/agent/descriptors/{gre,ipip,vxlan,vxlan_gpe,gtpu,l2tp,pppoe}.md`, `apps/agent/internal/desired/tunnels*.go`,
`apps/agent/internal/subsystems/tunnels*.go`, `apps/agent/internal/agent/rpc_tunnels*.go`, `packages/schema/src/semantic/tunnels.ts`,
`apps/api/src/features/tunnels/**`, `apps/web/src/domains/vpn/tunnels/**`, `apps/web/src/locales/*/tunnels.json`, `docs/user/vpn/tunnels.md`,
`test/topology/tunnels/**`. Shared files: one-line appends only, under your anchor (`domains/tunnels.ts` `TunnelsSchema` and
`Domains["tunnels"]` are shared with F-lisp); `descriptors/df6/**` and `semantic/tunnels-common.ts` are read-only.

- **TD-11c creator guard (2026-09-25):** every interface creator you add registers its VPP device class with `iface.RegisterKind` or provides the `interface/<name>` alias key (KeyProvider), and removes its own entry from the guard allowlist in the same branch; the allowlist only shrinks. Every descriptor also declares `RecordsNoOwnership()` or `CheckPersistent()` (TD-11b).

## Acceptance (paste the evidence)
- [ ] After commit `Retrieve()` == desired; `vppctl show gre tunnel`, `show ipip tunnel`, `show vxlan tunnel`, `show vxlan-gpe` list them with addresses
- [ ] `systemctl show vpp -p NRestarts` unchanged before/after your test run (pasted)
- [ ] Agent-restart simulation → tunnels and addresses back within 30 s (log excerpt)
- [ ] Rollback deletes the tunnels (Retrieve empty; L2TPv3 documented as V14 exception)
- [ ] Duplicate (src, dst, vni) VXLAN → 400 problem+json with a `pointer` to the second entry
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
IPsec protection of tunnels (P11); WireGuard (F-wireguard); SRv6 (F-srv6); LISP-GPE (F-lisp); MPLS tunnels (F-mpls-srmpls); bridge
domains and L2XC (F-bridge-l2); ERSPAN mirror sessions (F-loopback-bvi-gso-lldp-span); PPPoE client/server daemons; GTP-U hardware
offload; Tunnel Infra / PVTI / Geneve; tunnel dashboards (F-dashboard-prom-alarms).

## Open questions to surface, not to decide silently
GTP-U forwarding entries are effectively VPP-global per type (DF-6 review M2) — model as a single optional object or drop? Default: drop.
