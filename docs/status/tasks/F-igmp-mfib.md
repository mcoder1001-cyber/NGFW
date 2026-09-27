# F-igmp-mfib — IPv4 multicast (IGMP / mFIB / PIM)

Merged in PR #52 (2026-09-27). In-container slice; the data-plane is F-igmp-mfib-host.

## Delivered

- **Contract**: `routing.multicast` (igmp interfaces/joins/ssmRanges/proxies, static mroutes, pim interfaces/rp) with
  semantic validation (real multicast groups, INCLUDE-only joins, SSM source requirement, one accept interface,
  interfaces exist). Proto `RoutingConfig.multicast=16` + messages + `EventKind 24`; drift guard green; YANG regenerated.
- **API**: `MulticastState` RPC + agent client; `GET /api/v1/state/routing/multicast/{groups,mroutes,pim-neighbors}`
  (agentError + empty when the agent lacks multicast); `multicast.events` WS topic + relay mapping. Config via the
  generic pointer routes. Fake agent implements MulticastState over the applied document; e2e green.
- **Web**: Routing → Multicast live view (groups, mFIB, PIM neighbours); en/fa.
- **Docs**: `docs/user/routing/igmp-mfib.md`.

## Deferred → F-igmp-mfib-host (needs lab VPP/host)

- VPP mFIB programming: the `mfib.route` descriptor over `ip_mroute_add_del`/`ip_mroute_dump` (owned tables, delete
  mroutes before the table — V15).
- Projection of `routing.multicast.igmp` onto the DF-7 IGMP objects; feeding VPP IGMP `WatchEvents` into
  `multicast.events` (EVENT_KIND_IGMP_GROUP_CHANGED).
- FRR `pim` renderer section + `frrsync/pim`: poll `show ip mroute json`, translate (S,G)/(*,G)+OIL to `mfib.route`
  via P12's LCP mapping through seam S1.
- BIER (T3) — optional, likely record as not built.
- The VPP-engine IGMP host test is opt-in and runs alone in a manager window (D-087/D-090, V22b).

## Notes

- VPP 26.06's IGMP plugin consumes IGMP and is IGMPv3/INCLUDE-only; receivers come from VPP router-mode events or
  static joins, not from pimd's own querier (recorded as V-new in the prep report).
- The state RPC and the fake agent already return groups/mroutes derived from the applied document, so the API and web
  are exercised end to end without the data plane.
