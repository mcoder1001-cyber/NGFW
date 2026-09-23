# Descriptors — l3xc plugin (DF-1)

Package `apps/agent/internal/descriptors/l3xc`, model `l3xc_model.proto` (agent-internal stand-in, D-055).
`l3xc.Register(r, client, owner)`.

| Object type (descriptor) | Key | Depends on | VPP messages | Update | Notes / limitations |
|---|---|---|---|---|---|
| `l3xc.l3xc` | `l3xc.l3xc/<rx interface id>/<ip4\|ip6>` | rx interface key; every path interface key; `vrf/<table id>` for every path with a non-zero table (Optional=false) | `l3xc_update` (replaces the path list), `l3xc_del`; Retrieve `l3xc_dump` | paths in place (`l3xc_update` is add-or-replace); interface / af change = ErrRecreate | Every packet received on the interface is forwarded via the paths (next-hop + interface, or table lookup), bypassing the FIB. Paths are canonical: next-hop via `netip`, sorted by (next_hop, interface, table) — `l3xc.SortPaths`; weight / preference ≤ 255. l3xc has no tag: an l3xc is ours iff its rx interface is owned. The table dependency key `vrf/<id>` follows the DF task prompts; P05 core has not published its VRF descriptor name yet — see DF-1-questions.md Q1 (a one-line change in `l3xc.TableDescriptor`). |
