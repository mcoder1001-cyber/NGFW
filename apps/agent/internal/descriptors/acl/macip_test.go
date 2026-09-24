package acl

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	vppacl "ngfw/agent/binapi/acl"
	"ngfw/agent/binapi/acl_types"
	"ngfw/agent/internal/scheduler"
)

func macipRules() []MacipRule {
	return []MacipRule{
		{Action: ActionPermit, SrcMac: "02:00:00:00:00:01", SrcMacMask: "ff:ff:ff:ff:ff:ff", SrcPrefix: "10.10.1.0/24"},
		{Action: ActionDeny, SrcMac: "00:00:00:00:00:00", SrcMacMask: "00:00:00:00:00:00", SrcPrefix: AnyV4},
	}
}

func TestMacipACLDescriptor(t *testing.T) {
	ctx := t.Context()
	v := newFakeVPP()
	foreign := v.addMacipACL("w3:m", acl_types.MacipACLRule{IsPermit: acl_types.ACL_ACTION_API_PERMIT})
	d := NewMacipACL(v, owner)
	desired := MacipACL{Name: "l2-guard", Rules: macipRules()}
	if d.KeyOf(desired.Proto()) != "acl.macip-acl/l2-guard" || d.KeyOf(desired.Proto()) != KeyMacipACL("l2-guard") {
		t.Fatalf("key %q", d.KeyOf(desired.Proto()))
	}
	meta, err := d.Create(ctx, desired.Proto())
	if err != nil {
		t.Fatal(err)
	}
	req := v.CallsNamed("macip_acl_add_replace")[0].(*vppacl.MacipACLAddReplace)
	if req.ACLIndex != noACL || req.Tag != "w10:l2-guard" || len(req.R) != 2 {
		t.Fatalf("request %+v", req)
	}
	actual := mustRetrieve(t, d)
	if len(actual) != 1 || !proto.Equal(actual[0].Value, desired.Proto()) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v", actual)
	}
	assertEmptyPlan(t, d, kv(d, desired.Proto()))
	updated := MacipACL{Name: "l2-guard", Rules: macipRules()[1:]}
	if m2, err := d.Update(ctx, desired.Proto(), updated.Proto(), meta); err != nil || m2 != meta {
		t.Fatalf("Update: %v", err)
	}
	if req := v.CallsNamed("macip_acl_add_replace")[1].(*vppacl.MacipACLAddReplace); req.ACLIndex != meta.(MacipMeta).ACLIndex {
		t.Fatalf("update must replace on the same index: %+v", req)
	}
	assertEmptyPlan(t, d, kv(d, updated.Proto()))
	if _, err := d.Update(ctx, updated.Proto(), MacipACL{Name: "x"}.Proto(), meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("rename: %v", err)
	}
	if err := d.Delete(ctx, updated.Proto(), meta); err != nil {
		t.Fatal(err)
	}
	if actual := mustRetrieve(t, d); len(actual) != 0 {
		t.Fatalf("after delete: %+v", actual)
	}
	if _, ok := v.macips[foreign]; !ok {
		t.Fatal("foreign macip acl touched")
	}
}

func TestMacipBindingLifecycle(t *testing.T) {
	ctx := t.Context()
	v := newFakeVPP()
	acls := NewMacipACL(v, owner)
	m1, _ := acls.Create(ctx, MacipACL{Name: "m1", Rules: macipRules()}.Proto())
	m2, _ := acls.Create(ctx, MacipACL{Name: "m2", Rules: macipRules()[:1]}.Proto())
	foreign := v.addMacipACL("w3:f")
	v.macipBind[ifLoop300] = foreign

	d := NewMacipBinding(v, owner)
	desired := MacipBinding{Interface: "loop1040", ACL: "m1"}
	if d.KeyOf(desired.Proto()) != "acl.macip-interface-binding/loop1040" {
		t.Fatalf("key %q", d.KeyOf(desired.Proto()))
	}
	deps := d.Dependencies(desired.Proto())
	if len(deps) != 2 || deps[0] != (scheduler.Dependency{Key: "interface/loop1040", Optional: true}) || deps[1] != (scheduler.Dependency{Key: KeyMacipACL("m1")}) {
		t.Fatalf("deps %+v", deps)
	}
	meta, err := d.Create(ctx, desired.Proto())
	if err != nil {
		t.Fatal(err)
	}
	if meta != (MacipBindingMeta{SwIfIndex: ifLoop1040, ACLIndex: m1.(MacipMeta).ACLIndex}) {
		t.Fatalf("meta %+v", meta)
	}
	actual := mustRetrieve(t, d)
	if len(actual) != 1 || !proto.Equal(actual[0].Value, desired.Proto()) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v (foreign binding must be filtered)", actual)
	}
	assertEmptyPlan(t, d, kv(d, desired.Proto()))
	// switch to m2 in place: one add, VPP replaces
	updated := MacipBinding{Interface: "loop1040", ACL: "m2"}
	newMeta, err := d.Update(ctx, desired.Proto(), updated.Proto(), meta)
	if err != nil || newMeta != (MacipBindingMeta{SwIfIndex: ifLoop1040, ACLIndex: m2.(MacipMeta).ACLIndex}) {
		t.Fatalf("Update: %v %+v", err, newMeta)
	}
	calls := v.CallsNamed("macip_acl_interface_add_del")
	if r := calls[len(calls)-1].(*vppacl.MacipACLInterfaceAddDel); !r.IsAdd || r.ACLIndex != m2.(MacipMeta).ACLIndex {
		t.Fatalf("update request %+v", r)
	}
	assertEmptyPlan(t, d, kv(d, updated.Proto()))
	if _, err := d.Update(ctx, updated.Proto(), MacipBinding{Interface: "loop1041", ACL: "m2"}.Proto(), newMeta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("interface change: %v", err)
	}
	if err := d.Delete(ctx, updated.Proto(), newMeta); err != nil {
		t.Fatal(err)
	}
	calls = v.CallsNamed("macip_acl_interface_add_del")
	if r := calls[len(calls)-1].(*vppacl.MacipACLInterfaceAddDel); r.IsAdd || uint32(r.SwIfIndex) != ifLoop1040 {
		t.Fatalf("delete request %+v", r)
	}
	if actual := mustRetrieve(t, d); len(actual) != 0 {
		t.Fatalf("after delete: %+v", actual)
	}
	if err := acls.Delete(ctx, MacipACL{Name: "m2"}.Proto(), m2); err != nil {
		t.Fatalf("m2 deletable after unbind: %v", err)
	}
	// VPP semantics (verified on the host): deleting a bound MACIP ACL unapplies it itself, so a
	// binding disappears with its ACL; the mandatory dependency still orders delete binding → ACL.
	m3, _ := acls.Create(ctx, MacipACL{Name: "m3", Rules: macipRules()}.Proto())
	if _, err := d.Create(ctx, MacipBinding{Interface: "loop1040", ACL: "m3"}.Proto()); err != nil {
		t.Fatal(err)
	}
	if err := acls.Delete(ctx, MacipACL{Name: "m3"}.Proto(), m3); err != nil {
		t.Fatalf("macip_acl_del while bound succeeds on VPP (auto-unbind): %v", err)
	}
	if actual := mustRetrieve(t, d); len(actual) != 0 {
		t.Fatalf("binding must vanish with its MACIP ACL: %+v", actual)
	}
	if _, err := d.Create(ctx, MacipBinding{Interface: "loop1040", ACL: "nope"}.Proto()); !errors.Is(err, ErrNoMacipACL) {
		t.Fatalf("unknown macip acl: %v", err)
	}
	if _, err := d.Create(ctx, MacipBinding{Interface: "nope", ACL: "m1"}.Proto()); !errors.Is(err, ErrNoInterface) {
		t.Fatalf("unknown interface: %v", err)
	}
}
