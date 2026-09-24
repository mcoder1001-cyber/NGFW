# Bond interfaces (link aggregation, LACP)

A bond combines several NICs into one logical interface, `BondEthernet<id>`, for more bandwidth and for redundancy. VRX
builds it with VPP's bond and LACP plugins. Reference behaviour: TNSR "Bond interfaces / LACP".

| mode | what it does | `loadBalance` | member options |
|---|---|---|---|
| `lacp` | 802.3ad: the members negotiate with the switch (LACPDUs); only members in sync with a partner transmit | `l2` (default), `l23`, `l34` | `passive`, `longTimeout` |
| `xor` | static aggregation: the transmit hash picks the member (the switch needs a static LAG) | `l2` (default), `l23`, `l34` | — |
| `active-backup` | one member transmits; the next one takes over when it goes down | — (VPP: active-backup) | `weight` (1–255; the highest up member is active) |
| `round-robin` | members take turns per packet | — (VPP: round-robin) | — |
| `broadcast` | every packet on every member | — (VPP: broadcast) | — |

`loadBalance` hashes: `l2` = source/destination MAC, `l23` = MAC + IP, `l34` = IP addresses + TCP/UDP ports.

## Rules (checked before commit — `400 problem+json` with a `pointer` to the field)

- The bond is named `BondEthernet<id>` (VPP's name); `bond.id` is optional and must equal `<id>`.
- Every member is configured under `interfaces` too (`enabled: true` makes it eligible for the active set) and is a NIC:
  not a bond, not a loopback, not a sub-interface.
- A member belongs to at most one bond: a second membership is refused at
  `/interfaces/<second bond>/bond/members/<member>`.
- A member has no addresses, DHCP client, IP unnumbered, VRF or sub-interfaces of its own — configure them on the bond.
- `loadBalance` only for `lacp`/`xor`; `passive`/`longTimeout` only for `lacp`; `weight` only for `active-backup`.

Changing the mode, the load balance or the id re-creates the bond: its members, addresses and sub-interfaces are
re-created with it (a short outage). A weight change is applied in place.

The bond is an ordinary interface otherwise: admin state, MTU, MAC, addresses, VRF and VLAN sub-interfaces of
`interfaces.BondEthernet<id>` are set as for any interface ([basics](basics.md)). One limitation of this release: to remove
a VLAN sub-interface of a bond that has its own attributes (enabled, addresses), remove those attributes in one commit
and the sub-interface (or the bond) in the next (the agent's delete ordering, tracked as TD-11c).

## The Bonds screen

**Interfaces → Bonds** (`/interfaces/bonds`) lists every bond with its mode, load balance, status, active members and one
chip per member (for LACP bonds the member's LACP state: *detached* until a partner answers, *collecting/distributing*
when aggregated). **Add bond** creates `BondEthernet<id>` with the next free id. Click a bond for its live state, the bond
form (mode, load balance, NUMA, id) and the members table: **Eligible interface** offers only NICs that may join (no
addresses, not in another bond); a NIC that is not configured yet is added to `interfaces` enabled. Everything goes to
the candidate; the pending-change bar shows the diff and **Commit** applies it.

## Example 1 — LACP with two members (REST)

```
T=<access token>   # POST /api/v1/auth/login
curl -s -X PATCH -H "authorization: Bearer $T" -H 'content-type: application/merge-patch+json' \
  http://127.0.0.1:3000/api/v1/config/interfaces -d '{
  "BondEthernet0": {"enabled": true, "ipv4": ["192.0.2.1/24"],
    "bond": {"mode": "lacp", "loadBalance": "l34",
             "members": {"TenGigabitEthernet0/0/0": {}, "TenGigabitEthernet0/0/1": {}}}},
  "TenGigabitEthernet0/0/0": {"enabled": true},
  "TenGigabitEthernet0/0/1": {"enabled": true}}'
curl -s -X POST -H "authorization: Bearer $T" 'http://127.0.0.1:3000/api/v1/config/commit?comment=lacp'
curl -s -H "authorization: Bearer $T" http://127.0.0.1:3000/api/v1/state/interfaces/bonds     # live state
```

`GET /api/v1/state/interfaces/bonds` (CLI operation `Bonding_bonds`) returns per bond the live state from VPP (mode,
load balance, admin/link, member and active-member counts, and per member link, weight and the LACP actor/partner
state: rx/mux/ptx state machines, system, key, port, state flags), the running and the candidate `bond` leaf, and
whether the candidate changes it (`live: false` when the agent does not report live bond state).

## Example 2 — active-backup with a preferred member

```json
{
  "BondEthernet1": {
    "enabled": true,
    "ipv4": ["203.0.113.1/24"],
    "bond": {
      "mode": "active-backup",
      "members": { "TenGigabitEthernet0/0/2": { "weight": 200 }, "TenGigabitEthernet0/0/3": { "weight": 100 } }
    }
  },
  "TenGigabitEthernet0/0/2": { "enabled": true },
  "TenGigabitEthernet0/0/3": { "enabled": true }
}
```

`TenGigabitEthernet0/0/2` carries the traffic while it is up; `…/3` takes over when it goes down.

## The same with the CLI

```
vrx configure
edit interfaces
merge BondEthernet0 '{"enabled":true,"ipv4":["192.0.2.1/24"],"bond":{"mode":"lacp","loadBalance":"l34"}}'
set TenGigabitEthernet0/0/0 enabled true
set TenGigabitEthernet0/0/1 enabled true
merge BondEthernet0 bond members '{"TenGigabitEthernet0/0/0":{},"TenGigabitEthernet0/0/1":{"passive":true}}'
merge BondEthernet1 '{"enabled":true,"bond":{"mode":"active-backup","members":{"TenGigabitEthernet0/0/2":{"weight":200}}}}'
set TenGigabitEthernet0/0/2 enabled true
set BondEthernet1 bond members TenGigabitEthernet0/0/2 weight 150   # a weight change is applied in place
compare
commit comment "bonds"
exit
vrx show interfaces BondEthernet0          # the bond as an interface (addresses, admin/link, counters)
vrx show configuration interfaces BondEthernet0 bond
vrx rollback <rev>                          # removes bonds and memberships; the NICs stay plain interfaces
```

The live bond/LACP table has no `show` command in this release; read it with `GET /api/v1/state/interfaces/bonds`.

## What happens on the data plane

The agent creates the bond (`bond_create2`, tagged with its owner), attaches the members (`bond_add_member`, with
`is_passive`/`is_long_timeout`), sets weights (`sw_interface_set_bond_weight`) and then the bond's own attributes and
addresses. `vppctl show bond details` and `vppctl show lacp` show the result. Member NICs are never tagged: the agent
records its memberships in its persisted claim store, so it never touches a NIC it was not told to use. If the agent
restarts, or the bond disappears from VPP behind its back, the next reconcile re-creates the bond, its members, weights
and addresses (0.42 s from agent start in the F-bonding host test). A rollback to a revision without the bond removes the
memberships, addresses and the bond, in that order; the members are plain L3 interfaces again.

LACP needs a partner: without one (a switch without LACP, or the lab's taps) the members stay *detached* with
the partner all zero and the bond has no active member.
