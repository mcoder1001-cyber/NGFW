// Package df2 holds what the DF-2 descriptor packages (ip_neighbor, arp, ip6_nd, urpf, adl,
// abf, classify, ip_session_redirect) share: the foreign-key builders for objects other
// tasks own (interfaces, VRFs, ACLs), address/MAC canonicalisation between net/netip and the
// generated ip_types, one sw_interface_dump snapshot for name ↔ sw_if_index resolution and
// owner filtering, the fib_types.FibPath codec used by abf and ip_session_redirect, the
// typed errors integration tests skip on, and the numeric id range that scopes ownership of
// untagged objects on the shared VPP.
//
// Nothing here talks to a specific plugin; each descriptor package imports it. When P05
// core lands a shared descriptor helper package, this one folds into it.
package df2
