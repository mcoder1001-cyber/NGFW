// Package df7 holds what the DF-7 descriptor packages (policer, qos, lb, span, lldp, bfd, vrrp,
// igmp, mpls) share: typed errors, the structpb codec for the typed specs (D-055 stand-in until
// P03b adds the domain leaf messages), the sw_interface_dump snapshot with owner attribution,
// the ownership scope for untagged numeric ids, address and FIB-path conversions, the cross-task
// dependency keys and the plugin Options.
//
// Ownership on the shared VPP (docs/lab/shared-host-rules.md, internal/descriptors/README.md):
//
//   - objects with a name/tag field (policer name, MPLS table name, MPLS tunnel tag) carry
//     vpp.OwnerTag(owner, id);
//   - objects hanging off an interface (QoS record/store/mark, SPAN, BFD, VRRP, IGMP, MPLS
//     interface) belong to the owner of the interface: its tag parses as this owner's, or — with
//     WithClaimUntagged, the production setting where one agent owns the box — it is untagged;
//   - objects identified only by a number (QoS egress map id, BFD conf-key id) are attributed by
//     an IDRange (tests: the slot's VRX_VPP_TABLE_BASE..+999; production: every id).
package df7
