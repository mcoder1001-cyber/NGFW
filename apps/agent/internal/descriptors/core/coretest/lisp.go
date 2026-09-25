package coretest

// F-lisp extension of the model: the LISP / LISP-GPE control plane (installed on demand with
// InstallLisp; the default model has no lisp plugin, like a VPP that does not load it).

import (
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	lispapi "ngfw/agent/binapi/lisp"
	gpeapi "ngfw/agent/binapi/lisp_gpe"
	"ngfw/agent/binapi/lisp_types"
)

// handle registers a handler that runs under f.mu.
func (f *Lisp) handle(name string, h func(api.Message) ([]api.Message, error)) {
	f.On(name, func(m api.Message) ([]api.Message, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return h(m)
	})
}

// State summarises the model for tests: locator sets (local and leaked remote), mappings, adjacencies,
// EID-table maps, GPE entries.
func (f *Lisp) State() (sets, mappings, adjacencies, maps, fwd int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.adjs {
		adjacencies += len(a)
	}
	return len(f.sets), len(f.mappings), adjacencies, len(f.maps), len(f.fwd)
}

// RemoteSets is the number of remote locator sets VPP created for remote mappings (never freed, V14).
func (f *Lisp) RemoteSets() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for i := range f.locs {
		if i >= 1000 {
			n++
		}
	}
	return n
}

// ForgetGpe drops every GPE forwarding entry (a VPP restart as seen by the write-only descriptor).
func (f *Lisp) ForgetGpe() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fwd = nil
}

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

// Lisp models the LISP control plane: enable flags, locator sets (local + the remote
// sets VPP creates per remote mapping), mappings, adjacencies, EID-table maps, resolvers,
// servers, PITR and GPE forwarding entries.
type Lisp struct {
	*VPP
	mu             sync.Mutex
	Enabled, GpeOn bool
	sets           map[uint32]string
	locs           map[uint32][]loc
	nextSet        uint32
	mappings       []*mapping
	adjs           map[uint32][]lispapi.LispAdjacency
	maps           map[[2]uint32]uint32 // {vni, is_l2} → dp table
	mr, ms         []ip_types.Address
	pitr           string
	fwd            []gpeapi.GpeAddDelFwdEntry
}

func b2u(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

func ok(m api.Message) ([]api.Message, error) { return []api.Message{m}, nil }

// InstallLisp adds the LISP / LISP-GPE control plane to the model (F-lisp): enable flags, local locator
// sets and the remote ones VPP creates per remote mapping (never deleted, V14), mappings, adjacencies,
// EID-table maps, resolvers, servers, PITR and GPE forwarding entries. Ported from DF-6's unit-test fake.
// Every handler holds f.mu.
func (v *VPP) InstallLisp() *Lisp {
	f := &Lisp{VPP: v, sets: map[uint32]string{}, locs: map[uint32][]loc{}, adjs: map[uint32][]lispapi.LispAdjacency{}, maps: map[[2]uint32]uint32{}}
	f.handle("lisp_enable_disable", func(req api.Message) ([]api.Message, error) {
		f.Enabled = req.(*lispapi.LispEnableDisable).IsEnable
		f.GpeOn = f.Enabled
		return ok(&lispapi.LispEnableDisableReply{})
	})
	f.handle("gpe_enable_disable", func(req api.Message) ([]api.Message, error) {
		f.GpeOn = req.(*gpeapi.GpeEnableDisable).IsEnable
		return ok(&gpeapi.GpeEnableDisableReply{})
	})
	f.handle("show_lisp_status", func(api.Message) ([]api.Message, error) {
		return ok(&lispapi.ShowLispStatusReply{IsLispEnabled: f.Enabled, IsGpeEnabled: f.GpeOn})
	})
	setIdx := func(name string) (uint32, bool) {
		for i, n := range f.sets {
			if n == name {
				return i, true
			}
		}
		return 0, false
	}
	f.handle("lisp_add_del_locator_set", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispAddDelLocatorSet)
		if !f.Enabled {
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
	f.handle("lisp_locator_set_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for i, n := range f.sets {
			out = append(out, &lispapi.LispLocatorSetDetails{LsIndex: i, LsName: n})
		}
		return out, nil
	})
	f.handle("lisp_add_del_locator", func(req api.Message) ([]api.Message, error) {
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
	f.handle("lisp_locator_dump", func(req api.Message) ([]api.Message, error) {
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
	f.handle("lisp_eid_table_add_del_map", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispEidTableAddDelMap)
		k := [2]uint32{r.Vni, b2u(r.IsL2)}
		if r.IsAdd {
			f.maps[k] = r.DpTable
		} else {
			delete(f.maps, k)
		}
		return ok(&lispapi.LispEidTableAddDelMapReply{})
	})
	f.handle("lisp_eid_table_map_dump", func(req api.Message) ([]api.Message, error) {
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
	f.handle("lisp_add_del_local_eid", func(req api.Message) ([]api.Message, error) {
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
	f.handle("lisp_add_del_remote_mapping", func(req api.Message) ([]api.Message, error) {
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
	f.handle("lisp_eid_table_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispEidTableDump)
		var out []api.Message
		for _, m := range f.mappings {
			if r.Filter != lispapi.LISP_LOCATOR_SET_FILTER_API_ALL && (r.Filter == lispapi.LISP_LOCATOR_SET_FILTER_API_LOCAL) != m.local {
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
	f.handle("lisp_eid_table_vni_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, m := range f.mappings {
			out = append(out, &lispapi.LispEidTableVniDetails{Vni: m.vni})
		}
		return out, nil
	})
	f.handle("lisp_add_del_adjacency", func(req api.Message) ([]api.Message, error) {
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
	f.handle("lisp_adjacencies_get", func(req api.Message) ([]api.Message, error) {
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
	f.handle("lisp_add_del_map_resolver", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispAddDelMapResolver)
		addr(&f.mr, r.IPAddress, r.IsAdd)
		return ok(&lispapi.LispAddDelMapResolverReply{})
	})
	f.handle("lisp_add_del_map_server", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispAddDelMapServer)
		addr(&f.ms, r.IPAddress, r.IsAdd)
		return ok(&lispapi.LispAddDelMapServerReply{})
	})
	f.handle("lisp_map_resolver_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, a := range f.mr {
			out = append(out, &lispapi.LispMapResolverDetails{IPAddress: a})
		}
		return out, nil
	})
	f.handle("lisp_map_server_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, a := range f.ms {
			out = append(out, &lispapi.LispMapServerDetails{IPAddress: a})
		}
		return out, nil
	})
	f.handle("lisp_pitr_set_locator_set", func(req api.Message) ([]api.Message, error) {
		r := req.(*lispapi.LispPitrSetLocatorSet)
		if r.IsAdd {
			f.pitr = r.LsName
		} else {
			f.pitr = ""
		}
		return ok(&lispapi.LispPitrSetLocatorSetReply{})
	})
	f.handle("show_lisp_pitr", func(api.Message) ([]api.Message, error) {
		return ok(&lispapi.ShowLispPitrReply{IsEnabled: f.pitr != "", LocatorSetName: f.pitr})
	})
	f.handle("gpe_add_del_fwd_entry", func(req api.Message) ([]api.Message, error) {
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
	f.handle("gpe_fwd_entry_vnis_get", func(api.Message) ([]api.Message, error) {
		rep := &gpeapi.GpeFwdEntryVnisGetReply{}
		for _, e := range f.fwd {
			rep.Vnis = append(rep.Vnis, e.Vni)
		}
		return ok(rep)
	})
	f.handle("gpe_fwd_entries_get", func(req api.Message) ([]api.Message, error) {
		rep := &gpeapi.GpeFwdEntriesGetReply{}
		for i, e := range f.fwd {
			if e.Vni == req.(*gpeapi.GpeFwdEntriesGet).Vni {
				rep.Entries = append(rep.Entries, gpeapi.GpeFwdEntry{FwdEntryIndex: uint32(i), DpTable: e.DpTable, Leid: e.LclEid, Reid: e.RmtEid, Vni: e.Vni, Action: e.Action}) //nolint:gosec // small
			}
		}
		return ok(rep)
	})
	f.handle("gpe_fwd_entry_path_dump", func(req api.Message) ([]api.Message, error) {
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
