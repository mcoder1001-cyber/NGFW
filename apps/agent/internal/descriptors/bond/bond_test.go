package bond_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	bondapi "ngfw/agent/binapi/bond"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/bond"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/scheduler"
)

const (
	owner   = "w2"
	tapKey  = "tapv2.tap/w2-tap0"
	tap2Key = "tapv2.tap/w2-tap1"
	bondKey = "bond.bond/w2-bond0"
)

var ctx = context.Background()

type fakeBond struct {
	*ifacetest.VPP
	bonds   map[uint32]*bondapi.SwBondInterfaceDetails
	members map[uint32]*bondapi.SwMemberInterfaceDetails // member sw_if_index → details
	ofBond  map[uint32]uint32                             // member → bond
	tap, tap2, other uint32
}

func newFake() *fakeBond {
	f := &fakeBond{VPP: ifacetest.New(), bonds: map[uint32]*bondapi.SwBondInterfaceDetails{}, members: map[uint32]*bondapi.SwMemberInterfaceDetails{}, ofBond: map[uint32]uint32{}}
	f.tap = f.Add("tap0", "virtio", "w2:w2-tap0")
	f.tap2 = f.Add("tap1", "virtio", "w2:w2-tap1")
	f.other = f.Add("BondEthernet9", "bond", "w3:w3-bond0")
	f.bonds[f.other] = &bondapi.SwBondInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(f.other), ID: 9, Mode: bondapi.BOND_API_MODE_LACP, InterfaceName: "BondEthernet9"}
	f.On("bond_create2", func(req api.Message) ([]api.Message, error) {
		r := req.(*bondapi.BondCreate2)
		for _, b := range f.bonds {
			if b.ID == r.ID {
				return []api.Message{&bondapi.BondCreate2Reply{Retval: -100}}, nil
			}
		}
		idx := f.Add(fmt.Sprintf("BondEthernet%d", r.ID), "bond", "")
		f.bonds[idx] = &bondapi.SwBondInterfaceDetails{SwIfIndex: interface_types.InterfaceIndex(idx), ID: r.ID, Mode: r.Mode, Lb: r.Lb, NumaOnly: r.NumaOnly, InterfaceName: fmt.Sprintf("BondEthernet%d", r.ID)}
		return []api.Message{&bondapi.BondCreate2Reply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
	})
	f.On("bond_delete", func(req api.Message) ([]api.Message, error) {
		r := req.(*bondapi.BondDelete)
		if _, ok := f.bonds[uint32(r.SwIfIndex)]; !ok {
			return []api.Message{&bondapi.BondDeleteReply{Retval: -2}}, nil
		}
		delete(f.bonds, uint32(r.SwIfIndex))
		f.Remove(uint32(r.SwIfIndex))
		return []api.Message{&bondapi.BondDeleteReply{}}, nil
	})
	f.On("bond_add_member", func(req api.Message) ([]api.Message, error) {
		r := req.(*bondapi.BondAddMember)
		if _, ok := f.bonds[uint32(r.BondSwIfIndex)]; !ok {
			return []api.Message{&bondapi.BondAddMemberReply{Retval: -2}}, nil
		}
		name, _ := f.Get(uint32(r.SwIfIndex))
		f.members[uint32(r.SwIfIndex)] = &bondapi.SwMemberInterfaceDetails{SwIfIndex: r.SwIfIndex, InterfaceName: name.InterfaceName, IsPassive: r.IsPassive, IsLongTimeout: r.IsLongTimeout}
		f.ofBond[uint32(r.SwIfIndex)] = uint32(r.BondSwIfIndex)
		f.bonds[uint32(r.BondSwIfIndex)].Members++
		return []api.Message{&bondapi.BondAddMemberReply{}}, nil
	})
	f.On("bond_detach_member", func(req api.Message) ([]api.Message, error) {
		r := req.(*bondapi.BondDetachMember)
		b, ok := f.ofBond[uint32(r.SwIfIndex)]
		if !ok {
			return []api.Message{&bondapi.BondDetachMemberReply{Retval: -2}}, nil
		}
		f.bonds[b].Members--
		delete(f.members, uint32(r.SwIfIndex))
		delete(f.ofBond, uint32(r.SwIfIndex))
		return []api.Message{&bondapi.BondDetachMemberReply{}}, nil
	})
	f.On("sw_bond_interface_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, b := range f.bonds {
			out = append(out, b)
		}
		return out, nil
	})
	f.On("sw_member_interface_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*bondapi.SwMemberInterfaceDump)
		var out []api.Message
		for m, b := range f.ofBond {
			if b == uint32(r.SwIfIndex) {
				out = append(out, f.members[m])
			}
		}
		return out, nil
	})
	return f
}

func retrieve(t *testing.T, d scheduler.Descriptor) []scheduler.KV {
	t.Helper()
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatalf("%s Retrieve: %v", d.Name(), err)
	}
	return kvs
}

func TestBondAndMember(t *testing.T) {
	f := newFake()
	r := scheduler.NewRegistry()
	bond.Register(r, f, owner)
	if r.Len() != 2 {
		t.Fatal(r.Names())
	}
	d := bond.NewBond(f, owner)
	desired := &bond.Bond{Name: "w2-bond0", Id: 200, Mode: bond.Mode_MODE_LACP, Lb: bond.LoadBalance_LOAD_BALANCE_L34}
	if d.KeyOf(desired) != bondKey || d.Dependencies(desired) != nil {
		t.Fatal("key/deps")
	}
	if kvs := retrieve(t, d); len(kvs) != 0 {
		t.Fatalf("the other owner's bond is visible: %+v", kvs)
	}
	for _, bad := range []*bond.Bond{
		{Name: "x", Id: 1, Mode: bond.Mode_MODE_ROUND_ROBIN, Lb: bond.LoadBalance_LOAD_BALANCE_L2},
		{Name: "x", Id: 1, Mode: bond.Mode_MODE_XOR, Lb: bond.LoadBalance_LOAD_BALANCE_ROUND_ROBIN},
		{Name: "x", Id: 1},
		{Id: 1, Mode: bond.Mode_MODE_LACP},
	} {
		if _, err := d.Create(ctx, bad); err == nil {
			t.Fatalf("accepted invalid %v", bad)
		}
	}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	req := f.CallsNamed("bond_create2")[0].(*bondapi.BondCreate2)
	if req.Mode != bondapi.BOND_API_MODE_LACP || req.Lb != bondapi.BOND_API_LB_ALGO_L34 || req.ID != 200 || req.UseCustomMac || req.EnableGso {
		t.Fatalf("bond_create2 = %+v", req)
	}
	row, _ := f.Get(f.Next - 1)
	if row.Tag != "w2:w2-bond0" {
		t.Fatalf("tag = %q", row.Tag)
	}
	kvs := retrieve(t, d)
	if len(kvs) != 1 || kvs[0].Key != bondKey || !proto.Equal(kvs[0].Value, desired) || kvs[0].Meta != meta {
		t.Fatalf("Retrieve = %+v", kvs)
	}
	rr := &bond.Bond{Name: "w2-bond1", Id: 201, Mode: bond.Mode_MODE_ROUND_ROBIN, Lb: bond.LoadBalance_LOAD_BALANCE_ROUND_ROBIN}
	if _, err := d.Create(ctx, rr); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Update(ctx, desired, &bond.Bond{Name: "w2-bond0", Id: 200, Mode: bond.Mode_MODE_XOR, Lb: bond.LoadBalance_LOAD_BALANCE_L34}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("mode change: %v", err)
	}

	// members
	md := bond.NewMember(f, owner)
	m1 := &bond.Member{Bond: bondKey, Interface: tapKey}
	m2 := &bond.Member{Bond: bondKey, Interface: tap2Key, Passive: true, LongTimeout: true}
	if md.KeyOf(m1) != "bond.member/w2-bond0/w2-tap0" {
		t.Fatal(md.KeyOf(m1))
	}
	if deps := md.Dependencies(m1); len(deps) != 2 || deps[0].Key != bondKey || deps[1].Key != tapKey {
		t.Fatalf("deps = %+v", deps)
	}
	mm1, err := md.Create(ctx, m1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := md.Create(ctx, m2); err != nil {
		t.Fatal(err)
	}
	add := f.CallsNamed("bond_add_member")[1].(*bondapi.BondAddMember)
	if uint32(add.SwIfIndex) != f.tap2 || !add.IsPassive || !add.IsLongTimeout {
		t.Fatalf("bond_add_member = %+v", add)
	}
	mk := retrieve(t, md)
	if len(mk) != 2 {
		t.Fatalf("members = %+v", mk)
	}
	for _, kv := range mk {
		want := m1
		if kv.Key == "bond.member/w2-bond0/w2-tap1" {
			want = m2
		}
		if !proto.Equal(kv.Value, want) {
			t.Fatalf("member %s = %v", kv.Key, kv.Value)
		}
	}
	if _, err := md.Update(ctx, m1, &bond.Member{Bond: bondKey, Interface: tapKey, Passive: true}, mm1); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("member update: %v", err)
	}
	if err := md.Delete(ctx, m1, mm1); err != nil {
		t.Fatal(err)
	}
	if mk = retrieve(t, md); len(mk) != 1 || mk[0].Key != "bond.member/w2-bond0/w2-tap1" {
		t.Fatalf("after Delete members = %+v", mk)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if kvs = retrieve(t, d); len(kvs) != 1 || kvs[0].Key != "bond.bond/w2-bond1" {
		t.Fatalf("after Delete = %+v", kvs)
	}
	if _, ok := f.bonds[f.other]; !ok {
		t.Fatal("the other owner's bond was touched")
	}
}
