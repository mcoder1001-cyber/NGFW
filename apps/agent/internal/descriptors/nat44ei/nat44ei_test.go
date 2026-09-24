package nat44ei_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat44_ei"
	"ngfw/agent/binapi/nat_types"
	"ngfw/agent/internal/descriptors/nat44ei"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

type fakeEI struct {
	*fake.Client
	enabled  bool
	cfg      nat44_ei.Nat44EiPluginEnableDisable
	timeouts nat_types.NatTimeouts
	fwd      bool
	ipfix    bool
	features map[uint32]nat44_ei.Nat44EiConfigFlags
	outputs  map[uint32]nat44_ei.Nat44EiConfigFlags
	ifAddrs  map[uint32]bool
	addrs    []*nat44_ei.Nat44EiAddressDetails
	statics  []*nat44_ei.Nat44EiStaticMappingDetails
	idents   []*nat44_ei.Nat44EiIdentityMappingDetails
	ed       bool // nat44-ed enabled (mutually exclusive)
}

var owner = natcommon.WithGlobalsOwner(true)

func newFakeEI() *fakeEI {
	f := &fakeEI{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})),
		features: map[uint32]nat44_ei.Nat44EiConfigFlags{}, outputs: map[uint32]nat44_ei.Nat44EiConfigFlags{}, ifAddrs: map[uint32]bool{}}
	ifaces := []*interfaces.SwInterfaceDetails{
		{SwIfIndex: 0, InterfaceName: "local0"},
		{SwIfIndex: 1, InterfaceName: "loop900", Tag: "w9:loop900"},
		{SwIfIndex: 2, InterfaceName: "loop901", Tag: "w9:loop901"},
		{SwIfIndex: 3, InterfaceName: "loop300", Tag: "w3:loop300"},
	}
	f.Reply("ip_address_dump") // loopbacks of the fake carry no addresses
	f.On("sw_interface_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*interfaces.SwInterfaceDump)
		var out []api.Message
		for _, i := range ifaces {
			if !r.NameFilterValid || len(i.InterfaceName) >= len(r.NameFilter) && i.InterfaceName[:len(r.NameFilter)] == r.NameFilter {
				out = append(out, i)
			}
		}
		return out, nil
	})
	f.On("nat44_show_running_config", func(api.Message) ([]api.Message, error) {
		rep := &nat44_ed.Nat44ShowRunningConfigReply{}
		if f.ed {
			rep.Sessions = 1
		}
		return []api.Message{rep}, nil
	})
	f.On("nat44_ei_show_running_config", func(api.Message) ([]api.Message, error) {
		rep := &nat44_ei.Nat44EiShowRunningConfigReply{}
		if f.enabled {
			rep.Sessions, rep.UserSessions, rep.InsideVrf, rep.OutsideVrf, rep.Flags = 10*1024, 10*1024, f.cfg.InsideVrf, f.cfg.OutsideVrf, f.cfg.Flags
			rep.Timeouts, rep.ForwardingEnabled, rep.IpfixLoggingEnabled = f.timeouts, f.fwd, f.ipfix
		}
		return []api.Message{rep}, nil
	})
	f.On("nat44_ei_plugin_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ei.Nat44EiPluginEnableDisable)
		rep := &nat44_ei.Nat44EiPluginEnableDisableReply{}
		switch {
		case r.Enable && f.enabled:
			rep.Retval = int32(api.FEATURE_ALREADY_ENABLED)
		case !r.Enable && !f.enabled:
			rep.Retval = int32(api.FEATURE_ALREADY_DISABLED)
		case r.Enable:
			f.enabled, f.cfg = true, *r
			f.timeouts = nat_types.NatTimeouts{UDP: 300, TCPEstablished: 7440, TCPTransitory: 240, ICMP: 60}
		default:
			f.enabled, f.fwd, f.ipfix = false, false, false
		}
		return []api.Message{rep}, nil
	})
	f.On("nat44_ei_set_timeouts", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ei.Nat44EiSetTimeouts)
		f.timeouts = nat_types.NatTimeouts{UDP: r.UDP, TCPEstablished: r.TCPEstablished, TCPTransitory: r.TCPTransitory, ICMP: r.ICMP}
		return []api.Message{&nat44_ei.Nat44EiSetTimeoutsReply{}}, nil
	})
	f.On("nat44_ei_forwarding_enable_disable", func(req api.Message) ([]api.Message, error) {
		f.fwd = req.(*nat44_ei.Nat44EiForwardingEnableDisable).Enable
		return []api.Message{&nat44_ei.Nat44EiForwardingEnableDisableReply{}}, nil
	})
	f.On("nat44_ei_ipfix_enable_disable", func(req api.Message) ([]api.Message, error) {
		f.ipfix = req.(*nat44_ei.Nat44EiIpfixEnableDisable).Enable
		return []api.Message{&nat44_ei.Nat44EiIpfixEnableDisableReply{}}, nil
	})
	flagMap := func(m map[uint32]nat44_ei.Nat44EiConfigFlags, idx uint32, fl nat44_ei.Nat44EiConfigFlags, add bool) {
		if add {
			m[idx] |= fl
		} else {
			m[idx] &^= fl
			if m[idx] == 0 {
				delete(m, idx)
			}
		}
	}
	f.On("nat44_ei_interface_add_del_feature", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ei.Nat44EiInterfaceAddDelFeature)
		flagMap(f.features, uint32(r.SwIfIndex), r.Flags, r.IsAdd)
		return []api.Message{&nat44_ei.Nat44EiInterfaceAddDelFeatureReply{}}, nil
	})
	f.On("nat44_ei_interface_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 10; idx++ {
			if fl, ok := f.features[idx]; ok {
				out = append(out, &nat44_ei.Nat44EiInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx), Flags: fl})
			}
		}
		return out, nil
	})
	f.On("nat44_ei_add_del_output_interface", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ei.Nat44EiAddDelOutputInterface)
		if r.IsAdd {
			f.outputs[uint32(r.SwIfIndex)] = 1
		} else {
			delete(f.outputs, uint32(r.SwIfIndex))
		}
		return []api.Message{&nat44_ei.Nat44EiAddDelOutputInterfaceReply{}}, nil
	})
	f.On("nat44_ei_output_interface_get", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 10; idx++ {
			if _, ok := f.outputs[idx]; ok {
				out = append(out, &nat44_ei.Nat44EiOutputInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx)})
			}
		}
		return append(out, &nat44_ei.Nat44EiOutputInterfaceGetReply{Cursor: ^uint32(0)}), nil
	})
	f.On("nat44_ei_add_del_interface_addr", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ei.Nat44EiAddDelInterfaceAddr)
		if r.IsAdd {
			f.ifAddrs[uint32(r.SwIfIndex)] = true
		} else {
			delete(f.ifAddrs, uint32(r.SwIfIndex))
		}
		return []api.Message{&nat44_ei.Nat44EiAddDelInterfaceAddrReply{}}, nil
	})
	f.On("nat44_ei_interface_addr_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 10; idx++ {
			if f.ifAddrs[idx] {
				out = append(out, &nat44_ei.Nat44EiInterfaceAddrDetails{SwIfIndex: interface_types.InterfaceIndex(idx)})
			}
		}
		return out, nil
	})
	f.On("nat44_ei_add_del_address_range", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ei.Nat44EiAddDelAddressRange)
		for a := r.FirstIPAddress; ; a[3]++ {
			if r.IsAdd {
				f.addrs = append(f.addrs, &nat44_ei.Nat44EiAddressDetails{IPAddress: a, VrfID: r.VrfID})
			} else {
				for i, d := range f.addrs {
					if d.IPAddress == a {
						f.addrs = append(f.addrs[:i], f.addrs[i+1:]...)
						break
					}
				}
			}
			if a == r.LastIPAddress {
				break
			}
		}
		return []api.Message{&nat44_ei.Nat44EiAddDelAddressRangeReply{}}, nil
	})
	f.On("nat44_ei_address_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(f.addrs))
		for _, a := range f.addrs {
			out = append(out, a)
		}
		return out, nil
	})
	f.On("nat44_ei_add_del_static_mapping", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ei.Nat44EiAddDelStaticMapping)
		if r.IsAdd {
			f.statics = append(f.statics, &nat44_ei.Nat44EiStaticMappingDetails{Flags: r.Flags, LocalIPAddress: r.LocalIPAddress, ExternalIPAddress: r.ExternalIPAddress, Protocol: r.Protocol, LocalPort: r.LocalPort, ExternalPort: r.ExternalPort, ExternalSwIfIndex: r.ExternalSwIfIndex, VrfID: r.VrfID, Tag: r.Tag})
		} else {
			for i, m := range f.statics {
				if m.Tag == r.Tag {
					f.statics = append(f.statics[:i], f.statics[i+1:]...)
					break
				}
			}
		}
		return []api.Message{&nat44_ei.Nat44EiAddDelStaticMappingReply{}}, nil
	})
	f.On("nat44_ei_static_mapping_dump", func(api.Message) ([]api.Message, error) {
		// nat44_ei_api.c: resolved entries, then the to-resolve records (finding 2 twins)
		var resolved, toResolve []api.Message
		for _, m := range f.statics {
			if m.ExternalSwIfIndex != ^interface_types.InterfaceIndex(0) {
				twin := *m
				twin.ExternalSwIfIndex, twin.ExternalIPAddress = ^interface_types.InterfaceIndex(0), [4]uint8{10, 9, 40, 1}
				resolved, toResolve = append(resolved, &twin), append(toResolve, m)
				continue
			}
			resolved = append(resolved, m)
		}
		return append(resolved, toResolve...), nil
	})
	f.On("nat44_ei_add_del_identity_mapping", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat44_ei.Nat44EiAddDelIdentityMapping)
		if r.IsAdd {
			f.idents = append(f.idents, &nat44_ei.Nat44EiIdentityMappingDetails{Flags: r.Flags, IPAddress: r.IPAddress, Protocol: r.Protocol, Port: r.Port, SwIfIndex: r.SwIfIndex, VrfID: r.VrfID, Tag: r.Tag})
		} else {
			for i, m := range f.idents {
				if m.Tag == r.Tag {
					f.idents = append(f.idents[:i], f.idents[i+1:]...)
					break
				}
			}
		}
		return []api.Message{&nat44_ei.Nat44EiAddDelIdentityMappingReply{}}, nil
	})
	f.On("nat44_ei_identity_mapping_dump", func(api.Message) ([]api.Message, error) {
		var resolved, toResolve []api.Message
		for _, m := range f.idents {
			if m.SwIfIndex != ^interface_types.InterfaceIndex(0) {
				twin := *m
				twin.SwIfIndex, twin.IPAddress = ^interface_types.InterfaceIndex(0), [4]uint8{10, 9, 40, 1}
				resolved, toResolve = append(resolved, &twin), append(toResolve, m)
				continue
			}
			resolved = append(resolved, m)
		}
		return append(resolved, toResolve...), nil
	})
	f.Reply("nat44_ei_user_dump", &nat44_ei.Nat44EiUserDetails{IPAddress: [4]uint8{192, 168, 9, 2}, Nsessions: 1})
	f.Reply("nat44_ei_user_session_v2_dump", &nat44_ei.Nat44EiUserSessionV2Details{InsideIPAddress: [4]uint8{192, 168, 9, 2}, InsidePort: 1000, Protocol: 6, Flags: nat44_ei.NAT44_EI_STATIC_MAPPING})
	f.Reply("nat44_ei_del_session", &nat44_ei.Nat44EiDelSessionReply{})
	return f
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	p := nat44ei.Register(reg, newFakeEI(), "w9")
	if reg.Len() != 10 || len(p.Descriptors()) != 10 {
		t.Fatalf("registered %d", reg.Len())
	}
}

func TestSingletons(t *testing.T) {
	f := newFakeEI()
	p := nat44ei.New(f, "w9", owner)
	ctx := context.Background()
	en := natcommon.MustEncode(&nat44ei.EnableSpec{InsideVRF: 9001, StaticMappingOnly: true})
	if len(nattest.Keys(t, p.Enable)) != 0 || nattest.Apply(t, p.Enable, en) != 1 || nattest.Apply(t, p.Enable, en) != 0 {
		t.Fatal("enable create/idempotent")
	}
	req := f.CallsNamed("nat44_ei_plugin_enable_disable")[0].(*nat44_ei.Nat44EiPluginEnableDisable)
	if !req.Enable || req.InsideVrf != 9001 || req.Flags != nat44_ei.NAT44_EI_STATIC_MAPPING_ONLY {
		t.Fatalf("enable request %+v", req)
	}
	// non-owner (D-071): requires, never sets
	w3 := nat44ei.New(f, "w3")
	if _, err := w3.Enable.Create(ctx, natcommon.MustEncode(&nat44ei.EnableSpec{})); !errors.Is(err, natcommon.ErrGlobalMismatch) {
		t.Fatalf("non-owner mismatch: %v", err)
	}
	if _, err := w3.Enable.Create(ctx, en); err != nil {
		t.Fatalf("non-owner compatible: %v", err)
	}
	if err := w3.Enable.Delete(ctx, en, nil); err != nil || !f.enabled {
		t.Fatalf("non-owner delete must not disable: %v", err)
	}
	if _, err := w3.Timeouts.Create(ctx, natcommon.MustEncode(&nat44ei.TimeoutsSpec{UDP: 1, TCPEstablished: 2, TCPTransitory: 3, ICMP: 4})); !errors.Is(err, natcommon.ErrGlobalMismatch) {
		t.Fatalf("non-owner timeouts: %v", err)
	}
	if _, err := w3.Ipfix.Create(ctx, natcommon.MustEncode(&nat44ei.IpfixSpec{})); !errors.Is(err, natcommon.ErrGlobalNotSet) && err != nil {
		t.Fatalf("non-owner ipfix: %v", err)
	}
	if n := len(f.CallsNamed("nat44_ei_plugin_enable_disable")) + len(f.CallsNamed("nat44_ei_set_timeouts")) + len(f.CallsNamed("nat44_ei_ipfix_enable_disable")); n != 1 {
		t.Fatalf("non-owner sent global-changing messages (%d calls total, want the owner's 1 enable)", n)
	}
	// owner Update on an empty plugin: disable + enable
	if _, err := p.Enable.Update(ctx, en, natcommon.MustEncode(&nat44ei.EnableSpec{InsideVRF: 9002}), nil); err != nil || f.cfg.InsideVrf != 9002 {
		t.Fatalf("update: %v", err)
	}
	if _, err := p.Enable.Update(ctx, natcommon.MustEncode(&nat44ei.EnableSpec{InsideVRF: 9002}), en, nil); err != nil {
		t.Fatalf("update back: %v", err)
	}
	tmo := natcommon.MustEncode(&nat44ei.TimeoutsSpec{UDP: 10, TCPEstablished: 20, TCPTransitory: 30, ICMP: 40})
	if nattest.Apply(t, p.Timeouts, tmo) != 1 || nattest.Apply(t, p.Timeouts, tmo) != 0 || f.timeouts.ICMP != 40 {
		t.Fatal("timeouts")
	}
	if nattest.Apply(t, p.Timeouts) != 1 || f.timeouts.UDP != nat44ei.DefaultUDPTimeout {
		t.Fatal("timeouts delete restores defaults")
	}
	fwd := natcommon.MustEncode(&nat44ei.ForwardingSpec{})
	if nattest.Apply(t, p.Forwarding, fwd) != 1 || !f.fwd || nattest.Apply(t, p.Forwarding, fwd) != 0 || nattest.Apply(t, p.Forwarding) != 1 || f.fwd {
		t.Fatal("forwarding")
	}
	ipfix := natcommon.MustEncode(&nat44ei.IpfixSpec{DomainID: 9, SrcPort: 4739})
	nattest.AssertWriteOnly(t, p.Ipfix) // domain id / src port have no getter (D-063)
	for i := 0; i < 2; i++ {
		if _, err := p.Ipfix.Create(ctx, ipfix); err != nil || !f.ipfix {
			t.Fatalf("ipfix enable #%d: %v", i, err)
		}
	}
	if r := f.CallsNamed("nat44_ei_ipfix_enable_disable")[0].(*nat44_ei.Nat44EiIpfixEnableDisable); r.DomainID != 9 || r.SrcPort != 4739 {
		t.Fatalf("ipfix request %+v", r)
	}
	if err := p.Ipfix.Delete(ctx, ipfix, nil); err != nil || f.ipfix {
		t.Fatalf("ipfix disable: %v", err)
	}
	if deps := p.Ipfix.Dependencies(ipfix); len(deps) != 1 || deps[0].Key != nat44ei.EnableKey {
		t.Fatalf("deps %+v", deps)
	}
	// finding 1: any object of any owner and kind keeps the plugin enabled
	for name, add := range map[string]func(){
		"output interface": func() { f.outputs[3] = nat44_ei.NAT44_EI_IF_OUTSIDE },
		"interface":        func() { f.features[3] = nat44_ei.NAT44_EI_IF_INSIDE },
		"pool address":     func() { f.addrs = append(f.addrs, &nat44_ei.Nat44EiAddressDetails{IPAddress: [4]uint8{10, 3, 0, 1}}) },
		"interface addr":   func() { f.ifAddrs[3] = true },
		"static mapping": func() {
			f.statics = append(f.statics, &nat44_ei.Nat44EiStaticMappingDetails{Tag: "w3:m", ExternalSwIfIndex: ^interface_types.InterfaceIndex(0)})
		},
		"identity mapping": func() {
			f.idents = append(f.idents, &nat44_ei.Nat44EiIdentityMappingDetails{Tag: "w3:i", SwIfIndex: ^interface_types.InterfaceIndex(0)})
		},
	} {
		add()
		if err := p.Enable.Delete(ctx, en, nil); err != nil || !f.enabled {
			t.Fatalf("%s: plugin disabled although another object exists (%v)", name, err)
		}
		f.outputs, f.features, f.ifAddrs = map[uint32]nat44_ei.Nat44EiConfigFlags{}, map[uint32]nat44_ei.Nat44EiConfigFlags{}, map[uint32]bool{}
		f.addrs, f.statics, f.idents = nil, nil, nil
	}
	if err := p.Enable.Delete(ctx, en, nil); err != nil || f.enabled {
		t.Fatal("delete of an empty plugin disables")
	}
	f.ed = true
	if _, err := p.Enable.Create(ctx, en); !errors.Is(err, nat44ei.ErrOtherVariant) {
		t.Fatalf("ED enabled: %v", err)
	}
}

func TestObjects(t *testing.T) {
	f := newFakeEI()
	p := nat44ei.New(f, "w9", owner)
	nattest.Apply(t, p.Enable, natcommon.MustEncode(&nat44ei.EnableSpec{}))
	f.features[3], f.outputs[3], f.ifAddrs[3] = nat44_ei.NAT44_EI_IF_INSIDE, nat44_ei.NAT44_EI_IF_OUTSIDE, true
	f.addrs = append(f.addrs, &nat44_ei.Nat44EiAddressDetails{IPAddress: [4]uint8{10, 3, 0, 1}})
	f.statics = append(f.statics, &nat44_ei.Nat44EiStaticMappingDetails{Tag: "w3:x", ExternalSwIfIndex: ^interface_types.InterfaceIndex(0)})

	in := natcommon.MustEncode(&nat44ei.InterfaceFeatureSpec{Interface: "loop900", Side: "inside"})
	out := natcommon.MustEncode(&nat44ei.InterfaceFeatureSpec{Interface: "loop901", Side: "outside"})
	if nattest.Apply(t, p.InterfaceFeature, in, out) != 2 || nattest.Apply(t, p.InterfaceFeature, in, out) != 0 || f.features[1] != nat44_ei.NAT44_EI_IF_INSIDE {
		t.Fatal("features")
	}
	if keys := nattest.Keys(t, p.InterfaceFeature); len(keys) != 2 || keys[0] != "nat44-ei.interface-feature/loop900/inside" {
		t.Fatalf("keys %v", keys)
	}
	if deps := p.InterfaceFeature.Dependencies(in); len(deps) != 2 || deps[1].Key != "interface/loop900" {
		t.Fatalf("deps %+v", deps)
	}
	of := natcommon.MustEncode(&nat44ei.OutputFeatureSpec{Interface: "loop901"})
	if nattest.Apply(t, p.OutputFeature, of) != 1 || nattest.Apply(t, p.OutputFeature, of) != 0 || f.outputs[2] != 1 {
		t.Fatal("output feature")
	}
	ia := natcommon.MustEncode(&nat44ei.InterfaceAddressSpec{Interface: "loop901"})
	if nattest.Apply(t, p.InterfaceAddress, ia) != 1 || nattest.Apply(t, p.InterfaceAddress, ia) != 0 || !f.ifAddrs[2] {
		t.Fatal("interface address")
	}
	pool := natcommon.MustEncode(&nat44ei.AddressPoolSpec{First: "10.9.0.3", Last: "10.9.0.1", VRF: 9001})
	if nattest.Apply(t, p.AddressPool, pool) != 1 || nattest.Apply(t, p.AddressPool, pool) != 0 {
		t.Fatal("pool")
	}
	if keys := nattest.Keys(t, p.AddressPool); len(keys) != 1 || keys[0] != "nat44-ei.address-pool/10.9.0.1-10.9.0.3/9001" {
		t.Fatalf("pool keys %v", keys)
	}
	sm := natcommon.MustEncode(&nat44ei.StaticMappingSpec{Name: "pf", Local: nat44ei.Endpoint{IP: "192.168.9.1", Port: 80}, External: nat44ei.Endpoint{IP: "10.9.0.1", Port: 8080}, Protocol: "tcp"})
	sm2 := natcommon.MustEncode(&nat44ei.StaticMappingSpec{Name: "if", Local: nat44ei.Endpoint{IP: "192.168.9.2"}, External: nat44ei.Endpoint{Interface: "loop901"}, AddrOnly: true})
	if nattest.Apply(t, p.StaticMapping, sm, sm2) != 2 || nattest.Apply(t, p.StaticMapping, sm, sm2) != 0 {
		t.Fatal("static mappings")
	}
	r := f.CallsNamed("nat44_ei_add_del_static_mapping")[1].(*nat44_ei.Nat44EiAddDelStaticMapping)
	if r.Flags != nat44_ei.NAT44_EI_ADDR_ONLY_MAPPING || r.ExternalSwIfIndex != 2 || r.Tag != "w9:if" {
		t.Fatalf("addr-only request %+v", r)
	}
	if keys := nattest.Keys(t, p.StaticMapping); len(keys) != 2 {
		t.Fatalf("foreign mapping visible: %v", keys)
	}
	id := natcommon.MustEncode(&nat44ei.IdentityMappingSpec{Name: "id", IP: "10.9.0.2", Protocol: "udp", Port: 500})
	if nattest.Apply(t, p.IdentityMapping, id) != 1 || nattest.Apply(t, p.IdentityMapping, id) != 0 {
		t.Fatal("identity")
	}
	for _, d := range []scheduler.Descriptor{p.IdentityMapping, p.StaticMapping, p.AddressPool, p.InterfaceAddress, p.OutputFeature, p.InterfaceFeature} {
		if nattest.Apply(t, d) == 0 || len(nattest.Keys(t, d)) != 0 {
			t.Fatalf("%s leftovers not deleted", d.Name())
		}
	}
	if len(f.addrs) != 1 || len(f.statics) != 1 || f.features[3] == 0 || f.outputs[3] == 0 || !f.ifAddrs[3] {
		t.Fatal("foreign objects touched")
	}
	users, err := p.Users(context.Background())
	if err != nil || len(users) != 1 {
		t.Fatal(err)
	}
	if s, err := p.UserSessions(context.Background(), users[0], 0, 0); err != nil || len(s) != 1 || !s[0].Static || s[0].Protocol != "tcp" {
		t.Fatalf("sessions %+v %v", s, err)
	}
	if err := p.DeleteSession(context.Background(), nat44ei.Endpoint{IP: "192.168.9.2", Port: 1000}, "tcp", 0, nat44ei.Endpoint{}); err != nil {
		t.Fatal(err)
	}
}
