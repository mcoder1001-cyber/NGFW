package acl

import (
	"fmt"
	"net"
	"net/netip"

	"ngfw/agent/binapi/acl_types"
	"ngfw/agent/binapi/ethernet_types"
	"ngfw/agent/binapi/ip_types"
)

// This file is the single rule codec shared by acl.acl and acl.macip-acl. VPP stores rules
// exactly as sent (acl.c acl_add_list / copy_acl_rule_to_api_rule copy the fields without
// masking or normalising), so idempotency depends on the desired text being canonical: the
// codec accepts only canonical prefixes and MACs and decodes back to the same text.

// parsePrefix accepts a canonical CIDR prefix: netip.Prefix.Masked().String() == s.
func parsePrefix(s string) (netip.Prefix, error) {
	if s == "" {
		return netip.Prefix{}, fmt.Errorf("%w: prefix is empty (use %q or %q for any)", ErrSpec, AnyV4, AnyV6)
	}
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("%w: %v", ErrSpec, err)
	}
	if canon := p.Masked().String(); canon != s {
		return netip.Prefix{}, fmt.Errorf("%w: prefix %q is not canonical, want %q", ErrSpec, s, canon)
	}
	return p, nil
}

func prefixToAPI(s string) (ip_types.Prefix, error) {
	p, err := parsePrefix(s)
	if err != nil {
		return ip_types.Prefix{}, err
	}
	var out ip_types.Prefix
	out.Len = uint8(p.Bits()) //nolint:gosec // Bits() is 0..128
	if p.Addr().Is4() {
		out.Address.Af = ip_types.ADDRESS_IP4
		out.Address.Un.SetIP4(ip_types.IP4Address(p.Addr().As4()))
	} else {
		out.Address.Af = ip_types.ADDRESS_IP6
		out.Address.Un.SetIP6(ip_types.IP6Address(p.Addr().As16()))
	}
	return out, nil
}

// prefixFromAPI decodes what VPP returned without normalising it (byte-identical round trip).
func prefixFromAPI(p ip_types.Prefix) (string, error) {
	var addr netip.Addr
	switch p.Address.Af {
	case ip_types.ADDRESS_IP4:
		addr = netip.AddrFrom4(p.Address.Un.GetIP4())
		if p.Len > 32 {
			return "", fmt.Errorf("acl: IPv4 prefix length %d from VPP", p.Len)
		}
	case ip_types.ADDRESS_IP6:
		addr = netip.AddrFrom16(p.Address.Un.GetIP6())
		if p.Len > 128 {
			return "", fmt.Errorf("acl: IPv6 prefix length %d from VPP", p.Len)
		}
	default:
		return "", fmt.Errorf("acl: unknown address family %v from VPP", p.Address.Af)
	}
	return netip.PrefixFrom(addr, int(p.Len)).String(), nil
}

// parseMAC accepts a canonical 48-bit MAC: lower-case, colon separated (net.HardwareAddr.String()).
func parseMAC(s string) (net.HardwareAddr, error) {
	if s == "" {
		return nil, fmt.Errorf("%w: MAC is empty", ErrSpec)
	}
	hw, err := net.ParseMAC(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSpec, err)
	}
	if len(hw) != 6 {
		return nil, fmt.Errorf("%w: MAC %q is not 48 bits", ErrSpec, s)
	}
	if canon := hw.String(); canon != s {
		return nil, fmt.Errorf("%w: MAC %q is not canonical, want %q", ErrSpec, s, canon)
	}
	return hw, nil
}

func macToAPI(s string) (ethernet_types.MacAddress, error) {
	hw, err := parseMAC(s)
	if err != nil {
		return ethernet_types.MacAddress{}, err
	}
	return ethernet_types.NewMacAddress(hw), nil
}

func macFromAPI(m ethernet_types.MacAddress) string { return m.ToMAC().String() }

func actionToAPI(a Action) (acl_types.ACLAction, error) {
	switch a {
	case ActionDeny:
		return acl_types.ACL_ACTION_API_DENY, nil
	case ActionPermit:
		return acl_types.ACL_ACTION_API_PERMIT, nil
	case ActionReflect:
		return acl_types.ACL_ACTION_API_PERMIT_REFLECT, nil
	default:
		return 0, fmt.Errorf("%w: action %q must be %q, %q or %q", ErrSpec, a, ActionDeny, ActionPermit, ActionReflect)
	}
}

func actionFromAPI(a acl_types.ACLAction) (Action, error) {
	switch a {
	case acl_types.ACL_ACTION_API_DENY:
		return ActionDeny, nil
	case acl_types.ACL_ACTION_API_PERMIT:
		return ActionPermit, nil
	case acl_types.ACL_ACTION_API_PERMIT_REFLECT:
		return ActionReflect, nil
	default:
		return "", fmt.Errorf("acl: unknown action %d from VPP", uint8(a))
	}
}

// encodeRules converts the desired rules to the wire type, in order. It validates as it goes.
func encodeRules(rules []Rule) ([]acl_types.ACLRule, error) {
	out := make([]acl_types.ACLRule, 0, len(rules))
	for i, r := range rules {
		if err := r.validate(); err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		action, _ := actionToAPI(r.Action)
		src, _ := prefixToAPI(r.Src)
		dst, _ := prefixToAPI(r.Dst)
		out = append(out, acl_types.ACLRule{
			IsPermit:               action,
			SrcPrefix:              src,
			DstPrefix:              dst,
			Proto:                  ip_types.IPProto(r.Proto),
			SrcportOrIcmptypeFirst: r.SrcPortFirst,
			SrcportOrIcmptypeLast:  r.SrcPortLast,
			DstportOrIcmpcodeFirst: r.DstPortFirst,
			DstportOrIcmpcodeLast:  r.DstPortLast,
			TCPFlagsMask:           r.TCPFlagsMask,
			TCPFlagsValue:          r.TCPFlagsValue,
		})
	}
	return out, nil
}

// decodeRules converts acl_details rules back, in order, byte-identical to what encodeRules sent.
func decodeRules(rules []acl_types.ACLRule) ([]Rule, error) {
	out := make([]Rule, 0, len(rules))
	for i, r := range rules {
		action, err := actionFromAPI(r.IsPermit)
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		src, err := prefixFromAPI(r.SrcPrefix)
		if err != nil {
			return nil, fmt.Errorf("rule %d src: %w", i, err)
		}
		dst, err := prefixFromAPI(r.DstPrefix)
		if err != nil {
			return nil, fmt.Errorf("rule %d dst: %w", i, err)
		}
		out = append(out, Rule{
			Action:        action,
			Src:           src,
			Dst:           dst,
			Proto:         uint8(r.Proto),
			SrcPortFirst:  r.SrcportOrIcmptypeFirst,
			SrcPortLast:   r.SrcportOrIcmptypeLast,
			DstPortFirst:  r.DstportOrIcmpcodeFirst,
			DstPortLast:   r.DstportOrIcmpcodeLast,
			TCPFlagsMask:  r.TCPFlagsMask,
			TCPFlagsValue: r.TCPFlagsValue,
		})
	}
	return out, nil
}

// encodeMacipRules converts the desired MACIP rules to the wire type, in order, validating.
func encodeMacipRules(rules []MacipRule) ([]acl_types.MacipACLRule, error) {
	out := make([]acl_types.MacipACLRule, 0, len(rules))
	for i, r := range rules {
		if err := r.validate(); err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		action, _ := actionToAPI(r.Action)
		mac, _ := macToAPI(r.SrcMac)
		mask, _ := macToAPI(r.SrcMacMask)
		src, _ := prefixToAPI(r.SrcPrefix)
		out = append(out, acl_types.MacipACLRule{IsPermit: action, SrcMac: mac, SrcMacMask: mask, SrcPrefix: src})
	}
	return out, nil
}

// decodeMacipRules converts macip_acl_details rules back, byte-identical to what was sent.
func decodeMacipRules(rules []acl_types.MacipACLRule) ([]MacipRule, error) {
	out := make([]MacipRule, 0, len(rules))
	for i, r := range rules {
		action, err := actionFromAPI(r.IsPermit)
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}
		src, err := prefixFromAPI(r.SrcPrefix)
		if err != nil {
			return nil, fmt.Errorf("rule %d src: %w", i, err)
		}
		out = append(out, MacipRule{
			Action:     action,
			SrcMac:     macFromAPI(r.SrcMac),
			SrcMacMask: macFromAPI(r.SrcMacMask),
			SrcPrefix:  src,
		})
	}
	return out, nil
}
