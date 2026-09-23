package df6

import "net/netip"

// IDRange is a closed range of numeric ids (table ids, MPLS labels, VNIs).
type IDRange struct{ Lo, Hi uint32 }

// Owns reports whether id is inside the range; a nil range owns everything.
func (r *IDRange) Owns(id uint32) bool { return r == nil || (id >= r.Lo && id <= r.Hi) }

// Scope says which untagged VPP objects this agent owns on a shared VPP. Tunnel interfaces
// carry an owner tag and need none of this; SR policies, localsids, steering entries, LISP
// mappings, map resolvers and MPLS BSIDs have no tag field, so they are attributed by the
// numeric ranges and address blocks the slot owns (docs/lab/shared-host-rules.md: tables
// N000–N999, addresses 10.N.0.0/16 and fdNN::/16, labels N000–N999, VNIs N000–N999). In
// production every field is nil: the agent owns everything on its VPP.
type Scope struct {
	// Owner is the tag owner (VRX_OWNER / VRX_TEST_PREFIX).
	Owner string
	// Tables scopes FIB / EID table ids; nil = all.
	Tables *IDRange
	// Labels scopes MPLS labels (SR-MPLS BSIDs); nil = all.
	Labels *IDRange
	// VNIs scopes VXLAN/LISP VNIs and GTP-U TEIDs; nil = all.
	VNIs *IDRange
	// Addrs scopes addresses and prefixes (SIDs, BSIDs, EIDs, map resolvers); nil = all.
	Addrs []netip.Prefix
	// NamePrefix scopes named objects (LISP locator sets); "" = all.
	NamePrefix string
}

// OwnsTable reports whether table id is in scope.
func (s *Scope) OwnsTable(id uint32) bool { return s == nil || s.Tables.Owns(id) }

// OwnsLabel reports whether MPLS label l is in scope.
func (s *Scope) OwnsLabel(l uint32) bool { return s == nil || s.Labels.Owns(l) }

// OwnsVNI reports whether VNI / TEID v is in scope.
func (s *Scope) OwnsVNI(v uint32) bool { return s == nil || s.VNIs.Owns(v) }

// OwnsAddr reports whether a is inside one of the scope's address blocks.
func (s *Scope) OwnsAddr(a netip.Addr) bool {
	if s == nil || s.Addrs == nil {
		return true
	}
	for _, p := range s.Addrs {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// OwnsPrefix reports whether p lies inside one of the scope's address blocks.
func (s *Scope) OwnsPrefix(p netip.Prefix) bool {
	if s == nil || s.Addrs == nil {
		return true
	}
	for _, b := range s.Addrs {
		if b.Bits() <= p.Bits() && b.Contains(p.Addr()) {
			return true
		}
	}
	return false
}

// OwnsAddrString parses a and reports OwnsAddr; unparsable / empty addresses are not owned.
func (s *Scope) OwnsAddrString(a string) bool {
	if s == nil || s.Addrs == nil {
		return true
	}
	n, err := ParseAddr(a)
	if err != nil {
		return false
	}
	return s.OwnsAddr(n)
}

// OwnsName reports whether name carries the scope's name prefix.
func (s *Scope) OwnsName(name string) bool {
	return s == nil || s.NamePrefix == "" || (len(name) >= len(s.NamePrefix) && name[:len(s.NamePrefix)] == s.NamePrefix)
}

// OwnerOf returns the tag owner ("" for a nil scope).
func (s *Scope) OwnerOf() string {
	if s == nil {
		return ""
	}
	return s.Owner
}
