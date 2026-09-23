# Task: F-igmp-mfib — IGMPv3, static mFIB, PIM via agent-side FRR sync (BIER optional)   (prepend 00-CONTEXT.md)

## Goal
IPv4 multicast in FAST MODE: IGMPv3 host/router mode and proxy, static multicast routes in the VPP mFIB, and PIM where FRR `pimd`
builds the multicast state and **the agent programs VPP mFIB entries** (V5 fallback — linux-nl does not sync multicast). Reference: TNSR
"IGMP / multicast"; VPP `igmp` plugin, `ip_mroute_add_del`; FRR pimd. WBS D2.9 (IGMPv3, PIM via FRR, mfib, BIER; T3 — "BIER has near-zero
commercial demand").

## Inputs to read first
- Schema: **no multicast model exists** → contract branch `contract/F-igmp-mfib` (additive): `routing.multicast{igmp{interfaces{<if>:
  {mode: host|router, joins[{group, sources[]}]}}, ssmRanges[], proxies{<vrf>: {upstream, downstream[]}}}, mroutes[{vrf, group, source?,
  paths[{interface, flags: accept|forward}]}], pim{interfaces[], rp[{address, groups[]}]}}` + proto; `docs/status/tasks/F-igmp-mfib-contract.md`.
- IGMP descriptors (DF-7, **branch**: `git show task/DF-7:docs/agent/descriptors/igmp.md`): `igmp.interface` (write-only), `igmp.listen`
  (INCLUDE only, ≥ 1 source — VPP 26.06 has no EXCLUDE), global `igmp.group-prefix` (write-only, globals owner only, D-071),
  `igmp.proxy-device`, `igmp.proxy-downstream` (write-only), `igmp.WatchEvents`; V20: `igmp_group_prefix_dump` answers with the wrong id.
- `apps/agent/binapi/ip/` — `ip_mroute_add_del`, `ip_mroute_dump`, `ip_mtable_dump` (verified present); `mfib_types`; `apps/agent/binapi/bier/`
  (`bier_table_add_del`, `bier_route_add_del`, `bier_imp_add`, `bier_disp_*`). New descriptors `mfib.route` (key `mfib.route/<vrf>/<group>/<source>`)
  with Retrieve from `ip_mroute_dump` filtered to your owner's tables/claims (D-071 claim store; use `descriptors/dfkit`).
- RF-1 FRR framework (`renderers/frr/README.md`, `section.go`, `state.go`, `frrtest`), D-056 (needs RF-1, not linux-cp); `docs/vpp-code-track.md` V5, V20.

## Scope — build exactly this
Files you own: `apps/agent/internal/descriptors/{igmp,mfib,bier}/**`, `apps/agent/internal/renderers/frr/pim/**`, `apps/agent/internal/frrsync/pim/**`,
`docs/agent/descriptors/{igmp,mfib,bier}.md`, `docs/agent/renderers/frr-pim.md`, `apps/agent/internal/agent/project_igmp_mfib*.go`, `apps/api/src/features/igmp-mfib/**`,
`apps/web/src/domains/routing/igmp-mfib/**`, `apps/web/src/locales/*/igmp-mfib.json`, `docs/user/routing/igmp-mfib.md`, `test/topology/igmp-mfib/**`.
Shared files: one-line appends only.
1. **Schema**: groups in 224.0.0.0/4 (not 224.0.0.0/24 link-local); SSM joins need sources inside `ssmRanges` (default 232/8); host joins only on
   host-mode interfaces; mroute accept interface ≠ forward interfaces; interfaces exist.
2. **Agent**: project IGMP onto DF-7 objects; new `mfib.route` descriptor (Create/Update/Delete via `ip_mroute_add_del`, Retrieve via
   `ip_mroute_dump`, owned tables only; delete checks existence first); FRR `pim` section (`ip pim` on interfaces, `ip pim rp`);
   **`frrsync/pim`**: poll `show ip mroute json`, translate (S,G)/(*,G) + OIL to `mfib.route` objects in a separate owner scope, feed them
   through the scheduler. Unit tests on the fake; one host integration check with prefixed VRF tables.
3. **API**: pointer routes; `GET /api/v1/state/routing/multicast/{groups,mroutes,pim-neighbors}`; IGMP events on the WS topic.
4. **UI**: Routing → Multicast: IGMP interfaces/joins, static mroutes, PIM RP/interfaces, live groups grid; en + fa; screenshot.
5. **Docs**: `docs/user/routing/igmp-mfib.md` (SSM host join, static (S,G), PIM-SM with static RP + V5 note).
6. **BIER — only if steps 1–5 are green inside the time box**: `bier.table` + `bier.route` descriptors with fake-client tests, no UI/API; otherwise
   record as not built in your status file.

## Acceptance (paste the evidence)
- [ ] `vppctl show ip mfib` contains the static (S,G) with accept/forward flags; `show igmp config` shows the joins; rollback removes both (Retrieve / write-only rule)
- [ ] PIM: netns FRR pimd peer (own pathspace) → an (S,G) learned by FRR appears in the VPP mFIB via frrsync (pasted), or the reason it cannot on the shared host + fake test
- [ ] Agent-restart simulation → IGMP + mroutes back within 30 s (write-only objects re-applied once per boot identity, D-076/D-080)
- [ ] Join 239.1.1.1 without sources (EXCLUDE) → 400 problem+json with `pointer` ("VPP supports INCLUDE only")
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
IPv6 multicast / MLD / pim6d; MSDP; multicast over tunnels (F-tunnels); MPLS multicast (F-mpls-srmpls); BIER UI/API; unicast routing
protocols (P12, F-ospf, F-isis-rip); any VPP C change or restart.

## Open questions to surface, not to decide silently
Whether BIER stays in the product at all (T3); the globals owner for the SSM range (`igmp.group-prefix`) on a real box.
