// Package pim renders the existing IPv4 PIM-SM contract through FRR seams S2.
package pim

import (
	"fmt"
	"net/netip"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/frr"
	"slices"
)

const Name = "pim"
const ShowMroute frr.ShowCommand = "show ip mroute json"

func init() {
	frr.RegisterSection(Section{})
	frr.RegisterInterfaceLines(Name, InterfaceLines)
}

type Section struct{}

func (Section) Name() string { return Name }
func (Section) Order() int   { return 650 }
func (Section) Render(rc *frr.RenderContext) ([]string, error) {
	return Render(rc.Desired.GetRouting().GetMulticast().GetPim())
}

// Render emits deterministic RP commands, rejecting unsafe or unsupported tokens.
func Render(c *ngfwv1.PimConfig) ([]string, error) {
	var lines []string
	seen := map[string]bool{}
	for _, rp := range c.GetRp() {
		a, e := netip.ParseAddr(rp.GetAddress())
		if e != nil || !a.Is4() || !a.IsGlobalUnicast() {
			return nil, fmt.Errorf("PIM RP requires IPv4 unicast")
		}
		groups := rp.GetGroups()
		if len(groups) == 0 {
			groups = []string{"224.0.0.0/4"}
		}
		for _, g := range groups {
			p, e := netip.ParsePrefix(g)
			if e != nil || !p.Addr().Is4() || p != p.Masked() || p.Bits() < 4 || !p.Addr().IsMulticast() {
				return nil, fmt.Errorf("PIM RP group requires canonical IPv4 multicast prefix")
			}
			key := p.String()
			if seen[key] {
				return nil, fmt.Errorf("duplicate PIM RP group range")
			}
			seen[key] = true
			lines = append(lines, "ip pim rp "+a.String()+" "+p.String())
		}
	}
	slices.Sort(lines)
	return lines, nil
}

// InterfaceLines adds ip pim inside the framework's single interface block.
func InterfaceLines(rc *frr.RenderContext) (map[string][]string, error) {
	out := map[string][]string{}
	for _, logical := range rc.Desired.GetRouting().GetMulticast().GetPim().GetInterfaces() {
		host, ok := rc.MapInterface(logical)
		if !ok {
			return nil, fmt.Errorf("PIM interface has no LCP mapping")
		}
		if _, e := frr.IfName(host); e != nil {
			return nil, e
		}
		if _, ok := out[host]; ok {
			return nil, fmt.Errorf("duplicate PIM interface mapping")
		}
		out[host] = []string{" ip pim"}
	}
	return out, nil
}
