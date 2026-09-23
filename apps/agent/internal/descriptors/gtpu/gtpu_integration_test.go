package gtpu_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/gtpu"
)

func TestTunnelOnHost(t *testing.T) {
	h := df6test.Connect(t)
	loop, _ := h.Loopback(4, h.IP4(4, 1)+"/24")
	vrf := h.Table(4)
	h.IPTable(vrf, false)

	d := gtpu.NewTunnel(h.Client, h.Owner)
	fw := gtpu.NewForward(h.Client, h.Owner)
	teid := h.Table(300)
	cases := []*gtpu.Tunnel{
		{Name: h.Name("gtpu1"), Src: h.IP4(4, 1), Dst: h.IP4(4, 2), DecapNext: gtpu.DecapNext_L2, Teid: teid, Tteid: teid + 100},
		{Name: h.Name("gtpu2"), Src: h.IP4(4, 1), Dst: h.IP4(4, 3), DecapNext: gtpu.DecapNext_IP4, Teid: teid + 1, EncapVrfId: vrf, PduExtension: true, Qfi: 5},
	}
	for _, c := range cases {
		meta, err := d.Create(h.Ctx, c)
		if err != nil {
			t.Fatalf("create %v: %v", c, err)
		}
		t.Cleanup(func() { _ = d.Delete(h.Ctx, c, meta) })
	}
	forward := &gtpu.Forward{Name: h.Name("gtpufw"), Dst: h.IP4(4, 9), ForwardingType: 2, DecapNext: gtpu.DecapNext_L2, EncapVrfId: vrf}
	fmeta, err := fw.Create(h.Ctx, forward)
	if err != nil {
		t.Fatalf("create forward: %v", err)
	}
	t.Cleanup(func() { _ = fw.Delete(h.Ctx, forward, fmeta) })
	b := gtpu.NewBypass(h.Client, h.Owner)
	bdesired := &gtpu.Bypass{Interface: loop, Ipv4: true}
	bmeta, err := b.Create(h.Ctx, bdesired)
	if err != nil {
		t.Fatalf("bypass create: %v", err)
	}
	t.Cleanup(func() { _ = b.Delete(h.Ctx, bdesired, bmeta) })

	h.Hold()
	actual, err := d.Retrieve(h.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != len(cases) {
		t.Fatalf("Retrieve = %d objects, want %d: %+v", len(actual), len(cases), actual)
	}
	for _, c := range cases {
		found := false
		for _, kv := range actual {
			if kv.Key == d.KeyOf(c) {
				found = true
				if !proto.Equal(kv.Value, c) {
					t.Errorf("Retrieve %v != desired %v", kv.Value, c)
				}
			}
		}
		if !found {
			t.Errorf("%s not retrieved", d.KeyOf(c))
		}
	}
	t.Logf("retrieved %d gtpu tunnels: %v", len(actual), actual)
	factual, err := fw.Retrieve(h.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(factual) != 1 || !proto.Equal(factual[0].Value, forward) {
		t.Fatalf("forward Retrieve = %+v, want %v", factual, forward)
	}
	t.Logf("retrieved forward: %v", factual)
	// tteid in place on the host.
	upd := proto.Clone(cases[0]).(*gtpu.Tunnel)
	upd.Tteid = teid + 101
	if _, err := d.Update(h.Ctx, cases[0], upd, actual[0].Meta); err != nil {
		t.Fatalf("tteid update: %v", err)
	}
	if again, _ := d.Retrieve(h.Ctx); len(again) != 2 || !(proto.Equal(again[0].Value, upd) || proto.Equal(again[1].Value, upd)) {
		t.Fatalf("after tteid update Retrieve = %+v", again)
	}
	if err := b.Delete(h.Ctx, bdesired, bmeta); err != nil {
		t.Fatalf("bypass delete: %v", err)
	}
	if err := fw.Delete(h.Ctx, forward, fmeta); err != nil {
		t.Fatalf("delete forward: %v", err)
	}
	for _, kv := range actual {
		if err := d.Delete(h.Ctx, kv.Value, kv.Meta); err != nil {
			t.Fatalf("delete %s: %v", kv.Key, err)
		}
	}
	if after, _ := d.Retrieve(h.Ctx); len(after) != 0 {
		t.Fatalf("after delete Retrieve = %+v", after)
	}
	if after, _ := fw.Retrieve(h.Ctx); len(after) != 0 {
		t.Fatalf("after delete forward Retrieve = %+v", after)
	}
}
