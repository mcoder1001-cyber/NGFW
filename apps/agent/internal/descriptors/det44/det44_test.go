package det44_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/det44"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/memclnt"
	det44d "ngfw/agent/internal/descriptors/det44"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

type fakeDet struct {
	*fake.Client
	enabled  bool
	timeouts det44.Det44GetTimeoutsReply
	ifaces   map[uint32]*det44.Det44InterfaceDetails
	maps     []*det44.Det44MapDetails
}

func newFakeDet() *fakeDet {
	f := &fakeDet{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), ifaces: map[uint32]*det44.Det44InterfaceDetails{},
		timeouts: det44.Det44GetTimeoutsReply{UDP: 300, TCPEstablished: 7440, TCPTransitory: 240, ICMP: 60}}
	f.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 1, InterfaceName: "loop900", Tag: "w9:loop900"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 2, InterfaceName: "loop901", Tag: "w9:loop901"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 3, InterfaceName: "loop300", Tag: "w3:loop300"})
	f.On("det44_plugin_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*det44.Det44PluginEnableDisable)
		rep := &det44.Det44PluginEnableDisableReply{}
		if r.Enable == f.enabled {
			rep.Retval = 1
		}
		f.enabled = r.Enable
		return []api.Message{rep}, nil
	})
	f.On("det44_get_timeouts", func(api.Message) ([]api.Message, error) {
		t := f.timeouts
		return []api.Message{&t}, nil
	})
	f.On("det44_set_timeouts", func(req api.Message) ([]api.Message, error) {
		r := req.(*det44.Det44SetTimeouts)
		f.timeouts = det44.Det44GetTimeoutsReply{UDP: r.UDP, TCPEstablished: r.TCPEstablished, TCPTransitory: r.TCPTransitory, ICMP: r.ICMP}
		return []api.Message{&det44.Det44SetTimeoutsReply{}}, nil
	})
	f.On("det44_interface_add_del_feature", func(req api.Message) ([]api.Message, error) {
		r := req.(*det44.Det44InterfaceAddDelFeature)
		idx := uint32(r.SwIfIndex)
		d, ok := f.ifaces[idx]
		if !ok {
			d = &det44.Det44InterfaceDetails{SwIfIndex: r.SwIfIndex}
			f.ifaces[idx] = d
		}
		switch {
		case r.IsAdd && r.IsInside:
			d.IsInside = true
		case r.IsAdd:
			d.IsOutside = true
		case r.IsInside:
			d.IsInside = false
		default:
			d.IsOutside = false
		}
		if !d.IsInside && !d.IsOutside {
			delete(f.ifaces, idx)
		}
		return []api.Message{&det44.Det44InterfaceAddDelFeatureReply{}}, nil
	})
	f.On("det44_interface_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 10; idx++ {
			if d, ok := f.ifaces[idx]; ok {
				out = append(out, d)
			}
		}
		return out, nil
	})
	f.On("det44_add_del_map", func(req api.Message) ([]api.Message, error) {
		r := req.(*det44.Det44AddDelMap)
		if r.IsAdd {
			f.maps = append(f.maps, &det44.Det44MapDetails{InAddr: r.InAddr, InPlen: r.InPlen, OutAddr: r.OutAddr, OutPlen: r.OutPlen, SharingRatio: 4, PortsPerHost: 1000})
		} else {
			for i, m := range f.maps {
				if m.InAddr == r.InAddr && m.InPlen == r.InPlen {
					f.maps = append(f.maps[:i], f.maps[i+1:]...)
					break
				}
			}
		}
		return []api.Message{&det44.Det44AddDelMapReply{}}, nil
	})
	f.On("det44_map_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(f.maps))
		for _, m := range f.maps {
			out = append(out, m)
		}
		return out, nil
	})
	f.Reply("det44_session_dump", &det44.Det44SessionDetails{InPort: 1000, OutPort: 2000, ExtAddr: [4]uint8{8, 8, 8, 8}, ExtPort: 53, State: 1, Expire: 30})
	f.Reply("det44_close_session_in", &det44.Det44CloseSessionInReply{})
	f.Reply("det44_close_session_out", &det44.Det44CloseSessionOutReply{})
	return f
}

func TestDet44(t *testing.T) {
	f := newFakeDet()
	p := det44d.New(f, "w9")
	ctx := context.Background()
	reg := scheduler.NewRegistry()
	det44d.Register(reg, f, "w9")
	if reg.Len() != 4 {
		t.Fatalf("registered %d", reg.Len())
	}
	en := natcommon.MustEncode(&det44d.EnableSpec{InsideVRF: 9001, OutsideVRF: 9002})
	if len(nattest.Keys(t, p.Enable)) != 0 || nattest.Apply(t, p.Enable, en) != 1 || nattest.Apply(t, p.Enable, en) != 0 || !f.enabled {
		t.Fatal("enable")
	}
	req := f.CallsNamed("det44_plugin_enable_disable")[0].(*det44.Det44PluginEnableDisable)
	if req.InsideVrf != 9001 || req.OutsideVrf != 9002 || !req.Enable {
		t.Fatalf("enable request %+v", req)
	}
	if deps := p.Enable.Dependencies(en); len(deps) != 2 || deps[0].Key != "vrf/9001" || deps[1].Key != "vrf/9002" {
		t.Fatalf("deps %+v", deps)
	}
	// a fresh plugin: heuristic (foreign map exists) → enabled; disable refused
	f.maps = append(f.maps, &det44.Det44MapDetails{InAddr: [4]uint8{10, 3, 0, 0}, InPlen: 24, OutAddr: [4]uint8{10, 3, 1, 0}, OutPlen: 30})
	fresh := det44d.New(f, "w9")
	if len(nattest.Keys(t, fresh.Enable)) != 1 {
		t.Fatal("heuristic")
	}
	if err := fresh.Enable.Delete(ctx, en, nil); !errors.Is(err, det44d.ErrForeignObjects) {
		t.Fatalf("foreign delete: %v", err)
	}

	tmo := natcommon.MustEncode(&det44d.TimeoutsSpec{UDP: 10, TCPEstablished: 7440, TCPTransitory: 240, ICMP: 60})
	if len(nattest.Keys(t, p.Timeouts)) != 0 || nattest.Apply(t, p.Timeouts, tmo) != 1 || nattest.Apply(t, p.Timeouts, tmo) != 0 || f.timeouts.UDP != 10 {
		t.Fatal("timeouts")
	}
	if nattest.Apply(t, p.Timeouts) != 1 || f.timeouts.UDP != 300 {
		t.Fatal("timeouts delete → defaults")
	}

	f.ifaces[3] = &det44.Det44InterfaceDetails{SwIfIndex: 3, IsInside: true}
	in := natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "loop900", Side: "inside"})
	out := natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "loop901", Side: "outside"})
	if nattest.Apply(t, p.Interface, in, out) != 2 || nattest.Apply(t, p.Interface, in, out) != 0 {
		t.Fatal("interfaces")
	}
	if !f.ifaces[1].IsInside || !f.ifaces[2].IsOutside {
		t.Fatalf("interface flags %+v %+v", f.ifaces[1], f.ifaces[2])
	}
	if keys := nattest.Keys(t, p.Interface); len(keys) != 2 || keys[0] != "det44.interface/loop900/inside" || keys[1] != "det44.interface/loop901/outside" {
		t.Fatalf("keys %v", keys)
	}
	if _, err := p.Interface.Create(ctx, natcommon.MustEncode(&det44d.InterfaceSpec{Interface: "loop900", Side: "x"})); err == nil {
		t.Fatal("bad side")
	}

	m := natcommon.MustEncode(&det44d.MapSpec{Inside: "10.9.10.7/24", Outside: "10.9.11.0/30"})
	if nattest.Apply(t, p.Map, m) != 1 || nattest.Apply(t, p.Map, m) != 0 {
		t.Fatal("map")
	}
	if keys := nattest.Keys(t, p.Map); len(keys) != 1 || keys[0] != "det44.map/10.9.10.0/24/10.9.11.0/30" {
		t.Fatalf("map keys %v (masked, foreign filtered)", keys)
	}
	r := f.CallsNamed("det44_add_del_map")[0].(*det44.Det44AddDelMap)
	if r.InPlen != 24 || r.OutPlen != 30 || r.InAddr != [4]uint8{10, 9, 10, 0} {
		t.Fatalf("map request %+v", r)
	}
	sess, err := p.Sessions(ctx, "10.9.10.1", 0, 0)
	if err != nil || len(sess) != 1 || sess[0].ExternalPort != 53 {
		t.Fatalf("sessions %+v %v", sess, err)
	}
	if err := p.CloseSessionIn(ctx, "10.9.10.1", 1000, "8.8.8.8", 53); err != nil {
		t.Fatal(err)
	}
	if err := p.CloseSessionOut(ctx, "10.9.11.1", 2000, "8.8.8.8", 53); err != nil {
		t.Fatal(err)
	}
	if nattest.Apply(t, p.Map) != 1 || nattest.Apply(t, p.Interface) != 2 || len(f.maps) != 1 || f.ifaces[3] == nil {
		t.Fatal("delete leftovers, foreign kept")
	}
	f.maps = nil
	delete(f.ifaces, 3)
	if err := p.Enable.Delete(ctx, en, nil); err != nil || f.enabled {
		t.Fatalf("disable: %v", err)
	}
}
