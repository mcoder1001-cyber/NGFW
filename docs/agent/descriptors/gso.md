# Descriptors — gso (F-loopback-bvi-gso-lldp-span, WBS D1.8)

Package `apps/agent/internal/descriptors/gso`, built on `descriptors/dfkit` (D-077): the value is a dfkit structpb spec
(`Interface{interface}`). `gso.Register(r, client, owner, store)` — `store` is the owner's persisted D-076 BootStore
(`subsystems.Wiring.BootStore()`, never in memory in the product; `CheckPersistent`, TD-11b). Messages only from
`apps/agent/binapi/{gso,feature}`.

| Object type (descriptor) | Key | Depends on | VPP messages | Update | Notes / limitations |
|---|---|---|---|---|---|
| `gso.interface` | `gso.interface/<interface>` | `interface/<name>` (D-065) — GSO goes before the interface | `feature_gso_enable_disable` (the gso-ip4/-ip6 nodes on ip4/ip6-output and gso-l2-* on the L2 output arcs); readback `feature_is_enabled("ip4-output", "gso-ip4", sw_if_index)` | none (the value is the interface) | Present ⇔ on. VPP 26.06 has no GSO getter and **stacks the feature on every enable** (a disable removes one instance), so the enable is applied **once per VPP boot**: the applied-once record (D-076/D-080) holds `<sw_if_index>/<logical name>` under the boot identity. Retrieve reports an interface only when that record matches this boot, index and name **and** the read-back is true (the read-back alone says "true" for an index the arc never reached, V23 a). A Create that finds GSO on without a record disables until the read-back says off (bounded: 16) and then enables once — a lost record or a stale stacked enable is normalised to exactly one. Delete re-resolves the name (indexes are reused), disables once when the handle's index still has the name, forgets the record and releases the claim on untagged interfaces. The claim is recorded **before** the first VPP write (TD-11b claim first); an enable whose record cannot be written is taken back. |

Not inherited: VPP clears every vnet feature arc of an interface when it is deleted (`vnet_feature_add_del_sw_interface`),
so GSO left on a deleted interface is not inherited by the next one on the index — checked on the host
(`TestGSONotInheritedOnHost`), unlike span/LLDP bookkeeping (docs/vpp-code-track.md V-new (F-loopback-bvi-gso-lldp-span)).

Projection (`internal/desired/gso.go`): `interfaces.<if>.gso: true` → `gso.interface/<if>`; Retrieve assembles
`gso: true`, and `gso: false` where the stored document sets the key and VPP has no GSO.

Unit tests (`gso_test.go`, a fake modelling the stacking enable and the out-of-range read-back): `TestGSOAppliedOnce`.
Host evidence (`VRX_INTEGRATION=1`): `TestGSOOnHost` (three Creates then ONE Delete leaves it off — not stacked; Retrieve ==
desired; loss behind the agent's back restored once) and `TestGSONotInheritedOnHost`.
