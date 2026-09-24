// Package df6 holds what the DF-6 descriptor packages (gre, ipip, vxlan, vxlan_gpe, gtpu,
// l2tp, pppoe, sr, sr_mpls, lisp) share: the foreign-key builders for objects other tasks own
// (interfaces, VRFs, bridge domains, MPLS tables), address canonicalisation between
// net/netip and the generated ip_types, one sw_interface_dump snapshot for name ↔ sw_if_index
// resolution and owner filtering, the owner-tag stamping of tunnel interfaces, the typed
// errors integration tests skip on, and the Scope that attributes untagged objects (SR
// policies, LISP mappings, …) to this agent on the shared VPP.
//
// Nothing here talks to a specific plugin; each descriptor package imports it. It mirrors
// DF-2's df2 package on purpose; when P05 core lands a shared descriptor helper package,
// both fold into it.
package df6
