// Package rip renders `routing.rip` into FRR's `router rip` block (the `rip` section of the RF-1 framework, order 420
// per docs/status/wave-BC-numbers.md, F-isis-rip). RIPng (`ripng`, order 430) belongs here too but has no model yet
// (`routing.ripng` is an additive contract that does not exist) and is not registered. Canonical forms and open points:
// docs/agent/renderers/frr-rip.md.
//
// Rendering rules:
//   - always `version 2` (RIPv1 is out of scope);
//   - networks are IPv4 prefixes in canonical form (host bits clear), each once;
//   - every interface under `interfaces` is enabled with `network <linux ifname>` (VPP names mapped through
//     rc.MapInterface; unmapped = error at its pointer) and `passive-interface <ifname>` when passive;
//   - default-metric and redistribution metrics are RIP hop counts (1–16 / 0–16); RIP never redistributes into itself.
//
// RIP has no interface-level lines, so this package registers no interface-lines producer. FRR 10 has no JSON form of
// `show ip rip status` / `show ip rip`, so there is no state reader (the framework takes `show … json` only); RIP routes
// are visible through the framework's `show ip route … json` readers (proto rip).
package rip

import (
	"maps"
	"net/netip"
	"slices"
	"strconv"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/policy"
)

// OrderRIP places `router rip` (wave-BC-numbers.md: rip 420, ripng 430).
const OrderRIP = 420

// Name is the section name.
const Name = "rip"

func init() {
	frr.RegisterSection(Section{})
}

// Section is the frr.Section of `routing.rip`.
type Section struct{}

// Name implements frr.Section.
func (Section) Name() string { return Name }

// Order implements frr.Section.
func (Section) Order() int { return OrderRIP }

// Render implements frr.Section.
func (Section) Render(rc *frr.RenderContext) ([]string, error) {
	return Render(rc.Desired.GetRouting().GetRip(), rc.MapInterface)
}

var base = policy.P("routing", "rip")

// Render returns the `router rip` block for r (nil = RIP not configured).
func Render(r *vrxv1.RipConfig, mapIf frr.InterfaceMapper) ([]string, error) {
	if r == nil {
		return nil, nil
	}
	head := "router rip"
	if v := r.GetVrf(); v != "" && v != frr.DefaultVRF {
		name, err := frr.VRFName(v)
		if err != nil {
			return nil, policy.Wrap(base.At("vrf"), err)
		}
		head += " vrf " + name
	}
	out := []string{head, " version 2"}
	if r.DefaultMetric != nil {
		if m := r.GetDefaultMetric(); m < 1 || m > 16 {
			return nil, policy.Errf(base.At("defaultMetric"), "default metric %d not in 1–16", m)
		}
		out = append(out, " default-metric "+strconv.FormatUint(uint64(r.GetDefaultMetric()), 10))
	}
	seen := map[netip.Prefix]bool{}
	for i, n := range r.GetNetworks() {
		p, err := netip.ParsePrefix(n)
		path := base.At("networks", strconv.Itoa(i))
		if err != nil || !p.Addr().Is4() {
			return nil, policy.Errf(path, "network %q is not an IPv4 prefix", n)
		}
		if p.Masked() != p {
			return nil, policy.Errf(path, "network %q has host bits set (%s)", n, p.Masked())
		}
		if seen[p] {
			return nil, policy.Errf(path, "network %s is listed twice", p)
		}
		seen[p] = true
		out = append(out, " network "+p.String())
	}
	var passive []string
	owner := map[string]string{}
	for _, vppName := range slices.Sorted(maps.Keys(r.GetInterfaces())) {
		path := base.At("interfaces", vppName)
		linux, ok := mapIf(vppName)
		if !ok {
			return nil, policy.Errf(path, "interface %q has no Linux interface for FRR (interfaces.%s.lcp)", vppName, vppName)
		}
		name, err := frr.IfName(linux)
		if err != nil {
			return nil, policy.Wrap(path, err)
		}
		if prev, dup := owner[name]; dup {
			return nil, policy.Errf(path, "interfaces %q and %q map to the same Linux interface %q", prev, vppName, name)
		}
		owner[name] = vppName
		out = append(out, " network "+name)
		if r.GetInterfaces()[vppName].GetPassive() {
			passive = append(passive, " passive-interface "+name)
		}
	}
	out = append(out, passive...)
	redist, err := redistribute(r.GetRedistribute())
	if err != nil {
		return nil, err
	}
	out = append(out, redist...)
	return append(out, "exit"), nil
}

func redistribute(r *vrxv1.Redistribute) ([]string, error) {
	if r == nil {
		return nil, nil
	}
	if r.GetRip() != nil {
		return nil, policy.Errf(base.At("redistribute", "rip"), "RIP cannot redistribute into itself")
	}
	var out []string
	for _, src := range []struct {
		name string
		opt  *vrxv1.RedistributeOptions
	}{
		{"connected", r.GetConnected()}, {"static", r.GetStatic()}, {"ospf", r.GetOspf()},
		{"isis", r.GetIsis()}, {"bgp", r.GetBgp()},
	} {
		if src.opt == nil {
			continue
		}
		line := " redistribute " + src.name
		if src.opt.Metric != nil {
			if m := src.opt.GetMetric(); m > 16 {
				return nil, policy.Errf(base.At("redistribute", src.name, "metric"), "metric %d not in 0–16", m)
			}
			line += " metric " + strconv.FormatUint(uint64(src.opt.GetMetric()), 10)
		}
		if rm := src.opt.GetRouteMap(); rm != "" {
			if _, err := policy.ObjectName("route map", rm); err != nil {
				return nil, policy.Wrap(base.At("redistribute", src.name, "routeMap"), err)
			}
			line += " route-map " + rm
		}
		out = append(out, line)
	}
	return out, nil
}
