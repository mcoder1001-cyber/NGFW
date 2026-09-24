// Package df7 holds what the DF-7 descriptor packages (policer, qos, lb, span, lldp, bfd, vrrp,
// igmp, mpls) share: typed errors, the structpb codec for the typed specs (D-055 stand-in until
// P03b adds the domain leaf messages), the interface snapshot on top of DF-1's logical-name
// resolver, the ownership scope for untagged numeric ids, the boot-identity record of
// write-only objects, address and FIB-path conversions, the cross-task dependency keys and the
// plugin Options.
//
// Ownership on the shared VPP (docs/lab/shared-host-rules.md, D-069, D-071):
//
//   - objects with a name/tag field (policer name, MPLS table name, MPLS tunnel tag) carry
//     vpp.OwnerTag(owner, id);
//   - objects hanging off an interface (QoS record/store/mark, SPAN, BFD, VRRP, IGMP, MPLS
//     interface) belong to the owner of the interface: our tag → ours; another owner's tag →
//     never touched (the resolver refuses it); untagged → only with a claim record for the
//     object's key in DF-1's per-owner ClaimStore (iface.Claims);
//   - objects identified only by a number (QoS egress map id, BFD conf-key id) are attributed by
//     an IDRange (tests: the slot's VRX_VPP_TABLE_BASE..+999; production: every id);
//   - VPP-global settings (lb.conf, lldp.global, bfd.echo-source, igmp.group-prefix) are only
//     registered by the plugins' RegisterGlobals, which only the globals owner calls (D-071).
package df7
