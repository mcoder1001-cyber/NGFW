package acl

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	vppacl "ngfw/agent/binapi/acl"
	"ngfw/agent/internal/scheduler"
)

// Review finding 2: a whitelist on an untagged interface (a physical port) is visible to
// Retrieve once this descriptor applied it, so re-apply plans nothing and Delete converges.
func TestEtypeWhitelistUntaggedInterface(t *testing.T) {
	ctx := t.Context()
	v := newFakeVPP()
	claims := NewMemoryClaimStore()
	d := NewEtypeWhitelist(v, owner, WithEtypeClaims(claims))

	desired := EtypeWhitelist{Interface: "host-eth0", Input: []uint16{0x0806}, Output: []uint16{0x0806, 0x88cc}}
	meta, err := d.Create(ctx, desired.Proto())
	if err != nil {
		t.Fatal(err)
	}
	actual := mustRetrieve(t, d)
	if len(actual) != 1 || actual[0].Key != KeyEtypeWhitelist("host-eth0") || !proto.Equal(actual[0].Value, desired.Proto()) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v, want the whitelist on the untagged interface", actual)
	}
	assertEmptyPlan(t, d, kv(d, desired.Proto()))

	// a restarted agent sharing the (persisted) claim store still sees it
	again := NewEtypeWhitelist(v, owner, WithEtypeClaims(claims))
	assertEmptyPlan(t, again, kv(again, desired.Proto()))
	// another owner never sees it
	if kvs := mustRetrieve(t, NewEtypeWhitelist(v, "w3")); len(kvs) != 0 {
		t.Fatalf("owner w3 must not see our whitelist: %+v", kvs)
	}

	// removed from desired → the scheduler plans a Delete, which clears VPP and the claim
	p := diffPlan(nil, mustRetrieve(t, d))
	if len(p.Delete) != 1 {
		t.Fatalf("plan: %s", planString(p))
	}
	if err := d.Delete(ctx, p.Delete[0].Value, p.Delete[0].Meta); err != nil {
		t.Fatal(err)
	}
	last := v.CallsNamed("acl_interface_set_etype_whitelist")
	if r := last[len(last)-1].(*vppacl.ACLInterfaceSetEtypeWhitelist); uint32(r.SwIfIndex) != ifHostEth0 || len(r.Whitelist) != 0 {
		t.Fatalf("delete request %+v", r)
	}
	if claims.Claimed("host-eth0") {
		t.Fatal("claim must be released on Delete")
	}
	if kvs := mustRetrieve(t, d); len(kvs) != 0 {
		t.Fatalf("after Delete: %+v", kvs)
	}

	// an interface tagged by another owner is refused, nothing is sent
	n := len(v.CallsNamed("acl_interface_set_etype_whitelist"))
	if _, err := d.Create(ctx, EtypeWhitelist{Interface: "loop300", Input: []uint16{0x0806}}.Proto()); !errors.Is(err, ErrForeignInterface) {
		t.Fatalf("foreign interface: %v", err)
	}
	if len(v.CallsNamed("acl_interface_set_etype_whitelist")) != n {
		t.Fatal("no request may be sent for a foreign interface")
	}
	// a failed set releases the claim again
	v.Reply("acl_interface_set_etype_whitelist", &vppacl.ACLInterfaceSetEtypeWhitelistReply{Retval: rvInvalidValue})
	if _, err := d.Create(ctx, desired.Proto()); err == nil {
		t.Fatal("retval must surface")
	}
	if claims.Claimed("host-eth0") {
		t.Fatal("a failed Create must not leave a claim")
	}
}

// Review finding 4: two VPP ACLs with the same owner tag. The lowest index is the object; the
// other is reported under "<name>#<index>", which is never desired, so the scheduler deletes it.
func TestDuplicateTagACL(t *testing.T) {
	ctx := t.Context()
	v := newFakeVPP()
	first := v.addACL("w10:x")
	second := v.addACL("w10:x")
	d := NewACL(v, owner)
	desired := ACL{Name: "x"}

	actual := mustRetrieve(t, d)
	if len(actual) != 2 || actual[0].Key != KeyACL("x") || actual[0].Meta != (Meta{ACLIndex: first}) ||
		actual[1].Key != "acl.acl/x#1" || actual[1].Meta != (Meta{ACLIndex: second}) {
		t.Fatalf("Retrieve = %+v", actual)
	}
	if d.KeyOf(actual[1].Value) != actual[1].Key {
		t.Fatalf("KeyOf(value) %q != key %q", d.KeyOf(actual[1].Value), actual[1].Key)
	}
	if idx, err := LookupIndex(ctx, v, owner, "x"); err != nil || idx != first {
		t.Fatalf("LookupIndex = %d, %v; want the lowest index %d", idx, err, first)
	}
	// a binding that references the extra shows it under its dup name → planned as an update
	v.bind(ifLoop1040, 1, second)
	bd := NewInterfaceBinding(v, owner)
	bkv := mustRetrieve(t, bd)
	want := InterfaceBinding{Interface: "loop1040", Input: []string{"x#1"}, Output: []string{}}
	if len(bkv) != 1 || !proto.Equal(bkv[0].Value, want.Proto()) {
		t.Fatalf("binding Retrieve = %+v", bkv)
	}
	desiredBinding := InterfaceBinding{Interface: "loop1040", Input: []string{"x"}}
	p := diffPlan([]scheduler.KV{kv(d, desired.Proto()), kv(bd, desiredBinding.Proto())}, append(actual, bkv...))
	if len(p.Update) != 1 || len(p.Delete) != 1 || p.Delete[0].Key != "acl.acl/x#1" || len(p.Create) != 0 {
		t.Fatalf("plan:\n%s", planString(p))
	}
	// apply in scheduler order: update the binding (dependents first), then delete the extra
	if _, err := bd.Update(ctx, bkv[0].Value, desiredBinding.Proto(), bkv[0].Meta); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, p.Delete[0].Value, p.Delete[0].Meta); err != nil {
		t.Fatal(err)
	}
	assertEmptyPlan(t, d, kv(d, desired.Proto()))
	assertEmptyPlan(t, bd, kv(bd, desiredBinding.Proto()))
	if v.hasACL(second) || !v.hasACL(first) {
		t.Fatal("the extra must be gone, the object kept")
	}
}

func TestDuplicateTagMacipACL(t *testing.T) {
	ctx := t.Context()
	v := newFakeVPP()
	first := v.addMacipACL("w10:m")
	second := v.addMacipACL("w10:m")
	d := NewMacipACL(v, owner)
	actual := mustRetrieve(t, d)
	if len(actual) != 2 || actual[0].Key != KeyMacipACL("m") || actual[0].Meta != (MacipMeta{ACLIndex: first}) ||
		actual[1].Key != "acl.macip-acl/m#1" || actual[1].Meta != (MacipMeta{ACLIndex: second}) {
		t.Fatalf("Retrieve = %+v", actual)
	}
	p := diffPlan([]scheduler.KV{kv(d, MacipACL{Name: "m"}.Proto())}, actual)
	if len(p.Delete) != 1 || p.Delete[0].Key != "acl.macip-acl/m#1" {
		t.Fatalf("plan:\n%s", planString(p))
	}
	if err := d.Delete(ctx, p.Delete[0].Value, p.Delete[0].Meta); err != nil {
		t.Fatal(err)
	}
	assertEmptyPlan(t, d, kv(d, MacipACL{Name: "m"}.Proto()))
	// binding by name resolves to the lowest index
	b := NewMacipBinding(v, owner)
	meta, err := b.Create(ctx, MacipBinding{Interface: "loop1040", ACL: "m"}.Proto())
	if err != nil || meta.(MacipBindingMeta).ACLIndex != first {
		t.Fatalf("binding: %+v %v", meta, err)
	}
}

// A MACIP binding never replaces another owner's MACIP ACL (VPP holds one per interface).
func TestMacipBindingRefusesForeign(t *testing.T) {
	ctx := t.Context()
	v := newFakeVPP()
	foreign := v.addMacipACL("w3:m")
	v.mu.Lock()
	v.macipBind[ifHostEth0] = foreign
	v.mu.Unlock()
	if _, err := NewMacipACL(v, owner).Create(ctx, MacipACL{Name: "m"}.Proto()); err != nil {
		t.Fatal(err)
	}
	d := NewMacipBinding(v, owner)
	if _, err := d.Create(ctx, MacipBinding{Interface: "host-eth0", ACL: "m"}.Proto()); !errors.Is(err, ErrForeignMacipBinding) {
		t.Fatalf("foreign MACIP binding: %v", err)
	}
	if len(v.CallsNamed("macip_acl_interface_add_del")) != 0 {
		t.Fatal("nothing may be sent")
	}
	// VPP reports ~0 for an interface whose MACIP ACL was removed (seen on the host): free
	v.mu.Lock()
	v.macipBind[ifLoop1040] = noACL
	v.mu.Unlock()
	if _, err := d.Create(ctx, MacipBinding{Interface: "loop1040", ACL: "m"}.Proto()); err != nil {
		t.Fatalf("free interface: %v", err)
	}
}
