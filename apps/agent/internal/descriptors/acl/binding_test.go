package acl

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	vppacl "ngfw/agent/binapi/acl"
	"ngfw/agent/internal/scheduler"
)

func TestInterfaceBindingDependencies(t *testing.T) {
	d := NewInterfaceBinding(newFakeVPP(), owner)
	b := InterfaceBinding{Interface: "loop1040", Input: []string{"a", "b"}, Output: []string{"a", "c"}}
	deps := d.Dependencies(b.Proto())
	want := []scheduler.Dependency{
		{Key: "interface/loop1040"}, // mandatory (review 3.4)
		{Key: KeyACL("a")}, {Key: KeyACL("b")}, {Key: KeyACL("c")},
	}
	if len(deps) != len(want) {
		t.Fatalf("deps %+v", deps)
	}
	for i := range want {
		if deps[i] != want[i] {
			t.Fatalf("dep %d = %+v, want %+v", i, deps[i], want[i])
		}
	}
	custom := NewInterfaceBinding(newFakeVPP(), owner, WithInterfaceKey(func(n string) scheduler.Key { return scheduler.Join("interface.loopback", n) }))
	if deps := custom.Dependencies(b.Proto()); deps[0].Key != "interface.loopback/loop1040" || deps[0].Optional {
		t.Fatalf("custom interface key: %+v", deps[0])
	}
	if d.KeyOf(b.Proto()) != "acl.interface-binding/loop1040" {
		t.Fatalf("key %q", d.KeyOf(b.Proto()))
	}
}

func TestInterfaceBindingLifecycle(t *testing.T) {
	ctx := t.Context()
	v := newFakeVPP()
	acls := NewACL(v, owner)
	a, _ := acls.Create(ctx, ACL{Name: "a", Rules: sampleRules()[:1]}.Proto())
	b, _ := acls.Create(ctx, ACL{Name: "b", Rules: sampleRules()[4:5]}.Proto())
	aIdx, bIdx := a.(Meta).ACLIndex, b.(Meta).ACLIndex
	// a foreign binding exists too
	foreign := v.addACL("w3:f")
	v.bind(ifLoop300, 1, foreign)

	d := NewInterfaceBinding(v, owner)
	desired := InterfaceBinding{Interface: "loop1040", Input: []string{"a", "b"}, Output: []string{"a"}}
	meta, err := d.Create(ctx, desired.Proto())
	if err != nil {
		t.Fatal(err)
	}
	if meta != (BindingMeta{SwIfIndex: ifLoop1040}) {
		t.Fatalf("meta %+v", meta)
	}
	req := v.CallsNamed("acl_interface_set_acl_list")[0].(*vppacl.ACLInterfaceSetACLList)
	if uint32(req.SwIfIndex) != ifLoop1040 || req.NInput != 2 || len(req.Acls) != 3 || req.Acls[0] != aIdx || req.Acls[1] != bIdx || req.Acls[2] != aIdx {
		t.Fatalf("set list request %+v", req)
	}

	actual := mustRetrieve(t, d)
	if len(actual) != 1 || actual[0].Key != KeyInterfaceBinding("loop1040") || !proto.Equal(actual[0].Value, desired.Proto()) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v (foreign bindings must be filtered)", actual)
	}
	assertEmptyPlan(t, d, kv(d, desired.Proto()))

	// reorder = update in place (same message), not recreate
	reordered := InterfaceBinding{Interface: "loop1040", Input: []string{"b", "a"}, Output: []string{"a"}}
	if p := diffPlan([]scheduler.KV{kv(d, reordered.Proto())}, actual); len(p.Update) != 1 || len(p.Create) != 0 {
		t.Fatalf("reorder must plan an update: %s", planString(p))
	}
	if m2, err := d.Update(ctx, desired.Proto(), reordered.Proto(), meta); err != nil || m2 != meta {
		t.Fatalf("Update: %v %+v", err, m2)
	}
	req = v.CallsNamed("acl_interface_set_acl_list")[1].(*vppacl.ACLInterfaceSetACLList)
	if req.Acls[0] != bIdx || req.Acls[1] != aIdx || req.NInput != 2 {
		t.Fatalf("reorder request %+v", req)
	}
	assertEmptyPlan(t, d, kv(d, reordered.Proto()))

	// output-only binding is fine (n_input = 0)
	outOnly := InterfaceBinding{Interface: "loop1041", Input: []string{}, Output: []string{"b"}}
	if _, err := d.Create(ctx, outOnly.Proto()); err != nil {
		t.Fatal(err)
	}
	assertEmptyPlan(t, d, kv(d, reordered.Proto()), kv(d, outOnly.Proto()))

	// interface change → recreate
	if _, err := d.Update(ctx, reordered.Proto(), InterfaceBinding{Interface: "loop1041", Input: []string{"a"}}.Proto(), meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("interface change: %v", err)
	}

	// delete = empty list; then the ACL can be deleted
	if err := d.Delete(ctx, reordered.Proto(), meta); err != nil {
		t.Fatal(err)
	}
	last := v.CallsNamed("acl_interface_set_acl_list")
	if r := last[len(last)-1].(*vppacl.ACLInterfaceSetACLList); len(r.Acls) != 0 || r.NInput != 0 || uint32(r.SwIfIndex) != ifLoop1040 {
		t.Fatalf("delete request %+v", r)
	}
	if actual := mustRetrieve(t, d); len(actual) != 1 || actual[0].Key != KeyInterfaceBinding("loop1041") {
		t.Fatalf("after delete Retrieve = %+v", actual)
	}
	// "b" is still bound on loop1041: VPP refuses acl_del (in use); "a" is unbound everywhere
	if err := acls.Delete(ctx, ACL{Name: "b"}.Proto(), b); err == nil {
		t.Fatal("acl b is still bound on loop1041; acl_del must fail")
	}
	if err := acls.Delete(ctx, ACL{Name: "a"}.Proto(), a); err != nil {
		t.Fatalf("acl a should be deletable once unbound everywhere: %v", err)
	}
}

// Review finding 6: ACLs of another owner on the same interface are preserved (kept first in
// their direction), never reported as ours, and survive our Update and Delete.
func TestInterfaceBindingPreservesForeignACLs(t *testing.T) {
	ctx := t.Context()
	v := newFakeVPP()
	acls := NewACL(v, owner)
	a, _ := acls.Create(ctx, ACL{Name: "a", Rules: sampleRules()[:1]}.Proto())
	b, _ := acls.Create(ctx, ACL{Name: "b", Rules: sampleRules()[4:5]}.Proto())
	aIdx, bIdx := a.(Meta).ACLIndex, b.(Meta).ACLIndex
	fIn, fOut, untagged := v.addACL("w3:f-in"), v.addACL("w3:f-out"), v.addACL("")
	v.bind(ifHostEth0, 2, fIn, untagged, fOut)

	d := NewInterfaceBinding(v, owner)
	if kvs := mustRetrieve(t, d); len(kvs) != 0 {
		t.Fatalf("an interface with only foreign ACLs has no binding of ours: %+v", kvs)
	}
	desired := InterfaceBinding{Interface: "host-eth0", Input: []string{"a"}, Output: []string{"b"}}
	meta, err := d.Create(ctx, desired.Proto())
	if err != nil {
		t.Fatal(err)
	}
	req := v.CallsNamed("acl_interface_set_acl_list")[0].(*vppacl.ACLInterfaceSetACLList)
	if want := []uint32{fIn, untagged, aIdx, fOut, bIdx}; req.NInput != 3 || !equalU32(req.Acls, want) {
		t.Fatalf("create request n_input=%d acls=%v, want 3 %v", req.NInput, req.Acls, want)
	}
	actual := mustRetrieve(t, d)
	if len(actual) != 1 || !proto.Equal(actual[0].Value, desired.Proto()) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v, want only our entries", actual)
	}
	assertEmptyPlan(t, d, kv(d, desired.Proto()))

	updated := InterfaceBinding{Interface: "host-eth0", Input: []string{"b", "a"}}
	if _, err := d.Update(ctx, desired.Proto(), updated.Proto(), meta); err != nil {
		t.Fatal(err)
	}
	req = v.CallsNamed("acl_interface_set_acl_list")[1].(*vppacl.ACLInterfaceSetACLList)
	if want := []uint32{fIn, untagged, bIdx, aIdx, fOut}; req.NInput != 4 || !equalU32(req.Acls, want) {
		t.Fatalf("update request n_input=%d acls=%v, want 4 %v", req.NInput, req.Acls, want)
	}
	assertEmptyPlan(t, d, kv(d, updated.Proto()))

	if err := d.Delete(ctx, updated.Proto(), meta); err != nil {
		t.Fatal(err)
	}
	req = v.CallsNamed("acl_interface_set_acl_list")[2].(*vppacl.ACLInterfaceSetACLList)
	if want := []uint32{fIn, untagged, fOut}; req.NInput != 2 || !equalU32(req.Acls, want) {
		t.Fatalf("delete request n_input=%d acls=%v, want 2 %v", req.NInput, req.Acls, want)
	}
	if kvs := mustRetrieve(t, d); len(kvs) != 0 {
		t.Fatalf("after Delete: %+v", kvs)
	}
}

func equalU32(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestInterfaceBindingErrors(t *testing.T) {
	ctx := t.Context()
	v := newFakeVPP()
	d := NewInterfaceBinding(v, owner)
	if _, err := d.Create(ctx, InterfaceBinding{Interface: "nope", Input: []string{"a"}}.Proto()); !errors.Is(err, ErrNoInterface) {
		t.Fatalf("unknown interface: %v", err)
	}
	if _, err := d.Create(ctx, InterfaceBinding{Interface: "loop1040", Input: []string{"a"}}.Proto()); !errors.Is(err, ErrNoACL) {
		t.Fatalf("unknown acl: %v", err)
	}
	v.addACL("w3:a") // another owner's ACL with the same name is not ours to bind
	if _, err := d.Create(ctx, InterfaceBinding{Interface: "loop1040", Input: []string{"a"}}.Proto()); !errors.Is(err, ErrNoACL) {
		t.Fatalf("foreign acl by name: %v", err)
	}
	if _, err := d.Create(ctx, InterfaceBinding{Interface: "loop1040", Input: []string{"a", "a"}}.Proto()); !errors.Is(err, ErrSpec) {
		t.Fatalf("duplicate: %v", err)
	}
	// review finding 3: an empty binding is never reported by Retrieve, so it is rejected
	if _, err := d.Create(ctx, InterfaceBinding{Interface: "loop1040", Input: []string{}, Output: []string{}}.Proto()); !errors.Is(err, ErrSpec) {
		t.Fatalf("empty binding: %v", err)
	}
	if len(v.CallsNamed("acl_interface_set_acl_list")) != 0 {
		t.Fatal("no set_acl_list must be sent on validation errors")
	}
	if err := d.Delete(ctx, InterfaceBinding{Interface: "loop1040"}.Proto(), Meta{}); err == nil {
		t.Fatal("wrong meta type")
	}
}
