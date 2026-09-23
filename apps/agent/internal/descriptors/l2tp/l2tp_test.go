package l2tp_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	l2tpapi "ngfw/agent/binapi/l2tp"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/l2tp"
	"ngfw/agent/internal/scheduler"
)

type fakeL2TP struct {
	*df6test.FakeVPP
	tunnels map[uint32]*l2tpapi.SwIfL2tpv3TunnelDetails
	enabled map[uint32]bool
	key     l2tpapi.L2tLookupKey
}

func newFakeL2TP() *fakeL2TP {
	f := &fakeL2TP{FakeVPP: df6test.NewFakeVPP(), tunnels: map[uint32]*l2tpapi.SwIfL2tpv3TunnelDetails{}, enabled: map[uint32]bool{}}
	n := 0
	f.On("l2tpv3_create_tunnel", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2tpapi.L2tpv3CreateTunnel)
		name := fmt.Sprintf("l2tpv3_tunnel%d", n)
		n++
		idx := f.AddInterface(name, "")
		f.tunnels[idx] = &l2tpapi.SwIfL2tpv3TunnelDetails{SwIfIndex: interface_types.InterfaceIndex(idx), InterfaceName: name, ClientAddress: r.ClientAddress, OurAddress: r.OurAddress,
			LocalSessionID: r.LocalSessionID, RemoteSessionID: r.RemoteSessionID, LocalCookie: []uint64{r.LocalCookie, 0}, RemoteCookie: r.RemoteCookie, L2SublayerPresent: r.L2SublayerPresent}
		return []api.Message{&l2tpapi.L2tpv3CreateTunnelReply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
	})
	f.On("l2tpv3_set_tunnel_cookies", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2tpapi.L2tpv3SetTunnelCookies)
		t, ok := f.tunnels[uint32(r.SwIfIndex)]
		if !ok {
			return []api.Message{&l2tpapi.L2tpv3SetTunnelCookiesReply{Retval: -6}}, nil
		}
		t.LocalCookie = []uint64{r.NewLocalCookie, t.LocalCookie[0]}
		t.RemoteCookie = r.NewRemoteCookie
		return []api.Message{&l2tpapi.L2tpv3SetTunnelCookiesReply{}}, nil
	})
	f.On("sw_if_l2tpv3_tunnel_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := uint32(0); idx < 100; idx++ {
			if t, ok := f.tunnels[idx]; ok {
				c := *t
				out = append(out, &c)
			}
		}
		return out, nil
	})
	f.On("l2tpv3_interface_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*l2tpapi.L2tpv3InterfaceEnableDisable)
		f.SetFeature("ip6-unicast", "l2tp-decap", uint32(r.SwIfIndex), r.EnableDisable)
		f.enabled[uint32(r.SwIfIndex)] = r.EnableDisable
		return []api.Message{&l2tpapi.L2tpv3InterfaceEnableDisableReply{}}, nil
	})
	f.On("l2tpv3_set_lookup_key", func(req api.Message) ([]api.Message, error) {
		f.key = req.(*l2tpapi.L2tpv3SetLookupKey).Key
		return []api.Message{&l2tpapi.L2tpv3SetLookupKeyReply{}}, nil
	})
	return f
}

func TestTunnelDescriptor(t *testing.T) {
	ctx := context.Background()
	f := newFakeL2TP()
	other := f.AddInterface("l2tpv3_tunnel9", "w3:l2tp-a")
	f.tunnels[other] = &l2tpapi.SwIfL2tpv3TunnelDetails{SwIfIndex: interface_types.InterfaceIndex(other), ClientAddress: df6test.Addr("fd03::2"), OurAddress: df6test.Addr("fd03::1"), LocalCookie: []uint64{1, 0}}

	reg := scheduler.NewRegistry()
	l2tp.Register(reg, f, "w11")
	if reg.Len() != 3 {
		t.Fatalf("registered %d", reg.Len())
	}
	d := l2tp.NewTunnel(f, "w11")
	desired := &l2tp.Tunnel{Name: "w11-l2tp1", ClientAddress: "fd11:6::2", OurAddress: "fd11:6::1", LocalSessionId: 11001, RemoteSessionId: 11002, LocalCookie: 0x1111, RemoteCookie: 0x2222, L2SublayerPresent: true}
	if k := d.KeyOf(desired); k != "l2tp.tunnel/w11-l2tp1" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 0 {
		t.Fatalf("deps = %+v", deps)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	if f.Tag(meta.(df6.IfMeta).SwIfIndex) != "w11:w11-l2tp1" {
		t.Fatalf("tag = %q", f.Tag(meta.(df6.IfMeta).SwIfIndex))
	}
	req := f.CallsNamed("l2tpv3_create_tunnel")[0].(*l2tpapi.L2tpv3CreateTunnel)
	if req.LocalSessionID != 11001 || req.RemoteSessionID != 11002 || req.LocalCookie != 0x1111 || req.RemoteCookie != 0x2222 || !req.L2SublayerPresent || req.ClientAddress != df6test.Addr("fd11:6::2") {
		t.Fatalf("request = %+v", req)
	}
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != 1 || !proto.Equal(actual[0].Value, desired) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v", actual)
	}
	// Cookies in place, everything else needs a recreate — which VPP cannot do.
	newCookies := proto.Clone(desired).(*l2tp.Tunnel)
	newCookies.LocalCookie, newCookies.RemoteCookie = 0x3333, 0x4444
	if _, err := d.Update(ctx, desired, newCookies, meta); err != nil {
		t.Fatal(err)
	}
	if a, _ := d.Retrieve(ctx); !proto.Equal(a[0].Value, newCookies) {
		t.Fatalf("cookies not updated: %v", a[0].Value)
	}
	if _, err := d.Update(ctx, newCookies, &l2tp.Tunnel{Name: "w11-l2tp1", ClientAddress: "fd11:6::3", OurAddress: "fd11:6::1", LocalCookie: 0x3333, RemoteCookie: 0x4444, LocalSessionId: 11001, RemoteSessionId: 11002, L2SublayerPresent: true}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update = %v", err)
	}
	if err := d.Delete(ctx, newCookies, meta); !errors.Is(err, df6.ErrNoDelete) {
		t.Fatalf("Delete = %v, want ErrNoDelete", err)
	}
	if !f.Has(other) {
		t.Fatal("other owner's tunnel touched")
	}
	bad := []*l2tp.Tunnel{
		{ClientAddress: "fd11::2", OurAddress: "fd11::1"},
		{Name: "x", ClientAddress: "10.11.1.2", OurAddress: "fd11::1"},
		{Name: "x", ClientAddress: "fd11::2", OurAddress: "fd11::1", EncapVrfId: 11001},
	}
	for _, b := range bad {
		if _, err := d.Create(ctx, b); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("%v: %v", b, err)
		}
	}
}

func TestGlobals(t *testing.T) {
	ctx := context.Background()
	f := newFakeL2TP()
	idx := f.AddInterface("loop1101", "w11:loop1101")
	e := l2tp.NewInterfaceEnable(f, "w11")
	en := &l2tp.InterfaceEnable{Interface: "loop1101"}
	if k := e.KeyOf(en); k != "l2tp.interface-enable/loop1101" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := e.Dependencies(en); len(deps) != 1 || deps[0].Key != "interface/loop1101" {
		t.Fatalf("deps = %+v", deps)
	}
	meta, err := e.Create(ctx, en)
	if err != nil || !f.enabled[idx] {
		t.Fatalf("enable: %v %v", err, f.enabled)
	}
	if _, err := e.Retrieve(ctx); !errors.Is(err, df6.ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve = %v", err)
	}
	if err := e.Delete(ctx, en, meta); err != nil || f.enabled[idx] {
		t.Fatalf("disable: %v %v", err, f.enabled)
	}

	k := l2tp.NewLookupKey(f)
	key := &l2tp.LookupKey{Key: l2tp.LookupKeyType_SESSION_ID}
	if k.KeyOf(key) != "l2tp.lookup-key/global" {
		t.Fatalf("KeyOf = %s", k.KeyOf(key))
	}
	if _, err := k.Create(ctx, key); err != nil || f.key != l2tpapi.L2T_LOOKUP_KEY_API_SESSION_ID {
		t.Fatalf("set: %v %v", err, f.key)
	}
	if _, err := k.Update(ctx, key, &l2tp.LookupKey{Key: l2tp.LookupKeyType_DST_ADDR}, nil); err != nil || f.key != l2tpapi.L2T_LOOKUP_KEY_API_DST_ADDR {
		t.Fatalf("update: %v %v", err, f.key)
	}
	if _, err := k.Retrieve(ctx); !errors.Is(err, df6.ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve = %v", err)
	}
	if err := k.Delete(ctx, key, nil); err != nil {
		t.Fatalf("Delete (documented no-op) = %v", err)
	}
	if _, err := k.Create(ctx, &l2tp.LookupKey{Key: 7}); !errors.Is(err, df6.ErrBadValue) {
		t.Fatalf("bad key: %v", err)
	}
}

// TestInterfaceEnableResync (review H3): two resyncs send one enable (VPP stacks features).
func TestInterfaceEnableResync(t *testing.T) {
	ctx := context.Background()
	f := newFakeL2TP()
	f.AddInterface("loop1190", "w11rs:loop1190")
	e := l2tp.NewInterfaceEnable(f, "w11rs")
	for i := 0; i < 2; i++ {
		if _, err := e.Create(ctx, &l2tp.InterfaceEnable{Interface: "loop1190"}); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(f.CallsNamed("l2tpv3_interface_enable_disable")); n != 1 {
		t.Fatalf("sent %d enables, want 1", n)
	}
	if err := e.Delete(ctx, &l2tp.InterfaceEnable{Interface: "loop1190"}, nil); err != nil {
		t.Fatal(err)
	}
	if n := len(f.CallsNamed("l2tpv3_interface_enable_disable")); n != 2 {
		t.Fatalf("delete: %d calls, want 2", n)
	}
}
