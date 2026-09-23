package ipip_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/ipip"
)

func TestTunnelOnHost(t *testing.T) {
	h := df6test.Connect(t)
	h.Loopback(2, h.IP4(2, 1)+"/24")
	vrf := h.Table(2)
	h.IPTable(vrf, false)

	d := ipip.NewTunnel(h.Client, h.Owner)
	s := ipip.NewSixrd(h.Client, h.Owner)
	cases := []*ipip.Tunnel{
		{Instance: uint32(h.Slot*100 + 1), Src: h.IP4(2, 1), Dst: h.IP4(2, 2), Dscp: 46},               //nolint:gosec // slot ≤ 12
		{Instance: uint32(h.Slot*100 + 2), Src: h.IP4(2, 1), Dst: h.IP4(2, 3), TableId: vrf, Flags: 4}, //nolint:gosec // slot ≤ 12
		{Instance: uint32(h.Slot*100 + 3), Mode: ipip.TunnelMode_MP, Src: h.IP4(2, 1)},                 //nolint:gosec // slot ≤ 12
	}
	for _, c := range cases {
		meta, err := d.Create(h.Ctx, c)
		if err != nil {
			t.Fatalf("create %v: %v", c, err)
		}
		t.Cleanup(func() { _ = d.Delete(h.Ctx, c, meta) })
	}
	// One 6rd tunnel next to them: write-only, but it must not be claimed by ipip.tunnel.
	sixrd := &ipip.Tunnel6Rd{Name: h.Name("6rd"), Ip6Prefix: "2001:db8:" + h.Owner[1:] + "::/48", Ip4Prefix: h.IP4(0, 0) + "/16", Ip4Src: h.IP4(2, 1)}
	smeta, err := s.Create(h.Ctx, sixrd)
	if err != nil {
		t.Fatalf("create 6rd: %v", err)
	}
	t.Cleanup(func() { _ = s.Delete(h.Ctx, sixrd, smeta) })

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
	t.Logf("retrieved %d ipip tunnels: %v", len(actual), actual)
	for _, kv := range actual {
		if err := d.Delete(h.Ctx, kv.Value, kv.Meta); err != nil {
			t.Fatalf("delete %s: %v", kv.Key, err)
		}
	}
	if err := s.Delete(h.Ctx, sixrd, smeta); err != nil {
		t.Fatalf("delete 6rd: %v", err)
	}
	if after, _ := d.Retrieve(h.Ctx); len(after) != 0 {
		t.Fatalf("after delete Retrieve = %+v", after)
	}
}
