package afpacket_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	afpapi "ngfw/agent/binapi/af_packet"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/af_packet"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/scheduler"
)

const owner = "w2"

var ctx = context.Background()

type fakeAfp struct {
	*ifacetest.VPP
	hosts map[string]uint32 // host_if_name → sw_if_index
	links *fakeLinks        // the Linux netdevs (quiesce_test.go)
}

func newFake() *fakeAfp {
	f := &fakeAfp{VPP: ifacetest.New(), hosts: map[string]uint32{}}
	f.links = newFakeLinks(f.Client)
	f.hosts["w3-l0"] = f.Add("host-w3-l0", "af-packet", "w3:w3-l0")
	f.On("af_packet_create_v3", func(req api.Message) ([]api.Message, error) {
		r := req.(*afpapi.AfPacketCreateV3)
		if _, dup := f.hosts[r.HostIfName]; dup || r.HostIfName == "missing" {
			return []api.Message{&afpapi.AfPacketCreateV3Reply{Retval: -1}}, nil
		}
		f.links.setUp(r.HostIfName) // af_packet_create_if sets IFF_UP itself (af_packet.c:683-692)
		idx := f.Add("host-"+r.HostIfName, "af-packet", "")
		if r.Mode == afpapi.AF_PACKET_API_MODE_IP {
			f.Ifs[idx].L2Address = [6]uint8{}
		}
		f.hosts[r.HostIfName] = idx
		return []api.Message{&afpapi.AfPacketCreateV3Reply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
	})
	f.On("af_packet_delete", func(req api.Message) ([]api.Message, error) {
		r := req.(*afpapi.AfPacketDelete)
		idx, ok := f.hosts[r.HostIfName]
		if !ok {
			return []api.Message{&afpapi.AfPacketDeleteReply{Retval: -1}}, nil
		}
		delete(f.hosts, r.HostIfName)
		f.Remove(idx)
		return []api.Message{&afpapi.AfPacketDeleteReply{}}, nil
	})
	f.On("af_packet_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for name, idx := range f.hosts {
			out = append(out, &afpapi.AfPacketDetails{SwIfIndex: interface_types.InterfaceIndex(idx), HostIfName: name})
		}
		return out, nil
	})
	return f
}

// desc is the descriptor on the fake VPP with the fake link controller and a recorded, sleepless settle.
func (f *fakeAfp) desc() *afpacket.HostInterfaceDescriptor {
	return afpacket.New(f, owner, f.links.options()...)
}

func TestHostInterface(t *testing.T) {
	f := newFake()
	r := scheduler.NewRegistry()
	afpacket.Register(r, f, owner, f.links.options()...)
	if r.Len() != 1 || r.Names()[0] != "af-packet.host-interface" {
		t.Fatal(r.Names())
	}
	d := f.desc()
	eth := &afpacket.HostInterface{Name: "w2-l0", HostIfName: "w2-l0"}
	ip := &afpacket.HostInterface{Name: "w2-w0", HostIfName: "w2-w0", Mode: afpacket.Mode_MODE_IP}
	if d.KeyOf(eth) != "af-packet.host-interface/w2-l0" || d.Dependencies(eth) != nil {
		t.Fatal("key/deps")
	}
	if kvs, _ := d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("the other owner's host-interface is visible: %+v", kvs)
	}
	if _, err := d.Create(ctx, &afpacket.HostInterface{Name: "x", HostIfName: "missing"}); err == nil {
		t.Fatal("VPP error (netdev missing) must surface")
	}
	if _, err := d.Create(ctx, &afpacket.HostInterface{Name: "x"}); err == nil {
		t.Fatal("empty host_if_name accepted")
	}
	meta, err := d.Create(ctx, eth)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Create(ctx, ip); err != nil {
		t.Fatal(err)
	}
	req := f.CallsNamed("af_packet_create_v3")[1].(*afpapi.AfPacketCreateV3)
	if req.Mode != afpapi.AF_PACKET_API_MODE_ETHERNET || !req.UseRandomHwAddr || req.HostIfName != "w2-l0" {
		t.Fatalf("af_packet_create_v3 = %+v", req)
	}
	// D-108 amended by D-113: rings shrunk by frame COUNT (TX 67584 × 16 ≈ 1 MiB, RX 2048 × 8 per block ≈ 2.5 MiB, instead of
	// VPP's ≈ 76 MiB defaults, I6 stalls); the TX frame stays VPP's 2048 × 33, because the TX copy (device.c:561-573) has no
	// length check and a GSO/jumbo frame would overrun a smaller slot. The TX block must be a page multiple (the kernel's rule).
	for _, c := range f.CallsNamed("af_packet_create_v3") {
		r := c.(*afpapi.AfPacketCreateV3)
		if r.TxFrameSize != 2048*33 || r.TxFramesPerBlock != 16 || r.RxFrameSize != 2048 || r.RxFramesPerBlock != 8 || r.NumRxQueues != 1 || r.NumTxQueues != 1 {
			t.Fatalf("af_packet_create_v3 ring: tx %d × %d, rx %d × %d, queues %d/%d", r.TxFrameSize, r.TxFramesPerBlock, r.RxFrameSize, r.RxFramesPerBlock, r.NumRxQueues, r.NumTxQueues)
		}
		if block := r.TxFrameSize * r.TxFramesPerBlock; block%4096 != 0 || block > 1<<21 {
			t.Fatalf("TX block %d B: not a page multiple or over 2 MiB", block)
		}
	}
	if row, _ := f.Get(f.hosts["w2-l0"]); row.Tag != "w2:w2-l0" {
		t.Fatalf("tag = %q", row.Tag)
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil || len(kvs) != 2 {
		t.Fatalf("Retrieve = %+v (%v)", kvs, err)
	}
	for _, kv := range kvs {
		want := eth
		if kv.Key == "af-packet.host-interface/w2-w0" {
			want = ip
		}
		if !proto.Equal(kv.Value, want) {
			t.Fatalf("%s = %v, want %v", kv.Key, kv.Value, want)
		}
		if kv.Key == "af-packet.host-interface/w2-l0" && kv.Meta != meta {
			t.Fatalf("meta = %+v", kv.Meta)
		}
	}
	if _, err := d.Update(ctx, eth, ip, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, eth, meta); err != nil {
		t.Fatal(err)
	}
	if f.CallsNamed("af_packet_delete")[0].(*afpapi.AfPacketDelete).HostIfName != "w2-l0" {
		t.Fatal("delete by host_if_name")
	}
	if kvs, _ = d.Retrieve(ctx); len(kvs) != 1 || kvs[0].Key != "af-packet.host-interface/w2-w0" {
		t.Fatalf("after Delete: %+v", kvs)
	}
	if _, ok := f.hosts["w3-l0"]; !ok {
		t.Fatal("the other owner's host-interface was touched")
	}
}
