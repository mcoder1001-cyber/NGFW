package abf

import (
	"context"
	"testing"

	"ngfw/agent/binapi/acl"
	aclpkg "ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/descriptors/df2"
)

// M5 / D-066: with two ACLs tagged w3:dup (indices 2 and 1, dump order), the policy binds
// the lowest index — the one DF-4's acl.LookupIndex calls acl.acl/dup — and a policy found on
// the other index is reported against "dup#2", so it diffs and is recreated.
func TestDuplicateACLTagsFollowDF4(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	v.Reply("acl_dump",
		&acl.ACLDetails{ACLIndex: 2, Tag: "w3:dup"},
		&acl.ACLDetails{ACLIndex: 1, Tag: "w3:dup"},
	)
	idx, err := aclpkg.LookupIndex(ctx, v, "w3", "dup")
	if err != nil || idx != 1 {
		t.Fatalf("DF-4 LookupIndex = %d, %v", idx, err)
	}
	d := NewPolicy(v, "w3", nil)
	p := &Policy{PolicyId: 3001, Acl: "dup", Paths: []*df2.FibPath{{Type: df2.FibPath_DROP}}}
	meta, err := d.Create(ctx, p)
	if err != nil || meta != (PolicyMeta{ACLIndex: 1}) {
		t.Fatalf("Create = %+v, %v; want acl_index 1", meta, err)
	}
	// A policy that ended up on the non-canonical ACL (e.g. a lost reply) is not "dup".
	v.policies[3002] = v.policies[3001]
	pol := v.policies[3002]
	pol.PolicyID, pol.ACLIndex = 3002, 2
	v.policies[3002] = pol
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := map[uint32]string{}
	for _, kv := range actual {
		names[kv.Value.(*Policy).GetPolicyId()] = kv.Value.(*Policy).GetAcl()
	}
	if names[3001] != "dup" || names[3002] != "dup#2" {
		t.Fatalf("Retrieve ACL names = %v, want 3001→dup, 3002→dup#2", names)
	}
	if deps := d.Dependencies(p); deps[0].Key != aclpkg.KeyACL("dup") {
		t.Fatalf("Dependencies = %+v, want DF-4 key %s", deps, aclpkg.KeyACL("dup"))
	}
}
