package nat66_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/binapi/nat66"
	"ngfw/agent/binapi/nat_types"
	nat66d "ngfw/agent/internal/descriptors/nat66"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

type fake66 struct {
	*fake.Client
	enabled  bool
	ifaces   map[uint32]nat_types.NatConfigFlags
	mappings []*nat66.Nat66StaticMappingDetails
}

func newFake66() *fake66 {
	f := &fake66{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), ifaces: map[uint32]nat_types.NatConfigFlags{}}
	f.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 1, InterfaceName: "loop900", Tag: "w9:loop900"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 3, InterfaceName: "loop300", Tag: "w3:loop300"})
	f.On("nat66_plugin_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat66.Nat66PluginEnableDisable)
		rep := &nat66.Nat66PluginEnableDisableReply{}
		if r.Enable == f.enabled {
			rep.Retval = int32(api.FEATURE_ALREADY_ENABLED)
		}
		f.enabled = r.Enable
		return []api.Message{rep}, nil
	})
	f.On("nat66_add_del_interface", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat66.Nat66AddDelInterface)
		idx := uint32(r.SwIfIndex)
		if r.IsAdd {
			f.ifaces[idx] = r.Flags & nat_types.NAT_IS_INSIDE // VPP keeps one side, reports only the inside bit
		} else {
			delete(f.ifaces, idx)
		}
		return []api.Message{&nat66.Nat66AddDelInterfaceReply{}}, nil
	})
	f.On("nat66_interface_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 10; idx++ {
			if fl, ok := f.ifaces[idx]; ok {
				out = append(out, &nat66.Nat66InterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx), Flags: fl})
			}
		}
		return out, nil
	})
	f.On("nat66_add_del_static_mapping", func(req api.Message) ([]api.Message, error) {
		r := req.(*nat66.Nat66AddDelStaticMapping)
		if r.IsAdd {
			f.mappings = append(f.mappings, &nat66.Nat66StaticMappingDetails{LocalIPAddress: r.LocalIPAddress, ExternalIPAddress: r.ExternalIPAddress, VrfID: r.VrfID, TotalPkts: 7})
		} else {
			for i, m := range f.mappings {
				if m.LocalIPAddress == r.LocalIPAddress && m.VrfID == r.VrfID {
					f.mappings = append(f.mappings[:i], f.mappings[i+1:]...)
					break
				}
			}
		}
		return []api.Message{&nat66.Nat66AddDelStaticMappingReply{}}, nil
	})
	f.On("nat66_static_mapping_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(f.mappings))
		for _, m := range f.mappings {
			out = append(out, m)
		}
		return out, nil
	})
	return f
}

func TestNat66(t *testing.T) {
	f := newFake66()
	p := nat66d.New(f, "w9")
	ctx := context.Background()
	reg := scheduler.NewRegistry()
	nat66d.Register(reg, f, "w9")
	if reg.Len() != 3 {
		t.Fatalf("registered %d", reg.Len())
	}
	en := natcommon.MustEncode(&nat66d.EnableSpec{OutsideVRF: 9001})
	if len(nattest.Keys(t, p.Enable)) != 0 || nattest.Apply(t, p.Enable, en) != 1 || nattest.Apply(t, p.Enable, en) != 0 || !f.enabled {
		t.Fatal("enable")
	}
	if req := f.CallsNamed("nat66_plugin_enable_disable")[0].(*nat66.Nat66PluginEnableDisable); req.OutsideVrf != 9001 {
		t.Fatalf("enable request %+v", req)
	}
	if deps := p.Enable.Dependencies(en); len(deps) != 1 || deps[0].Key != "vrf/9001" {
		t.Fatalf("deps %+v", deps)
	}
	// foreign mapping outside our scope: heuristic says enabled, disable refused
	f.mappings = append(f.mappings, &nat66.Nat66StaticMappingDetails{LocalIPAddress: ip_types.IP6Address{0xfd, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, ExternalIPAddress: ip_types.IP6Address{0x20, 0x01, 0x0d, 0xb8}, VrfID: 3001})
	fresh := nat66d.New(f, "w9")
	if len(nattest.Keys(t, fresh.Enable)) != 1 {
		t.Fatal("heuristic")
	}
	if err := fresh.Enable.Delete(ctx, en, nil); !errors.Is(err, nat66d.ErrForeignObjects) {
		t.Fatalf("foreign delete: %v", err)
	}
	if _, err := p.Enable.Update(ctx, en, natcommon.MustEncode(&nat66d.EnableSpec{}), nil); !errors.Is(err, nat66d.ErrForeignObjects) {
		t.Fatalf("foreign update: %v", err)
	}
	f.ifaces[3] = 0 // w3's outside interface (flags 0 = outside in nat66)
	in := natcommon.MustEncode(&nat66d.InterfaceSpec{Interface: "loop900", Side: "inside"})
	if nattest.Apply(t, p.Interface, in) != 1 || nattest.Apply(t, p.Interface, in) != 0 || f.ifaces[1] != nat_types.NAT_IS_INSIDE {
		t.Fatal("interface")
	}
	if keys := nattest.Keys(t, p.Interface); len(keys) != 1 || keys[0] != "nat66.interface/loop900" {
		t.Fatalf("keys %v", keys)
	}
	// side change = in-place Update (delete old side, add new); outside is flags 0 in the dump
	out := natcommon.MustEncode(&nat66d.InterfaceSpec{Interface: "loop900", Side: "outside"})
	if nattest.Apply(t, p.Interface, out) != 1 || nattest.Apply(t, p.Interface, out) != 0 || f.ifaces[1] != 0 {
		t.Fatal("outside interface (flags 0) must round-trip")
	}
	if calls := f.CallsNamed("nat66_add_del_interface"); len(calls) != 3 || calls[1].(*nat66.Nat66AddDelInterface).IsAdd || !calls[2].(*nat66.Nat66AddDelInterface).IsAdd {
		t.Fatalf("side change must be delete+add: %+v", calls)
	}
	if nattest.Apply(t, p.Interface, in) != 1 || f.ifaces[1] != nat_types.NAT_IS_INSIDE {
		t.Fatal("back to inside")
	}
	m := natcommon.MustEncode(&nat66d.StaticMappingSpec{Local: "fd00:9::0001", External: "fd00:9::0100", VRF: 0})
	if nattest.Apply(t, p.StaticMapping, m) != 1 || nattest.Apply(t, p.StaticMapping, m) != 0 {
		t.Fatal("mapping")
	}
	if keys := nattest.Keys(t, p.StaticMapping); len(keys) != 1 || keys[0] != "nat66.static-mapping/fd00:9::1/0" {
		t.Fatalf("mapping keys %v (canonical, foreign filtered)", keys)
	}
	m2 := natcommon.MustEncode(&nat66d.StaticMappingSpec{Local: "fd00:9::1", External: "fd00:9::200"})
	if nattest.Apply(t, p.StaticMapping, m2) != 1 || natcommon.IP6String(f.mappings[1].ExternalIPAddress) != "fd00:9::200" {
		t.Fatal("recreate on external change")
	}
	if _, foreign := f.ifaces[3]; nattest.Apply(t, p.StaticMapping) != 1 || nattest.Apply(t, p.Interface) != 1 || len(f.mappings) != 1 || !foreign {
		t.Fatal("delete leftovers, foreign kept")
	}
	f.mappings = nil
	delete(f.ifaces, 3)
	if err := p.Enable.Delete(ctx, en, nil); err != nil || f.enabled {
		t.Fatalf("disable: %v", err)
	}
}
