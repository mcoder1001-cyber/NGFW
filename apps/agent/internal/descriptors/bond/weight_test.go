package bond_test

import (
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	bondapi "ngfw/agent/binapi/bond"
	"ngfw/agent/internal/descriptors/bond"
	"ngfw/agent/internal/descriptors/dfkit"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

// withWeights adds VPP's sw_interface_set_bond_weight to the DF-1 fake: members only (INVALID_INTERFACE otherwise),
// active-backup bonds only (INVALID_ARGUMENT), the value kept in the membership and reported by
// sw_member_interface_dump (bond_add_member starts it at 0).
func withWeights(f *fakeBond) *fakeBond {
	f.On("sw_interface_set_bond_weight", func(req api.Message) ([]api.Message, error) {
		r := req.(*bondapi.SwInterfaceSetBondWeight)
		b, ok := f.ofBond[uint32(r.SwIfIndex)]
		if !ok {
			return []api.Message{&bondapi.SwInterfaceSetBondWeightReply{Retval: int32(api.INVALID_INTERFACE)}}, nil
		}
		if f.bonds[b].Mode != bondapi.BOND_API_MODE_ACTIVE_BACKUP {
			return []api.Message{&bondapi.SwInterfaceSetBondWeightReply{Retval: int32(api.INVALID_ARGUMENT)}}, nil
		}
		f.members[uint32(r.SwIfIndex)].Weight = r.Weight
		return []api.Message{&bondapi.SwInterfaceSetBondWeightReply{}}, nil
	})
	return f
}

func mustCreate(t *testing.T, d scheduler.Descriptor, v proto.Message) any {
	t.Helper()
	meta, err := d.Create(ctx, v)
	if err != nil {
		t.Fatalf("%s Create %s: %v", d.Name(), d.KeyOf(v), err)
	}
	return meta
}

func TestMemberWeight(t *testing.T) {
	f := withWeights(newFake())
	bd, md, wd := bond.NewBond(f, owner), bond.NewMember(f, owner), bond.NewWeight(f, owner)
	ab := &bond.Bond{Name: "w2-bond0", Id: 200, Mode: bond.Mode_MODE_ACTIVE_BACKUP, Lb: bond.LoadBalance_LOAD_BALANCE_ACTIVE_BACKUP}
	mustCreate(t, bd, ab)
	m1 := &bond.Member{Bond: "interface/w2-bond0", Interface: "interface/w2-tap0"}
	m2 := &bond.Member{Bond: bondKey, Interface: tap2Key}
	mustCreate(t, md, m1)
	mustCreate(t, md, m2)

	w1 := bond.Weight{Bond: "interface/w2-bond0", Interface: "interface/w2-tap0", Weight: 200}.Proto()
	if k := wd.KeyOf(w1); k != "bond.member-weight/w2-bond0/w2-tap0" {
		t.Fatalf("key %s", k)
	}
	if deps := wd.Dependencies(w1); len(deps) != 1 || deps[0].Key != "bond.member/w2-bond0/w2-tap0" {
		t.Fatalf("deps %v", deps)
	}
	// creator-key references normalise to the alias form Retrieve reports
	w2raw := bond.Weight{Bond: bondKey, Interface: tap2Key, Weight: 100}.Proto()
	w2 := wd.Normalize(w2raw)
	if want := (bond.Weight{Bond: "interface/w2-bond0", Interface: "interface/w2-tap1", Weight: 100}).Proto(); !proto.Equal(w2, want) {
		t.Fatalf("Normalize = %v", w2)
	}
	if kvs := retrieve(t, wd); len(kvs) != 0 {
		t.Fatalf("weight 0 (VPP's unset) reported: %v", kvs)
	}
	meta1 := mustCreate(t, wd, w1)
	if meta1.(bond.WeightMeta).SwIfIndex != f.tap {
		t.Fatalf("meta %v", meta1)
	}
	mustCreate(t, wd, w2raw)
	if f.members[f.tap].Weight != 200 || f.members[f.tap2].Weight != 100 {
		t.Fatalf("VPP weights %d/%d", f.members[f.tap].Weight, f.members[f.tap2].Weight)
	}
	kvs := retrieve(t, wd)
	if len(kvs) != 2 {
		t.Fatalf("Retrieve %v", kvs)
	}
	for _, kv := range kvs {
		want := w1
		if kv.Key == "bond.member-weight/w2-bond0/w2-tap1" {
			want = w2
		}
		if !proto.Equal(kv.Value, want) {
			t.Fatalf("%s = %v, want %v", kv.Key, kv.Value, want)
		}
	}
	// in-place update
	w1b := bond.Weight{Bond: "interface/w2-bond0", Interface: "interface/w2-tap0", Weight: 50}.Proto()
	if _, err := wd.Update(ctx, w1, w1b, meta1); err != nil || f.members[f.tap].Weight != 50 {
		t.Fatalf("update: %v, weight %d", err, f.members[f.tap].Weight)
	}
	// Delete resets to 0; after the membership is gone Delete is a no-op
	if err := wd.Delete(ctx, w1b, meta1); err != nil || f.members[f.tap].Weight != 0 {
		t.Fatalf("delete: %v, weight %d", err, f.members[f.tap].Weight)
	}
	if err := md.Delete(ctx, m2, bond.MemberMeta{SwIfIndex: f.tap2, Bond: f.ofBond[f.tap2]}); err != nil {
		t.Fatal(err)
	}
	n := len(f.CallsNamed("sw_interface_set_bond_weight"))
	if err := wd.Delete(ctx, w2, bond.WeightMeta{SwIfIndex: f.tap2}); err != nil {
		t.Fatalf("delete of a detached member's weight: %v", err)
	}
	if len(f.CallsNamed("sw_interface_set_bond_weight")) != n {
		t.Fatal("a detached member's weight was written")
	}
	// Create on a non-member / weight 0 / wrong mode fails
	if _, err := wd.Create(ctx, w2); err == nil {
		t.Fatal("weight on a non-member accepted")
	}
	if _, err := wd.Create(ctx, bond.Weight{Bond: "interface/w2-bond0", Interface: "interface/w2-tap0"}.Proto()); !errors.Is(err, dfkit.ErrSpec) {
		t.Fatalf("weight 0: %v", err)
	}
	xor := &bond.Bond{Name: "w2-bond1", Id: 201, Mode: bond.Mode_MODE_XOR, Lb: bond.LoadBalance_LOAD_BALANCE_L2}
	mustCreate(t, bd, xor)
	mustCreate(t, md, &bond.Member{Bond: "interface/w2-bond1", Interface: "interface/w2-tap1"})
	if _, err := wd.Create(ctx, bond.Weight{Bond: "interface/w2-bond1", Interface: "interface/w2-tap1", Weight: 3}.Proto()); !dfkit.IsVPPError(err, api.INVALID_ARGUMENT) {
		t.Fatalf("weight on a xor bond: %v", err)
	}
	// the other owner's bond is never looked at
	for _, kv := range retrieve(t, wd) {
		if iface.RefID(weightBond(t, kv)) != "w2-bond0" && iface.RefID(weightBond(t, kv)) != "w2-bond1" {
			t.Fatalf("foreign weight %v", kv)
		}
	}
}

func weightBond(t *testing.T, kv scheduler.KV) string {
	t.Helper()
	w, err := bond.WeightFromProto(kv.Value)
	if err != nil {
		t.Fatal(err)
	}
	return w.Bond
}

// TestBondProvidesAlias (D-125): bond.bond satisfies "interface/<name>", so a delete-only plan orders the bond after
// everything that depends on the alias.
func TestBondProvidesAlias(t *testing.T) {
	d := bond.NewBond(nil, owner)
	var kp scheduler.KeyProvider = d
	if got := kp.ProvidedKeys(&bond.Bond{Name: "BondEthernet6000", Id: 6000}); len(got) != 1 || got[0] != "interface/BondEthernet6000" {
		t.Fatalf("ProvidedKeys = %v", got)
	}
	if got := kp.ProvidedKeys(&bond.Bond{}); got != nil {
		t.Fatalf("ProvidedKeys of an empty bond = %v", got)
	}
}
