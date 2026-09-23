# Descriptors — l3xc plugin (DF-1)

Package `apps/agent/internal/descriptors/l3xc`, model `l3xc_model.proto` (agent-internal stand-in, D-055).
`l3xc.Register(r, client, owner)`.

| Object type (descriptor) | Key | Depends on | VPP messages | Update | Notes / limitations |
|---|---|---|---|---|---|
| `l3xc.l3xc` | `l3xc.l3xc/<rx interface id>/<ip4\|ip6>` | rx interface key; every path interface key; `vrf/<table id>` for every path with a non-zero table (Optional=false) | `l3xc_update` (replaces the path list), `l3xc_del`; Retrieve `l3xc_dump` | paths in place (`l3xc_update` is add-or-replace); interface / af change = ErrRecreate | Every packet received on the interface is forwarded via the paths (next-hop + interface, or table lookup), bypassing the FIB. Paths are canonical and `Normalize` (P05 `Normalizer`) produces that form from any desired value (review M4): references as `interface/<name>`, next-hop via `netip`, weight 0 → 1 (VPP stores 0 as 1, fib_api.c), sorted by (next_hop, interface, table); weight / preference ≤ 255. l3xc has no tag: an l3xc is ours iff its rx interface is ours (tagged, or untagged and claimed in the `iface.ClaimStore`, one claim per address family). The table dependency key `vrf/<id>` is P05 core's VRF key (`core.VRFName = "vrf"` on task/P05; Q1 resolved). Interface fields keep DF-1's full creator keys; consumers of the l3xc object itself do not need them. |
