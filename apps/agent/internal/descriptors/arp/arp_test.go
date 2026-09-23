package arp

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	arpapi "ngfw/agent/binapi/arp"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

type fakeVPP struct {
	*fake.Client
	ranges  []arpapi.ProxyArp
	enabled map[uint32]bool
}

func newFakeVPP() *fakeVPP {
	v := &fakeVPP{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), enabled: map[uint32]bool{}}
	v.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 5, InterfaceName: "loop300", Tag: "w3:loop300"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 7, InterfaceName: "loop200", Tag: "w2:loop200"},
	)
	v.On("proxy_arp_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*arpapi.ProxyArpAddDel)
		for i, p := range v.ranges {
			if p == r.Proxy {
				if !r.IsAdd {
					v.ranges = append(v.ranges[:i], v.ranges[i+1:]...)
				}
				return []api.Message{&arpapi.ProxyArpAddDelReply{}}, nil
			}
		}
		if !r.IsAdd {
			return []api.Message{&arpapi.ProxyArpAddDelReply{Retval: -6}}, nil // NO_SUCH_ENTRY
		}
		v.ranges = append(v.ranges, r.Proxy)
		return []api.Message{&arpapi.ProxyArpAddDelReply{}}, nil
	})
	v.On("proxy_arp_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(v.ranges))
		for _, p := range v.ranges {
			out = append(out, &arpapi.ProxyArpDetails{Proxy: p})
		}
		return out, nil
	})
	v.On("proxy_arp_intfc_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*arpapi.ProxyArpIntfcEnableDisable)
		if r.Enable {
			v.enabled[uint32(r.SwIfIndex)] = true
		} else {
			delete(v.enabled, uint32(r.SwIfIndex))
		}
		return []api.Message{&arpapi.ProxyArpIntfcEnableDisableReply{}}, nil
	})
	v.On("proxy_arp_intfc_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for idx := range v.enabled {
			out = append(out, &arpapi.ProxyArpIntfcDetails{SwIfIndex: idx})
		}
		return out, nil
	})
	return v
}

func ip4(s string) ip_types.IP4Address {
	a, err := ip_types.ParseIP4Address(s)
	if err != nil {
		panic(err)
	}
	return a
}

func TestRangeLifecycle(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	// Another worker's range (table 2001) and one in the default table must stay invisible.
	v.ranges = []arpapi.ProxyArp{{TableID: 2001, Low: ip4("10.2.0.1"), Hi: ip4("10.2.0.9")}, {TableID: 0, Low: ip4("192.168.1.1"), Hi: ip4("192.168.1.9")}}
	d := NewRange(v, &df2.IDRange{Lo: 3000, Hi: 3999})
	if !scheduler.ValidName(d.Name()) {
		t.Fatal(d.Name())
	}

	desired := &ProxyRange{TableId: 3001, Low: "10.3.0.1", High: "10.3.0.9"}
	if k := d.KeyOf(desired); k != "arp.proxy-range/3001/10.3.0.1-10.3.0.9" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 1 || deps[0].Key != "vrf/3001" {
		t.Fatalf("Dependencies = %+v", deps)
	}
	if deps := d.Dependencies(&ProxyRange{TableId: 0}); deps != nil {
		t.Fatalf("table 0 must not depend on a vrf object: %+v", deps)
	}

	if _, err := d.Create(ctx, desired); err != nil {
		t.Fatal(err)
	}
	req := v.CallsNamed("proxy_arp_add_del")[0].(*arpapi.ProxyArpAddDel)
	if !req.IsAdd || req.Proxy != (arpapi.ProxyArp{TableID: 3001, Low: ip4("10.3.0.1"), Hi: ip4("10.3.0.9")}) {
		t.Fatalf("request = %+v", req)
	}
	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 1 || actual[0].Key != d.KeyOf(desired) || !proto.Equal(actual[0].Value, desired) {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	if _, err := d.Update(ctx, desired, &ProxyRange{TableId: 3001, Low: "10.3.0.1", High: "10.3.0.20"}, nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if err := d.Delete(ctx, desired, nil); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 0 {
		t.Fatalf("after Delete Retrieve = %+v", actual)
	}
	if len(v.ranges) != 2 {
		t.Fatalf("foreign ranges touched: %+v", v.ranges)
	}

	// Errors: outside our table range, bad addresses, inverted range, VPP retval.
	if _, err := d.Create(ctx, &ProxyRange{TableId: 2001, Low: "10.3.0.1", High: "10.3.0.9"}); err == nil {
		t.Fatal("range outside the owned tables accepted")
	}
	if _, err := d.Create(ctx, &ProxyRange{TableId: 3001, Low: "2001:db8::1", High: "2001:db8::9"}); err == nil {
		t.Fatal("IPv6 accepted")
	}
	if _, err := d.Create(ctx, &ProxyRange{TableId: 3001, Low: "10.3.0.9", High: "10.3.0.1"}); err == nil {
		t.Fatal("inverted range accepted")
	}
	if err := d.Delete(ctx, desired, nil); err == nil {
		t.Fatal("deleting a missing range must surface VPP's retval")
	}
	// Production scope: a nil range owns everything.
	all := NewRange(v, nil)
	if actual, _ = all.Retrieve(ctx); len(actual) != 2 {
		t.Fatalf("nil range Retrieve = %+v", actual)
	}
}

func TestInterfaceLifecycle(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	v.enabled[7] = true // another worker's interface
	d := NewInterface(v, "w3")
	desired := &ProxyInterface{Interface: "loop300"}
	if k := d.KeyOf(desired); k != "arp.proxy-interface/loop300" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 1 || deps[0].Key != "interface/loop300" {
		t.Fatalf("Dependencies = %+v", deps)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil || meta != (InterfaceMeta{SwIfIndex: 5}) {
		t.Fatalf("Create: %v %+v", err, meta)
	}
	req := v.CallsNamed("proxy_arp_intfc_enable_disable")[0].(*arpapi.ProxyArpIntfcEnableDisable)
	if req.SwIfIndex != 5 || !req.Enable {
		t.Fatalf("request = %+v", req)
	}
	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 1 || !proto.Equal(actual[0].Value, desired) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	if _, err := d.Update(ctx, desired, &ProxyInterface{Interface: "loop301"}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 0 || !v.enabled[7] {
		t.Fatalf("after Delete: %+v enabled=%v", actual, v.enabled)
	}
	if _, err := d.Create(ctx, &ProxyInterface{Interface: "nope"}); !errors.Is(err, df2.ErrNoSuchInterface) {
		t.Fatalf("unknown interface: %v", err)
	}
	if err := d.Delete(ctx, desired, nil); !errors.Is(err, df2.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	Register(reg, fake.New(), "w3", nil)
	if got := reg.Names(); len(got) != 2 || got[0] != RangeName || got[1] != InterfaceName {
		t.Fatalf("Names = %v", got)
	}
}
