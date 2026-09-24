package vxlan_gpe_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	gpeapi "ngfw/agent/binapi/vxlan_gpe"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/vxlan_gpe"
	"ngfw/agent/internal/scheduler"
)

type fakeGPE struct {
	*df6test.FakeVPP
	tunnels map[uint32]*gpeapi.VxlanGpeAddDelTunnelV2
	bypass  map[uint32][2]bool
}

func newFakeGPE() *fakeGPE {
	f := &fakeGPE{FakeVPP: df6test.NewFakeVPP(), tunnels: map[uint32]*gpeapi.VxlanGpeAddDelTunnelV2{}, bypass: map[uint32][2]bool{}}
	n := 0
	f.On("vxlan_gpe_add_del_tunnel_v2", func(req api.Message) ([]api.Message, error) {
		r := *req.(*gpeapi.VxlanGpeAddDelTunnelV2)
		if r.IsAdd {
			if r.LocalPort == 0 {
				r.LocalPort = vxlan_gpe.DefaultPort
			}
			if r.RemotePort == 0 {
				r.RemotePort = vxlan_gpe.DefaultPort
			}
			idx := f.AddInterface(fmt.Sprintf("vxlan_gpe_tunnel%d", n), "")
			n++
			f.tunnels[idx] = &r
			return []api.Message{&gpeapi.VxlanGpeAddDelTunnelV2Reply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
		}
		for idx, t := range f.tunnels {
			if t.Local == r.Local && t.Remote == r.Remote && t.Vni == r.Vni {
				delete(f.tunnels, idx)
				f.RemoveInterface(idx)
				return []api.Message{&gpeapi.VxlanGpeAddDelTunnelV2Reply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
			}
		}
		return []api.Message{&gpeapi.VxlanGpeAddDelTunnelV2Reply{Retval: -6}}, nil
	})
	f.On("vxlan_gpe_tunnel_v2_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 100; idx++ {
			if t, ok := f.tunnels[idx]; ok {
				out = append(out, &gpeapi.VxlanGpeTunnelV2Details{SwIfIndex: interface_types.InterfaceIndex(idx), Local: t.Local, Remote: t.Remote, LocalPort: t.LocalPort, RemotePort: t.RemotePort,
					Vni: t.Vni, Protocol: t.Protocol, McastSwIfIndex: t.McastSwIfIndex, EncapVrfID: t.EncapVrfID, DecapVrfID: t.DecapVrfID})
			}
		}
		return out, nil
	})
	f.On("sw_interface_set_vxlan_gpe_bypass", func(req api.Message) ([]api.Message, error) {
		r := req.(*gpeapi.SwInterfaceSetVxlanGpeBypass)
		{
			arc, node := "ip4-unicast", "ip4-vxlan-gpe-bypass"
			if r.IsIPv6 {
				arc, node = "ip6-unicast", "ip6-vxlan-gpe-bypass"
			}
			f.SetFeature(arc, node, uint32(r.SwIfIndex), r.Enable)
		}
		b := f.bypass[uint32(r.SwIfIndex)]
		if r.IsIPv6 {
			b[1] = r.Enable
		} else {
			b[0] = r.Enable
		}
		f.bypass[uint32(r.SwIfIndex)] = b
		return []api.Message{&gpeapi.SwInterfaceSetVxlanGpeBypassReply{}}, nil
	})
	return f
}

func TestTunnelDescriptor(t *testing.T) {
	ctx := context.Background()
	f := newFakeGPE()
	mcastIf := f.AddInterface("loop1101", "w11:loop1101")
	other := f.AddInterface("vxlan_gpe_tunnel9", "w3:gpe-a")
	f.tunnels[other] = &gpeapi.VxlanGpeAddDelTunnelV2{Local: df6test.Addr("10.3.0.1"), Remote: df6test.Addr("10.3.0.2"), Vni: 3000, Protocol: 1}

	reg := scheduler.NewRegistry()
	vxlan_gpe.Register(reg, f, "w11")
	if reg.Len() != 2 {
		t.Fatalf("registered %d", reg.Len())
	}
	d := vxlan_gpe.NewTunnel(f, "w11")
	desired := &vxlan_gpe.Tunnel{Name: "w11-gpe1", Local: "10.11.1.1", Remote: "10.11.1.2", Vni: 11100, Protocol: vxlan_gpe.Protocol_IP4, EncapVrfId: 11001, DecapVrfId: 11002}
	if k := d.KeyOf(desired); k != "vxlan-gpe.tunnel/w11-gpe1" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 2 || deps[0].Key != "vrf/11001" || deps[1].Key != "vrf/11002" {
		t.Fatalf("deps = %+v", deps)
	}
	mcast := &vxlan_gpe.Tunnel{Name: "w11-gpe2", Local: "fd11::1", Remote: "ff05::11", McastInterface: "loop1101", Vni: 11101, Protocol: vxlan_gpe.Protocol_ETHERNET, LocalPort: 4791, RemotePort: 4791}
	if deps := d.Dependencies(mcast); len(deps) != 1 || deps[0].Key != "interface/loop1101" {
		t.Fatalf("mcast deps = %+v", deps)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	mmeta, err := d.Create(ctx, mcast)
	if err != nil {
		t.Fatal(err)
	}
	if f.Tag(meta.(df6.IfMeta).SwIfIndex) != "w11:w11-gpe1" {
		t.Fatalf("tag = %q", f.Tag(meta.(df6.IfMeta).SwIfIndex))
	}
	reqs := f.CallsNamed("vxlan_gpe_add_del_tunnel_v2")
	r0, r1 := reqs[0].(*gpeapi.VxlanGpeAddDelTunnelV2), reqs[1].(*gpeapi.VxlanGpeAddDelTunnelV2)
	if r0.Vni != 11100 || r0.EncapVrfID != 11001 || r0.DecapVrfID != 11002 || r0.Protocol != 1 || r0.LocalPort != 0 || r0.McastSwIfIndex != interface_types.InterfaceIndex(df6.NoInterface) {
		t.Fatalf("request 0 = %+v", r0)
	}
	if r1.McastSwIfIndex != interface_types.InterfaceIndex(mcastIf) || r1.Protocol != 3 || r1.LocalPort != 4791 {
		t.Fatalf("request 1 = %+v", r1)
	}
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != 2 || !proto.Equal(actual[0].Value, desired) || actual[0].Meta != meta || !proto.Equal(actual[1].Value, mcast) || actual[1].Meta != mmeta {
		t.Fatalf("Retrieve = %+v", actual)
	}
	if _, err := d.Update(ctx, desired, mcast, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update = %v", err)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, mcast, mmeta); err != nil {
		t.Fatal(err)
	}
	if actual, _ := d.Retrieve(ctx); len(actual) != 0 {
		t.Fatalf("after delete: %+v", actual)
	}
	if !f.Has(other) {
		t.Fatal("other owner's tunnel touched")
	}
	bad := []*vxlan_gpe.Tunnel{
		{Local: "10.11.1.1", Remote: "10.11.1.2", Protocol: 1},
		{Name: "x", Local: "10.11.1.1", Remote: "10.11.1.2"},
		{Name: "x", Local: "10.11.1.1", Remote: "10.11.1.2", Protocol: 5},
		{Name: "x", Local: "10.11.1.1", Remote: "fd11::1", Protocol: 1},
		{Name: "x", Local: "10.11.1.1", Remote: "239.1.1.1", Protocol: 1},
		{Name: "x", Local: "10.11.1.1", Remote: "10.11.1.2", Protocol: 1, Vni: 1 << 24},
		{Name: "x", Local: "10.11.1.1", Remote: "10.11.1.2", Protocol: vxlan_gpe.Protocol_ETHERNET, DecapVrfId: 5},
	}
	for _, b := range bad {
		if _, err := d.Create(ctx, b); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("%v: %v", b, err)
		}
	}
	// Bypass (write-only).
	bp := vxlan_gpe.NewBypass(f, "w11")
	bdesired := &vxlan_gpe.Bypass{Interface: "loop1101", Ipv6: true}
	bmeta, err := bp.Create(ctx, bdesired)
	if err != nil {
		t.Fatal(err)
	}
	if f.bypass[mcastIf] != [2]bool{false, true} {
		t.Fatalf("bypass = %v", f.bypass[mcastIf])
	}
	if _, err := bp.Retrieve(ctx); !errors.Is(err, df6.ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve = %v", err)
	}
	if err := bp.Delete(ctx, bdesired, bmeta); err != nil || f.bypass[mcastIf] != [2]bool{} {
		t.Fatalf("bypass delete: %v %v", err, f.bypass[mcastIf])
	}
}

// TestBypassResync (review H3): two resyncs send one enable per family (VPP stacks features).
func TestBypassResync(t *testing.T) {
	ctx := context.Background()
	f := newFakeGPE()
	f.AddInterface("loop1190", "w11rs:loop1190")
	b := vxlan_gpe.NewBypass(f, "w11rs")
	for i := 0; i < 2; i++ {
		if _, err := b.Create(ctx, &vxlan_gpe.Bypass{Interface: "loop1190", Ipv6: true}); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(f.CallsNamed("sw_interface_set_vxlan_gpe_bypass")); n != 1 {
		t.Fatalf("sent %d enables, want 1", n)
	}
}
