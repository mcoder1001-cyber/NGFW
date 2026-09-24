package iface_test

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

func TestSubinterfaceFlagsRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		in    *iface.Subinterface
		flags interface_types.SubIfFlags
		tags  uint8
	}{
		{"dot1q exact", &iface.Subinterface{Parent: loopKey, SubId: 100, OuterVlan: 100, ExactMatch: true},
			interface_types.SUB_IF_API_FLAG_ONE_TAG | interface_types.SUB_IF_API_FLAG_EXACT_MATCH, 1},
		{"dot1ad qinq", &iface.Subinterface{Parent: loopKey, SubId: 200, OuterVlan: 200, InnerVlan: 300, Dot1Ad: true, ExactMatch: true},
			interface_types.SUB_IF_API_FLAG_TWO_TAGS | interface_types.SUB_IF_API_FLAG_DOT1AD | interface_types.SUB_IF_API_FLAG_EXACT_MATCH, 2},
		{"outer any", &iface.Subinterface{Parent: loopKey, SubId: 3, OuterVlanAny: true},
			interface_types.SUB_IF_API_FLAG_ONE_TAG | interface_types.SUB_IF_API_FLAG_OUTER_VLAN_ID_ANY, 1},
		{"inner any", &iface.Subinterface{Parent: loopKey, SubId: 4, OuterVlan: 10, InnerVlanAny: true},
			interface_types.SUB_IF_API_FLAG_TWO_TAGS | interface_types.SUB_IF_API_FLAG_INNER_VLAN_ID_ANY, 2},
		{"untagged", &iface.Subinterface{Parent: loopKey, SubId: 5, Untagged: true}, interface_types.SUB_IF_API_FLAG_NO_TAGS, 0},
		{"default", &iface.Subinterface{Parent: loopKey, SubId: 6, DefaultSubif: true}, interface_types.SUB_IF_API_FLAG_DEFAULT, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if f := iface.SubifFlags(c.in); f != c.flags {
				t.Fatalf("SubifFlags = %v, want %v", f, c.flags)
			}
			row := &ifapi.SwInterfaceDetails{SubID: c.in.SubId, SubIfFlags: c.flags, SubNumberOfTags: c.tags,
				SubOuterVlanID: uint16(c.in.OuterVlan), SubInnerVlanID: uint16(c.in.InnerVlan)} //nolint:gosec // test values
			if got := iface.DecodeSubif(loopKey, row); !proto.Equal(got, c.in) {
				t.Fatalf("DecodeSubif = %v, want %v", got, c.in)
			}
		})
	}
}

func TestSubinterface(t *testing.T) {
	w := newWorld()
	d := iface.NewSubinterface(w.v, owner)
	desired := &iface.Subinterface{Parent: loopKey, SubId: 100, OuterVlan: 100, ExactMatch: true}
	if k := d.KeyOf(desired); k != "interface.subinterface/loop201.100" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 1 || deps[0].Key != loopKey {
		t.Fatalf("Dependencies = %+v", deps)
	}
	meta := mustCreate(t, d, desired)
	req := w.v.CallsNamed("create_subif")[0].(*ifapi.CreateSubif)
	if uint32(req.SwIfIndex) != w.loop || req.SubID != 100 || req.OuterVlanID != 100 || req.SubIfFlags != interface_types.SUB_IF_API_FLAG_ONE_TAG|interface_types.SUB_IF_API_FLAG_EXACT_MATCH {
		t.Fatalf("create_subif = %+v", req)
	}
	idx := meta.(iface.Meta).SwIfIndex
	if row, _ := w.v.Get(idx); row.Tag != "w2:loop201.100" {
		t.Fatalf("tag = %q", row.Tag)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"interface.subinterface/loop201.100": desired}, map[scheduler.Key]any{"interface.subinterface/loop201.100": meta})
	// attributes see the sub-interface as an interface in its own right
	subMtu := &iface.Mtu{Interface: "interface.subinterface/loop201.100", Mtu: 1496}
	mustCreate(t, iface.NewMtu(w.v, owner), subMtu)
	found := false
	for _, kv := range retrieve(t, iface.NewMtu(w.v, owner)) {
		found = found || (kv.Key == "interface.mtu/loop201.100" && proto.Equal(kv.Value, iface.Normalize(iface.NewMtu(w.v, owner), subMtu)))
	}
	if !found {
		t.Fatal("the sub-interface must be decorated by the attribute descriptors")
	}
	if _, err := d.Update(ctx, desired, &iface.Subinterface{Parent: loopKey, SubId: 100, OuterVlan: 101}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if _, err := d.Create(ctx, &iface.Subinterface{Parent: loopKey, SubId: 7, OuterVlan: 5000}); err == nil {
		t.Fatal("vlan 5000 accepted")
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if kvs := retrieve(t, d); len(kvs) != 0 {
		t.Fatalf("after Delete: %+v", kvs)
	}
	if _, ok := w.v.Get(idx); ok {
		t.Fatal("sub-interface still exists")
	}
}

// TestSubinterfaceTagFailure (review M3): when tagging fails after create_subif, the untagged
// orphan is deleted, so the scheduler's retry succeeds instead of failing with "already exists".
func TestSubinterfaceTagFailure(t *testing.T) {
	w := newWorld()
	d := iface.NewSubinterface(w.v, owner)
	sub := &iface.Subinterface{Parent: loopKey, SubId: 7, OuterVlan: 7}
	before := len(w.v.Ifs)
	w.v.FailTag = 1
	if _, err := d.Create(ctx, sub); err == nil {
		t.Fatal("tag failure not reported")
	}
	if len(w.v.Ifs) != before || len(w.v.CallsNamed("delete_subif")) != 1 {
		t.Fatalf("orphan left behind: %d interfaces (want %d), delete_subif calls %d", len(w.v.Ifs), before, len(w.v.CallsNamed("delete_subif")))
	}
	if _, err := d.Create(ctx, sub); err != nil {
		t.Fatalf("retry after the orphan was removed: %v", err)
	}
}
