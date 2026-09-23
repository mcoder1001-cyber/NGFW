package sr_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/sr"
	"ngfw/agent/internal/scheduler"
)

// checkRoundTrip creates cases, compares Retrieve (filtered to the slot scope) with them,
// deletes them and checks that Retrieve is empty; cleanup deletes again (no-op when gone).
func checkRoundTrip[T proto.Message](t *testing.T, h *df6test.Host, d scheduler.Descriptor, cases []T, want func(T) T) {
	t.Helper()
	for _, c := range cases {
		if _, err := d.Create(h.Ctx, c); err != nil {
			t.Fatalf("create %v: %v", c, err)
		}
		t.Cleanup(func() { _ = d.Delete(h.Ctx, c, nil) })
	}
	h.Hold()
	h.AssertEmptyPlan(d, df6test.Msgs(cases)...)
	actual, err := d.Retrieve(h.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != len(cases) {
		t.Fatalf("Retrieve = %d objects, want %d: %+v", len(actual), len(cases), actual)
	}
	byKey := map[scheduler.Key]proto.Message{}
	for _, kv := range actual {
		byKey[kv.Key] = kv.Value
	}
	for _, c := range cases {
		w := c
		if want != nil {
			w = want(c)
		}
		if got := byKey[d.KeyOf(c)]; !proto.Equal(got, w) {
			t.Errorf("Retrieve %s = %v, want %v", d.KeyOf(c), got, w)
		}
	}
	t.Logf("retrieved %d %s: %v", len(actual), d.Name(), actual)
	for _, kv := range actual {
		if err := d.Delete(h.Ctx, kv.Value, kv.Meta); err != nil {
			t.Fatalf("delete %s: %v", kv.Key, err)
		}
	}
	if after, _ := d.Retrieve(h.Ctx); len(after) != 0 {
		t.Fatalf("after delete Retrieve = %+v", after)
	}
}

func TestLocalSidOnHost(t *testing.T) {
	h := df6test.Connect(t)
	loop, _ := h.Loopback(8, h.IP6(8, 1)+"/64")
	v4 := h.Table(8)
	v6 := h.Table(9)
	h.IPTable(v4, false)
	h.IPTable(v6, true)
	d := sr.NewLocalSid(h.Client, h.Owner)
	cases := []*sr.LocalSid{
		{Sid: h.IP6(0x51, 1), Behavior: sr.Behavior_END, EndPsp: true},
		{Sid: h.IP6(0x51, 2), Behavior: sr.Behavior_END_X, Interface: loop, NextHop: h.IP6(8, 2), FibTable: v6},
		{Sid: h.IP6(0x51, 3), Behavior: sr.Behavior_END_DX6, Interface: loop, NextHop: h.IP6(8, 3)},
		{Sid: h.IP6(0x51, 4), Behavior: sr.Behavior_END_DT4, LookupTable: v4},
		{Sid: h.IP6(0x51, 5), Behavior: sr.Behavior_END_DT6, LookupTable: v6},
	}
	checkRoundTrip(t, h, d, cases, nil)
}

func TestPolicyOnHost(t *testing.T) {
	h := df6test.Connect(t)
	v6 := h.Table(10)
	h.IPTable(v6, true)
	p := sr.NewPolicy(h.Client, h.Owner)
	cases := []*sr.Policy{
		{Bsid: h.IP6(0xb, 1), Encap: true, EncapSrc: h.IP6(0xa, 1), SidLists: []*sr.SidList{
			{Sids: []string{h.IP6(0x51, 1), h.IP6(0x52, 1)}, Weight: 1},
			{Sids: []string{h.IP6(0x53, 1)}, Weight: 5},
		}},
		{Bsid: h.IP6(0xb, 2), Type: sr.PolicyType_SPRAY, FibTable: v6, SidLists: []*sr.SidList{{Sids: []string{h.IP6(0x54, 1)}, Weight: 1}}},
	}
	checkRoundTrip(t, h, p, cases, nil)
}

func TestSteeringOnHost(t *testing.T) {
	h := df6test.Connect(t)
	v4 := h.Table(11)
	h.IPTable(v4, false)
	p := sr.NewPolicy(h.Client, h.Owner)
	pol := &sr.Policy{Bsid: h.IP6(0xb, 3), Encap: true, EncapSrc: h.IP6(0xa, 1), SidLists: []*sr.SidList{{Sids: []string{h.IP6(0x55, 1)}, Weight: 1}}}
	if _, err := p.Create(h.Ctx, pol); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Delete(h.Ctx, pol, nil) })
	s := sr.NewSteering(h.Client, h.Owner)
	cases := []*sr.Steering{
		{TrafficType: sr.SteerType_IPV4, Prefix: h.IP4(12, 0) + "/24", TableId: v4, Bsid: pol.GetBsid()},
		{TrafficType: sr.SteerType_IPV6, Prefix: canon(t, h.IP6(0x12, 0)+"/64"), Bsid: pol.GetBsid()},
	}
	checkRoundTrip(t, h, s, cases, nil)
	if err := p.Delete(h.Ctx, pol, nil); err != nil {
		t.Fatal(err)
	}
}

func canon(t *testing.T, p string) string {
	t.Helper()
	c, err := df6.CanonicalPrefix(p)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
