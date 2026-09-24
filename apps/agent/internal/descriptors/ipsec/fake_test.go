package ipsec_test

// A stateful fake VPP for the ipsec descriptors: handlers keep maps of "VPP" objects so a dump
// reflects earlier creates, exactly like the scheduler's example test. It also models the two
// quirks the descriptors work around: ipsec_spd_interface_details carries the SPD pool index, and
// ipsec_sa_v5_details returns the key material.

import (
	"fmt"
	"maps"
	"math/bits"
	"slices"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/binapi/ipsec_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/vpp/fake"
	"ngfw/agent/internal/vpp/ifsanitize/sanitizetest"
)

const (
	retvalInvalid = -1  // VNET_API_ERROR_UNSPECIFIED
	retvalNoSuch  = -50 // arbitrary non-zero: the descriptor only needs err != nil
)

type fakeVPP struct {
	*fake.Client
	nextIf     uint32
	ifaces     map[uint32]*interfaces.SwInterfaceDetails
	nextSpdIdx uint32
	spds       map[uint32]uint32 // spd_id → pool index
	bindings   map[uint32]uint32 // sw_if_index → pool index
	policies   map[uint32][]ipsec_types.IpsecSpdEntryV2
	sas        map[uint32]ipsec_types.IpsecSadEntryV4
	saStat     map[uint32]uint32
	nextStat   uint32
	tps        map[uint32]ipsec.IpsecTunnelProtect
	itfs       map[uint32]ipsec.IpsecItf
	backends   []ipsec.IpsecBackendDetails
	async      []bool
	// VPP's SA lock counting (ipsec_sa.c): the add holds one lock, every protect policy and every
	// tunnel protection holds one more; ipsec_sad_entry_del is an UNLOCK, the SA is freed at 0.
	// freedWhileReferenced records a use-after-free (a policy/protection still pointing at it).
	saLocks              map[uint32]int
	freedWhileReferenced []uint32
	failPolicyDel        bool // policy deletes fail with a VPP error (sweep regression)
}

func (v *fakeVPP) lockSA(id uint32) { v.saLocks[id]++ }

func (v *fakeVPP) unlockSA(id uint32) {
	if _, ok := v.sas[id]; !ok {
		return
	}
	v.saLocks[id]--
	if v.saLocks[id] > 0 {
		return
	}
	delete(v.sas, id)
	delete(v.saLocks, id)
	for _, pols := range v.policies {
		for _, p := range pols {
			if p.Policy == ipsec_types.IPSEC_API_SPD_ACTION_PROTECT && p.SaID == id {
				v.freedWhileReferenced = append(v.freedWhileReferenced, id)
			}
		}
	}
	for _, tp := range v.tps {
		for _, x := range append([]uint32{tp.SaOut}, tp.SaIn...) {
			if x == id {
				v.freedWhileReferenced = append(v.freedWhileReferenced, id)
			}
		}
	}
}

func protectSA(e ipsec_types.IpsecSpdEntryV2) (uint32, bool) {
	return e.SaID, e.Policy == ipsec_types.IPSEC_API_SPD_ACTION_PROTECT
}

func newFakeVPP() *fakeVPP {
	v := &fakeVPP{
		Client:     fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})),
		nextIf:     1,
		ifaces:     map[uint32]*interfaces.SwInterfaceDetails{0: {SwIfIndex: 0, InterfaceName: "local0"}},
		nextSpdIdx: 7, // pool indices are unrelated to ids
		spds:       map[uint32]uint32{},
		bindings:   map[uint32]uint32{},
		policies:   map[uint32][]ipsec_types.IpsecSpdEntryV2{},
		sas:        map[uint32]ipsec_types.IpsecSadEntryV4{},
		saStat:     map[uint32]uint32{},
		nextStat:   100,
		tps:        map[uint32]ipsec.IpsecTunnelProtect{},
		itfs:       map[uint32]ipsec.IpsecItf{},
		saLocks:    map[uint32]int{},
		backends: []ipsec.IpsecBackendDetails{
			{Name: "crypto engine backend", Protocol: ipsec_types.IPSEC_API_PROTO_ESP, Index: 0, Active: true},
			{Name: "dpdk backend", Protocol: ipsec_types.IPSEC_API_PROTO_ESP, Index: 1},
			{Name: "crypto engine backend", Protocol: ipsec_types.IPSEC_API_PROTO_AH, Index: 0, Active: true},
		},
	}
	v.On("sw_interface_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(v.ifaces))
		for _, i := range slices.Sorted(maps.Keys(v.ifaces)) {
			out = append(out, v.ifaces[i])
		}
		return out, nil
	})
	v.On("sw_interface_tag_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*interfaces.SwInterfaceTagAddDel)
		i, ok := v.ifaces[uint32(r.SwIfIndex)]
		if !ok {
			return []api.Message{&interfaces.SwInterfaceTagAddDelReply{Retval: retvalNoSuch}}, nil
		}
		i.Tag = r.Tag
		return []api.Message{&interfaces.SwInterfaceTagAddDelReply{}}, nil
	})
	v.On("ipsec_spd_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*ipsec.IpsecSpdAddDel)
		_, exists := v.spds[r.SpdID]
		switch {
		case r.IsAdd && exists, !r.IsAdd && !exists:
			return []api.Message{&ipsec.IpsecSpdAddDelReply{Retval: retvalInvalid}}, nil
		case r.IsAdd:
			v.spds[r.SpdID] = v.nextSpdIdx
			v.nextSpdIdx++
		default:
			poolIdx := v.spds[r.SpdID]
			// VPP (ipsec_spd.c:24-104) frees the policy vectors WITHOUT unlocking their SAs: the
			// locks of protect policies stay held (leaked) — modelled exactly (fix round 2, N1)
			delete(v.spds, r.SpdID)
			delete(v.policies, r.SpdID)
			for sw, idx := range v.bindings { // VPP unbinds every interface from a deleted SPD
				if idx == poolIdx {
					delete(v.bindings, sw)
				}
			}
		}
		return []api.Message{&ipsec.IpsecSpdAddDelReply{}}, nil
	})
	v.On("ipsec_spds_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for id := range v.spds {
			out = append(out, &ipsec.IpsecSpdsDetails{SpdID: id, Npolicies: uint32(len(v.policies[id]))}) //nolint:gosec // test sizes
		}
		return out, nil
	})
	v.On("ipsec_interface_add_del_spd", func(req api.Message) ([]api.Message, error) {
		r := req.(*ipsec.IpsecInterfaceAddDelSpd)
		spdIdx, ok := v.spds[r.SpdID]
		if !ok {
			return []api.Message{&ipsec.IpsecInterfaceAddDelSpdReply{Retval: retvalNoSuch}}, nil
		}
		if _, bound := v.bindings[uint32(r.SwIfIndex)]; bound == r.IsAdd {
			return []api.Message{&ipsec.IpsecInterfaceAddDelSpdReply{Retval: retvalInvalid}}, nil
		}
		if r.IsAdd {
			v.bindings[uint32(r.SwIfIndex)] = spdIdx
		} else {
			delete(v.bindings, uint32(r.SwIfIndex))
		}
		return []api.Message{&ipsec.IpsecInterfaceAddDelSpdReply{}}, nil
	})
	v.On("ipsec_spd_interface_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for sw, spdIdx := range v.bindings {
			out = append(out, &ipsec.IpsecSpdInterfaceDetails{SpdIndex: spdIdx, SwIfIndex: interface_types.InterfaceIndex(sw)})
		}
		return out, nil
	})
	v.On("ipsec_spd_entry_add_del_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*ipsec.IpsecSpdEntryAddDelV2)
		if !r.IsAdd && v.failPolicyDel {
			return []api.Message{&ipsec.IpsecSpdEntryAddDelV2Reply{Retval: retvalInvalid}}, nil
		}
		if _, ok := v.spds[r.Entry.SpdID]; !ok {
			return []api.Message{&ipsec.IpsecSpdEntryAddDelV2Reply{Retval: retvalNoSuch}}, nil
		}
		e := r.Entry // stored as sent: the v2 handler does not map 0 to "any" (unlike v1)
		list := v.policies[e.SpdID]
		pos := -1
		for i, p := range list {
			if p == e {
				pos = i
			}
		}
		switch {
		case r.IsAdd && pos >= 0, !r.IsAdd && pos < 0:
			return []api.Message{&ipsec.IpsecSpdEntryAddDelV2Reply{Retval: retvalInvalid}}, nil
		case r.IsAdd:
			if id, ok := protectSA(e); ok {
				if _, exists := v.sas[id]; !exists {
					return []api.Message{&ipsec.IpsecSpdEntryAddDelV2Reply{Retval: retvalNoSuch}}, nil
				}
				v.lockSA(id)
			}
			v.policies[e.SpdID] = append(list, e)
		default:
			v.policies[e.SpdID] = append(list[:pos], list[pos+1:]...)
			if id, ok := protectSA(e); ok {
				v.unlockSA(id)
			}
		}
		v.nextStat++
		return []api.Message{&ipsec.IpsecSpdEntryAddDelV2Reply{StatIndex: v.nextStat}}, nil
	})
	v.On("ipsec_spd_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*ipsec.IpsecSpdDump)
		var out []api.Message
		for _, p := range v.policies[r.SpdID] {
			out = append(out, &ipsec.IpsecSpdDetails{Entry: ipsec_types.IpsecSpdEntry(p)})
		}
		return out, nil
	})
	v.On("ipsec_sad_entry_add_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*ipsec.IpsecSadEntryAddV2)
		if _, exists := v.sas[r.Entry.SadID]; exists {
			return []api.Message{&ipsec.IpsecSadEntryAddV2Reply{Retval: retvalInvalid}}, nil
		}
		e := r.Entry
		e.Salt = bits.ReverseBytes32(e.Salt) // ipsec_sa_v5_details reports the salt byte-swapped (see decodeSa)
		// VPP keeps its own copy of the key: the descriptor zeroes the request buffer afterwards
		e.CryptoKey.Data = append([]byte(nil), e.CryptoKey.Data...)
		e.IntegrityKey.Data = append([]byte(nil), e.IntegrityKey.Data...)
		if e.Flags&ipsec_types.IPSEC_API_SAD_FLAG_USE_ANTI_REPLAY != 0 && e.AntiReplayWindowSize < 64 {
			e.AntiReplayWindowSize = 64
		}
		v.sas[e.SadID] = e
		v.saLocks[e.SadID] = 1
		v.nextStat++
		v.saStat[e.SadID] = v.nextStat
		return []api.Message{&ipsec.IpsecSadEntryAddV2Reply{StatIndex: v.nextStat}}, nil
	})
	v.On("ipsec_sad_entry_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*ipsec.IpsecSadEntryDel)
		if _, exists := v.sas[r.ID]; !exists {
			return []api.Message{&ipsec.IpsecSadEntryDelReply{Retval: retvalNoSuch}}, nil
		}
		v.unlockSA(r.ID) // an unlock, not a delete
		return []api.Message{&ipsec.IpsecSadEntryDelReply{}}, nil
	})
	v.On("ipsec_sa_v5_dump", func(req api.Message) ([]api.Message, error) {
		want := req.(*ipsec.IpsecSaV5Dump).SaID
		var out []api.Message
		for id, e := range v.sas {
			if want != ^uint32(0) && id != want {
				continue
			}
			c := e
			c.CryptoKey.Data = append([]byte(nil), e.CryptoKey.Data...)
			c.IntegrityKey.Data = append([]byte(nil), e.IntegrityKey.Data...)
			out = append(out, &ipsec.IpsecSaV5Details{Entry: c, StatIndex: v.saStat[id], SwIfIndex: ^interface_types.InterfaceIndex(0)})
		}
		return out, nil
	})
	v.On("ipsec_tunnel_protect_update", func(req api.Message) ([]api.Message, error) {
		r := req.(*ipsec.IpsecTunnelProtectUpdate)
		if _, ok := v.ifaces[uint32(r.Tunnel.SwIfIndex)]; !ok {
			return []api.Message{&ipsec.IpsecTunnelProtectUpdateReply{Retval: retvalNoSuch}}, nil
		}
		for _, sa := range append([]uint32{r.Tunnel.SaOut}, r.Tunnel.SaIn...) {
			if _, ok := v.sas[sa]; !ok {
				return []api.Message{&ipsec.IpsecTunnelProtectUpdateReply{Retval: retvalNoSuch}}, nil
			}
		}
		tp := r.Tunnel
		tp.SaIn = append([]uint32(nil), tp.SaIn...)
		for _, x := range append([]uint32{tp.SaOut}, tp.SaIn...) {
			v.lockSA(x)
		}
		if old, ok := v.tps[uint32(tp.SwIfIndex)]; ok {
			defer func() {
				for _, x := range append([]uint32{old.SaOut}, old.SaIn...) {
					v.unlockSA(x)
				}
			}()
		}
		v.tps[uint32(tp.SwIfIndex)] = tp
		return []api.Message{&ipsec.IpsecTunnelProtectUpdateReply{}}, nil
	})
	v.On("ipsec_tunnel_protect_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*ipsec.IpsecTunnelProtectDel)
		old, ok := v.tps[uint32(r.SwIfIndex)]
		if !ok {
			return []api.Message{&ipsec.IpsecTunnelProtectDelReply{Retval: retvalNoSuch}}, nil
		}
		delete(v.tps, uint32(r.SwIfIndex))
		for _, x := range append([]uint32{old.SaOut}, old.SaIn...) {
			v.unlockSA(x)
		}
		return []api.Message{&ipsec.IpsecTunnelProtectDelReply{}}, nil
	})
	v.On("ipsec_tunnel_protect_dump", func(req api.Message) ([]api.Message, error) {
		want := uint32(req.(*ipsec.IpsecTunnelProtectDump).SwIfIndex)
		var out []api.Message
		for sw, tp := range v.tps {
			if want != ^uint32(0) && sw != want {
				continue
			}
			out = append(out, &ipsec.IpsecTunnelProtectDetails{Tun: tp})
		}
		return out, nil
	})
	v.On("ipsec_itf_create", func(req api.Message) ([]api.Message, error) {
		r := req.(*ipsec.IpsecItfCreate)
		idx := v.addIface(fmt.Sprintf("ipsec%d", r.Itf.UserInstance), "")
		itf := r.Itf
		itf.SwIfIndex = interface_types.InterfaceIndex(idx)
		v.itfs[idx] = itf
		return []api.Message{&ipsec.IpsecItfCreateReply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
	})
	v.On("ipsec_itf_delete", func(req api.Message) ([]api.Message, error) {
		r := req.(*ipsec.IpsecItfDelete)
		if _, ok := v.itfs[uint32(r.SwIfIndex)]; !ok {
			return []api.Message{&ipsec.IpsecItfDeleteReply{Retval: retvalNoSuch}}, nil
		}
		delete(v.itfs, uint32(r.SwIfIndex))
		delete(v.ifaces, uint32(r.SwIfIndex))
		return []api.Message{&ipsec.IpsecItfDeleteReply{}}, nil
	})
	v.On("ipsec_itf_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, itf := range v.itfs {
			out = append(out, &ipsec.IpsecItfDetails{Itf: itf})
		}
		return out, nil
	})
	v.On("ipsec_backend_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(v.backends))
		for i := range v.backends {
			b := v.backends[i]
			out = append(out, &b)
		}
		return out, nil
	})
	v.On("ipsec_select_backend", func(req api.Message) ([]api.Message, error) {
		r := req.(*ipsec.IpsecSelectBackend)
		found := false
		for i := range v.backends {
			if v.backends[i].Protocol == r.Protocol {
				v.backends[i].Active = v.backends[i].Index == r.Index
				found = found || v.backends[i].Active
			}
		}
		if !found {
			return []api.Message{&ipsec.IpsecSelectBackendReply{Retval: retvalNoSuch}}, nil
		}
		return []api.Message{&ipsec.IpsecSelectBackendReply{}}, nil
	})
	v.On("ipsec_set_async_mode", func(req api.Message) ([]api.Message, error) {
		v.async = append(v.async, req.(*ipsec.IpsecSetAsyncMode).AsyncEnable)
		return []api.Message{&ipsec.IpsecSetAsyncModeReply{}}, nil
	})
	sanitizetest.Clean(v.Client) // D-095: interface creates sanitize the new sw_if_index (TD-3 re-review H1)
	return v
}

// addIface adds an interface with the given name and tag and returns its sw_if_index.
func (v *fakeVPP) addIface(name, tag string) uint32 {
	idx := v.nextIf
	v.nextIf++
	v.ifaces[idx] = &interfaces.SwInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx), InterfaceName: name, Tag: tag}
	return idx
}
