// Package pim renders the existing IPv4 PIM-SM contract through FRR's section
// and interface-line seams. Importing this package registers it; agent wiring
// and live daemon acceptance are separate work.
package pim

import (
	"net/netip"
	"slices"
	"strings"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/policy"
)

const Name = "pim"
const OrderPIM = 480

var base = policy.P("routing", "multicast", "pim")

func init() {
	frr.RegisterSection(Section{})
	frr.RegisterInterfaceLines(Name, InterfaceLines)
}

type Section struct{}

func (Section) Name() string { return Name }
func (Section) Order() int   { return OrderPIM }
func (Section) Render(rc *frr.RenderContext) ([]string, error) {
	if _, err := InterfaceLines(rc); err != nil {
		return nil, err
	}
	return Render(rc.Desired.GetRouting().GetMulticast().GetPim())
}

// Render produces global static RP lines; the contract has no per-VRF RP field.
func Render(p *vrxv1.PimConfig) ([]string, error) {
	if p == nil {
		return nil, nil
	}
	if len(p.Rp) > 64 {
		return nil, policy.Errf(base.At("rp"), "too many rendezvous points")
	}
	var lines []string
	seen := map[netip.Prefix]bool{}
	for i, rp := range p.Rp {
		a, err := netip.ParseAddr(rp.GetAddress())
		path := base.At("rp").Index(i)
		if err != nil || !a.Is4() || a.String() != rp.GetAddress() || !a.IsGlobalUnicast() {
			return nil, policy.Errf(path.At("address"), "RP must be a canonical IPv4 unicast address")
		}
		if len(rp.GetGroups()) == 0 || len(rp.GetGroups()) > 64 {
			return nil, policy.Errf(path.At("groups"), "RP requires 1–64 group prefixes")
		}
		for j, raw := range rp.GetGroups() {
			g, err := netip.ParsePrefix(raw)
			if err != nil || !g.Addr().Is4() || g.Masked() != g || g.String() != raw || g.Bits() < 4 || !g.Addr().IsMulticast() {
				return nil, policy.Errf(path.At("groups").Index(j), "group range must be a canonical IPv4 multicast prefix")
			}
			for prev := range seen {
				if prev.Overlaps(g) {
					return nil, policy.Errf(path.At("groups").Index(j), "overlapping RP group ranges")
				}
			}
			seen[g] = true
			lines = append(lines, "ip pim rp "+a.String()+" "+g.String())
		}
	}
	slices.Sort(lines)
	return lines, nil
}

// InterfaceLines refuses absent/non-LCP/aliased names and cross-VRF RP ambiguity.
func InterfaceLines(rc *frr.RenderContext) (map[string][]string, error) {
	p := rc.Desired.GetRouting().GetMulticast().GetPim()
	out := map[string][]string{}
	if len(p.GetInterfaces()) > 256 {
		return nil, policy.Errf(base.At("interfaces"), "too many PIM interfaces")
	}
	seen := map[string]bool{}
	for i, logical := range p.GetInterfaces() {
		path := base.At("interfaces").Index(i)
		itf, exists := rc.Desired.GetInterfaces()[logical]
		if !exists || itf == nil || itf.GetLcp() == nil || strings.TrimSpace(logical) != logical || logical == "" {
			return nil, policy.Errf(path, "PIM interface must exist with a Linux control-plane pair")
		}
		if seen[logical] {
			return nil, policy.Errf(path, "duplicate PIM interface")
		}
		seen[logical] = true
		if len(p.GetRp()) > 0 && itf.GetVrf() != "" && itf.GetVrf() != frr.DefaultVRF {
			return nil, policy.Errf(path, "global RP contract cannot represent a non-default VRF RP")
		}
		if rc.MapInterface == nil {
			return nil, policy.Errf(path, "interface mapper is unavailable")
		}
		linux, ok := rc.MapInterface(logical)
		if !ok {
			return nil, policy.Errf(path, "PIM interface has no Linux mapping")
		}
		if _, err := frr.IfName(linux); err != nil {
			return nil, policy.Wrap(path, err)
		}
		if _, dup := out[linux]; dup {
			return nil, policy.Errf(path, "PIM interfaces share a Linux mapping")
		}
		out[linux] = []string{" ip pim"}
	}
	return out, nil
}
