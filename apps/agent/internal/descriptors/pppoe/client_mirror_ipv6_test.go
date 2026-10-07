package pppoe_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/fib_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/pppoe"
)

// recordMirror6 answers sw_interface_get_table per family (IPv4 table 9401, IPv6 table 9402) and records the calls in
// order, so the tests can check family, table and add/withdraw ordering.
func newRecordMirror6() (*recordMirror, *[]string) {
	f := newRecordMirror(0)
	order := &[]string{}
	f.On("sw_interface_get_table", func(req api.Message) ([]api.Message, error) {
		if req.(*interfaces.SwInterfaceGetTable).IsIPv6 {
			return []api.Message{&interfaces.SwInterfaceGetTableReply{VrfID: 9402}}, nil
		}
		return []api.Message{&interfaces.SwInterfaceGetTableReply{VrfID: 9401}}, nil
	})
	f.On("sw_interface_add_del_address", func(req api.Message) ([]api.Message, error) {
		r := req.(*interfaces.SwInterfaceAddDelAddress)
		f.addrs = append(f.addrs, r)
		*order = append(*order, "addr "+r.Prefix.String())
		return []api.Message{&interfaces.SwInterfaceAddDelAddressReply{}}, nil
	})
	f.On("ip_route_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip.IPRouteAddDel)
		f.routes = append(f.routes, r)
		*order = append(*order, "route "+r.Route.Prefix.String())
		return []api.Message{&ip.IPRouteAddDelReply{}}, nil
	})
	return f, order
}

var dual = pppoe.Mirror{Interface: "wan0", LocalIPv4: "203.0.113.7/32", PeerIPv4: "203.0.113.1",
	LocalIPv6: []string{"2001:db8:9::100/128", "2001:db8:9:0:1:2:3:4/128"}, PeerIPv6: "fe80::940e:fe14:36be:d2f3",
	DefaultRoute: true, MSSClamp: true, MTU: 1492}

func TestClientMirrorIPv6Up(t *testing.T) {
	f, order := newRecordMirror6()
	wan := f.AddInterface("wan0", "")
	m := pppoe.NewClientMirror(f, "w9", nil, pppoe.WithRouteTablePolicy(allowAll))
	if err := m.Apply(context.Background(), dual, true); err != nil {
		t.Fatal(err)
	}
	want := []string{"addr 203.0.113.7/32", "addr 2001:db8:9::100/128", "addr 2001:db8:9:0:1:2:3:4/128", "route 0.0.0.0/0", "route ::/0"}
	if len(*order) != len(want) {
		t.Fatalf("calls %v, want %v", *order, want)
	}
	for i := range want {
		if (*order)[i] != want[i] {
			t.Fatalf("calls %v, want %v", *order, want)
		}
	}
	r6 := f.routes[1]
	if !r6.IsAdd || !r6.IsMultipath || r6.Route.TableID != 9402 || r6.Route.NPaths != 1 {
		t.Fatalf("IPv6 default must be a single multipath path in the interface's IPv6 table: %+v", r6)
	}
	p := r6.Route.Paths[0]
	if p.Proto != fib_types.FIB_API_PATH_NH_PROTO_IP6 || p.TableID != 9402 || p.SwIfIndex != wan ||
		p.Nh.Address.GetIP6().String() != "fe80::940e:fe14:36be:d2f3" {
		t.Fatalf("IPv6 path %+v", p)
	}
	if f.routes[0].Route.TableID != 9401 {
		t.Fatalf("IPv4 default in table %d, want 9401", f.routes[0].Route.TableID)
	}
	// one clamp call covers both families: IPv6 MSS = MTU-60, RX+TX, consistent with IPv4's MTU-40
	if len(f.mss) != 1 || f.mss[0].IPv4Mss != 1452 || f.mss[0].IPv6Mss != 1432 || f.mss[0].IPv6Direction != f.mss[0].IPv4Direction {
		t.Fatalf("mss %+v", f.mss)
	}
}

// Withdrawal removes the routes first, then the addresses, only this session's path (IsMultipath).
func TestClientMirrorIPv6WithdrawRoutesBeforeAddresses(t *testing.T) {
	f, order := newRecordMirror6()
	f.AddInterface("wan0", "")
	present := map[string]bool{}
	for _, a := range append([]string{dual.LocalIPv4}, dual.LocalIPv6...) {
		present[a] = true
	}
	f.On("ip_address_dump", func(req api.Message) ([]api.Message, error) {
		var out []api.Message
		for a := range present {
			p, _ := ip_types.ParseAddressWithPrefix(a)
			if (p.Address.Af == ip_types.ADDRESS_IP6) == req.(*ip.IPAddressDump).IsIPv6 {
				out = append(out, &ip.IPAddressDetails{Prefix: p})
			}
		}
		return out, nil
	})
	m := pppoe.NewClientMirror(f, "w9", nil, pppoe.WithRouteTablePolicy(allowAll))
	if err := m.Apply(context.Background(), dual, false); err != nil {
		t.Fatal(err)
	}
	want := []string{"route 0.0.0.0/0", "route ::/0", "addr 203.0.113.7/32", "addr 2001:db8:9::100/128", "addr 2001:db8:9:0:1:2:3:4/128"}
	for i := range want {
		if i >= len(*order) || (*order)[i] != want[i] {
			t.Fatalf("withdraw order %v, want %v", *order, want)
		}
	}
	for _, r := range f.routes {
		if r.IsAdd || !r.IsMultipath || r.Route.NPaths != 1 {
			t.Fatalf("withdraw must delete only this path: %+v", r)
		}
	}
	for _, a := range f.addrs {
		if a.IsAdd {
			t.Fatalf("withdraw added %v", a.Prefix)
		}
	}
	if f.mss[0].IPv6Mss != 0 || f.mss[0].IPv4Mss != 0 {
		t.Fatalf("clamp not disabled: %+v", f.mss)
	}
}

// IPv6 addresses without a router, or a session without the default route, get no ::/0; an IPv6-only session gets
// no IPv4 route.
func TestClientMirrorIPv6RouteConditions(t *testing.T) {
	for name, mir := range map[string]pppoe.Mirror{
		"no router":     {Interface: "wan0", LocalIPv6: []string{"2001:db8:9::100/128"}, DefaultRoute: true},
		"no default":    {Interface: "wan0", LocalIPv6: []string{"2001:db8:9::100/128"}, PeerIPv6: "fe80::1"},
		"no v6 address": {Interface: "wan0", PeerIPv6: "fe80::1", DefaultRoute: true},
	} {
		t.Run(name, func(t *testing.T) {
			f, _ := newRecordMirror6()
			f.AddInterface("wan0", "")
			if err := pppoe.NewClientMirror(f, "w9", nil, pppoe.WithRouteTablePolicy(allowAll)).Apply(context.Background(), mir, true); err != nil {
				t.Fatal(err)
			}
			if len(f.routes) != 0 {
				t.Fatalf("unexpected routes %+v", f.routes)
			}
		})
	}
	f, _ := newRecordMirror6()
	f.AddInterface("wan0", "")
	v6only := pppoe.Mirror{Interface: "wan0", LocalIPv6: []string{"2001:db8:9::100/128"}, PeerIPv6: "fe80::1", DefaultRoute: true}
	if err := pppoe.NewClientMirror(f, "w9", nil, pppoe.WithRouteTablePolicy(allowAll)).Apply(context.Background(), v6only, true); err != nil {
		t.Fatal(err)
	}
	if len(f.routes) != 1 || f.routes[0].Route.Prefix.String() != "::/0" {
		t.Fatalf("IPv6-only routes %+v", f.routes)
	}
}

// The IPv6 table is policy-checked like IPv4, before anything is written.
func TestClientMirrorIPv6RouteRefusedByPolicy(t *testing.T) {
	f, _ := newRecordMirror6()
	f.AddInterface("wan0", "")
	m := pppoe.NewClientMirror(f, "w9", nil, pppoe.WithRouteTablePolicy(func(table uint32) bool { return table == 9401 }))
	if err := m.Apply(context.Background(), dual, true); !errors.Is(err, pppoe.ErrNotGlobalsOwner) {
		t.Fatalf("IPv6 table outside the policy must be refused, got %v", err)
	}
	if len(f.addrs)+len(f.routes)+len(f.mss) != 0 {
		t.Fatalf("refused up wrote %d/%d/%d", len(f.addrs), len(f.routes), len(f.mss))
	}
}

func TestClientMirrorIPv6RejectsBadValues(t *testing.T) {
	for name, mir := range map[string]pppoe.Mirror{
		"v4 in v6 list":  {Interface: "wan0", LocalIPv6: []string{"203.0.113.7/32"}},
		"mapped":         {Interface: "wan0", LocalIPv6: []string{"::ffff:203.0.113.7/128"}},
		"junk":           {Interface: "wan0", LocalIPv6: []string{"2001:db8::zz/128"}},
		"v4 router":      {Interface: "wan0", PeerIPv6: "203.0.113.1"},
		"zoned router":   {Interface: "wan0", PeerIPv6: "fe80::1%ppp0"},
		"v6 in v4 field": {Interface: "wan0", LocalIPv4: "2001:db8::1/128"},
	} {
		t.Run(name, func(t *testing.T) {
			f, _ := newRecordMirror6()
			f.AddInterface("wan0", "")
			if err := pppoe.NewClientMirror(f, "w9", nil, pppoe.WithRouteTablePolicy(allowAll)).Apply(context.Background(), mir, true); err == nil {
				t.Fatal("accepted")
			}
			if len(f.addrs)+len(f.routes) != 0 {
				t.Fatal("wrote before validation")
			}
		})
	}
}
