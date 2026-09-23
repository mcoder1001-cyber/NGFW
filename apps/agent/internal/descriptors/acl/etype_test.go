package acl

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	vppacl "ngfw/agent/binapi/acl"
	"ngfw/agent/internal/scheduler"
)

func TestEtypeWhitelistLifecycle(t *testing.T) {
	ctx := t.Context()
	v := newFakeVPP()
	v.setEtypes(ifLoop300, 1, 0x0806)  // another owner's interface
	v.setEtypes(ifHostEth0, 1, 0x0806) // untagged interface: not ours
	d := NewEtypeWhitelist(v, owner)

	desired := EtypeWhitelist{Interface: "loop1040", Input: []uint16{0x0806, 0x88cc}, Output: []uint16{0x0806}}
	if d.KeyOf(desired.Proto()) != "acl.etype-whitelist/loop1040" {
		t.Fatalf("key %q", d.KeyOf(desired.Proto()))
	}
	if deps := d.Dependencies(desired.Proto()); len(deps) != 1 || deps[0] != (scheduler.Dependency{Key: "interface/loop1040", Optional: true}) {
		t.Fatalf("deps %+v", deps)
	}
	meta, err := d.Create(ctx, desired.Proto())
	if err != nil {
		t.Fatal(err)
	}
	req := v.CallsNamed("acl_interface_set_etype_whitelist")[0].(*vppacl.ACLInterfaceSetEtypeWhitelist)
	if uint32(req.SwIfIndex) != ifLoop1040 || req.NInput != 2 || len(req.Whitelist) != 3 || req.Whitelist[2] != 0x0806 {
		t.Fatalf("request %+v", req)
	}
	actual := mustRetrieve(t, d)
	if len(actual) != 1 || !proto.Equal(actual[0].Value, desired.Proto()) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v (foreign/untagged interfaces must be filtered)", actual)
	}
	assertEmptyPlan(t, d, kv(d, desired.Proto()))

	updated := EtypeWhitelist{Interface: "loop1040", Input: []uint16{0x88cc}, Output: []uint16{}}
	if m2, err := d.Update(ctx, desired.Proto(), updated.Proto(), meta); err != nil || m2 != meta {
		t.Fatalf("Update: %v", err)
	}
	assertEmptyPlan(t, d, kv(d, updated.Proto()))
	if _, err := d.Update(ctx, updated.Proto(), EtypeWhitelist{Interface: "loop1041", Input: []uint16{1}}.Proto(), meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("interface change: %v", err)
	}
	if _, err := d.Create(ctx, EtypeWhitelist{Interface: "loop1041", Input: []uint16{0x88cc, 0x0806}}.Proto()); !errors.Is(err, ErrSpec) {
		t.Fatalf("unsorted: %v", err)
	}
	if _, err := d.Create(ctx, EtypeWhitelist{Interface: "nope", Input: []uint16{1}}.Proto()); !errors.Is(err, ErrNoInterface) {
		t.Fatalf("unknown interface: %v", err)
	}
	if err := d.Delete(ctx, updated.Proto(), meta); err != nil {
		t.Fatal(err)
	}
	if actual := mustRetrieve(t, d); len(actual) != 0 {
		t.Fatalf("after delete Retrieve = %+v", actual)
	}
}
