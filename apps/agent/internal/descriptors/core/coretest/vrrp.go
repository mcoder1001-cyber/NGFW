package coretest

// F-vrrp-config-sync extension of the model (A6: own file, installed through the extensions seam): the VPP
// vrrp plugin as far as DF-7's vrrp descriptors use it — a pool of VRs (vrrp_vr_update creates/updates,
// vrrp_vr_add_del deletes, vrrp_vr_dump), unicast peers (refused while the VR runs, as VPP), tracked
// interfaces and start/stop. Ported from DF-7's unit-test fake (descriptors/vrrp/vrrp_test.go). A started VR
// goes to Backup (no packets in the model).

import (
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	vrrpapi "ngfw/agent/binapi/vrrp"
)

type vrrpVR struct {
	det    *vrrpapi.VrrpVrDetails
	peers  []ip_types.Address
	tracks []vrrpapi.VrrpVrTrackIf
}

// VrrpModel is the vrrp plugin state of one model.
type VrrpModel struct {
	mu   sync.Mutex
	pool map[uint32]*vrrpVR
	next uint32
}

var vrrpModels sync.Map // *VPP → *VrrpModel

func init() { RegisterExtension("vrrp", installVrrp) }

// Vrrp returns the vrrp model of v.
func (v *VPP) Vrrp() *VrrpModel {
	m, _ := vrrpModels.Load(v)
	return m.(*VrrpModel)
}

// Count is the number of VRs and how many of them run.
func (m *VrrpModel) Count() (vrs, running int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range m.pool {
		vrs++
		if x.det.Runtime.State != vrrpapi.VRRP_API_VR_STATE_INIT {
			running++
		}
	}
	return vrs, running
}

// Forget drops every VR (a VPP restart as seen by the agent).
func (m *VrrpModel) Forget() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pool = map[uint32]*vrrpVR{}
}

func (m *VrrpModel) find(idx interface_types.InterfaceIndex, id uint8, v6 bool) (uint32, *vrrpVR) {
	for i, x := range m.pool {
		if x.det.Config.SwIfIndex == idx && x.det.Config.VrID == id && (x.det.Config.Flags&vrrpapi.VRRP_API_VR_IPV6 != 0) == v6 {
			return i, x
		}
	}
	return 0, nil
}

func installVrrp(v *VPP) {
	m := &VrrpModel{pool: map[uint32]*vrrpVR{}, next: 1}
	vrrpModels.Store(v, m)
	on := func(name string, h func(api.Message) []api.Message) {
		v.On(name, func(req api.Message) ([]api.Message, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			return h(req), nil
		})
	}
	on("vrrp_vr_update", func(req api.Message) []api.Message {
		r := req.(*vrrpapi.VrrpVrUpdate)
		conf := vrrpapi.VrrpVrConf{SwIfIndex: r.SwIfIndex, VrID: r.VrID, Priority: r.Priority, Interval: r.Interval, Flags: r.Flags}
		if r.VrrpIndex == ^uint32(0) {
			if _, x := m.find(r.SwIfIndex, r.VrID, r.Flags&vrrpapi.VRRP_API_VR_IPV6 != 0); x != nil {
				return []api.Message{&vrrpapi.VrrpVrUpdateReply{Retval: int32(api.ENTRY_ALREADY_EXISTS)}}
			}
			idx := m.next
			m.next++
			m.pool[idx] = &vrrpVR{det: &vrrpapi.VrrpVrDetails{Config: conf, Addrs: r.Addrs, NAddrs: r.NAddrs}}
			return []api.Message{&vrrpapi.VrrpVrUpdateReply{VrrpIndex: idx}}
		}
		x, ok := m.pool[r.VrrpIndex]
		if !ok {
			return []api.Message{&vrrpapi.VrrpVrUpdateReply{Retval: int32(api.NO_SUCH_ENTRY)}}
		}
		if x.det.Config.SwIfIndex != r.SwIfIndex || x.det.Config.VrID != r.VrID {
			return []api.Message{&vrrpapi.VrrpVrUpdateReply{Retval: int32(api.INVALID_ARGUMENT)}}
		}
		x.det.Config, x.det.Addrs, x.det.NAddrs = conf, r.Addrs, r.NAddrs
		return []api.Message{&vrrpapi.VrrpVrUpdateReply{VrrpIndex: r.VrrpIndex}}
	})
	on("vrrp_vr_add_del", func(req api.Message) []api.Message {
		r := req.(*vrrpapi.VrrpVrAddDel)
		i, x := m.find(r.SwIfIndex, r.VrID, r.Flags&vrrpapi.VRRP_API_VR_IPV6 != 0)
		if x == nil || r.IsAdd != 0 {
			return []api.Message{&vrrpapi.VrrpVrAddDelReply{Retval: int32(api.NO_SUCH_ENTRY)}}
		}
		delete(m.pool, i)
		return []api.Message{&vrrpapi.VrrpVrAddDelReply{}}
	})
	on("vrrp_vr_dump", func(api.Message) []api.Message {
		var out []api.Message
		for _, x := range m.pool {
			d := *x.det
			out = append(out, &d)
		}
		return out
	})
	on("vrrp_vr_start_stop", func(req api.Message) []api.Message {
		r := req.(*vrrpapi.VrrpVrStartStop)
		_, x := m.find(r.SwIfIndex, r.VrID, r.IsIPv6 != 0)
		if x == nil {
			return []api.Message{&vrrpapi.VrrpVrStartStopReply{Retval: int32(api.NO_SUCH_ENTRY)}}
		}
		x.det.Runtime.State = vrrpapi.VRRP_API_VR_STATE_INIT
		if r.IsStart != 0 {
			x.det.Runtime.State = vrrpapi.VRRP_API_VR_STATE_BACKUP
		}
		return []api.Message{&vrrpapi.VrrpVrStartStopReply{}}
	})
	on("vrrp_vr_set_peers", func(req api.Message) []api.Message {
		r := req.(*vrrpapi.VrrpVrSetPeers)
		_, x := m.find(r.SwIfIndex, r.VrID, r.IsIPv6 != 0)
		switch {
		case x == nil:
			return []api.Message{&vrrpapi.VrrpVrSetPeersReply{Retval: int32(api.NO_SUCH_ENTRY)}}
		case x.det.Runtime.State != vrrpapi.VRRP_API_VR_STATE_INIT:
			return []api.Message{&vrrpapi.VrrpVrSetPeersReply{Retval: int32(api.RSRC_IN_USE)}}
		}
		x.peers = r.Addrs
		return []api.Message{&vrrpapi.VrrpVrSetPeersReply{}}
	})
	on("vrrp_vr_peer_dump", func(req api.Message) []api.Message {
		r := req.(*vrrpapi.VrrpVrPeerDump)
		_, x := m.find(r.SwIfIndex, r.VrID, r.IsIPv6 != 0)
		if x == nil || len(x.peers) == 0 {
			return nil
		}
		return []api.Message{&vrrpapi.VrrpVrPeerDetails{SwIfIndex: r.SwIfIndex, VrID: r.VrID, IsIPv6: r.IsIPv6, NPeerAddrs: uint8(len(x.peers)), PeerAddrs: x.peers}} //nolint:gosec // ≤ 255 in the model
	})
	on("vrrp_vr_track_if_add_del", func(req api.Message) []api.Message {
		r := req.(*vrrpapi.VrrpVrTrackIfAddDel)
		_, x := m.find(r.SwIfIndex, r.VrID, r.IsIPv6 != 0)
		if x == nil {
			return []api.Message{&vrrpapi.VrrpVrTrackIfAddDelReply{Retval: int32(api.NO_SUCH_ENTRY)}}
		}
		for _, t := range r.Ifs {
			kept := x.tracks[:0]
			found := false
			for _, e := range x.tracks {
				if e.SwIfIndex == t.SwIfIndex {
					found = true
					if r.IsAdd != 0 {
						kept = append(kept, e)
					}
					continue
				}
				kept = append(kept, e)
			}
			if r.IsAdd != 0 && !found {
				kept = append(kept, t)
			}
			x.tracks = kept
		}
		return []api.Message{&vrrpapi.VrrpVrTrackIfAddDelReply{}}
	})
	on("vrrp_vr_track_if_dump", func(api.Message) []api.Message {
		var out []api.Message
		for _, x := range m.pool {
			if len(x.tracks) > 0 {
				out = append(out, &vrrpapi.VrrpVrTrackIfDetails{SwIfIndex: x.det.Config.SwIfIndex, VrID: x.det.Config.VrID, IsIPv6: boolU8(x.det.Config.Flags&vrrpapi.VRRP_API_VR_IPV6 != 0), NIfs: uint8(len(x.tracks)), Ifs: append([]vrrpapi.VrrpVrTrackIf(nil), x.tracks...)}) //nolint:gosec // model
			}
		}
		return out
	})
}

func boolU8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
