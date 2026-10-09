package coretest

import (
	"go.fd.io/govpp/api"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
)

func (v *VPP) installUnnumbered() {
	relationships := map[uint32]uint32{}
	v.On("sw_interface_set_unnumbered", func(m api.Message) ([]api.Message, error) {
		r := m.(*ifapi.SwInterfaceSetUnnumbered)
		v.mu.Lock()
		defer v.mu.Unlock()
		b, d := uint32(r.UnnumberedSwIfIndex), uint32(r.SwIfIndex)
		if v.Ifaces[b] == nil || v.Ifaces[d] == nil {
			return reply(&ifapi.SwInterfaceSetUnnumberedReply{Retval: RetvalInvalidSwIfIndex})
		}
		if r.IsAdd {
			relationships[b] = d
		} else {
			delete(relationships, b)
		}
		return reply(&ifapi.SwInterfaceSetUnnumberedReply{})
	})
	v.On("ip_unnumbered_dump", func(m api.Message) ([]api.Message, error) {
		r := m.(*ip.IPUnnumberedDump)
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for b, d := range relationships {
			if v.Ifaces[b] == nil || v.Ifaces[d] == nil {
				delete(relationships, b)
				continue
			}
			if uint32(r.SwIfIndex) != ^uint32(0) && uint32(r.SwIfIndex) != b {
				continue
			}
			out = append(out, &ip.IPUnnumberedDetails{SwIfIndex: interface_types.InterfaceIndex(b), IPSwIfIndex: interface_types.InterfaceIndex(d)})
		}
		return out, nil
	})
}
