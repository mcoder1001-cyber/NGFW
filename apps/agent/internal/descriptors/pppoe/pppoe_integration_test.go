package pppoe_test

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/pppoe"
)

// TestCpOnHost: pppoe_add_del_cp on a prefixed loopback (write-only: no dump exists).
func TestCpOnHost(t *testing.T) {
	h := df6test.Connect(t)
	loop, _ := h.Loopback(7, h.IP4(7, 1)+"/24")
	cp := pppoe.NewCp(h.Client, h.Owner)
	cpd := &pppoe.Cp{Interface: loop}
	cpmeta, err := cp.Create(h.Ctx, cpd)
	if err != nil {
		t.Fatalf("cp create: %v", err)
	}
	h.Hold()
	if err := cp.Delete(h.Ctx, cpd, cpmeta); err != nil {
		t.Fatalf("cp delete: %v", err)
	}
}

// TestSessionOnHost: create → Retrieve → delete. VPP only creates a session for a client MAC
// learned from PPPoE discovery packets; the host has no PPPoE clients and DF-6 sends no
// packets, so on the host this verifies the typed ErrClientNotLearned path and that nothing
// is left, then skips the create/retrieve part with that reason.
func TestSessionOnHost(t *testing.T) {
	h := df6test.Connect(t)
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
		if errors.Is(err, pppoe.ErrClientNotLearned) {
			if after, rerr := d.Retrieve(h.Ctx); rerr != nil || len(after) != 0 {
				t.Fatalf("after failed create Retrieve = %+v, %v", after, rerr)
			}
			t.Skipf("pppoe.session create needs a client MAC learned from PPPoE discovery traffic (none on the host, no packet tests in DF-6): %v", err)
		}
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
	for _, kv := range actual {
		if err := d.Delete(h.Ctx, kv.Value, kv.Meta); err != nil {
			t.Fatalf("delete %s: %v", kv.Key, err)
		}
	}
	if after, _ := d.Retrieve(h.Ctx); len(after) != 0 {
		t.Fatalf("after delete Retrieve = %+v", after)
	}
}
