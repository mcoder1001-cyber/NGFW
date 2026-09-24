package coretest

// F-nat44-ed-sessions: a stateful model of the nat44-ed plugin (DF-3's descriptors and session helpers) on top of
// the VPP model, so the agent's `nat` domain (desired/nat.go, the NatSessions RPC and the kill action) is unit-tested
// against the same fake as every other domain. It follows DF-3's per-package fake (descriptors/nat44ed/nat44ed_test.go)
// and VPP 26.06's API handlers where the agent depends on them: pool addresses are stored one by one, static /
// identity mappings bound to an interface are dumped twice (the resolved twin, then the to-resolve record), deletes
// of static mappings match the endpoint (not the tag), sessions are dumped per user and deleted by the full ED
// 5-tuple (nat44_ed_del_session: NO_SUCH_ENTRY otherwise, UNSUPPORTED while the plugin is disabled).

import (
	"net/netip"
	"sort"
	"sync"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/binapi/nat_types"
)

// NatSession is one modelled NAT44-ED session: the details VPP's per-user dump returns plus the user's VRF.
type NatSession struct {
	VRF uint32
	nat44_ed.Nat44UserSessionV3Details
}

// Nat44ED is the nat44-ed plugin model; tests may read and seed the exported fields under Lock/Unlock.
type Nat44ED struct {
	v  *VPP
	mu sync.Mutex

	Enabled    bool
	Cfg        nat44_ed.Nat44EdPluginEnableDisable
	Timeouts   nat_types.NatTimeouts
	Forwarding bool
	Features   map[uint32]nat_types.NatConfigFlags
	Outputs    map[uint32]bool
	IfAddrs    map[uint32]nat_types.NatConfigFlags
	Addrs      []*nat44_ed.Nat44AddressDetails
	Statics    []*nat44_ed.Nat44StaticMappingDetails
	Idents     []*nat44_ed.Nat44IdentityMappingDetails
	LBs        []*nat44_ed.Nat44LbStaticMappingDetails
	Sessions   []NatSession
	// SessionDumps counts nat44_user_session_v3_dump requests (paging tests: only the users of a page are dumped).
	SessionDumps int
}

// Lock / Unlock guard the exported fields while a test seeds or inspects them.
func (n *Nat44ED) Lock()   { n.mu.Lock() }
func (n *Nat44ED) Unlock() { n.mu.Unlock() }

// NatEnable enables the modelled plugin as a test fixture would (VPP defaults: 63×1024 sessions, default
// timeouts), without going through a descriptor.
func (n *Nat44ED) NatEnable() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.enableLocked(nat44_ed.Nat44EdPluginEnableDisable{Enable: true})
}

func (n *Nat44ED) enableLocked(r nat44_ed.Nat44EdPluginEnableDisable) {
	n.Enabled, n.Cfg = true, r
	if n.Cfg.Sessions == 0 {
		n.Cfg.Sessions = 63 * 1024
	}
	n.Timeouts = nat_types.NatTimeouts{UDP: 300, TCPEstablished: 7440, TCPTransitory: 240, ICMP: 60}
}

// AddNatSession seeds one session (inside/outside/external endpoints in "a.b.c.d", host-order ports).
func (n *Nat44ED) AddNatSession(vrf uint32, proto uint16, in string, inPort uint16, out string, outPort uint16, ext string, extPort uint16) {
	n.mu.Lock()
	defer n.mu.Unlock()
	d := nat44_ed.Nat44UserSessionV3Details{
		InsideIPAddress: ip4(in), InsidePort: inPort, OutsideIPAddress: ip4(out), OutsidePort: outPort,
		ExtHostAddress: ip4(ext), ExtHostPort: extPort, ExtHostNatAddress: ip4(ext), ExtHostNatPort: extPort,
		Protocol: proto, TotalBytes: 100, TotalPkts: 2, TimeSinceLastHeard: 1,
	}
	n.Sessions = append(n.Sessions, NatSession{VRF: vrf, Nat44UserSessionV3Details: d})
}

// SessionCount returns the number of modelled sessions.
func (n *Nat44ED) SessionCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.Sessions)
}

// Empty reports whether the model holds no configuration object (sessions are state).
func (n *Nat44ED) Empty() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.Features)+len(n.Outputs)+len(n.IfAddrs)+len(n.Addrs)+len(n.Statics)+len(n.Idents)+len(n.LBs) == 0
}

// SetIPv4 gives a modelled interface an IPv4 address ("a.b.c.d/len"), e.g. the address an interface pool uses.
func (v *VPP) SetIPv4(name, prefix string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, i := range v.Ifaces {
		if i.Name == name {
			i.Addrs[netip.MustParsePrefix(prefix).String()] = true
			return true
		}
	}
	return false
}

func ip4(s string) [4]uint8 {
	if s == "" {
		return [4]uint8{}
	}
	return netip.MustParseAddr(s).As4()
}

// firstIPv4 is the lowest IPv4 address of interface idx (VPP resolves interface-bound mappings to it).
func (v *VPP) firstIPv4(idx uint32) ([4]uint8, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	i, ok := v.Ifaces[idx]
	if !ok {
		return [4]uint8{}, false
	}
	var as []netip.Addr
	for a := range i.Addrs {
		if p := netip.MustParsePrefix(a); p.Addr().Is4() {
			as = append(as, p.Addr())
		}
	}
	if len(as) == 0 {
		return [4]uint8{}, false
	}
	sort.Slice(as, func(a, b int) bool { return as[a].Less(as[b]) })
	return as[0].As4(), true
}

// nat44EDModels maps each VPP model to its nat44-ed model (installed by New through the extensions seam).
var nat44EDModels sync.Map

func init() {
	extensions = append(extensions, func(v *VPP) { nat44EDModels.Store(v, v.installNat44ED()) })
}

// Nat44ED returns the nat44-ed plugin model of v (every model built by New has one; the plugin starts disabled).
func (v *VPP) Nat44ED() *Nat44ED {
	m, ok := nat44EDModels.Load(v)
	if !ok {
		m, _ = nat44EDModels.LoadOrStore(v, v.installNat44ED())
	}
	return m.(*Nat44ED)
}

// installNat44ED installs the nat44-ed (and the nat44-ei running-config) handlers and returns the model.
func (v *VPP) installNat44ED() *Nat44ED {
	n := &Nat44ED{v: v, Features: map[uint32]nat_types.NatConfigFlags{}, Outputs: map[uint32]bool{}, IfAddrs: map[uint32]nat_types.NatConfigFlags{}}
	one := func(m api.Message) ([]api.Message, error) { return []api.Message{m}, nil }
	rv := func(e api.VPPApiError) int32 { return int32(e) }

	v.On("nat44_ei_show_running_config", func(api.Message) ([]api.Message, error) {
		return one(&nat44_ei.Nat44EiShowRunningConfigReply{}) // EI off: ED and EI are exclusive
	})
	v.On("nat44_show_running_config", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		rep := &nat44_ed.Nat44ShowRunningConfigReply{Flags: nat44_ed.NAT44_IS_ENDPOINT_DEPENDENT}
		if n.Enabled {
			rep.Sessions, rep.InsideVrf, rep.OutsideVrf = n.Cfg.Sessions, n.Cfg.InsideVrf, n.Cfg.OutsideVrf
			rep.Flags |= n.Cfg.Flags
			rep.Timeouts, rep.ForwardingEnabled = n.Timeouts, n.Forwarding
		}
		return one(rep)
	})
	v.On("nat44_ed_plugin_enable_disable", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ed.Nat44EdPluginEnableDisable)
		n.mu.Lock()
		defer n.mu.Unlock()
		rep := &nat44_ed.Nat44EdPluginEnableDisableReply{}
		switch {
		case r.Enable && n.Enabled:
			rep.Retval = rv(api.FEATURE_ALREADY_ENABLED)
		case !r.Enable && !n.Enabled:
			rep.Retval = rv(api.FEATURE_ALREADY_DISABLED)
		case r.Enable:
			n.enableLocked(*r)
		default: // VPP wipes everything on disable
			n.Enabled, n.Forwarding, n.Timeouts = false, false, nat_types.NatTimeouts{}
			n.Features, n.Outputs, n.IfAddrs = map[uint32]nat_types.NatConfigFlags{}, map[uint32]bool{}, map[uint32]nat_types.NatConfigFlags{}
			n.Addrs, n.Statics, n.Idents, n.LBs, n.Sessions = nil, nil, nil, nil, nil
		}
		return one(rep)
	})
	v.On("nat_set_timeouts", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ed.NatSetTimeouts)
		n.mu.Lock()
		n.Timeouts = nat_types.NatTimeouts{UDP: r.UDP, TCPEstablished: r.TCPEstablished, TCPTransitory: r.TCPTransitory, ICMP: r.ICMP}
		n.mu.Unlock()
		return one(&nat44_ed.NatSetTimeoutsReply{})
	})
	v.On("nat44_forwarding_enable_disable", func(m api.Message) ([]api.Message, error) {
		n.mu.Lock()
		n.Forwarding = m.(*nat44_ed.Nat44ForwardingEnableDisable).Enable
		n.mu.Unlock()
		return one(&nat44_ed.Nat44ForwardingEnableDisableReply{})
	})
	v.On("nat44_interface_add_del_feature", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ed.Nat44InterfaceAddDelFeature)
		n.mu.Lock()
		defer n.mu.Unlock()
		idx := uint32(r.SwIfIndex)
		if r.IsAdd {
			n.Features[idx] |= r.Flags
		} else {
			if n.Features[idx]&r.Flags == 0 {
				return one(&nat44_ed.Nat44InterfaceAddDelFeatureReply{Retval: rv(api.NO_SUCH_ENTRY)})
			}
			n.Features[idx] &^= r.Flags
			if n.Features[idx] == 0 {
				delete(n.Features, idx)
			}
		}
		return one(&nat44_ed.Nat44InterfaceAddDelFeatureReply{})
	})
	v.On("nat44_interface_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedIdx(n.Features) {
			out = append(out, &nat44_ed.Nat44InterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx), Flags: n.Features[idx]})
		}
		return out, nil
	})
	v.On("nat44_ed_add_del_output_interface", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ed.Nat44EdAddDelOutputInterface)
		n.mu.Lock()
		defer n.mu.Unlock()
		if r.IsAdd {
			n.Outputs[uint32(r.SwIfIndex)] = true
		} else {
			delete(n.Outputs, uint32(r.SwIfIndex))
		}
		return one(&nat44_ed.Nat44EdAddDelOutputInterfaceReply{})
	})
	v.On("nat44_ed_output_interface_get", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedIdx(n.Outputs) {
			out = append(out, &nat44_ed.Nat44EdOutputInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx)})
		}
		return append(out, &nat44_ed.Nat44EdOutputInterfaceGetReply{Cursor: ^uint32(0)}), nil
	})
	v.On("nat44_add_del_interface_addr", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ed.Nat44AddDelInterfaceAddr)
		n.mu.Lock()
		defer n.mu.Unlock()
		if r.IsAdd {
			n.IfAddrs[uint32(r.SwIfIndex)] = r.Flags
		} else {
			delete(n.IfAddrs, uint32(r.SwIfIndex))
		}
		return one(&nat44_ed.Nat44AddDelInterfaceAddrReply{})
	})
	v.On("nat44_interface_addr_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		var out []api.Message
		for _, idx := range sortedIdx(n.IfAddrs) {
			out = append(out, &nat44_ed.Nat44InterfaceAddrDetails{SwIfIndex: interface_types.InterfaceIndex(idx), Flags: n.IfAddrs[idx]})
		}
		return out, nil
	})
	v.On("nat44_add_del_address_range", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ed.Nat44AddDelAddressRange)
		n.mu.Lock()
		defer n.mu.Unlock()
		first, last := netip.AddrFrom4(r.FirstIPAddress), netip.AddrFrom4(r.LastIPAddress)
		twice := r.Flags & nat_types.NAT_IS_TWICE_NAT
		find := func(a netip.Addr) int {
			for i, d := range n.Addrs {
				if netip.AddrFrom4(d.IPAddress) == a && d.Flags&nat_types.NAT_IS_TWICE_NAT == twice {
					return i
				}
			}
			return -1
		}
		for a := first; !last.Less(a); a = a.Next() { // VPP checks the whole range first
			if (find(a) >= 0) == r.IsAdd {
				e := api.VALUE_EXIST
				if !r.IsAdd {
					e = api.NO_SUCH_ENTRY
				}
				return one(&nat44_ed.Nat44AddDelAddressRangeReply{Retval: rv(e)})
			}
		}
		for a := first; !last.Less(a); a = a.Next() {
			if r.IsAdd {
				n.Addrs = append(n.Addrs, &nat44_ed.Nat44AddressDetails{IPAddress: a.As4(), Flags: twice, VrfID: r.VrfID})
			} else {
				i := find(a)
				n.Addrs = append(n.Addrs[:i], n.Addrs[i+1:]...)
			}
		}
		return one(&nat44_ed.Nat44AddDelAddressRangeReply{})
	})
	v.On("nat44_address_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		out := make([]api.Message, 0, len(n.Addrs))
		for _, a := range n.Addrs {
			c := *a
			out = append(out, &c)
		}
		return out, nil
	})
	v.On("nat44_add_del_static_mapping_v2", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ed.Nat44AddDelStaticMappingV2)
		n.mu.Lock()
		defer n.mu.Unlock()
		same := func(d *nat44_ed.Nat44StaticMappingDetails) bool { // VPP matches the endpoint, never the tag
			return d.LocalIPAddress == r.LocalIPAddress && d.LocalPort == r.LocalPort && d.Protocol == r.Protocol && d.VrfID == r.VrfID && d.ExternalPort == r.ExternalPort
		}
		for i, d := range n.Statics {
			if same(d) {
				if r.IsAdd {
					return one(&nat44_ed.Nat44AddDelStaticMappingV2Reply{Retval: rv(api.VALUE_EXIST)})
				}
				n.Statics = append(n.Statics[:i], n.Statics[i+1:]...)
				return one(&nat44_ed.Nat44AddDelStaticMappingV2Reply{})
			}
		}
		if !r.IsAdd {
			return one(&nat44_ed.Nat44AddDelStaticMappingV2Reply{Retval: rv(api.NO_SUCH_ENTRY)})
		}
		n.Statics = append(n.Statics, &nat44_ed.Nat44StaticMappingDetails{Flags: r.Flags, LocalIPAddress: r.LocalIPAddress, ExternalIPAddress: r.ExternalIPAddress,
			Protocol: r.Protocol, LocalPort: r.LocalPort, ExternalPort: r.ExternalPort, ExternalSwIfIndex: r.ExternalSwIfIndex, VrfID: r.VrfID, Tag: r.Tag})
		return one(&nat44_ed.Nat44AddDelStaticMappingV2Reply{})
	})
	v.On("nat44_static_mapping_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		statics := append([]*nat44_ed.Nat44StaticMappingDetails{}, n.Statics...)
		n.mu.Unlock()
		// nat44_ed_api.c: resolved mappings first, then the to-resolve records of interface-bound ones
		var resolved, toResolve []api.Message
		for _, d := range statics {
			if d.ExternalSwIfIndex != ^interface_types.InterfaceIndex(0) {
				if a, ok := v.firstIPv4(uint32(d.ExternalSwIfIndex)); ok {
					twin := *d
					twin.ExternalSwIfIndex, twin.ExternalIPAddress = ^interface_types.InterfaceIndex(0), a
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
	v.On("nat44_add_del_identity_mapping", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ed.Nat44AddDelIdentityMapping)
		n.mu.Lock()
		defer n.mu.Unlock()
		for i, d := range n.Idents {
			if d.IPAddress == r.IPAddress && d.SwIfIndex == r.SwIfIndex && d.Protocol == r.Protocol && d.Port == r.Port && d.VrfID == r.VrfID {
				if r.IsAdd {
					return one(&nat44_ed.Nat44AddDelIdentityMappingReply{Retval: rv(api.VALUE_EXIST)})
				}
				n.Idents = append(n.Idents[:i], n.Idents[i+1:]...)
				return one(&nat44_ed.Nat44AddDelIdentityMappingReply{})
			}
		}
		if !r.IsAdd {
			return one(&nat44_ed.Nat44AddDelIdentityMappingReply{Retval: rv(api.NO_SUCH_ENTRY)})
		}
		n.Idents = append(n.Idents, &nat44_ed.Nat44IdentityMappingDetails{Flags: r.Flags, IPAddress: r.IPAddress, Protocol: r.Protocol, Port: r.Port, SwIfIndex: r.SwIfIndex, VrfID: r.VrfID, Tag: r.Tag})
		return one(&nat44_ed.Nat44AddDelIdentityMappingReply{})
	})
	v.On("nat44_identity_mapping_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		idents := append([]*nat44_ed.Nat44IdentityMappingDetails{}, n.Idents...)
		n.mu.Unlock()
		var resolved, toResolve []api.Message
		for _, d := range idents {
			if d.SwIfIndex != ^interface_types.InterfaceIndex(0) {
				if a, ok := v.firstIPv4(uint32(d.SwIfIndex)); ok {
					twin := *d
					twin.SwIfIndex, twin.IPAddress = ^interface_types.InterfaceIndex(0), a
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
	v.On("nat44_add_del_lb_static_mapping", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ed.Nat44AddDelLbStaticMapping)
		n.mu.Lock()
		defer n.mu.Unlock()
		for i, d := range n.LBs {
			if d.ExternalAddr == r.ExternalAddr && d.ExternalPort == r.ExternalPort && d.Protocol == r.Protocol {
				if r.IsAdd {
					return one(&nat44_ed.Nat44AddDelLbStaticMappingReply{Retval: rv(api.VALUE_EXIST)})
				}
				n.LBs = append(n.LBs[:i], n.LBs[i+1:]...)
				return one(&nat44_ed.Nat44AddDelLbStaticMappingReply{})
			}
		}
		if !r.IsAdd {
			return one(&nat44_ed.Nat44AddDelLbStaticMappingReply{Retval: rv(api.NO_SUCH_ENTRY)})
		}
		n.LBs = append(n.LBs, &nat44_ed.Nat44LbStaticMappingDetails{ExternalAddr: r.ExternalAddr, ExternalPort: r.ExternalPort, Protocol: r.Protocol,
			Flags: r.Flags, Affinity: r.Affinity, Tag: r.Tag, LocalNum: r.LocalNum, Locals: append([]nat44_ed.Nat44LbAddrPort{}, r.Locals...)})
		return one(&nat44_ed.Nat44AddDelLbStaticMappingReply{})
	})
	v.On("nat44_lb_static_mapping_add_del_local", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ed.Nat44LbStaticMappingAddDelLocal)
		n.mu.Lock()
		defer n.mu.Unlock()
		for _, d := range n.LBs {
			if d.ExternalAddr != r.ExternalAddr || d.ExternalPort != r.ExternalPort || d.Protocol != r.Protocol {
				continue
			}
			if r.IsAdd {
				d.Locals = append(d.Locals, r.Local)
			} else {
				for i, l := range d.Locals {
					if l == r.Local {
						d.Locals = append(d.Locals[:i], d.Locals[i+1:]...)
						break
					}
				}
			}
			d.LocalNum = uint32(len(d.Locals)) //nolint:gosec // test data
			return one(&nat44_ed.Nat44LbStaticMappingAddDelLocalReply{})
		}
		return one(&nat44_ed.Nat44LbStaticMappingAddDelLocalReply{Retval: rv(api.NO_SUCH_ENTRY)})
	})
	v.On("nat44_lb_static_mapping_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		out := make([]api.Message, 0, len(n.LBs))
		for _, d := range n.LBs {
			c := *d
			c.Locals = append([]nat44_ed.Nat44LbAddrPort{}, d.Locals...)
			out = append(out, &c)
		}
		return out, nil
	})
	v.On("nat44_ed_vrf_tables_v2_dump", func(api.Message) ([]api.Message, error) { return nil, nil })
	v.On("nat44_user_dump", func(api.Message) ([]api.Message, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		type user struct {
			vrf uint32
			ip  [4]uint8
		}
		count := map[user]*nat44_ed.Nat44UserDetails{}
		var order []user
		for _, s := range n.Sessions {
			u := user{s.VRF, s.InsideIPAddress}
			d, ok := count[u]
			if !ok {
				d = &nat44_ed.Nat44UserDetails{VrfID: s.VRF, IPAddress: s.InsideIPAddress}
				count[u] = d
				order = append(order, u)
			}
			if s.Flags&nat_types.NAT_IS_STATIC != 0 {
				d.Nstaticsessions++
			} else {
				d.Nsessions++
			}
		}
		out := make([]api.Message, 0, len(order))
		for i := len(order) - 1; i >= 0; i-- { // VPP's hash order is not sorted: the agent must sort
			out = append(out, count[order[i]])
		}
		return out, nil
	})
	v.On("nat44_user_session_v3_dump", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ed.Nat44UserSessionV3Dump)
		n.mu.Lock()
		defer n.mu.Unlock()
		n.SessionDumps++
		var out []api.Message
		for _, s := range n.Sessions {
			if s.VRF == r.VrfID && s.InsideIPAddress == r.IPAddress {
				d := s.Nat44UserSessionV3Details
				out = append(out, &d)
			}
		}
		return out, nil
	})
	v.On("nat44_del_session", func(m api.Message) ([]api.Message, error) {
		r := m.(*nat44_ed.Nat44DelSession)
		n.mu.Lock()
		defer n.mu.Unlock()
		if !n.Enabled {
			return one(&nat44_ed.Nat44DelSessionReply{Retval: rv(api.UNSUPPORTED)})
		}
		for i, s := range n.Sessions {
			if r.Flags&nat_types.NAT_IS_INSIDE != 0 && s.VRF == r.VrfID && s.InsideIPAddress == r.Address && s.InsidePort == r.Port &&
				uint8(s.Protocol) == r.Protocol && s.ExtHostAddress == r.ExtHostAddress && s.ExtHostPort == r.ExtHostPort { //nolint:gosec // IP protocol numbers are 8-bit
				n.Sessions = append(n.Sessions[:i], n.Sessions[i+1:]...)
				return one(&nat44_ed.Nat44DelSessionReply{})
			}
		}
		return one(&nat44_ed.Nat44DelSessionReply{Retval: rv(api.NO_SUCH_ENTRY)})
	})
	return n
}

func sortedIdx[V any](m map[uint32]V) []uint32 {
	out := make([]uint32, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	return out
}
