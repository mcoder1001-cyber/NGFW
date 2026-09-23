package gtpu_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	gtpuapi "ngfw/agent/binapi/gtpu"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/gtpu"
	"ngfw/agent/internal/scheduler"
)

type fakeGTPU struct {
	*df6test.FakeVPP
	tunnels  map[uint32]*gtpuapi.GtpuTunnelV2Details
	bypass   map[uint32][2]bool
	tteidUpd int
	// crashes counts add/del requests VPP 26.06 would reject — each one segfaults the real
	// VPP (V8), so the descriptor must never send one.
	crashes int
}

func newFakeGTPU() *fakeGTPU {
	f := &fakeGTPU{FakeVPP: df6test.NewFakeVPP(), tunnels: map[uint32]*gtpuapi.GtpuTunnelV2Details{}, bypass: map[uint32][2]bool{}}
	n := 0
	f.On("gtpu_add_del_tunnel_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*gtpuapi.GtpuAddDelTunnelV2)
		if r.IsAdd {
			for _, t := range f.tunnels {
				if !t.IsForwarding && t.DstAddress == r.DstAddress && t.Teid == r.Teid {
					f.crashes++
					return []api.Message{&gtpuapi.GtpuAddDelTunnelV2Reply{Retval: -126}}, nil // TUNNEL_EXIST
				}
			}
			idx := f.AddInterface(fmt.Sprintf("gtpu_tunnel%d", n), "")
			n++
			tteid := r.Tteid
			if tteid == 0 {
				tteid = r.Teid
			}
			f.tunnels[idx] = &gtpuapi.GtpuTunnelV2Details{SwIfIndex: interface_types.InterfaceIndex(idx), SrcAddress: r.SrcAddress, DstAddress: r.DstAddress, McastSwIfIndex: r.McastSwIfIndex,
				EncapVrfID: r.EncapVrfID, DecapNextIndex: r.DecapNextIndex, Teid: r.Teid, Tteid: tteid, PduExtension: r.PduExtension, Qfi: r.Qfi}
			return []api.Message{&gtpuapi.GtpuAddDelTunnelV2Reply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
		}
		for idx, t := range f.tunnels {
			if !t.IsForwarding && t.DstAddress == r.DstAddress && t.Teid == r.Teid && t.EncapVrfID == r.EncapVrfID {
				delete(f.tunnels, idx)
				f.RemoveInterface(idx)
				return []api.Message{&gtpuapi.GtpuAddDelTunnelV2Reply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
			}
		}
		f.crashes++
		return []api.Message{&gtpuapi.GtpuAddDelTunnelV2Reply{Retval: -6}}, nil
	})
	f.On("gtpu_add_del_forward", func(req api.Message) ([]api.Message, error) {
		r := req.(*gtpuapi.GtpuAddDelForward)
		if r.IsAdd {
			idx := f.AddInterface(fmt.Sprintf("gtpu_tunnel%d", n), "")
			n++
			f.tunnels[idx] = &gtpuapi.GtpuTunnelV2Details{SwIfIndex: interface_types.InterfaceIndex(idx), SrcAddress: r.DstAddress, DstAddress: df6test.Addr("127.0.0.128"), EncapVrfID: r.EncapVrfID, DecapNextIndex: r.DecapNextIndex,
				IsForwarding: true, ForwardingType: r.ForwardingType, McastSwIfIndex: interface_types.InterfaceIndex(df6.NoInterface)}
			return []api.Message{&gtpuapi.GtpuAddDelForwardReply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
		}
		for idx, t := range f.tunnels {
			if t.IsForwarding && t.SrcAddress == r.DstAddress && t.ForwardingType == r.ForwardingType {
				delete(f.tunnels, idx)
				f.RemoveInterface(idx)
				return []api.Message{&gtpuapi.GtpuAddDelForwardReply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
			}
		}
		return []api.Message{&gtpuapi.GtpuAddDelForwardReply{Retval: -6}}, nil
	})
	f.On("gtpu_tunnel_update_tteid", func(req api.Message) ([]api.Message, error) {
		r := req.(*gtpuapi.GtpuTunnelUpdateTteid)
		f.tteidUpd++
		for _, t := range f.tunnels {
			if t.DstAddress == r.DstAddress && t.Teid == r.Teid && t.EncapVrfID == r.EncapVrfID {
				t.Tteid = r.Tteid
				return []api.Message{&gtpuapi.GtpuTunnelUpdateTteidReply{}}, nil
			}
		}
		return []api.Message{&gtpuapi.GtpuTunnelUpdateTteidReply{Retval: -6}}, nil
	})
	f.On("gtpu_tunnel_v2_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 100; idx++ {
			if t, ok := f.tunnels[idx]; ok {
				c := *t
				out = append(out, &c)
			}
		}
		return out, nil
	})
	f.On("sw_interface_set_gtpu_bypass", func(req api.Message) ([]api.Message, error) {
		r := req.(*gtpuapi.SwInterfaceSetGtpuBypass)
		b := f.bypass[uint32(r.SwIfIndex)]
		if r.IsIPv6 {
			b[1] = r.Enable
		} else {
			b[0] = r.Enable
		}
		f.bypass[uint32(r.SwIfIndex)] = b
		return []api.Message{&gtpuapi.SwInterfaceSetGtpuBypassReply{}}, nil
	})
	return f
}

func TestTunnelAndForward(t *testing.T) {
	ctx := context.Background()
	f := newFakeGTPU()
	mcastIf := f.AddInterface("loop1101", "w11:loop1101")
	other := f.AddInterface("gtpu_tunnel9", "w3:gtpu-a")
	f.tunnels[other] = &gtpuapi.GtpuTunnelV2Details{SwIfIndex: interface_types.InterfaceIndex(other), SrcAddress: df6test.Addr("10.3.0.1"), DstAddress: df6test.Addr("10.3.0.2"), Teid: 3}

	reg := scheduler.NewRegistry()
	gtpu.Register(reg, f, "w11")
	if reg.Len() != 3 {
		t.Fatalf("registered %d", reg.Len())
	}
	d := gtpu.NewTunnel(f, "w11")
	fw := gtpu.NewForward(f, "w11")

	desired := &gtpu.Tunnel{Name: "w11-gtpu1", Src: "10.11.1.1", Dst: "10.11.1.2", EncapVrfId: 11001, DecapNext: gtpu.DecapNext_L2, Teid: 11100, Tteid: 11200}
	if k := d.KeyOf(desired); k != "gtpu.tunnel/w11-gtpu1" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 1 || deps[0].Key != "vrf/11001" {
		t.Fatalf("deps = %+v", deps)
	}
	mcast := &gtpu.Tunnel{Name: "w11-gtpu2", Src: "10.11.1.1", Dst: "239.11.1.1", McastInterface: "loop1101", DecapNext: gtpu.DecapNext_IP4, Teid: 11101, PduExtension: true, Qfi: 9}
	forward := &gtpu.Forward{Name: "w11-gtpufw", Dst: "10.11.1.9", ForwardingType: 3, DecapNext: gtpu.DecapNext_L2}
	if k := fw.KeyOf(forward); k != "gtpu.forward/w11-gtpufw" {
		t.Fatalf("forward KeyOf = %s", k)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	mmeta, err := d.Create(ctx, mcast)
	if err != nil {
		t.Fatal(err)
	}
	fmeta, err := fw.Create(ctx, forward)
	if err != nil {
		t.Fatal(err)
	}
	if f.Tag(fmeta.(df6.IfMeta).SwIfIndex) != "w11:w11-gtpufw" {
		t.Fatalf("forward tag = %q", f.Tag(fmeta.(df6.IfMeta).SwIfIndex))
	}
	reqs := f.CallsNamed("gtpu_add_del_tunnel_v2")
	r0, r1 := reqs[0].(*gtpuapi.GtpuAddDelTunnelV2), reqs[1].(*gtpuapi.GtpuAddDelTunnelV2)
	if r0.Teid != 11100 || r0.Tteid != 11200 || r0.EncapVrfID != 11001 || r0.DecapNextIndex != gtpuapi.GTPU_API_DECAP_NEXT_L2 || r0.McastSwIfIndex != interface_types.InterfaceIndex(df6.NoInterface) {
		t.Fatalf("request 0 = %+v", r0)
	}
	if r1.McastSwIfIndex != interface_types.InterfaceIndex(mcastIf) || !r1.PduExtension || r1.Qfi != 9 || r1.DecapNextIndex != gtpuapi.GTPU_API_DECAP_NEXT_IP4 || r1.Tteid != 11101 {
		t.Fatalf("request 1 = %+v", r1)
	}
	fr := f.CallsNamed("gtpu_add_del_forward")[0].(*gtpuapi.GtpuAddDelForward)
	if fr.ForwardingType != 3 || fr.DstAddress != df6test.Addr("10.11.1.9") {
		t.Fatalf("forward request = %+v", fr)
	}

	// Each descriptor sees only its kind of record (and only ours).
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != 2 || !proto.Equal(actual[0].Value, desired) || actual[0].Meta != meta || !proto.Equal(actual[1].Value, mcast) || actual[1].Meta != mmeta {
		t.Fatalf("tunnel Retrieve = %+v", actual)
	}
	factual, err := fw.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(factual) != 1 || !proto.Equal(factual[0].Value, forward) || factual[0].Meta != fmeta {
		t.Fatalf("forward Retrieve = %+v", factual)
	}

	// tteid changes in place; anything else recreates.
	newTteid := proto.Clone(desired).(*gtpu.Tunnel)
	newTteid.Tteid = 11201
	if m2, err := d.Update(ctx, desired, newTteid, meta); err != nil || m2 != meta {
		t.Fatalf("tteid update: %v %v", err, m2)
	}
	if f.tteidUpd != 1 || f.tunnels[meta.(df6.IfMeta).SwIfIndex].Tteid != 11201 {
		t.Fatalf("tteid not updated in place")
	}
	if _, err := d.Update(ctx, newTteid, &gtpu.Tunnel{Name: "w11-gtpu1", Src: "10.11.1.1", Dst: "10.11.1.2", EncapVrfId: 11001, DecapNext: gtpu.DecapNext_L2, Teid: 11199, Tteid: 11201}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("teid update = %v, want ErrRecreate", err)
	}
	if _, err := fw.Update(ctx, forward, forward, fmeta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("forward update = %v", err)
	}

	if err := d.Delete(ctx, newTteid, meta); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, mcast, mmeta); err != nil {
		t.Fatal(err)
	}
	if err := fw.Delete(ctx, forward, fmeta); err != nil {
		t.Fatal(err)
	}
	if a, _ := d.Retrieve(ctx); len(a) != 0 {
		t.Fatalf("after delete: %+v", a)
	}
	if a, _ := fw.Retrieve(ctx); len(a) != 0 {
		t.Fatalf("forward after delete: %+v", a)
	}
	if !f.Has(other) {
		t.Fatal("other owner's tunnel touched")
	}
	if f.crashes != 0 {
		t.Fatalf("%d requests would have crashed VPP", f.crashes)
	}
	bad := []*gtpu.Tunnel{
		{Src: "10.11.1.1", Dst: "10.11.1.2"},
		{Name: "x", Src: "10.11.1.1", Dst: "fd11::1"},
		{Name: "x", Src: "10.11.1.1", Dst: "239.1.1.1"},
		{Name: "x", Src: "10.11.1.1", Dst: "10.11.1.2", Qfi: 64, PduExtension: true},
		{Name: "x", Src: "10.11.1.1", Dst: "10.11.1.2", Qfi: 1},
	}
	for _, b := range bad {
		if _, err := d.Create(ctx, b); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("%v: %v", b, err)
		}
	}
	for _, b := range []*gtpu.Forward{{Dst: "10.11.1.9", ForwardingType: 1}, {Name: "x", Dst: "10.11.1.9"}, {Name: "x", Dst: "10.11.1.9", ForwardingType: 8}} {
		if _, err := fw.Create(ctx, b); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("forward %v: %v", b, err)
		}
	}
	// Bypass (write-only).
	bp := gtpu.NewBypass(f, "w11")
	bmeta, err := bp.Create(ctx, &gtpu.Bypass{Interface: "loop1101", Ipv4: true, Ipv6: true})
	if err != nil {
		t.Fatal(err)
	}
	if f.bypass[mcastIf] != [2]bool{true, true} {
		t.Fatalf("bypass = %v", f.bypass[mcastIf])
	}
	if _, err := bp.Retrieve(ctx); !errors.Is(err, df6.ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve = %v", err)
	}
	if err := bp.Delete(ctx, &gtpu.Bypass{Interface: "loop1101", Ipv4: true, Ipv6: true}, bmeta); err != nil || f.bypass[mcastIf] != [2]bool{} {
		t.Fatalf("bypass delete: %v %v", err, f.bypass[mcastIf])
	}
}

// TestV8Guard: the descriptor never sends a gtpu_add_del_tunnel_v2 VPP would reject (a
// duplicate add or the delete of a missing tunnel), because VPP 26.06 segfaults on those.
func TestV8Guard(t *testing.T) {
	ctx := context.Background()
	f := newFakeGTPU()
	d := gtpu.NewTunnel(f, "w11")
	a := &gtpu.Tunnel{Name: "w11-a", Src: "10.11.1.1", Dst: "10.11.1.2", DecapNext: gtpu.DecapNext_L2, Teid: 11100}
	meta, err := d.Create(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	dup := &gtpu.Tunnel{Name: "w11-b", Src: "10.11.1.3", Dst: "10.11.1.2", DecapNext: gtpu.DecapNext_IP4, Teid: 11100}
	if _, err := d.Create(ctx, dup); !errors.Is(err, gtpu.ErrTunnelExists) {
		t.Fatalf("duplicate create = %v, want ErrTunnelExists", err)
	}
	if err := d.Delete(ctx, a, meta); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, a, meta); err != nil {
		t.Fatalf("second delete = %v, want nil (already gone)", err)
	}
	if _, err := d.Create(ctx, &gtpu.Tunnel{Name: "x", Src: "10.11.1.1", Dst: "10.11.1.1"}); !errors.Is(err, df6.ErrBadValue) {
		t.Fatalf("src == dst: %v", err)
	}
	if _, err := d.Create(ctx, &gtpu.Tunnel{Name: "x", Src: "10.11.1.1", Dst: "10.11.1.2", DecapNext: 9}); !errors.Is(err, df6.ErrBadValue) {
		t.Fatalf("decap_next 9: %v", err)
	}
	if f.crashes != 0 {
		t.Fatalf("%d requests would have crashed VPP", f.crashes)
	}
	if n := len(f.CallsNamed("gtpu_add_del_tunnel_v2")); n != 2 {
		t.Fatalf("sent %d add/del, want 2", n)
	}
}
