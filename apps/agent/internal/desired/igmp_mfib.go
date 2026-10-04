package desired

import (
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/igmp"
	"ngfw/agent/internal/descriptors/mfib"
	"ngfw/agent/internal/scheduler"
	"sort"
	"strconv"
	"sync/atomic"
)

var igmpGlobals atomic.Bool

// ConfigureIgmpGlobals records the process globals-owner role.
func ConfigureIgmpGlobals(enabled bool) { igmpGlobals.Store(enabled) }

// IgmpGlobalsOwner reports whether global SSM ranges may be projected.
func IgmpGlobalsOwner() bool { return igmpGlobals.Load() }

// IgmpMfib projects the existing multicast contract; globals are emitted only by their owner.
func IgmpMfib(s Sink, ds *ngfwv1.DesiredState, in map[string]bool, vrfID func(string) (uint32, bool), globals bool) {
	if !in["routing"] {
		return
	}
	m := ds.GetRouting().GetMulticast()
	if m == nil {
		return
	}
	base := Ptr("routing", "multicast")
	g := m.GetIgmp()
	for _, name := range sortedKeys(g.GetInterfaces()) {
		v := g.GetInterfaces()[name]
		at := base + "/igmp/interfaces/" + Ptr(name)[1:]
		i := igmp.Interface{Interface: name, Mode: v.GetMode()}
		if e := i.Validate(); e != nil {
			s.Errorf(at, "multicast.igmp-interface", "%v", e)
			continue
		}
		s.Add(igmp.KeyInterface(name), df7.Encode(i), at)
		s.Warnf(at, RuleWriteOnly, "IGMP interface mode is write-only in VPP 26.06")
		for j, join := range v.GetJoins() {
			jat := at + "/joins/" + strconv.Itoa(j)
			src := append([]string(nil), join.GetSources()...)
			df7.SortAddrStrings(src)
			l := igmp.Listen{Interface: name, Group: join.GetGroup(), Sources: src}
			if i.Mode != igmp.ModeHost {
				s.Errorf(jat, "multicast.host-join", "joins require host mode")
				continue
			}
			if e := l.Validate(); e != nil {
				s.Errorf(jat, "multicast.include-only", "%v", e)
				continue
			}
			s.Add(igmp.KeyListen(name, l.Group), df7.Encode(l), jat)
		}
	}
	if globals {
		ranges := g.GetSsmRanges()

		for j, p := range ranges {
			v := igmp.GroupPrefix{Prefix: p}
			at := base + "/igmp/ssmRanges/" + strconv.Itoa(j)
			if e := v.Validate(); e != nil {
				s.Errorf(at, "multicast.ssm-range", "%v", e)
				continue
			}
			s.Add(igmp.KeyGroupPrefix(p), df7.Encode(v), at)
			s.Warnf(at, RuleWriteOnly, "IGMP SSM ranges are global and write-only")
		}
	}
	for _, vrf := range sortedKeys(g.GetProxies()) {
		v := g.GetProxies()[vrf]
		id, ok := vrfID(vrf)
		at := base + "/igmp/proxies/" + Ptr(vrf)[1:]
		if !ok {
			s.Errorf(at, "multicast.vrf", "unknown VRF %q", vrf)
			continue
		}
		s.Add(igmp.KeyProxyDevice(id), df7.Encode(igmp.ProxyDevice{VRF: id, Upstream: v.GetUpstream()}), at)
		for _, name := range v.GetDownstream() {
			s.Add(igmp.KeyDownstream(id, name), df7.Encode(igmp.Downstream{VRF: id, Interface: name}), at)
		}
		s.Warnf(at, RuleWriteOnly, "IGMP proxies are write-only")
	}
	for j, v := range m.GetMroutes() {
		at := base + "/mroutes/" + strconv.Itoa(j)
		vrf := v.GetVrf()
		if vrf == "" {
			vrf = "default"
		}
		id, ok := vrfID(vrf)
		if !ok {
			s.Errorf(at+"/vrf", "multicast.vrf", "unknown VRF %q", vrf)
			continue
		}
		r := mfib.Route{Table: id, Group: v.GetGroup(), Source: v.GetSource()}
		for _, p := range v.GetPaths() {
			r.Paths = append(r.Paths, mfib.Path{Interface: p.GetInterface(), Flags: p.GetFlags()})
		}
		sort.Slice(r.Paths, func(i, j int) bool { return r.Paths[i].Interface < r.Paths[j].Interface })
		if e := r.Validate(); e != nil {
			s.Errorf(at, "multicast.route", "%v", e)
			continue
		}
		s.Add(mfib.Key(r), df7.Encode(r), at)
	}
}

// AssembleIgmpMfib reports readable static joins and mFIB entries. Write-only leaves remain omitted.
func AssembleIgmpMfib(ds *ngfwv1.DesiredState, kvs []scheduler.KV, nameOf func(uint32) string) {
	m := &ngfwv1.MulticastConfig{}
	for _, kv := range kvs {
		switch kv.Key.Descriptor() {
		case mfib.Name:
			r, e := df7.Decode[mfib.Route](kv.Value)
			if e != nil {
				continue
			}
			v := &ngfwv1.Mroute{Vrf: strPtr(nameOf(r.Table)), Group: strPtr(r.Group)}
			if r.Source != "" {
				v.Source = strPtr(r.Source)
			}
			for _, p := range r.Paths {
				v.Paths = append(v.Paths, &ngfwv1.MroutePath{Interface: strPtr(p.Interface), Flags: strPtr(p.Flags)})
			}
			m.Mroutes = append(m.Mroutes, v)
		case igmp.NameListen:
			r, e := df7.Decode[igmp.Listen](kv.Value)
			if e != nil {
				continue
			}
			if m.Igmp == nil {
				m.Igmp = &ngfwv1.IgmpConfig{Interfaces: map[string]*ngfwv1.IgmpInterface{}}
			}
			i := m.Igmp.Interfaces[r.Interface]
			if i == nil {
				i = &ngfwv1.IgmpInterface{Mode: strPtr(igmp.ModeHost)}
				m.Igmp.Interfaces[r.Interface] = i
			}
			i.Joins = append(i.Joins, &ngfwv1.IgmpJoin{Group: strPtr(r.Group), Sources: r.Sources})
		}
	}
	if len(m.Mroutes) > 0 || m.Igmp != nil {
		if ds.Routing == nil {
			ds.Routing = &ngfwv1.RoutingConfig{}
		}
		ds.Routing.Multicast = m
	}
}
