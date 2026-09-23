package lisp_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	lispapi "ngfw/agent/binapi/lisp"
	gpeapi "ngfw/agent/binapi/lisp_gpe"
	"ngfw/agent/binapi/lisp_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/lisp"
	"ngfw/agent/internal/scheduler"
)

type mapping struct {
	vni    uint32
	eid    lisp_types.Eid
	lsIdx  uint32
	local  bool
	action uint8
}

type loc struct {
	local     bool
	sw        interface_types.InterfaceIndex
	addr      ip_types.Address
	prio, wgt uint8
}

// fakeLISP models the LISP control plane: enable flags, locator sets (local + the remote
// sets VPP creates per remote mapping), mappings, adjacencies, EID-table maps, resolvers,
// servers, PITR and GPE forwarding entries.
type fakeLISP struct {
	*df6test.FakeVPP
	on, gpe  bool
	sets     map[uint32]string
	locs     map[uint32][]loc
	nextSet  uint32
	mappings []*mapping
	adjs     map[uint32][]lispapi.LispAdjacency
	maps     map[[2]uint32]uint32 // {vni, is_l2} → dp table
	mr, ms   []ip_types.Address
	pitr     string
	fwd      []gpeapi.GpeAddDelFwdEntry
	tables   []uint32
}

// tablesIP4 makes IPv4 tables exist for RequireTable.
func (f *fakeLISP) tablesIP4(ids ...uint32) {
	f.tables = append(f.tables, ids...)
	f.On("ip_table_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, id := range f.tables {
			out = append(out, &ip.IPTableDetails{Table: ip.IPTable{TableID: id}})
		}
		return out, nil
	})
}

func b2u(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

func ok[T api.Message](m T) ([]api.Message, error) { return []api.Message{m}, nil }

func newFake() *fakeLISP {
	f := &fakeLISP{FakeVPP: df6test.NewFakeVPP(), sets: map[uint32]string{}, locs: map[uint32][]loc{}, adjs: map[uint32][]lispapi.LispAdjacency{}, maps: map[[2]uint32]uint32{}}
	f.On("lisp_enable_disable", func(req api.Message) ([]api.Message, error) {
		f.on = req.(*lispapi.LispEnableDisable).IsEnable
		f.gpe = f.on
		return ok(&lispapi.LispEnableDisableReply{})
	})
	f.On("gpe_enable_disable", func(req api.Message) ([]api.Message, error) {
		f.gpe = req.(*gpeapi.GpeEnableDisable).IsEnable
		return ok(&gpeapi.GpeEnableDisableReply{})
	})
	f.On("show_lisp_status", func(api.Message) ([]api.Message, error) {
		return ok(&lispapi.ShowLispStatusReply{IsLispEnabled: f.on, IsGpeEnabled: f.gpe})
	})
	setIdx := func(name string) (uint32, bool) {
		for i, n := range f.sets {
			if n == name {
				return i, true
			}
		}
		return 0, false
	}
	f.On("lisp_add_del_locator_set", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispAddDelLocatorSet)
		if !f.on {
			return ok(&lispapi.LispAddDelLocatorSetReply{Retval: -110})
		}
		i, exists := setIdx(r.LocatorSetName)
		if r.IsAdd {
			if !exists {
				i = f.nextSet
				f.nextSet++
				f.sets[i] = r.LocatorSetName
			}
			return ok(&lispapi.LispAddDelLocatorSetReply{LsIndex: i})
		}
		if !exists {
			return ok(&lispapi.LispAddDelLocatorSetReply{Retval: -1})
		}
		delete(f.sets, i)
		delete(f.locs, i)
		return ok(&lispapi.LispAddDelLocatorSetReply{})
	})
	f.On("lisp_locator_set_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for i, n := range f.sets {
			out = append(out, &lispapi.LispLocatorSetDetails{LsIndex: i, LsName: n})
		}
		return out, nil
	})
	f.On("lisp_add_del_locator", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispAddDelLocator)
		i, exists := setIdx(r.LocatorSetName)
		if !exists {
			return ok(&lispapi.LispAddDelLocatorReply{Retval: -1})
		}
		if r.IsAdd {
			f.locs[i] = append(f.locs[i], loc{local: true, sw: r.SwIfIndex, prio: r.Priority, wgt: r.Weight})
		} else {
			var keep []loc
			for _, l := range f.locs[i] {
				if l.sw != r.SwIfIndex {
					keep = append(keep, l)
				}
			}
			f.locs[i] = keep
		}
		return ok(&lispapi.LispAddDelLocatorReply{})
	})
	f.On("lisp_locator_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispLocatorDump)
		var out []api.Message
		for _, l := range f.locs[r.LsIndex] {
			d := &lispapi.LispLocatorDetails{SwIfIndex: l.sw, IPAddress: l.addr, Priority: l.prio, Weight: l.wgt}
			if l.local {
				d.Local = 1
			}
			out = append(out, d)
		}
		return out, nil
	})
	f.On("lisp_eid_table_add_del_map", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispEidTableAddDelMap)
		k := [2]uint32{r.Vni, b2u(r.IsL2)}
		if r.IsAdd {
			f.maps[k] = r.DpTable
		} else {
			delete(f.maps, k)
		}
		return ok(&lispapi.LispEidTableAddDelMapReply{})
	})
	f.On("lisp_eid_table_map_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispEidTableMapDump)
		out := []api.Message{&lispapi.LispEidTableMapDetails{Vni: 0, DpTable: 0}}
		for k, t := range f.maps {
			if k[1] == b2u(r.IsL2) {
				out = append(out, &lispapi.LispEidTableMapDetails{Vni: k[0], DpTable: t})
			}
		}
		return out, nil
	})
	find := func(vni uint32, eid lisp_types.Eid) int {
		for i, m := range f.mappings {
			if m.vni == vni && m.eid == eid {
				return i
			}
		}
		return -1
	}
	f.On("lisp_add_del_local_eid", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispAddDelLocalEid)
		i := find(r.Vni, r.Eid)
		if r.IsAdd {
			idx, exists := setIdx(r.LocatorSetName)
			if !exists || i >= 0 {
				return ok(&lispapi.LispAddDelLocalEidReply{Retval: -1})
			}
			f.mappings = append(f.mappings, &mapping{vni: r.Vni, eid: r.Eid, lsIdx: idx, local: true})
		} else if i >= 0 {
			f.mappings = append(f.mappings[:i], f.mappings[i+1:]...)
		}
		return ok(&lispapi.LispAddDelLocalEidReply{})
	})
	f.On("lisp_add_del_remote_mapping", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispAddDelRemoteMapping)
		i := find(r.Vni, r.Deid)
		if r.IsAdd {
			m := &mapping{vni: r.Vni, eid: r.Deid, action: r.Action, lsIdx: ^uint32(0)}
			if len(r.Rlocs) > 0 {
				m.lsIdx = 1000 + f.nextSet // remote sets are not in the local dump
				f.nextSet++
				for _, rl := range r.Rlocs {
					f.locs[m.lsIdx] = append(f.locs[m.lsIdx], loc{addr: rl.IPAddress, prio: rl.Priority, wgt: rl.Weight})
				}
			}
			f.mappings = append(f.mappings, m)
		} else if i >= 0 {
			f.mappings = append(f.mappings[:i], f.mappings[i+1:]...)
		}
		return ok(&lispapi.LispAddDelRemoteMappingReply{})
	})
	f.On("lisp_eid_table_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispEidTableDump)
		var out []api.Message
		for _, m := range f.mappings {
			if (r.Filter == lispapi.LISP_LOCATOR_SET_FILTER_API_LOCAL) != m.local {
				continue
			}
			idx := m.lsIdx
			if len(f.locs[idx]) == 0 {
				idx = ^uint32(0) // VPP reports ~0 for a locator set without locators
			}
			out = append(out, &lispapi.LispEidTableDetails{LocatorSetIndex: idx, Action: m.action, IsLocal: m.local, Vni: m.vni, Seid: m.eid})
		}
		return out, nil
	})
	f.On("lisp_eid_table_vni_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, m := range f.mappings {
			out = append(out, &lispapi.LispEidTableVniDetails{Vni: m.vni})
		}
		return out, nil
	})
	f.On("lisp_add_del_adjacency", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispAddDelAdjacency)
		var keep []lispapi.LispAdjacency
		for _, a := range f.adjs[r.Vni] {
			if a.Reid != r.Reid || a.Leid != r.Leid {
				keep = append(keep, a)
			}
		}
		if r.IsAdd {
			keep = append(keep, lispapi.LispAdjacency{Reid: r.Reid, Leid: r.Leid})
		}
		f.adjs[r.Vni] = keep
		return ok(&lispapi.LispAddDelAdjacencyReply{})
	})
	f.On("lisp_adjacencies_get", func(req api.Message) ([]api.Message, error) {
		return ok(&lispapi.LispAdjacenciesGetReply{Adjacencies: f.adjs[req.(*lispapi.LispAdjacenciesGet).Vni]})
	})
	addr := func(list *[]ip_types.Address, a ip_types.Address, add bool) {
		var keep []ip_types.Address
		for _, x := range *list {
			if x != a {
				keep = append(keep, x)
			}
		}
		if add {
			keep = append(keep, a)
		}
		*list = keep
	}
	f.On("lisp_add_del_map_resolver", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispAddDelMapResolver)
		addr(&f.mr, r.IPAddress, r.IsAdd)
		return ok(&lispapi.LispAddDelMapResolverReply{})
	})
	f.On("lisp_add_del_map_server", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispAddDelMapServer)
		addr(&f.ms, r.IPAddress, r.IsAdd)
		return ok(&lispapi.LispAddDelMapServerReply{})
	})
	f.On("lisp_map_resolver_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, a := range f.mr {
			out = append(out, &lispapi.LispMapResolverDetails{IPAddress: a})
		}
		return out, nil
	})
	f.On("lisp_map_server_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, a := range f.ms {
			out = append(out, &lispapi.LispMapServerDetails{IPAddress: a})
		}
		return out, nil
	})
	f.On("lisp_pitr_set_locator_set", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispPitrSetLocatorSet)
		if r.IsAdd {
			f.pitr = r.LsName
		} else {
			f.pitr = ""
		}
		return ok(&lispapi.LispPitrSetLocatorSetReply{})
	})
	f.On("show_lisp_pitr", func(api.Message) ([]api.Message, error) {
		return ok(&lispapi.ShowLispPitrReply{IsEnabled: f.pitr != "", LocatorSetName: f.pitr})
	})
	f.On("gpe_add_del_fwd_entry", func(req api.Message) ([]api.Message, error) {
		r := req.(*gpeapi.GpeAddDelFwdEntry)
		if r.IsAdd {
			for _, e := range f.fwd {
				if e.Vni == r.Vni && e.RmtEid == r.RmtEid && e.LclEid == r.LclEid {
					return ok(&gpeapi.GpeAddDelFwdEntryReply{Retval: -7}) // "don't support updates"
				}
			}
		}
		var keep []gpeapi.GpeAddDelFwdEntry
		for _, e := range f.fwd {
			if e.Vni != r.Vni || e.RmtEid != r.RmtEid || e.LclEid != r.LclEid {
				keep = append(keep, e)
			}
		}
		if r.IsAdd {
			keep = append(keep, *r)
		}
		f.fwd = keep
		return ok(&gpeapi.GpeAddDelFwdEntryReply{})
	})
	f.On("gpe_fwd_entry_vnis_get", func(api.Message) ([]api.Message, error) {
		rep := &gpeapi.GpeFwdEntryVnisGetReply{}
		for _, e := range f.fwd {
			rep.Vnis = append(rep.Vnis, e.Vni)
		}
		return ok(rep)
	})
	f.On("gpe_fwd_entries_get", func(req api.Message) ([]api.Message, error) {
		rep := &gpeapi.GpeFwdEntriesGetReply{}
		for i, e := range f.fwd {
			if e.Vni == req.(*gpeapi.GpeFwdEntriesGet).Vni {
				rep.Entries = append(rep.Entries, gpeapi.GpeFwdEntry{FwdEntryIndex: uint32(i), DpTable: e.DpTable, Leid: e.LclEid, Reid: e.RmtEid, Vni: e.Vni, Action: e.Action}) //nolint:gosec // small
			}
		}
		return ok(rep)
	})
	f.On("gpe_fwd_entry_path_dump", func(req api.Message) ([]api.Message, error) {
		e := f.fwd[req.(*gpeapi.GpeFwdEntryPathDump).FwdEntryIndex]
		n := len(e.Locs) / 2
		var out []api.Message
		for i := 0; i < n; i++ {
			out = append(out, &gpeapi.GpeFwdEntryPathDetails{LclLoc: e.Locs[i], RmtLoc: e.Locs[n+i]})
		}
		return out, nil
	})
	return f
}

func mustRetrieve(t *testing.T, d scheduler.Descriptor) map[scheduler.Key]proto.Message {
	t.Helper()
	kvs, err := d.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("%s Retrieve: %v", d.Name(), err)
	}
	m := map[scheduler.Key]proto.Message{}
	for _, kv := range kvs {
		m[kv.Key] = kv.Value
	}
	return m
}

func TestLISP(t *testing.T) {
	ctx := context.Background()
	f := newFake()
	sc := "w11" + t.Name() // owner
	iface.SetClaimStore(sc, nil)
	loop := f.AddInterface("loop1114", sc+":loop1114")
	reg := scheduler.NewRegistry()
	lisp.Register(reg, f, sc, df6.WithGlobalsOwner(true))
	if reg.Len() != 12 {
		t.Fatalf("registered %d", reg.Len())
	}
	en := lisp.NewEnable(f)
	if got := mustRetrieve(t, en); len(got) != 0 {
		t.Fatalf("disabled LISP retrieved as %v", got)
	}
	if _, err := en.Create(ctx, &lisp.Enable{}); err != nil {
		t.Fatal(err)
	}
	if got := mustRetrieve(t, en); len(got) != 1 || got[lisp.EnableKey] == nil {
		t.Fatalf("enable Retrieve = %v", got)
	}
	if got := mustRetrieve(t, lisp.NewGpeEnable(f)); len(got) != 1 {
		t.Fatalf("gpe follows lisp enable: %v", got)
	}

	// Another slot's locator set stays invisible.
	f.sets[99] = "w3-ls"
	f.nextSet = 100
	type step struct {
		d    scheduler.Descriptor
		obj  proto.Message
		key  scheduler.Key
		deps []scheduler.Key
	}
	steps := []step{
		{lisp.NewLocatorSet(f, sc), &lisp.LocatorSet{Name: "w11-ls1"}, "lisp.locator-set/w11-ls1", []scheduler.Key{lisp.EnableKey}},
		{lisp.NewLocator(f, sc), &lisp.Locator{LocatorSet: "w11-ls1", Interface: "loop1114", Priority: 1, Weight: 10}, "lisp.locator/w11-ls1/loop1114", []scheduler.Key{"lisp.locator-set/w11-ls1", "interface/loop1114"}},
		{lisp.NewEidTableMap(f, sc), &lisp.EidTableMap{Vni: 11100, DpTable: 11013}, "lisp.eid-table-map/l3/11100", []scheduler.Key{"vrf/11013"}},
		{lisp.NewEidTableMap(f, sc), &lisp.EidTableMap{Vni: 11101, DpTable: 11001, IsL2: true}, "lisp.eid-table-map/l2/11101", []scheduler.Key{"bridge-domain/11001"}},
		{lisp.NewLocalEid(f, sc), &lisp.LocalEid{Vni: 11100, Eid: "10.11.14.0/24", LocatorSet: "w11-ls1"}, "lisp.local-eid/11100/10.11.14.0/24", []scheduler.Key{"lisp.locator-set/w11-ls1", lisp.EnableKey, "lisp.eid-table-map/l3/11100"}},
		{lisp.NewLocalEid(f, sc), &lisp.LocalEid{Vni: 11101, Eid: "02:0b:00:00:00:01", LocatorSet: "w11-ls1"}, "lisp.local-eid/11101/02:0b:00:00:00:01", []scheduler.Key{"lisp.locator-set/w11-ls1", lisp.EnableKey, "lisp.eid-table-map/l2/11101"}},
		{lisp.NewMapResolver(f, sc), &lisp.MapResolver{Address: "10.11.14.100"}, "lisp.map-resolver/10.11.14.100", []scheduler.Key{lisp.EnableKey}},
		{lisp.NewMapServer(f, sc), &lisp.MapServer{Address: "fd11:14::100"}, "lisp.map-server/fd11:14::100", []scheduler.Key{lisp.EnableKey}},
		{lisp.NewRemoteMapping(f, sc), &lisp.RemoteMapping{Vni: 11100, Eid: "10.11.15.0/24", Rlocs: []*lisp.Rloc{{Address: "10.11.14.2", Priority: 1, Weight: 1}, {Address: "10.11.14.3", Priority: 2, Weight: 5}}}, "lisp.remote-mapping/11100/10.11.15.0/24", []scheduler.Key{lisp.EnableKey, "lisp.eid-table-map/l3/11100"}},
		{lisp.NewRemoteMapping(f, sc), &lisp.RemoteMapping{Vni: 11100, Eid: "10.11.16.0/24", Action: 3}, "lisp.remote-mapping/11100/10.11.16.0/24", []scheduler.Key{lisp.EnableKey, "lisp.eid-table-map/l3/11100"}},
		{lisp.NewAdjacency(f, sc), &lisp.Adjacency{Vni: 11100, Reid: "10.11.15.0/24", Leid: "10.11.14.0/24"}, "lisp.adjacency/11100/10.11.15.0/24/10.11.14.0/24", []scheduler.Key{"lisp.remote-mapping/11100/10.11.15.0/24", "lisp.local-eid/11100/10.11.14.0/24"}},
		{lisp.NewPitr(f), &lisp.Pitr{LocatorSet: "w11-ls1"}, "lisp.pitr/global", []scheduler.Key{"lisp.locator-set/w11-ls1", lisp.EnableKey}},
		{lisp.NewGpeFwdEntry(f, sc), &lisp.GpeFwdEntry{Vni: 11100, DpTable: 11013, Reid: "10.11.17.0/24", Leid: "10.11.14.0/24", Pairs: []*lisp.LocatorPair{{Local: "10.11.14.1", Remote: "10.11.14.9", Weight: 1}}}, "lisp-gpe.fwd-entry/11100/10.11.17.0/24/10.11.14.0/24", []scheduler.Key{lisp.GpeEnableKey, "vrf/11013"}},
		{lisp.NewGpeFwdEntry(f, sc), &lisp.GpeFwdEntry{Vni: 11100, Reid: "10.11.18.0/24", Leid: "10.11.14.0/24", Action: 3}, "lisp-gpe.fwd-entry/11100/10.11.18.0/24/10.11.14.0/24", []scheduler.Key{lisp.GpeEnableKey}},
	}
	f.tablesIP4(11013)
	for _, s := range steps {
		if k := s.d.KeyOf(s.obj); k != s.key {
			t.Errorf("KeyOf = %s, want %s", k, s.key)
		}
		var deps []scheduler.Key
		for _, d := range s.d.Dependencies(s.obj) {
			deps = append(deps, d.Key)
		}
		if !equalKeys(deps, s.deps) {
			t.Errorf("%s deps = %v, want %v", s.key, deps, s.deps)
		}
		if _, err := s.d.Create(ctx, s.obj); err != nil {
			t.Fatalf("create %s: %v", s.key, err)
		}
		if !observed(t, s.d, s.obj) {
			t.Fatalf("%s not observed after create", s.key)
		}
	}
	// Resync (review H3): re-applying every object — write-only GPE entries included — is a
	// no-op that sends no add.
	adds := len(f.CallsNamed("gpe_add_del_fwd_entry"))
	for _, s := range steps {
		if _, err := s.d.Create(ctx, s.obj); err != nil {
			t.Fatalf("resync %s: %v", s.key, err)
		}
	}
	if len(f.CallsNamed("gpe_add_del_fwd_entry")) != adds {
		t.Fatal("resync re-sent gpe_add_del_fwd_entry")
	}
	if got := mustRetrieve(t, steps[0].d); len(got) != 1 {
		t.Fatalf("locator sets = %v (other slot's must be filtered)", got)
	}
	// Canonicalisation: unsorted rlocs / unmasked prefixes produce the same key and value.
	rm := steps[8].d
	messy := &lisp.RemoteMapping{Vni: 11100, Eid: "10.11.15.7/24", Rlocs: []*lisp.Rloc{{Address: "10.11.14.3", Priority: 2, Weight: 5}, {Address: "10.11.14.2", Priority: 1, Weight: 1}}}
	if rm.KeyOf(messy) != steps[8].key {
		t.Fatalf("messy key = %s", rm.KeyOf(messy))
	}
	// Delete in reverse order, twice each (second is a no-op).
	for i := len(steps) - 1; i >= 0; i-- {
		for j := 0; j < 2; j++ {
			if err := steps[i].d.Delete(ctx, steps[i].obj, nil); err != nil {
				t.Fatalf("delete %s: %v", steps[i].key, err)
			}
		}
		if observedAny(t, steps[i].d, steps[i].obj) {
			t.Fatalf("%s still present", steps[i].key)
		}
	}
	// D-071: another owner's locator set still exists → the globals owner leaves LISP on.
	if err := en.Delete(ctx, &lisp.Enable{}, nil); err != nil || !f.on {
		t.Fatalf("disable while another owner uses LISP: %v on=%v", err, f.on)
	}
	delete(f.sets, 99)
	if err := en.Delete(ctx, &lisp.Enable{}, nil); err != nil || f.on {
		t.Fatalf("disable: %v", err)
	}
	if en.DeleteOnAbsence() {
		t.Fatal("lisp.enable must never be deleted on absence (review H1)")
	}
	// Non-owners: require variants only — never enable, never disable.
	reg2 := scheduler.NewRegistry()
	lisp.Register(reg2, f, sc)
	for _, name := range []string{lisp.EnableName, lisp.GpeEnableName, lisp.PitrName} {
		d, _ := reg2.Get(name)
		if _, ok := d.(*df6.RequireDescriptor[*lisp.Enable]); name == lisp.EnableName && !ok {
			t.Fatalf("%s: non-owner got %T", name, d)
		}
	}
	req, _ := reg2.Get(lisp.EnableName)
	calls := len(f.CallsNamed("lisp_enable_disable"))
	if _, err := req.Create(ctx, &lisp.Enable{}); !errors.Is(err, df6.ErrNotGlobalsOwner) {
		t.Fatalf("require with LISP off = %v", err)
	}
	f.on = true
	if _, err := req.Create(ctx, &lisp.Enable{}); err != nil {
		t.Fatalf("require with LISP on = %v", err)
	}
	if err := req.Delete(ctx, &lisp.Enable{}, nil); err != nil || !f.on {
		t.Fatalf("non-owner delete must be a no-op: %v", err)
	}
	if len(f.CallsNamed("lisp_enable_disable")) != calls {
		t.Fatal("non-owner sent lisp_enable_disable")
	}
	for _, b := range []struct {
		d   scheduler.Descriptor
		obj proto.Message
	}{
		{steps[0].d, &lisp.LocatorSet{}},
		{steps[1].d, &lisp.Locator{LocatorSet: "w11-ls1", Interface: "loop1114", Priority: 256}},
		{steps[2].d, &lisp.EidTableMap{Vni: 0, DpTable: 1}},
		{steps[4].d, &lisp.LocalEid{Vni: 1, Eid: "not-an-eid", LocatorSet: "w11-ls1"}},
		{steps[8].d, &lisp.RemoteMapping{Vni: 1, Eid: "10.0.0.0/8", Action: 3, Rlocs: []*lisp.Rloc{{Address: "10.0.0.1"}}}},
		{steps[12].d, &lisp.GpeFwdEntry{Reid: "10.0.0.0/8", Pairs: []*lisp.LocatorPair{{Local: "10.0.0.1", Remote: "fd11::1"}}}},
	} {
		if _, err := b.d.Create(ctx, b.obj); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("%s %v: %v", b.d.Name(), b.obj, err)
		}
	}
	_ = loop
}

func equalKeys(a, b []scheduler.Key) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// presence is implemented by the write-only descriptors (lisp-gpe.fwd-entry).
type presence interface {
	Present(ctx context.Context, obj proto.Message) (bool, error)
}

// observed: Retrieve returns obj exactly, or — for a write-only descriptor — Present says
// it exists.
func observed(t *testing.T, d scheduler.Descriptor, obj proto.Message) bool {
	t.Helper()
	kvs, err := d.Retrieve(context.Background())
	if errors.Is(err, df6.ErrRetrieveUnsupported) {
		ok, perr := d.(presence).Present(context.Background(), obj)
		if perr != nil {
			t.Fatal(perr)
		}
		return ok
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range kvs {
		if kv.Key == d.KeyOf(obj) {
			if !proto.Equal(kv.Value, obj) {
				t.Errorf("Retrieve %s = %v, want %v", kv.Key, kv.Value, obj)
			}
			return true
		}
	}
	return false
}

// observedAny: obj's key is still in VPP (by Retrieve or Present).
func observedAny(t *testing.T, d scheduler.Descriptor, obj proto.Message) bool {
	t.Helper()
	kvs, err := d.Retrieve(context.Background())
	if errors.Is(err, df6.ErrRetrieveUnsupported) {
		ok, perr := d.(presence).Present(context.Background(), obj)
		if perr != nil {
			t.Fatal(perr)
		}
		return ok
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range kvs {
		if kv.Key == d.KeyOf(obj) {
			return true
		}
	}
	return false
}
