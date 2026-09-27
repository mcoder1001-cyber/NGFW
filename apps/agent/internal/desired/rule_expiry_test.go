package desired

// F-rule-expiry: expired ACL rules and NAT static mappings are left out of the projection with a rule.expired note;
// a future expiry is rendered and noted for the re-projection at that instant.

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	aclstate "ngfw/agent/internal/actions/acl"
	descacl "ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/subsystems/ruleexpiry"
)

func TestACLRuleExpiry(t *testing.T) {
	ruleexpiry.Reset()
	t.Cleanup(ruleexpiry.Reset)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	rec := aclstate.NewRecord(0)
	withEnv(t, ACLEnv{Owner: "w3", Now: func() time.Time { return now }, Record: rec})
	ds := &vrxv1.DesiredState{Acl: &vrxv1.AclConfig{Lists: map[string]*vrxv1.AclList{"l": {Rules: []*vrxv1.AclRule{
		rule(10, "permit", func(r *vrxv1.AclRule) {
			r.Source = prefix("10.3.1.0/24")
			r.ExpiresAt = proto.String("2026-09-27T11:00:00Z") // expired an hour ago
			r.Owner, r.Ticket = proto.String("netops"), proto.String("CHG-1")
		}),
		rule(20, "deny", func(r *vrxv1.AclRule) {
			r.Source = prefix("10.3.2.0/24")
			r.ExpiresAt = proto.String("2026-09-27T13:00:00Z") // in an hour
		}),
		rule(30, "permit", func(r *vrxv1.AclRule) { r.Source = prefix("10.3.3.0/24") }),
	}}}}}
	s := newRecSink()
	ACL(s, ds, map[string]bool{"acl": true})
	if len(s.issues) != 1 || !strings.HasPrefix(s.issues[0], "W /acl/lists/l/rules/0 rule.expired ") {
		t.Fatalf("issues %v", s.issues)
	}
	a, err := descacl.FromProto(s.value(t, descacl.KeyACL("l")))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range a.Rules {
		if r.Src == "10.3.1.0/24" {
			t.Fatalf("the expired rule was rendered: %+v", a.Rules)
		}
	}
	if len(a.Rules) != 2 { // rules 20 and 30: one IPv4 VPP rule each
		t.Fatalf("rendered %+v", a.Rules)
	}
	var statuses []vrxv1.AclRuleStatus
	exp, ok := rec.ACL("l", aclstate.Fingerprint(a.Rules), aclstate.ConfigHash(ds.Acl.Lists["l"]))
	if ok {
		for _, r := range exp.Rules {
			statuses = append(statuses, r.Status)
		}
	}
	if len(statuses) != 3 || statuses[0] != vrxv1.AclRuleStatus_ACL_RULE_STATUS_EXPIRED || statuses[1] != vrxv1.AclRuleStatus_ACL_RULE_STATUS_APPLIED {
		t.Fatalf("rule statuses %v", statuses)
	}
	if p := ruleexpiry.Pending(); len(p) != 1 || !p[0].Equal(now.Add(time.Hour)) {
		t.Fatalf("noted %v, want rule 20's expiry", p)
	}
	// an hour later the re-projection drops rule 20 too
	now = now.Add(time.Hour)
	s = newRecSink()
	ACL(s, ds, map[string]bool{"acl": true})
	a2, _ := descacl.FromProto(s.value(t, descacl.KeyACL("l")))
	for _, r := range a2.Rules {
		if r.Src == "10.3.2.0/24" {
			t.Fatal("rule 20 still rendered at its expiry")
		}
	}
	if len(s.issues) != 2 {
		t.Fatalf("issues %v", s.issues)
	}
	// an unparsable expiry is a projection error at its pointer
	ds.Acl.Lists["l"].Rules[2].ExpiresAt = proto.String("friday")
	s = newRecSink()
	ACL(s, ds, map[string]bool{"acl": true})
	if !strings.Contains(strings.Join(s.issues, "\n"), "E /acl/lists/l/rules/2/expiresAt rule.expires-at") {
		t.Fatalf("issues %v", s.issues)
	}
}

func TestNATStaticMappingExpiry(t *testing.T) {
	ruleexpiry.Reset()
	t.Cleanup(ruleexpiry.Reset)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	prev := ruleexpiry.Now
	ruleexpiry.Now = func() time.Time { return now }
	t.Cleanup(func() { ruleexpiry.Now = prev })
	s := newRecSink()
	expired := &vrxv1.NatStaticMapping{Name: proto.String("old"), ExpiresAt: proto.String("2026-09-27T11:00:00Z")}
	later := &vrxv1.NatStaticMapping{Name: proto.String("web"), ExpiresAt: proto.String("2026-09-28T12:00:00Z")}
	if !natMappingExpired(s, expired, 0) || natMappingExpired(s, later, 1) {
		t.Fatal("expired/later mis-classified")
	}
	if len(s.issues) != 1 || !strings.HasPrefix(s.issues[0], "W /nat/staticMappings/0 rule.expired ") {
		t.Fatalf("issues %v", s.issues)
	}
	if p := ruleexpiry.Pending(); len(p) != 1 || !p[0].Equal(now.Add(24*time.Hour)) {
		t.Fatalf("noted %v", p)
	}
}
