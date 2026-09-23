// Package acl holds the reconciler descriptors for the VPP acl plugin (task DF-4, WBS D5.2):
// L3/L4 ACLs (stateless and stateful/reflect), per-interface in/out ACL lists, ethertype
// whitelists, L2 MACIP ACLs and their interface binding, the global stats-counters switch and a
// typed reader for the per-rule hit counters in the stats segment.
//
// Every VPP message name and field comes from the generated bindings in apps/agent/binapi/acl
// (VPP 26.06, P04). The desired-state values are *structpb.Struct documents built from the typed
// specs in spec.go (ACL, MacipACL, InterfaceBinding, EtypeWhitelist, MacipBinding, StatsEnable)
// because the P03 proto contract has no ACL messages yet; swapping to the real proto type later
// touches spec.go only. docs/agent/descriptors/acl.md is the object ↔ message table and the key
// contract other tasks (DF-2 ABF, F-*) build against.
//
// Ownership on the shared VPP: ACLs and MACIP ACLs carry the tag "<owner>:<name>" (vpp.OwnerTag);
// bindings are owned through the ACLs they reference; ethertype whitelists through the tag of the
// interface they sit on. Retrieve never returns another owner's objects.
package acl
