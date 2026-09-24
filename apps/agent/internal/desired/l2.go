package desired

// F-bridge-l2: the L2 half of the desired state (D-109 c) — per-port leaves `interfaces.<if>.l2` /
// `….subinterfaces.<id>.l2` and the container `routing.l2` — projected onto DF-1's l2 / l3xc
// descriptors and the mactime descriptors, and assembled back from their retrieved objects:
//
//	routing.l2.bridgeDomains.<name>            → l2.bridge-domain/<id>          (name in the bd_tag)
//	  .staticMacs[]                            → l2.fib-entry/<id>/<mac>        (static)
//	interfaces.<if>.l2.bridgeDomain (+shg,bvi,uuFwd)
//	                                           → l2.bridge-domain-member/<id>/<if>
//	interfaces.<if>.l2.tagRewrite              → l2.vlan-tag-rewrite/<if>       (dependent of the membership)
//	interfaces.<if>.l2.macFilter               → mactime.enable/<if>
//	routing.l2.xconnects.<rx>                  → l2.xconnect/<rx>
//	routing.l2.l3xc.<rx>                       → l3xc.l3xc/<rx>/ip4|ip6         (vrf → table id)
//	routing.l2.macFilters.<name>               → mactime.range/<name>          (one VPP range per day)
//
// Every interface reference is the D-065 alias `interface/<name>`, so every object depends on the
// interface alias and the scheduler deletes these dependents before any interface (D-095c).
// interfaces.go is not edited: this builder iterates ds.Interfaces itself (wave-A-hotspots A3).

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/dfkit"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/l2"
	"ngfw/agent/internal/descriptors/l3xc"
	"ngfw/agent/internal/descriptors/mactime"
	"ngfw/agent/internal/scheduler"
)

// vtrOps maps the configuration spelling of a tag-rewrite operation to VPP's l2_vtr_op_t.
var vtrOps = map[string]l2.VtrOp{
	"push-1":        l2.VtrOp_VTR_OP_PUSH_1,
	"push-2":        l2.VtrOp_VTR_OP_PUSH_2,
	"pop-1":         l2.VtrOp_VTR_OP_POP_1,
	"pop-2":         l2.VtrOp_VTR_OP_POP_2,
	"translate-1-1": l2.VtrOp_VTR_OP_TRANSLATE_1_1,
	"translate-1-2": l2.VtrOp_VTR_OP_TRANSLATE_1_2,
	"translate-2-1": l2.VtrOp_VTR_OP_TRANSLATE_2_1,
	"translate-2-2": l2.VtrOp_VTR_OP_TRANSLATE_2_2,
}

// TagRewriteName is the configuration spelling of op ("" for disabled / unknown).
func TagRewriteName(op l2.VtrOp) string {
	for n, v := range vtrOps {
		if v == op {
			return n
		}
	}
	return ""
}

// vtrTags is how many tags op writes (tag1 / tag1+tag2).
func vtrTags(op l2.VtrOp) int {
	switch op {
	case l2.VtrOp_VTR_OP_POP_1, l2.VtrOp_VTR_OP_POP_2:
		return 0
	case l2.VtrOp_VTR_OP_PUSH_1, l2.VtrOp_VTR_OP_TRANSLATE_1_1, l2.VtrOp_VTR_OP_TRANSLATE_2_1:
		return 1
	}
	return 2
}

// VPP's mactime clock counts from Sunday; the configuration lists days in the `objects` order.
var (
	weekdayIndex = map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	weekdayOrder = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
)

func hhmm(s string) (int, bool) {
	h, m, ok := strings.Cut(s, ":")
	if !ok {
		return 0, false
	}
	hv, err1 := strconv.Atoi(h)
	mv, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hv < 0 || mv < 0 || mv > 59 || hv*60+mv > 24*60 {
		return 0, false
	}
	return (hv*60 + mv) * 60, true
}

func fmtHHMM(sec int) string { return fmt.Sprintf("%02d:%02d", sec/3600, sec%3600/60) }

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// l2Port is one (sub-)interface of the document that carries an `l2` leaf.
type l2Port struct {
	name string
	ptr  string
	l2   *vrxv1.BridgeL2Port
	sub  bool
}

func l2Ports(ifs map[string]*vrxv1.Interface) []l2Port {
	var out []l2Port
	for _, name := range sortedKeys(ifs) {
		itf := ifs[name]
		if itf.GetL2() != nil {
			out = append(out, l2Port{name: name, ptr: Ptr("interfaces", name), l2: itf.GetL2()})
		}
		for _, id := range sortedKeys(itf.GetSubinterfaces()) {
			if sub := itf.GetSubinterfaces()[id]; sub.GetL2() != nil {
				out = append(out, l2Port{name: SubName(name, id), ptr: Ptr("interfaces", name, "subinterfaces", id), l2: sub.GetL2(), sub: true})
			}
		}
	}
	return out
}

func ifRef(name string) string { return string(iface.AliasKey(name)) }

// BridgeL2 emits the objects of the L2 model (interfaces.<if>.l2 + routing.l2). vrfID maps a VRF
// name to its table id (false: unknown VRF).
func BridgeL2(s Sink, ds *vrxv1.DesiredState, vrfID func(string) (uint32, bool)) {
	cfg := ds.GetRouting().GetL2()
	bdID := map[string]uint32{}
	for _, name := range sortedKeys(cfg.GetBridgeDomains()) {
		bd := cfg.GetBridgeDomains()[name]
		pt := Ptr("routing", "l2", "bridgeDomains", name)
		if bd.Id == nil || bd.GetId() == 0 || bd.GetId() > 16777215 {
			s.Errorf(pt+"/id", "routing.bridge-l2-domain-id", "bridge domain %q needs an id 1–16777215", name)
			continue
		}
		bdID[name] = bd.GetId()
		s.Add(l2.BridgeDomainKey(bd.GetId()), &l2.BridgeDomain{
			Id: bd.GetId(), Name: name,
			Flood: boolOr(bd.Flood, true), UuFlood: boolOr(bd.UuFlood, true), Forward: boolOr(bd.Forward, true), Learn: boolOr(bd.Learn, true),
			ArpTerm: bd.GetArpTerm(), MacAge: bd.GetMacAgeMin(),
		}, pt)
		for i, e := range bd.GetStaticMacs() {
			ept := Ptr("routing", "l2", "bridgeDomains", name, "staticMacs", strconv.Itoa(i))
			mac, err := iface.ParseMAC(e.GetMac())
			if err != nil || e.GetInterface() == "" {
				s.Errorf(ept, "routing.bridge-l2-static-mac", "static MAC %q needs a valid MAC and an interface", e.GetMac())
				continue
			}
			m := iface.FormatMAC(mac)
			s.Add(l2.FibEntryKey(bd.GetId(), m), &l2.FibEntry{BridgeDomain: bd.GetId(), Mac: m, Interface: ifRef(e.GetInterface()), Static: true}, ept)
		}
	}

	xcRx := map[string]bool{}
	for _, rx := range sortedKeys(cfg.GetXconnects()) {
		x := cfg.GetXconnects()[rx]
		pt := Ptr("routing", "l2", "xconnects", rx)
		if x.GetTx() == "" || x.GetTx() == rx {
			s.Errorf(pt+"/tx", "routing.bridge-l2-xconnect", "cross-connect %s needs a transmit interface other than itself", rx)
			continue
		}
		xcRx[rx] = true
		s.Add(l2.XconnectKey(ifRef(rx)), &l2.Xconnect{Rx: ifRef(rx), Tx: ifRef(x.GetTx())}, pt)
	}

	for _, p := range l2Ports(ds.GetInterfaces()) {
		ref := ifRef(p.name)
		var memberOf uint32
		if bdName := p.l2.BridgeDomain; bdName != nil {
			id, ok := bdID[*bdName]
			if !ok {
				s.Errorf(p.ptr+"/l2/bridgeDomain", "interfaces.bridge-l2-domain-exists", "bridge domain %q does not exist under /routing/l2/bridgeDomains", *bdName)
			} else {
				pt := l2.PortType_PORT_TYPE_NORMAL
				switch {
				case p.l2.GetBvi() && p.l2.GetUuFwd():
					s.Errorf(p.ptr+"/l2/uuFwd", "interfaces.bridge-l2-port-role", "a member is either the BVI or the uu-fwd port, not both")
					continue
				case p.l2.GetBvi():
					pt = l2.PortType_PORT_TYPE_BVI
				case p.l2.GetUuFwd():
					pt = l2.PortType_PORT_TYPE_UU_FWD
				}
				if p.l2.GetShg() > 255 {
					s.Errorf(p.ptr+"/l2/shg", "interfaces.bridge-l2-port-role", "split-horizon group %d out of range 0–255", p.l2.GetShg())
					continue
				}
				s.Add(l2.MemberKey(id, ref), &l2.BridgeDomainMember{BridgeDomain: id, Interface: ref, PortType: pt, Shg: p.l2.GetShg()}, p.ptr+"/l2/bridgeDomain")
				if !p.l2.GetBvi() {
					memberOf = id
				}
			}
		}
		if tr := p.l2.GetTagRewrite(); tr != nil {
			tpt := p.ptr + "/l2/tagRewrite"
			op, ok := vtrOps[tr.GetOp()]
			switch {
			case !ok:
				s.Errorf(tpt+"/op", "interfaces.bridge-l2-tag-rewrite", "unknown tag-rewrite operation %q", tr.GetOp())
			case memberOf == 0 && !xcRx[p.name]:
				s.Errorf(tpt, "interfaces.bridge-l2-tag-rewrite-l2-only", "VLAN tag rewrite needs an L2 port: %s is not a (non-BVI) bridge member or an L2 cross-connect rx", p.name)
			default:
				v := &l2.VlanTagRewrite{Interface: ref, Op: op, BridgeDomain: memberOf, Xconnect: xcRx[p.name]}
				n := vtrTags(op)
				if n >= 1 {
					v.Tag1 = tr.GetTag1()
					v.PushDot1Q = !tr.GetDot1Ad()
				}
				if n == 2 {
					v.Tag2 = tr.GetTag2()
				}
				s.Add(scheduler.Join(l2.VlanTagRewriteName, p.name), v, tpt)
			}
		}
		if p.l2.GetMacFilter() {
			if p.sub {
				s.Errorf(p.ptr+"/l2/macFilter", "interfaces.bridge-l2-mac-filter-parent", "the MAC filter runs on parent (hardware) interfaces only")
			} else {
				s.Add(mactime.EnableKey(p.name), mactime.Enable{Interface: p.name}.Proto(), p.ptr+"/l2/macFilter")
			}
		}
	}

	for _, rx := range sortedKeys(cfg.GetL3Xc()) {
		x := cfg.GetL3Xc()[rx]
		for _, fam := range []struct {
			ipv6  bool
			field string
			paths []*vrxv1.BridgeL2L3XcPath
		}{{false, "ipv4Paths", x.GetIpv4Paths()}, {true, "ipv6Paths", x.GetIpv6Paths()}} {
			if len(fam.paths) == 0 {
				continue
			}
			v := &l3xc.L3Xc{Interface: ifRef(rx), Ipv6: fam.ipv6}
			bad := false
			for j, p := range fam.paths {
				ppt := Ptr("routing", "l2", "l3xc", rx, fam.field, strconv.Itoa(j))
				vrf := p.GetVrf()
				if vrf == "" {
					vrf = "default"
				}
				table, ok := vrfID(vrf)
				if !ok {
					s.Errorf(ppt+"/vrf", "routing.bridge-l2-l3xc", "VRF %q does not exist", vrf)
					bad = true
					continue
				}
				path := &l3xc.Path{Table: table, Weight: p.GetWeight(), Preference: p.GetPreference()}
				if p.Weight == nil || path.Weight == 0 {
					path.Weight = 1
				}
				if p.NextHop != nil {
					a, err := netip.ParseAddr(p.GetNextHop())
					if err != nil || a.Is6() != fam.ipv6 {
						s.Errorf(ppt+"/nextHop", "routing.bridge-l2-l3xc", "next hop %q is not an address of the list's family", p.GetNextHop())
						bad = true
						continue
					}
					path.NextHop = a.String()
				}
				if p.GetInterface() != "" {
					path.Interface = ifRef(p.GetInterface())
				}
				v.Paths = append(v.Paths, path)
			}
			if bad {
				continue
			}
			l3xc.SortPaths(v.Paths)
			af := "ip4"
			if fam.ipv6 {
				af = "ip6"
			}
			s.Add(scheduler.Join(l3xc.L3xcName, rx, af), v, Ptr("routing", "l2", "l3xc", rx, fam.field))
		}
	}

	for _, name := range sortedKeys(cfg.GetMacFilters()) {
		f := cfg.GetMacFilters()[name]
		pt := Ptr("routing", "l2", "macFilters", name)
		mac, err := iface.ParseMAC(f.GetMac())
		if err != nil {
			s.Errorf(pt+"/mac", "routing.bridge-l2-mac-filter", "invalid MAC %q", f.GetMac())
			continue
		}
		dev := mactime.Device{Name: name, MAC: iface.FormatMAC(mac), Drop: f.GetAction() == "drop", Ranges: []mactime.Range{}}
		bad := false
		for i, r := range f.GetRanges() {
			start, ok1 := hhmm(r.GetStart())
			end, ok2 := hhmm(r.GetEnd())
			if !ok1 || !ok2 || end <= start {
				s.Errorf(Ptr("routing", "l2", "macFilters", name, "ranges", strconv.Itoa(i)), "routing.bridge-l2-mac-filter", "range %s–%s is not HH:MM with end after start", r.GetStart(), r.GetEnd())
				bad = true
				continue
			}
			for _, day := range r.GetDays() {
				d, ok := weekdayIndex[day]
				if !ok {
					s.Errorf(Ptr("routing", "l2", "macFilters", name, "ranges", strconv.Itoa(i), "days"), "routing.bridge-l2-mac-filter", "unknown day %q", day)
					bad = true
					continue
				}
				dev.Ranges = append(dev.Ranges, mactime.Range{Start: float64(d*86400 + start), End: float64(d*86400 + end)})
			}
		}
		if !bad {
			s.Add(mactime.RangeKey(name), dev.Proto(), pt)
		}
	}
}

// AssembleBridgeL2 adds the L2 model to a DesiredState assembled by Assemble: the per-port leaves
// on ds.Interfaces and routing.l2. stored is the agent's stored `interfaces` document (a leaf the
// document sets on an existing interface without any L2 object is reported with its defaults, so a
// removed membership shows as drift); tableName names a FIB table.
func AssembleBridgeL2(ds *vrxv1.DesiredState, kvs []scheduler.KV, stored map[string]*vrxv1.Interface, tableName func(uint32) string) {
	cfg := &vrxv1.BridgeL2Config{}
	bdName := map[uint32]string{}
	for _, kv := range kvs {
		if b, ok := kv.Value.(*l2.BridgeDomain); ok {
			name := b.GetName()
			if name == "" {
				name = strconv.FormatUint(uint64(b.GetId()), 10)
			}
			bdName[b.GetId()] = name
			if cfg.BridgeDomains == nil {
				cfg.BridgeDomains = map[string]*vrxv1.BridgeL2Domain{}
			}
			cfg.BridgeDomains[name] = &vrxv1.BridgeL2Domain{
				Id: proto.Uint32(b.GetId()), Flood: proto.Bool(b.GetFlood()), UuFlood: proto.Bool(b.GetUuFlood()),
				Forward: proto.Bool(b.GetForward()), Learn: proto.Bool(b.GetLearn()), ArpTerm: proto.Bool(b.GetArpTerm()),
				MacAgeMin: proto.Uint32(b.GetMacAge()),
			}
		}
	}
	nameOfBD := func(id uint32) string {
		if n, ok := bdName[id]; ok {
			return n
		}
		return strconv.FormatUint(uint64(id), 10)
	}
	ports := map[string]*vrxv1.BridgeL2Port{}
	port := func(name string) *vrxv1.BridgeL2Port {
		if p, ok := ports[name]; ok {
			return p
		}
		p := &vrxv1.BridgeL2Port{}
		ports[name] = p
		return p
	}
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case *l2.BridgeDomainMember:
			p := port(iface.RefID(v.GetInterface()))
			p.BridgeDomain = proto.String(nameOfBD(v.GetBridgeDomain()))
			p.Shg = proto.Uint32(v.GetShg())
			p.Bvi = proto.Bool(v.GetPortType() == l2.PortType_PORT_TYPE_BVI)
			p.UuFwd = proto.Bool(v.GetPortType() == l2.PortType_PORT_TYPE_UU_FWD)
		case *l2.VlanTagRewrite:
			tr := &vrxv1.BridgeL2TagRewrite{Op: proto.String(TagRewriteName(v.GetOp())), Dot1Ad: proto.Bool(false)}
			if n := vtrTags(v.GetOp()); n >= 1 {
				tr.Tag1 = proto.Uint32(v.GetTag1())
				tr.Dot1Ad = proto.Bool(!v.GetPushDot1Q())
				if n == 2 {
					tr.Tag2 = proto.Uint32(v.GetTag2())
				}
			}
			port(iface.RefID(v.GetInterface())).TagRewrite = tr
		case *l2.FibEntry:
			bd := cfg.GetBridgeDomains()[nameOfBD(v.GetBridgeDomain())]
			if bd != nil && v.GetStatic() && v.GetInterface() != "" {
				bd.StaticMacs = append(bd.StaticMacs, &vrxv1.BridgeL2StaticMac{Mac: proto.String(v.GetMac()), Interface: proto.String(iface.RefID(v.GetInterface()))})
			}
		case *l2.Xconnect:
			if cfg.Xconnects == nil {
				cfg.Xconnects = map[string]*vrxv1.BridgeL2Xconnect{}
			}
			cfg.Xconnects[iface.RefID(v.GetRx())] = &vrxv1.BridgeL2Xconnect{Tx: proto.String(iface.RefID(v.GetTx()))}
		case *l3xc.L3Xc:
			if cfg.L3Xc == nil {
				cfg.L3Xc = map[string]*vrxv1.BridgeL2L3Xc{}
			}
			rx := iface.RefID(v.GetInterface())
			x := cfg.L3Xc[rx]
			if x == nil {
				x = &vrxv1.BridgeL2L3Xc{}
				cfg.L3Xc[rx] = x
			}
			var paths []*vrxv1.BridgeL2L3XcPath
			for _, p := range v.GetPaths() {
				out := &vrxv1.BridgeL2L3XcPath{Vrf: proto.String(tableName(p.GetTable())), Weight: proto.Uint32(p.GetWeight()), Preference: proto.Uint32(p.GetPreference())}
				if p.GetNextHop() != "" {
					out.NextHop = proto.String(p.GetNextHop())
				}
				if p.GetInterface() != "" {
					out.Interface = proto.String(iface.RefID(p.GetInterface()))
				}
				paths = append(paths, out)
			}
			if v.GetIpv6() {
				x.Ipv6Paths = paths
			} else {
				x.Ipv4Paths = paths
			}
		}
		switch kv.Key.Descriptor() {
		case mactime.EnableName:
			port(kv.Key.ID()).MacFilter = proto.Bool(true)
		case mactime.RangeName:
			var dev mactime.Device
			if err := dfkit.Decode(kv.Value, &dev); err == nil {
				if cfg.MacFilters == nil {
					cfg.MacFilters = map[string]*vrxv1.BridgeL2MacFilter{}
				}
				cfg.MacFilters[dev.Name] = macFilterOf(dev)
			}
		}
	}
	for _, bd := range cfg.GetBridgeDomains() {
		sort.Slice(bd.StaticMacs, func(i, j int) bool { return bd.StaticMacs[i].GetMac() < bd.StaticMacs[j].GetMac() })
	}
	// a leaf the stored document sets on an interface that exists, without any L2 object: defaults
	for name, itf := range stored {
		if itf.GetL2() != nil && lookupNode(ds, name, false) {
			port(name)
		}
		for id, sub := range itf.GetSubinterfaces() {
			if sname := SubName(name, id); sub.GetL2() != nil && lookupNode(ds, sname, false) {
				port(sname)
			}
		}
	}
	for name, p := range ports {
		if p.Shg == nil {
			p.Shg = proto.Uint32(0)
		}
		if p.Bvi == nil {
			p.Bvi = proto.Bool(false)
		}
		if p.UuFwd == nil {
			p.UuFwd = proto.Bool(false)
		}
		if p.MacFilter == nil {
			p.MacFilter = proto.Bool(false)
		}
		setPort(ds, name, p)
	}
	if len(cfg.GetBridgeDomains())+len(cfg.GetXconnects())+len(cfg.GetL3Xc())+len(cfg.GetMacFilters()) > 0 {
		if ds.Routing == nil {
			ds.Routing = &vrxv1.RoutingConfig{}
		}
		ds.Routing.L2 = cfg
	}
}

// macFilterOf turns VPP's per-day ranges back into the configuration's weekly ranges: ranges with
// the same times are grouped (days in the configuration's mon…sun order), groups sorted by their
// first day, then (start, end) — the canonical order Retrieve reports.
func macFilterOf(dev mactime.Device) *vrxv1.BridgeL2MacFilter {
	action := "allow"
	if dev.Drop {
		action = "drop"
	}
	out := &vrxv1.BridgeL2MacFilter{Mac: proto.String(dev.MAC), Action: proto.String(action)}
	type span struct{ start, end int }
	days := map[span]map[string]bool{}
	var spans []span
	for _, r := range dev.Ranges {
		day := int(r.Start) / 86400
		if day < 0 || day > 6 {
			continue
		}
		sp := span{int(r.Start) - day*86400, int(r.End) - day*86400}
		if days[sp] == nil {
			days[sp] = map[string]bool{}
			spans = append(spans, sp)
		}
		for n, i := range weekdayIndex {
			if i == day {
				days[sp][n] = true
			}
		}
	}
	first := func(sp span) int { // position of the span's first day in the configuration's week order
		for i, d := range weekdayOrder {
			if days[sp][d] {
				return i
			}
		}
		return len(weekdayOrder)
	}
	sort.Slice(spans, func(i, j int) bool {
		if fi, fj := first(spans[i]), first(spans[j]); fi != fj {
			return fi < fj
		}
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end < spans[j].end
	})
	for _, sp := range spans {
		r := &vrxv1.BridgeL2MacFilterRange{Start: proto.String(fmtHHMM(sp.start)), End: proto.String(fmtHHMM(sp.end))}
		for _, d := range weekdayOrder {
			if days[sp][d] {
				r.Days = append(r.Days, d)
			}
		}
		out.Ranges = append(out.Ranges, r)
	}
	return out
}

// lookupNode reports whether ds.Interfaces has the (sub-)interface name; with create it adds a
// parent that is missing (a physical member the assembler did not name otherwise).
func lookupNode(ds *vrxv1.DesiredState, name string, create bool) bool {
	parent, id, isSub := strings.Cut(name, ".")
	itf := ds.GetInterfaces()[parent]
	if itf == nil {
		if !create || isSub {
			return false
		}
		if ds.Interfaces == nil {
			ds.Interfaces = map[string]*vrxv1.Interface{}
		}
		ds.Interfaces[parent] = &vrxv1.Interface{Enabled: proto.Bool(false), Promiscuous: proto.Bool(false), Vrf: proto.String("default")}
		return true
	}
	if !isSub {
		return true
	}
	_, ok := itf.GetSubinterfaces()[id]
	return ok
}

func setPort(ds *vrxv1.DesiredState, name string, p *vrxv1.BridgeL2Port) {
	if !lookupNode(ds, name, true) {
		return // a sub-interface the agent did not assemble: not ours to report
	}
	parent, id, isSub := strings.Cut(name, ".")
	if isSub {
		ds.Interfaces[parent].Subinterfaces[id].L2 = p
		return
	}
	ds.Interfaces[parent].L2 = p
}
