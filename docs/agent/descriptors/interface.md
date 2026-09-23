# Descriptors — interface plugin (DF-1)

Package `apps/agent/internal/descriptors/interface` (Go package `iface`), models in `iface_model.proto`
(agent-internal stand-in per D-055 until P03b adds the domain leaf messages). Registered by
`iface.Register(r, client, owner)`.

## Interface names and keys — the contract for DF-2 … DF-8, F-*, P08 (D-065, D-069)

### Logical names
Every interface has one **logical name** (`names.go`):

| Interface | Logical name | Example |
|---|---|---|
| created by this agent (tap, bond, memif, af_packet, sub-interface, P05's loopback, tunnels of DF-6 …) | the id of its creator key = its owner-tag id (`"<owner>:<name>"`) | `w2-tap40` (VPP calls it `tap240`), `loop201`, `w2-tap11.100`, `ens224.100` |
| physical / pre-existing (DPDK NIC, anything untagged) | VPP's interface name — F-startup-gen names DPDK NICs by their logical name (`dpdk { dev <pci> { name lan } }`) so both coincide | `lan`, `ens224`, `tap271` |
| tagged by **another owner** | none: never resolvable, never reported | — |

**The one resolver** (logical name → sw_if_index) for every descriptor package:
`iface.ResolveName(ctx, client, owner, name)` / `iface.Dump(...)` + `Table.IndexByName(name)`:
our tag id first, then an untagged interface's VPP name; `ErrForeignInterface` for another owner's
interface, `ErrNotFound` otherwise (also for VPP's index-based name of one of *our* interfaces and for
`local0`). `Table.Logical(idx)` / `Table.Ref(idx)` go the other way. DF-4 acl uses it (D-069, commit
`fix(acl): resolve interfaces by logical name (D-069)`).

### `interface/<name>` — the key consumers depend on
The alias descriptor `interface` (`iface.AliasName`, `iface.AliasKey(name)`), value
`InterfaceAlias{name, creator}`:

| Field | Meaning |
|---|---|
| `name` | the logical name |
| `creator` | full key of the creating descriptor's object (`tapv2.tap/w2-tap10`), **empty** for physical / pre-existing interfaces |

| Method | Behaviour |
|---|---|
| Dependencies | `creator` when set (mandatory); none for physical / pre-existing interfaces |
| Create / Update | create nothing; verify the interface exists (creator key → owner tag, else `IndexByName`) and return `Meta{SwIfIndex}`; `ErrNotFound` / `ErrForeignInterface` otherwise |
| Delete | **no-op** — never touches VPP, so it can never remove any interface |
| Retrieve | one alias per interface with a logical name for this owner (ours + untagged; never another owner's, never `local0`) |
| DeleteOnAbsence | **false** (P05 `scheduler.AbsenceDeleter`): observe-only — undesired aliases (NICs the config does not mention, …) are never planned for Delete and never fail verification. **Reconciler requirement** for P05. |

The desired-state builder emits one alias per interface the config names (with `creator` for the ones
it also creates).

### References inside DF-1 models — canonical form `interface/<name>`
Every DF-1 model field that names an interface (`interface`, `parent`, `bond`, `rx`, `tx`, l3xc path
`interface`) accepts the alias key **or** a creator key (`tapv2.tap/w2-tap40`); `Dependencies()`
returns it verbatim. **Retrieve always reports the alias form**, and every such descriptor implements
P05's `Normalizer` (`Normalize` maps creator keys to `interface/<id>` via `iface.CanonicalRef`, plus
other canonicalisation — l3xc weight/next-hop/path order, fib MAC case, tap host prefixes), so desired
and actual compare equal after P05 normalises the desired state. Object keys use the logical name
(`interface.mtu/w2-tap40`, `interface.mtu/ens224`).

Creator keys (DF-1-internal):

| Interface kind | Creating descriptor | Creator key | Example |
|---|---|---|---|
| loopback (P05 core) | `interface.loopback` | `interface.loopback/<name>` | `interface.loopback/loop201` |
| sub-interface | `interface.subinterface` | `interface.subinterface/<parent id>.<sub_id>` | `interface.subinterface/w2-tap11.100` |
| tap | `tapv2.tap` | `tapv2.tap/<name>` | `tapv2.tap/w2-tap10` |
| host-interface (af_packet) | `af-packet.host-interface` | `af-packet.host-interface/<name>` | `af-packet.host-interface/w2-af50` |
| bond | `bond.bond` | `bond.bond/<name>` | `bond.bond/w2-bond20` |
| memif | `memif.memif` | `memif.memif/<name>` | `memif.memif/w2-memif40` |
| other plugins (tunnels, …) | their own descriptor | `<descriptor>/<name>` + `iface.RegisterKind(devType, descriptor)` | `gre.tunnel/w2-gre0` |

Ownership of created interfaces: the creating descriptor stamps tag `"<owner>:<name>"` (D-030); if the
tag fails, the just-created interface is deleted again so a retry is not blocked (review M3).
`sw_if_index` lives in `Meta` (`iface.Meta{SwIfIndex}`), never in a key.

### Physical / untagged interfaces — ClaimStore (review H2)
Per-interface objects on an untagged interface (admin-state, MTU, MAC, promisc, rx-mode, rx-placement,
bond membership, xconnect rx, vlan-tag-rewrite, l3xc) carry no owner in VPP. As in DF-4's acl
`ClaimStore`, the descriptor records a claim `(interface name, descriptor name)` in the owner's
`iface.ClaimStore` after a successful Create and releases it on a successful Delete; Retrieve reports
the object iff the claim exists (`Table.Owns` / `Table.OwnedRef`). Objects owned through something else
need no claim: bridge membership / l2 flags / fib entries (our bridge domain), sub-interfaces (tagged
themselves). The store is in-memory per owner by default; **P05 should install a persisted store with
`iface.SetClaimStore(owner, store)`** so claims survive an agent restart. No DF-1 descriptor ever
deletes an interface it did not create, so a physical NIC is never removed. Creation defaults of a NIC
(for the "present ⇔ differs from default" rules below) are assumed to follow the other hardware
interfaces (`{link_mtu,0,0,0}`, rx-mode polling); not verifiable on the dev host (no data NICs).

## Object types

| Object type (descriptor) | Key | Depends on | VPP messages | Update | Notes / limitations |
|---|---|---|---|---|---|
| `interface` (alias) | `interface/<name>` | creator key when set | `sw_interface_dump` only | re-verify | See above; observe-only. |
| `interface.subinterface` | `interface.subinterface/<parent id>.<sub_id>` | parent reference | `create_subif`, `delete_subif`, `sw_interface_tag_add_del`; Retrieve `sw_interface_dump` (`sub_id`, `sub_number_of_tags`, `sub_outer/inner_vlan_id`, `sub_if_flags`) | ErrRecreate | dot1q / dot1ad / QinQ / exact-match / default / untagged / outer-any / inner-any via `sub_if_flags`; `create_vlan_subif` not used (one code path). The parent may be a physical NIC (`interface/ens224` → `ens224.100`). |
| `interface.admin-state` | `interface.admin-state/<name>` | interface reference | `sw_interface_set_flags`; Retrieve `sw_interface_dump.flags` | no-op | Present ⇔ ADMIN_UP. Delete → admin down. |
| `interface.mtu` | `interface.mtu/<name>` | interface reference | `sw_interface_set_mtu` (per-protocol L3/IP4/IP6/MPLS); Retrieve `sw_interface_dump.mtu[]`, `link_mtu` | in place | sw (per-protocol) MTU, not `hw_interface_set_mtu`. Present ⇔ MTUs ≠ creation default (`{link_mtu,0,0,0}`, sub-interface `{0,0,0,0}`, host-verified). Delete restores the default (no-op when the interface is gone or no longer ours). Rejected: L3 mtu 0 (`ErrZeroMtu`) and a value equal to the default (`ErrMtuDefault`, review M4 — it would be invisible and re-created forever). |
| `interface.mac-address` | `interface.mac-address/<name>` | interface reference | `sw_interface_set_mac_address`; Retrieve `sw_interface_dump.l2_address` | in place | Reported only for interfaces this process set (VPP has no configured-vs-default MAC); re-applied once after an agent restart (write-only semantics, D-063); the memory is dropped when VPP restarts (main-thread PID, review L1). Delete is a no-op. Not on sub-interfaces. |
| `interface.promisc` | `interface.promisc/<name>` | interface reference | `sw_interface_set_promisc` | no-op | No readback in VPP 26.06: same process-memory rule as MAC. Delete → off. |
| `interface.rx-mode` | `interface.rx-mode/<name>` | interface reference | `sw_interface_set_rx_mode` (all queues); Retrieve `sw_interface_rx_placement_dump.mode` | in place | Present ⇔ a queue's mode ≠ the class default (polling; **interrupt for af-packet**, host-verified). Desiring the default → `ErrRxModeDefault`; Delete restores it. |
| `interface.rx-placement` | `interface.rx-placement/<name>/<queue>` | interface reference | `sw_interface_set_rx_placement`; Retrieve `sw_interface_rx_placement_dump.worker_id` | in place | Present ⇔ queue on a worker; Delete → main thread. Host has no workers: integration subtest skips with `skip: no workers on host`. |

Loopback (`interface.loopback`) and IP addresses / VRF binding are P05 core's; DF-1 tests create
prefixed loopbacks directly (`ifacetest.Loopback`) with the same tag scheme.

## Restart safety (host evidence)

`TestRestartSimulationOnHost` (this package; owner `w2r`, 38 objects incl. 11 aliases, desired state
normalised as P05 does): apply → same agent re-plan empty → restart (new connection, fresh descriptors):
Retrieve rebuilds every object with equal Meta, plan = one re-apply each of promisc / MAC → empty →
**loss** (a tap with its sub-interface and L2 dependents, a memif + socket and an l3xc deleted via the
binary API behind the agent's back) → plan = exactly the creates of what is gone → re-created in
dependency order → plan empty → delete all → Retrieve empty. `TestAliasOnHost` configures a pre-existing
untagged tap (admin-up, MTU, rx-mode, VLAN sub-interface) through `interface/tap271`. Output in
`docs/status/tasks/DF-1.md`.
