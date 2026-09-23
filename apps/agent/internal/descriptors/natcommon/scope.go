package natcommon

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Scope says which NAT objects belong to this agent when the object carries no tag
// (docs/lab/shared-host-rules.md §2, internal/descriptors/README.md "Ownership"):
//
//   - a test slot owner "w<N>" owns IPv4 addresses in 10.<N>.0.0/16, IPv6 addresses in
//     fd00:<N>::/32, VRF/table ids N000–N999 and interfaces tagged "w<N>:…";
//   - any other owner (the production agent, VRX_OWNER=vrx) owns everything on the VPP —
//     there is exactly one agent per data plane.
//
// Objects that do carry a tag (nat44 static/identity/lb mappings, map domains) use the
// owner tag "<owner>:<id>" (vpp.OwnerTag) and are filtered with ParseTag; interface-bound
// objects (features, nat64/nat66/det44 interfaces, attachments) are owned when the
// interface is (OwnsInterface).
type Scope struct {
	Owner string
	// All is true for production owners: every object on the VPP is ours.
	All     bool
	V4      netip.Prefix
	V6      netip.Prefix
	TableLo uint32
	TableHi uint32
}

// ScopeFor derives the Scope from the owner id.
func ScopeFor(owner string) Scope {
	s := Scope{Owner: owner, All: true}
	if !strings.HasPrefix(owner, "w") {
		return s
	}
	n, err := strconv.Atoi(owner[1:])
	if err != nil || n < 1 || n > 255 {
		return s
	}
	s.All = false
	s.V4 = netip.MustParsePrefix(fmt.Sprintf("10.%d.0.0/16", n))
	s.V6 = netip.MustParsePrefix(fmt.Sprintf("fd00:%x::/32", n))
	s.TableLo = uint32(n) * 1000 //nolint:gosec // n ≤ 255
	s.TableHi = s.TableLo + 999
	return s
}

// OwnsAddr reports whether the address is inside this owner's range (v4 or v6).
func (s Scope) OwnsAddr(a netip.Addr) bool {
	if s.All {
		return true
	}
	if a.Is4() {
		return s.V4.Contains(a)
	}
	return s.V6.Contains(a.Unmap())
}

// OwnsAddrString is OwnsAddr for a textual address; unparsable never matches.
func (s Scope) OwnsAddrString(a string) bool {
	addr, err := netip.ParseAddr(a)
	return err == nil && s.OwnsAddr(addr)
}

// OwnsPrefix reports whether a prefix lies inside this owner's range.
func (s Scope) OwnsPrefix(p netip.Prefix) bool {
	if s.All {
		return true
	}
	if p.Addr().Is4() {
		return s.V4.Contains(p.Addr()) && p.Bits() >= s.V4.Bits()
	}
	return s.V6.Contains(p.Addr().Unmap()) && p.Bits() >= s.V6.Bits()
}

// OwnsTable reports whether a VRF/table id is inside this owner's range. Table 0 is never a
// slot's, so slot-scoped objects in the default table are owned only by their addresses.
func (s Scope) OwnsTable(id uint32) bool {
	return s.All || (id >= s.TableLo && id <= s.TableHi)
}

// Tag builds the owner tag "<owner>:<id>" for tagged objects.
func (s Scope) Tag(id string) (string, error) { return vpp.OwnerTag(s.Owner, id) }

// ParseTag splits an owner tag; ok is false for other owners' and unformatted tags.
func (s Scope) ParseTag(tag string) (id string, ok bool) {
	return vpp.ParseOwnerTag(tag, s.Owner)
}

// InterfaceOwnership classifies an interface for interface-bound objects (D-071 claim
// rule): tagged by this owner → ours; tagged by anyone else → never ours (ok=false); untagged
// → ours only if claimed (needsClaim=true). local0 (sw_if_index 0) is never ours. Production
// owners (All) follow the same rule — they no longer claim other owners' tagged interfaces
// (review finding 5).
func (s Scope) InterfaceOwnership(i Iface) (ok, needsClaim bool) {
	if i.SwIfIndex == 0 {
		return false, false
	}
	if _, own := s.ParseTag(i.Tag); own {
		return true, false
	}
	if i.Tag != "" {
		return false, false
	}
	return true, true
}

// NeedsClaim reports whether an untagged object (pool, prefix, translation, binding, …) needs
// a claim: a test slot owns the objects inside its range (the shared-host rules are the
// slot's standing claim); everything else — including every untagged object of a production
// owner — is ours only when claimed (D-071).
func (s Scope) NeedsClaim(inSlotRange bool) bool { return s.All || !inSlotRange }

// Dependency key builders. DF-1/DF-2 own the interface and table descriptors; until their
// names are merged the keys follow the DF-3 task prompt ("interface/<name>", "vrf/<id>").
// Change them here, in one place, if the merged names differ (open question in
// docs/status/tasks/DF-3-questions.md).
var (
	InterfaceDescriptor = "interface"
	VRFDescriptor       = "vrf"
)

// InterfaceDep is a mandatory dependency on the interface with the given name.
func InterfaceDep(name string) scheduler.Dependency {
	return scheduler.Dependency{Key: scheduler.Join(InterfaceDescriptor, name)}
}

// VRFDep is an optional dependency on the VRF/table with the given id; table 0 always
// exists and yields no dependency.
func VRFDep(id uint32) (scheduler.Dependency, bool) {
	if id == 0 {
		return scheduler.Dependency{}, false
	}
	return scheduler.Dependency{Key: scheduler.Join(VRFDescriptor, strconv.FormatUint(uint64(id), 10)), Optional: true}, true
}

// WithVRF appends the optional VRF dependency for id (if any) to deps.
func WithVRF(deps []scheduler.Dependency, id uint32) []scheduler.Dependency {
	if d, ok := VRFDep(id); ok {
		deps = append(deps, d)
	}
	return deps
}

// Dep is a mandatory dependency on another NAT object by key.
func Dep(key scheduler.Key) scheduler.Dependency { return scheduler.Dependency{Key: key} }

// OptionalDep is an optional dependency on another object by key.
func OptionalDep(key scheduler.Key) scheduler.Dependency {
	return scheduler.Dependency{Key: key, Optional: true}
}
