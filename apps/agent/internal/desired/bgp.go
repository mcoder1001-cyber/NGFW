package desired

// P12 builder and assembler of the FRR stage (wave-A-hotspots A2: projection.go calls them under its anchors).
//
//	routing.bgp, routing.policy, routing.static[i] with viaFrr (D-072),
//	+ interfaces.<n>{lcp, ipv4, ipv6, description} of every paired interface   → frr.config/vrx (one singleton object)
//
// The object's value is the FRR-relevant subset of the document (FRRDoc) — secret *references* only, never values —
// wrapped with a status: the desired object says "applied"; the descriptor's Retrieve reports "applied" when FRR's running
// configuration is the rendering of that document (frr-reload.py --test shows no diff), "drift" / "unreachable" /
// "unknown" otherwise, so the reconciler re-applies (Update) or removes (Delete) it. The object exists only when the
// document has FRR content, so an agent without routing protocols never talks to FRR (P12-questions Q3).

import (
	"errors"
	"fmt"
	"sort"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/lcpmap"
	"ngfw/agent/internal/scheduler"
)

// FRR stage object.
const (
	// FRRConfigName is the descriptor of the FRR stage (the renderer as one scheduler object, D-109 d).
	FRRConfigName = "frr.config"
	// FRRConfigID is its only id.
	FRRConfigID = "vrx"
)

// FRRConfigKey is the key of the singleton.
var FRRConfigKey = scheduler.Join(FRRConfigName, FRRConfigID)

// Status values of the frr.config object.
const (
	FRRApplied     = "applied"     // FRR runs the rendering of doc
	FRRDrift       = "drift"       // FRR's running configuration differs from the rendering of doc
	FRRUnreachable = "unreachable" // FRR does not answer (the last applied doc is reported)
	FRRUnknown     = "unknown"     // FRR holds configuration this agent has not applied since it started
)

// FRRDoc returns the FRR-relevant subset of ds, or nil when ds has no FRR content (no bgp, no policy object, no
// viaFrr static route). Interfaces are included only with a linux-cp pair (their Linux side is what FRR sees).
// selector reports whether routing.static[i] belongs to FRR (frr.StaticOwnedByFRR, D-072).
func FRRDoc(ds *vrxv1.DesiredState, selector func(i int, sr *vrxv1.StaticRoute) bool) *vrxv1.DesiredState {
	rt := ds.GetRouting()
	out := &vrxv1.RoutingConfig{}
	content := false
	if rt.GetBgp() != nil {
		out.Bgp = proto.Clone(rt.GetBgp()).(*vrxv1.BgpConfig)
		content = true
	}
	if pol := rt.GetPolicy(); len(pol.GetPrefixLists()) > 0 || len(pol.GetRouteMaps()) > 0 {
		out.Policy = proto.Clone(pol).(*vrxv1.RoutingPolicy)
		content = true
	}
	for i, sr := range rt.GetStatic() {
		if selector != nil && selector(i, sr) {
			out.Static = append(out.Static, proto.Clone(sr).(*vrxv1.StaticRoute))
			content = true
		}
	}
	if !content {
		return nil
	}
	doc := &vrxv1.DesiredState{Routing: out}
	for name, itf := range ds.GetInterfaces() {
		if itf.Lcp == nil {
			continue
		}
		if doc.Interfaces == nil {
			doc.Interfaces = map[string]*vrxv1.Interface{}
		}
		doc.Interfaces[name] = &vrxv1.Interface{
			Lcp:         proto.Clone(itf.GetLcp()).(*vrxv1.InterfaceLcp),
			Ipv4:        append([]string(nil), itf.GetIpv4()...),
			Ipv6:        append([]string(nil), itf.GetIpv6()...),
			Description: itf.Description,
		}
	}
	return doc
}

// FRRValue wraps doc with a status into the frr.config value.
func FRRValue(doc *vrxv1.DesiredState, status string) *structpb.Struct {
	raw, err := protojson.Marshal(doc)
	if err != nil {
		panic(fmt.Sprintf("desired: encode FRR document: %v", err)) // only on a broken proto message
	}
	d := &structpb.Struct{}
	if err := protojson.Unmarshal(raw, d); err != nil {
		panic(fmt.Sprintf("desired: FRR document as struct: %v", err))
	}
	return &structpb.Struct{Fields: map[string]*structpb.Value{
		"doc":    structpb.NewStructValue(d),
		"status": structpb.NewStringValue(status),
	}}
}

// ErrFRRValue is returned for a malformed frr.config value.
var ErrFRRValue = errors.New("desired: malformed frr.config value")

// ParseFRRValue unwraps an frr.config value.
func ParseFRRValue(v proto.Message) (*vrxv1.DesiredState, string, error) {
	s, ok := v.(*structpb.Struct)
	if !ok || s == nil {
		return nil, "", fmt.Errorf("%w: %T", ErrFRRValue, v)
	}
	status := s.GetFields()["status"].GetStringValue()
	raw, err := protojson.Marshal(s.GetFields()["doc"].GetStructValue())
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrFRRValue, err)
	}
	doc := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal(raw, doc); err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrFRRValue, err)
	}
	return doc, status, nil
}

// FRRDependencies are the linux-cp pairs the FRR document names (their taps must exist before FRR configures them).
func FRRDependencies(doc *vrxv1.DesiredState) []scheduler.Dependency {
	var out []scheduler.Dependency
	for _, name := range sortedKeys(doc.GetInterfaces()) {
		out = append(out, scheduler.Dependency{Key: scheduler.Join(lcp.NameItfPair, name)})
	}
	return out
}

// FRROptions are the agent-side facts the FRR builder needs.
type FRROptions struct {
	// Selector is the D-072 static-route selector (frr.StaticOwnedByFRR).
	Selector func(i int, sr *vrxv1.StaticRoute) bool
	// Secrets reports whether the agent can resolve secret references (false until PENDING-secret-channel).
	Secrets bool
	// Check renders the document without applying it (the FRR renderer's pure Render); nil = no check.
	Check func(doc *vrxv1.DesiredState) error
	// Disabled: this agent drives no FRR; FRR content is reported as agent.unsupported-field and not projected.
	Disabled bool
}

// FRR projects the FRR stage object for a transaction that includes `routing`.
func FRR(s Sink, ds *vrxv1.DesiredState, in map[string]bool, o FRROptions) {
	if !in["routing"] {
		return
	}
	doc := FRRDoc(ds, o.Selector)
	if doc == nil {
		return
	}
	if o.Disabled {
		s.Warnf(Ptr("routing"), "agent.unsupported-field", "this agent drives no FRR (VRX_FRR / VRX_FRR_PATHSPACE): routing.bgp, routing.policy and viaFrr routes are not applied")
		return
	}
	bad := false
	if !o.Secrets {
		refs := map[string]string{}
		for name, g := range doc.GetRouting().GetBgp().GetPeerGroups() {
			if g.GetPasswordRef() != "" {
				refs[Ptr("routing", "bgp", "peerGroups", name, "passwordRef")] = g.GetPasswordRef()
			}
		}
		for addr, n := range doc.GetRouting().GetBgp().GetNeighbors() {
			if n.GetPasswordRef() != "" {
				refs[Ptr("routing", "bgp", "neighbors", addr, "passwordRef")] = n.GetPasswordRef()
			}
		}
		ptrs := make([]string, 0, len(refs))
		for p := range refs {
			ptrs = append(ptrs, p)
		}
		sort.Strings(ptrs)
		for _, p := range ptrs {
			s.Errorf(p, "routing.bgp-password-unavailable",
				"BGP MD5 password %s needs the API→agent secret channel (PENDING-secret-channel), which this build does not have yet: remove passwordRef or wait for the channel", refs[p])
			bad = true
		}
	}
	for name, itf := range doc.GetInterfaces() {
		if _, err := lcpmap.HostName(name, itf.GetLcp()); err != nil {
			s.Errorf(Ptr("interfaces", name, "lcp", "hostIfName"), "routing.bgp-lcp-host-name", "%v", err)
			bad = true
		}
	}
	if bad {
		return
	}
	if o.Check != nil {
		if err := o.Check(doc); err != nil {
			s.Errorf(Ptr("routing"), "routing.bgp-render", "FRR configuration: %v", err)
			return
		}
	}
	s.Add(FRRConfigKey, FRRValue(doc, FRRApplied), Ptr("routing"))
}

// AssembleFRR adds what the retrieved frr.config object reports to ds: routing.bgp, routing.policy and the viaFrr
// static routes (sorted into routing.static by VRF name, then prefix — the order assemble uses). Only an object FRR
// runs as applied is reported: drift or an unreachable FRR leaves the leaves unset (contract §5).
func AssembleFRR(ds *vrxv1.DesiredState, kvs []scheduler.KV) {
	for _, kv := range kvs {
		if kv.Key != FRRConfigKey {
			continue
		}
		doc, status, err := ParseFRRValue(kv.Value)
		if err != nil || status != FRRApplied {
			return
		}
		if ds.Routing == nil {
			ds.Routing = &vrxv1.RoutingConfig{}
		}
		rt := doc.GetRouting()
		ds.Routing.Bgp = rt.GetBgp()
		ds.Routing.Policy = rt.GetPolicy()
		if len(rt.GetStatic()) > 0 {
			ds.Routing.Static = append(ds.Routing.Static, rt.GetStatic()...)
			sort.SliceStable(ds.Routing.Static, func(a, b int) bool {
				x, y := ds.Routing.Static[a], ds.Routing.Static[b]
				if x.GetVrf() != y.GetVrf() {
					return x.GetVrf() < y.GetVrf()
				}
				return x.GetPrefix() < y.GetPrefix()
			})
		}
		return
	}
}
