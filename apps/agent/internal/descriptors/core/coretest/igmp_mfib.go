package coretest

import (
	"go.fd.io/govpp/api"
	"ngfw/agent/binapi/igmp"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
)

func init() { RegisterExtension("igmp-mfib", (*VPP).installIgmpMfib) }
func (v *VPP) installIgmpMfib() {
	modes := map[interface_types.InterfaceIndex]uint8{}
	type key struct {
		index interface_types.InterfaceIndex
		group ip_types.IP4Address
	}
	groups := map[key][]ip_types.IP4Address{}
	v.On("ip_mtable_dump", func(api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		var out []api.Message
		for k, n := range v.Tables {
			out = append(out, &ip.IPMtableDetails{Table: ip.IPTable{TableID: k.id, IsIP6: k.v6, Name: n}})
		}
		return out, nil
	})
	v.On("igmp_enable_disable", func(m api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		r := m.(*igmp.IgmpEnableDisable)
		if r.Enable {
			if _, exists := modes[r.SwIfIndex]; exists {
				return reply(&igmp.IgmpEnableDisableReply{Retval: int32(api.UNSPECIFIED)})
			}
			modes[r.SwIfIndex] = r.Mode
		} else {
			delete(modes, r.SwIfIndex)
			for k := range groups {
				if k.index == r.SwIfIndex {
					delete(groups, k)
				}
			}
		}
		return reply(&igmp.IgmpEnableDisableReply{})
	})
	v.On("igmp_listen", func(m api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		r := m.(*igmp.IgmpListen)
		k := key{r.Group.SwIfIndex, r.Group.Gaddr}
		if r.Group.NSrcs == 0 {
			delete(groups, k)
		} else {
			groups[k] = append([]ip_types.IP4Address(nil), r.Group.Saddrs...)
		}
		return reply(&igmp.IgmpListenReply{})
	})
	v.On("igmp_dump", func(m api.Message) ([]api.Message, error) {
		v.mu.Lock()
		defer v.mu.Unlock()
		r := m.(*igmp.IgmpDump)
		var out []api.Message
		for k, sources := range groups {
			if r.SwIfIndex != interface_types.InterfaceIndex(^uint32(0)) && r.SwIfIndex != k.index {
				continue
			}
			for _, s := range sources {
				out = append(out, &igmp.IgmpDetails{SwIfIndex: k.index, Gaddr: k.group, Saddr: s})
			}
		}
		return out, nil
	})
	v.On("igmp_proxy_device_add_del", func(api.Message) ([]api.Message, error) { return reply(&igmp.IgmpProxyDeviceAddDelReply{}) })
	v.On("igmp_proxy_device_add_del_interface", func(api.Message) ([]api.Message, error) { return reply(&igmp.IgmpProxyDeviceAddDelInterfaceReply{}) })
	v.On("igmp_group_prefix_set", func(api.Message) ([]api.Message, error) { return reply(&igmp.IgmpGroupPrefixSetReply{}) })
	v.On("want_igmp_events", func(api.Message) ([]api.Message, error) { return reply(&igmp.WantIgmpEventsReply{}) })
}
