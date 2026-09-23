# sr (SRv6) descriptors (DF-6, WBS D6.7 / D2.8)

Package `apps/agent/internal/descriptors/sr`. Messages only from `apps/agent/binapi/sr` (+ `sr_types`, `ip`). Shared rules: [df6.md](df6.md).
SR objects carry no owner tag: `df6.Scope` attributes them by address (SID / BSID in the slot's `fdNN::/16`; production: nil scope).

| Object type | Descriptor / key | Create / Delete | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| Local SID (END, END.X, END.T, END.DX2/DX4/DX6, END.DT4/DT6) | `sr.localsid` · `sr.localsid/<sid>` | `sr_localsid_add_del` (is_del) | `sr_localsids_dump` (classic behaviours only) | `ErrRecreate` | `vrf/<fib_table>`, `vrf/<lookup_table>`, `interface/<interface>` |
| Policy (default / spray / TEF, encap / insert, ≥1 SID list) | `sr.policy` · `sr.policy/<bsid>` | `sr_policy_add_v2` + `sr_policy_mod_v2` (ADD) per further list / `sr_policy_del` | `sr_policies_v2_dump` | `ErrRecreate` (steering recreated by the scheduler) | `vrf/<fib_table>` |
| Steering (L2 interface or L3 prefix + table → BSID) | `sr.steering` · `sr.steering/l2/<if>` or `sr.steering/<ipv4\|ipv6>/<table>/<prefix>` | `sr_steering_add_del` (by BSID) | `sr_steering_pol_dump` | new BSID re-points in place (VPP add on existing key); else `ErrRecreate` | `sr.policy/<bsid>`, `vrf/<table>` or `interface/<if>` |
| Encap source (global) | `sr.encap-source` · `…/global` | `sr_set_encap_source`; Delete resets to `::` | **write-only** (no getter) | set in place | — |
| Encap hop limit (global) | `sr.encap-hop-limit` · `…/global` | `sr_set_encap_hop_limit`; Delete resets to 64 | **write-only** | set in place | — |

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
