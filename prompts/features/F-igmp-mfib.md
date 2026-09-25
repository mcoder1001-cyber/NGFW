# Task: F-igmp-mfib — IGMPv3, static mFIB, PIM via agent-side FRR sync (BIER optional)   (prepend 00-CONTEXT.md)

> Refreshed 2026-09-24 on `task/prep-rest` against main (DF-7 merged), `task/P08`, `task/W-seed` and the VPP 26.06 igmp / linux-cp
> source. Your TASK ENVELOPE (`docs/status/tasks/F-igmp-mfib.envelope.md`) wins for branch, files, numbers, anchors and process.

## Goal
IPv4 multicast in FAST MODE: IGMPv3 host/router mode and proxy, static multicast routes in the VPP mFIB, and PIM where FRR `pimd`
builds the multicast state and **the agent programs VPP mFIB entries** (V5 fallback — linux-nl does not sync multicast). Reference: TNSR
"IGMP / multicast"; VPP `igmp` plugin, `ip_mroute_add_del`; FRR pimd. WBS D2.9 (IGMPv3, PIM via FRR, mfib, BIER; T3 — "BIER has near-zero
commercial demand").

## Inputs to read first
- Schema: **no multicast model exists** → additive contract, committed **first on your task branch** as separate `contract(schema): …` /
  `contract(proto): …` commits (never a `contract/` branch): `routing.multicast{igmp{interfaces{<if>: {mode: host|router, joins[{group,
  sources[]}]}}, ssmRanges[], proxies{<vrf>: {upstream, downstream[]}}}, mroutes[{vrf, group, source?, paths[{interface, flags:
  accept|forward}]}], pim{interfaces[], rp[{address, groups[]}]}}` + proto; `docs/status/tasks/F-igmp-mfib-contract.md`. Numbers from
  `docs/status/wave-BC-numbers.md`: RoutingConfig 16 `multicast`, EventKind 24 (IGMP group), 25 (PIM neighbour, optional).
- IGMP descriptors (DF-7, **merged**): `docs/agent/descriptors/igmp.md` + `apps/agent/internal/descriptors/igmp/` on main — `igmp.interface`
  (write-only), `igmp.listen` (INCLUDE only, ≥ 1 source — VPP 26.06 has no EXCLUDE), global `igmp.group-prefix` (write-only,
  `igmp.RegisterGlobals`, globals owner only, D-071), `igmp.proxy-device`, `igmp.proxy-downstream` (write-only), `igmp.WatchEvents`;
  V20: `igmp_group_prefix_dump` answers with the wrong id. **The IGMP host test is opt-in** (`VRX_DF7_IGMP_HOST=1`, alone, manager window —
  D-087/D-090: IGMP router-alert packets on loopbacks crashed VPP in `ip4_options_node_fn`, V22b).
- VPP 26.06 source facts (read-only): `src/plugins/igmp/igmp_input.c:441` — the igmp plugin claims IP protocol 2 for all local delivery,
  drops IGMP on interfaces without igmp config and handles only IGMPv3, so FRR pimd's own IGMP querier behind linux-cp never sees
  membership reports: receivers come from VPP IGMP (router mode, `igmp.WatchEvents`) or static joins — record it (`### V-new`), no C.
  PIM (IP proto 103) is not claimed by VPP and reaches the LCP tap through the ip4-punt arc; `lcp_router.c` installs a (*,224.0.0.0/24)
  accept mfib entry on LCP pairs — confirm PIM hellos arrive.
- `apps/agent/binapi/ip/` — `ip_mroute_add_del`, `ip_mroute_dump`, `ip_mtable_dump` (verified present); `mfib_types`; `apps/agent/binapi/bier/`
  (`bier_table_add_del`, `bier_route_add_del`, `bier_imp_add`, `bier_disp_*`). New descriptors `mfib.route` (key `mfib.route/<vrf>/<group>/<source>`)
  with Retrieve from `ip_mroute_dump` filtered to your owner's tables/claims (D-071 claim store; use `descriptors/dfkit`).
- RF-1 FRR framework (`renderers/frr/README.md`, `section.go`, `state.go`, `frrtest`), D-056 (FRR part needs RF-1). `frrsync/pim` translates
  OIL interfaces (Linux names) to VPP logical names through **P12's** LCP mapping (`internal/lcpmap`, reverse direction in an adapter in
  your package) and the VRX-side pimd runs over P12's LCP pairs — without P12 on main, build the translation against a fake mapper and keep
  the netns PIM step open (the prep report proposes P12 as a board dep). `docs/vpp-code-track.md` V5, V20, V22.
- Seam S1 (`docs/status/wave-BC-numbers.md`): the generic "plan and apply a scoped KV set under the transaction lock" hook, shared with
  F-mpls-ldp. `agent.go`/`service.go` are agent core — do not edit them; if the manager has not seeded the seam, keep the sync loop behind
  an interface in `frrsync/pim` with a fake apply in the tests and write the question. Seam S2 for `ip pim` interface lines.

## Scope — build exactly this
Files you own: see the envelope (`descriptors/{igmp,mfib,bier}/**` (igmp gap-only), `renderers/frr/pim/**`, `frrsync/pim/**`,
`desired/igmp_mfib*.go`, `subsystems/igmp_mfib*.go`, `agent/rpc_igmp_mfib*.go`, `ext/igmp-mfib*.ts`, `semantic/igmp-mfib*.ts`,
`features/igmp-mfib/**`, `domains/routing/igmp-mfib/**`, `igmp-mfib.json`, docs, `test/topology/igmp-mfib/**`). Shared files: one line
under your `// wave-BC: F-igmp-mfib` anchor only.
1. **Schema**: groups in 224.0.0.0/4 (not 224.0.0.0/24 link-local); SSM joins need sources inside `ssmRanges` (default 232/8); host joins only on
   host-mode interfaces; mroute accept interface ≠ forward interfaces; interfaces exist.
2. **Agent**: project IGMP onto DF-7 objects; new `mfib.route` descriptor (Create/Update/Delete via `ip_mroute_add_del`, Retrieve via
   `ip_mroute_dump`, owned tables only; delete checks existence first; mroutes before their table on every delete path — V15); FRR `pim`
   section (`ip pim` on interfaces, `ip pim rp`, order 480); **`frrsync/pim`**: poll `show ip mroute json`, translate (S,G)/(*,G) + OIL to
   `mfib.route` objects in a separate owner scope, feed them through the scheduler (seam S1). Unit tests on the fake; one host integration
   check with prefixed VRF tables (the IGMP part only in the opt-in window).
3. **API**: pointer routes; `GET /api/v1/state/routing/multicast/{groups,mroutes,pim-neighbors}` (`MulticastState` RPC); IGMP events on the WS topic.
4. **UI**: Routing → Multicast: IGMP interfaces/joins, static mroutes, PIM RP/interfaces, live groups grid; en + fa; screenshot.
5. **Docs**: `docs/user/routing/igmp-mfib.md` (SSM host join, static (S,G), PIM-SM with static RP + V5 note + the IGMP/pimd limit above).
6. **BIER — only if steps 1–5 are green inside the time box**: `bier.table` + `bier.route` descriptors with fake-client tests, no UI/API; otherwise
   record as not built in your status file.

## Acceptance (paste the evidence)
- [ ] `vppctl show ip mfib` contains the static (S,G) with accept/forward flags; `show igmp config` shows the joins (opt-in window); rollback removes both (Retrieve / write-only rule)
- [ ] PIM: netns FRR pimd peer (own pathspace) → an (S,G) learned by FRR appears in the VPP mFIB via frrsync (pasted), or the reason it cannot on the shared host + fake test
- [ ] Agent-restart simulation → IGMP + mroutes back within 30 s (write-only objects re-applied once per boot identity, D-076/D-080)
- [ ] Join 239.1.1.1 without sources (EXCLUDE) → 400 problem+json with `pointer` ("VPP supports INCLUDE only")
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
IPv6 multicast / MLD / pim6d; MSDP; multicast over tunnels (F-tunnels); MPLS multicast (F-mpls-srmpls); BIER UI/API; unicast routing
protocols (P12, F-ospf, F-isis-rip); any VPP C change or restart.

## Open questions to surface, not to decide silently
Whether BIER stays in the product at all (T3); the globals owner for the SSM range (`igmp.group-prefix`) on a real box; how PIM learns
receivers given that VPP's igmp plugin consumes IGMP (VPP router-mode events fed to pimd vs static joins only).
