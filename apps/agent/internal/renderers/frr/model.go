package frr

import (
	"cmp"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

// DefaultVRF is the name of FRR's (and the document's) default VRF; it never gets a
// `vrf` block.
const DefaultVRF = "default"

// ErrInput is wrapped by every error about the desired state (as opposed to I/O or daemon
// errors), so the commit engine can report it as a validation issue.
var ErrInput = errors.New("frr: invalid desired state")

// Model is the typed, validated view of the framework part of the desired state: exactly
// what the framework sections render. Every string in it has passed its validator; addresses
// and prefixes are net/netip values.
type Model struct {
	// Hostname is system.hostname ("" = not rendered: FRR keeps the system hostname).
	Hostname string
	// VRFs are the non-default VRFs, sorted by name, each with its static routes.
	VRFs []VRF
	// Interfaces are the interfaces with a description, sorted by (Linux) name.
	Interfaces []Interface
	// Static are the default-VRF static routes, one entry per next hop, sorted.
	Static []Route
}

// VRF is one `vrf <name>` block.
type VRF struct {
	Name   string
	Static []Route
}

// Interface is one `interface <name>` block: the description (framework) and the lines registered producers add
// (RegisterInterfaceLines, seam S2).
type Interface struct {
	Name        string
	Description string
	// Lines are complete indented commands from the registered interface-line producers, in producer-name order.
	Lines []string
}

// Route is one staticd line: `ip[v6] route <prefix> <nexthop> [tag T] [distance]`.
type Route struct {
	// AFI is "ip" or "ipv6" (the command keyword).
	AFI    string
	Prefix netip.Prefix
	// Gateway is the next-hop address (invalid = none).
	Gateway netip.Addr
	// Interface is the Linux egress interface ("" = none).
	Interface string
	// Blackhole drops the traffic (`blackhole`); Gateway and Interface are then unset.
	Blackhole bool
	// Tag is the route tag (0 = none).
	Tag uint32
	// Distance is the administrative distance (1 = default, not rendered).
	Distance uint32
}

// PrefixText is the canonical prefix for the template.
func (r Route) PrefixText() string { return r.Prefix.String() }

// ShowDistance reports whether the distance differs from FRR's default (1) and is rendered.
func (r Route) ShowDistance() bool { return r.Distance > 1 }

// GatewayText is the canonical gateway address ("" when unset).
func (r Route) GatewayText() string {
	if !r.Gateway.IsValid() {
		return ""
	}
	return r.Gateway.String()
}

// Extensions carries framework fields that the desired-state proto does not have yet (D-055
// stand-in, docs/status/tasks/RF-1-questions.md Q1). They are read from a *structpb.Struct
// input at the same JSON paths the schema will use:
//
//	routing.static[i].tag  uint32 route tag
//	routing.static[i].frr  bool   D-072 flag: FRR (staticd) programs this route, the agent does not
type Extensions struct {
	// Tag maps a routing.static index to its tag.
	Tag map[int]uint32
	// FRR holds the routing.static indexes flagged for FRR (D-072).
	FRR map[int]bool
}

// ---------------------------------------------------------------- D-072: one programmer per static route

// StaticSelector decides whether routing.static[i] is programmed by FRR (true) or by the agent
// directly in VPP (false, the default). D-072: never both.
type StaticSelector func(i int, sr *vrxv1.StaticRoute, ext *Extensions) bool

// FlaggedStatic is the default selector: only routes carrying the explicit flag go to FRR
// (today the stand-in `routing.static[i].frr: true`; when the schema/proto field lands, the
// owner of that field registers a selector reading it).
func FlaggedStatic(i int, _ *vrxv1.StaticRoute, ext *Extensions) bool {
	return ext != nil && ext.FRR[i]
}

var (
	selectorMu     sync.Mutex
	staticSelector StaticSelector = FlaggedStatic
	selectorSet    bool
)

// RegisterStaticSelector replaces FlaggedStatic once (P03b/P12 when the real flag exists).
// It panics on nil or a second registration (init()-time programming errors).
func RegisterStaticSelector(fn StaticSelector) {
	if fn == nil {
		panic("frr: RegisterStaticSelector(nil)")
	}
	selectorMu.Lock()
	defer selectorMu.Unlock()
	if selectorSet {
		panic("frr: static selector registered twice")
	}
	staticSelector, selectorSet = fn, true
}

// StaticOwnedByFRR reports whether routing.static[i] belongs to FRR. The FRR renderer renders
// exactly these routes; the P05 static-route descriptor must skip exactly these (D-072). Both
// call this one function so the two programmers can never disagree.
func StaticOwnedByFRR(i int, sr *vrxv1.StaticRoute, ext *Extensions) bool {
	selectorMu.Lock()
	fn := staticSelector
	selectorMu.Unlock()
	return fn(i, sr, ext)
}

// InterfaceMapper maps a VPP interface name from the document to the Linux interface name
// FRR sees (the linux-cp pair; P12 supplies the real mapping). ok=false means the interface
// has no Linux side.
type InterfaceMapper func(vppName string) (linuxName string, ok bool)

// NoMapper is the product default until P12 injects the linux-cp mapping: no VPP interface
// has a known Linux side, so no interface block is rendered and an FRR static route with a
// next-hop interface is an error (RF-1 review L4).
func NoMapper(string) (string, bool) { return "", false }

// IdentityMapper maps every name that is already a valid Linux interface name to itself and
// reports ok=false for the rest. Tests and the frrtest harness use it (their interfaces are
// Linux devices in the test namespace).
func IdentityMapper(name string) (string, bool) {
	if _, err := IfName(name); err != nil {
		return "", false
	}
	return name, true
}

// Desired normalises the renderer input: a *vrxv1.DesiredState is used as is; a
// *structpb.Struct holding the JSON configuration document is decoded into a DesiredState
// (unknown fields ignored) and its stand-in fields returned as Extensions; nil is the empty
// state. Protocol sections may call it to get the typed state from whatever Render received.
func Desired(msg proto.Message) (*vrxv1.DesiredState, *Extensions, error) {
	ext := &Extensions{Tag: map[int]uint32{}, FRR: map[int]bool{}}
	switch m := msg.(type) {
	case nil:
		return &vrxv1.DesiredState{}, ext, nil
	case *vrxv1.DesiredState:
		if m == nil {
			return &vrxv1.DesiredState{}, ext, nil
		}
		return m, ext, nil
	case *structpb.Struct:
		if m == nil {
			return &vrxv1.DesiredState{}, ext, nil
		}
		raw, err := protojson.Marshal(m)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: encode document: %v", ErrInput, err)
		}
		ds := &vrxv1.DesiredState{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, ds); err != nil {
			return nil, nil, fmt.Errorf("%w: decode document: %v", ErrInput, err)
		}
		if err := readExtensions(m, ext); err != nil {
			return nil, nil, err
		}
		return ds, ext, nil
	default:
		return nil, nil, fmt.Errorf("%w: unsupported input type %T (want *vrxv1.DesiredState or *structpb.Struct)", ErrInput, msg)
	}
}

func readExtensions(doc *structpb.Struct, ext *Extensions) error {
	routing := doc.GetFields()["routing"].GetStructValue()
	statics := routing.GetFields()["static"].GetListValue().GetValues()
	for i, v := range statics {
		route := v.GetStructValue()
		if tag, ok := route.GetFields()["tag"]; ok {
			n, isNum := tag.GetKind().(*structpb.Value_NumberValue)
			if !isNum || n.NumberValue < 0 || n.NumberValue > 4294967295 || n.NumberValue != float64(uint32(n.NumberValue)) {
				return fmt.Errorf("%w: routing.static[%d].tag must be an integer 0..4294967295", ErrInput, i)
			}
			ext.Tag[i] = uint32(n.NumberValue)
		}
		if f, ok := route.GetFields()["frr"]; ok {
			b, isBool := f.GetKind().(*structpb.Value_BoolValue)
			if !isBool {
				return fmt.Errorf("%w: routing.static[%d].frr must be a boolean", ErrInput, i)
			}
			if b.BoolValue {
				ext.FRR[i] = true
			}
		}
	}
	return nil
}

// BuildModel validates the framework part of ds (+ stand-in extensions) and returns the
// model the framework sections render. Every error wraps ErrInput and names the JSON path.
func BuildModel(ds *vrxv1.DesiredState, ext *Extensions, mapIf InterfaceMapper) (*Model, error) {
	if ds == nil {
		ds = &vrxv1.DesiredState{}
	}
	if ext == nil {
		ext = &Extensions{}
	}
	if mapIf == nil {
		mapIf = NoMapper
	}
	m := &Model{}
	if h := ds.GetSystem().GetHostname(); h != "" {
		v, err := Hostname(h)
		if err != nil {
			return nil, inputErr("system.hostname", err)
		}
		m.Hostname = v
	}

	vrfs := map[string]*VRF{}
	addVRF := func(name, path string) (*VRF, error) {
		if _, err := VRFName(name); err != nil {
			return nil, inputErr(path, err)
		}
		if v, ok := vrfs[name]; ok {
			return v, nil
		}
		v := &VRF{Name: name}
		vrfs[name] = v
		return v, nil
	}
	for name := range ds.GetVrfs() {
		if name == DefaultVRF {
			continue
		}
		if _, err := addVRF(name, "vrfs."+jsonKey(name)); err != nil {
			return nil, err
		}
	}

	for vppName, itf := range ds.GetInterfaces() {
		if itf.Description == nil {
			continue
		}
		linux, ok := mapIf(vppName)
		if !ok {
			continue // no Linux side: nothing for FRR to describe
		}
		path := "interfaces." + jsonKey(vppName)
		name, err := IfName(linux)
		if err != nil {
			return nil, inputErr(path, err)
		}
		desc, err := Description(itf.GetDescription())
		if err != nil {
			return nil, inputErr(path+".description", err)
		}
		m.Interfaces = append(m.Interfaces, Interface{Name: name, Description: desc})
	}
	slices.SortFunc(m.Interfaces, func(a, b Interface) int { return cmp.Compare(a.Name, b.Name) })
	for i := 1; i < len(m.Interfaces); i++ {
		if m.Interfaces[i].Name == m.Interfaces[i-1].Name {
			return nil, fmt.Errorf("%w: interfaces: two interfaces map to Linux name %q", ErrInput, m.Interfaces[i].Name)
		}
	}

	for i, sr := range ds.GetRouting().GetStatic() {
		if !StaticOwnedByFRR(i, sr, ext) {
			continue // D-072: programmed in VPP by the agent, never by FRR as well
		}
		path := fmt.Sprintf("routing.static[%d]", i)
		routes, err := buildRoutes(sr, i, path, ext, mapIf)
		if err != nil {
			return nil, err
		}
		vrf := sr.GetVrf()
		if vrf == "" || vrf == DefaultVRF {
			m.Static = append(m.Static, routes...)
			continue
		}
		v, err := addVRF(vrf, path+".vrf")
		if err != nil {
			return nil, err
		}
		v.Static = append(v.Static, routes...)
	}
	sortRoutes(m.Static)
	for _, v := range vrfs {
		sortRoutes(v.Static)
		m.VRFs = append(m.VRFs, *v)
	}
	slices.SortFunc(m.VRFs, func(a, b VRF) int { return cmp.Compare(a.Name, b.Name) })
	return m, nil
}

func buildRoutes(sr *vrxv1.StaticRoute, idx int, path string, ext *Extensions, mapIf InterfaceMapper) ([]Route, error) {
	pfx, err := netip.ParsePrefix(sr.GetPrefix())
	if err != nil {
		return nil, inputErr(path+".prefix", fmt.Errorf("%w: prefix %q: %v", renderers.ErrUnsafe, sr.GetPrefix(), err))
	}
	pfx = pfx.Masked()
	afi := "ip"
	if pfx.Addr().Is6() {
		afi = "ipv6"
	}
	distance := sr.GetDistance()
	switch {
	case distance == 0:
		distance = 1
	case distance > 255:
		return nil, fmt.Errorf("%w: %s.distance %d not in 1..255", ErrInput, path, distance)
	}
	tag := sr.GetTag() // StaticRoute.tag (8, P12); the D-055 stand-in only for documents without it
	if tag == 0 {
		tag = ext.Tag[idx]
	}
	base := Route{AFI: afi, Prefix: pfx, Tag: tag, Distance: distance}
	if sr.GetBlackhole() {
		if len(sr.GetNextHops()) != 0 {
			return nil, fmt.Errorf("%w: %s: a blackhole route has no next hops", ErrInput, path)
		}
		base.Blackhole = true
		return []Route{base}, nil
	}
	if len(sr.GetNextHops()) == 0 {
		return nil, fmt.Errorf("%w: %s.nextHops is empty (and blackhole is not set)", ErrInput, path)
	}
	out := make([]Route, 0, len(sr.GetNextHops()))
	for j, nh := range sr.GetNextHops() {
		hpath := fmt.Sprintf("%s.nextHops[%d]", path, j)
		r := base
		if nh.Address == nil && nh.Interface == nil {
			return nil, fmt.Errorf("%w: %s needs an address or an interface", ErrInput, hpath)
		}
		if nh.Address != nil {
			a, err := gateway(nh.GetAddress(), pfx, nh.Interface != nil)
			if err != nil {
				return nil, inputErr(hpath+".address", err)
			}
			r.Gateway = a
		}
		if nh.Interface != nil {
			linux, ok := mapIf(nh.GetInterface())
			if !ok {
				return nil, fmt.Errorf("%w: %s.interface %q has no Linux interface for FRR", ErrInput, hpath, nh.GetInterface())
			}
			name, err := RouteIfName(linux)
			if err != nil {
				return nil, inputErr(hpath+".interface", err)
			}
			r.Interface = name
		}
		out = append(out, r)
	}
	return out, nil
}

// gateway validates a next-hop address (RF-1 review M1): a plain unicast address of the
// prefix's family; not unspecified, multicast or loopback; link-local only with an interface.
func gateway(s string, pfx netip.Prefix, hasIf bool) (netip.Addr, error) {
	a, err := netip.ParseAddr(s)
	switch {
	case err != nil || a.Zone() != "":
		return netip.Addr{}, fmt.Errorf("%w: address %q is not a plain IP address", renderers.ErrUnsafe, s)
	case a.Is4() != pfx.Addr().Is4():
		return netip.Addr{}, fmt.Errorf("%w: address %s is not in the address family of %s", renderers.ErrUnsafe, a, pfx)
	case a.IsUnspecified(), a.IsMulticast(), a.IsLoopback():
		return netip.Addr{}, fmt.Errorf("%w: address %s is unspecified, multicast or loopback", renderers.ErrUnsafe, a)
	case a.IsLinkLocalUnicast() && !hasIf:
		return netip.Addr{}, fmt.Errorf("%w: link-local gateway %s needs an interface", renderers.ErrUnsafe, a)
	}
	return a, nil
}

func sortRoutes(rs []Route) {
	slices.SortStableFunc(rs, func(a, b Route) int {
		return cmp.Or(
			cmp.Compare(a.AFI, b.AFI),
			a.Prefix.Addr().Compare(b.Prefix.Addr()),
			cmp.Compare(a.Prefix.Bits(), b.Prefix.Bits()),
			boolCmp(a.Blackhole, b.Blackhole),
			a.Gateway.Compare(b.Gateway),
			cmp.Compare(a.Interface, b.Interface),
		)
	})
}

func boolCmp(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	default:
		return -1
	}
}

func inputErr(path string, err error) error {
	return fmt.Errorf("%w: %s: %w", ErrInput, path, err)
}

// jsonKey quotes a record key for an error path when it is not a plain token.
func jsonKey(k string) string {
	if k != "" && !strings.ContainsAny(k, ".[]\"' \t\r\n") {
		return k
	}
	return fmt.Sprintf("%q", k)
}
