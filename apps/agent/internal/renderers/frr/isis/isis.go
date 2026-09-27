// Package isis renders `routing.isis` into FRR's `router isis vrx` block (the `isis` section of the RF-1 framework,
// order 470 per docs/status/wave-BC-numbers.md, F-isis-rip) and its per-interface `ip router isis` / `isis …` lines
// through the framework's interface-lines seam (S2), and reads IS-IS adjacencies back (`show isis vrf all neighbor
// json` state reader, the `isis-adjacencies` poller). Canonical forms and open points: docs/agent/renderers/frr-isis.md.
//
// Rendering rules:
//   - `net` is required and must be an ISO NET (`AA[.AAAA…].SSSS.SSSS.SSSS.00`, area 1–13 bytes, NSEL 00);
//   - an interface's circuitType must be within the IS level (a level-1 IS cannot run a level-2 circuit);
//   - every interface runs both address families (`ip router isis` + `ipv6 router isis`): the per-family switch is an
//     additive contract that does not exist yet;
//   - metrics are wide (`metric-style wide`), 1–16777215;
//   - redistribution is rendered per family and per level the IS runs; IS-IS never redistributes into itself.
package isis

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/policy"
)

// OrderISIS places `router isis` (wave-BC-numbers.md: isis 470).
const OrderISIS = 470

// Name is the section name (also the interface-lines producer name).
const Name = "isis"

// Tag is the FRR IS-IS instance name the agent owns.
const Tag = "vrx"

func init() {
	frr.RegisterSection(Section{})
	frr.RegisterInterfaceLines(Name, InterfaceLines)
	frr.RegisterStateReader(frr.StateReader{Key: NeighborsReader, Command: ShowNeighbors})
	frr.RegisterPoller(PollerAdjacencies, PollAdjacencies)
}

// Section is the frr.Section of `routing.isis`.
type Section struct{}

// Name implements frr.Section.
func (Section) Name() string { return Name }

// Order implements frr.Section.
func (Section) Order() int { return OrderISIS }

// Render implements frr.Section.
func (Section) Render(rc *frr.RenderContext) ([]string, error) {
	return Render(rc.Desired.GetRouting().GetIsis())
}

// InterfaceLines is the frr.InterfaceLinesFunc of `routing.isis.interfaces`.
func InterfaceLines(rc *frr.RenderContext) (map[string][]string, error) {
	return RenderInterfaces(rc.Desired.GetRouting().GetIsis(), rc.MapInterface)
}

var base = policy.P("routing", "isis")

var netRe = regexp.MustCompile(`^[0-9a-f]{2}(\.[0-9a-f]{4}){3,9}\.00$`)

// levels maps the document's level names to FRR's keywords and the levels they cover (bit 1 = L1, bit 2 = L2).
var levels = map[string]struct {
	isType, circuit string
	mask            int
}{
	"level-1":   {"level-1", "level-1", 1},
	"level-2":   {"level-2-only", "level-2-only", 2},
	"level-1-2": {"level-1-2", "level-1-2", 3},
}

func level(o *vrxv1.IsisConfig) (string, error) {
	l := o.GetLevel()
	if l == "" {
		l = "level-1-2"
	}
	if _, ok := levels[l]; !ok {
		return "", policy.Errf(base.At("level"), "level %q is not level-1, level-2 or level-1-2", l)
	}
	return l, nil
}

// Render returns the `router isis vrx` block for o (nil = IS-IS not configured).
func Render(o *vrxv1.IsisConfig) ([]string, error) {
	if o == nil {
		return nil, nil
	}
	head := "router isis " + Tag
	if v := o.GetVrf(); v != "" && v != frr.DefaultVRF {
		name, err := frr.VRFName(v)
		if err != nil {
			return nil, policy.Wrap(base.At("vrf"), err)
		}
		head += " vrf " + name
	}
	lvl, err := level(o)
	if err != nil {
		return nil, err
	}
	n := strings.ToLower(o.GetNet())
	if n == "" {
		return nil, policy.Errf(base.At("net"), "net is required")
	}
	if !netRe.MatchString(n) {
		return nil, policy.Errf(base.At("net"), "net %q is not an ISO NET (AA.AAAA.SSSS.SSSS.SSSS.00)", o.GetNet())
	}
	out := []string{head, " is-type " + levels[lvl].isType, " net " + n, " metric-style wide"}
	redist, err := redistribute(o.GetRedistribute(), lvl)
	if err != nil {
		return nil, err
	}
	out = append(out, redist...)
	return append(out, "exit"), nil
}

// redistribute renders `redistribute ipv4|ipv6 <src> level-N …` lines (FRR's route-type order, both families where the
// source has an IPv6 twin under the same name).
func redistribute(r *vrxv1.Redistribute, lvl string) ([]string, error) {
	if r == nil {
		return nil, nil
	}
	if r.GetIsis() != nil {
		return nil, policy.Errf(base.At("redistribute", "isis"), "IS-IS cannot redistribute into itself")
	}
	var lvls []string
	if m := levels[lvl].mask; m&1 != 0 {
		lvls = append(lvls, "level-1")
	}
	if levels[lvl].mask&2 != 0 {
		lvls = append(lvls, "level-2")
	}
	var out []string
	for _, src := range []struct {
		name, v6 string
		opt      *vrxv1.RedistributeOptions
	}{
		{"connected", "connected", r.GetConnected()}, {"static", "static", r.GetStatic()}, {"rip", "", r.GetRip()},
		{"ospf", "", r.GetOspf()}, {"bgp", "bgp", r.GetBgp()},
	} {
		if src.opt == nil {
			continue
		}
		tail := ""
		if src.opt.Metric != nil {
			if m := src.opt.GetMetric(); m > 16777215 {
				return nil, policy.Errf(base.At("redistribute", src.name, "metric"), "metric %d not in 0–16777215", m)
			}
			tail += " metric " + strconv.FormatUint(uint64(src.opt.GetMetric()), 10)
		}
		if rm := src.opt.GetRouteMap(); rm != "" {
			if _, err := policy.ObjectName("route map", rm); err != nil {
				return nil, policy.Wrap(base.At("redistribute", src.name, "routeMap"), err)
			}
			tail += " route-map " + rm
		}
		for _, l := range lvls {
			out = append(out, " redistribute ipv4 "+src.name+" "+l+tail)
			if src.v6 != "" {
				out = append(out, " redistribute ipv6 "+src.v6+" "+l+tail)
			}
		}
	}
	return out, nil
}

// RenderInterfaces returns the IS-IS interface lines per Linux interface (nil = IS-IS not configured).
func RenderInterfaces(o *vrxv1.IsisConfig, mapIf frr.InterfaceMapper) (map[string][]string, error) {
	if o == nil || len(o.GetInterfaces()) == 0 {
		return nil, nil
	}
	lvl, err := level(o)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	owner := map[string]string{}
	for _, vppName := range slices.Sorted(maps.Keys(o.GetInterfaces())) {
		itf := o.GetInterfaces()[vppName]
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
		lines := []string{" ip router isis " + Tag, " ipv6 router isis " + Tag}
		if ct := itf.GetCircuitType(); ct != "" {
			c, ok := levels[ct]
			if !ok {
				return nil, policy.Errf(path.At("circuitType"), "circuit type %q is not level-1, level-2 or level-1-2", ct)
			}
			if c.mask&^levels[lvl].mask != 0 {
				return nil, policy.Errf(path.At("circuitType"), "a %s IS cannot run a %s circuit", lvl, ct)
			}
			lines = append(lines, " isis circuit-type "+c.circuit)
		}
		switch nt := itf.GetNetworkType(); nt {
		case "", "broadcast":
		case "point-to-point":
			lines = append(lines, " isis network point-to-point")
		default:
			return nil, policy.Errf(path.At("networkType"), "network type %q is not broadcast or point-to-point", nt)
		}
		if itf.Metric != nil {
			if m := itf.GetMetric(); m < 1 || m > 16777215 {
				return nil, policy.Errf(path.At("metric"), "metric %d not in 1–16777215", m)
			}
			lines = append(lines, fmt.Sprintf(" isis metric %d", itf.GetMetric()))
		}
		if itf.GetPassive() {
			lines = append(lines, " isis passive")
		}
		if itf.GetBfd() {
			lines = append(lines, " isis bfd")
		}
		out[name] = lines
	}
	return out, nil
}
