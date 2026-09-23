package gre_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/gre"
)

// TestTunnelOnHost: create → Retrieve shows it → delete → Retrieve shows nothing of ours, on
// the shared host VPP (VRX_INTEGRATION=1, shared lab lock, slot-prefixed objects only).
func TestTunnelOnHost(t *testing.T) {
	h := df6test.Connect(t)
	loop, _ := h.Loopback(1, h.IP4(1, 1)+"/24")
	_ = loop
	vrf := h.Table(1)
	h.IPTable(vrf, false)

	d := gre.NewTunnel(h.Client, h.Owner)
	cases := []*gre.Tunnel{
		{Instance: uint32(h.Slot*100 + 1), Src: h.IP4(1, 1), Dst: h.IP4(1, 2)},                                              //nolint:gosec // slot ≤ 12
		{Instance: uint32(h.Slot*100 + 2), Type: gre.TunnelType_TEB, Src: h.IP4(1, 1), Dst: h.IP4(1, 3), OuterTableId: vrf}, //nolint:gosec // slot ≤ 12
		{Instance: uint32(h.Slot*100 + 3), Type: gre.TunnelType_ERSPAN, Src: h.IP4(1, 1), Dst: h.IP4(1, 4), SessionId: 5},   //nolint:gosec // slot ≤ 12
		{Instance: uint32(h.Slot*100 + 4), Mode: gre.TunnelMode_MP, Src: h.IP4(1, 1)},                                       //nolint:gosec // slot ≤ 12
	}
	for _, c := range cases {
		meta, err := d.Create(h.Ctx, c)
		if err != nil {
			t.Fatalf("create %v: %v", c, err)
		}
		t.Cleanup(func() { _ = d.Delete(h.Ctx, c, meta) })
	}
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
	t.Logf("retrieved %d gre tunnels: %v", len(actual), actual)
	for _, kv := range actual {
		if err := d.Delete(h.Ctx, kv.Value, kv.Meta); err != nil {
			t.Fatalf("delete %s: %v", kv.Key, err)
		}
	}
	if after, _ := d.Retrieve(h.Ctx); len(after) != 0 {
		t.Fatalf("after delete Retrieve = %+v", after)
	}
}
