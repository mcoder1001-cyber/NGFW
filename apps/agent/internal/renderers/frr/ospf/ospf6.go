package ospf

import (
	"errors"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/policy"
	"strings"
)

// OSPFv3 uses the same area/timer checks but renders its own supported command family.
func error6(err error) error {
	var fe *policy.FieldError
	if errors.As(err, &fe) && len(fe.Path) > 1 && fe.Path[1] == "ospf" {
		path := append(policy.Path(nil), fe.Path...)
		path[1] = "ospf6"
		return &policy.FieldError{Path: path, Err: fe.Err}
	}
	return err
}

// Section6 renders the OSPFv3 process configuration.
type Section6 struct{}

// Name returns the renderer section identifier.
func (Section6) Name() string { return "ospf6" }

// Order places OSPFv3 after the other IGP sections.
func (Section6) Order() int { return 450 }
func v2(o *ngfwv1.Ospf6Config) *ngfwv1.OspfConfig {
	if o == nil {
		return nil
	}
	out := &ngfwv1.OspfConfig{RouterId: o.RouterId, Vrf: o.Vrf, Areas: o.Areas, Redistribute: o.Redistribute, Interfaces: map[string]*ngfwv1.OspfInterface{}}
	for name, i := range o.Interfaces {
		out.Interfaces[name] = &ngfwv1.OspfInterface{Area: i.Area, Cost: i.Cost, Passive: i.Passive, NetworkType: i.NetworkType, HelloIntervalSec: i.HelloIntervalSec, DeadIntervalSec: i.DeadIntervalSec, Priority: i.Priority}
	}
	return out
}

// Render validates and renders the desired OSPFv3 process.
func (Section6) Render(rc *frr.RenderContext) ([]string, error) {
	lines, err := Render(v2(rc.Desired.GetRouting().GetOspf6()))
	if err != nil {
		return nil, error6(err)
	}
	for i, line := range lines {
		if strings.HasPrefix(line, " redistribute rip") {
			lines[i] = strings.Replace(line, " redistribute rip", " redistribute ripng", 1)
		}
		if strings.HasPrefix(line, "router ospf") {
			lines[i] = strings.Replace(line, "router ospf", "router ospf6", 1)
		}
		if strings.HasPrefix(line, " ospf router-id") {
			lines[i] = strings.Replace(line, " ospf router-id", " ospf6 router-id", 1)
		}
	}
	return lines, nil
}

// InterfaceLines6 renders validated per-interface OSPFv3 commands.
func InterfaceLines6(rc *frr.RenderContext) (map[string][]string, error) {
	o := rc.Desired.GetRouting().GetOspf6()
	for name, iface := range o.GetInterfaces() {
		if iface.GetNetworkType() == "non-broadcast" {
			return nil, policy.Errf(policy.P("routing", "ospf6", "interfaces", name), "OSPFv3 does not support v2 auth, BFD or NBMA in this model")
		}
	}
	out, err := RenderInterfaces(v2(o), rc.MapInterface)
	if err != nil {
		return nil, error6(err)
	}
	for name, lines := range out {
		for i, line := range lines {
			lines[i] = strings.Replace(line, " ip ospf ", " ipv6 ospf6 ", 1)
		}
		out[name] = lines
	}
	return out, nil
}
