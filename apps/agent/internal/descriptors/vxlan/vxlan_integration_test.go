package vxlan_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/vxlan"
)

func TestTunnelOnHost(t *testing.T) {
	h := df6test.Connect(t)
	loop, _ := h.Loopback(3, h.IP4(3, 1)+"/24")
	vrf := h.Table(3)
	h.IPTable(vrf, false)

	d := vxlan.NewTunnel(h.Client, h.Owner)
	base := uint32(h.Slot * 100) //nolint:gosec // slot ≤ 12
	vni := h.Table(100)
	cases := []*vxlan.Tunnel{
		{Instance: base + 1, Src: h.IP4(3, 1), Dst: h.IP4(3, 2), Vni: vni},
		{Instance: base + 2, Src: h.IP4(3, 1), Dst: h.IP4(3, 3), Vni: vni + 1, EncapVrfId: vrf, SrcPort: 14789, DstPort: 14789, IsL3: true},
		{Instance: base + 3, Src: h.IP4(3, 1), Dst: "239.11.11.11", McastInterface: loop, Vni: vni + 2},
	}
	for _, c := range cases {
		meta, err := d.Create(h.Ctx, c)
		if err != nil {
			t.Fatalf("create %v: %v", c, err)
		}
		t.Cleanup(func() { _ = d.Delete(h.Ctx, c, meta) })
	}
	// Bypass on our loopback (write-only): enable → disable must both succeed.
	b := vxlan.NewBypass(h.Client, h.Owner)
	bdesired := &vxlan.Bypass{Interface: loop, Ipv4: true, Ipv6: true}
	bmeta, err := b.Create(h.Ctx, bdesired)
	if err != nil {
		t.Fatalf("bypass create: %v", err)
	}
	t.Cleanup(func() { _ = b.Delete(h.Ctx, bdesired, bmeta) })

	h.Hold()
	h.AssertEmptyPlan(d, df6test.Msgs(cases)...)
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
	t.Logf("retrieved %d vxlan tunnels: %v", len(actual), actual)
	if err := b.Delete(h.Ctx, bdesired, bmeta); err != nil {
		t.Fatalf("bypass delete: %v", err)
	}
	for _, kv := range actual {
		if err := d.Delete(h.Ctx, kv.Value, kv.Meta); err != nil {
			t.Fatalf("delete %s: %v", kv.Key, err)
		}
	}
	if after, _ := d.Retrieve(h.Ctx); len(after) != 0 {
		t.Fatalf("after delete Retrieve = %+v", after)
	}
}
