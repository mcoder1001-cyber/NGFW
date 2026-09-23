package vxlan_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	vxlanapi "ngfw/agent/binapi/vxlan"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/vxlan"
	"ngfw/agent/internal/scheduler"
)

type fakeVXLAN struct {
	*df6test.FakeVPP
	tunnels map[uint32]*vxlanapi.VxlanAddDelTunnelV3
	bypass  map[uint32][2]bool
}

func newFakeVXLAN() *fakeVXLAN {
	f := &fakeVXLAN{FakeVPP: df6test.NewFakeVPP(), tunnels: map[uint32]*vxlanapi.VxlanAddDelTunnelV3{}, bypass: map[uint32][2]bool{}}
	f.On("vxlan_add_del_tunnel_v3", func(req api.Message) ([]api.Message, error) {
		r := *req.(*vxlanapi.VxlanAddDelTunnelV3)
		if r.IsAdd {
			if r.SrcPort == 0 {
				r.SrcPort = vxlan.DefaultPort
			}
			if r.DstPort == 0 {
				r.DstPort = vxlan.DefaultPort
			}
			idx := f.AddInterface(vxlan.InterfaceName(r.Instance), "")
			if !r.IsL3 {
				f.SetL2Address(idx, [6]byte{2, 0xfe, 1, 2, 3, 4})
			}
			f.tunnels[idx] = &r
			return []api.Message{&vxlanapi.VxlanAddDelTunnelV3Reply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
		}
		for idx, t := range f.tunnels {
			if t.SrcAddress == r.SrcAddress && t.DstAddress == r.DstAddress && t.Vni == r.Vni && t.EncapVrfID == r.EncapVrfID {
				delete(f.tunnels, idx)
				f.RemoveInterface(idx)
				return []api.Message{&vxlanapi.VxlanAddDelTunnelV3Reply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
			}
		}
		return []api.Message{&vxlanapi.VxlanAddDelTunnelV3Reply{Retval: -6}}, nil
	})
	f.On("vxlan_tunnel_v2_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 100; idx++ {
			if t, ok := f.tunnels[idx]; ok {
				out = append(out, &vxlanapi.VxlanTunnelV2Details{SwIfIndex: interface_types.InterfaceIndex(idx), Instance: t.Instance, SrcAddress: t.SrcAddress, DstAddress: t.DstAddress,
					SrcPort: t.SrcPort, DstPort: t.DstPort, McastSwIfIndex: t.McastSwIfIndex, EncapVrfID: t.EncapVrfID, DecapNextIndex: 1, Vni: t.Vni})
			}
		}
		return out, nil
	})
	f.On("sw_interface_set_vxlan_bypass", func(req api.Message) ([]api.Message, error) {
		r := req.(*vxlanapi.SwInterfaceSetVxlanBypass)
		{
			arc, node := "ip4-unicast", "ip4-vxlan-bypass"
			if r.IsIPv6 {
				arc, node = "ip6-unicast", "ip6-vxlan-bypass"
			}
			f.SetFeature(arc, node, uint32(r.SwIfIndex), r.Enable)
		}
		if !f.Has(uint32(r.SwIfIndex)) {
			return []api.Message{&vxlanapi.SwInterfaceSetVxlanBypassReply{Retval: -2}}, nil
		}
		b := f.bypass[uint32(r.SwIfIndex)]
		if r.IsIPv6 {
			b[1] = r.Enable
		} else {
			b[0] = r.Enable
		}
		f.bypass[uint32(r.SwIfIndex)] = b
		return []api.Message{&vxlanapi.SwInterfaceSetVxlanBypassReply{}}, nil
	})
	return f
}

func TestTunnelDescriptor(t *testing.T) {
	ctx := context.Background()
	f := newFakeVXLAN()
	loop, _ := f.Index("local0")
	_ = loop
	mcastIf := f.AddInterface("loop1101", "w11:loop1101")
	other := f.AddInterface("vxlan_tunnel3", "w3:vxlan_tunnel3")
	f.tunnels[other] = &vxlanapi.VxlanAddDelTunnelV3{Instance: 3, SrcAddress: df6test.Addr("10.3.0.1"), DstAddress: df6test.Addr("10.3.0.2"), Vni: 3000, SrcPort: 4789, DstPort: 4789}

	reg := scheduler.NewRegistry()
	vxlan.Register(reg, f, "w11")
	if reg.Len() != 2 {
		t.Fatalf("registered %d", reg.Len())
	}
	d := vxlan.NewTunnel(f, "w11")
	desired := &vxlan.Tunnel{Instance: 1100, Src: "10.11.1.1", Dst: "10.11.1.2", Vni: 11100, EncapVrfId: 11001}
	if k := d.KeyOf(desired); k != "vxlan.tunnel/vxlan_tunnel1100" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 1 || deps[0].Key != "vrf/11001" {
		t.Fatalf("deps = %+v", deps)
	}
	mcast := &vxlan.Tunnel{Instance: 1101, Src: "10.11.1.1", Dst: "239.11.1.1", McastInterface: "loop1101", Vni: 11101, SrcPort: 4790, DstPort: 4790, IsL3: true}
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
	if f.Tag(meta.(df6.IfMeta).SwIfIndex) != "w11:vxlan_tunnel1100" {
		t.Fatalf("tag = %q", f.Tag(meta.(df6.IfMeta).SwIfIndex))
	}
	reqs := f.CallsNamed("vxlan_add_del_tunnel_v3")
	r0, r1 := reqs[0].(*vxlanapi.VxlanAddDelTunnelV3), reqs[1].(*vxlanapi.VxlanAddDelTunnelV3)
	if r0.Vni != 11100 || r0.EncapVrfID != 11001 || r0.SrcPort != 0 || r0.McastSwIfIndex != interface_types.InterfaceIndex(df6.NoInterface) || r0.DecapNextIndex != df6.NoInterface || r0.IsL3 {
		t.Fatalf("request 0 = %+v", r0)
	}
	if r1.McastSwIfIndex != interface_types.InterfaceIndex(mcastIf) || r1.SrcPort != 4790 || !r1.IsL3 {
		t.Fatalf("request 1 = %+v", r1)
	}
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != 2 {
		t.Fatalf("Retrieve = %+v", actual)
	}
	if !proto.Equal(actual[0].Value, desired) || actual[0].Meta != meta {
		t.Fatalf("Retrieve[0] = %v, want %v", actual[0].Value, desired)
	}
	if !proto.Equal(actual[1].Value, mcast) || actual[1].Meta != mmeta {
		t.Fatalf("Retrieve[1] = %v, want %v", actual[1].Value, mcast)
	}
	if _, err := d.Update(ctx, desired, &vxlan.Tunnel{Instance: 1100, Src: "10.11.1.1", Dst: "10.11.1.2", Vni: 11101}, meta); !errors.Is(err, scheduler.ErrRecreate) {
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
	bad := []*vxlan.Tunnel{
		{Instance: df6.NoInterface, Src: "10.11.1.1", Dst: "10.11.1.2"},
		{Instance: 1, Src: "10.11.1.1", Dst: "fd11::1"},
		{Instance: 1, Src: "10.11.1.1", Dst: "239.1.1.1"},
		{Instance: 1, Src: "10.11.1.1", Dst: "10.11.1.2", McastInterface: "loop1101"},
		{Instance: 1, Src: "10.11.1.1", Dst: "10.11.1.2", Vni: 1 << 24},
		{Instance: 1, Src: "10.11.1.1", Dst: "10.11.1.2", SrcPort: 70000},
	}
	for _, b := range bad {
		if _, err := d.Create(ctx, b); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("%v: %v", b, err)
		}
	}
	if _, err := d.Create(ctx, &vxlan.Tunnel{Instance: 1, Src: "10.11.1.1", Dst: "239.1.1.1", McastInterface: "nope"}); !errors.Is(err, df6.ErrNoSuchInterface) {
		t.Errorf("unknown mcast interface: %v", err)
	}
}

func TestBypassDescriptor(t *testing.T) {
	ctx := context.Background()
	f := newFakeVXLAN()
	idx := f.AddInterface("loop1101", "w11:loop1101")
	d := vxlan.NewBypass(f, "w11")
	desired := &vxlan.Bypass{Interface: "loop1101", Ipv4: true}
	if k := d.KeyOf(desired); k != "vxlan.bypass/loop1101" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 1 || deps[0].Key != "interface/loop1101" {
		t.Fatalf("deps = %+v", deps)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	if f.bypass[idx] != [2]bool{true, false} {
		t.Fatalf("bypass state = %v", f.bypass[idx])
	}
	both := &vxlan.Bypass{Interface: "loop1101", Ipv4: true, Ipv6: true}
	if _, err := d.Update(ctx, desired, both, meta); err != nil {
		t.Fatal(err)
	}
	if f.bypass[idx] != [2]bool{true, true} {
		t.Fatalf("bypass state = %v", f.bypass[idx])
	}
	enables := 0
	for _, m := range f.CallsNamed("sw_interface_set_vxlan_bypass") {
		if m.(*vxlanapi.SwInterfaceSetVxlanBypass).Enable {
			enables++
		}
	}
	// each enable is preceded by a stale-bitmap reset (ResetBeforeEnable, a VPP no-op when clear)
	if enables != 2 {
		t.Fatalf("enables = %d, want only the changed family (2 in total)", enables)
	}
	if _, err := d.Update(ctx, both, &vxlan.Bypass{Interface: "loop1102", Ipv4: true}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update other interface = %v", err)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, df6.ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve = %v", err)
	}
	if err := d.Delete(ctx, both, meta); err != nil {
		t.Fatal(err)
	}
	if f.bypass[idx] != [2]bool{false, false} {
		t.Fatalf("bypass state after delete = %v", f.bypass[idx])
	}
	if _, err := d.Create(ctx, &vxlan.Bypass{Interface: "loop1101"}); !errors.Is(err, df6.ErrBadValue) {
		t.Fatalf("no family: %v", err)
	}
	if _, err := d.Create(ctx, &vxlan.Bypass{Interface: "nope", Ipv4: true}); !errors.Is(err, df6.ErrNoSuchInterface) {
		t.Fatalf("unknown interface: %v", err)
	}
}
