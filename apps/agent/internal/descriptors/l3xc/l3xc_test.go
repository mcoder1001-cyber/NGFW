package l3xc_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/interface_types"
	l3xcapi "ngfw/agent/binapi/l3xc"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/descriptors/l3xc"
	"ngfw/agent/internal/scheduler"
)

const (
	owner   = "w2"
	loopKey = "interface.loopback/loop201"
	tapKey  = "tapv2.tap/w2-tap0"
)

var ctx = context.Background()

type key struct {
	sw  uint32
	ip6 bool
}

type fakeL3xc struct {
	*ifacetest.VPP
	xcs              map[key]l3xcapi.L3xc
	loop, tap, other uint32
}

func newFake() *fakeL3xc {
	f := &fakeL3xc{VPP: ifacetest.New(), xcs: map[key]l3xcapi.L3xc{}}
	f.loop = f.Add("loop201", "Loopback", "w2:loop201")
	f.tap = f.Add("tap0", "tap", "w2:w2-tap0")
	f.other = f.Add("tap1", "tap", "w3:w3-tap0")
	f.xcs[key{f.other, false}] = l3xcapi.L3xc{SwIfIndex: interface_types.InterfaceIndex(f.other), NPaths: 1, Paths: []fib_types.FibPath{{SwIfIndex: ^uint32(0), TableID: 3001}}}
	f.On("l3xc_update", func(req api.Message) ([]api.Message, error) {
		r := req.(*l3xcapi.L3xcUpdate)
		if _, ok := f.Ifs[uint32(r.L3xc.SwIfIndex)]; !ok {
			return []api.Message{&l3xcapi.L3xcUpdateReply{Retval: -2}}, nil
		}
		f.xcs[key{uint32(r.L3xc.SwIfIndex), r.L3xc.IsIP6}] = r.L3xc
		return []api.Message{&l3xcapi.L3xcUpdateReply{StatsIndex: 7}}, nil
	})
	f.On("l3xc_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*l3xcapi.L3xcDel)
		if _, ok := f.xcs[key{uint32(r.SwIfIndex), r.IsIP6}]; !ok {
			return []api.Message{&l3xcapi.L3xcDelReply{Retval: -6}}, nil
		}
		delete(f.xcs, key{uint32(r.SwIfIndex), r.IsIP6})
		return []api.Message{&l3xcapi.L3xcDelReply{}}, nil
	})
	f.On("l3xc_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*l3xcapi.L3xcDump)
		var out []api.Message
		for k, x := range f.xcs {
			if uint32(r.SwIfIndex) == ^uint32(0) || uint32(r.SwIfIndex) == k.sw {
				out = append(out, &l3xcapi.L3xcDetails{L3xc: x})
			}
		}
		return out, nil
	})
	return f
}

func TestL3xc(t *testing.T) {
	f := newFake()
	r := scheduler.NewRegistry()
	l3xc.Register(r, f, owner)
	if r.Len() != 1 || r.Names()[0] != "l3xc.l3xc" {
		t.Fatal(r.Names())
	}
	d := l3xc.New(f, owner)
	desired := &l3xc.L3Xc{Interface: tapKey, Paths: []*l3xc.Path{
		{NextHop: "10.2.1.254", Interface: loopKey, Weight: 1},
		{NextHop: "10.2.2.254", Table: 2001, Weight: 1, Preference: 1},
	}}
	if d.KeyOf(desired) != "l3xc.l3xc/w2-tap0/ip4" {
		t.Fatal(d.KeyOf(desired))
	}
	deps := d.Dependencies(desired)
	if len(deps) != 3 || deps[0].Key != tapKey || deps[1].Key != loopKey || deps[2].Key != "vrf/2001" || deps[2].Optional {
		t.Fatalf("deps = %+v", deps)
	}
	if kvs, _ := d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("the other owner's l3xc is visible: %+v", kvs)
	}
	for _, bad := range []*l3xc.L3Xc{
		{Interface: tapKey},
		{Interface: tapKey, Paths: []*l3xc.Path{{NextHop: "fd00::1"}}},
		{Interface: tapKey, Ipv6: true, Paths: []*l3xc.Path{{NextHop: "10.0.0.1"}}},
		{Interface: tapKey, Paths: []*l3xc.Path{{NextHop: "10.0.0.1", Interface: "tapv2.tap/w3-tap0"}}},
	} {
		if _, err := d.Create(ctx, bad); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	req := f.CallsNamed("l3xc_update")
	x := req[len(req)-1].(*l3xcapi.L3xcUpdate).L3xc
	if uint32(x.SwIfIndex) != f.tap || x.IsIP6 || x.NPaths != 2 || x.Paths[0].SwIfIndex != f.loop || x.Paths[0].Proto != fib_types.FIB_API_PATH_NH_PROTO_IP4 ||
		x.Paths[0].Nh.Address.GetIP4().String() != "10.2.1.254" || x.Paths[1].SwIfIndex != ^uint32(0) || x.Paths[1].TableID != 2001 || x.Paths[1].Preference != 1 {
		t.Fatalf("l3xc_update = %+v", x)
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil || len(kvs) != 1 || kvs[0].Key != "l3xc.l3xc/w2-tap0/ip4" || !proto.Equal(kvs[0].Value, desired) || kvs[0].Meta != meta {
		t.Fatalf("Retrieve = %+v (%v)", kvs, err)
	}
	// Update replaces paths in place; a reversed desired order still decodes sorted
	updated := &l3xc.L3Xc{Interface: tapKey, Paths: []*l3xc.Path{{NextHop: "10.2.3.1", Weight: 1}}}
	if m, err := d.Update(ctx, desired, updated, meta); err != nil || m != meta {
		t.Fatalf("Update: %v", err)
	}
	if kvs, _ = d.Retrieve(ctx); !proto.Equal(kvs[0].Value, updated) {
		t.Fatalf("after Update: %v", kvs[0].Value)
	}
	if _, err := d.Update(ctx, updated, &l3xc.L3Xc{Interface: tapKey, Ipv6: true, Paths: updated.Paths}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("af change: %v", err)
	}
	v6 := &l3xc.L3Xc{Interface: loopKey, Ipv6: true, Paths: []*l3xc.Path{{NextHop: "fd00:2::1", Interface: tapKey, Weight: 1}}}
	if _, err := d.Create(ctx, v6); err != nil {
		t.Fatal(err)
	}
	if kvs, _ = d.Retrieve(ctx); len(kvs) != 2 {
		t.Fatalf("Retrieve = %+v", kvs)
	}
	if err := d.Delete(ctx, updated, meta); err != nil {
		t.Fatal(err)
	}
	if kvs, _ = d.Retrieve(ctx); len(kvs) != 1 || kvs[0].Key != "l3xc.l3xc/loop201/ip6" || !proto.Equal(kvs[0].Value, v6) {
		t.Fatalf("after Delete: %+v", kvs)
	}
	if _, ok := f.xcs[key{f.other, false}]; !ok {
		t.Fatal("the other owner's l3xc was touched")
	}
}
