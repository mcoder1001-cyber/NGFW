package sr_mpls_test

import (
	"errors"
	"testing"

	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/sr_mpls"
)

// TestPolicySteeringOnHost: create → present (BSID FIB entry, steering route) → delete →
// absent. Retrieve is write-only for SR-MPLS (no dump), so presence is checked through the
// same FIB probes the descriptors use.
func TestPolicySteeringOnHost(t *testing.T) {
	h := df6test.Connect(t)
	h.MPLSTable(0) // DF-7's mpls-table fixture (reference counted)
	v4 := h.Table(12)
	h.IPTable(v4, false)
	p := sr_mpls.NewPolicy(h.Client)
	s := sr_mpls.NewSteering(h.Client)
	bsid := h.Table(600)
	pol := &sr_mpls.Policy{Bsid: bsid, SegmentLists: []*sr_mpls.SegmentList{
		{Labels: []uint32{h.Table(700), h.Table(701)}, Weight: 1},
		{Labels: []uint32{h.Table(702)}, Weight: 2},
	}}
	st := &sr_mpls.Steering{Prefix: h.IP4(13, 0) + "/24", TableId: v4, Bsid: bsid, VpnLabel: h.Table(800)}
	if _, err := p.Create(h.Ctx, pol); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Delete(h.Ctx, pol, nil) })
	if _, err := s.Create(h.Ctx, st); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Delete(h.Ctx, st, nil) })
	h.Hold()
	if ok, err := sr_mpls.BSIDPresent(h.Ctx, h.Client, bsid); err != nil || !ok {
		t.Fatalf("BSID %d not in MPLS table 0: %v", bsid, err)
	}
	if _, err := p.Retrieve(h.Ctx); !errors.Is(err, df6.ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve = %v", err)
	}
	if ok, err := s.Present(h.Ctx, st); err != nil || !ok {
		t.Fatalf("steering route not present: %v", err)
	}
	if err := s.Delete(h.Ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Present(h.Ctx, st); err != nil || ok {
		t.Fatalf("steering route still present after delete: %v", err)
	}
	if err := p.Delete(h.Ctx, pol, nil); err != nil {
		t.Fatal(err)
	}
	if ok, err := sr_mpls.BSIDPresent(h.Ctx, h.Client, bsid); err != nil || ok {
		t.Fatalf("BSID %d still present after delete: %v", bsid, err)
	}
	t.Logf("sr-mpls policy %d + steering %s/%d created, observed, deleted", bsid, st.GetPrefix(), v4)
}

// TestEndpointColorOnHost is skipped: the assignment allocates internal labels and creates
// VPP's global SR-MPLS TE (next-hop, color) MPLS table, and it cannot be removed except by
// deleting the policy; covered by the unit test on the fake.
func TestEndpointColorOnHost(t *testing.T) {
	t.Skip("sr-mpls.endpoint-color is write-only with global side effects (TE table, internal labels) and no un-assign message; unit-tested on the fake only")
}
