# Descriptors — interface plugin (DF-1)

Package `apps/agent/internal/descriptors/interface` (Go package `iface`), models in `iface_model.proto`
(agent-internal stand-in per D-055 until P03b adds the domain leaf messages). Registered by
`iface.Register(r, client, owner)`.

## Interface keys — what consumers use: `interface/<name>` (D-065)

**Every consumer outside DF-1 (DF-2 … DF-8, F-*, P08) depends on `interface/<name>`** and never needs
to know which descriptor created the interface. That key is an object of the alias descriptor
`interface` (`iface.AliasName`, `iface.AliasKey(name)`), value `InterfaceAlias{name, creator}`:

| Field | Meaning |
|---|---|
| `name` | the interface's stable name: the creator key's id for interfaces we create (`w2-tap10`, `loop201`, `w2-tap11.100`), VPP's interface name for physical / pre-existing ones (`ens161`, `tap271`) |
| `creator` | full key of the creating descriptor's object (`tapv2.tap/w2-tap10`), **empty** for DPDK NICs and anything not created by a descriptor |

| Method | Behaviour |
|---|---|
| Dependencies | `creator` when set (mandatory); none for physical / pre-existing interfaces |
| Create / Update | create nothing; verify the interface exists in one `sw_interface_dump` (by creator key → owner tag, else this owner's tag id, else VPP interface name; never `local0`) and return `Meta{SwIfIndex}`; `ErrNotFound` otherwise |
| Delete | **no-op** — an alias Delete never touches VPP, so it can never remove a foreign interface |
| Retrieve | one alias per VPP interface except `local0`, **including interfaces we do not own**; ours carry name = tag id and their creator key |

The desired-state builder emits one alias per interface the config names (with `creator` for the ones
it also creates). Reconciler note for P05 (DF-1-questions.md Q4): Retrieve returns foreign aliases, so
undesired aliases must not count as drift (their Delete is a no-op).

### Creator keys (DF-1-internal)

Inside DF-1 an interface is referenced by the **full scheduler key of the descriptor that creates it**:

| Interface kind | Creating descriptor | Creator key | Example |
|---|---|---|---|
| loopback (P05 core) | `interface.loopback` | `interface.loopback/<name>` | `interface.loopback/loop201` |
| sub-interface | `interface` (alias, D-065) | `interface/<name>` | creator key when set | `sw_interface_dump` only | re-verify | See above: creates nothing, Delete no-op, Retrieve lists every VPP interface except local0. |
| `interface.subinterface` | `interface.subinterface/<parent id>.<sub_id>` | `interface.subinterface/w2-tap11.100` |
| tap | `tapv2.tap` | `tapv2.tap/<name>` | `tapv2.tap/w2-tap10` |
| host-interface (af_packet) | `af-packet.host-interface` | `af-packet.host-interface/<name>` | `af-packet.host-interface/w2-af50` |
| bond | `bond.bond` | `bond.bond/<name>` | `bond.bond/w2-bond20` |
| memif | `memif.memif` | `memif.memif/<name>` | `memif.memif/w2-memif40` |
| other plugins (tunnels, …) | their own descriptor | `<descriptor>/<name>` + `iface.RegisterKind(devType, descriptor)` | `gre.tunnel/w2-gre0` |

- The id part (`<name>`) is the interface's **stable name** chosen by the desired state, not VPP's
  index-based name (`tap3`, `BondEthernet220`, `memif40/0`, `host-w2-af50`). Interface names form one
  namespace across all kinds, as in VPP.
- DF-1's own model fields that point at an interface (`interface`, `parent`, `bond`, `rx`, `tx`, l3xc
  path `interface`) hold creator keys; `Dependencies()` returns them verbatim.
- Ownership: the creating descriptor stamps `sw_interface_tag_add_del` tag `"<owner>:<name>"`
  (`vpp.OwnerTag`, D-030). Resolution (`iface.Table.Index`) finds the `sw_if_index` whose tag is
  `"<owner>:<name>"` in one `sw_interface_dump`, and checks that the VPP device class matches the key's
  descriptor (`ErrWrongKind`). `Table.KeyFor` rebuilds the full key at Retrieve time from the tag and
  `interface_dev_type` (`Loopback`, `tap`, `af-packet`, `bond`, `memif`; sub-interfaces by
  `IF_API_TYPE_SUB`; all verified on the host).
- `sw_if_index` lives in `Meta` (`iface.Meta{SwIfIndex}`), never in a key. The contract gives Create no
  access to a dependency's Meta, so Create resolves the interface by owner tag (one dump) — the
  stable-name → index map is VPP's own tag table, not a second map in the agent.

## Object types

| Object type (descriptor) | Key | Depends on | VPP messages | Update | Notes / limitations |
|---|---|---|---|---|---|
| `interface.subinterface` | `interface.subinterface/<parent id>.<sub_id>` | parent interface key | `create_subif`, `delete_subif`, `sw_interface_tag_add_del`; Retrieve `sw_interface_dump` (`sub_id`, `sub_number_of_tags`, `sub_outer/inner_vlan_id`, `sub_if_flags`) | ErrRecreate | dot1q / dot1ad / QinQ (outer+inner) / exact-match / default / untagged / outer-any / inner-any via `sub_if_flags`. `create_vlan_subif` not used (one code path; it is `create_subif` with ONE_TAG\|EXACT_MATCH). |
| `interface.admin-state` | `interface.admin-state/<id>` | interface key | `sw_interface_set_flags`; Retrieve `sw_interface_dump.flags` | no-op | Present ⇔ ADMIN_UP. Delete → admin down. |
| `interface.mtu` | `interface.mtu/<id>` | interface key | `sw_interface_set_mtu` (per-protocol L3/IP4/IP6/MPLS); Retrieve `sw_interface_dump.mtu[]`, `link_mtu` | in place | Decision: sw (per-protocol) MTU, not `hw_interface_set_mtu` (driver property, rejected by most virtual devices; sw MTU works on sub-interfaces too). Present ⇔ MTUs ≠ creation default: `{link_mtu,0,0,0}` for hardware interfaces, `{0,0,0,0}` for sub-interfaces (host-verified). Delete restores that default. L3 `mtu` must be non-zero (`ErrZeroMtu`). |
| `interface.mac-address` | `interface.mac-address/<id>` | interface key | `sw_interface_set_mac_address`; Retrieve `sw_interface_dump.l2_address` | in place | VPP has no "configured vs default" MAC, so Retrieve reports only interfaces this process set (with the address VPP reports now → drift visible). After an agent restart the desired MAC is re-applied once (idempotent). Delete is a no-op (no "unset MAC"). Not on sub-interfaces. |
| `interface.promisc` | `interface.promisc/<id>` | interface key | `sw_interface_set_promisc` | no-op | VPP 26.06 reports promisc in no dump: Retrieve reflects what this process switched on; re-applied once after a restart. Delete → off. |
| `interface.rx-mode` | `interface.rx-mode/<id>` | interface key | `sw_interface_set_rx_mode` (all queues); Retrieve `sw_interface_rx_placement_dump.mode` | in place | Present ⇔ a queue's mode ≠ the class default: polling (interface_main default) for every class, **interrupt for af-packet** (af_packet.c forces it; host-verified). Desiring the default → `ErrRxModeDefault`. Delete restores the default. |
| `interface.rx-placement` | `interface.rx-placement/<id>/<queue>` | interface key | `sw_interface_set_rx_placement`; Retrieve `sw_interface_rx_placement_dump.worker_id` | in place | Present ⇔ queue on a worker thread; Delete → main thread. The dev host runs `cpu { }` (no workers): the integration subtest skips with `skip: no workers on host`. |

Loopback (`interface.loopback`) and IP addresses / VRF binding are P05 core's; DF-1 tests create
prefixed loopbacks directly (`ifacetest.Loopback`) with the same tag scheme.

## Restart safety (host evidence)

`TestRestartSimulationOnHost` (this package) applies one desired state with every DF-1 object type
(28 objects, owner `w2r`), re-Retrieves with the same agent (plan empty), then with a new API connection
and freshly constructed descriptors (plan empty except `interface.promisc` / `interface.mac-address`,
which VPP cannot report — they are re-applied once, after which the plan is empty), checks that every
retrieved Meta equals the Meta Create returned, deletes everything with the restarted agent and checks
Retrieve is empty. Output in `docs/status/tasks/DF-1.md`.
