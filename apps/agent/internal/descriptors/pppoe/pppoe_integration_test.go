package pppoe_test

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/pppoe"
)

func TestSessionOnHost(t *testing.T) {
	h := df6test.Connect(t)
	loop, _ := h.Loopback(7, h.IP4(7, 1)+"/24")
	vrf := h.Table(7)
	h.IPTable(vrf, false)

	d := pppoe.NewSession(h.Client, h.Owner)
	mac := fmt.Sprintf("02:%02x:00:00:00:%02x", h.Slot, 1)
	cases := []*pppoe.Session{
		{SessionId: h.Table(500), ClientIp: h.IP4(7, 2), ClientMac: mac},
		{SessionId: h.Table(501), ClientIp: h.IP4(7, 3), ClientMac: mac, DecapVrfId: vrf},
	}
	for _, c := range cases {
		meta, err := d.Create(h.Ctx, c)
		if err != nil {
			t.Fatalf("create %v: %v", c, err)
		}
		t.Cleanup(func() { _ = d.Delete(h.Ctx, c, meta) })
	}
	cp := pppoe.NewCp(h.Client, h.Owner)
	cpd := &pppoe.Cp{Interface: loop}
	cpmeta, err := cp.Create(h.Ctx, cpd)
	if err != nil {
		t.Fatalf("cp create: %v", err)
	}
	t.Cleanup(func() { _ = cp.Delete(h.Ctx, cpd, cpmeta) })

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
	t.Logf("retrieved %d pppoe sessions: %v", len(actual), actual)
	if err := cp.Delete(h.Ctx, cpd, cpmeta); err != nil {
		t.Fatalf("cp delete: %v", err)
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
