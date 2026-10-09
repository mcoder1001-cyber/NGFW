package desired

// P12 builder and assembler of the FRR stage (wave-A-hotspots A2: projection.go calls them under its anchors).
//
//	routing.bgp, routing.policy, routing.static[i] with viaFrr (D-072),
//	+ interfaces.<n>{lcp, ipv4, ipv6, description} of every paired interface   → frr.config/ngfw (one singleton object)
//
// The object's value is the FRR-relevant subset of the document (FRRDoc) — secret *references* only, never values —
// wrapped with a status: the desired object says "applied"; the descriptor's Retrieve reports "applied" when FRR's running
// configuration is the rendering of that document (frr-reload.py --test shows no diff), "drift" / "unreachable" /
// "unknown" otherwise, so the reconciler re-applies (Update) or removes (Delete) it. The object exists only when the
// document has FRR content, so an agent without routing protocols never talks to FRR (P12-questions Q3).

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/lcpmap"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/policy"
	"ngfw/agent/internal/scheduler"
)

// FRR stage object.
const (
	// FRRConfigName is the descriptor of the FRR stage (the renderer as one scheduler object, D-109 d).
	FRRConfigName = "frr.config"
	// FRRConfigID is its only id.
	FRRConfigID = "ngfw"
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

// FRRDoc returns the FRR-relevant subset of ds, or nil when ds has no FRR content: no bgp, no ospf/isis/rip, no policy object, no viaFrr
// static route and no interface with a linux-cp pair. A paired interface alone is FRR content: FRR puts the VPP
// addresses on its Linux side (lcpmap, S2) for as long as the pair exists — were they removed with the last BGP line,
// linux-nl would mirror the removal into VPP and take the addresses off the VPP interface as well.
// selector reports whether routing.static[i] belongs to FRR (frr.StaticOwnedByFRR, D-072).
func FRRDoc(ds *ngfwv1.DesiredState, selector func(i int, sr *ngfwv1.StaticRoute) bool) *ngfwv1.DesiredState {
	rt := ds.GetRouting()
	out := &ngfwv1.RoutingConfig{}
	content := false
	// wave-BC: F-bfd-redistribution
	if profiles := rt.GetBfd().GetProfiles(); len(profiles) > 0 {
		out.Bfd = &ngfwv1.BfdConfig{Profiles: profiles}
		content = true
	}
	if pim := rt.GetMulticast().GetPim(); pim != nil {
		out.Multicast = &ngfwv1.MulticastConfig{Pim: proto.Clone(pim).(*ngfwv1.PimConfig)}
		content = true
	}
	if rt.GetBgp() != nil {
		out.Bgp = proto.Clone(rt.GetBgp()).(*ngfwv1.BgpConfig)
		content = true
	}
	if rt.GetOspf6() != nil {
		out.Ospf6 = proto.Clone(rt.GetOspf6()).(*ngfwv1.Ospf6Config)
		content = true
	}
	if rt.GetOspf() != nil { // F-ospf
		out.Ospf = proto.Clone(rt.GetOspf()).(*ngfwv1.OspfConfig)
		content = true
	}
	if rt.GetIsis() != nil { // F-isis-rip
		out.Isis = proto.Clone(rt.GetIsis()).(*ngfwv1.IsisConfig)
		content = true
	}
	if rt.GetRipng() != nil {
		out.Ripng = proto.Clone(rt.GetRipng()).(*ngfwv1.RipngConfig)
		content = true
	}
	if rt.GetRip() != nil { // F-isis-rip
		out.Rip = proto.Clone(rt.GetRip()).(*ngfwv1.RipConfig)
		content = true
	}
	// wave-BC: F-mpls-ldp
	if cfg := rt.GetMpls().GetLdp(); cfg != nil {
		out.Mpls = &ngfwv1.MplsConfig{Ldp: proto.Clone(cfg).(*ngfwv1.MplsLdp)}
		content = true
	}
	if pol := rt.GetPolicy(); len(pol.GetPrefixLists()) > 0 || len(pol.GetRouteMaps()) > 0 {
		out.Policy = proto.Clone(pol).(*ngfwv1.RoutingPolicy)
		content = true
	}
	for i, sr := range rt.GetStatic() {
		if selector != nil && selector(i, sr) {
			out.Static = append(out.Static, proto.Clone(sr).(*ngfwv1.StaticRoute))
			content = true
		}
	}
	doc := &ngfwv1.DesiredState{Routing: out}
	for name, itf := range ds.GetInterfaces() {
		if itf.Lcp == nil {
			continue
		}
		content = true
		if doc.Interfaces == nil {
			doc.Interfaces = map[string]*ngfwv1.Interface{}
		}
		// no description (review M2): FRR does not need it, and the interface description is free text (Persian, '|', up
		// to 255 characters) that FRR's LINE token cannot carry
		doc.Interfaces[name] = &ngfwv1.Interface{
			Lcp:  proto.Clone(itf.GetLcp()).(*ngfwv1.InterfaceLcp),
			Ipv4: append([]string(nil), itf.GetIpv4()...),
			Ipv6: append([]string(nil), itf.GetIpv6()...),
		}
	}
	if !content {
		return nil
	}
	return doc
}

// FRRValue wraps doc with a status into the frr.config value.
func FRRValue(doc *ngfwv1.DesiredState, status string) *structpb.Struct {
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
func ParseFRRValue(v proto.Message) (*ngfwv1.DesiredState, string, error) {
	s, ok := v.(*structpb.Struct)
	if !ok || s == nil {
		return nil, "", fmt.Errorf("%w: %T", ErrFRRValue, v)
	}
	status := s.GetFields()["status"].GetStringValue()
	raw, err := protojson.Marshal(s.GetFields()["doc"].GetStructValue())
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrFRRValue, err)
	}
	doc := &ngfwv1.DesiredState{}
	if err := protojson.Unmarshal(raw, doc); err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrFRRValue, err)
	}
	return doc, status, nil
}

// FRROptions are the agent-side facts the FRR builder needs.
type FRROptions struct {
	// SecretRef fingerprints only document-referenced passwords in the selected sealed
	// snapshot. Bindings stay internal to frr.config; historical resolution is required
	// when applying/rolling back this value. Nil preserves legacy unbound values.
	SecretRef func(context.Context, string) (string, error)
	// Selector is the D-072 static-route selector (frr.StaticOwnedByFRR).
	Selector func(i int, sr *ngfwv1.StaticRoute) bool
	// Secrets reports readiness of both selected fingerprints and sealed historical resolution.
	Secrets bool
	// Check renders the document without applying it (the FRR renderer's pure Render); nil = no check.
	Check func(doc *ngfwv1.DesiredState) error
	// Disabled: this agent drives no FRR; FRR content is reported as agent.unsupported-field and not projected.
	Disabled bool
}

// FRR projects the FRR stage object for a transaction that includes `routing`.
func FRR(s Sink, ds *ngfwv1.DesiredState, in map[string]bool, o FRROptions) {
	if !in["routing"] {
		return
	}
	doc := FRRDoc(ds, o.Selector)
	if doc == nil {
		return
	}
	if o.Disabled {
		if rt := doc.GetRouting(); rt.GetBgp() != nil || rt.GetPolicy() != nil {
			s.Warnf(Ptr("routing"), "agent.unsupported-field", "this agent drives no FRR (NGFW_FRR / NGFW_FRR_PATHSPACE): routing.bgp and routing.policy are not applied")
		}
		// the D-072 notice of each FRR-owned static route (the single reporter: with FRR, the frr.config stage renders them)
		for i, sr := range ds.GetRouting().GetStatic() {
			if o.Selector != nil && o.Selector(i, sr) {
				s.Warnf(Ptr("routing", "static", strconv.Itoa(i)), "agent.unsupported-field",
					"routing.static[%d] (%s) is programmed by FRR (viaFrr, D-072), not by the agent; this agent drives no FRR (NGFW_FRR / NGFW_FRR_PATHSPACE), so it is not applied", i, sr.GetPrefix())
			}
		}
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
				"BGP MD5 password %s requires a ready selected sealed credential channel", refs[p])
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
			s.Errorf(renderPointer(err), "routing.bgp-render", "FRR configuration: %v", err)
			return
		}
	}
	var bindings map[string]string
	if refs := FRRReferencedSecrets(doc); len(refs) > 0 && o.SecretRef != nil {
		bindings = make(map[string]string, len(refs))
		for _, ref := range refs {
			fingerprint, err := o.SecretRef(context.Background(), ref)
			if err != nil {
				s.Errorf(Ptr("routing"), "routing.secret-generation-unavailable", "FRR password generation is unavailable")
				return
			}
			bindings[ref] = fingerprint
		}
	}
	value, err := FRRValueWithSecretBindings(doc, FRRApplied, bindings)
	if err != nil {
		s.Errorf(Ptr("routing"), "routing.secret-generation-invalid", "FRR password generation is invalid")
		return
	}
	s.Add(FRRConfigKey, value, Ptr("routing"))
}

// AssembleFRR adds what the retrieved frr.config object reports to ds: routing.bgp, routing.policy and the viaFrr
// static routes (sorted into routing.static by VRF name, then prefix — the order assemble uses). Only an object FRR
// runs as applied is reported: drift or an unreachable FRR leaves the leaves unset (contract §5).
func AssembleFRR(ds *ngfwv1.DesiredState, kvs []scheduler.KV) {
	for _, kv := range kvs {
		if kv.Key != FRRConfigKey {
			continue
		}
		doc, status, err := ParseFRRValue(kv.Value)
		if err != nil || status != FRRApplied {
			return
		}
		if ds.Routing == nil {
			ds.Routing = &ngfwv1.RoutingConfig{}
		}
		rt := doc.GetRouting()
		ds.Routing.Bgp = rt.GetBgp()
		ds.Routing.Policy = rt.GetPolicy()
		ds.Routing.Ospf = rt.GetOspf() // F-ospf
		ds.Routing.Ospf6 = rt.GetOspf6()
		ds.Routing.Isis = rt.GetIsis() // F-isis-rip
		ds.Routing.Ripng = rt.GetRipng()
		ds.Routing.Rip = rt.GetRip() // F-isis-rip
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

// staticPathRe is the framework's path of a static route error ("routing.static[3].nextHops[0].interface").
var staticPathRe = regexp.MustCompile(`^routing\.static\[(\d+)\](?:\.nextHops\[(\d+)\])?`)

// renderPointer is the JSON pointer of a render error (review M2): the field of a policy.FieldError (bgp, policy), the
// static route of a framework error, else /routing.
func renderPointer(err error) string {
	var fe *policy.FieldError
	if errors.As(err, &fe) {
		return fe.Path.Pointer()
	}
	msg := strings.TrimPrefix(err.Error(), frr.ErrInput.Error()+": ")
	if m := staticPathRe.FindStringSubmatch(msg); m != nil {
		if m[2] != "" {
			return Ptr("routing", "static", m[1], "nextHops", m[2])
		}
		return Ptr("routing", "static", m[1])
	}
	return Ptr("routing")
}
