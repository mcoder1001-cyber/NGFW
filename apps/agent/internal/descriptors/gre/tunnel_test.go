package gre_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	greapi "ngfw/agent/binapi/gre"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/gre"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// fakeGRE models the gre plugin: tunnels keyed by sw_if_index, add creates an interface.
type fakeGRE struct {
	*df6test.FakeVPP
	tunnels map[uint32]greapi.GreTunnelV2
}

func newFakeGRE() *fakeGRE {
	f := &fakeGRE{FakeVPP: df6test.NewFakeVPP(), tunnels: map[uint32]greapi.GreTunnelV2{}}
	f.On("gre_tunnel_add_del_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*greapi.GreTunnelAddDelV2)
		if r.IsAdd {
			idx := f.AddInterface(gre.InterfaceName(r.Tunnel.Instance), "")
			t := r.Tunnel
			t.SwIfIndex = interface_types.InterfaceIndex(idx)
			f.tunnels[idx] = t
			return []api.Message{&greapi.GreTunnelAddDelV2Reply{SwIfIndex: t.SwIfIndex}}, nil
		}
		for idx, t := range f.tunnels {
			if t.Src == r.Tunnel.Src && t.Dst == r.Tunnel.Dst && t.OuterTableID == r.Tunnel.OuterTableID {
				delete(f.tunnels, idx)
				f.RemoveInterface(idx)
				return []api.Message{&greapi.GreTunnelAddDelV2Reply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
			}
		}
		return []api.Message{&greapi.GreTunnelAddDelV2Reply{Retval: -6}}, nil // VNET_API_ERROR_NO_SUCH_ENTRY
	})
	f.On("gre_tunnel_v2_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 100; idx++ {
			if t, ok := f.tunnels[idx]; ok {
				out = append(out, &greapi.GreTunnelV2Details{Tunnel: t})
			}
		}
		return out, nil
	})
	return f
}

func TestTunnelDescriptor(t *testing.T) {
	ctx := context.Background()
	f := newFakeGRE()
	// Another owner's tunnel on the same VPP.
	other := f.AddInterface("gre7", "w3:gre7")
	f.tunnels[other] = greapi.GreTunnelV2{Instance: 7, SwIfIndex: interface_types.InterfaceIndex(other), Src: df6test.Addr("10.3.0.1"), Dst: df6test.Addr("10.3.0.2")}

	d := gre.NewTunnel(f, "w11")
	reg := scheduler.NewRegistry()
	gre.Register(reg, f, "w11")
	if _, ok := reg.Get(gre.TunnelName); !ok {
		t.Fatal("Register did not register gre.tunnel")
	}

	desired := &gre.Tunnel{Instance: 1100, Src: "10.11.1.1", Dst: "10.11.1.2", OuterTableId: 11001, Type: gre.TunnelType_L3}
	if k := d.KeyOf(desired); k != "gre.tunnel/gre1100" {
		t.Fatalf("KeyOf = %s", k)
	}
	deps := d.Dependencies(desired)
	if len(deps) != 1 || deps[0].Key != "vrf/11001" || deps[0].Optional {
		t.Fatalf("Dependencies = %+v", deps)
	}
	if deps := d.Dependencies(&gre.Tunnel{Instance: 1, Src: "10.11.1.1", Dst: "10.11.1.2"}); len(deps) != 0 {
		t.Fatalf("table 0 must not be a dependency: %+v", deps)
	}

	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	m := meta.(df6.IfMeta)
	if f.Tag(m.SwIfIndex) != "w11:gre1100" {
		t.Fatalf("owner tag = %q", f.Tag(m.SwIfIndex))
	}
	calls := f.CallsNamed("gre_tunnel_add_del_v2")
	if len(calls) != 1 {
		t.Fatalf("add calls = %d", len(calls))
	}
	req := calls[0].(*greapi.GreTunnelAddDelV2)
	if !req.IsAdd || req.Tunnel.Instance != 1100 || req.Tunnel.OuterTableID != 11001 || req.Tunnel.Src != df6test.Addr("10.11.1.1") || req.Tunnel.Dst != df6test.Addr("10.11.1.2") {
		t.Fatalf("request = %+v", req)
	}

	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != 1 {
		t.Fatalf("Retrieve = %d objects, want only ours: %+v", len(actual), actual)
	}
	if actual[0].Key != "gre.tunnel/gre1100" || !proto.Equal(actual[0].Value, desired) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v, want key/value/meta as created (value %v)", actual[0], actual[0].Value)
	}

	// Update: everything is immutable.
	if _, err := d.Update(ctx, desired, &gre.Tunnel{Instance: 1100, Src: "10.11.1.1", Dst: "10.11.1.3"}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v, want ErrRecreate", err)
	}

	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if actual, _ := d.Retrieve(ctx); len(actual) != 0 {
		t.Fatalf("after Delete Retrieve = %+v", actual)
	}
	if !f.Has(other) {
		t.Fatal("the other owner's tunnel was touched")
	}
}

func TestTunnelVariants(t *testing.T) {
	ctx := context.Background()
	f := newFakeGRE()
	d := gre.NewTunnel(f, "w11")
	cases := []*gre.Tunnel{
		{Instance: 1101, Type: gre.TunnelType_TEB, Src: "10.11.1.1", Dst: "10.11.1.2"},
		{Instance: 1102, Type: gre.TunnelType_ERSPAN, Src: "10.11.1.1", Dst: "10.11.1.2", SessionId: 7},
		{Instance: 1103, Mode: gre.TunnelMode_MP, Src: "10.11.1.1"},
		{Instance: 1104, Src: "fd11:1::1", Dst: "fd11:1::2", Key: 42, Flags: 4},
	}
	for _, c := range cases {
		meta, err := d.Create(ctx, c)
		if err != nil {
			t.Fatalf("%v: %v", c, err)
		}
		t.Cleanup(func() { _ = d.Delete(ctx, c, meta) })
	}
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != len(cases) {
		t.Fatalf("Retrieve = %d, want %d", len(actual), len(cases))
	}
	for i, c := range cases {
		if !proto.Equal(actual[i].Value, c) {
			t.Errorf("case %d: Retrieve %v != desired %v", i, actual[i].Value, c)
		}
	}
	// Canonical addresses: a non-canonical desired string is normalised by Retrieve, but KeyOf
	// stays the same.
	if d.KeyOf(&gre.Tunnel{Instance: 1104, Src: "FD11:1:0::1", Dst: "fd11:1::2"}) != "gre.tunnel/gre1104" {
		t.Fatal("key must be independent of address spelling")
	}
}

func TestTunnelErrors(t *testing.T) {
	ctx := context.Background()
	f := newFakeGRE()
	d := gre.NewTunnel(f, "w11")
	bad := []*gre.Tunnel{
		{Instance: df6.NoInterface, Src: "10.11.1.1", Dst: "10.11.1.2"},
		{Instance: 1, Dst: "10.11.1.2"},
		{Instance: 1, Src: "10.11.1.1"},
		{Instance: 1, Mode: gre.TunnelMode_MP, Src: "10.11.1.1", Dst: "10.11.1.2"},
		{Instance: 1, Src: "10.11.1.1", Dst: "10.11.1.2", SessionId: 5},
		{Instance: 1, Type: gre.TunnelType_ERSPAN, Src: "10.11.1.1", Dst: "10.11.1.2", SessionId: 5000},
	}
	for _, b := range bad {
		if _, err := d.Create(ctx, b); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("%v: err = %v, want ErrBadValue", b, err)
		}
		if k := d.KeyOf(b); k != "gre.tunnel/invalid" {
			t.Errorf("%v: KeyOf = %s", b, k)
		}
	}
	if _, err := d.Create(ctx, &gre.Tunnel{Instance: 1, Src: "nope", Dst: "10.11.1.2"}); !errors.Is(err, df6.ErrBadValue) {
		t.Errorf("bad address: %v", err)
	}
	if len(f.CallsNamed("gre_tunnel_add_del_v2")) != 0 {
		t.Fatal("invalid objects must not reach VPP")
	}
	// Tagging failure rolls the add back.
	f.Fail("sw_interface_tag_add_del", fmt.Errorf("boom"))
	if _, err := d.Create(ctx, &gre.Tunnel{Instance: 1105, Src: "10.11.1.1", Dst: "10.11.1.9"}); err == nil {
		t.Fatal("tag failure must fail Create")
	}
	if n := len(f.CallsNamed("gre_tunnel_add_del_v2")); n != 2 {
		t.Fatalf("add + rollback del expected, got %d calls", n)
	}
	if len(f.tunnels) != 0 {
		t.Fatal("rollback left a tunnel behind")
	}
	// VPP errors surface.
	f.SetConnected(false)
	if _, err := d.Retrieve(ctx); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected: %v", err)
	}
	f.SetConnected(true)
	// A tunnel VPP no longer has is already deleted: no request is sent (V8 lesson).
	before := len(f.CallsNamed("gre_tunnel_add_del_v2"))
	if err := d.Delete(ctx, &gre.Tunnel{Instance: 9, Src: "10.11.9.1", Dst: "10.11.9.2"}, df6.IfMeta{SwIfIndex: 99}); err != nil {
		t.Fatalf("delete of a missing tunnel = %v, want nil", err)
	}
	if len(f.CallsNamed("gre_tunnel_add_del_v2")) != before {
		t.Fatal("delete of a missing tunnel reached VPP")
	}
	// A VPP error on a present tunnel surfaces.
	idx := f.AddInterface("gre1109", "w11:gre1109")
	f.tunnels[idx] = greapi.GreTunnelV2{Instance: 1109, SwIfIndex: interface_types.InterfaceIndex(idx), Src: df6test.Addr("10.11.9.1"), Dst: df6test.Addr("10.11.9.3")}
	f.On("gre_tunnel_add_del_v2", func(api.Message) ([]api.Message, error) {
		return []api.Message{&greapi.GreTunnelAddDelV2Reply{Retval: -6}}, nil
	})
	if err := d.Delete(ctx, &gre.Tunnel{Instance: 1109, Src: "10.11.9.1", Dst: "10.11.9.3"}, df6.IfMeta{SwIfIndex: idx}); err == nil {
		t.Fatal("non-zero retval must be an error")
	}
	if err := d.Delete(ctx, &gre.Tunnel{Instance: 9, Src: "10.11.9.1", Dst: "10.11.9.2"}, "bad"); !errors.Is(err, df6.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}
}
