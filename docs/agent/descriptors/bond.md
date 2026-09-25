# Descriptors — bond plugin (DF-1, F-bonding)

Package `apps/agent/internal/descriptors/bond`, models `bond_model.proto` (agent-internal stand-in, D-055) and, for
F-bonding's weight, a dfkit spec (D-077). `bond.Register(r, client, owner)` registers DF-1's two descriptors;
`bond.NewWeight` is registered by the product wiring next to them (`internal/subsystems/bonding.go`). Interface keys: see
[interface.md](interface.md). The configuration model is `interfaces.<BondEthernet<id>>.bond` (F-bonding,
`internal/desired/bond.go`, [user doc](../../user/interfaces/bonding.md)).

| Object type (descriptor) | Key | Depends on | VPP messages | Update | Notes / limitations |
|---|---|---|---|---|---|
| `bond.bond` | `bond.bond/<name>` | — | `bond_create2`, `bond_delete`, `sw_interface_tag_add_del`; Retrieve `sw_bond_interface_dump` (+ `sw_interface_dump` for the owner tag) | ErrRecreate (everything is creation-time in VPP) | mode lacp / xor / round-robin / active-backup / broadcast; lb l2 / l23 / l34 for xor and lacp. VPP **forces** lb for the other modes (rr → round-robin, ab → active-backup, broadcast → broadcast) and reports the forced value, so the model must carry it: a mismatching lb is rejected at Create (host-verified). `numa_only`, `id` (→ `BondEthernet<id>`). Not modelled: `enable_gso` (not reported by `sw_bond_interface_details`, would never round-trip) and the MAC (use `interface.mac-address`). |
| `bond.member` | `bond.member/<bond name>/<member name>` | bond reference, member interface reference (`interface/<name>` canonical) | `bond_add_member` (`is_passive`, `is_long_timeout`), `bond_detach_member`; Retrieve `sw_member_interface_dump` per owned bond | ErrRecreate | Member: an owned hardware interface or a physical (untagged) NIC via `interface/<name>` — the membership is then ours through the `iface.ClaimStore` (`TestBondPhysicalMembers`); another owner's interface is refused. If tagging a new bond fails it is deleted again (review M3). Meta `{SwIfIndex, Bond}`. F-bonding fix round 1 (review F4): Create refuses, before `bond_add_member`, a member that is a sub-interface, a loopback or a bond, or has no L2 address (tunnels and other L3 classes) — VPP 26.06 checks only "not a bond" and then copies the member's hardware address. |
| `bond.member-weight` (F-bonding) | `bond.member-weight/<bond name>/<member name>` | `bond.member/<bond>/<member>` | `sw_interface_set_bond_weight`; Retrieve `sw_member_interface_dump.weight` of the memberships `bond.member` reports | in place | Value: dfkit spec `{bond, interface, weight}` (alias references, normalised). VPP keeps the weight in the membership (`member_if_t`, zeroed by `bond_add_member`), so the object depends on the membership and is re-created with it; 0 is VPP's "never set" and is never desired (Create refuses it, Retrieve skips it) — a real Retrieve, not write-only (D-063/D-076). VPP accepts it on active-backup bonds only (INVALID_ARGUMENT otherwise). Delete sets 0 while the membership still exists (re-resolved by name first, D-071); a detached member took its weight with it. |

`bond.bond` also implements the scheduler's `KeyProvider` (F-bonding, D-125): it provides `interface/<name>`, so a plan that
only deletes (a rollback) orders the bond's address, attributes and members before `bond.bond` — the alias object itself
is observe-only and never planned. Sub-interfaces of the bond are ordered through their parent the same way; the
attributes of a sub-interface (admin state, addresses) are not (their alias `interface/<bond>.<id>` has no planned
provider — TD-11c 3.1c), so they are removed in an earlier commit than the sub-interface.

Ownership declarations (TD-11b guard): `bond.bond` and `bond.member-weight` declare `RecordsNoOwnership()` (the bond is ours by
its owner tag; the weight lives in a membership bond.member owns); `bond.member` declares `CheckPersistent()` (claims on
untagged member NICs in the owner's `iface.ClaimStore`, which must be the persisted one).

Ownership: bond interfaces carry the owner tag like every interface; members are retrieved only for
owned bonds and only when the member interface itself is owned. The live view of all three is the agent's `BondState` RPC
(`internal/agent/rpc_bonding.go`: `sw_bond_interface_dump`, `sw_member_interface_dump`, `sw_interface_lacp_dump`).

Host evidence (F-bonding, `test/topology/bonding`, slot 6): LACP bond `BondEthernet6000` (lb l34, 2 fixture-tap members,
one passive) and active-backup `BondEthernet6001` (member weight 200) through the real agent + API; Retrieve == running;
restart simulation (members, addresses, bonds deleted behind the agent's back) converged in 0.42 s; rollback deleted
weight → members → admin state → address → bond; `docs/status/tasks/F-bonding.md`.
