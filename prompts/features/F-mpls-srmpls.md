# Task: F-mpls-srmpls — static MPLS, SR-MPLS, LDP via agent-side FRR sync   (prepend 00-CONTEXT.md)

## Goal
MPLS label switching end to end in FAST MODE: MPLS-enabled interfaces, static label routes / LSPs, label↔IP bindings, MPLS tunnels,
SR-MPLS policies + steering, and LDP where FRR `ldpd` computes the labels and **the agent programs them into VPP** (V5 fallback — linux-nl
syncs unicast only). Reference: TNSR "MPLS"; VPP `mpls`, `sr_mpls`; FRR ldpd. WBS D2.8 (label ops, LSP, MPLS-over-Ethernet, basic L3VPN, SR-MPLS; T2).

## Inputs to read first
- Schema: **no MPLS model exists** → contract branch `contract/F-mpls-srmpls` first (additive, commit `contract(schema): mpls`):
  `routing.mpls{interfaces[ifName], tables{<id>: {}}, labelRoutes[{table, label, eos, paths[{nextHop?, interface?, outLabels[], weight}]}],
  ipBindings[{label, vrf, prefix}], tunnels{<name>: {paths[…], l2Only?}}, ldp{routerId, transportAddress, interfaces[], neighbors?},
  sr{policies{<bsid>: {segmentLists[{labels[], weight}], spray?}}, steering[{vrf, prefix, bsid, vpnLabel?}]}}` + proto;
  `docs/status/tasks/F-mpls-srmpls-contract.md`. Tell the manager; continue against your branch.
- MPLS descriptors (DF-7, **branch** until merged: `git show task/DF-7:docs/agent/descriptors/mpls.md`): `mpls-table/<id>`, `mpls-interface/<if>`,
  `mpls-route/<t>/<label>/<eos>`, `mpls-ip-bind` (write-only, D-063/D-076), `mpls-tunnel/<name>`; **MPLS table 0 is VPP-global** →
  only the globals owner declares `mpls-table/0` (D-071); on the shared host the enable/bind parts run only with `VRX_DF7_GLOBALS=1` under
  `flock -x /run/lock/vrx-globals.lock` (D-082).
- SR-MPLS (DF-6, on main): `docs/agent/descriptors/sr_mpls.md` — `sr-mpls.policy/<bsid>`, `sr-mpls.steering`, `sr-mpls.endpoint-color`;
  **no dump in binapi (V14)** → write-only with exact presence probes; keep segment lists sorted (D-074); delete checks existence first (D-074).
- `apps/agent/binapi/{mpls,sr_mpls}` — confirm every message (`mpls_route_add_del`, `mpls_table_add_del`, `mpls_ip_bind_unbind`,
  `mpls_tunnel_add_del`, `sw_interface_set_mpls_enable`, `sr_mpls_policy_add/mod/del`, `sr_mpls_steering_add_del`).
- RF-1 FRR framework (`renderers/frr/README.md`, `section.go`, `state.go`): LDP is an FRR section + a state reader (D-056: needs RF-1, not linux-cp
  for the sync). `docs/vpp-code-track.md` V5 (this task *is* the fallback), V14, V15 (delete routes before tables).

## Scope — build exactly this
Files you own: `apps/agent/internal/descriptors/{mpls,sr_mpls}/**`, `apps/agent/internal/renderers/frr/ldp/**`, `apps/agent/internal/frrsync/ldp/**`,
`docs/agent/descriptors/{mpls,sr_mpls}.md`, `docs/agent/renderers/frr-ldp.md`, `apps/agent/internal/agent/project_mpls_srmpls*.go`, `apps/api/src/features/mpls-srmpls/**`,
`apps/web/src/domains/routing/mpls-srmpls/**`, `apps/web/src/locales/*/mpls-srmpls.json`, `docs/user/routing/mpls-srmpls.md`, `test/topology/mpls-srmpls/**`.
Shared files: one-line appends only.
1. **Schema**: labels 16–1048575 (0–15 reserved); out-label stack ≤ 16; unique (table,label,eos); tunnel/steering references exist; BSID not
   used as a static label route; LDP interfaces are MPLS-enabled.
2. **Agent**: project `routing.mpls` onto the DF-7 + DF-6 descriptors (dependency order interface → table → routes/bindings → tunnels →
   SR policy → steering). FRR `ldp` section (`mpls ldp`, router-id, `address-family ipv4 … interface …`). **`frrsync/ldp`**: poll
   `show mpls table json` (and `show mpls ldp binding json`) from FRR, translate to `mpls-route` objects with an `frr-ldp` owner scope,
   hand them to the scheduler as a second desired-state source (never write VPP outside the scheduler); withdraw → delete. Unit tests on
   the fake client; ONE host integration check (prefixed tables, globals-lock rules above).
3. **API**: pointer routes; `GET /api/v1/state/routing/mpls/{fib,tunnels,ldp}` (MPLS FIB paged, LDP neighbours/bindings from FRR).
4. **UI**: Routing → MPLS: interfaces, label routes, tunnels, SR policies/steering tabs; LDP neighbours grid; en + fa; screenshot.
5. **Docs**: `docs/user/routing/mpls-srmpls.md` (static LSP, SR-MPLS policy + steering, LDP with the V5 note, CLI equivalent).

## Acceptance (paste the evidence)
- [ ] `vppctl show mpls fib <table>` shows the static label routes and `show sr mpls policies` the policy; after rollback neither remains (Retrieve / probe)
- [ ] LDP: netns FRR ldpd peer (own pathspace) → learned bindings appear as `mpls-route` objects in VPP (`show mpls fib`) — or, without table 0 on the shared host, the fake-client test + the `VRX_DF7_GLOBALS=1` run pasted
- [ ] Agent-restart simulation → label routes + SR policy back within 30 s (write-only objects re-applied once per boot identity, D-076/D-080)
- [ ] Label 5 / duplicate (table,label,eos) → 400 problem+json with `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
L3VPN / VPNv4 BGP labels (P12 follow-up), RSVP-TE, SR-TE color automated steering, SRv6 (F-srv6), multicast (F-igmp-mfib), MPLS QoS/EXP
marking (F-qos-flat), OSPF/IS-IS segment-routing extensions (F-ospf, F-isis-rip), any VPP C change or restart.

## Open questions to surface, not to decide silently
Who is the globals owner for MPLS table 0 in production (proposed: product agent always declares `mpls-table/0` when `routing.mpls` is non-empty);
the kernel needs `mpls_router`/`mpls_iptunnel` modules for ldpd — do not load them on the shared host; ask.
