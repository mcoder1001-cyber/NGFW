package desired

// F-vrf-static-ecmp builder and assembler (wave-A-hotspots A2: projection.go calls them under its anchors).
//
//	vrfs.<vrf>.sourceSelect[k]{prefix, interface}  → svs.table/<id>             one per ingress interface (id: svs.Allocate)
//	                                               → svs.interface/<interface>  (svs_enable_disable IPv4 + IPv6)
//	                                               → svs.route/<id>/<prefix>    source table = <vrf>'s table id
//	routing.static[i].viaFrr = true                → nothing (D-072: FRR programs it; warning agent.unsupported-field)
//
// The weighted ECMP paths, blackhole and the next-hop VRF of routing.static are projected by P08's routing.static block in
// projection.go (the next-hop VRF lines are this feature's).

import (
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/svs"
	"ngfw/agent/internal/scheduler"
)

// VrfStaticEcmp projects the feature's leaves of ds for the domains in `in`: `vrfs` → svs objects, `routing` → the
// D-072 notice for viaFrr routes. vrfID resolves a VRF name to its table id; rng is where svs table ids come from.
func VrfStaticEcmp(s Sink, ds *vrxv1.DesiredState, in map[string]bool, vrfID func(string) (uint32, bool), rng svs.Range) {
	if in["routing"] {
		for i, r := range ds.GetRouting().GetStatic() {
			if r.GetViaFrr() {
				s.Warnf(Ptr("routing", "static", strconv.Itoa(i)), "agent.unsupported-field",
					"routing.static[%d] (%s) is programmed by FRR (viaFrr, D-072), not by the agent; FRR rendering is P12's", i, r.GetPrefix())
			}
		}
	}
	if in["vrfs"] {
		sourceSelect(s, ds, vrfID, rng)
	}
}

type selectEntry struct {
	vrf, iface, prefix string
	table              uint32
	pointer            string
}

func sourceSelect(s Sink, ds *vrxv1.DesiredState, vrfID func(string) (uint32, bool), rng svs.Range) {
	var entries []selectEntry
	firstPtr := map[string]string{} // interface → pointer of its first entry
	for _, name := range sortedKeys(ds.GetVrfs()) {
		for k, e := range ds.GetVrfs()[name].GetSourceSelect() {
			pt := Ptr("vrfs", name, "sourceSelect", strconv.Itoa(k))
			prefix, err := svs.CanonPrefix(e.GetPrefix())
			if err != nil {
				s.Errorf(pt+"/prefix", "vrfs.source-select.prefix", "%v", err)
				continue
			}
			if e.GetInterface() == "" {
				s.Errorf(pt+"/interface", "vrfs.source-select.interface", "a source-VRF-select entry needs an ingress interface")
				continue
			}
			table, ok := vrfID(name)
			if !ok {
				s.Errorf(Ptr("vrfs", name, "id"), "vrfs.id-required", "VRF %q has no table id", name)
				continue
			}
			entries = append(entries, selectEntry{vrf: name, iface: e.GetInterface(), prefix: prefix, table: table, pointer: pt})
			if _, seen := firstPtr[e.GetInterface()]; !seen {
				firstPtr[e.GetInterface()] = pt
			}
		}
	}
	if len(entries) == 0 {
		return
	}
	declared := map[uint32]bool{}
	for _, v := range ds.GetVrfs() {
		if v.Id != nil {
			declared[v.GetId()] = true
		}
	}
	ifaces := make([]string, 0, len(firstPtr))
	for n := range firstPtr {
		ifaces = append(ifaces, n)
	}
	ids, err := svs.Allocate(ifaces, rng, func(id uint32) bool { return declared[id] })
	if err != nil {
		s.Errorf(Ptr("vrfs"), "vrfs.source-select.tables", "%v", err)
		return
	}
	sort.Strings(ifaces)
	for _, n := range ifaces {
		s.Add(svs.TableKey(ids[n]), &svs.Table{Id: ids[n]}, firstPtr[n])
		s.Add(svs.InterfaceKey(n), &svs.Interface{Interface: n, TableId: ids[n]}, firstPtr[n])
	}
	for _, e := range entries {
		id := ids[e.iface]
		s.Add(svs.RouteKey(id, e.prefix), &svs.Route{TableId: id, Prefix: e.prefix, SourceTableId: e.table}, e.pointer)
	}
}

// AssembleVrfStaticEcmp adds `vrfs.<vrf>.sourceSelect` to ds from the retrieved svs objects: an svs route belongs to the
// interface whose enablement uses its table and to the VRF of its selected table (nameOf). Entries whose selected table
// is unknown (no applied-once record for the running VPP) are left out: they are re-programmed by the next reconcile.
func AssembleVrfStaticEcmp(ds *vrxv1.DesiredState, kvs []scheduler.KV, nameOf func(id uint32) string) {
	ifaceOf := map[uint32]string{}
	for _, kv := range kvs {
		if v, ok := kv.Value.(*svs.Interface); ok {
			ifaceOf[v.GetTableId()] = v.GetInterface()
		}
	}
	for _, kv := range kvs {
		r, ok := kv.Value.(*svs.Route)
		if !ok || r.GetSourceTableId() == svs.UnknownTable {
			continue
		}
		n, ok := ifaceOf[r.GetTableId()]
		if !ok {
			continue // a table entry without an enabled interface has no configuration form
		}
		vrf := nameOf(r.GetSourceTableId())
		if ds.Vrfs == nil {
			ds.Vrfs = map[string]*vrxv1.Vrf{}
		}
		v := ds.Vrfs[vrf]
		if v == nil {
			v = &vrxv1.Vrf{}
			if r.GetSourceTableId() == 0 {
				v.Id = proto.Uint32(0) // `default`: VPP's table 0 always exists
			}
			ds.Vrfs[vrf] = v
		}
		v.SourceSelect = append(v.SourceSelect, &vrxv1.VrfSourceSelect{Prefix: proto.String(r.GetPrefix()), Interface: proto.String(n)})
	}
	for _, v := range ds.GetVrfs() {
		sort.Slice(v.SourceSelect, func(i, j int) bool {
			a, b := v.SourceSelect[i], v.SourceSelect[j]
			if a.GetInterface() != b.GetInterface() {
				return a.GetInterface() < b.GetInterface()
			}
			return a.GetPrefix() < b.GetPrefix()
		})
	}
}
