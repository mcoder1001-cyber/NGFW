package nat64_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/binapi/nat64"
	"ngfw/agent/binapi/nat_types"
	nat64d "ngfw/agent/internal/descriptors/nat64"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

type fake64 struct {
	*fake.Client
	enabled  bool
	timeouts nat64.Nat64GetTimeoutsReply
	ifaces   map[uint32]nat_types.NatConfigFlags
	prefixes []*nat64.Nat64PrefixDetails
	pool     []*nat64.Nat64PoolAddrDetails
	bibs     []*nat64.Nat64BibDetails
}

func newFake64() *fake64 {
	f := &fake64{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), ifaces: map[uint32]nat_types.NatConfigFlags{},
		timeouts: nat64.Nat64GetTimeoutsReply{UDP: 300, TCPEstablished: 7440, TCPTransitory: 240, ICMP: 60}}
	f.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 1, InterfaceName: "loop900", Tag: "w9:loop900"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 2, InterfaceName: "loop901", Tag: "w9:loop901"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 3, InterfaceName: "loop300", Tag: "w3:loop300"})
	f.On("nat64_plugin_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat64.Nat64PluginEnableDisable)
		rep := &nat64.Nat64PluginEnableDisableReply{}
		if r.Enable == f.enabled {
			rep.Retval = 1 // nat64.c: "plugin already enabled/disabled!" → 1
		}
		f.enabled = r.Enable
		return []api.Message{rep}, nil
	})
	f.On("nat64_get_timeouts", func(api.Message) ([]api.Message, error) {
		t := f.timeouts
		return []api.Message{&t}, nil
	})
	f.On("nat64_set_timeouts", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat64.Nat64SetTimeouts)
		f.timeouts = nat64.Nat64GetTimeoutsReply{UDP: r.UDP, TCPEstablished: r.TCPEstablished, TCPTransitory: r.TCPTransitory, ICMP: r.ICMP}
		return []api.Message{&nat64.Nat64SetTimeoutsReply{}}, nil
	})
	f.On("nat64_add_del_interface", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat64.Nat64AddDelInterface)
		idx := uint32(r.SwIfIndex)
		if r.IsAdd {
			f.ifaces[idx] |= r.Flags
		} else if f.ifaces[idx] &^= r.Flags; f.ifaces[idx] == 0 {
			delete(f.ifaces, idx)
		}
		return []api.Message{&nat64.Nat64AddDelInterfaceReply{}}, nil
	})
	f.On("nat64_interface_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 10; idx++ {
			if fl, ok := f.ifaces[idx]; ok {
				out = append(out, &nat64.Nat64InterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx), Flags: fl})
			}
		}
		return out, nil
	})
	f.On("nat64_add_del_prefix", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat64.Nat64AddDelPrefix)
		if r.IsAdd {
			f.prefixes = append(f.prefixes, &nat64.Nat64PrefixDetails{Prefix: r.Prefix, VrfID: r.VrfID})
		} else {
			for i, d := range f.prefixes {
				if d.VrfID == r.VrfID {
					f.prefixes = append(f.prefixes[:i], f.prefixes[i+1:]...)
					break
				}
			}
		}
		return []api.Message{&nat64.Nat64AddDelPrefixReply{}}, nil
	})
	f.On("nat64_prefix_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(f.prefixes))
		for _, d := range f.prefixes {
			out = append(out, d)
		}
		return out, nil
	})
	f.On("nat64_add_del_pool_addr_range", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat64.Nat64AddDelPoolAddrRange)
		for a := r.StartAddr; ; a[3]++ {
			if r.IsAdd {
				f.pool = append(f.pool, &nat64.Nat64PoolAddrDetails{Address: a, VrfID: r.VrfID})
			} else {
				for i, d := range f.pool {
					if d.Address == a {
						f.pool = append(f.pool[:i], f.pool[i+1:]...)
						break
					}
				}
			}
			if a == r.EndAddr {
				break
			}
		}
		return []api.Message{&nat64.Nat64AddDelPoolAddrRangeReply{}}, nil
	})
	f.On("nat64_pool_addr_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(f.pool))
		for _, d := range f.pool {
			out = append(out, d)
		}
		return out, nil
	})
	f.On("nat64_add_del_static_bib", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat64.Nat64AddDelStaticBib)
		if r.IsAdd {
			f.bibs = append(f.bibs, &nat64.Nat64BibDetails{IAddr: r.IAddr, OAddr: r.OAddr, IPort: r.IPort, OPort: r.OPort, VrfID: r.VrfID, Proto: r.Proto, Flags: nat_types.NAT_IS_STATIC})
		} else {
			for i, d := range f.bibs {
				if d.IAddr == r.IAddr && d.IPort == r.IPort && d.Proto == r.Proto {
					f.bibs = append(f.bibs[:i], f.bibs[i+1:]...)
					break
				}
			}
		}
		return []api.Message{&nat64.Nat64AddDelStaticBibReply{}}, nil
	})
	f.On("nat64_bib_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(f.bibs))
		for _, d := range f.bibs {
			out = append(out, d)
		}
		return out, nil
	})
	f.Reply("nat64_st_dump", &nat64.Nat64StDetails{IlAddr: ip_types.IP6Address{0xfd, 0, 0, 9}, OlAddr: [4]uint8{10, 9, 0, 1}, Proto: 6, RPort: 443})
	return f
}

func TestNat64(t *testing.T) {
	f := newFake64()
	p := nat64d.New(f, "w9")
	ctx := context.Background()
	reg := scheduler.NewRegistry()
	nat64d.Register(reg, f, "w9")
	if reg.Len() != 6 {
		t.Fatalf("registered %d", reg.Len())
	}

	// enable: write-only (D-063); create; re-apply is idempotent (already-enabled retval 1 tolerated)
	en := natcommon.MustEncode(&nat64d.EnableSpec{})
	nattest.AssertWriteOnly(t, p.Enable)
	for i := 0; i < 2; i++ {
		if _, err := p.Enable.Create(ctx, en); err != nil || !f.enabled {
			t.Fatalf("enable #%d: %v", i, err)
		}
	}
	// disable refused while another owner's objects exist
	f.bibs = append(f.bibs, &nat64.Nat64BibDetails{OAddr: [4]uint8{10, 3, 0, 1}, Flags: nat_types.NAT_IS_STATIC}) // w3's static bib
	fresh := nat64d.New(f, "w9")
	if err := fresh.Enable.Delete(ctx, en, nil); !errors.Is(err, nat64d.ErrForeignObjects) {
		t.Fatalf("disable with foreign bib: %v", err)
	}
	f.bibs = nil

	tmo := natcommon.MustEncode(&nat64d.TimeoutsSpec{UDP: 100, TCPEstablished: 7440, TCPTransitory: 240, ICMP: 60})
	if len(nattest.Keys(t, p.Timeouts)) != 0 || nattest.Apply(t, p.Timeouts, tmo) != 1 || nattest.Apply(t, p.Timeouts, tmo) != 0 || f.timeouts.UDP != 100 {
		t.Fatal("timeouts")
	}
	if nattest.Apply(t, p.Timeouts) != 1 || f.timeouts.UDP != 300 || len(nattest.Keys(t, p.Timeouts)) != 0 {
		t.Fatal("timeouts delete restores defaults → absent")
	}

	f.prefixes = append(f.prefixes, &nat64.Nat64PrefixDetails{Prefix: ip_types.IP6Prefix{Address: ip_types.IP6Address{0x20, 0x01, 0x0d, 0xb8}, Len: 96}, VrfID: 3001})
	pfx := natcommon.MustEncode(&nat64d.PrefixSpec{Prefix: "64:ff9b::1/96", VRF: 9001})
	if nattest.Apply(t, p.Prefix, pfx) != 1 || nattest.Apply(t, p.Prefix, pfx) != 0 {
		t.Fatal("prefix")
	}
	if keys := nattest.Keys(t, p.Prefix); len(keys) != 1 || keys[0] != "nat64.prefix/64:ff9b::/96/9001" {
		t.Fatalf("prefix keys %v (masked, foreign filtered)", keys)
	}
	if deps := p.Prefix.Dependencies(pfx); len(deps) != 2 || deps[1].Key != "vrf/9001" {
		t.Fatalf("deps %+v", deps)
	}

	f.pool = append(f.pool, &nat64.Nat64PoolAddrDetails{Address: [4]uint8{10, 3, 0, 1}})
	pool := natcommon.MustEncode(&nat64d.PoolSpec{First: "10.9.0.1", Last: "10.9.0.3"})
	if nattest.Apply(t, p.Pool, pool) != 1 || nattest.Apply(t, p.Pool, pool) != 0 || len(f.pool) != 4 {
		t.Fatal("pool")
	}
	if keys := nattest.Keys(t, p.Pool); len(keys) != 1 || keys[0] != "nat64.pool/10.9.0.1-10.9.0.3/0" {
		t.Fatalf("pool keys %v", keys)
	}

	f.ifaces[3] = nat_types.NAT_IS_INSIDE
	in := natcommon.MustEncode(&nat64d.InterfaceSpec{Interface: "loop900", Side: "inside"})
	out := natcommon.MustEncode(&nat64d.InterfaceSpec{Interface: "loop901", Side: "outside"})
	if nattest.Apply(t, p.Interface, in, out) != 2 || nattest.Apply(t, p.Interface, in, out) != 0 || f.ifaces[1] != nat_types.NAT_IS_INSIDE {
		t.Fatal("interfaces")
	}
	if _, err := p.Interface.Create(ctx, natcommon.MustEncode(&nat64d.InterfaceSpec{Interface: "loop900", Side: "up"})); err == nil {
		t.Fatal("bad side")
	}

	bib := natcommon.MustEncode(&nat64d.StaticBIBSpec{InsideIP: "fd00:9::0010", InsidePort: 80, OutsideIP: "10.9.0.1", OutsidePort: 8080, Protocol: "6"})
	if nattest.Apply(t, p.StaticBIB, bib) != 1 || nattest.Apply(t, p.StaticBIB, bib) != 0 {
		t.Fatal("static bib")
	}
	if keys := nattest.Keys(t, p.StaticBIB); len(keys) != 1 || keys[0] != "nat64.static-bib/tcp/fd00:9::10/80/0" {
		t.Fatalf("bib keys %v", keys)
	}
	req := f.CallsNamed("nat64_add_del_static_bib")[0].(*nat64.Nat64AddDelStaticBib)
	if req.Proto != 6 || req.OPort != 8080 || !req.IsAdd {
		t.Fatalf("bib request %+v", req)
	}
	// dynamic bib entries are state, never retrieved
	f.bibs = append(f.bibs, &nat64.Nat64BibDetails{OAddr: [4]uint8{10, 9, 0, 2}, Proto: 17})
	if len(nattest.Keys(t, p.StaticBIB)) != 1 {
		t.Fatal("dynamic bib retrieved")
	}
	f.bibs = f.bibs[:1]

	sess, err := p.Sessions(ctx, "any", 0, 10)
	if err != nil || len(sess) != 1 || sess[0].Protocol != "tcp" || sess[0].RemotePort != 443 {
		t.Fatalf("sessions %+v %v", sess, err)
	}

	for _, d := range []scheduler.Descriptor{p.StaticBIB, p.Interface, p.Pool, p.Prefix} {
		if nattest.Apply(t, d) == 0 || len(nattest.Keys(t, d)) != 0 {
			t.Fatalf("%s leftovers", d.Name())
		}
	}
	if len(f.pool) != 1 || len(f.prefixes) != 1 || f.ifaces[3] == 0 {
		t.Fatal("foreign objects touched")
	}
	if err := p.Enable.Delete(ctx, en, nil); !errors.Is(err, nat64d.ErrForeignObjects) {
		t.Fatalf("disable with foreign objects: %v", err)
	}
	f.pool, f.prefixes = nil, nil
	delete(f.ifaces, 3)
	if err := p.Enable.Delete(ctx, en, nil); err != nil || f.enabled {
		t.Fatalf("disable: %v", err)
	}
	nattest.AssertWriteOnly(t, p.Enable)
}
