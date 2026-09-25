package desired

// LISP / LISP-GPE builder and assembler (F-lisp, WBS D6.8): `tunnels.lisp` (vrx.v1.LispConfig) ⇄ DF-6's
// lisp descriptors.
//
//	enabled                       → lisp.enable/global          (setter on the globals owner, require variant elsewhere)
//	gpe                           → lisp-gpe.enable/global      (idem)
//	pitr                          → lisp.pitr/global            (idem)
//	locatorSets.<n>               → lisp.locator-set/<n>
//	locatorSets.<n>.locators[]    → lisp.locator/<n>/<interface>
//	localEids[]                   → lisp.local-eid/<vni>/<eid>
//	eidTables.<vni>               → lisp.eid-table-map/l3|l2/<vni>   (vrf → table id; bridgeDomain → bd id)
//	remoteMappings[]              → lisp.remote-mapping/<vni>/<eid>
//	adjacencies[]                 → lisp.adjacency/<vni>/<reid>/<leid>
//	gpeEntries[]                  → lisp-gpe.fwd-entry/<vni>/<reid>/<leid>   (write-only, V13)
//	mapResolvers[] / mapServers[] → lisp.map-resolver|map-server/<address>
//
// Values are built in the descriptors' canonical form (masked prefixes, lower-case MACs, canonical
// addresses, sorted RLOCs) so the key equals the descriptor's KeyOf. The assembler never echoes desired
// state: write-only objects (GPE entries; the globals on an agent that is not the globals owner) are
// left out of Retrieve (contract §5), and `enabled` is reported when VPP shows the switch or when any
// owned LISP object exists (VPP cannot hold one with LISP off).

import (
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/lisp"
	"ngfw/agent/internal/scheduler"
)

// lispActions are the negative-mapping actions in VPP order (lisp_types action 0–3).
var lispActions = []string{"no-action", "natively-forward", "send-map-request", "drop"}

func lispAction(s string) (uint32, bool) {
	if s == "" {
		return 0, true
	}
	for i, a := range lispActions {
		if a == s {
			return uint32(i), true //nolint:gosec // < 4
		}
	}
	return 0, false
}

func lispActionName(v uint32) string {
	if int(v) < len(lispActions) {
		return lispActions[v]
	}
	return strconv.FormatUint(uint64(v), 10)
}

func canonLispEID(s string) (string, error) {
	if strings.Contains(s, "/") {
		return df6.CanonicalPrefix(s)
	}
	return df6.CanonicalMAC(s)
}

func pw(v *uint32) uint32 {
	if v == nil {
		return 1 // schema default
	}
	return *v
}

func lispPtr(segs ...string) string { return Ptr(append([]string{"tunnels", "lisp"}, segs...)...) }

// Lisp projects tunnels.lisp. vrfID maps VRF names to table ids. Unset lisp: nothing (every owned LISP
// object is deleted when `tunnels` is authoritative). Until F-tunnels wires gre / vxlan / ipip, a
// non-empty one of those is reported as agent.unsupported-field (this build implements `tunnels`
// through LISP only).
func Lisp(s Sink, t *vrxv1.TunnelsConfig, vrfID func(string) (uint32, bool)) {
	for _, k := range []struct {
		kind string
		n    int
	}{{"gre", len(t.GetGre())}, {"vxlan", len(t.GetVxlan())}, {"ipip", len(t.GetIpip())}} {
		if kind, n := k.kind, k.n; n > 0 {
			s.Warnf(Ptr("tunnels", kind), "agent.unsupported-field", "tunnels.%s is not implemented by this agent build and is not applied", kind)
		}
	}
	l := t.GetLisp()
	if l == nil {
		return
	}
	i := strconv.Itoa
	eid := func(v string, segs ...string) (string, bool) {
		c, err := canonLispEID(v)
		if err != nil {
			s.Errorf(lispPtr(segs...), "tunnels.lisp-eid-canonical", "%v", err)
			return "", false
		}
		return c, true
	}
	addr := func(v string, segs ...string) (string, bool) {
		a, err := df6.ParseAddr(v)
		if err != nil {
			s.Errorf(lispPtr(segs...), "tunnels.lisp-address", "%v", err)
			return "", false
		}
		return a.String(), true
	}
	if l.GetEnabled() {
		s.Add(lisp.EnableKey, &lisp.Enable{}, lispPtr("enabled"))
		if !l.GetGpe() {
			s.Warnf(lispPtr("gpe"), "tunnels.lisp-gpe-implied", "VPP enables LISP-GPE together with LISP; gpe: false is not enforced")
		}
	}
	if l.GetGpe() {
		s.Add(lisp.GpeEnableKey, &lisp.GpeEnable{}, lispPtr("gpe"))
	}
	for _, name := range sortedKeys(l.GetLocatorSets()) {
		s.Add(scheduler.Join(lisp.LocatorSetName, name), &lisp.LocatorSet{Name: name}, lispPtr("locatorSets", name))
		for j, loc := range l.GetLocatorSets()[name].GetLocators() {
			v := &lisp.Locator{LocatorSet: name, Interface: loc.GetInterface(), Priority: pw(loc.Priority), Weight: pw(loc.Weight)}
			s.Add(scheduler.Join(lisp.LocatorName, name+"/"+v.GetInterface()), v, lispPtr("locatorSets", name, "locators", i(j)))
		}
	}
	for j, e := range l.GetLocalEids() {
		c, ok := eid(e.GetEid(), "localEids", i(j), "eid")
		if !ok {
			continue
		}
		v := &lisp.LocalEid{Vni: e.GetVni(), Eid: c, LocatorSet: e.GetLocatorSet()}
		s.Add(scheduler.Join(lisp.LocalEidName, df6.U32(v.GetVni())+"/"+c), v, lispPtr("localEids", i(j)))
	}
	for _, k := range sortedKeys(l.GetEidTables()) {
		t := l.GetEidTables()[k]
		vni, err := strconv.ParseUint(k, 10, 32)
		if err != nil {
			s.Errorf(lispPtr("eidTables", k), "tunnels.lisp-eid-table", "VNI %q is not a number", k)
			continue
		}
		m := &lisp.EidTableMap{Vni: uint32(vni)}
		switch {
		case t.Vrf != nil && t.BridgeDomain == nil:
			id, ok := vrfID(t.GetVrf())
			if !ok {
				s.Errorf(lispPtr("eidTables", k, "vrf"), "tunnels.lisp-references", "VRF %q does not exist", t.GetVrf())
				continue
			}
			m.DpTable = id
		case t.BridgeDomain != nil && t.Vrf == nil:
			m.DpTable, m.IsL2 = t.GetBridgeDomain(), true
		default:
			s.Errorf(lispPtr("eidTables", k), "tunnels.lisp-references", "VNI %s needs exactly one of vrf or bridgeDomain", k)
			continue
		}
		s.Add(scheduler.Join(lisp.EidTableMapName, lisp.EidTableMapID(m)), m, lispPtr("eidTables", k))
	}
	for j, m := range l.GetRemoteMappings() {
		c, ok := eid(m.GetEid(), "remoteMappings", i(j), "eid")
		if !ok {
			continue
		}
		act, ok := lispAction(m.GetAction())
		if !ok {
			s.Errorf(lispPtr("remoteMappings", i(j), "action"), "tunnels.lisp-action", "unknown action %q", m.GetAction())
			continue
		}
		v := &lisp.RemoteMapping{Vni: m.GetVni(), Eid: c, Action: act}
		bad := false
		for k, r := range m.GetRlocs() {
			a, ok := addr(r.GetAddress(), "remoteMappings", i(j), "rlocs", i(k), "address")
			bad = bad || !ok
			v.Rlocs = append(v.Rlocs, &lisp.Rloc{Address: a, Priority: pw(r.Priority), Weight: pw(r.Weight)})
		}
		if bad {
			continue
		}
		sort.Slice(v.Rlocs, func(a, b int) bool { return v.Rlocs[a].GetAddress() < v.Rlocs[b].GetAddress() })
		s.Add(scheduler.Join(lisp.RemoteMappingName, df6.U32(v.GetVni())+"/"+c), v, lispPtr("remoteMappings", i(j)))
	}
	for j, a := range l.GetAdjacencies() {
		r, ok1 := eid(a.GetReid(), "adjacencies", i(j), "reid")
		le, ok2 := eid(a.GetLeid(), "adjacencies", i(j), "leid")
		if !ok1 || !ok2 {
			continue
		}
		v := &lisp.Adjacency{Vni: a.GetVni(), Reid: r, Leid: le}
		s.Add(scheduler.Join(lisp.AdjacencyName, df6.U32(v.GetVni())+"/"+r+"/"+le), v, lispPtr("adjacencies", i(j)))
	}
	for j, g := range l.GetGpeEntries() {
		r, ok1 := eid(g.GetReid(), "gpeEntries", i(j), "reid")
		le, ok2 := eid(g.GetLeid(), "gpeEntries", i(j), "leid")
		if !ok1 || !ok2 {
			continue
		}
		table, ok := vrfID(g.GetVrf())
		if !ok {
			s.Errorf(lispPtr("gpeEntries", i(j), "vrf"), "tunnels.lisp-references", "VRF %q does not exist", g.GetVrf())
			continue
		}
		act, ok := lispAction(g.GetAction())
		if !ok {
			s.Errorf(lispPtr("gpeEntries", i(j), "action"), "tunnels.lisp-action", "unknown action %q", g.GetAction())
			continue
		}
		v := &lisp.GpeFwdEntry{Vni: g.GetVni(), DpTable: table, Reid: r, Leid: le, Action: act}
		bad := false
		for k, p := range g.GetPairs() {
			lo, ok1 := addr(p.GetLocal(), "gpeEntries", i(j), "pairs", i(k), "local")
			rm, ok2 := addr(p.GetRemote(), "gpeEntries", i(j), "pairs", i(k), "remote")
			bad = bad || !ok1 || !ok2
			v.Pairs = append(v.Pairs, &lisp.LocatorPair{Local: lo, Remote: rm, Weight: pw(p.Weight)})
		}
		if bad {
			continue
		}
		sort.Slice(v.Pairs, func(a, b int) bool {
			return v.Pairs[a].GetLocal()+" "+v.Pairs[a].GetRemote() < v.Pairs[b].GetLocal()+" "+v.Pairs[b].GetRemote()
		})
		s.Add(scheduler.Join(lisp.GpeFwdEntryName, df6.U32(v.GetVni())+"/"+r+"/"+le), v, lispPtr("gpeEntries", i(j)))
	}
	for j, a := range l.GetMapResolvers() {
		if c, ok := addr(a, "mapResolvers", i(j)); ok {
			s.Add(scheduler.Join(lisp.MapResolverName, c), &lisp.MapResolver{Address: c}, lispPtr("mapResolvers", i(j)))
		}
	}
	for j, a := range l.GetMapServers() {
		if c, ok := addr(a, "mapServers", i(j)); ok {
			s.Add(scheduler.Join(lisp.MapServerName, c), &lisp.MapServer{Address: c}, lispPtr("mapServers", i(j)))
		}
	}
	if l.Pitr != nil {
		s.Add(scheduler.Join(lisp.PitrName, df6.SingletonID), &lisp.Pitr{LocatorSet: l.GetPitr()}, lispPtr("pitr"))
	}
}

// AssembleLisp sets ds.tunnels.lisp from retrieved objects (left unset when no LISP object is
// reported). nameOf maps a table id to its VRF name.
func AssembleLisp(ds *vrxv1.DesiredState, kvs []scheduler.KV, nameOf func(uint32) string) {
	if l := assembleLisp(kvs, nameOf); l != nil {
		if ds.Tunnels == nil {
			ds.Tunnels = &vrxv1.TunnelsConfig{}
		}
		ds.Tunnels.Lisp = l
	}
}

func assembleLisp(kvs []scheduler.KV, nameOf func(uint32) string) *vrxv1.LispConfig {
	l := &vrxv1.LispConfig{}
	globals := false
	sets := map[string]*vrxv1.LispLocatorSet{}
	set := func(n string) *vrxv1.LispLocatorSet {
		if sets[n] == nil {
			sets[n] = &vrxv1.LispLocatorSet{}
		}
		return sets[n]
	}
	objects := false
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case *lisp.Enable:
			l.Enabled, globals = proto.Bool(true), true
		case *lisp.GpeEnable:
			l.Gpe, globals = proto.Bool(true), true
		case *lisp.Pitr:
			l.Pitr, globals = proto.String(v.GetLocatorSet()), true
		case *lisp.LocatorSet:
			set(v.GetName())
			objects = true
		case *lisp.Locator:
			ls := set(v.GetLocatorSet())
			ls.Locators = append(ls.Locators, &vrxv1.LispLocator{Interface: proto.String(v.GetInterface()), Priority: proto.Uint32(v.GetPriority()), Weight: proto.Uint32(v.GetWeight())})
			objects = true
		case *lisp.LocalEid:
			l.LocalEids = append(l.LocalEids, &vrxv1.LispLocalEid{Vni: proto.Uint32(v.GetVni()), Eid: proto.String(v.GetEid()), LocatorSet: proto.String(v.GetLocatorSet())})
			objects = true
		case *lisp.EidTableMap:
			if l.EidTables == nil {
				l.EidTables = map[string]*vrxv1.LispEidTable{}
			}
			t := &vrxv1.LispEidTable{}
			if v.GetIsL2() {
				t.BridgeDomain = proto.Uint32(v.GetDpTable())
			} else {
				t.Vrf = proto.String(nameOf(v.GetDpTable()))
			}
			l.EidTables[df6.U32(v.GetVni())] = t
			objects = true
		case *lisp.RemoteMapping:
			m := &vrxv1.LispRemoteMapping{Vni: proto.Uint32(v.GetVni()), Eid: proto.String(v.GetEid()), Action: proto.String(lispActionName(v.GetAction()))}
			for _, r := range v.GetRlocs() {
				m.Rlocs = append(m.Rlocs, &vrxv1.LispRloc{Address: proto.String(r.GetAddress()), Priority: proto.Uint32(r.GetPriority()), Weight: proto.Uint32(r.GetWeight())})
			}
			l.RemoteMappings = append(l.RemoteMappings, m)
			objects = true
		case *lisp.Adjacency:
			l.Adjacencies = append(l.Adjacencies, &vrxv1.LispAdjacency{Vni: proto.Uint32(v.GetVni()), Reid: proto.String(v.GetReid()), Leid: proto.String(v.GetLeid())})
			objects = true
		case *lisp.MapResolver:
			l.MapResolvers = append(l.MapResolvers, v.GetAddress())
			objects = true
		case *lisp.MapServer:
			l.MapServers = append(l.MapServers, v.GetAddress())
			objects = true
		}
	}
	if !globals && !objects {
		return nil
	}
	if objects {
		l.Enabled = proto.Bool(true) // VPP holds no LISP object with LISP off
	}
	for n, ls := range sets {
		sort.Slice(ls.Locators, func(a, b int) bool { return ls.Locators[a].GetInterface() < ls.Locators[b].GetInterface() })
		if l.LocatorSets == nil {
			l.LocatorSets = map[string]*vrxv1.LispLocatorSet{}
		}
		l.LocatorSets[n] = ls
	}
	vk := func(vni uint32, s ...string) string { return df6.U32(vni) + "/" + strings.Join(s, "/") }
	sort.Slice(l.LocalEids, func(a, b int) bool {
		return vk(l.LocalEids[a].GetVni(), l.LocalEids[a].GetEid()) < vk(l.LocalEids[b].GetVni(), l.LocalEids[b].GetEid())
	})
	sort.Slice(l.RemoteMappings, func(a, b int) bool {
		return vk(l.RemoteMappings[a].GetVni(), l.RemoteMappings[a].GetEid()) < vk(l.RemoteMappings[b].GetVni(), l.RemoteMappings[b].GetEid())
	})
	sort.Slice(l.Adjacencies, func(a, b int) bool {
		x, y := l.Adjacencies[a], l.Adjacencies[b]
		return vk(x.GetVni(), x.GetReid(), x.GetLeid()) < vk(y.GetVni(), y.GetReid(), y.GetLeid())
	})
	sort.Strings(l.MapResolvers)
	sort.Strings(l.MapServers)
	return l
}
