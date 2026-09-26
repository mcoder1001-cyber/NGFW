package desired

// F-nat44-ei-64-66-nptv6: `nat.nptv6` onto the npt66 descriptor (docs/agent/descriptors/npt66.md).
//
//	nat.nptv6.bindings[i] {interface, internal, external} → npt66.binding/<if>/<internal prefix>
//
// npt66 has no dump in VPP 26.06: the descriptor is write-only (D-063), so the agent's Retrieve never reports NPTv6 and
// never echoes the desired bindings (`GET /api/v1/state/nat/nptv6` shows the running bindings with a write-only marker
// instead). assembleNptv6 still maps npt66.binding objects it is given (a projection assembled again, tests), like
// every assembler. The npt66 plugin has no enable message (it is enabled in startup.conf, D-060).

import (
	"net/netip"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/npt66"
	"ngfw/agent/internal/scheduler"
)

func nptv6Build(s Sink, n *vrxv1.Nptv6Config) {
	seen := map[string]int{}
	for i, b := range n.GetBindings() {
		pt := Ptr("nat", "nptv6", "bindings", strconv.Itoa(i))
		in, okIn := nptv6Prefix(s, b.GetInternal(), pt+"/internal")
		ex, okEx := nptv6Prefix(s, b.GetExternal(), pt+"/external")
		if !okIn || !okEx {
			continue
		}
		if in.Bits() != ex.Bits() {
			s.Errorf(pt+"/external", "nat.nptv6-valid", "internal /%d and external /%d must have the same length (RFC 6296)", in.Bits(), ex.Bits())
			continue
		}
		if in.Bits() > npt66.MaxPrefixLen {
			s.Errorf(pt+"/internal", "nat.nptv6-valid", "VPP translates prefixes of at most /%d, got /%d", npt66.MaxPrefixLen, in.Bits())
			continue
		}
		if j, dup := seen[b.GetInterface()]; dup {
			s.Errorf(pt+"/interface", "nat.nptv6-valid", "interface %q already has binding %d: VPP keeps one NPTv6 binding per interface", b.GetInterface(), j)
			continue
		}
		seen[b.GetInterface()] = i
		spec := npt66.BindingSpec{Interface: b.GetInterface(), Internal: in.String(), External: ex.String()}
		natAdd(s, npt66.NameBinding, npt66.BindingID(spec), &spec, pt)
	}
}

func nptv6Prefix(s Sink, text, pointer string) (netip.Prefix, bool) {
	p, err := netip.ParsePrefix(text)
	switch {
	case err != nil || !p.Addr().Is6() || p.Addr().Is4In6():
		s.Errorf(pointer, "nat.nptv6-valid", "%q is not an IPv6 prefix", text)
		return p, false
	case p.Masked() != p:
		s.Errorf(pointer, "nat.prefixes-are-networks", "%s has host bits set (network %s)", text, p.Masked())
		return p, false
	}
	return p, true
}

// assembleNptv6 maps npt66.binding objects to `nat.nptv6.bindings` (sorted by interface).
func assembleNptv6(out *vrxv1.NatConfig, kvs []scheduler.KV) {
	var bs []npt66.BindingSpec
	for _, kv := range kvs {
		if kv.Key.Descriptor() != npt66.NameBinding {
			continue
		}
		if v, err := natcommon.Decode[npt66.BindingSpec](kv.Value); err == nil {
			bs = append(bs, v)
		}
	}
	if len(bs) == 0 {
		return
	}
	sort.Slice(bs, func(a, b int) bool { return bs[a].Interface < bs[b].Interface })
	n := &vrxv1.Nptv6Config{}
	for _, b := range bs {
		n.Bindings = append(n.Bindings, &vrxv1.Nptv6Config_Binding{Interface: proto.String(b.Interface), Internal: proto.String(b.Internal), External: proto.String(b.External)})
	}
	out.Nptv6 = n
}
