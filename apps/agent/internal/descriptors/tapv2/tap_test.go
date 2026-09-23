package tapv2_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	tapapi "ngfw/agent/binapi/tapv2"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/scheduler"
)

const owner = "w2"

var ctx = context.Background()

type fakeTap struct {
	*ifacetest.VPP
	taps map[uint32]*tapapi.SwInterfaceTapV2Details
}

func newFake() *fakeTap {
	f := &fakeTap{VPP: ifacetest.New(), taps: map[uint32]*tapapi.SwInterfaceTapV2Details{}}
	o := f.Add("tap9", "tap", "w3:w3-tap0")
	f.taps[o] = &tapapi.SwInterfaceTapV2Details{SwIfIndex: o, ID: 9, HostIfName: "w3-tap0", RxRingSz: 256, TxRingSz: 256}
	f.On("tap_create_v3", func(req api.Message) ([]api.Message, error) {
		r := req.(*tapapi.TapCreateV3)
		for _, tp := range f.taps {
			if tp.ID == r.ID {
				return []api.Message{&tapapi.TapCreateV3Reply{Retval: -100}}, nil
			}
		}
		idx := f.Add(fmt.Sprintf("tap%d", r.ID), "tap", r.Tag)
		d := &tapapi.SwInterfaceTapV2Details{SwIfIndex: idx, ID: r.ID, TxRingSz: r.TxRingSz, RxRingSz: r.RxRingSz, TapFlags: r.TapFlags, DevName: fmt.Sprintf("tap%d", r.ID)}
		if r.HostIfNameSet {
			d.HostIfName = r.HostIfName
		} else {
			d.HostIfName = fmt.Sprintf("tap%d", r.ID)
		}
		if r.HostNamespaceSet {
			d.HostNamespace = r.HostNamespace
		}
		if r.HostBridgeSet {
			d.HostBridge = r.HostBridge
		}
		if r.HostMacAddrSet {
			d.HostMacAddr = r.HostMacAddr
		}
		if r.HostIP4PrefixSet {
			d.HostIP4Prefix = r.HostIP4Prefix
		}
		if r.HostIP6PrefixSet {
			d.HostIP6Prefix = r.HostIP6Prefix
		}
		if r.HostMtuSet {
			d.HostMtuSize = r.HostMtuSize
		}
		f.taps[idx] = d
		return []api.Message{&tapapi.TapCreateV3Reply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
	})
	f.On("tap_delete_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*tapapi.TapDeleteV2)
		if _, ok := f.taps[uint32(r.SwIfIndex)]; !ok {
			return []api.Message{&tapapi.TapDeleteV2Reply{Retval: -2}}, nil
		}
		delete(f.taps, uint32(r.SwIfIndex))
		f.Remove(uint32(r.SwIfIndex))
		return []api.Message{&tapapi.TapDeleteV2Reply{}}, nil
	})
	f.On("sw_interface_tap_v2_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, tp := range f.taps {
			out = append(out, tp)
		}
		return out, nil
	})
	return f
}

func TestTap(t *testing.T) {
	f := newFake()
	r := scheduler.NewRegistry()
	tapv2.Register(r, f, owner)
	if r.Len() != 1 || r.Names()[0] != "tapv2.tap" {
		t.Fatal(r.Names())
	}
	d := tapv2.New(f, owner)
	desired := &tapv2.Tap{Name: "w2-tap0", Id: 200, HostIfName: "w2-tap0", HostIp4Prefix: "10.2.0.1/24", HostIp6Prefix: "fd00:2::1/64",
		HostMtu: 1400, RxRingSize: 512, TxRingSize: 256, Gso: true}
	if d.KeyOf(desired) != "tapv2.tap/w2-tap0" || d.Dependencies(desired) != nil {
		t.Fatal("key/deps")
	}
	if kvs, _ := d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("the other owner's tap is visible: %+v", kvs)
	}
	for _, bad := range []*tapv2.Tap{{Name: "x", Id: 1}, {Name: "x", Id: 1, RxRingSize: 256, TxRingSize: 256, HostIp4Prefix: "nope"}, {Id: 1, RxRingSize: 256, TxRingSize: 256},
		{Name: "x", Id: 1, RxRingSize: 256, TxRingSize: 256, HostIfName: "a-very-long-host-name"},
		{Name: "x", Id: 1, RxRingSize: 256, TxRingSize: 256}} { // no host_if_name: VPP would pick one → perpetual recreate (review M4)
		if _, err := d.Create(ctx, bad); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	req := f.CallsNamed("tap_create_v3")[0].(*tapapi.TapCreateV3)
	if req.ID != 200 || !req.UseRandomMac || req.Tag != "w2:w2-tap0" || !req.HostIfNameSet || req.HostIfName != "w2-tap0" || req.HostMacAddrSet ||
		!req.HostIP4PrefixSet || req.HostIP4Prefix.Len != 24 || !req.HostIP6PrefixSet || req.HostIP6Prefix.Len != 64 || !req.HostMtuSet || req.HostMtuSize != 1400 ||
		req.RxRingSz != 512 || req.TxRingSz != 256 || req.TapFlags != tapapi.TAP_API_FLAG_GSO || req.HostNamespaceSet || req.HostBridgeSet {
		t.Fatalf("tap_create_v3 = %+v", req)
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil || len(kvs) != 1 || kvs[0].Key != "tapv2.tap/w2-tap0" || !proto.Equal(kvs[0].Value, desired) || kvs[0].Meta != meta {
		t.Fatalf("Retrieve = %+v (%v)", kvs, err)
	}
	if _, err := d.Update(ctx, desired, &tapv2.Tap{Name: "w2-tap0", Id: 201}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if kvs, _ = d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("after Delete: %+v", kvs)
	}
	if len(f.taps) != 1 {
		t.Fatal("the other owner's tap was touched")
	}
}
