package ipip_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	ipipapi "ngfw/agent/binapi/ipip"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/ipip"
	"ngfw/agent/internal/scheduler"
)

type fakeIPIP struct {
	*df6test.FakeVPP
	tunnels map[uint32]ipipapi.IpipTunnel
	sixrd   map[uint32]*ipipapi.Ipip6rdAddTunnel
}

func newFakeIPIP() *fakeIPIP {
	f := &fakeIPIP{FakeVPP: df6test.NewFakeVPP(), tunnels: map[uint32]ipipapi.IpipTunnel{}, sixrd: map[uint32]*ipipapi.Ipip6rdAddTunnel{}}
	f.On("ipip_add_tunnel", func(req api.Message) ([]api.Message, error) {
		r := req.(*ipipapi.IpipAddTunnel)
		idx := f.AddInterface(ipip.InterfaceName(r.Tunnel.Instance), "")
		t := r.Tunnel
		t.SwIfIndex = interface_types.InterfaceIndex(idx)
		f.tunnels[idx] = t
		return []api.Message{&ipipapi.IpipAddTunnelReply{SwIfIndex: t.SwIfIndex}}, nil
	})
	f.On("ipip_del_tunnel", func(req api.Message) ([]api.Message, error) {
		idx := uint32(req.(*ipipapi.IpipDelTunnel).SwIfIndex)
		if _, ok := f.tunnels[idx]; !ok {
			return []api.Message{&ipipapi.IpipDelTunnelReply{Retval: -6}}, nil
		}
		delete(f.tunnels, idx)
		f.RemoveInterface(idx)
		return []api.Message{&ipipapi.IpipDelTunnelReply{}}, nil
	})
	f.On("ipip_tunnel_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 100; idx++ {
			if t, ok := f.tunnels[idx]; ok {
				out = append(out, &ipipapi.IpipTunnelDetails{Tunnel: t})
			}
			if s, ok := f.sixrd[idx]; ok {
				// VPP lists 6rd tunnels in the same dump, without their prefixes.
				out = append(out, &ipipapi.IpipTunnelDetails{Tunnel: ipipapi.IpipTunnel{Instance: idx, Src: df6test.Addr(s.IP4Src.String()), SwIfIndex: interface_types.InterfaceIndex(idx), TableID: s.IP4TableID}})
			}
		}
		return out, nil
	})
	f.On("ipip_6rd_add_tunnel", func(req api.Message) ([]api.Message, error) {
		r := req.(*ipipapi.Ipip6rdAddTunnel)
		for _, x := range f.sixrd {
			if x.IP6Prefix == r.IP6Prefix && x.IP6TableID == r.IP6TableID {
				return []api.Message{&ipipapi.Ipip6rdAddTunnelReply{Retval: -65}}, nil // IF_ALREADY_EXISTS
			}
		}
		idx := f.AddInterface(ipip.InterfaceName(50+uint32(len(f.sixrd))), "") //nolint:gosec // tiny test map
		f.sixrd[idx] = r
		return []api.Message{&ipipapi.Ipip6rdAddTunnelReply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
	})
	f.On("ipip_6rd_del_tunnel", func(req api.Message) ([]api.Message, error) {
		idx := uint32(req.(*ipipapi.Ipip6rdDelTunnel).SwIfIndex)
		delete(f.sixrd, idx)
		f.RemoveInterface(idx)
		return []api.Message{&ipipapi.Ipip6rdDelTunnelReply{}}, nil
	})
	return f
}

func TestTunnelDescriptor(t *testing.T) {
	ctx := context.Background()
	f := newFakeIPIP()
	other := f.AddInterface("ipip3", "w3:ipip3")
	f.tunnels[other] = ipipapi.IpipTunnel{Instance: 3, SwIfIndex: interface_types.InterfaceIndex(other), Src: df6test.Addr("10.3.0.1"), Dst: df6test.Addr("10.3.0.2")}

	reg := scheduler.NewRegistry()
	ipip.Register(reg, f, "w11")
	if reg.Len() != 2 {
		t.Fatalf("registered %d descriptors, want 2", reg.Len())
	}
	d := ipip.NewTunnel(f, "w11")
	desired := &ipip.Tunnel{Instance: 1100, Src: "10.11.1.1", Dst: "10.11.1.2", TableId: 11001, Dscp: 46, Flags: 1}
	if k := d.KeyOf(desired); k != "ipip.tunnel/ipip1100" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 1 || deps[0].Key != "vrf/11001" {
		t.Fatalf("Dependencies = %+v", deps)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	m := meta.(df6.IfMeta)
	if f.Tag(m.SwIfIndex) != "w11:ipip1100" {
		t.Fatalf("tag = %q", f.Tag(m.SwIfIndex))
	}
	req := f.CallsNamed("ipip_add_tunnel")[0].(*ipipapi.IpipAddTunnel)
	if req.Tunnel.Instance != 1100 || req.Tunnel.TableID != 11001 || req.Tunnel.Dscp != 46 || req.Tunnel.Flags != 1 || req.Tunnel.Dst != df6test.Addr("10.11.1.2") {
		t.Fatalf("request = %+v", req)
	}
	// A 6rd tunnel of ours is in the same dump but must not be claimed by ipip.tunnel.
	s := ipip.NewSixrd(f, "w11")
	sixrd := &ipip.Tunnel6Rd{Name: "w11-6rd", Ip6Prefix: "2001:db8:1100::/48", Ip4Prefix: "10.11.0.0/16", Ip4Src: "10.11.1.1"}
	smeta, err := s.Create(ctx, sixrd)
	if err != nil {
		t.Fatal(err)
	}
	if f.Tag(smeta.(df6.IfMeta).SwIfIndex) != "w11:w11-6rd" {
		t.Fatalf("6rd tag = %q", f.Tag(smeta.(df6.IfMeta).SwIfIndex))
	}
	sreq := f.CallsNamed("ipip_6rd_add_tunnel")[0].(*ipipapi.Ipip6rdAddTunnel)
	if sreq.IP6Prefix.Len != 48 || sreq.IP4Prefix.Len != 16 || sreq.IP4Src != df6test.IP4("10.11.1.1") {
		t.Fatalf("6rd request = %+v", sreq)
	}
	if _, err := s.Retrieve(ctx); !errors.Is(err, df6.ErrRetrieveUnsupported) {
		t.Fatalf("6rd Retrieve = %v, want ErrRetrieveUnsupported (write-only)", err)
	}
	if _, err := s.Update(ctx, sixrd, sixrd, smeta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("6rd Update = %v", err)
	}

	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != 1 || actual[0].Key != "ipip.tunnel/ipip1100" || !proto.Equal(actual[0].Value, desired) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v", actual)
	}
	if _, err := d.Update(ctx, desired, &ipip.Tunnel{Instance: 1100, Src: "10.11.1.1", Dst: "10.11.1.3"}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update = %v", err)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, sixrd, smeta); err != nil {
		t.Fatal(err)
	}
	if actual, _ := d.Retrieve(ctx); len(actual) != 0 {
		t.Fatalf("after delete: %+v", actual)
	}
	if !f.Has(other) {
		t.Fatal("other owner's tunnel touched")
	}
	if del := f.CallsNamed("ipip_del_tunnel"); len(del) != 1 || del[0].(*ipipapi.IpipDelTunnel).SwIfIndex != interface_types.InterfaceIndex(m.SwIfIndex) {
		t.Fatalf("del calls = %+v", del)
	}
}

func TestTunnelVariantsAndErrors(t *testing.T) {
	ctx := context.Background()
	f := newFakeIPIP()
	d := ipip.NewTunnel(f, "w11")
	cases := []*ipip.Tunnel{
		{Instance: 1101, Mode: ipip.TunnelMode_MP, Src: "10.11.1.1"},
		{Instance: 1102, Src: "fd11:1::1", Dst: "fd11:1::2", Flags: 8},
	}
	for _, c := range cases {
		if _, err := d.Create(ctx, c); err != nil {
			t.Fatalf("%v: %v", c, err)
		}
	}
	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 2 {
		t.Fatalf("Retrieve = %v, %v", actual, err)
	}
	for i, c := range cases {
		if !proto.Equal(actual[i].Value, c) {
			t.Errorf("case %d: %v != %v", i, actual[i].Value, c)
		}
	}
	bad := []*ipip.Tunnel{
		{Instance: df6.NoInterface, Src: "10.11.1.1", Dst: "10.11.1.2"},
		{Instance: 1, Dst: "10.11.1.2"},
		{Instance: 1, Src: "10.11.1.1"},
		{Instance: 1, Src: "10.11.1.1", Dst: "10.11.1.2", Dscp: 64},
		{Instance: 1, Mode: ipip.TunnelMode_MP, Src: "10.11.1.1", Dst: "10.11.1.2"},
	}
	for _, b := range bad {
		if _, err := d.Create(ctx, b); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("%v: %v", b, err)
		}
	}
	s := ipip.NewSixrd(f, "w11")
	for _, b := range []*ipip.Tunnel6Rd{{}, {Name: "x"}, {Name: "x", Ip6Prefix: "2001:db8::/32", Ip4Prefix: "10.0.0.0/8", Ip4Src: "10.0.0.1", TcTos: 256}} {
		if _, err := s.Create(ctx, b); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("6rd %v: %v", b, err)
		}
	}
	if deps := s.Dependencies(&ipip.Tunnel6Rd{Ip6TableId: 11001, Ip4TableId: 11002}); len(deps) != 2 {
		t.Fatalf("6rd deps = %+v", deps)
	}
}

// TestSixrdResyncAndRestart (review H3): the write-only 6RD tunnel is re-applied on every
// resync without a second add, and is deletable after an agent restart (no Meta) by its tag.
func TestSixrdResyncAndRestart(t *testing.T) {
	ctx := context.Background()
	f := newFakeIPIP()
	d := ipip.NewSixrd(f, "w11rs")
	obj := &ipip.Tunnel6Rd{Name: "w11rs-6rd", Ip6Prefix: "fd11:6d::/32", Ip4Prefix: "10.11.0.0/16", Ip4Src: "10.11.1.1"}
	m1, err := d.Create(ctx, obj)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := d.Create(ctx, obj) // resync
	if err != nil || m2 != m1 {
		t.Fatalf("resync create = %v %v, want adopt %v", m2, err, m1)
	}
	if n := len(f.CallsNamed("ipip_6rd_add_tunnel")); n != 1 {
		t.Fatalf("sent %d adds, want 1", n)
	}
	fresh := ipip.NewSixrd(f, "w11rs") // agent restart
	if err := fresh.Delete(ctx, obj, nil); err != nil {
		t.Fatalf("delete without meta: %v", err)
	}
	if len(f.sixrd) != 0 {
		t.Fatal("6rd tunnel not deleted")
	}
	if err := fresh.Delete(ctx, obj, nil); err != nil || len(f.CallsNamed("ipip_6rd_del_tunnel")) != 1 {
		t.Fatalf("second delete: %v", err)
	}
}
