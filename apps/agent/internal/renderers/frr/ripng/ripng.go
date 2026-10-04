package ripng

import (
	"maps"
	"net/netip"
	"slices"
	"strconv"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/policy"
)

// OrderRIP places `router rip` (wave-BC-numbers.md: rip 420, ripng 430).
const OrderRIP = 430

// Name is the section name.
const Name = "ripng"

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
	return Render(rc.Desired.GetRouting().GetRipng(), rc.MapInterface)
}

var base = policy.P("routing", "ripng")

// Render returns the `router rip` block for r (nil = RIP not configured).
func Render(r *ngfwv1.RipngConfig, mapIf frr.InterfaceMapper) ([]string, error) {
	if r == nil {
		return nil, nil
	}
	head := "router ripng"
	if v := r.GetVrf(); v != "" && v != frr.DefaultVRF {
		name, err := frr.VRFName(v)
		if err != nil {
			return nil, policy.Wrap(base.At("vrf"), err)
		}
		head += " vrf " + name
	}
	out := []string{head}
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
		if err != nil || !p.Addr().Is6() || p.Addr().Is4In6() {
			return nil, policy.Errf(path, "network %q is not an IPv6 prefix", n)
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

func redistribute(r *ngfwv1.Redistribute) ([]string, error) {
	if r == nil {
		return nil, nil
	}
	if r.GetRip() != nil {
		return nil, policy.Errf(base.At("redistribute", "rip"), "RIP cannot redistribute into itself")
	}
	var out []string
	for _, src := range []struct {
		name string
		opt  *ngfwv1.RedistributeOptions
	}{
		{"connected", r.GetConnected()}, {"static", r.GetStatic()}, {"ospf", r.GetOspf()},
		{"isis", r.GetIsis()}, {"bgp", r.GetBgp()},
	} {
		if src.opt == nil {
			continue
		}
		keyword := src.name
		if keyword == "ospf" {
			keyword = "ospf6"
		}
		line := " redistribute " + keyword
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
