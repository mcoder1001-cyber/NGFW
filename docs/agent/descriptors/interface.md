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

## New interfaces are sanitized before they are reported created (TD-3, D-095 a; fix rounds 1 and 2)

VPP reuses a deleted interface's sw_if_index and keeps per-index state across the delete (V19, V21, V23): the ip
classify table, l2/in/out ACL, policer/flow classify tables, the l2-input/l2-output feature bits (L2 ACL, L2 policer
classify — not feature arcs, reset on delete only for bridged/xconnected interfaces), vxlan bypass, ADL and the IPsec
SPD binding. Every interface creator — `interface.loopback` (core), `tapv2.tap`, `af-packet.host-interface`,
`memif.memif`, `bond.bond`, `interface.subinterface` (via `iface.AcquireAndTag`), all DF-6 interface types
(`df6.IfDescriptor`: gre, ipip, 6rd, vxlan, vxlan-gpe, gtpu, l2tp, pppoe), `mpls.tunnel`, the VPP-side host tap of
`lcp.itf-pair`, `ipsec.itf` and `wireguard.interface` (DF-5; fix round 2, re-review H1) — creates the interface through
`internal/vpp/ifsanitize.Acquire`, which sanitizes the new index before it is tagged and before Create returns.
`TestEveryInterfaceCreatorIsSanitized` (`ifsanitize/guard_test.go`) keeps it that way: it derives the interface-create
messages from the generated binapi (every request whose reply carries a sw_if_index, minus three reads, plus
`gpe_add_del_iface`) and fails, naming file:line, when a descriptor under `internal/descriptors` sends one outside an
`ifsanitize.Acquire` / `iface.AcquireAndTag` closure or a `df6.IfSpec` Add/Del, or creates through Acquire in a package
that never calls `ifsanitize.BeforeDelete`. Test helpers (`*_test.go`, `*test/` packages) are not scanned.

1. **L3 mode** (`sw_interface_set_l2_bridge enable=0` → `set_int_l2_mode(MODE_L3)`): zeroes the l2 feature bitmaps,
   whatever tables they name — removes the `l2-input-acl` crash path (a stale L2 ACL bit on a later bridged port reads a
   freed table on the first frame).
2. **Resurrect**: placeholder classify tables (16-byte signature mask, 2 buckets) are created until the pool's free indices
   are filled — the pool hands out the most recently freed index first — including every table an input ACL binding names
   (read back exactly), then until `FreshRun` (8) consecutive fresh indices came back. A hole another client takes after
   the `classify_table_ids` snapshot never comes back from the pool: once the run looks fresh with holes left (or at the
   cap) the table list is read again, holes that are live now are dropped and every table that appeared during the run
   is probed like a live one (re-review M1). `MaxPlaceholders` (**16 per create**) bounds it; a run that reaches the cap
   without that proof **fails closed** with `ErrCapped` (which is `ErrNoCleanIndex`), counted in
   `vrx_agent_iface_sanitize_capped_total{phase="create"}` — see "Placeholder cap" below.
3. **Clear**: ip classify, l2 classify, ADL, vxlan bypass reset blindly; input ACL read with `classify_table_by_interface`
   and unbound; output ACL / policer / flow classify probed with an unbind per table (live and placeholder; NO_SUCH_TABLE =
   not bound) — so bindings to deleted tables are no longer invisible; SPD read with `ipsec_spd_interface_dump`.
4. The placeholders are deleted again (identity re-checked with `classify_table_info` first).
5. **Quarantine**: anything still bound (a binding to a deleted table that could not be resurrected) is `ErrUnclearable`:
   the interface is deleted, an admin-down loopback tagged `quarantine:<owner>` takes the dirty index (the sw_interface
   pool is LIFO too) and the interface is created again on a fresh index (at most `MaxAcquireAttempts`, then Create fails
   with `ErrNoCleanIndex`). `ifsanitize.Release` re-sanitizes the owner's holders and deletes the clean ones.
   A capped run (`ErrCapped`) deletes the interface and fails Create **without a retry** (the cap is a property of the
   classify pool, not of the index — the next index would be capped too); its index is quarantined only if the run also
   proved a binding unclearable, so a failed create makes at most one holder.
   Any other sanitize failure removes the interface and fails Create.

### Reserved loopback instances loop16000–loop16383 (quarantine holders)

A quarantine holder is created with `create_loopback_instance(is_specified=1)` on the **highest free instance of
16000–16383**, scanning down from 16383 (VPP's `LOOPBACK_MAX_INSTANCE` is 16384, `vnet/ethernet/interface.c:740`; an
instance in use answers INVALID_REGISTRATION before anything is allocated) — never VPP's lowest free instance, so a holder
never becomes `loop0`/`loop1` and never blocks a user's loopback (re-review M2). The instance only names the loopback;
the holder still lands on the dirty sw_if_index. If the whole range is taken the quarantine fails and so does the Create.
**loop16000–loop16383 are reserved for the agent**: do not configure user loopbacks there. `packages/schema` does not
reject these names yet (`vppInterfaceName` accepts any `loopN`) — a contract change is proposed in
`docs/status/tasks/TD-3-questions.md` (CONTRACT); until then a user `loop16383` makes holders use 16382 and below, and a
holder on an instance a user later wants makes that user's Create fail with "instance in use" until `Release` or a VPP
restart.

### Placeholder cap (fail closed)

At most 16 placeholder tables per create. The cap is reached when the classify pool's free list holds more than about
`16 − FreshRun` = 8 indices that do not come back in ascending order (e.g. ≥ 9 tables deleted in creation order and not
reused). The create then fails with `ErrCapped` and the scheduler retries it later; it succeeds once the free list is
shorter (new classify tables reuse freed indices) or VPP restarts. `vrx_agent_iface_sanitize_capped_total` > 0 is the
signal. The exact per-interface readback that would remove the cap and the `FreshRun` guess is tracked in
`docs/tech-debt.md` (TD-3 re-review M3), due before any production image.

Every interface Delete (loopback, tap, memif, bond, af_packet, sub-interface, DF-6 types, mpls tunnel, lcp pair host tap,
ipsec itf, wireguard interface) calls `ifsanitize.BeforeDelete` right before the VPP delete — the only moment every table the interface is bound to still
exists (review H3); an unclearable binding there is logged and counted, the delete goes on.

Handled by VPP itself (no action): ACL plugin in/out lists (`acl.c` resets them on interface delete), NAT44-ED/EI interface
flags, ADL per-index config (re-initialised on interface add). Known, not handled (no crash path found; recorded in
`docs/vpp-code-track.md` V23 b): ABF attachments, NAT64/NAT66/DET44 interface flags, cnat snat-if, flowprobe.

Only binapi messages are used. Every run is logged at info and counted per phase (`create` for a new index, `delete`
before a delete): `vrx_agent_iface_sanitize_{total,errors_total,inherited_total}{phase}`,
`vrx_agent_iface_sanitize_{cleared_total,freed_table_total,unclearable_total}{phase,state}`,
`vrx_agent_iface_sanitize_capped_total{phase}` (placeholder cap reached; create failed closed), plus the gauge
`vrx_agent_iface_quarantined` and the counter `vrx_agent_iface_quarantine_total`.
