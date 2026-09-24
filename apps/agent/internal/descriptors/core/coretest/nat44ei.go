package coretest

// F-nat44-ei-64-66-nptv6: a stateful model of the nat44-ei plugin (DF-3's descriptors/nat44ei and its session helpers)
// on top of the VPP model, following DF-3's per-package fake (descriptors/nat44ei/nat44ei_test.go) and VPP 26.06's
// nat44_ei_api.c where the agent depends on it: pool addresses one by one (interface-pool addresses are listed in the
// address dump too), interface-bound static / identity mappings dumped twice (resolved twin, then the to-resolve
// record), mapping deletes matched by the endpoint (never the tag), sessions dumped per user and deleted by the
// inside endpoint only (nat44_ei_del_session ignores the external host), every dump empty while disabled, a disable
// wiping everything. It replaces the nat44_ei_show_running_config stub of coretest/nat44ed.go (installed later: the
// extensions run in file order).

import (
	"net/netip"
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/binapi/nat_types"
)

// EISession is one modelled NAT44-EI session: the per-user dump details plus the user's VRF.
type EISession struct {
	VRF uint32
	nat44_ei.Nat44EiUserSessionV2Details
}

// Nat44EI is the nat44-ei plugin model; tests may read and seed the exported fields under Lock/Unlock.
type Nat44EI struct {
	v  *VPP
	mu sync.Mutex

	Enabled    bool
	Cfg        nat44_ei.Nat44EiPluginEnableDisable
	Timeouts   nat_types.NatTimeouts
	Forwarding bool
	Ipfix      bool
	Features   map[uint32]nat44_ei.Nat44EiConfigFlags
	Outputs    map[uint32]bool
	IfAddrs    map[uint32]bool
	Addrs      []*nat44_ei.Nat44EiAddressDetails
	Statics    []*nat44_ei.Nat44EiStaticMappingDetails
	Idents     []*nat44_ei.Nat44EiIdentityMappingDetails
	Sessions   []EISession
	// SessionDumps counts nat44_ei_user_session_v2_dump requests.
	SessionDumps int
}

// Lock guards the exported fields while a test seeds or inspects them.
func (n *Nat44EI) Lock() { n.mu.Lock() }

// Unlock releases Lock.
func (n *Nat44EI) Unlock() { n.mu.Unlock() }

// NatEnable enables the modelled plugin as a test fixture would (default timeouts), without a descriptor.
func (n *Nat44EI) NatEnable() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.enableLocked(nat44_ei.Nat44EiPluginEnableDisable{Enable: true})
}

func (n *Nat44EI) enableLocked(r nat44_ei.Nat44EiPluginEnableDisable) {
	n.Enabled, n.Cfg = true, r
	n.Timeouts = nat_types.NatTimeouts{UDP: 300, TCPEstablished: 7440, TCPTransitory: 240, ICMP: 60}
}

// Empty reports whether the model holds no configuration object (sessions are state).
func (n *Nat44EI) Empty() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.Features)+len(n.Outputs)+len(n.IfAddrs)+len(n.Addrs)+len(n.Statics)+len(n.Idents) == 0
}

// AddEISession seeds one session (addresses "a.b.c.d", host-order ports; static = from a static mapping).
func (n *Nat44EI) AddEISession(vrf uint32, proto uint16, in string, inPort uint16, out string, outPort uint16, ext string, extPort uint16, static bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	d := nat44_ei.Nat44EiUserSessionV2Details{
		InsideIPAddress: ip4(in), InsidePort: inPort, OutsideIPAddress: ip4(out), OutsidePort: outPort,
		ExtHostAddress: ip4(ext), ExtHostPort: extPort, Protocol: proto, TotalBytes: 100, TotalPkts: 2, TimeSinceLastHeard: 1,
	}
	if static {
		d.Flags |= nat44_ei.NAT44_EI_STATIC_MAPPING
	}
	n.Sessions = append(n.Sessions, EISession{VRF: vrf, Nat44EiUserSessionV2Details: d})
}

// SessionCount returns the number of modelled sessions.
func (n *Nat44EI) SessionCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.Sessions)
}

var nat44EIModels sync.Map

func init() {
	extensions = append(extensions, func(v *VPP) { nat44EIModels.Store(v, v.installNat44EI()) })
}

// Nat44EI returns the nat44-ei plugin model of v (starts disabled).
func (v *VPP) Nat44EI() *Nat44EI {
	m, ok := nat44EIModels.Load(v)
	if !ok {
		m, _ = nat44EIModels.LoadOrStore(v, v.installNat44EI())
	}
	return m.(*Nat44EI)
}

func (v *VPP) ifExists(idx uint32) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.Ifaces[idx]
	return ok
}

func (v *VPP) installNat44EI() *Nat44EI {
	n := &Nat44EI{v: v, Features: map[uint32]nat44_ei.Nat44EiConfigFlags{}, Outputs: map[uint32]bool{}, IfAddrs: map[uint32]bool{}}
	one := func(m api.Message) ([]api.Message, error) { return []api.Message{m}, nil }
	rv := func(e api.VPPApiError) int32 { return int32(e) }
	const noIf = ^interface_types.InterfaceIndex(0)

	v.On("nat44_ei_show_running_config", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		rep := &nat44_ei.Nat44EiShowRunningConfigReply{}
		if n.Enabled {
			rep.Sessions, rep.UserSessions, rep.Users = 10*1024, 10*1024, 1024
			rep.InsideVrf, rep.OutsideVrf, rep.Flags = n.Cfg.InsideVrf, n.Cfg.OutsideVrf, n.Cfg.Flags
			rep.Timeouts, rep.ForwardingEnabled, rep.IpfixLoggingEnabled = n.Timeouts, n.Forwarding, n.Ipfix
		}
		return one(rep)
	})
	v.On("nat44_ei_plugin_enable_disable", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ei.Nat44EiPluginEnableDisable)
		n.mu.Lock()
		defer n.mu.Unlock()
		rep := &nat44_ei.Nat44EiPluginEnableDisableReply{}
		switch {
		case r.Enable && n.Enabled:
			rep.Retval = rv(api.FEATURE_ALREADY_ENABLED)
		case !r.Enable && !n.Enabled:
			rep.Retval = rv(api.FEATURE_ALREADY_DISABLED)
		case r.Enable:
			n.enableLocked(*r)
		default:
			n.Enabled, n.Forwarding, n.Ipfix, n.Timeouts = false, false, false, nat_types.NatTimeouts{}
			n.Features, n.Outputs, n.IfAddrs = map[uint32]nat44_ei.Nat44EiConfigFlags{}, map[uint32]bool{}, map[uint32]bool{}
			n.Addrs, n.Statics, n.Idents, n.Sessions = nil, nil, nil, nil
		}
		return one(rep)
	})
	v.On("nat44_ei_set_timeouts", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ei.Nat44EiSetTimeouts)
		n.mu.Lock()
		n.Timeouts = nat_types.NatTimeouts{UDP: r.UDP, TCPEstablished: r.TCPEstablished, TCPTransitory: r.TCPTransitory, ICMP: r.ICMP}
		n.mu.Unlock()
		return one(&nat44_ei.Nat44EiSetTimeoutsReply{})
	})
	v.On("nat44_ei_forwarding_enable_disable", func(m api.Message) ([]api.Message, error) {
		n.mu.Lock()
		n.Forwarding = m.(*nat44_ei.Nat44EiForwardingEnableDisable).Enable
		n.mu.Unlock()
		return one(&nat44_ei.Nat44EiForwardingEnableDisableReply{})
	})
	v.On("nat44_ei_ipfix_enable_disable", func(m api.Message) ([]api.Message, error) {
		n.mu.Lock()
		n.Ipfix = m.(*nat44_ei.Nat44EiIpfixEnableDisable).Enable
		n.mu.Unlock()
		return one(&nat44_ei.Nat44EiIpfixEnableDisableReply{})
	})
	v.On("nat44_ei_interface_add_del_feature", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ei.Nat44EiInterfaceAddDelFeature)
		idx := uint32(r.SwIfIndex)
		if !v.ifExists(idx) {
			return one(&nat44_ei.Nat44EiInterfaceAddDelFeatureReply{Retval: rv(api.INVALID_SW_IF_INDEX)})
		}
		n.mu.Lock()
		defer n.mu.Unlock()
		if !n.Enabled {
			return one(&nat44_ei.Nat44EiInterfaceAddDelFeatureReply{Retval: rv(api.UNSUPPORTED)})
		}
		if r.IsAdd {
			n.Features[idx] |= r.Flags
		} else {
			if n.Features[idx]&r.Flags == 0 {
				return one(&nat44_ei.Nat44EiInterfaceAddDelFeatureReply{Retval: rv(api.NO_SUCH_ENTRY)})
			}
			if n.Features[idx] &^= r.Flags; n.Features[idx] == 0 {
				delete(n.Features, idx)
			}
		}
		return one(&nat44_ei.Nat44EiInterfaceAddDelFeatureReply{})
	})
	v.On("nat44_ei_interface_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedIdx(n.Features) {
			out = append(out, &nat44_ei.Nat44EiInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx), Flags: n.Features[idx]})
		}
		return out, nil
	})
	v.On("nat44_ei_add_del_output_interface", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ei.Nat44EiAddDelOutputInterface)
		n.mu.Lock()
		defer n.mu.Unlock()
		if r.IsAdd {
			n.Outputs[uint32(r.SwIfIndex)] = true
		} else {
			if !n.Outputs[uint32(r.SwIfIndex)] {
				return one(&nat44_ei.Nat44EiAddDelOutputInterfaceReply{Retval: rv(api.NO_SUCH_ENTRY)})
			}
			delete(n.Outputs, uint32(r.SwIfIndex))
		}
		return one(&nat44_ei.Nat44EiAddDelOutputInterfaceReply{})
	})
	v.On("nat44_ei_output_interface_get", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedIdx(n.Outputs) {
			out = append(out, &nat44_ei.Nat44EiOutputInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx)})
		}
		return append(out, &nat44_ei.Nat44EiOutputInterfaceGetReply{Cursor: ^uint32(0)}), nil
	})
	v.On("nat44_ei_add_del_interface_addr", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ei.Nat44EiAddDelInterfaceAddr)
		n.mu.Lock()
		defer n.mu.Unlock()
		if r.IsAdd {
			n.IfAddrs[uint32(r.SwIfIndex)] = true
		} else {
			delete(n.IfAddrs, uint32(r.SwIfIndex))
		}
		return one(&nat44_ei.Nat44EiAddDelInterfaceAddrReply{})
	})
	v.On("nat44_ei_interface_addr_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedIdx(n.IfAddrs) {
			out = append(out, &nat44_ei.Nat44EiInterfaceAddrDetails{SwIfIndex: interface_types.InterfaceIndex(idx)})
		}
		return out, nil
	})
	v.On("nat44_ei_add_del_address_range", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ei.Nat44EiAddDelAddressRange)
		n.mu.Lock()
		defer n.mu.Unlock()
		first, last := netip.AddrFrom4(r.FirstIPAddress), netip.AddrFrom4(r.LastIPAddress)
		find := func(a netip.Addr) int {
			for i, d := range n.Addrs {
				if netip.AddrFrom4(d.IPAddress) == a {
					return i
				}
			}
			return -1
		}
		for a := first; !last.Less(a); a = a.Next() {
			if (find(a) >= 0) == r.IsAdd {
				e := api.VALUE_EXIST
				if !r.IsAdd {
					e = api.NO_SUCH_ENTRY
				}
				return one(&nat44_ei.Nat44EiAddDelAddressRangeReply{Retval: rv(e)})
			}
		}
		for a := first; !last.Less(a); a = a.Next() {
			if r.IsAdd {
				n.Addrs = append(n.Addrs, &nat44_ei.Nat44EiAddressDetails{IPAddress: a.As4(), VrfID: r.VrfID})
			} else {
				i := find(a)
				n.Addrs = append(n.Addrs[:i], n.Addrs[i+1:]...)
			}
		}
		return one(&nat44_ei.Nat44EiAddDelAddressRangeReply{})
	})
	v.On("nat44_ei_address_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		out := make([]api.Message, 0, len(n.Addrs))
		for _, a := range n.Addrs {
			c := *a
			out = append(out, &c)
		}
		ifs := sortedIdx(n.IfAddrs)
		n.mu.Unlock()
		for _, idx := range ifs { // interface-pool addresses are pool addresses in VPP's dump
			if a, ok := v.firstIPv4(idx); ok {
				out = append(out, &nat44_ei.Nat44EiAddressDetails{IPAddress: a, VrfID: ^uint32(0)})
			}
		}
		return out, nil
	})
	v.On("nat44_ei_add_del_static_mapping", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ei.Nat44EiAddDelStaticMapping)
		n.mu.Lock()
		defer n.mu.Unlock()
		for i, d := range n.Statics { // VPP matches the endpoint, never the tag
			if d.LocalIPAddress == r.LocalIPAddress && d.LocalPort == r.LocalPort && d.Protocol == r.Protocol && d.VrfID == r.VrfID {
				if r.IsAdd {
					return one(&nat44_ei.Nat44EiAddDelStaticMappingReply{Retval: rv(api.VALUE_EXIST)})
				}
				n.Statics = append(n.Statics[:i], n.Statics[i+1:]...)
				return one(&nat44_ei.Nat44EiAddDelStaticMappingReply{})
			}
		}
		if !r.IsAdd {
			return one(&nat44_ei.Nat44EiAddDelStaticMappingReply{Retval: rv(api.NO_SUCH_ENTRY)})
		}
		n.Statics = append(n.Statics, &nat44_ei.Nat44EiStaticMappingDetails{Flags: r.Flags, LocalIPAddress: r.LocalIPAddress, ExternalIPAddress: r.ExternalIPAddress,
			Protocol: r.Protocol, LocalPort: r.LocalPort, ExternalPort: r.ExternalPort, ExternalSwIfIndex: r.ExternalSwIfIndex, VrfID: r.VrfID, Tag: r.Tag})
		return one(&nat44_ei.Nat44EiAddDelStaticMappingReply{})
	})
	v.On("nat44_ei_static_mapping_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		statics := append([]*nat44_ei.Nat44EiStaticMappingDetails{}, n.Statics...)
		n.mu.Unlock()
		var resolved, toResolve []api.Message
		for _, d := range statics {
			if d.ExternalSwIfIndex != noIf {
				if a, ok := v.firstIPv4(uint32(d.ExternalSwIfIndex)); ok {
					twin := *d
					twin.ExternalSwIfIndex, twin.ExternalIPAddress = noIf, a
					resolved = append(resolved, &twin)
				}
				c := *d
				toResolve = append(toResolve, &c)
				continue
			}
			c := *d
			resolved = append(resolved, &c)
		}
		return append(resolved, toResolve...), nil
	})
	v.On("nat44_ei_add_del_identity_mapping", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ei.Nat44EiAddDelIdentityMapping)
		n.mu.Lock()
		defer n.mu.Unlock()
		for i, d := range n.Idents {
			if d.IPAddress == r.IPAddress && d.SwIfIndex == r.SwIfIndex && d.Protocol == r.Protocol && d.Port == r.Port && d.VrfID == r.VrfID {
				if r.IsAdd {
					return one(&nat44_ei.Nat44EiAddDelIdentityMappingReply{Retval: rv(api.VALUE_EXIST)})
				}
				n.Idents = append(n.Idents[:i], n.Idents[i+1:]...)
				return one(&nat44_ei.Nat44EiAddDelIdentityMappingReply{})
			}
		}
		if !r.IsAdd {
			return one(&nat44_ei.Nat44EiAddDelIdentityMappingReply{Retval: rv(api.NO_SUCH_ENTRY)})
		}
		n.Idents = append(n.Idents, &nat44_ei.Nat44EiIdentityMappingDetails{Flags: r.Flags, IPAddress: r.IPAddress, Protocol: r.Protocol, Port: r.Port, SwIfIndex: r.SwIfIndex, VrfID: r.VrfID, Tag: r.Tag})
		return one(&nat44_ei.Nat44EiAddDelIdentityMappingReply{})
	})
	v.On("nat44_ei_identity_mapping_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		idents := append([]*nat44_ei.Nat44EiIdentityMappingDetails{}, n.Idents...)
		n.mu.Unlock()
		var resolved, toResolve []api.Message
		for _, d := range idents {
			if d.SwIfIndex != noIf {
				if a, ok := v.firstIPv4(uint32(d.SwIfIndex)); ok {
					twin := *d
					twin.SwIfIndex, twin.IPAddress = noIf, a
					resolved = append(resolved, &twin)
				}
				c := *d
				toResolve = append(toResolve, &c)
				continue
			}
			c := *d
			resolved = append(resolved, &c)
		}
		return append(resolved, toResolve...), nil
	})
	v.On("nat44_ei_user_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		type user struct {
			vrf uint32
			ip  [4]uint8
		}
		count := map[user]*nat44_ei.Nat44EiUserDetails{}
		var order []user
		for _, s := range n.Sessions {
			u := user{s.VRF, s.InsideIPAddress}
			d, ok := count[u]
			if !ok {
				d = &nat44_ei.Nat44EiUserDetails{VrfID: s.VRF, IPAddress: s.InsideIPAddress}
				count[u] = d
				order = append(order, u)
			}
			if s.Flags&nat44_ei.NAT44_EI_STATIC_MAPPING != 0 {
				d.Nstaticsessions++
			} else {
				d.Nsessions++
			}
		}
		out := make([]api.Message, 0, len(order))
		for i := len(order) - 1; i >= 0; i-- { // hash order, not sorted
			out = append(out, count[order[i]])
		}
		return out, nil
	})
	v.On("nat44_ei_user_session_v2_dump", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ei.Nat44EiUserSessionV2Dump)
		n.mu.Lock()
		defer n.mu.Unlock()
		n.SessionDumps++
		var out []api.Message
		for _, s := range n.Sessions {
			if s.VRF == r.VrfID && s.InsideIPAddress == r.IPAddress {
				d := s.Nat44EiUserSessionV2Details
				out = append(out, &d)
			}
		}
		return out, nil
	})
	v.On("nat44_ei_del_session", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ei.Nat44EiDelSession)
		n.mu.Lock()
		defer n.mu.Unlock()
		if !n.Enabled {
			return one(&nat44_ei.Nat44EiDelSessionReply{Retval: rv(api.UNSUPPORTED)})
		}
		for i, s := range n.Sessions { // nat44_ei_del_session: in2out lookup by (address, port, protocol, fib)
			match := s.VRF == r.VrfID && s.Protocol == uint16(r.Protocol)
			if r.Flags&nat44_ei.NAT44_EI_IF_INSIDE != 0 {
				match = match && s.InsideIPAddress == r.Address && s.InsidePort == r.Port
			} else {
				match = match && s.OutsideIPAddress == r.Address && s.OutsidePort == r.Port
			}
			if match {
				n.Sessions = append(n.Sessions[:i], n.Sessions[i+1:]...)
				return one(&nat44_ei.Nat44EiDelSessionReply{})
			}
		}
		return one(&nat44_ei.Nat44EiDelSessionReply{Retval: rv(api.NO_SUCH_ENTRY)})
	})
	return n
}
