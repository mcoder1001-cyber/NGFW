// Package ospf renders `routing.ospf` into FRR's `router ospf` block (the `ospf` section of the RF-1 framework, order
// 440 per docs/status/wave-BC-numbers.md, F-ospf) and its per-interface `ip ospf …` lines through the framework's
// interface-lines seam (S2), and reads OSPF state back (`show ip ospf vrf all neighbor json` /
// `… interface json` state readers, the `ospf-neighbors` poller). Canonical forms and open points:
// docs/agent/renderers/frr-ospf.md.
//
// Rendering rules:
//   - an interface's `area` must name an area under `areas` (area 0 is not implied, D-070); the interface line uses the
//     area id exactly as the `areas` key spells it (FRR remembers the decimal / dotted-quad format per area);
//   - stub / NSSA is refused for the backbone (0 / 0.0.0.0);
//   - interface names are VPP names mapped to their Linux (linux-cp) names with rc.MapInterface; an unmapped
//     interface is an error at its pointer;
//   - redistribution lines are the router block's own (`redistribute <src> [metric N] [route-map R]`), in FRR's
//     route-type order; OSPF never redistributes into itself;
//   - OSPFv3 (`ospf6`, order 450) is registered separately, using the same validated area/timer semantics.
package ospf

import (
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/policy"
)

// OrderOSPF places `router ospf` (wave-BC-numbers.md: ospf 440, ospf6 450).
const OrderOSPF = 440

// Name is the section name (also the interface-lines producer name).
const Name = "ospf"

func init() {
	frr.RegisterStateReader(frr.StateReader{Key: "ospfDatabase", OnDemand: true, Command: "show ip ospf vrf all database json"})
	frr.RegisterStateReader(frr.StateReader{Key: "ospf6Database", OnDemand: true, Command: "show ipv6 ospf6 vrf all database json"})
	frr.RegisterSection(Section{})
	frr.RegisterSection(Section6{})
	frr.RegisterInterfaceLines("ospf6", InterfaceLines6)
	frr.RegisterStateReader(frr.StateReader{Key: NeighborsReader6, OnDemand: true, Command: ShowNeighbors6})
	frr.RegisterStateReader(frr.StateReader{Key: InterfacesReader6, OnDemand: true, Command: ShowInterfaces6})
	frr.RegisterPoller("ospf6-neighbors", PollNeighbors6)
	frr.RegisterInterfaceLines(Name, InterfaceLines)
	frr.RegisterStateReader(frr.StateReader{Key: NeighborsReader, Command: ShowNeighbors})
	frr.RegisterStateReader(frr.StateReader{Key: InterfacesReader, Command: ShowInterfaces})
	frr.RegisterPoller(PollerNeighbors, PollNeighbors)
}

// Section is the frr.Section of `routing.ospf`.
type Section struct{}

// Name implements frr.Section.
func (Section) Name() string { return Name }

// Order implements frr.Section.
func (Section) Order() int { return OrderOSPF }

// Render implements frr.Section.
func (Section) Render(rc *frr.RenderContext) ([]string, error) {
	return Render(rc.Desired.GetRouting().GetOspf())
}

// InterfaceLines is the frr.InterfaceLinesFunc of `routing.ospf.interfaces`.
func InterfaceLines(rc *frr.RenderContext) (map[string][]string, error) {
	o := rc.Desired.GetRouting().GetOspf()
	out, err := RenderInterfaces(o, rc.MapInterface)
	if err != nil {
		return nil, err
	}
	for vppName, iface := range o.GetInterfaces() {
		auth := iface.GetAuth()
		if auth == nil {
			continue
		}
		linux, _ := rc.MapInterface(vppName)
		path := base.At("interfaces", vppName, "auth")
		switch auth.GetType() {
		case "none":
			if auth.GetKeyRef() != "" || auth.KeyId != nil {
				return nil, policy.Errf(path, "authentication none cannot carry a key")
			}
			out[linux] = append(out[linux], " ip ospf authentication null")
		case "md5":
			if auth.GetKeyId() < 1 || auth.GetKeyId() > 255 || !strings.HasPrefix(auth.GetKeyRef(), "password/") {
				return nil, policy.Errf(path, "MD5 requires key id 1–255 and a password reference")
			}
			key, err := rc.Secret(auth.GetKeyRef())
			if err != nil {
				return nil, policy.Wrap(path.At("keyRef"), err)
			}
			if len(key) > 16 {
				return nil, policy.Errf(path.At("keyRef"), "OSPF MD5 key must not exceed 16 bytes")
			}
			out[linux] = append(out[linux], " ip ospf authentication message-digest", fmt.Sprintf(" ip ospf message-digest-key %d md5 %s", auth.GetKeyId(), key))
		default:
			return nil, policy.Errf(path.At("type"), "unsupported authentication type")
		}
	}
	return out, nil
}

var base = policy.P("routing", "ospf")

// area is one parsed area id.
type area struct {
	key string // document key, rendered as is
	num uint32
}

// parseArea accepts a decimal uint32 or a dotted quad.
func parseArea(s string) (uint32, bool) {
	if n, err := strconv.ParseUint(s, 10, 32); err == nil && (s == "0" || s[0] != '0') {
		return uint32(n), true
	}
	if a, err := netip.ParseAddr(s); err == nil && a.Is4() && a.String() == s {
		b := a.As4()
		return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]), true
	}
	return 0, false
}

// areas returns the areas by number (every key validated, duplicates refused).
func areas(o *ngfwv1.OspfConfig) (map[uint32]area, error) {
	out := map[uint32]area{}
	for _, k := range slices.Sorted(maps.Keys(o.GetAreas())) {
		n, ok := parseArea(k)
		if !ok {
			return nil, policy.Errf(base.At("areas", k), "area id %q is not a decimal number or dotted quad", k)
		}
		if prev, dup := out[n]; dup {
			return nil, policy.Errf(base.At("areas", k), "areas %q and %q are the same area", prev.key, k)
		}
		out[n] = area{key: k, num: n}
	}
	return out, nil
}

// Render returns the `router ospf` block for o (nil = OSPF not configured).
func Render(o *ngfwv1.OspfConfig) ([]string, error) {
	if o == nil {
		return nil, nil
	}
	head := "router ospf"
	if v := o.GetVrf(); v != "" && v != frr.DefaultVRF {
		name, err := frr.VRFName(v)
		if err != nil {
			return nil, policy.Wrap(base.At("vrf"), err)
		}
		head += " vrf " + name
	}
	out := []string{head}
	if rid := o.GetRouterId(); rid != "" {
		a, err := netip.ParseAddr(rid)
		if err != nil || !a.Is4() {
			return nil, policy.Wrap(base.At("routerId"), fmt.Errorf("%w: router id %q is not a dotted quad", renderers.ErrUnsafe, rid))
		}
		out = append(out, " ospf router-id "+a.String())
	}
	redist, err := redistribute(o.GetRedistribute())
	if err != nil {
		return nil, err
	}
	out = append(out, redist...)
	byNum, err := areas(o)
	if err != nil {
		return nil, err
	}
	for _, n := range slices.Sorted(maps.Keys(byNum)) {
		a := byNum[n]
		cfg := o.GetAreas()[a.key]
		path := base.At("areas", a.key)
		t := cfg.GetType()
		switch t {
		case "", "normal":
			if cfg.GetNoSummary() {
				return nil, policy.Errf(path.At("noSummary"), "noSummary needs a stub or NSSA area")
			}
			continue
		case "stub", "nssa":
		default:
			return nil, policy.Errf(path.At("type"), "area type %q is not normal, stub or nssa", t)
		}
		if n == 0 {
			return nil, policy.Errf(path.At("type"), "the backbone area cannot be %s", t)
		}
		line := " area " + a.key + " " + t
		if cfg.GetNoSummary() {
			line += " no-summary"
		}
		out = append(out, line)
	}
	switch d := o.GetDefaultInformationOriginate(); d {
	case "", "off":
	case "on":
		out = append(out, " default-information originate")
	case "always":
		out = append(out, " default-information originate always")
	default:
		return nil, policy.Errf(base.At("defaultInformationOriginate"), "%q is not off, on or always", d)
	}
	return append(out, "exit"), nil
}

// redistribute renders the router block's `redistribute` lines in FRR's route-type order.
func redistribute(r *ngfwv1.Redistribute) ([]string, error) {
	if r == nil {
		return nil, nil
	}
	if r.GetOspf() != nil {
		return nil, policy.Errf(base.At("redistribute", "ospf"), "OSPF cannot redistribute into itself")
	}
	var out []string
	for _, src := range []struct {
		name string
		opt  *ngfwv1.RedistributeOptions
	}{
		{"connected", r.GetConnected()}, {"static", r.GetStatic()}, {"rip", r.GetRip()},
		{"isis", r.GetIsis()}, {"bgp", r.GetBgp()},
	} {
		if src.opt == nil {
			continue
		}
		line := " redistribute " + src.name
		if src.opt.Metric != nil {
			if m := src.opt.GetMetric(); m > 16777214 {
				return nil, policy.Errf(base.At("redistribute", src.name, "metric"), "metric %d not in 0–16777214", m)
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

var networkTypes = []string{"broadcast", "non-broadcast", "point-to-multipoint", "point-to-point"}

// RenderInterfaces returns the `ip ospf …` lines per Linux interface (nil = OSPF not configured).
func RenderInterfaces(o *ngfwv1.OspfConfig, mapIf frr.InterfaceMapper) (map[string][]string, error) {
	if o == nil || len(o.GetInterfaces()) == 0 {
		return nil, nil
	}
	byNum, err := areas(o)
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
		n, ok := parseArea(itf.GetArea())
		if !ok {
			return nil, policy.Errf(path.At("area"), "area id %q is not a decimal number or dotted quad", itf.GetArea())
		}
		a, ok := byNum[n]
		if !ok {
			return nil, policy.Errf(path.At("area"), "area %q is not defined under routing.ospf.areas", itf.GetArea())
		}
		var lines []string
		if nt := itf.GetNetworkType(); nt != "" {
			if !slices.Contains(networkTypes, nt) {
				return nil, policy.Errf(path.At("networkType"), "network type %q is not one of %s", nt, strings.Join(networkTypes, ", "))
			}
			lines = append(lines, " ip ospf network "+nt)
		}
		if itf.Cost != nil {
			if c := itf.GetCost(); c < 1 || c > 65535 {
				return nil, policy.Errf(path.At("cost"), "cost %d not in 1–65535", c)
			}
			lines = append(lines, fmt.Sprintf(" ip ospf cost %d", itf.GetCost()))
		}
		hello, dead := itf.GetHelloIntervalSec(), itf.GetDeadIntervalSec()
		if itf.HelloIntervalSec != nil {
			if hello < 1 || hello > 65535 {
				return nil, policy.Errf(path.At("helloIntervalSec"), "hello interval %d not in 1–65535", hello)
			}
			lines = append(lines, fmt.Sprintf(" ip ospf hello-interval %d", hello))
		}
		if itf.DeadIntervalSec != nil {
			if dead < 1 || dead > 65535 {
				return nil, policy.Errf(path.At("deadIntervalSec"), "dead interval %d not in 1–65535", dead)
			}
			if itf.HelloIntervalSec != nil && dead <= hello {
				return nil, policy.Errf(path.At("deadIntervalSec"), "dead interval %d must exceed the hello interval %d", dead, hello)
			}
			lines = append(lines, fmt.Sprintf(" ip ospf dead-interval %d", dead))
		}
		if itf.Priority != nil {
			if p := itf.GetPriority(); p > 255 {
				return nil, policy.Errf(path.At("priority"), "priority %d not in 0–255", p)
			}
			lines = append(lines, fmt.Sprintf(" ip ospf priority %d", itf.GetPriority()))
		}
		lines = append(lines, " ip ospf area "+a.key)
		if itf.GetBfd() {
			lines = append(lines, " ip ospf bfd")
		}
		if itf.GetPassive() {
			lines = append(lines, " ip ospf passive")
		}
		out[name] = lines
	}
	return out, nil
}
