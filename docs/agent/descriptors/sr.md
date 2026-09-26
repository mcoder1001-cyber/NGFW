# sr (SRv6) descriptors (DF-6, WBS D6.7 / D2.8)

Package `apps/agent/internal/descriptors/sr`. Messages only from `apps/agent/binapi/sr` (+ `sr_types`, `ip`). Shared rules: [df6.md](df6.md).
SR objects carry no owner tag: an SR object is ours only through the ClaimStore record of our own Create (D-071,
[df6.md](df6.md)); Create never takes over an existing unclaimed SID / policy / steering key.

| Object type | Descriptor / key | Create / Delete | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| Local SID (END, END.X, END.T, END.DX2/DX4/DX6, END.DT4/DT6) | `sr.localsid` · `sr.localsid/<sid>` | `sr_localsid_add_del` (is_del) | `sr_localsids_dump` (classic behaviours only) | `ErrRecreate` | `vrf/<fib_table>`, `vrf/<lookup_table>`, `interface/<interface>` |
| Policy (default / spray / TEF, encap / insert, ≥1 SID list) | `sr.policy` · `sr.policy/<bsid>` | `sr_policy_add_v2` + `sr_policy_mod_v2` (ADD) per further list / `sr_policy_del` | `sr_policies_v2_dump` | `ErrRecreate` (steering recreated by the scheduler) | `vrf/<fib_table>` |
| Steering (L2 interface or L3 prefix + table → BSID) | `sr.steering` · `sr.steering/l2/<if>` or `sr.steering/<ipv4\|ipv6>/<table>/<prefix>` | `sr_steering_add_del` (by BSID); Delete only while the entry still points at our BSID | `sr_steering_pol_dump` | our entry: new BSID re-points in place; else `ErrRecreate` | `sr.policy/<bsid>`, `vrf/<table>` or `interface/<if>` |
| Encap source (global, **globals owner only**) | `sr.encap-source` · `…/global` | `sr_set_encap_source`; Delete resets to `::` | **write-only** (no getter) | set in place | — |
| Encap hop limit (global, **globals owner only**) | `sr.encap-hop-limit` · `…/global` | `sr_set_encap_hop_limit`; Delete resets to 64 | **write-only** | set in place | — |

Models: `sr.LocalSid{sid, behavior, end_psp, fib_table, interface, next_hop, lookup_table}`,
`sr.Policy{bsid, type, encap, fib_table, sid_lists[{sids ≤16, weight}], encap_src}`,
`sr.Steering{traffic_type L2|IPV4|IPV6, prefix, table_id, interface, bsid}`, `sr.EncapSource{address}`, `sr.EncapHopLimit{hop_limit}`.

Notes / limitations / guards
- `encap_src` is mandatory for encap policies and forbidden for insert policies: with `::` VPP substitutes the global
  encap source, which cannot be read back, so an empty value would never diff clean.
- VPP 26.06 uses `fib_table_find()`'s `~0` unchecked for the policy BSID table, the steering table and the localsid
  table on delete; every descriptor checks the table (right family) with `ip_table_dump` first (`df6.ErrNoSuchTable`).
- VPP deletes a policy even while steering points at it (dangling steering); `sr.policy` Delete refuses with
  `sr.ErrPolicyInUse`.
- Deleting an IP table that still holds SR-sourced routes leaks the routes (seen on the host with SR-MPLS; repaired) —
  the scheduler's dependency order (steering before vrf) prevents it.
- uSID behaviours (`un`, `ua`, `sr_localsid_add_del_v2` locator lengths) and plugin behaviours (srv6-ad/am/as: **no
  binapi generated**) are not modelled; their SIDs are ignored by Retrieve. `srv6-mobile` (binapi `sr_mobile`) is not
  built — see DF-6-questions Q6. Path tracing (`sr_pt`) out of scope.

Product wiring and additions (F-srv6)
- `routing.srv6` is projected onto these descriptors by `apps/agent/internal/desired/srv6.go` (docs/contracts/proto.md
  "F-srv6: Srv6State"); the family is registered by `subsystems/srv6.go` with `df6.WithClaims(Wiring.PairClaims("df6"))`
  (claims persisted in `claims-df6-<owner>.json`, TD-11b) and `df6.WithGlobalsOwner(Env.GlobalsOwner)` (D-071).
- `sr.Register` registers the two globals wrapped in `sr.Global`: the df6 setter / require descriptor plus the TD-11b
  declaration `RecordsNoOwnership` (a VPP-wide setting records nothing; `sr_set_encap_source` /
  `sr_set_encap_hop_limit` overwrite one variable, so no D-076 applied-once record is needed) and its `Unwrap` /
  `DeleteOnAbsence` forwarded. On a non-owner the require variant of a write-only global always fails
  (`df6.ErrNotGlobalsOwner`): a slot agent gives each encapsulating policy its own `encap_src` instead.
- `sr.LocalSidCounters` reads `sr_localsids_with_packet_stats_dump` (good/bad packets and bytes per SID, keyed by
  canonical SID) for the `Srv6State` RPC; status only, never part of Retrieve.
- Delete order in the product: the scheduler removes steering (depends on `sr.policy/<bsid>` and the table) before
  its policy, and every SR object before its VRF (`vrf/<id>`), so no SR FIB entry stays in a deleted table (V15).
