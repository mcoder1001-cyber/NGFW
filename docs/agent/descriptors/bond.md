# Descriptors — bond plugin (DF-1)

Package `apps/agent/internal/descriptors/bond`, models `bond_model.proto` (agent-internal stand-in, D-055).
`bond.Register(r, client, owner)`. Interface keys: see [interface.md](interface.md).

| Object type (descriptor) | Key | Depends on | VPP messages | Update | Notes / limitations |
|---|---|---|---|---|---|
| `bond.bond` | `bond.bond/<name>` | — | `bond_create2`, `bond_delete`, `sw_interface_tag_add_del`; Retrieve `sw_bond_interface_dump` (+ `sw_interface_dump` for the owner tag) | ErrRecreate (everything is creation-time in VPP) | mode lacp / xor / round-robin / active-backup / broadcast; lb l2 / l23 / l34 for xor and lacp. VPP **forces** lb for the other modes (rr → round-robin, ab → active-backup, broadcast → broadcast) and reports the forced value, so the model must carry it: a mismatching lb is rejected at Create (host-verified). `numa_only`, `id` (→ `BondEthernet<id>`). Not modelled: `enable_gso` (not reported by `sw_bond_interface_details`, would never round-trip) and the MAC (use `interface.mac-address`). |
| `bond.member` | `bond.member/<bond name>/<member name>` | bond reference, member interface reference (`interface/<name>` canonical) | `bond_add_member` (`is_passive`, `is_long_timeout`), `bond_detach_member`; Retrieve `sw_member_interface_dump` per owned bond | ErrRecreate | Member: an owned hardware interface or a physical (untagged) NIC via `interface/<name>` — the membership is then ours through the `iface.ClaimStore` (`TestBondPhysicalMembers`); another owner's interface is refused. If tagging a new bond fails it is deleted again (review M3). Meta `{SwIfIndex, Bond}`. |

Ownership: bond interfaces carry the owner tag like every interface; members are retrieved only for
owned bonds and only when the member interface itself is owned.
