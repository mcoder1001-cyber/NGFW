package desired

// F-mpls-srmpls builder and assembler (wave-A-hotspots A2: projection.go calls them under its anchors).
// `routing.mpls` of the configuration document onto DF-7's MPLS and DF-6's SR-MPLS descriptors:
//
//	(table 0 needed)                     → mpls-table/0          interfaces, bindings, SR policies or a table-0 label route
//	routing.mpls.tables.<id>             → mpls-table/<id>       name "<owner>:<id>"
//	routing.mpls.interfaces[i]           → mpls-interface/<if>   (depends on interface/<if> and mpls-table/0)
//	routing.mpls.labelRoutes[i]          → mpls-route/<t>/<label>/<eos|neos>
//	routing.mpls.ipBindings[i]           → mpls-ip-bind/0/<label>/<vrf id>/<prefix>   (write-only, D-063)
//	routing.mpls.tunnels.<name>          → mpls-tunnel/<name>    (provides interface/<name>)
//	routing.mpls.sr.policies.<bsid>      → sr-mpls.policy/<bsid> (write-only; depends on mpls-table/0)
//	routing.mpls.sr.steering[i]          → sr-mpls.steering/<vrf id>/<prefix>          (write-only)
//
// Dependency order (scheduler): interface → table → routes / bindings → tunnels → SR policy → steering.
//
// MPLS table 0 is VPP-global (D-071): every agent declares mpls-table/0 when its MPLS configuration
// needs it; the descriptor's role decides what that means — the globals owner creates it (named
// "<owner>:0"), any other agent only requires it (mpls.NewTableFor). Retrieve never lists table 0
// under `tables` (it is implicit), and never reports ipBindings or sr (no dump in VPP 26.06).

import (
	"net/netip"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/mpls"
	srmpls "ngfw/agent/internal/descriptors/sr_mpls"
	"ngfw/agent/internal/scheduler"
)

// MplsNeedsTableZero reports whether m needs the default MPLS table 0 in VPP: MPLS-enabled
// interfaces, label bindings, SR-MPLS policies (their BSIDs live there) or a label route of table 0.
func MplsNeedsTableZero(m *vrxv1.MplsConfig) bool {
	if len(m.GetInterfaces()) > 0 || len(m.GetIpBindings()) > 0 || len(m.GetSr().GetPolicies()) > 0 {
		return true
	}
	for _, r := range m.GetLabelRoutes() {
		if r.GetTable() == 0 {
			return true
		}
	}
	return false
}

// SteeringKey is the key of the SR-MPLS steering entry of prefix (canonical) in IP table table.
func SteeringKey(table uint32, prefix string) scheduler.Key {
	return scheduler.Join(srmpls.SteeringName, df6.U32(table)+"/"+prefix)
}

// MplsPayload is the EOS payload of a label route: payload when set, else ip6 when a path has an
// IPv6 next hop, else ip4 (the schema's documented default; the assembler omits it when equal).
func MplsPayload(payload string, paths []*vrxv1.MplsPath) string {
	if payload != "" {
		return payload
	}
	for _, p := range paths {
		if a, err := netip.ParseAddr(p.GetNextHop()); err == nil && a.Is6() && !a.Is4In6() {
			return mpls.PayloadIP6
		}
	}
	return mpls.PayloadIP4
}

// MplsSrmpls projects `routing.mpls` of ds when the routing domain is in the transaction. vrfID
// resolves a VRF name to its table id.
func MplsSrmpls(s Sink, ds *vrxv1.DesiredState, in map[string]bool, vrfID func(string) (uint32, bool)) {
	if !in["routing"] {
		return
	}
	m := ds.GetRouting().GetMpls()
	if m == nil {
		return
	}
	pt := func(segs ...string) string { return Ptr(append([]string{"routing", "mpls"}, segs...)...) }
	if MplsNeedsTableZero(m) {
		s.Add(mpls.KeyTable(0), df7.Encode(mpls.Table{ID: 0}), pt())
	}
	for _, key := range sortedKeys(m.GetTables()) {
		id, err := strconv.ParseUint(key, 10, 32)
		if err != nil || id == 0 {
			s.Errorf(pt("tables", key), "routing.mpls-srmpls-table-id", "MPLS table %q: an id 1–4294967295 (table 0 is the default table)", key)
			continue
		}
		s.Add(mpls.KeyTable(uint32(id)), df7.Encode(mpls.Table{ID: uint32(id)}), pt("tables", key))
	}
	for i, name := range m.GetInterfaces() {
		s.Add(mpls.KeyInterface(name), df7.Encode(mpls.Interface{Interface: name}), pt("interfaces", strconv.Itoa(i)))
	}
	for i, lr := range m.GetLabelRoutes() {
		at := pt("labelRoutes", strconv.Itoa(i))
		eos := lr.Eos == nil || lr.GetEos() // Zod default true
		r := mpls.Route{Table: lr.GetTable(), Label: lr.GetLabel(), EOS: eos}
		payload := ""
		if eos {
			payload = MplsPayload(lr.GetPayload(), lr.GetPaths())
			r.EOSProto = payload
		} else if lr.GetPayload() != "" {
			s.Errorf(at+"/payload", "routing.mpls-srmpls-payload", "payload applies to end-of-stack routes only")
			continue
		}
		paths, ok := mplsPaths(s, lr.GetPaths(), payload, vrfID, at)
		if !ok {
			continue
		}
		r.Paths = paths
		if err := r.Validate(); err != nil {
			s.Errorf(at, "routing.mpls-srmpls-label-route", "%v", err)
			continue
		}
		s.Add(mpls.KeyRoute(r.Table, r.Label, r.EOS), df7.Encode(r), at+"/label")
	}
	for i, b := range m.GetIpBindings() {
		at := pt("ipBindings", strconv.Itoa(i))
		vrf, ok := vrfID(vrfOr(b.Vrf))
		if !ok {
			s.Errorf(at+"/vrf", "routing.mpls-srmpls-vrf-exists", "VRF %q does not exist", vrfOr(b.Vrf))
			continue
		}
		p, err := netip.ParsePrefix(b.GetPrefix())
		if err != nil {
			s.Errorf(at+"/prefix", "routing.mpls-srmpls-prefix", "%v", err)
			continue
		}
		bind := mpls.IPBind{MPLSTable: 0, Label: b.GetLabel(), VRF: vrf, Prefix: p.Masked().String()}
		if err := bind.Validate(); err != nil {
			s.Errorf(at, "routing.mpls-srmpls-binding", "%v", err)
			continue
		}
		s.Add(mpls.KeyIPBind(bind), df7.Encode(bind), at+"/label")
	}
	for _, name := range sortedKeys(m.GetTunnels()) {
		t := m.GetTunnels()[name]
		at := pt("tunnels", name)
		paths, ok := mplsPaths(s, t.GetPaths(), "", vrfID, at)
		if !ok {
			continue
		}
		tn := mpls.Tunnel{Name: name, L2Only: t.GetL2Only(), Paths: paths}
		if err := tn.Validate(); err != nil {
			s.Errorf(at, "routing.mpls-srmpls-tunnel", "%v", err)
			continue
		}
		s.Add(mpls.KeyTunnel(name), df7.Encode(tn), at)
	}
	for _, key := range sortedKeys(m.GetSr().GetPolicies()) {
		p := m.GetSr().GetPolicies()[key]
		at := pt("sr", "policies", key)
		bsid, err := strconv.ParseUint(key, 10, 32)
		if err != nil {
			s.Errorf(at, "routing.mpls-srmpls-bsid", "binding SID %q is not a label", key)
			continue
		}
		pol := &srmpls.Policy{Bsid: uint32(bsid), Spray: p.GetSpray()}
		for _, sl := range p.GetSegmentLists() {
			w := uint32(1)
			if sl.Weight != nil {
				w = sl.GetWeight()
			}
			pol.SegmentLists = append(pol.SegmentLists, &srmpls.SegmentList{Labels: append([]uint32(nil), sl.GetLabels()...), Weight: w})
		}
		// D-074: kept in ascending order (DF-6 requires it; VPP's path list does not keep the given order)
		sort.SliceStable(pol.SegmentLists, func(a, b int) bool {
			return lessSegmentList(pol.SegmentLists[a], pol.SegmentLists[b])
		})
		s.Add(srmpls.PolicyKey(pol.GetBsid()), pol, at)
	}
	for i, st := range m.GetSr().GetSteering() {
		at := pt("sr", "steering", strconv.Itoa(i))
		vrf, ok := vrfID(vrfOr(st.Vrf))
		if !ok {
			s.Errorf(at+"/vrf", "routing.mpls-srmpls-vrf-exists", "VRF %q does not exist", vrfOr(st.Vrf))
			continue
		}
		p, err := netip.ParsePrefix(st.GetPrefix())
		if err != nil {
			s.Errorf(at+"/prefix", "routing.mpls-srmpls-prefix", "%v", err)
			continue
		}
		prefix := p.Masked().String()
		v := &srmpls.Steering{Prefix: prefix, TableId: vrf, Bsid: st.GetBsid(), VpnLabel: st.GetVpnLabel()}
		s.Add(SteeringKey(vrf, prefix), v, at+"/prefix")
	}
}

func vrfOr(v *string) string {
	if v == nil || *v == "" {
		return "default"
	}
	return *v
}

func lessSegmentList(a, b *srmpls.SegmentList) bool {
	la, lb := a.GetLabels(), b.GetLabels()
	for i := 0; i < len(la) && i < len(lb); i++ {
		if la[i] != lb[i] {
			return la[i] < lb[i]
		}
	}
	if len(la) != len(lb) {
		return len(la) < len(lb)
	}
	return a.GetWeight() < b.GetWeight()
}

// mplsPaths converts document paths to DF-7's canonical paths. payload is the EOS payload of a label
// route ("" for non-EOS routes and tunnels): a path without a next hop carries it as its protocol
// (an IPv6 lookup, an ethernet pseudowire), so VPP resolves it in the right family.
func mplsPaths(s Sink, in []*vrxv1.MplsPath, payload string, vrfID func(string) (uint32, bool), at string) ([]df7.Path, bool) {
	out := make([]df7.Path, 0, len(in))
	ok := true
	for j, p := range in {
		pat := at + "/paths/" + strconv.Itoa(j)
		w := uint32(1)
		if p.Weight != nil {
			w = p.GetWeight()
		}
		if w == 0 || w > 255 {
			s.Errorf(pat+"/weight", "routing.mpls-srmpls-weight", "weight %d outside 1–255", w)
			ok = false
			continue
		}
		dp := df7.Path{NextHop: p.GetNextHop(), Interface: p.GetInterface(), Weight: uint8(w)}
		for _, l := range p.GetOutLabels() {
			dp.Labels = append(dp.Labels, df7.Label{Label: l})
		}
		if p.Vrf != nil {
			id, found := vrfID(p.GetVrf())
			if !found {
				s.Errorf(pat+"/vrf", "routing.mpls-srmpls-vrf-exists", "VRF %q does not exist", p.GetVrf())
				ok = false
				continue
			}
			dp.TableID = id
		}
		if dp.NextHop == "" {
			switch {
			case payload == mpls.PayloadIP6:
				dp.Proto = df7.ProtoIP6
			case payload == mpls.PayloadEthernet && dp.Interface != "":
				dp.Proto = df7.ProtoEthernet
			}
		}
		out = append(out, dp)
	}
	if !ok {
		return nil, false
	}
	norm, err := df7.NormalizePaths(out)
	if err != nil {
		s.Errorf(at+"/paths", "routing.mpls-srmpls-path", "%v", err)
		return nil, false
	}
	return norm, true
}

// AssembleMplsSrmpls adds `routing.mpls` to ds from the retrieved MPLS objects (canonical form):
// interfaces sorted, tables without table 0, label routes by table, label and end-of-stack first,
// tunnels by name. ipBindings and sr are write-only in VPP 26.06 and never assembled (D-063).
// nameOf maps an IP table id to its VRF name. ds.Routing must be set.
func AssembleMplsSrmpls(ds *vrxv1.DesiredState, kvs []scheduler.KV, nameOf func(uint32) string) {
	m := &vrxv1.MplsConfig{}
	var routes []mpls.Route
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case mpls.NameTable:
			if t, err := df7.Decode[mpls.Table](kv.Value); err == nil && t.ID != 0 {
				if m.Tables == nil {
					m.Tables = map[string]*vrxv1.MplsTable{}
				}
				m.Tables[strconv.FormatUint(uint64(t.ID), 10)] = &vrxv1.MplsTable{}
			}
		case mpls.NameInterface:
			if i, err := df7.Decode[mpls.Interface](kv.Value); err == nil {
				m.Interfaces = append(m.Interfaces, i.Interface)
			}
		case mpls.NameRoute:
			if r, err := df7.Decode[mpls.Route](kv.Value); err == nil {
				routes = append(routes, r)
			}
		case mpls.NameTunnel:
			if t, err := df7.Decode[mpls.Tunnel](kv.Value); err == nil {
				if m.Tunnels == nil {
					m.Tunnels = map[string]*vrxv1.MplsTunnel{}
				}
				m.Tunnels[t.Name] = &vrxv1.MplsTunnel{Paths: docPaths(t.Paths, nameOf), L2Only: proto.Bool(t.L2Only)}
			}
		}
	}
	sort.Strings(m.Interfaces)
	sort.Slice(routes, func(a, b int) bool {
		x, y := routes[a], routes[b]
		if x.Table != y.Table {
			return x.Table < y.Table
		}
		if x.Label != y.Label {
			return x.Label < y.Label
		}
		return x.EOS && !y.EOS
	})
	for _, r := range routes {
		lr := &vrxv1.MplsLabelRoute{Table: proto.Uint32(r.Table), Label: proto.Uint32(r.Label), Eos: proto.Bool(r.EOS), Paths: docPaths(r.Paths, nameOf)}
		if r.EOS && r.EOSProto != MplsPayload("", lr.GetPaths()) {
			lr.Payload = proto.String(r.EOSProto)
		}
		m.LabelRoutes = append(m.LabelRoutes, lr)
	}
	if len(m.Interfaces) == 0 && len(m.Tables) == 0 && len(m.LabelRoutes) == 0 && len(m.Tunnels) == 0 {
		return
	}
	if ds.Routing == nil {
		ds.Routing = &vrxv1.RoutingConfig{}
	}
	ds.Routing.Mpls = m
}

// docPaths converts DF-7 paths back to document paths: a path with neither next hop nor interface
// that resolves in an IP table is a pop-and-lookup path of that table's VRF.
func docPaths(paths []df7.Path, nameOf func(uint32) string) []*vrxv1.MplsPath {
	out := make([]*vrxv1.MplsPath, 0, len(paths))
	for _, p := range paths {
		mp := &vrxv1.MplsPath{Weight: proto.Uint32(uint32(p.Weight))}
		if p.NextHop != "" {
			mp.NextHop = proto.String(p.NextHop)
		}
		if p.Interface != "" {
			mp.Interface = proto.String(p.Interface)
		}
		for _, l := range p.Labels {
			mp.OutLabels = append(mp.OutLabels, l.Label)
		}
		if p.NextHop == "" && p.Interface == "" && p.Type == df7.PathNormal && (p.Proto == df7.ProtoIP4 || p.Proto == df7.ProtoIP6) {
			mp.Vrf = proto.String(nameOf(p.TableID))
		}
		out = append(out, mp)
	}
	return out
}
