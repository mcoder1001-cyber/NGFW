package acl

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	vppacl "ngfw/agent/binapi/acl"
	"ngfw/agent/binapi/acl_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

const owner = "w10"

func TestACLDescriptor(t *testing.T) {
	ctx := t.Context()
	v := newFakeVPP()
	// another owner's and an untagged ACL live on the same VPP; they must stay invisible
	foreign := v.addACL("w3:their-acl", acl_types.ACLRule{IsPermit: acl_types.ACL_ACTION_API_PERMIT})
	untagged := v.addACL("", acl_types.ACLRule{IsPermit: acl_types.ACL_ACTION_API_DENY})
	d := NewACL(v, owner)

	if d.Name() != NameACL || !scheduler.ValidName(d.Name()) {
		t.Fatalf("name %q", d.Name())
	}
	desired := ACL{Name: "lan-in", Rules: sampleRules()}
	if got := d.KeyOf(desired.Proto()); got != "acl.acl/lan-in" || got != KeyACL("lan-in") {
		t.Fatalf("key %q", got)
	}
	if deps := d.Dependencies(desired.Proto()); deps != nil {
		t.Fatalf("deps %+v", deps)
	}

	// Create: acl_add_replace with ~0, owner tag, rules in order
	meta, err := d.Create(ctx, desired.Proto())
	if err != nil {
		t.Fatal(err)
	}
	calls := v.CallsNamed("acl_add_replace")
	if len(calls) != 1 {
		t.Fatalf("%d acl_add_replace calls", len(calls))
	}
	req := calls[0].(*vppacl.ACLAddReplace)
	if req.ACLIndex != noACL || req.Tag != "w10:lan-in" || len(req.R) != len(desired.Rules) || req.R[0].DstportOrIcmpcodeFirst != 80 {
		t.Fatalf("create request %+v", req)
	}
	m := meta.(Meta)
	if m.ACLIndex != 2 {
		t.Fatalf("meta %+v", m)
	}

	// Retrieve == desired, foreign filtered, meta as Create
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) != 1 || actual[0].Key != KeyACL("lan-in") || !proto.Equal(actual[0].Value, desired.Proto()) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v", actual)
	}
	assertEmptyPlan(t, d, kv(d, desired.Proto()))

	// Update in place keeps the index; rule order change is an update
	updated := ACL{Name: "lan-in", Rules: []Rule{desired.Rules[1], desired.Rules[0], {Action: ActionDeny, Src: AnyV4, Dst: AnyV4}}}
	newMeta, err := d.Update(ctx, desired.Proto(), updated.Proto(), meta)
	if err != nil || newMeta != meta {
		t.Fatalf("Update: %v, meta %+v", err, newMeta)
	}
	req = v.CallsNamed("acl_add_replace")[1].(*vppacl.ACLAddReplace)
	if req.ACLIndex != m.ACLIndex || len(req.R) != 3 || req.R[0].IsPermit != acl_types.ACL_ACTION_API_PERMIT_REFLECT {
		t.Fatalf("update request %+v", req)
	}
	actual, _ = d.Retrieve(ctx)
	if len(actual) != 1 || !proto.Equal(actual[0].Value, updated.Proto()) {
		t.Fatalf("after update Retrieve = %+v", actual)
	}
	assertEmptyPlan(t, d, kv(d, updated.Proto()))

	// rename → recreate
	if _, err := d.Update(ctx, updated.Proto(), ACL{Name: "other", Rules: updated.Rules}.Proto(), meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("rename: %v", err)
	}

	// 50-rule ACL: byte-identical retrieve and empty plan at size
	big := ACL{Name: "big", Rules: manyRules(50)}
	bigMeta, err := d.Create(ctx, big.Proto())
	if err != nil {
		t.Fatal(err)
	}
	assertEmptyPlan(t, d, kv(d, updated.Proto()), kv(d, big.Proto()))

	// Delete → Retrieve shows nothing of ours; others untouched
	if err := d.Delete(ctx, updated.Proto(), meta); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, big.Proto(), bigMeta); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 0 {
		t.Fatalf("after delete Retrieve = %+v", actual)
	}
	if !v.hasACL(foreign) || !v.hasACL(untagged) || v.aclCount() != 2 {
		t.Fatal("another owner's ACL was touched")
	}
	// leftover of ours (not in desired) is reported by Retrieve so the scheduler can delete it
	v.addACL("w10:leftover")
	if p := diffPlan(nil, mustRetrieve(t, d)); len(p.Delete) != 1 || p.Delete[0].Key != KeyACL("leftover") {
		t.Fatalf("leftover plan %+v", p)
	}
}

func mustRetrieve(t *testing.T, d scheduler.Descriptor) []scheduler.KV {
	t.Helper()
	kvs, err := d.Retrieve(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return kvs
}

func TestACLDescriptorErrors(t *testing.T) {
	ctx := t.Context()
	v := newFakeVPP()
	d := NewACL(v, owner)

	if _, err := d.Create(ctx, ACL{Name: "bad", Rules: []Rule{{Action: ActionPermit, Src: "10.0.0.1/24", Dst: AnyV4}}}.Proto()); !errors.Is(err, ErrSpec) {
		t.Fatalf("non-canonical prefix: %v", err)
	}
	if len(v.CallsNamed("acl_add_replace")) != 0 {
		t.Fatal("invalid spec must not reach VPP")
	}
	// VPP retval surfaces (replace of a non-existent index)
	if _, err := d.Update(ctx, ACL{Name: "x"}.Proto(), ACL{Name: "x"}.Proto(), Meta{ACLIndex: 99}); err == nil {
		t.Fatal("retval must surface as error")
	}
	if _, err := d.Update(ctx, ACL{Name: "x"}.Proto(), ACL{Name: "x"}.Proto(), "wrong"); err == nil {
		t.Fatal("wrong meta type must be an error")
	}
	if err := d.Delete(ctx, ACL{Name: "x"}.Proto(), Meta{ACLIndex: 99}); err == nil {
		t.Fatal("delete of unknown index must fail")
	}
	// in use: the fake refuses acl_del while bound (ACL_IN_USE), like VPP
	meta, err := d.Create(ctx, ACL{Name: "bound"}.Proto())
	if err != nil {
		t.Fatal(err)
	}
	v.bind(ifLoop1040, 1, meta.(Meta).ACLIndex)
	if err := d.Delete(ctx, ACL{Name: "bound"}.Proto(), meta); err == nil {
		t.Fatal("delete while bound must fail (scheduler unbinds first through the dependency)")
	}
	v.SetConnected(false)
	if _, err := d.Create(ctx, ACL{Name: "x"}.Proto()); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected: %v", err)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected retrieve: %v", err)
	}
	v.SetConnected(true)
	// tag too long
	if _, err := d.Create(ctx, ACL{Name: strings.Repeat("x", 70)}.Proto()); !errors.Is(err, vpp.ErrTagTooLong) {
		t.Fatalf("long name: %v", err)
	}
}

func TestLookupIndex(t *testing.T) {
	v := newFakeVPP()
	v.addACL("w3:x")
	idx := v.addACL("w10:x")
	got, err := LookupIndex(t.Context(), v, owner, "x")
	if err != nil || got != idx {
		t.Fatalf("LookupIndex = %d, %v (want %d)", got, err, idx)
	}
	if _, err := LookupIndex(t.Context(), v, owner, "nope"); !errors.Is(err, ErrNoACL) {
		t.Fatalf("missing: %v", err)
	}
}
