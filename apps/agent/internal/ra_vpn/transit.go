package ravpn

import (
	"google.golang.org/protobuf/proto"
	"net/netip"
	"ngfw/agent/internal/descriptors/tapv2"
)

// TransitTAPs has no random ID fallback: caller reserves both IDs in its
// assigned VPP range before any external mutation. Namespace is the protected
// mount reference created by NamespaceDescriptor, never an operator path.
func TransitTAPs(plan *NetworkPlan, outerID, innerID uint32) (*tapv2.Tap, *tapv2.Tap, error) {
	if plan.Validate() != nil || outerID == innerID || outerID > 8191 || innerID > 8191 {
		return nil, nil, ErrBoundary
	}
	makeTAP := func(outer bool, id uint32, link Link) *tapv2.Tap {
		name := "inner0"
		mtu := uint32(1400)
		if outer {
			name = "outer0"
			mtu = 1500
		}
		tap := &tapv2.Tap{Name: LinkName(plan.Instance, outer), Id: id, HostIfName: name, HostNamespace: NamespacePath(plan.Instance), HostMtu: mtu, RxRingSize: 256, TxRingSize: 256}
		prefix, _ := netip.ParsePrefix(link.Namespace)
		if prefix.Addr().Is4() {
			tap.HostIp4Prefix = prefix.String()
		} else {
			tap.HostIp6Prefix = prefix.String()
		}
		if !outer && plan.InnerIPv6 != nil {
			tap.HostIp6Prefix = plan.InnerIPv6.Namespace
		}
		return tap
	}
	return makeTAP(true, outerID, plan.Outer), makeTAP(false, innerID, plan.Inner), nil
}

// VerifyTransitTAP compares all observable kernel endpoint settings, not just
// the owner tag or sw_if_index. Fresh owner-filtered VPP dump is mandatory.
func VerifyTransitTAP(expected, observed *tapv2.Tap) error {
	if expected == nil || observed == nil || !proto.Equal(expected, observed) {
		return ErrBoundary
	}
	return nil
}
