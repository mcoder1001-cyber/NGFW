# Task: F-mpls-srmpls — static MPLS and SR-MPLS   (prepend 00-CONTEXT.md)

> LDP is split out to **F-mpls-ldp** (D-085): the FRR `ldpd` section, the agent-side FRR→VPP label sync (V5) and the LDP state/UI
> belong to that task, which depends on this one. Build the `routing.mpls` model, projection and page so that F-mpls-ldp can add
> `routing.mpls.ldp` and one "LDP" tab additively. Do not build any LDP part here.

## Goal
MPLS label switching end to end in FAST MODE. It covers MPLS-enabled interfaces, static label routes / LSPs, label↔IP
bindings, MPLS tunnels, and SR-MPLS policies + steering. Reference: TNSR "MPLS"; VPP `mpls`, `sr_mpls`. WBS D2.8 (label ops,
LSP, MPLS-over-Ethernet, basic L3VPN, SR-MPLS; T2).

## Inputs to read first
- **Schema.** No MPLS model exists, so create contract branch `contract/F-mpls-srmpls` first (additive, commit
  `contract(schema): mpls`) with this model:
  `routing.mpls{interfaces[ifName], tables{<id>: {}}, labelRoutes[{table, label, eos, paths[{nextHop?, interface?, outLabels[], weight}]}],
  ipBindings[{label, vrf, prefix}], tunnels{<name>: {paths[…], l2Only?}},
  sr{policies{<bsid>: {segmentLists[{labels[], weight}], spray?}}, steering[{vrf, prefix, bsid, vpnLabel?}]}}` + proto. The
  `ldp` key is added later by F-mpls-ldp. Keep `routing.mpls` a plain object so that addition stays additive.

  Also write `docs/status/tasks/F-mpls-srmpls-contract.md`. Tell the manager, then continue against your branch.
- **MPLS descriptors** (DF-7, merged: `docs/agent/descriptors/mpls.md`): `mpls-table/<id>`, `mpls-interface/<if>`,
  `mpls-route/<t>/<label>/<eos>`, `mpls-ip-bind` (write-only, D-063/D-076), `mpls-tunnel/<name>`.
  - **MPLS table 0 is VPP-global**, so only the globals owner declares `mpls-table/0` (D-071).
  - On the shared host, the enable/bind parts run only with `VRX_DF7_GLOBALS=1` under `flock -x /run/lock/vrx-globals.lock`
    (D-082).
- **SR-MPLS** (DF-6, on main): `docs/agent/descriptors/sr_mpls.md` — `sr-mpls.policy/<bsid>`, `sr-mpls.steering`,
  `sr-mpls.endpoint-color`.
  - **No dump in binapi (V14)**, so these are write-only with exact presence probes.
  - Keep segment lists sorted (D-074), and check existence before delete (D-074).
- `apps/agent/binapi/{mpls,sr_mpls}`: confirm every message (`mpls_route_add_del`, `mpls_table_add_del`, `mpls_ip_bind_unbind`,
  `mpls_tunnel_add_del`, `sw_interface_set_mpls_enable`, `sr_mpls_policy_add/mod/del`, `sr_mpls_steering_add_del`).
- `docs/vpp-code-track.md` V14 and V15 (delete routes before tables).

## Scope — build exactly this
**Files you own:**
- `apps/agent/internal/descriptors/{mpls,sr_mpls}/**`
- `docs/agent/descriptors/{mpls,sr_mpls}.md`
- `apps/agent/internal/agent/project_mpls_srmpls*.go`
- `apps/api/src/features/mpls-srmpls/**`
- `apps/web/src/domains/routing/mpls-srmpls/**`
- `apps/web/src/locales/*/mpls-srmpls.json`
- `docs/user/routing/mpls-srmpls.md`
- `test/topology/mpls-srmpls/**`

**Shared files:** one-line appends only.

**Not yours:** `renderers/frr/ldp/**`, `frrsync/ldp/**` and `docs/agent/renderers/frr-ldp.md` belong to F-mpls-ldp.

1. **Schema** — semantic rules:
   - labels are 16–1048575 (0–15 are reserved);
   - the out-label stack has ≤ 16 labels;
   - (table, label, eos) is unique;
   - tunnel and steering references exist;
   - a BSID is not used as a static label route.
2. **Agent** — project `routing.mpls` onto the DF-7 + DF-6 descriptors. Dependency order: interface → table →
   routes/bindings → tunnels → SR policy → steering.
   - Unit tests on the fake client.
   - ONE host integration check (prefixed tables, globals-lock rules above).
3. **API** — pointer routes, plus `GET /api/v1/state/routing/mpls/{fib,tunnels}` (MPLS FIB paged).
4. **UI** — Routing → MPLS with tabs for interfaces, label routes, tunnels, and SR policies/steering. en + fa; screenshot.
   Keep the tab list a simple array, so F-mpls-ldp appends its "LDP" tab with one entry.
5. **Docs** — `docs/user/routing/mpls-srmpls.md`: static LSP, SR-MPLS policy + steering, CLI equivalent.

## Acceptance (paste the evidence)
- [ ] `vppctl show mpls fib <table>` shows the static label routes and `show sr mpls policies` shows the policy; after rollback
      neither remains (Retrieve / probe)
- [ ] Agent-restart simulation → label routes + SR policy back within 30 s (write-only objects re-applied once per boot identity, D-076/D-080)
- [ ] Label 5 / duplicate (table,label,eos) → 400 problem+json with `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
- **LDP** — all of it belongs to F-mpls-ldp (D-085):
  - FRR `mpls ldp` section;
  - `frrsync` / FRR→VPP label sync (V5);
  - `routing.mpls.ldp` schema;
  - LDP state routes, neighbours grid and LDP tab.
- L3VPN / VPNv4 BGP labels (P12 follow-up).
- RSVP-TE.
- SR-TE color automated steering.
- SRv6 (F-srv6).
- Multicast (F-igmp-mfib).
- MPLS QoS/EXP marking (F-qos-flat).
- OSPF/IS-IS segment-routing extensions (F-ospf, F-isis-rip).
- Any VPP C change or restart.

## Open questions to surface, not to decide silently
Who is the globals owner for MPLS table 0 in production? Proposed: the product agent always declares `mpls-table/0` when
`routing.mpls` is non-empty. F-mpls-ldp relies on your answer.
