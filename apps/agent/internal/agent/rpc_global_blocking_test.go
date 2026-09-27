package agent

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/acl_types"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
)

// F-global-blocking on the fake VPP: the block lists become bucket ACLs put before the user's ACLs,
// a direction without a user ACL gets the pass ACL, Retrieve == desired, one changed entry updates the
// bucket ACLs only (no rebinding), and removing the block lists restores the user's bindings.

const gbJSON = `{
  "interfaces": {"loop711": {"ipv4": ["10.71.1.1/24"]}, "loop712": {"ipv4": ["10.71.2.1/24"]}},
  "acl": {
    "lists": {"user-in": {"tags": [], "rules": [{"sequence": 10, "action": "permit", "enabled": true, "ipVersion": "any", "log": false}]}},
    "attachments": [{"list": "user-in", "target": {"kind": "interface", "interface": "loop711"}, "direction": "in", "sequence": 10, "enabled": true}],
    "globalBlocking": {"lists": {"bad": {
      "enabled": true, "source": {"kind": "upload"}, "allInterfaces": false, "interfaces": ["loop711", "loop712"],
      "direction": "both", "protectHost": false, "log": false,
      "entries": ["192.0.2.7/32", "198.51.100.0/24", "2001:db8::/48"]
    }}}
  }
}`

func aclNames(t *testing.T, v *coretest.VPP, idx []uint32) string {
	t.Helper()
	all := v.ACL().ACLs()
	out := make([]string, len(idx))
	for i, x := range idx {
		out[i] = strings.TrimPrefix(all[x], testOwner+":")
	}
	return strings.Join(out, ",")
}

func TestGlobalBlockingOnFake(t *testing.T) {
	v := coretest.New()
	s, _ := newACLSvc(t, v, t.TempDir(), false)
	want := doc(t, gbJSON)
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "g1", DesiredState: want}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)

	n1, b1 := v.ACL().Binding(loopIndex(t, v, "loop711"))
	n2, b2 := v.ACL().Binding(loopIndex(t, v, "loop712"))
	if n1 != 2 || aclNames(t, v, b1) != "_gb.bad.i00,user-in,_gb.bad.o00,_gb.pass" {
		t.Fatalf("loop711: %d %s", n1, aclNames(t, v, b1))
	}
	if n2 != 2 || aclNames(t, v, b2) != "_gb.bad.i00,_gb.pass,_gb.bad.o00,_gb.pass" {
		t.Fatalf("loop712: %d %s", n2, aclNames(t, v, b2))
	}
	rules := v.ACL().Rules(ownedACL(t, v, "_gb.bad.i00"))
	if len(rules) != 3 || rules[0].IsPermit != acl_types.ACL_ACTION_API_DENY || rules[0].SrcPrefix.Len != 32 || rules[0].DstPrefix.Len != 0 {
		t.Fatalf("inbound bucket: %+v", rules)
	}

	if got := retrieveACL(t, s); !proto.Equal(got, want.GetAcl()) {
		t.Fatalf("Retrieve != desired:\n%s\nwant\n%s", protojson.Format(got), protojson.Format(want.GetAcl()))
	}
	if sm := apply(t, s, &vrxv1.ApplyRequest{TxnId: "g2", DesiredState: want}).GetSummary(); sm.GetCreated()+sm.GetUpdated()+sm.GetDeleted() != 0 {
		t.Fatalf("second apply not empty: %v", sm)
	}

	// one more entry: the two bucket ACLs and the applied configuration change, nothing is rebound
	next := proto.Clone(want).(*vrxv1.DesiredState)
	l := next.GetAcl().GetGlobalBlocking().GetLists()["bad"]
	l.Entries = append(l.Entries, "203.0.113.5/32")
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "g3", DesiredState: next})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if sm := resp.GetSummary(); sm.GetCreated()+sm.GetDeleted() != 0 || sm.GetUpdated() != 3 {
		t.Fatalf("entry change: %v", sm)
	}
	for _, r := range resp.GetResults() {
		if strings.HasPrefix(r.GetKey(), "acl.interface-binding/") {
			t.Fatalf("rebound %s", r.GetKey())
		}
	}

	// AclState lists the bucket ACLs (per-list hit counters come from their sums)
	st, err := s.ACLState(context.Background(), &vrxv1.AclStateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, x := range st.GetLists() {
		names = append(names, x.GetName())
	}
	if strings.Join(names, ",") != "_gb.bad.i00,_gb.bad.o00,_gb.pass,user-in" {
		t.Fatalf("AclState lists: %v", names)
	}

	// block lists removed: their ACLs are deleted and the user's binding is back as it was
	plain := proto.Clone(next).(*vrxv1.DesiredState)
	plain.GetAcl().GlobalBlocking = nil
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "g4", DesiredState: plain}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	n1, b1 = v.ACL().Binding(loopIndex(t, v, "loop711"))
	_, b2 = v.ACL().Binding(loopIndex(t, v, "loop712"))
	if n1 != 1 || aclNames(t, v, b1) != "user-in" || len(b2) != 0 || len(v.ACL().ACLs()) != 1 {
		t.Fatalf("after removal: %s / %v / %v", aclNames(t, v, b1), b2, v.ACL().ACLs())
	}
	if got := retrieveACL(t, s); !proto.Equal(got, plain.GetAcl()) {
		t.Fatalf("Retrieve after removal: %s", protojson.Format(got))
	}
}
