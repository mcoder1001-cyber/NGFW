package coretest

// F-nat44-ei-64-66-nptv6: stateful models of the nat64 and nat66 plugins (DF-3's descriptors/nat64 and nat66) on top
// of the VPP model, following DF-3's per-package fakes and VPP 26.06's nat64_api.c / nat66_api.c where the agent
// depends on them: the enables answer the bare retval 1 for "already enabled/disabled" and a disable wipes the
// plugin; nothing is dumped while disabled; nat64 keeps one prefix per VRF (an add for the same VRF replaces it),
// pool addresses one by one, static BIBs with NAT_IS_STATIC (and a dynamic BIB entry per session), sessions per
// protocol (255 = all) with VPP 26.06's st_details defect (il_port carries the remote port, r_port is 0); nat66 keeps one
// entry per interface whose details carry only NAT_IS_INSIDE, and an entry whose interface is gone cannot be removed
// (retval -2) — the D-095c order the agent must keep.

import (
	"net/netip"
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/nat64"
	"ngfw/agent/binapi/nat66"
	"ngfw/agent/binapi/nat_types"
)

// Nat64 is the nat64 plugin model; tests may read and seed the exported fields under Lock/Unlock.
type Nat64 struct {
	v  *VPP
	mu sync.Mutex

	Enabled  bool
	Timeouts nat64.Nat64GetTimeoutsReply
	Ifaces   map[uint32]nat_types.NatConfigFlags
	Prefixes []*nat64.Nat64PrefixDetails
	Pool     []*nat64.Nat64PoolAddrDetails
	BIBs     []*nat64.Nat64BibDetails
	Sessions []*nat64.Nat64StDetails
}

// Lock guards the exported fields.
func (n *Nat64) Lock() { n.mu.Lock() }

// Unlock releases Lock.
func (n *Nat64) Unlock() { n.mu.Unlock() }

// NatEnable enables the modelled plugin as a test fixture would.
func (n *Nat64) NatEnable() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Enabled = true
}

// Empty reports whether the model holds no configuration object.
func (n *Nat64) Empty() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.Ifaces)+len(n.Prefixes)+len(n.Pool)+len(n.BIBs) == 0
}

// AddSession seeds one session: IPv6 client il:ilPort, IPv4 pool ol:olPort, IPv4 remote or:rPort (ir = the remote
// as the client sees it, i.e. inside the NAT64 prefix).
func (n *Nat64) AddSession(vrf uint32, proto uint8, il string, ilPort uint16, ol string, olPort uint16, ir, or string, rPort uint16) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Sessions = append(n.Sessions, &nat64.Nat64StDetails{
		IlAddr: ip_types.IP6Address(netip.MustParseAddr(il).As16()), IlPort: ilPort,
		OlAddr: ip4(ol), OlPort: olPort,
		IrAddr: ip_types.IP6Address(netip.MustParseAddr(ir).As16()), OrAddr: ip4(or), RPort: rPort,
		VrfID: vrf, Proto: proto,
	})
}

// Nat66 is the nat66 plugin model.
type Nat66 struct {
	v  *VPP
	mu sync.Mutex

	Enabled    bool
	OutsideVRF uint32
	Ifaces     map[uint32]nat_types.NatConfigFlags
	Mappings   []*nat66.Nat66StaticMappingDetails
}

// Lock guards the exported fields.
func (n *Nat66) Lock() { n.mu.Lock() }

// Unlock releases Lock.
func (n *Nat66) Unlock() { n.mu.Unlock() }

// NatEnable enables the modelled plugin as a test fixture would.
func (n *Nat66) NatEnable() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Enabled = true
}

// Empty reports whether the model holds no configuration object.
func (n *Nat66) Empty() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.Ifaces)+len(n.Mappings) == 0
}

var nat64Models, nat66Models sync.Map

func init() {
	extensions = append(extensions, func(v *VPP) {
		nat64Models.Store(v, v.installNat64())
		nat66Models.Store(v, v.installNat66())
	})
}

// Nat64 returns the nat64 plugin model of v (starts disabled).
func (v *VPP) Nat64() *Nat64 {
	m, ok := nat64Models.Load(v)
	if !ok {
		m, _ = nat64Models.LoadOrStore(v, v.installNat64())
	}
	return m.(*Nat64)
}

// Nat66 returns the nat66 plugin model of v (starts disabled).
func (v *VPP) Nat66() *Nat66 {
	m, ok := nat66Models.Load(v)
	if !ok {
		m, _ = nat66Models.LoadOrStore(v, v.installNat66())
	}
	return m.(*Nat66)
}

func (v *VPP) installNat64() *Nat64 {
	n := &Nat64{v: v, Ifaces: map[uint32]nat_types.NatConfigFlags{}}
	one := func(m api.Message) ([]api.Message, error) { return []api.Message{m}, nil }
	rv := func(e api.VPPApiError) int32 { return int32(e) }
	defaults := nat64.Nat64GetTimeoutsReply{UDP: 300, TCPEstablished: 7440, TCPTransitory: 240, ICMP: 60}
	n.Timeouts = defaults

	v.On("nat64_plugin_enable_disable", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat64.Nat64PluginEnableDisable)
		n.mu.Lock()
		defer n.mu.Unlock()
		if r.Enable == n.Enabled {
			return one(&nat64.Nat64PluginEnableDisableReply{Retval: 1}) // "plugin already enabled/disabled"
		}
		n.Enabled = r.Enable
		if !r.Enable {
			n.Ifaces, n.Prefixes, n.Pool, n.BIBs, n.Sessions = map[uint32]nat_types.NatConfigFlags{}, nil, nil, nil, nil
			n.Timeouts = defaults
		}
		return one(&nat64.Nat64PluginEnableDisableReply{})
	})
	v.On("nat64_get_timeouts", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		t := n.Timeouts
		return one(&t)
	})
	v.On("nat64_set_timeouts", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat64.Nat64SetTimeouts)
		n.mu.Lock()
		defer n.mu.Unlock()
		n.Timeouts = nat64.Nat64GetTimeoutsReply{UDP: r.UDP, TCPEstablished: r.TCPEstablished, TCPTransitory: r.TCPTransitory, ICMP: r.ICMP}
		return one(&nat64.Nat64SetTimeoutsReply{})
	})
	v.On("nat64_add_del_interface", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat64.Nat64AddDelInterface)
		idx := uint32(r.SwIfIndex)
		if !v.ifExists(idx) {
			return one(&nat64.Nat64AddDelInterfaceReply{Retval: rv(api.INVALID_SW_IF_INDEX)})
		}
		n.mu.Lock()
		defer n.mu.Unlock()
		if !n.Enabled {
			return one(&nat64.Nat64AddDelInterfaceReply{Retval: rv(api.UNSUPPORTED)})
		}
		if r.IsAdd {
			n.Ifaces[idx] |= r.Flags
		} else {
			if n.Ifaces[idx]&r.Flags == 0 {
				return one(&nat64.Nat64AddDelInterfaceReply{Retval: rv(api.NO_SUCH_ENTRY)})
			}
			if n.Ifaces[idx] &^= r.Flags; n.Ifaces[idx] == 0 {
				delete(n.Ifaces, idx)
			}
		}
		return one(&nat64.Nat64AddDelInterfaceReply{})
	})
	v.On("nat64_interface_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedIdx(n.Ifaces) {
			out = append(out, &nat64.Nat64InterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx), Flags: n.Ifaces[idx]})
		}
		return out, nil
	})
	v.On("nat64_add_del_prefix", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat64.Nat64AddDelPrefix)
		n.mu.Lock()
		defer n.mu.Unlock()
		for i, d := range n.Prefixes {
			if d.VrfID != r.VrfID {
				continue
			}
			if r.IsAdd { // nat64_add_del_prefix: one prefix per VRF, an add replaces it
				d.Prefix = r.Prefix
			} else {
				n.Prefixes = append(n.Prefixes[:i], n.Prefixes[i+1:]...)
			}
			return one(&nat64.Nat64AddDelPrefixReply{})
		}
		if !r.IsAdd {
			return one(&nat64.Nat64AddDelPrefixReply{Retval: rv(api.NO_SUCH_ENTRY)})
		}
		n.Prefixes = append(n.Prefixes, &nat64.Nat64PrefixDetails{Prefix: r.Prefix, VrfID: r.VrfID})
		return one(&nat64.Nat64AddDelPrefixReply{})
	})
	v.On("nat64_prefix_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		out := make([]api.Message, 0, len(n.Prefixes))
		for _, d := range n.Prefixes {
			c := *d
			out = append(out, &c)
		}
		return out, nil
	})
	v.On("nat64_add_del_pool_addr_range", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat64.Nat64AddDelPoolAddrRange)
		n.mu.Lock()
		defer n.mu.Unlock()
		first, last := netip.AddrFrom4(r.StartAddr), netip.AddrFrom4(r.EndAddr)
		for a := first; !last.Less(a); a = a.Next() {
			found := -1
			for i, d := range n.Pool {
				if netip.AddrFrom4(d.Address) == a {
					found = i
				}
			}
			switch {
			case r.IsAdd && found >= 0:
				return one(&nat64.Nat64AddDelPoolAddrRangeReply{Retval: rv(api.VALUE_EXIST)})
			case r.IsAdd:
				n.Pool = append(n.Pool, &nat64.Nat64PoolAddrDetails{Address: a.As4(), VrfID: r.VrfID})
			case found < 0:
				return one(&nat64.Nat64AddDelPoolAddrRangeReply{Retval: rv(api.NO_SUCH_ENTRY)})
			default:
				n.Pool = append(n.Pool[:found], n.Pool[found+1:]...)
			}
		}
		return one(&nat64.Nat64AddDelPoolAddrRangeReply{})
	})
	v.On("nat64_pool_addr_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		out := make([]api.Message, 0, len(n.Pool))
		for _, d := range n.Pool {
			c := *d
			out = append(out, &c)
		}
		return out, nil
	})
	v.On("nat64_add_del_static_bib", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat64.Nat64AddDelStaticBib)
		n.mu.Lock()
		defer n.mu.Unlock()
		for i, d := range n.BIBs {
			if d.IAddr == r.IAddr && d.IPort == r.IPort && d.Proto == r.Proto && d.VrfID == r.VrfID {
				if r.IsAdd {
					return one(&nat64.Nat64AddDelStaticBibReply{Retval: rv(api.VALUE_EXIST)})
				}
				n.BIBs = append(n.BIBs[:i], n.BIBs[i+1:]...)
				return one(&nat64.Nat64AddDelStaticBibReply{})
			}
		}
		if !r.IsAdd {
			return one(&nat64.Nat64AddDelStaticBibReply{Retval: rv(api.NO_SUCH_ENTRY)})
		}
		n.BIBs = append(n.BIBs, &nat64.Nat64BibDetails{IAddr: r.IAddr, OAddr: r.OAddr, IPort: r.IPort, OPort: r.OPort, VrfID: r.VrfID, Proto: r.Proto, Flags: nat_types.NAT_IS_STATIC})
		return one(&nat64.Nat64AddDelStaticBibReply{})
	})
	v.On("nat64_bib_dump", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat64.Nat64BibDump)
		n.mu.Lock()
		defer n.mu.Unlock()
		var out []api.Message
		for _, d := range n.BIBs {
			if r.Proto == 255 || r.Proto == d.Proto {
				c := *d
				out = append(out, &c)
			}
		}
		for _, d := range n.Sessions { // every dynamic session has its (dynamic) BIB entry
			if r.Proto == 255 || r.Proto == d.Proto {
				out = append(out, &nat64.Nat64BibDetails{IAddr: d.IlAddr, OAddr: d.OlAddr, IPort: d.IlPort, OPort: d.OlPort, VrfID: d.VrfID, Proto: d.Proto, SesNum: 1})
			}
		}
		return out, nil
	})
	v.On("nat64_st_dump", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat64.Nat64StDump)
		n.mu.Lock()
		defer n.mu.Unlock()
		var out []api.Message
		for _, d := range n.Sessions {
			if r.Proto == 255 || r.Proto == d.Proto {
				c := *d
				c.IlPort, c.RPort = d.RPort, 0 // VPP 26.06 nat64_api_st_walk: il_port = r_port, r_port never set
				out = append(out, &c)
			}
		}
		return out, nil
	})
	return n
}

func (v *VPP) installNat66() *Nat66 {
	n := &Nat66{v: v, Ifaces: map[uint32]nat_types.NatConfigFlags{}}
	one := func(m api.Message) ([]api.Message, error) { return []api.Message{m}, nil }
	rv := func(e api.VPPApiError) int32 { return int32(e) }

	v.On("nat66_plugin_enable_disable", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat66.Nat66PluginEnableDisable)
		n.mu.Lock()
		defer n.mu.Unlock()
		if r.Enable == n.Enabled {
			return one(&nat66.Nat66PluginEnableDisableReply{Retval: 1})
		}
		n.Enabled, n.OutsideVRF = r.Enable, r.OutsideVrf
		if !r.Enable {
			n.Ifaces, n.Mappings, n.OutsideVRF = map[uint32]nat_types.NatConfigFlags{}, nil, 0
		}
		return one(&nat66.Nat66PluginEnableDisableReply{})
	})
	v.On("nat66_add_del_interface", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat66.Nat66AddDelInterface)
		idx := uint32(r.SwIfIndex)
		exists := v.ifExists(idx)
		n.mu.Lock()
		defer n.mu.Unlock()
		if !exists {
			if _, stale := n.Ifaces[idx]; stale && !r.IsAdd {
				return one(&nat66.Nat66AddDelInterfaceReply{Retval: -2}) // the interface is gone: VPP cannot remove it
			}
			return one(&nat66.Nat66AddDelInterfaceReply{Retval: rv(api.INVALID_SW_IF_INDEX)})
		}
		if !n.Enabled {
			return one(&nat66.Nat66AddDelInterfaceReply{Retval: rv(api.UNSUPPORTED)})
		}
		if r.IsAdd {
			if _, dup := n.Ifaces[idx]; dup {
				return one(&nat66.Nat66AddDelInterfaceReply{Retval: rv(api.VALUE_EXIST)})
			}
			n.Ifaces[idx] = r.Flags & nat_types.NAT_IS_INSIDE // one side per interface, only the inside bit is reported
		} else {
			if _, ok := n.Ifaces[idx]; !ok {
				return one(&nat66.Nat66AddDelInterfaceReply{Retval: rv(api.NO_SUCH_ENTRY)})
			}
			delete(n.Ifaces, idx)
		}
		return one(&nat66.Nat66AddDelInterfaceReply{})
	})
	v.On("nat66_interface_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedIdx(n.Ifaces) {
			out = append(out, &nat66.Nat66InterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx), Flags: n.Ifaces[idx]})
		}
		return out, nil
	})
	v.On("nat66_add_del_static_mapping", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat66.Nat66AddDelStaticMapping)
		n.mu.Lock()
		defer n.mu.Unlock()
		for i, d := range n.Mappings {
			if d.LocalIPAddress == r.LocalIPAddress && d.VrfID == r.VrfID {
				if r.IsAdd {
					return one(&nat66.Nat66AddDelStaticMappingReply{Retval: rv(api.VALUE_EXIST)})
				}
				n.Mappings = append(n.Mappings[:i], n.Mappings[i+1:]...)
				return one(&nat66.Nat66AddDelStaticMappingReply{})
			}
		}
		if !r.IsAdd {
			return one(&nat66.Nat66AddDelStaticMappingReply{Retval: rv(api.NO_SUCH_ENTRY)})
		}
		n.Mappings = append(n.Mappings, &nat66.Nat66StaticMappingDetails{LocalIPAddress: r.LocalIPAddress, ExternalIPAddress: r.ExternalIPAddress, VrfID: r.VrfID})
		return one(&nat66.Nat66AddDelStaticMappingReply{})
	})
	v.On("nat66_static_mapping_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		out := make([]api.Message, 0, len(n.Mappings))
		for _, d := range n.Mappings {
			c := *d
			out = append(out, &c)
		}
		return out, nil
	})
	return n
}
