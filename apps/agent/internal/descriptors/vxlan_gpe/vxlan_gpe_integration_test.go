package vxlan_gpe_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/vxlan_gpe"
)

func TestTunnelOnHost(t *testing.T) {
	h := df6test.Connect(t)
	loop, _ := h.Loopback(5, h.IP4(5, 1)+"/24")
	vrf := h.Table(5)
	h.IPTable(vrf, false)

	d := vxlan_gpe.NewTunnel(h.Client, h.Owner)
	vni := h.Table(200)
	cases := []*vxlan_gpe.Tunnel{
		{Name: h.Name("gpe1"), Local: h.IP4(5, 1), Remote: h.IP4(5, 2), Vni: vni, Protocol: vxlan_gpe.Protocol_IP4, EncapVrfId: vrf, DecapVrfId: vrf},
		{Name: h.Name("gpe2"), Local: h.IP4(5, 1), Remote: h.IP4(5, 3), Vni: vni + 1, Protocol: vxlan_gpe.Protocol_ETHERNET, EncapVrfId: vrf, LocalPort: 14790, RemotePort: 14790},
	}
	for _, c := range cases {
		meta, err := d.Create(h.Ctx, c)
		if err != nil {
			t.Fatalf("create %v: %v", c, err)
		}
		t.Cleanup(func() { _ = d.Delete(h.Ctx, c, meta) })
	}
	b := vxlan_gpe.NewBypass(h.Client, h.Owner)
	bdesired := &vxlan_gpe.Bypass{Interface: loop, Ipv4: true}
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
	t.Logf("retrieved %d vxlan-gpe tunnels: %v", len(actual), actual)
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
