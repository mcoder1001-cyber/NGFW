package desired

// F-det44-map-dslite-cnat: `nat.pnat` (NatConfig 27) onto DF-3's pnat descriptors (docs/agent/descriptors/pnat.md).
//
//	nat.pnat.bindings[i]     → pnat.binding/<proto>/<src>/<sport>/<dst>/<dport> (the match tuple is VPP's only
//	                            stable identity; the name is a configuration-only label)
//	nat.pnat.attachments[i]  → pnat.attachment/<interface>/<point>/<binding id>
//
// IPv4 only. V11: the descriptors keep the lazy-init guards (no lookup/detach while no pnat interface exists).
// VPP keeps no binding names: the assembler names bindings "pnat-<n>" in match-tuple order, then the Service's
// Retrieve applies RelabelPnat with the last applied document (names and order matched by match tuple, no false drift).

import (
	"net/netip"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/pnat"
	"ngfw/agent/internal/scheduler"
)

// RulePnat is the rule id of the agent-side PNAT checks (the schema refines the same conditions inside nat.pnat).
const RulePnat = "nat.det44-map-dslite-cnat-pnat"

func pnatIP4(s Sink, v *string, pointer string) (string, bool) {
	if v == nil {
		return "", true
	}
	a, err := netip.ParseAddr(*v)
	if err != nil || !a.Is4() {
		s.Errorf(pointer, RulePnat, "%q is not an IPv4 address (pnat is IPv4-only)", *v)
		return "", false
	}
	return a.String(), true
}

func pnatPort(s Sink, v *uint32, pointer string) (uint32, bool) {
	if v == nil {
		return 0, true
	}
	if *v == 0 || *v > 65535 {
		s.Errorf(pointer, RulePnat, "port %d is not 1-65535", *v)
		return 0, false
	}
	return *v, true
}

func pnatBinding(s Sink, b *ngfwv1.PnatBinding, pt string) (pnat.BindingSpec, bool) {
	var spec pnat.BindingSpec
	m, r := b.GetMatch(), b.GetRewrite()
	ok := true
	check := func(v bool) {
		ok = ok && v
	}
	var o bool
	spec.Match.Src, o = pnatIP4(s, m.Src, pt+"/match/src")
	check(o)
	spec.Match.Dst, o = pnatIP4(s, m.Dst, pt+"/match/dst")
	check(o)
	spec.Match.SrcPort, o = pnatPort(s, m.Sport, pt+"/match/sport")
	check(o)
	spec.Match.DstPort, o = pnatPort(s, m.Dport, pt+"/match/dport")
	check(o)
	spec.Rewrite.Src, o = pnatIP4(s, r.Src, pt+"/rewrite/src")
	check(o)
	spec.Rewrite.Dst, o = pnatIP4(s, r.Dst, pt+"/rewrite/dst")
	check(o)
	spec.Rewrite.SrcPort, o = pnatPort(s, r.Sport, pt+"/rewrite/sport")
	check(o)
	spec.Rewrite.DstPort, o = pnatPort(s, r.Dport, pt+"/rewrite/dport")
	check(o)
	if !ok {
		return spec, false
	}
	spec.Match.Proto = m.GetProto()
	switch spec.Match.Proto {
	case "", "tcp", "udp", "icmp":
	default:
		s.Errorf(pt+"/match/proto", RulePnat, "protocol %q is not tcp, udp or icmp", spec.Match.Proto)
		return spec, false
	}
	l4 := spec.Match.Proto == "tcp" || spec.Match.Proto == "udp"
	switch {
	case spec.Match == (pnat.MatchSpec{}):
		s.Errorf(pt+"/match", RulePnat, "a match needs at least one field")
		return spec, false
	case spec.Rewrite == (pnat.RewriteSpec{}):
		s.Errorf(pt+"/rewrite", RulePnat, "a rewrite needs at least one field")
		return spec, false
	case !l4 && (spec.Match.SrcPort != 0 || spec.Match.DstPort != 0 || spec.Rewrite.SrcPort != 0 || spec.Rewrite.DstPort != 0):
		s.Errorf(pt+"/match/proto", RulePnat, "ports need a match on protocol tcp or udp")
		return spec, false
	}
	spec.Normalize()
	return spec, true
}

func pnatBuild(s Sink, p *ngfwv1.PnatConfig) {
	if p == nil {
		return
	}
	byName := map[string]string{}
	masks := map[string]string{}
	ids := map[string]bool{}
	for i, b := range p.GetBindings() {
		pt := Ptr("nat", "pnat", "bindings", strconv.Itoa(i))
		spec, ok := pnatBinding(s, b, pt)
		if !ok {
			continue
		}
		id := pnat.BindingID(spec)
		if ids[id] {
			s.Errorf(pt+"/match", RulePnat, "another binding has the same match %s (the match is the binding's identity)", id)
			continue
		}
		ids[id] = true
		if b.Name != nil {
			byName[b.GetName()] = id
		}
		masks[id] = pnatMask(spec.Match)
		natAdd(s, pnat.NameBinding, id, &spec, pt)
	}
	pointMask := map[string]string{}
	seen := map[string]bool{}
	for i, a := range p.GetAttachments() {
		pt := Ptr("nat", "pnat", "attachments", strconv.Itoa(i))
		id, ok := byName[a.GetBinding()]
		if !ok {
			s.Errorf(pt+"/binding", RulePnat, "binding %q does not exist (or is invalid)", a.GetBinding())
			continue
		}
		point := a.GetPoint()
		if point != pnat.PointInput && point != pnat.PointOutput {
			s.Errorf(pt+"/point", RulePnat, "point %q is not input or output", point)
			continue
		}
		at := a.GetInterface() + "/" + point
		if prev, ok := pointMask[at]; ok && prev != masks[id] {
			s.Errorf(pt+"/binding", RulePnat, "bindings on %s %s must match the same fields (VPP rejects mixed masks)", a.GetInterface(), point)
			continue
		}
		pointMask[at] = masks[id]
		spec := pnat.AttachmentSpec{Interface: a.GetInterface(), Point: point, Binding: id}
		key := spec.Interface + "/" + spec.Point + "/" + spec.Binding
		if seen[key] {
			s.Errorf(pt, RulePnat, "duplicate attachment")
			continue
		}
		seen[key] = true
		natAdd(s, pnat.NameAttachment, key, &spec, pt)
	}
}

func pnatMask(m pnat.MatchSpec) string {
	b := []byte("-----")
	for i, set := range []bool{m.Proto != "", m.Src != "", m.SrcPort != 0, m.Dst != "", m.DstPort != 0} {
		if set {
			b[i] = 'x'
		}
	}
	return string(b)
}

func optStr(v string) *string {
	if v == "" {
		return nil
	}
	return proto.String(v)
}

func optPort(v uint32) *uint32 {
	if v == 0 {
		return nil
	}
	return proto.Uint32(v)
}

// assemblePnat rebuilds nat.pnat from the retrieved bindings (Extra = 0 only) and attachments.
func assemblePnat(out *ngfwv1.NatConfig, kvs []scheduler.KV) {
	var (
		bs  []pnat.BindingSpec
		ats []pnat.AttachmentSpec
	)
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case pnat.NameBinding:
			if v, err := natcommon.Decode[pnat.BindingSpec](kv.Value); err == nil && v.Extra == 0 {
				bs = append(bs, v)
			}
		case pnat.NameAttachment:
			if v, err := natcommon.Decode[pnat.AttachmentSpec](kv.Value); err == nil {
				ats = append(ats, v)
			}
		}
	}
	if len(bs) == 0 && len(ats) == 0 {
		return
	}
	sort.Slice(bs, func(a, b int) bool { return pnat.BindingID(bs[a]) < pnat.BindingID(bs[b]) })
	p := &ngfwv1.PnatConfig{}
	names := map[string]string{}
	for i, b := range bs {
		name := "pnat-" + strconv.Itoa(i+1)
		names[pnat.BindingID(b)] = name
		pb := &ngfwv1.PnatBinding{
			Name: proto.String(name),
			Match: &ngfwv1.PnatMatch{Proto: optStr(b.Match.Proto), Src: optStr(b.Match.Src), Sport: optPort(b.Match.SrcPort),
				Dst: optStr(b.Match.Dst), Dport: optPort(b.Match.DstPort)},
			Rewrite: &ngfwv1.PnatRewrite{Src: optStr(b.Rewrite.Src), Sport: optPort(b.Rewrite.SrcPort),
				Dst: optStr(b.Rewrite.Dst), Dport: optPort(b.Rewrite.DstPort)},
		}
		p.Bindings = append(p.Bindings, pb)
	}
	sort.Slice(ats, func(a, b int) bool {
		x, y := ats[a], ats[b]
		if x.Interface != y.Interface {
			return x.Interface < y.Interface
		}
		if x.Point != y.Point {
			return x.Point < y.Point
		}
		return x.Binding < y.Binding
	})
	for _, a := range ats {
		name, ok := names[a.Binding]
		if !ok {
			name = a.Binding // binding not retrieved (foreign or gone): keep the id so the drift is visible
		}
		p.Attachments = append(p.Attachments, &ngfwv1.PnatAttachment{Binding: proto.String(name), Interface: proto.String(a.Interface), Point: proto.String(a.Point)})
	}
	out.Pnat = p
}

type discardSink struct{}

func (discardSink) Add(scheduler.Key, proto.Message, string) {}
func (discardSink) Errorf(string, string, string, ...any)    {}
func (discardSink) Warnf(string, string, string, ...any)     {}

// pnatID is the binding id (match tuple) of a document binding; ok false when it does not project.
func pnatID(b *ngfwv1.PnatBinding) (string, bool) {
	spec, ok := pnatBinding(discardSink{}, b, "")
	if !ok {
		return "", false
	}
	return pnat.BindingID(spec), true
}

// RelabelPnat gives a retrieved nat.pnat the binding names and the binding / attachment order of ref (the last
// applied document), matched by binding id (the match tuple), so Retrieve equals the applied document and drift stays
// empty. Retrieved bindings ref does not have keep their pnat-<n> names and follow the matched ones.
func RelabelPnat(got, ref *ngfwv1.PnatConfig) {
	if got == nil || ref == nil {
		return
	}
	type pos struct {
		name string
		idx  int
	}
	refB := map[string]pos{}
	for i, b := range ref.GetBindings() {
		if id, ok := pnatID(b); ok {
			if _, dup := refB[id]; !dup {
				refB[id] = pos{b.GetName(), i}
			}
		}
	}
	rename := map[string]string{} // retrieved name → document name
	gotID := map[string]string{}  // retrieved binding name → id
	rank := func(id string) int {
		if p, ok := refB[id]; ok {
			return p.idx
		}
		return len(ref.GetBindings())
	}
	for _, b := range got.GetBindings() {
		id, ok := pnatID(b)
		if !ok {
			continue
		}
		gotID[b.GetName()] = id
		if p, ok := refB[id]; ok {
			rename[b.GetName()] = p.name
		}
	}
	sort.SliceStable(got.Bindings, func(a, b int) bool {
		return rank(gotID[got.Bindings[a].GetName()]) < rank(gotID[got.Bindings[b].GetName()])
	})
	for _, b := range got.GetBindings() {
		if n, ok := rename[b.GetName()]; ok {
			b.Name = proto.String(n)
		}
	}
	refA := map[string]int{}
	refNameID := map[string]string{}
	for _, b := range ref.GetBindings() {
		if id, ok := pnatID(b); ok {
			refNameID[b.GetName()] = id
		}
	}
	for i, a := range ref.GetAttachments() {
		refA[a.GetInterface()+"/"+a.GetPoint()+"/"+refNameID[a.GetBinding()]] = i
	}
	aKey := func(a *ngfwv1.PnatAttachment) string {
		return a.GetInterface() + "/" + a.GetPoint() + "/" + gotID[a.GetBinding()]
	}
	aRank := func(a *ngfwv1.PnatAttachment) int {
		if i, ok := refA[aKey(a)]; ok {
			return i
		}
		return len(ref.GetAttachments())
	}
	sort.SliceStable(got.Attachments, func(a, b int) bool { return aRank(got.Attachments[a]) < aRank(got.Attachments[b]) })
	for _, a := range got.GetAttachments() {
		if n, ok := rename[a.GetBinding()]; ok {
			a.Binding = proto.String(n)
		}
	}
}
